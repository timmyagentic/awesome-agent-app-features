package featureauthor

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestScaffoldAndValidateThirdSourceSubtreeFeature(t *testing.T) {
	root := copyRepository(t)
	err := Scaffold(ScaffoldOptions{
		Root:    root,
		ID:      "diagnostics-export",
		Name:    "Diagnostics export",
		Kind:    KindSourceSubtree,
		Runtime: "javascript",
	})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if err := Validate(root); err != nil {
		t.Fatalf("Validate after adding third Feature: %v", err)
	}

	var entry struct {
		Features []struct {
			ID string `json:"id"`
		} `json:"features"`
	}
	readJSONFile(t, filepath.Join(root, "features", "index.json"), &entry)
	if len(entry.Features) != 3 {
		t.Fatalf("feature count = %d, want 3", len(entry.Features))
	}

	var manifest map[string]any
	readJSONFile(t, filepath.Join(root, "features", "diagnostics-export", "feature.json"), &manifest)
	if _, exists := manifest["package"]; exists {
		t.Fatal("source-subtree-only Feature unexpectedly requires a Go package")
	}
	examples, ok := manifest["remote_examples"].([]any)
	if !ok || len(examples) != 0 {
		t.Fatalf("remote_examples = %#v, want an empty array", manifest["remote_examples"])
	}
	for _, readme := range []string{"README.md", "README.en.md"} {
		content, err := os.ReadFile(filepath.Join(root, readme))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "Diagnostics export") || !strings.Contains(string(content), "features/diagnostics-export/source") {
			t.Errorf("%s was not synchronized from the third Feature manifest", readme)
		}
	}
}

func TestValidateRejectsGeneratedReadmeCatalogDrift(t *testing.T) {
	root := copyRepository(t)
	path := filepath.Join(root, "README.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "User-approved in-product feedback", "Drifted feedback title", 1))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(root); err == nil || !strings.Contains(err.Error(), "sync-docs") {
		t.Fatalf("Validate() error = %v, want generated-doc drift remediation", err)
	}
}

func TestScaffoldAndValidateThirdGoFeature(t *testing.T) {
	root := copyRepository(t)
	if err := Scaffold(ScaffoldOptions{
		Root: root, ID: "diagnostics", Name: "Diagnostics", Kind: KindGo,
	}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if err := Validate(root); err != nil {
		t.Fatalf("Validate after adding third Go Feature: %v", err)
	}
	for _, relative := range []string{
		"diagnostics/doc.go",
		"examples/diagnostics/main.go",
		"features/diagnostics/feature.json",
		"features/diagnostics/README.md",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Errorf("generated file %s is unavailable: %v", relative, err)
		}
	}
}

func TestValidateReleaseAllowsHistoricalAndUnreleasedFeatures(t *testing.T) {
	root := copyRepository(t)
	if err := Scaffold(ScaffoldOptions{
		Root:    root,
		ID:      "diagnostics-export",
		Name:    "Diagnostics export",
		Kind:    KindSourceSubtree,
		Runtime: "javascript",
	}); err != nil {
		t.Fatal(err)
	}

	if err := ValidateRelease(root, "v0.2.0", func(tag, relative string) (TagResolution, error) {
		if tag != "v0.1.0" && tag != "v0.2.0" {
			return TagResolution{}, nil
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		return TagResolution{Exists: true, AncestorOfRelease: true, File: data}, err
	}); err != nil {
		t.Fatalf("historical and unreleased Feature metadata should coexist: %v", err)
	}

	manifestPath := filepath.Join(root, "features", "diagnostics-export", "feature.json")
	var manifest map[string]any
	readJSONFile(t, manifestPath, &manifest)
	manifest["release_status"] = "released"
	manifest["since"] = "v0.2.0"
	writeJSONFile(t, manifestPath, manifest)
	if err := SyncReadmes(root); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRelease(root, "v0.2.0", func(tag, relative string) (TagResolution, error) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		return TagResolution{Exists: tag == "v0.1.0" || tag == "v0.2.0", AncestorOfRelease: true, File: data}, err
	}); err != nil {
		t.Fatalf("new Feature introduction tag should validate: %v", err)
	}
}

func TestValidateReleaseRejectsFeatureMissingAtClaimedSince(t *testing.T) {
	root := copyRepository(t)
	if err := Scaffold(ScaffoldOptions{Root: root, ID: "diagnostics-export", Name: "Diagnostics export", Kind: KindSourceSubtree}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "features", "diagnostics-export", "feature.json")
	var manifest map[string]any
	readJSONFile(t, manifestPath, &manifest)
	manifest["release_status"] = "released"
	manifest["since"] = "v0.1.0"
	writeJSONFile(t, manifestPath, manifest)

	err := ValidateRelease(root, "v0.2.0", func(tag, relative string) (TagResolution, error) {
		if tag == "v0.1.0" && relative == "features/diagnostics-export/feature.json" {
			return TagResolution{Exists: true, AncestorOfRelease: true}, nil
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		return TagResolution{Exists: true, AncestorOfRelease: true, File: data}, readErr
	})
	if err == nil {
		t.Fatal("release metadata accepted a Feature that did not exist at its claimed since tag")
	}
}

func TestValidateRejectsAdapterTraversal(t *testing.T) {
	root := copyRepository(t)
	path := filepath.Join(root, "features", "feedback", "feature.json")
	var manifest map[string]any
	readJSONFile(t, path, &manifest)
	foundation := manifest["foundation"].(map[string]any)
	adapters := foundation["adapters"].([]any)
	adapters[0].(map[string]any)["path"] = "./.."
	writeJSONFile(t, path, manifest)
	if err := Validate(root); err == nil {
		t.Fatal("adapter traversal path was accepted")
	}
}

func copyRepository(t *testing.T) string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	source := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	target := t.TempDir()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == ".git" || relative == "relay/cloudflare/node_modules" || relative == "relay/cloudflare/.wrangler" || relative == "dist" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func readJSONFile(t *testing.T, path string, destination any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatal(err)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
