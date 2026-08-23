package contract

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var stableVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type adapter struct {
	Path string `json:"path"`
	Role string `json:"role"`
}

type foundation struct {
	Core     []string  `json:"core"`
	Adapters []adapter `json:"adapters"`
	Host     []string  `json:"host"`
	Excludes []string  `json:"excludes"`
}

type manifest struct {
	SchemaURL        string     `json:"$schema"`
	Schema           int        `json:"schema"`
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Maturity         string     `json:"maturity"`
	Contract         string     `json:"contract"`
	ReleaseStatus    string     `json:"release_status"`
	Since            *string    `json:"since"`
	Runtime          []string   `json:"runtime"`
	Package          string     `json:"package"`
	IntegrationModel string     `json:"integration_model"`
	Foundation       foundation `json:"foundation"`
	Prerequisites    []string   `json:"prerequisites"`
	Invariants       []string   `json:"invariants"`
	IntegrationSteps []string   `json:"integration_steps"`
	Verification     []string   `json:"verification"`
	Relay            string     `json:"relay,omitempty"`
	Platforms        []string   `json:"platforms,omitempty"`
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
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&item); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if item.SchemaURL != "../feature.schema.json" || item.Schema != 1 || item.ID == "" || item.Name == "" || item.Maturity != "stable" || item.Contract != "v1" {
			t.Errorf("incomplete identity in %s: %+v", path, item)
		}
		switch item.ReleaseStatus {
		case "unreleased":
			if item.Since != nil {
				t.Errorf("unreleased manifest %s has since=%q", path, *item.Since)
			}
		case "released":
			if item.Since == nil || !stableVersion.MatchString(*item.Since) {
				t.Errorf("released manifest %s has invalid since=%v", path, item.Since)
			}
		default:
			t.Errorf("manifest %s has invalid release status %q", path, item.ReleaseStatus)
		}
		if item.IntegrationModel != "agent-assisted-code-change" {
			t.Errorf("%s integration model = %q", path, item.IntegrationModel)
		}
		if _, exists := seen[item.ID]; exists {
			t.Errorf("duplicate feature id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		for label, values := range map[string][]string{
			"runtime":             item.Runtime,
			"foundation.core":     item.Foundation.Core,
			"foundation.host":     item.Foundation.Host,
			"foundation.excludes": item.Foundation.Excludes,
			"prerequisites":       item.Prerequisites,
			"invariants":          item.Invariants,
			"integration_steps":   item.IntegrationSteps,
			"verification":        item.Verification,
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
		if len(item.Foundation.Adapters) == 0 {
			t.Errorf("%s has no provided adapters", path)
		}
		for _, adapter := range item.Foundation.Adapters {
			if strings.TrimSpace(adapter.Path) == "" || strings.TrimSpace(adapter.Role) == "" {
				t.Errorf("%s contains an incomplete adapter: %+v", path, adapter)
				continue
			}
			adapterPath := filepath.Join(root, strings.TrimPrefix(adapter.Path, "./"))
			if info, err := os.Stat(adapterPath); err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
				t.Errorf("%s adapter path %s is unavailable", path, adapterPath)
			}
		}
	}
}

func TestCorePackagesRemainHeadlessAndHostAgnostic(t *testing.T) {
	root := repositoryRoot(t)
	for _, packageName := range []string{"feedback", "updater"} {
		directory := filepath.Join(root, packageName)
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("read %s: %v", directory, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imported := range parsed.Imports {
				value, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					t.Fatalf("unquote import in %s: %v", path, err)
				}
				first := strings.Split(value, "/")[0]
				if strings.Contains(first, ".") {
					t.Errorf("core package %s imports non-standard dependency %q", packageName, value)
				}
			}

			lower := strings.ToLower(string(data))
			for _, forbidden := range []string{"feishu", "lark", "slack", "telegram", "discord", "cc-connect", "github", "gitlab", "cloudflare"} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("core source %s contains host/channel term %q", path, forbidden)
				}
			}
			if packageName == "feedback" {
				for _, forbidden := range []string{"[feedback]", "most recent error", "capabilities not available", "issueurl"} {
					if strings.Contains(lower, forbidden) {
						t.Errorf("feedback core source %s contains presentation/provider term %q", path, forbidden)
					}
				}
			}
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
