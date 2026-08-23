package contract

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type manifest struct {
	Schema           int      `json:"schema"`
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Maturity         string   `json:"maturity"`
	Runtime          []string `json:"runtime"`
	Package          string   `json:"package"`
	Prerequisites    []string `json:"prerequisites"`
	Invariants       []string `json:"invariants"`
	IntegrationSteps []string `json:"integration_steps"`
	Verification     []string `json:"verification"`
}

func TestFeatureManifestsAreCompleteAndPointToCode(t *testing.T) {
	root := repositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "features", "*", "feature.json"))
	if err != nil {
		t.Fatalf("glob manifests: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("feature manifest count = %d, want 2", len(paths))
	}
	seen := make(map[string]struct{})
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var item manifest
		if err := json.Unmarshal(data, &item); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if item.Schema != 1 || item.ID == "" || item.Name == "" || item.Maturity != "mvp" {
			t.Errorf("incomplete identity in %s: %+v", path, item)
		}
		if _, exists := seen[item.ID]; exists {
			t.Errorf("duplicate feature id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		for label, values := range map[string][]string{
			"runtime":           item.Runtime,
			"prerequisites":     item.Prerequisites,
			"invariants":        item.Invariants,
			"integration_steps": item.IntegrationSteps,
			"verification":      item.Verification,
		} {
			if len(values) == 0 {
				t.Errorf("%s has no %s", path, label)
			}
			for _, value := range values {
				if strings.TrimSpace(value) == "" {
					t.Errorf("%s contains empty %s", path, label)
				}
			}
		}
		packagePath := filepath.Join(root, strings.TrimPrefix(item.Package, "./"))
		if info, err := os.Stat(packagePath); err != nil || !info.IsDir() {
			t.Errorf("%s package path %s is unavailable", path, packagePath)
		}
	}
}

func TestRepositoryDoesNotMasqueradeAsSkillCollection(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
}

func TestSchemaIsValidJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "features", "feature.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if value["type"] != "object" {
		t.Fatalf("schema type = %v", value["type"])
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
