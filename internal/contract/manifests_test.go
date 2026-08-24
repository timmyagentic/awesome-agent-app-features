package contract

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/timmyagentic/awesome-agent-app-features"

type adapter struct {
	Path string `json:"path"`
}

type deliveryItem struct {
	Mode     string   `json:"mode"`
	Module   string   `json:"module,omitempty"`
	Packages []string `json:"packages,omitempty"`
	Path     string   `json:"path,omitempty"`
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
	if len(paths) != 2 {
		t.Fatalf("feature manifest count = %d, want 2", len(paths))
	}

	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		var item manifest
		readJSON(t, path, &item)
		if _, exists := seen[item.ID]; exists {
			t.Errorf("duplicate feature id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		assertRepositoryPath(t, root, path, strings.TrimPrefix(item.Package, "./"), true)

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
	for _, packageName := range []string{"feedback", "updater"} {
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
