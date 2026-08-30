package contract

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/timmyagentic/awesome-agent-app-features"

type deliveryItem struct {
	Mode     string   `json:"mode"`
	Packages []string `json:"packages,omitempty"`
}

type manifest struct {
	ID       string         `json:"id"`
	Contract string         `json:"contract"`
	Package  string         `json:"package"`
	Delivery []deliveryItem `json:"delivery"`
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
