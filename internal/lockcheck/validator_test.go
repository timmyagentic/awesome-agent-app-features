package lockcheck

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testCommit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testVersion = "v0.0.0-20260830000000-aaaaaaaaaaaa"

func TestValidateAcceptsExactDeclaredHostIntegration(t *testing.T) {
	options := validOptions(t)
	report, err := Validate(context.Background(), options)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if report.Features != 1 || report.Files != 4 || report.GoModules != 1 || report.SourceSubtrees != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestValidateAcceptsNestedGoModuleTarget(t *testing.T) {
	options := validOptions(t)
	nestedRoot := filepath.Join(options.HostRoot, "backend")
	writeFile(t, filepath.Join(nestedRoot, "go.mod"), "module example.com/host/backend\n\ngo 1.25.0\n")
	lock := readLockMap(t, options.LockPath)
	feature := lock["features"].([]any)[0].(map[string]any)
	delivery := feature["deliveries"].([]any)[0].(map[string]any)
	delivery["target"] = "backend/go.mod"
	feature["files"] = append(feature["files"].([]any), "backend/go.mod")
	writeJSON(t, options.LockPath, lock)
	options.ResolveModule = func(_ context.Context, moduleRoot, modulePath string) (ModuleInfo, error) {
		if moduleRoot != nestedRoot {
			t.Fatalf("module root = %q, want %q", moduleRoot, nestedRoot)
		}
		if modulePath != "github.com/timmyagentic/awesome-agent-app-features" {
			t.Fatalf("module path = %q", modulePath)
		}
		return ModuleInfo{Version: testVersion, Directory: options.SourceRoot}, nil
	}
	if _, err := Validate(context.Background(), options); err != nil {
		t.Fatalf("Validate nested module target: %v", err)
	}
}

func TestValidateAcceptsDeclaredHostOwnedSubtreeFiles(t *testing.T) {
	options := validOptions(t)
	target := filepath.Join(options.HostRoot, "infrastructure", "feedback-relay")
	writeFile(t, filepath.Join(target, "wrangler.jsonc"), "{\n  \"name\": \"host-relay\"\n}\n")
	writeFile(t, filepath.Join(target, "worker-configuration.d.ts"), "// generated for the host\n")
	writeFile(t, filepath.Join(target, ".dev.vars"), "HOST_ONLY_VALUE=placeholder\n")

	if _, err := Validate(context.Background(), options); err != nil {
		t.Fatalf("Validate host-owned subtree files: %v", err)
	}
}

func TestValidateResolvesRelativeLockInsideHostRoot(t *testing.T) {
	options := validOptions(t)
	options.LockPath = "agent-app-features.lock.json"
	if _, err := Validate(context.Background(), options); err != nil {
		t.Fatalf("Validate relative lock: %v", err)
	}
}

func TestValidateAcceptsRemainingFeatureAfterRemoval(t *testing.T) {
	options := validOptions(t)
	lock := readLockMap(t, options.LockPath)
	lock["features"] = []any{
		map[string]any{
			"id":       "updater",
			"contract": "v1",
			"deliveries": []any{
				map[string]any{
					"mode":    "go-module",
					"source":  "github.com/timmyagentic/awesome-agent-app-features/updater",
					"target":  "go.mod",
					"version": testVersion,
				},
			},
			"files":       []any{"go.mod", "go.sum"},
			"verified_at": "2026-08-30T00:00:00Z",
			"checks":      []any{"go test ./..."},
			"unverified":  []any{},
		},
	}
	writeJSON(t, options.LockPath, lock)
	report, err := Validate(context.Background(), options)
	if err != nil {
		t.Fatalf("Validate after removal: %v", err)
	}
	if report.Features != 1 || report.GoModules != 1 || report.SourceSubtrees != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestValidateRejectsSemanticDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, options *Options, lock map[string]any)
		want   string
	}{
		{
			name: "source commit mismatch",
			mutate: func(_ *testing.T, options *Options, _ map[string]any) {
				options.SourceCommit = strings.Repeat("b", 40)
			},
			want: "source commit",
		},
		{
			name: "duplicate feature",
			mutate: func(_ *testing.T, _ *Options, lock map[string]any) {
				features := lock["features"].([]any)
				lock["features"] = append(features, features[0])
			},
			want: "duplicate feature",
		},
		{
			name: "unknown feature",
			mutate: func(_ *testing.T, _ *Options, lock map[string]any) {
				feature := lock["features"].([]any)[0].(map[string]any)
				feature["id"] = "ghost-feature"
			},
			want: "not declared",
		},
		{
			name: "unreleased feature",
			mutate: func(t *testing.T, options *Options, _ map[string]any) {
				path := filepath.Join(options.SourceRoot, "features", "feedback", "feature.json")
				manifest := readLockMap(t, path)
				manifest["release_status"] = "unreleased"
				manifest["since"] = nil
				writeJSON(t, path, manifest)
			},
			want: "unreleased",
		},
		{
			name: "undeclared package",
			mutate: func(_ *testing.T, _ *Options, lock map[string]any) {
				feature := lock["features"].([]any)[0].(map[string]any)
				delivery := feature["deliveries"].([]any)[0].(map[string]any)
				delivery["source"] = "github.com/timmyagentic/awesome-agent-app-features/not-real"
			},
			want: "not declared",
		},
		{
			name: "local module replace",
			mutate: func(_ *testing.T, options *Options, _ map[string]any) {
				options.ResolveModule = func(context.Context, string, string) (ModuleInfo, error) {
					return ModuleInfo{Version: testVersion, Directory: options.SourceRoot, Replaced: true}, nil
				}
			},
			want: "local replace",
		},
		{
			name: "module version mismatch",
			mutate: func(_ *testing.T, options *Options, _ map[string]any) {
				options.ResolveModule = func(context.Context, string, string) (ModuleInfo, error) {
					return ModuleInfo{Version: "v0.0.0-20260830000000-bbbbbbbbbbbb", Directory: options.SourceRoot}, nil
				}
			},
			want: "module version",
		},
		{
			name: "module content mismatch",
			mutate: func(t *testing.T, options *Options, _ map[string]any) {
				moduleRoot := t.TempDir()
				writeFile(t, filepath.Join(moduleRoot, "go.mod"), "module github.com/timmyagentic/awesome-agent-app-features\n\ngo 1.25.0\n")
				writeFile(t, filepath.Join(moduleRoot, "feedback", "model.go"), "package feedback\n")
				options.ResolveModule = func(context.Context, string, string) (ModuleInfo, error) {
					return ModuleInfo{Version: testVersion, Directory: moduleRoot}, nil
				}
			},
			want: "module content",
		},
		{
			name: "missing host file",
			mutate: func(_ *testing.T, _ *Options, lock map[string]any) {
				feature := lock["features"].([]any)[0].(map[string]any)
				feature["files"] = append(feature["files"].([]any), "missing.go")
			},
			want: "host file",
		},
		{
			name: "missing source subtree target",
			mutate: func(_ *testing.T, options *Options, _ map[string]any) {
				if err := os.RemoveAll(filepath.Join(options.HostRoot, "infrastructure", "feedback-relay")); err != nil {
					t.Fatal(err)
				}
			},
			want: "source-subtree target",
		},
		{
			name: "source subtree content mismatch",
			mutate: func(t *testing.T, options *Options, _ map[string]any) {
				writeFile(t, filepath.Join(options.HostRoot, "infrastructure", "feedback-relay", "src", "relay.js"), "export const mixedSource = true;\n")
			},
			want: "source-subtree content mismatch",
		},
		{
			name: "source subtree symlinked parent",
			mutate: func(t *testing.T, options *Options, _ map[string]any) {
				target := filepath.Join(options.HostRoot, "infrastructure", "feedback-relay", "src")
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(options.SourceRoot, "relay", "cloudflare", "src"), target); err != nil {
					t.Fatal(err)
				}
			},
			want: "source-subtree content mismatch",
		},
		{
			name: "future verification time",
			mutate: func(_ *testing.T, _ *Options, lock map[string]any) {
				feature := lock["features"].([]any)[0].(map[string]any)
				feature["verified_at"] = "2026-08-31T00:00:00Z"
			},
			want: "future",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := validOptions(t)
			lock := readLockMap(t, options.LockPath)
			test.mutate(t, &options, lock)
			writeJSON(t, options.LockPath, lock)
			_, err := Validate(context.Background(), options)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsOversizedLockBeforeDecoding(t *testing.T) {
	options := validOptions(t)
	writeFile(t, options.LockPath, strings.Repeat(" ", maxContractJSONBytes+1))
	_, err := Validate(context.Background(), options)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want bounded lock rejection", err)
	}
}

func validOptions(t *testing.T) Options {
	t.Helper()
	repository := repositoryRoot(t)
	sourceRoot := t.TempDir()
	for _, relative := range []string{"go.mod", "features/index.json", "features/feedback/feature.json", "features/updater/feature.json"} {
		copyFile(t, filepath.Join(repository, relative), filepath.Join(sourceRoot, relative))
	}
	copyTree(t, filepath.Join(repository, "feedback"), filepath.Join(sourceRoot, "feedback"))
	copyTree(t, filepath.Join(repository, "updater"), filepath.Join(sourceRoot, "updater"))
	copyTree(t, filepath.Join(repository, "relay", "cloudflare"), filepath.Join(sourceRoot, "relay", "cloudflare"))
	hostRoot := t.TempDir()
	writeFile(t, filepath.Join(hostRoot, "go.mod"), "module example.com/host\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(hostRoot, "go.sum"), "module checksum\n")
	writeFile(t, filepath.Join(hostRoot, "internal", "feedback", "flow.go"), "package feedback\n")
	copyTree(
		t,
		filepath.Join(sourceRoot, "relay", "cloudflare"),
		filepath.Join(hostRoot, "infrastructure", "feedback-relay"),
	)

	lockPath := filepath.Join(hostRoot, "agent-app-features.lock.json")
	writeJSON(t, lockPath, map[string]any{
		"schema": 1,
		"source": map[string]any{
			"repository": "timmyagentic/awesome-agent-app-features",
			"commit":     testCommit,
		},
		"features": []any{
			map[string]any{
				"id":       "feedback",
				"contract": "v1",
				"deliveries": []any{
					map[string]any{
						"mode":    "go-module",
						"source":  "github.com/timmyagentic/awesome-agent-app-features/feedback",
						"target":  "go.mod",
						"version": testVersion,
					},
					map[string]any{
						"mode":   "source-subtree",
						"source": "relay/cloudflare",
						"target": "infrastructure/feedback-relay",
					},
				},
				"files": []any{
					"go.mod",
					"go.sum",
					"internal/feedback/flow.go",
					"infrastructure/feedback-relay/package.json",
				},
				"verified_at": "2026-08-30T00:00:00Z",
				"checks":      []any{"go test ./..."},
				"unverified":  []any{},
			},
		},
	})

	return Options{
		LockPath:     lockPath,
		HostRoot:     hostRoot,
		SourceRoot:   sourceRoot,
		SourceCommit: testCommit,
		Now:          func() time.Time { return time.Date(2026, 8, 30, 1, 0, 0, 0, time.UTC) },
		ResolveModule: func(context.Context, string, string) (ModuleInfo, error) {
			return ModuleInfo{Version: testVersion, Directory: sourceRoot}, nil
		},
	}
}

func copyFile(t *testing.T, source, target string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, string(data))
}

func copyTree(t *testing.T, source, target string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == source {
			return os.MkdirAll(target, 0o755)
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".wrangler") {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			t.Fatalf("unsupported source fixture entry: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data)+"\n")
}

func readLockMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
