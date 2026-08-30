package contract

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/timmyagentic/awesome-agent-app-features"

type adapter struct {
	Path string `json:"path"`
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
	Package string `json:"package"`
}

type manifest struct {
	ID             string          `json:"id"`
	Contract       string          `json:"contract"`
	ReleaseStatus  string          `json:"release_status"`
	Package        string          `json:"package"`
	Delivery       []deliveryItem  `json:"delivery"`
	RemoteExamples []remoteExample `json:"remote_examples"`
	Foundation     struct {
		Adapters []adapter `json:"adapters"`
	} `json:"foundation"`
}

func TestFeatureManifestPathsPointToCode(t *testing.T) {
	root := repositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "features", "*", "feature.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("repository has no Feature manifests")
	}

	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		var item manifest
		readJSON(t, path, &item)
		if _, exists := seen[item.ID]; exists {
			t.Errorf("duplicate feature id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if item.Package != "" {
			assertRepositoryPath(t, root, path, strings.TrimPrefix(item.Package, "./"), true)
		}

		for _, delivery := range item.Delivery {
			switch delivery.Mode {
			case "go-module":
				if delivery.Module != modulePath {
					t.Errorf("%s module = %q", path, delivery.Module)
				}
				for _, packagePath := range delivery.Packages {
					if !strings.HasPrefix(packagePath, modulePath+"/") {
						t.Errorf("%s package %q is outside the module", path, packagePath)
						continue
					}
					assertRepositoryPath(t, root, path, strings.TrimPrefix(packagePath, modulePath+"/"), true)
				}
			case "source-subtree":
				clean := filepath.ToSlash(filepath.Clean(delivery.Path))
				if clean != delivery.Path || strings.HasPrefix(clean, "../") {
					t.Errorf("%s source subtree path is not canonical: %q", path, delivery.Path)
					continue
				}
				localPath := assertRepositoryPath(t, root, path, delivery.Path, true)
				if delivery.Verify == "" {
					t.Errorf("%s source subtree has no verification entrypoint", path)
				} else {
					assertRepositoryPath(t, root, path, filepath.ToSlash(filepath.Join(delivery.Path, delivery.Verify)), false)
				}
				if err := filepath.WalkDir(localPath, func(entryPath string, entry fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".wrangler") {
						return filepath.SkipDir
					}
					if entry.Type()&os.ModeSymlink != 0 {
						t.Errorf("%s source subtree contains symlink %s", path, entryPath)
					}
					return nil
				}); err != nil {
					t.Errorf("%s walk source subtree: %v", path, err)
				}
				seenHostOwned := make(map[string]struct{}, len(delivery.HostOwnedFiles))
				for _, relative := range delivery.HostOwnedFiles {
					if strings.Contains(relative, "\\") || pathpkg.IsAbs(relative) || pathpkg.Clean(relative) != relative || relative == "." || strings.HasPrefix(relative, "../") {
						t.Errorf("%s host-owned file is not canonical: %q", path, relative)
						continue
					}
					if _, duplicate := seenHostOwned[relative]; duplicate {
						t.Errorf("%s duplicates host-owned file %q", path, relative)
					}
					seenHostOwned[relative] = struct{}{}
					file := assertRepositoryPath(t, root, path, filepath.ToSlash(filepath.Join(delivery.Path, relative)), false)
					info, err := os.Lstat(file)
					if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
						t.Errorf("%s host-owned file is unavailable or unsafe: %q", path, relative)
					}
				}
			}
		}

		for _, example := range item.RemoteExamples {
			if !strings.HasPrefix(example.Package, modulePath+"/") {
				t.Errorf("%s example %q is outside the module", path, example.Package)
				continue
			}
			assertRepositoryPath(t, root, path, strings.TrimPrefix(example.Package, modulePath+"/"), true)
		}
		for _, adapter := range item.Foundation.Adapters {
			assertRepositoryPath(t, root, path, strings.TrimPrefix(adapter.Path, "./"), false)
		}
	}
}

func assertRepositoryPath(t *testing.T, root, manifestPath, relative string, wantDirectory bool) string {
	t.Helper()
	localPath := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Stat(localPath)
	if err != nil || (wantDirectory && !info.IsDir()) {
		t.Errorf("%s references unavailable path %q", manifestPath, relative)
	}
	return localPath
}

func TestCorePackagesRemainHeadlessAndHostAgnostic(t *testing.T) {
	root := repositoryRoot(t)
	for _, packageName := range corePackageDirectories(t, root) {
		directory := filepath.Join(root, packageName)
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imported := range parsed.Imports {
				value, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(strings.Split(value, "/")[0], ".") {
					t.Errorf("core package %s imports non-standard dependency %q", packageName, value)
				}
			}

			lower := strings.ToLower(string(data))
			for _, forbidden := range []string{"feishu", "lark", "slack", "telegram", "discord", "cc-connect", "github", "gitlab", "cloudflare"} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("core source %s contains host/channel term %q", path, forbidden)
				}
			}
		}
	}
}

func corePackageDirectories(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	for _, item := range allManifests(t, root) {
		if item.Package != "" {
			result = append(result, strings.TrimPrefix(item.Package, "./"))
		}
	}
	sort.Strings(result)
	return result
}

func publicPackageDirectories(t *testing.T, root string) []string {
	t.Helper()
	seen := make(map[string]struct{})
	for _, item := range allManifests(t, root) {
		for _, delivery := range item.Delivery {
			if delivery.Mode != "go-module" {
				continue
			}
			for _, packagePath := range delivery.Packages {
				seen[strings.TrimPrefix(packagePath, modulePath+"/")] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for directory := range seen {
		result = append(result, directory)
	}
	sort.Strings(result)
	return result
}

func allManifests(t *testing.T, root string) []manifest {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "features", "*", "feature.json"))
	if err != nil {
		t.Fatal(err)
	}
	result := make([]manifest, 0, len(paths))
	for _, path := range paths {
		var item manifest
		readJSON(t, path, &item)
		result = append(result, item)
	}
	return result
}

func TestRepositoryDoesNotMasqueradeAsSkillCollection(t *testing.T) {
	root := repositoryRoot(t)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "SKILL.md") {
			t.Errorf("unexpected Skill entrypoint: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
