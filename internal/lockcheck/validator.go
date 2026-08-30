package lockcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var fullCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

const maxContractJSONBytes = 4 * 1024 * 1024

type ModuleInfo struct {
	Version   string
	Directory string
	Replaced  bool
}

type ModuleResolver func(context.Context, string, string) (ModuleInfo, error)

type Options struct {
	LockPath      string
	HostRoot      string
	SourceRoot    string
	SourceCommit  string
	ResolveModule ModuleResolver
	Now           func() time.Time
}

type Report struct {
	Features       int
	Files          int
	GoModules      int
	SourceSubtrees int
}

type lockDocument struct {
	Schema   int           `json:"schema"`
	Source   lockSource    `json:"source"`
	Features []lockFeature `json:"features"`
}

type lockSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

type lockFeature struct {
	ID         string         `json:"id"`
	Contract   string         `json:"contract"`
	Deliveries []lockDelivery `json:"deliveries"`
	Files      []string       `json:"files"`
	VerifiedAt string         `json:"verified_at"`
	Checks     []string       `json:"checks"`
	Unverified []string       `json:"unverified"`
}

type lockDelivery struct {
	Mode    string `json:"mode"`
	Source  string `json:"source"`
	Target  string `json:"target"`
	Version string `json:"version,omitempty"`
}

type remoteEntry struct {
	Module     string `json:"module"`
	Contract   string `json:"contract"`
	Resolution struct {
		Repository string `json:"repository"`
	} `json:"resolution"`
	Features []entryFeature `json:"features"`
}

type entryFeature struct {
	ID       string `json:"id"`
	Manifest string `json:"manifest"`
	Contract string `json:"contract"`
}

type featureManifest struct {
	ID            string             `json:"id"`
	Contract      string             `json:"contract"`
	ReleaseStatus string             `json:"release_status"`
	Since         *string            `json:"since"`
	Delivery      []manifestDelivery `json:"delivery"`
}

type manifestDelivery struct {
	Mode           string   `json:"mode"`
	Module         string   `json:"module,omitempty"`
	Packages       []string `json:"packages,omitempty"`
	Path           string   `json:"path,omitempty"`
	HostOwnedFiles []string `json:"host_owned_files,omitempty"`
	Verify         string   `json:"verify,omitempty"`
}

func Validate(ctx context.Context, options Options) (Report, error) {
	var report Report
	if err := validateOptions(&options); err != nil {
		return report, err
	}

	var lock lockDocument
	if err := readStrictJSON(options.LockPath, &lock); err != nil {
		return report, fmt.Errorf("read host lock: %w", err)
	}
	if lock.Schema != 1 || len(lock.Features) == 0 {
		return report, fmt.Errorf("host lock must contain schema 1 and at least one feature")
	}
	if lock.Source.Commit != options.SourceCommit {
		return report, fmt.Errorf("source commit mismatch: lock has %s, resolved source is %s", lock.Source.Commit, options.SourceCommit)
	}

	var entry remoteEntry
	if err := readJSON(filepath.Join(options.SourceRoot, "features", "index.json"), &entry); err != nil {
		return report, fmt.Errorf("read remote entry: %w", err)
	}
	if lock.Source.Repository != entry.Resolution.Repository {
		return report, fmt.Errorf("source repository mismatch: lock has %q, entry declares %q", lock.Source.Repository, entry.Resolution.Repository)
	}
	if strings.TrimSpace(entry.Module) == "" || strings.TrimSpace(entry.Contract) == "" {
		return report, fmt.Errorf("remote entry module or contract is empty")
	}

	entryFeatures := make(map[string]entryFeature, len(entry.Features))
	for _, feature := range entry.Features {
		if _, exists := entryFeatures[feature.ID]; exists {
			return report, fmt.Errorf("remote entry contains duplicate feature %q", feature.ID)
		}
		entryFeatures[feature.ID] = feature
	}

	seenFeatures := make(map[string]struct{}, len(lock.Features))
	moduleCache := make(map[string]ModuleInfo)
	checkedModuleFiles := make(map[string]struct{})
	for _, feature := range lock.Features {
		if _, exists := seenFeatures[feature.ID]; exists {
			return report, fmt.Errorf("duplicate feature %q in host lock", feature.ID)
		}
		seenFeatures[feature.ID] = struct{}{}
		entryFeature, exists := entryFeatures[feature.ID]
		if !exists {
			return report, fmt.Errorf("feature %q is not declared by the pinned entry", feature.ID)
		}
		if feature.Contract != entryFeature.Contract {
			return report, fmt.Errorf("feature %q contract does not match the pinned entry", feature.ID)
		}

		manifestPath, err := joinedPath(options.SourceRoot, entryFeature.Manifest)
		if err != nil {
			return report, fmt.Errorf("feature %q manifest path: %w", feature.ID, err)
		}
		var manifest featureManifest
		if err := readJSON(manifestPath, &manifest); err != nil {
			return report, fmt.Errorf("read feature %q manifest: %w", feature.ID, err)
		}
		if manifest.ID != feature.ID || manifest.Contract != feature.Contract {
			return report, fmt.Errorf("feature %q does not match its pinned manifest", feature.ID)
		}
		if manifest.ReleaseStatus != "released" || manifest.Since == nil || strings.TrimSpace(*manifest.Since) == "" {
			return report, fmt.Errorf("feature %q is unreleased and cannot be recorded as a stable host integration", feature.ID)
		}

		declaredPackages := make(map[string]string)
		declaredSubtrees := make(map[string][]string)
		for _, delivery := range manifest.Delivery {
			switch delivery.Mode {
			case "go-module":
				for _, packagePath := range delivery.Packages {
					declaredPackages[packagePath] = delivery.Module
				}
			case "source-subtree":
				declaredSubtrees[delivery.Path] = delivery.HostOwnedFiles
			}
		}

		if len(feature.Deliveries) == 0 || len(feature.Files) == 0 || len(feature.Checks) == 0 {
			return report, fmt.Errorf("feature %q must record deliveries, host files, and checks", feature.ID)
		}
		verifiedAt, err := time.Parse(time.RFC3339, feature.VerifiedAt)
		if err != nil {
			return report, fmt.Errorf("feature %q verified_at is invalid", feature.ID)
		}
		if verifiedAt.After(options.Now().Add(5 * time.Minute)) {
			return report, fmt.Errorf("feature %q verified_at is in the future", feature.ID)
		}

		seenDeliveries := make(map[string]struct{}, len(feature.Deliveries))
		for _, delivery := range feature.Deliveries {
			key := strings.Join([]string{delivery.Mode, delivery.Source, delivery.Target, delivery.Version}, "\x00")
			if _, exists := seenDeliveries[key]; exists {
				return report, fmt.Errorf("feature %q contains duplicate delivery %q", feature.ID, delivery.Source)
			}
			seenDeliveries[key] = struct{}{}
			switch delivery.Mode {
			case "go-module":
				modulePath, declared := declaredPackages[delivery.Source]
				if !declared {
					return report, fmt.Errorf("go package %q is not declared by feature %q", delivery.Source, feature.ID)
				}
				if modulePath != entry.Module || delivery.Target != "go.mod" || strings.TrimSpace(delivery.Version) == "" {
					return report, fmt.Errorf("go package %q has inconsistent module delivery metadata", delivery.Source)
				}
				module, ok := moduleCache[modulePath]
				if !ok {
					module, err = options.ResolveModule(ctx, options.HostRoot, modulePath)
					if err != nil {
						return report, fmt.Errorf("resolve Go module %s: %w", modulePath, err)
					}
					moduleCache[modulePath] = module
				}
				if module.Replaced {
					return report, fmt.Errorf("go module %s uses a forbidden local replace", modulePath)
				}
				if module.Version != delivery.Version {
					return report, fmt.Errorf("module version mismatch for %s: lock has %s, host has %s", modulePath, delivery.Version, module.Version)
				}
				packageRelative := strings.TrimPrefix(delivery.Source, modulePath+"/")
				contentKey := modulePath + "\x00" + packageRelative
				if _, checked := checkedModuleFiles[contentKey]; !checked {
					if err := compareModuleContent(options.SourceRoot, module.Directory, packageRelative); err != nil {
						return report, fmt.Errorf("module content mismatch for %s: %w", delivery.Source, err)
					}
					checkedModuleFiles[contentKey] = struct{}{}
				}
			case "source-subtree":
				hostOwnedFiles, declared := declaredSubtrees[delivery.Source]
				if !declared {
					return report, fmt.Errorf("source subtree %q is not declared by feature %q", delivery.Source, feature.ID)
				}
				if err := requireDirectory(options.SourceRoot, delivery.Source, "declared source subtree"); err != nil {
					return report, err
				}
				if err := requireDirectory(options.HostRoot, delivery.Target, "source-subtree target"); err != nil {
					return report, err
				}
				if err := compareSourceSubtreeContent(options.SourceRoot, delivery.Source, options.HostRoot, delivery.Target, hostOwnedFiles); err != nil {
					return report, fmt.Errorf("source-subtree content mismatch for %q: %w", delivery.Source, err)
				}
				report.SourceSubtrees++
			default:
				return report, fmt.Errorf("feature %q uses unsupported delivery mode %q", feature.ID, delivery.Mode)
			}
		}

		seenFiles := make(map[string]struct{}, len(feature.Files))
		for _, relative := range feature.Files {
			if _, exists := seenFiles[relative]; exists {
				return report, fmt.Errorf("feature %q contains duplicate host file %q", feature.ID, relative)
			}
			seenFiles[relative] = struct{}{}
			path, err := joinedPath(options.HostRoot, relative)
			if err != nil {
				return report, fmt.Errorf("host file %q: %w", relative, err)
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return report, fmt.Errorf("host file %q is missing or not a regular non-symlink file", relative)
			}
			report.Files++
		}
		report.Features++
	}
	report.GoModules = len(moduleCache)
	return report, nil
}

func validateOptions(options *Options) error {
	if options == nil {
		return fmt.Errorf("validation options are required")
	}
	if strings.TrimSpace(options.HostRoot) == "" || strings.TrimSpace(options.SourceRoot) == "" || strings.TrimSpace(options.LockPath) == "" {
		return fmt.Errorf("lock, host root, and source root are required")
	}
	hostRoot, err := filepath.Abs(options.HostRoot)
	if err != nil {
		return fmt.Errorf("resolve host root: %w", err)
	}
	sourceRoot, err := filepath.Abs(options.SourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	lockPath := options.LockPath
	if !filepath.IsAbs(lockPath) {
		lockPath = filepath.Join(hostRoot, lockPath)
	}
	lockPath, err = filepath.Abs(lockPath)
	if err != nil {
		return fmt.Errorf("resolve lock: %w", err)
	}
	relativeLock, err := filepath.Rel(hostRoot, lockPath)
	if err != nil || relativeLock == ".." || strings.HasPrefix(relativeLock, ".."+string(filepath.Separator)) {
		return fmt.Errorf("lock must be inside the host root")
	}
	options.HostRoot = hostRoot
	options.SourceRoot = sourceRoot
	options.LockPath = lockPath
	if !fullCommitPattern.MatchString(options.SourceCommit) || options.SourceCommit == strings.Repeat("0", 40) {
		return fmt.Errorf("resolved source commit must be a non-zero 40-character lowercase SHA")
	}
	if options.ResolveModule == nil {
		return fmt.Errorf("go module resolver is required")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return nil
}

func readStrictJSON(path string, destination any) error {
	return readJSONWithMode(path, destination, true)
}

func readJSON(path string, destination any) error {
	return readJSONWithMode(path, destination, false)
}

func readJSONWithMode(path string, destination any, strict bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxContractJSONBytes {
		return fmt.Errorf("json exceeds %d bytes", maxContractJSONBytes)
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxContractJSONBytes+1))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("json contains more than one value")
		}
		return err
	}
	return nil
}

func joinedPath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be a non-empty relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes its root")
	}
	path := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes its root")
	}
	return path, nil
}

func requireDirectory(root, relative, label string) error {
	_, err := checkedPath(root, relative, true)
	if err != nil {
		return fmt.Errorf("%s %q is missing or unsafe: %w", label, relative, err)
	}
	return nil
}

func requireRegularFile(root, relative, label string) (string, error) {
	file, err := checkedPath(root, relative, false)
	if err != nil {
		return "", fmt.Errorf("%s %q is missing or unsafe: %w", label, relative, err)
	}
	return file, nil
}

func checkedPath(root, relative string, wantDirectory bool) (string, error) {
	resolved, err := joinedPath(root, relative)
	if err != nil {
		return "", err
	}
	current := root
	parts := strings.Split(filepath.Clean(filepath.FromSlash(relative)), string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path component %q is a symlink", filepath.ToSlash(strings.Join(parts[:index+1], string(filepath.Separator))))
		}
		last := index == len(parts)-1
		if !last || wantDirectory {
			if !info.IsDir() {
				return "", fmt.Errorf("path component %q is not a directory", filepath.ToSlash(strings.Join(parts[:index+1], string(filepath.Separator))))
			}
		} else if !info.Mode().IsRegular() {
			return "", fmt.Errorf("path is not a regular file")
		}
	}
	return resolved, nil
}

func compareModuleContent(sourceRoot, moduleRoot, relative string) error {
	if strings.TrimSpace(moduleRoot) == "" {
		return fmt.Errorf("resolved module directory is empty")
	}
	for _, path := range []string{"go.mod", relative} {
		sourceHashes, err := treeHashes(sourceRoot, path)
		if err != nil {
			return fmt.Errorf("source %s: %w", path, err)
		}
		moduleHashes, err := treeHashes(moduleRoot, path)
		if err != nil {
			return fmt.Errorf("module %s: %w", path, err)
		}
		if len(sourceHashes) != len(moduleHashes) {
			return fmt.Errorf("%s file set differs", path)
		}
		for name, digest := range sourceHashes {
			if moduleHashes[name] != digest {
				return fmt.Errorf("%s differs", filepath.ToSlash(filepath.Join(path, name)))
			}
		}
	}
	return nil
}

func compareSourceSubtreeContent(sourceRoot, sourceRelative, hostRoot, targetRelative string, hostOwnedFiles []string) error {
	sourceStart, err := joinedPath(sourceRoot, sourceRelative)
	if err != nil {
		return fmt.Errorf("source path: %w", err)
	}
	targetStart, err := joinedPath(hostRoot, targetRelative)
	if err != nil {
		return fmt.Errorf("target path: %w", err)
	}

	hostOwned := make(map[string]struct{}, len(hostOwnedFiles))
	for _, relative := range hostOwnedFiles {
		if strings.Contains(relative, "\\") || path.IsAbs(relative) || path.Clean(relative) != relative || relative == "." || strings.HasPrefix(relative, "../") {
			return fmt.Errorf("host-owned file %q is not a canonical relative path", relative)
		}
		if _, duplicate := hostOwned[relative]; duplicate {
			return fmt.Errorf("host-owned file %q is declared more than once", relative)
		}
		for _, location := range []struct {
			label string
			root  string
		}{{"source", sourceStart}, {"target", targetStart}} {
			if _, err := requireRegularFile(location.root, relative, location.label+" host-owned file"); err != nil {
				return err
			}
		}
		hostOwned[relative] = struct{}{}
	}

	return filepath.WalkDir(sourceStart, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink %s is not allowed", sourcePath)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source non-regular file %s is not allowed", sourcePath)
		}
		relative, err := filepath.Rel(sourceStart, sourcePath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, mutable := hostOwned[relative]; mutable {
			return nil
		}
		targetPath, err := requireRegularFile(targetStart, relative, "target file")
		if err != nil {
			return err
		}
		sourceDigest, err := fileDigest(sourcePath)
		if err != nil {
			return fmt.Errorf("hash source file %q: %w", relative, err)
		}
		targetDigest, err := fileDigest(targetPath)
		if err != nil {
			return fmt.Errorf("hash target file %q: %w", relative, err)
		}
		if sourceDigest != targetDigest {
			return fmt.Errorf("target file %q differs from the pinned source", relative)
		}
		return nil
	})
}

func treeHashes(root, relative string) (map[string]string, error) {
	start, err := joinedPath(root, relative)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(start)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlink is not allowed")
	}
	result := make(map[string]string)
	if info.Mode().IsRegular() {
		digest, err := fileDigest(start)
		if err != nil {
			return nil, err
		}
		result[filepath.Base(start)] = digest
		return result, nil
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a regular file or directory")
	}
	err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink %s is not allowed", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular file %s is not allowed", path)
		}
		rel, err := filepath.Rel(start, path)
		if err != nil {
			return err
		}
		digest, err := fileDigest(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = digest
		return nil
	})
	return result, err
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
