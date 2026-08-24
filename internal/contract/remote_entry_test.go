package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type remoteEntry struct {
	SchemaURL        string  `json:"$schema"`
	Schema           int     `json:"schema"`
	ID               string  `json:"id"`
	Repository       string  `json:"repository"`
	Module           string  `json:"module"`
	Contract         string  `json:"contract"`
	ReleaseStatus    string  `json:"release_status"`
	Since            *string `json:"since"`
	IntegrationModel string  `json:"integration_model"`
	Entrypoint       struct {
		Path         string `json:"path"`
		DiscoveryURL string `json:"discovery_url"`
	} `json:"entrypoint"`
	Resolution struct {
		Provider     string `json:"provider"`
		Repository   string `json:"repository"`
		BootstrapRef string `json:"bootstrap_ref"`
		ImmutableRef string `json:"immutable_ref"`
		RequiredCI   string `json:"required_ci"`
	} `json:"resolution"`
	Delivery struct {
		UserCloneRequired bool     `json:"user_clone_required"`
		AllowedModes      []string `json:"allowed_modes"`
		Forbidden         []string `json:"forbidden"`
	} `json:"delivery"`
	Lock struct {
		SchemaPath string `json:"schema_path"`
		HostPath   string `json:"host_path"`
	} `json:"lock"`
	AgentFlow []string `json:"agent_flow"`
	Features  []struct {
		ID       string `json:"id"`
		Manifest string `json:"manifest"`
		Readme   string `json:"readme"`
		Contract string `json:"contract"`
	} `json:"features"`
}

func TestRemoteEntryResolvesEveryFeatureWithoutAClone(t *testing.T) {
	root := repositoryRoot(t)
	var entry remoteEntry
	readJSON(t, filepath.Join(root, "features", "index.json"), &entry)

	if entry.SchemaURL != "./index.schema.json" || entry.Schema != 1 ||
		entry.ID != "awesome-agent-app-features" ||
		entry.Repository != "https://github.com/timmyagentic/awesome-agent-app-features" ||
		entry.Module != "github.com/timmyagentic/awesome-agent-app-features" ||
		entry.Contract != "v1" || entry.ReleaseStatus != "unreleased" || entry.Since != nil ||
		entry.IntegrationModel != "remote-agent-assisted-code-change" {
		t.Fatalf("unexpected entry identity: %+v", entry)
	}
	if entry.Entrypoint.Path != "features/index.json" ||
		entry.Entrypoint.DiscoveryURL != "https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json" {
		t.Fatalf("unexpected entrypoint: %+v", entry.Entrypoint)
	}
	if entry.Resolution.Provider != "github" ||
		entry.Resolution.Repository != "timmyagentic/awesome-agent-app-features" ||
		entry.Resolution.BootstrapRef != "main" ||
		entry.Resolution.ImmutableRef != "full-commit-sha" ||
		entry.Resolution.RequiredCI != "CI" {
		t.Fatalf("unsafe resolution contract: %+v", entry.Resolution)
	}
	if entry.Delivery.UserCloneRequired {
		t.Fatal("remote entry unexpectedly requires a user clone")
	}
	assertStringSet(t, "allowed delivery modes", entry.Delivery.AllowedModes, []string{"go-module", "source-subtree"})
	assertStringSet(t, "forbidden integration paths", entry.Delivery.Forbidden, []string{"user-clone", "git-submodule", "local-replace", "floating-main"})
	if entry.Lock.SchemaPath != "features/integration-lock.schema.json" || entry.Lock.HostPath != "agent-app-features.lock.json" {
		t.Fatalf("unexpected lock contract: %+v", entry.Lock)
	}
	if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(entry.Lock.SchemaPath))); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("lock schema is unavailable: %v", err)
	}
	if len(entry.AgentFlow) < 5 {
		t.Fatalf("agent flow has %d steps", len(entry.AgentFlow))
	}

	manifestPaths, err := filepath.Glob(filepath.Join(root, "features", "*", "feature.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Features) != len(manifestPaths) {
		t.Fatalf("entry has %d features, repository has %d manifests", len(entry.Features), len(manifestPaths))
	}
	seen := make(map[string]struct{}, len(entry.Features))
	for _, feature := range entry.Features {
		if feature.Contract != "v1" || feature.Manifest != "features/"+feature.ID+"/feature.json" || feature.Readme != "features/"+feature.ID+"/README.md" {
			t.Errorf("invalid feature entry: %+v", feature)
		}
		if _, exists := seen[feature.ID]; exists {
			t.Errorf("duplicate feature entry %q", feature.ID)
		}
		seen[feature.ID] = struct{}{}
		for _, relative := range []string{feature.Manifest, feature.Readme} {
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil || !info.Mode().IsRegular() {
				t.Errorf("entry resource %q is unavailable", relative)
			}
		}
		var item manifest
		readJSON(t, filepath.Join(root, filepath.FromSlash(feature.Manifest)), &item)
		if item.ID != feature.ID || item.Contract != feature.Contract || item.ReleaseStatus != entry.ReleaseStatus {
			t.Errorf("entry/manifest mismatch for %q", feature.ID)
		}
	}
}

func TestUserDocumentationStartsInTheTargetProject(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{"README.md", "README.en.md", "docs/agent-integration.md", "docs/agent-integration.en.md"} {
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, obsolete := range []string{"FULL_REVIEWED_COMMIT_SHA", "把仓库和目标项目交给", "Give this repository and the target project", "git clone "} {
			if strings.Contains(text, obsolete) {
				t.Errorf("%s contains repository-centric onboarding %q", relative, obsolete)
			}
		}
		for _, required := range []string{"features/index.json", "commit SHA", "agent-app-features.lock.json"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s does not explain %q", relative, required)
			}
		}
	}
}

func TestRemoteConsumerCIRunsWithoutARepositoryCheckout(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	start := strings.Index(text, "  remote-consumer:")
	end := strings.Index(text, "\n  relay:")
	if start < 0 || end <= start {
		t.Fatal("remote-consumer CI job is unavailable")
	}
	job := text[start:end]
	if strings.Contains(job, "actions/checkout") {
		t.Fatal("remote consumer job checks out the foundation")
	}
	for _, required := range []string{
		"features/index.json",
		".lock.schema_path",
		"agent-app-features.lock.json",
		"full-commit-sha",
		"GOPROXY=direct go get",
		"/compat/v1",
		"/examples/feedback@",
		"/examples/updater-demo@",
		"/relay/cloudflare/package.json",
		"tar -tvzf",
	} {
		if !strings.Contains(job, required) {
			t.Errorf("remote consumer job does not prove %q", required)
		}
	}
}

func readJSON(t *testing.T, path string, destination any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func assertStringSet(t *testing.T, label string, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("%s = %v, want %v", label, actual, expected)
	}
	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		seen[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := seen[value]; !ok {
			t.Errorf("%s is missing %q", label, value)
		}
	}
}
