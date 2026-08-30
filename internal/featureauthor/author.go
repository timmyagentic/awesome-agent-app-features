// Package featureauthor implements the contributor-facing Feature catalog
// checks used by the CLI, tests, and release workflow.
package featureauthor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const modulePath = "github.com/timmyagentic/awesome-agent-app-features"

var (
	featureIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	versionPattern   = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

type Kind string

const (
	KindGo            Kind = "go"
	KindSourceSubtree Kind = "source-subtree"
)

type ScaffoldOptions struct {
	Root    string
	ID      string
	Name    string
	Kind    Kind
	Runtime string
}

type TagResolution struct {
	Exists            bool
	AncestorOfRelease bool
	File              []byte
}

type TagResolver func(tag, relativePath string) (TagResolution, error)

type entryDocument struct {
	ReleaseStatus string         `json:"release_status"`
	Since         *string        `json:"since"`
	Features      []entryFeature `json:"features"`
}

type entryFeature struct {
	ID       string `json:"id"`
	Manifest string `json:"manifest"`
	Readme   string `json:"readme"`
	Contract string `json:"contract"`
}

type manifestDocument struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Maturity       string          `json:"maturity"`
	Contract       string          `json:"contract"`
	ReleaseStatus  string          `json:"release_status"`
	Since          *string         `json:"since"`
	Package        string          `json:"package,omitempty"`
	Delivery       []deliveryItem  `json:"delivery"`
	RemoteExamples []remoteExample `json:"remote_examples"`
	Foundation     struct {
		Adapters []adapterItem `json:"adapters"`
	} `json:"foundation"`
}

type deliveryItem struct {
	Mode           string   `json:"mode"`
	Module         string   `json:"module,omitempty"`
	Packages       []string `json:"packages,omitempty"`
	Path           string   `json:"path,omitempty"`
	HostOwnedFiles []string `json:"host_owned_files,omitempty"`
	Verify         string   `json:"verify,omitempty"`
}

type remoteExample struct {
	Mode    string `json:"mode"`
	Package string `json:"package"`
	Network string `json:"network"`
}

type adapterItem struct {
	Path string `json:"path"`
}

func Validate(root string) error {
	root, err := normalizedRoot(root)
	if err != nil {
		return err
	}
	var entry entryDocument
	if err := readJSON(filepath.Join(root, "features", "index.json"), &entry); err != nil {
		return fmt.Errorf("read remote entry: %w", err)
	}
	if err := validatePublication(entry.ReleaseStatus, entry.Since, "remote entry"); err != nil {
		return err
	}
	manifestPaths, err := filepath.Glob(filepath.Join(root, "features", "*", "feature.json"))
	if err != nil {
		return err
	}
	if len(manifestPaths) == 0 || len(entry.Features) != len(manifestPaths) {
		return fmt.Errorf("remote entry has %d features but repository has %d manifests", len(entry.Features), len(manifestPaths))
	}
	seen := make(map[string]struct{}, len(entry.Features))
	for _, feature := range entry.Features {
		if !featureIDPattern.MatchString(feature.ID) {
			return fmt.Errorf("invalid feature id %q", feature.ID)
		}
		if feature.Contract != "v1" {
			return fmt.Errorf("feature %q has invalid contract %q", feature.ID, feature.Contract)
		}
		if _, duplicate := seen[feature.ID]; duplicate {
			return fmt.Errorf("duplicate feature id %q", feature.ID)
		}
		seen[feature.ID] = struct{}{}
		wantManifest := "features/" + feature.ID + "/feature.json"
		wantReadme := "features/" + feature.ID + "/README.md"
		if feature.Manifest != wantManifest || feature.Readme != wantReadme {
			return fmt.Errorf("feature %q entry paths do not match its id", feature.ID)
		}
		if err := requireRegular(root, feature.Readme); err != nil {
			return fmt.Errorf("feature %q README: %w", feature.ID, err)
		}
		var manifest manifestDocument
		if err := readJSON(filepath.Join(root, filepath.FromSlash(feature.Manifest)), &manifest); err != nil {
			return fmt.Errorf("read feature %q manifest: %w", feature.ID, err)
		}
		if manifest.ID != feature.ID || manifest.Contract != feature.Contract {
			return fmt.Errorf("feature %q entry and manifest disagree", feature.ID)
		}
		if strings.TrimSpace(manifest.Name) == "" {
			return fmt.Errorf("feature %q name is required", feature.ID)
		}
		if err := validatePublication(manifest.ReleaseStatus, manifest.Since, "feature "+feature.ID); err != nil {
			return err
		}
		if err := validateManifestPaths(root, manifest); err != nil {
			return fmt.Errorf("feature %q: %w", feature.ID, err)
		}
	}
	for _, path := range manifestPaths {
		id := filepath.Base(filepath.Dir(path))
		if _, registered := seen[id]; !registered {
			return fmt.Errorf("manifest for Feature %q is not registered", id)
		}
	}
	if err := validateReadmeCatalogs(root); err != nil {
		return err
	}
	return nil
}

func validateManifestPaths(root string, manifest manifestDocument) error {
	hasGoDelivery := false
	for _, delivery := range manifest.Delivery {
		switch delivery.Mode {
		case "go-module":
			hasGoDelivery = true
			if delivery.Module != modulePath || len(delivery.Packages) == 0 {
				return errors.New("go-module delivery must name this module and at least one package")
			}
			for _, packagePath := range delivery.Packages {
				if !strings.HasPrefix(packagePath, modulePath+"/") {
					return fmt.Errorf("package %q is outside the module", packagePath)
				}
				if err := requireDirectory(root, strings.TrimPrefix(packagePath, modulePath+"/")); err != nil {
					return err
				}
			}
		case "source-subtree":
			if !safeRelative(delivery.Path) {
				return fmt.Errorf("source subtree path %q is unsafe", delivery.Path)
			}
			if err := requireDirectory(root, delivery.Path); err != nil {
				return err
			}
			if !safeRelative(delivery.Verify) {
				return errors.New("source-subtree delivery must declare a safe verification entrypoint")
			}
			if err := requireRegular(root, filepath.ToSlash(filepath.Join(delivery.Path, delivery.Verify))); err != nil {
				return fmt.Errorf("source-subtree verification entrypoint: %w", err)
			}
			if err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(delivery.Path)), func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".wrangler") {
					return filepath.SkipDir
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("source subtree contains symlink %s", path)
				}
				return nil
			}); err != nil {
				return err
			}
			for _, relative := range delivery.HostOwnedFiles {
				if !safeRelative(relative) {
					return fmt.Errorf("host-owned path %q is unsafe", relative)
				}
				if err := requireRegular(root, filepath.ToSlash(filepath.Join(delivery.Path, relative))); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported delivery mode %q", delivery.Mode)
		}
	}
	if hasGoDelivery {
		if !strings.HasPrefix(manifest.Package, "./") {
			return errors.New("go-module Feature must declare its core package")
		}
		if err := requireDirectory(root, strings.TrimPrefix(manifest.Package, "./")); err != nil {
			return err
		}
		hasZeroNetworkExample := false
		for _, example := range manifest.RemoteExamples {
			if example.Mode == "go-run" && example.Network == "none" {
				hasZeroNetworkExample = true
			}
		}
		if !hasZeroNetworkExample {
			return errors.New("go-module feature must declare a zero-network go-run example")
		}
	} else if manifest.Package != "" {
		return errors.New("source-subtree-only Feature must not declare a synthetic package")
	}
	for _, example := range manifest.RemoteExamples {
		if example.Mode != "go-run" || !strings.HasPrefix(example.Package, modulePath+"/examples/") {
			return fmt.Errorf("invalid remote example %q", example.Package)
		}
		if err := requireDirectory(root, strings.TrimPrefix(example.Package, modulePath+"/")); err != nil {
			return err
		}
	}
	for _, adapter := range manifest.Foundation.Adapters {
		if !strings.HasPrefix(adapter.Path, "./") {
			return fmt.Errorf("adapter path %q must be repository-relative", adapter.Path)
		}
		relative := strings.TrimPrefix(adapter.Path, "./")
		if err := requireExistingPath(root, relative); err != nil {
			return fmt.Errorf("adapter path %q: %w", adapter.Path, err)
		}
	}
	return nil
}

func ValidateRelease(root, currentTag string, resolve TagResolver) error {
	if resolve == nil {
		return errors.New("tag resolver is required")
	}
	if _, ok := parseVersion(currentTag); !ok {
		return fmt.Errorf("release tag %q is not exact SemVer", currentTag)
	}
	if err := Validate(root); err != nil {
		return err
	}
	root, _ = normalizedRoot(root)
	var entry entryDocument
	if err := readJSON(filepath.Join(root, "features", "index.json"), &entry); err != nil {
		return err
	}
	if err := validateIntroduction("remote entry", entry.ReleaseStatus, entry.Since, currentTag, "features/index.json", "", "v1", resolve); err != nil {
		return err
	}
	for _, feature := range entry.Features {
		var manifest manifestDocument
		if err := readJSON(filepath.Join(root, filepath.FromSlash(feature.Manifest)), &manifest); err != nil {
			return err
		}
		if err := validateIntroduction("feature "+feature.ID, manifest.ReleaseStatus, manifest.Since, currentTag, feature.Manifest, feature.ID, feature.Contract, resolve); err != nil {
			return err
		}
	}
	return nil
}

func validateIntroduction(label, status string, since *string, currentTag, relativePath, expectedID, expectedContract string, resolve TagResolver) error {
	if status == "unreleased" {
		if since != nil {
			return fmt.Errorf("%s is unreleased but has an introduction tag", label)
		}
		return nil
	}
	if status != "released" || since == nil {
		return fmt.Errorf("%s release metadata is invalid", label)
	}
	introduced, ok := parseVersion(*since)
	if !ok {
		return fmt.Errorf("%s introduction tag %q is invalid", label, *since)
	}
	current, _ := parseVersion(currentTag)
	if compareVersion(introduced, current) > 0 {
		return fmt.Errorf("%s introduction tag %s is newer than %s", label, *since, currentTag)
	}
	resolution, err := resolve(*since, relativePath)
	if err != nil {
		return fmt.Errorf("resolve %s introduction tag: %w", label, err)
	}
	if !resolution.Exists || !resolution.AncestorOfRelease {
		return fmt.Errorf("%s introduction tag %s does not exist on the release history", label, *since)
	}
	if len(resolution.File) == 0 {
		return fmt.Errorf("%s did not exist at its claimed introduction tag %s", label, *since)
	}
	if expectedID == "" {
		var introduced entryDocument
		if err := json.Unmarshal(resolution.File, &introduced); err != nil || introduced.ReleaseStatus != "released" || introduced.Since == nil || *introduced.Since != *since {
			return fmt.Errorf("%s metadata did not match its claimed introduction tag %s", label, *since)
		}
	} else {
		var introduced manifestDocument
		if err := json.Unmarshal(resolution.File, &introduced); err != nil || introduced.ID != expectedID || introduced.Contract != expectedContract || introduced.ReleaseStatus != "released" || introduced.Since == nil || *introduced.Since != *since {
			return fmt.Errorf("%s manifest did not match its claimed introduction tag %s", label, *since)
		}
	}
	return nil
}

func Scaffold(options ScaffoldOptions) error {
	root, err := normalizedRoot(options.Root)
	if err != nil {
		return err
	}
	options.ID = strings.TrimSpace(options.ID)
	options.Name = strings.TrimSpace(options.Name)
	options.Runtime = strings.TrimSpace(options.Runtime)
	if !featureIDPattern.MatchString(options.ID) || options.Name == "" || strings.ContainsAny(options.Name, "\r\n") {
		return errors.New("feature id and name are required; id must use lowercase kebab-case")
	}
	if options.Kind != KindGo && options.Kind != KindSourceSubtree {
		return fmt.Errorf("unsupported feature kind %q", options.Kind)
	}
	if options.Runtime == "" {
		if options.Kind == KindGo {
			options.Runtime = "go"
		} else {
			options.Runtime = "source"
		}
	}
	featureDirectory := filepath.Join(root, "features", options.ID)
	targets := []string{featureDirectory}
	if options.Kind == KindGo {
		targets = append(targets, filepath.Join(root, filepath.FromSlash(options.ID)), filepath.Join(root, "examples", options.ID))
	}
	for _, target := range targets {
		if _, err := os.Stat(target); err == nil || !os.IsNotExist(err) {
			return fmt.Errorf("feature %q target already exists: %s", options.ID, target)
		}
	}
	var raw map[string]any
	indexPath := filepath.Join(root, "features", "index.json")
	if err := readJSON(indexPath, &raw); err != nil {
		return err
	}
	features, ok := raw["features"].([]any)
	if !ok {
		return errors.New("remote entry features are invalid")
	}
	for _, value := range features {
		item, _ := value.(map[string]any)
		if item["id"] == options.ID {
			return fmt.Errorf("feature %q is already registered", options.ID)
		}
	}
	if err := os.MkdirAll(featureDirectory, 0o755); err != nil {
		return err
	}
	manifest := scaffoldManifest(options)
	if options.Kind == KindGo {
		packageDirectory := filepath.Join(root, filepath.FromSlash(options.ID))
		exampleDirectory := filepath.Join(root, "examples", options.ID)
		if err := os.MkdirAll(packageDirectory, 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(exampleDirectory, 0o755); err != nil {
			return err
		}
		packageName := strings.ReplaceAll(options.ID, "-", "_")
		if err := os.WriteFile(filepath.Join(packageDirectory, "doc.go"), []byte("// Package "+packageName+" implements the provider-neutral "+options.Name+" Feature.\npackage "+packageName+"\n"), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(exampleDirectory, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println("+strconv.Quote(options.Name+" preview")+") }\n"), 0o644); err != nil {
			return err
		}
	} else {
		sourceDirectory := filepath.Join(featureDirectory, "source")
		if err := os.MkdirAll(sourceDirectory, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(sourceDirectory, "README.md"), []byte("# "+options.Name+" source subtree\n\nReplace this reference content with a self-contained, independently verifiable delivery.\n"), 0o644); err != nil {
			return err
		}
		verify := "#!/bin/sh\nset -eu\nsource_root=$(CDPATH='' cd -- \"$(dirname \"$0\")\" && pwd)\ntest -f \"$source_root/README.md\"\n"
		if err := os.WriteFile(filepath.Join(sourceDirectory, "verify.sh"), []byte(verify), 0o755); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(featureDirectory, "feature.json"), manifest); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(featureDirectory, "README.md"), []byte(scaffoldReadme(options)), 0o644); err != nil {
		return err
	}
	features = append(features, map[string]any{
		"id": options.ID, "manifest": "features/" + options.ID + "/feature.json", "readme": "features/" + options.ID + "/README.md", "contract": "v1",
	})
	sort.Slice(features, func(i, j int) bool {
		left, _ := features[i].(map[string]any)
		right, _ := features[j].(map[string]any)
		return fmt.Sprint(left["id"]) < fmt.Sprint(right["id"])
	})
	raw["features"] = features
	if err := writeJSON(indexPath, raw); err != nil {
		return err
	}
	if err := SyncReadmes(root); err != nil {
		return err
	}
	return Validate(root)
}

func scaffoldManifest(options ScaffoldOptions) map[string]any {
	delivery := []any{}
	examples := []any{}
	adapters := []any{}
	manifest := map[string]any{
		"$schema": "../feature.schema.json", "schema": 1, "id": options.ID, "name": options.Name,
		"maturity": "mvp", "contract": "v1", "release_status": "unreleased", "since": nil,
		"runtime": []string{options.Runtime}, "integration_model": "agent-assisted-code-change",
		"prerequisites":     []string{"The host can map and render this Feature without moving product policy into the Foundation."},
		"invariants":        []string{"The reusable core remains provider- and UI-independent."},
		"integration_steps": []string{"Resolve one CI-successful commit and map every declared responsibility into the host."},
		"verification": map[string]any{
			"remote":     []string{"Resolve and inspect every delivery from one commit SHA."},
			"foundation": []string{"Run the Feature's focused tests and the complete Foundation verification."},
			"host":       []string{"Run focused host adapter tests and the host's complete verification."},
		},
	}
	if options.Kind == KindGo {
		manifest["package"] = "./" + options.ID
		delivery = append(delivery, map[string]any{
			"mode": "go-module", "pin": "resolved-commit", "module": modulePath, "minimum_go": "1.25.0", "packages": []string{modulePath + "/" + options.ID},
		})
		examples = append(examples, map[string]any{
			"id": options.ID + "-preview", "mode": "go-run", "package": modulePath + "/examples/" + options.ID, "network": "none", "side_effects": "none",
		})
	} else {
		source := "features/" + options.ID + "/source"
		delivery = append(delivery, map[string]any{
			"mode": "source-subtree", "pin": "resolved-commit", "path": source, "destination": "host-infrastructure", "host_owned_files": []string{}, "verify": "verify.sh",
		})
		adapters = append(adapters, map[string]any{"path": "./" + source, "role": "Reference source-subtree adapter."})
	}
	manifest["delivery"] = delivery
	manifest["remote_examples"] = examples
	manifest["foundation"] = map[string]any{
		"core":     []string{"Provider-neutral values and safety invariants for " + options.Name + "."},
		"adapters": adapters,
		"host":     []string{"Choose product context, authorization, rendering, and failure UX."},
		"excludes": []string{"Product UI, channel commands, localization, credentials, and deployment policy."},
	}
	return manifest
}

func scaffoldReadme(options ScaffoldOptions) string {
	return "# " + options.Name + "\n\n" +
		"Status: unreleased MVP scaffold.\n\n" +
		"Document the provider-neutral core, generic adapters, host responsibilities, exclusions, invariants, and focused verification before changing `release_status`.\n"
}

func validatePublication(status string, since *string, label string) error {
	switch status {
	case "unreleased":
		if since != nil {
			return fmt.Errorf("%s is unreleased but since is not null", label)
		}
	case "released":
		if since == nil || !versionPattern.MatchString(*since) {
			return fmt.Errorf("%s is released without an exact introduction tag", label)
		}
	default:
		return fmt.Errorf("%s has invalid release_status %q", label, status)
	}
	return nil
}

func normalizedRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("repository root is unavailable")
	}
	return absolute, nil
}

func safeRelative(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") || filepath.ToSlash(filepath.Clean(value)) != value {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "." || component == ".." {
			return false
		}
	}
	return true
}

func requireDirectory(root, relative string) error {
	if !safeRelative(relative) {
		return fmt.Errorf("path %q is unsafe", relative)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("directory %q is unavailable", relative)
	}
	return nil
}

func requireRegular(root, relative string) error {
	if !safeRelative(relative) {
		return fmt.Errorf("path %q is unsafe", relative)
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("file %q is unavailable or unsafe", relative)
	}
	return nil
}

func requireExistingPath(root, relative string) error {
	if !safeRelative(relative) {
		return errors.New("path is unsafe")
	}
	current := root
	for _, component := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return errors.New("path is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("path contains a symlink")
		}
	}
	return nil
}

func readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, destination)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".feature-author-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

type semanticVersion [3]int

func parseVersion(value string) (semanticVersion, bool) {
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return semanticVersion{}, false
	}
	var result semanticVersion
	for index := range result {
		number, err := strconv.Atoi(match[index+1])
		if err != nil {
			return semanticVersion{}, false
		}
		result[index] = number
	}
	return result, true
}

func compareVersion(left, right semanticVersion) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}
