package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
		Path                  string `json:"path"`
		DiscoveryURL          string `json:"discovery_url"`
		IntegrationPlanSchema string `json:"integration_plan_schema"`
	} `json:"entrypoint"`
	Resolution struct {
		Provider     string `json:"provider"`
		Repository   string `json:"repository"`
		BootstrapRef string `json:"bootstrap_ref"`
		ImmutableRef string `json:"immutable_ref"`
		CI           struct {
			Workflow           string `json:"workflow"`
			RequiredStatus     string `json:"required_status"`
			RequiredConclusion string `json:"required_conclusion"`
		} `json:"ci"`
		Fetch struct {
			SameCommitRequired  bool   `json:"same_commit_required"`
			RawURLTemplate      string `json:"raw_url_template"`
			ContentsAPITemplate string `json:"contents_api_template"`
			ArchiveURLTemplate  string `json:"archive_url_template"`
		} `json:"fetch"`
	} `json:"resolution"`
	Delivery struct {
		UserCloneRequired bool     `json:"user_clone_required"`
		AllowedModes      []string `json:"allowed_modes"`
		Forbidden         []string `json:"forbidden"`
	} `json:"delivery"`
	AgentFlow []string `json:"agent_flow"`
	Features  []struct {
		ID       string `json:"id"`
		Manifest string `json:"manifest"`
		Readme   string `json:"readme"`
		Contract string `json:"contract"`
	} `json:"features"`
}

type integrationPlan struct {
	SchemaURL string `json:"$schema"`
	Schema    int    `json:"schema"`
	Source    struct {
		Repository     string `json:"repository"`
		ResolvedCommit string `json:"resolved_commit"`
		CIRunURL       string `json:"ci_run_url"`
		CIConclusion   string `json:"ci_conclusion"`
		EntryPath      string `json:"entry_path"`
		ManifestPath   string `json:"manifest_path"`
	} `json:"source"`
	Feature struct {
		ID         string `json:"id"`
		Contract   string `json:"contract"`
		Deliveries []struct {
			Mode   string `json:"mode"`
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"deliveries"`
	} `json:"feature"`
	Host struct {
		Root             string   `json:"root"`
		Runtime          []string `json:"runtime"`
		ExistingSurfaces []string `json:"existing_surfaces"`
		InstallKinds     []string `json:"install_kinds"`
		Decisions        []string `json:"decisions"`
	} `json:"host"`
	Mappings []struct {
		ContractItem   string   `json:"contract_item"`
		Owner          string   `json:"owner"`
		HostLocation   string   `json:"host_location"`
		Implementation string   `json:"implementation"`
		Tests          []string `json:"tests"`
	} `json:"mappings"`
	Invariants []struct {
		Invariant string `json:"invariant"`
		Status    string `json:"status"`
		Evidence  string `json:"evidence"`
	} `json:"invariants"`
	Changes []struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	} `json:"changes"`
	Verification struct {
		Foundation []string `json:"foundation"`
		Host       []string `json:"host"`
	} `json:"verification"`
	Unverified []string `json:"unverified"`
}

func TestRemoteEntryResolvesEveryFeatureWithoutAClone(t *testing.T) {
	root := repositoryRoot(t)
	var entry remoteEntry
	decodeStrictJSON(t, filepath.Join(root, "features", "index.json"), &entry)
	if entry.SchemaURL != "./index.schema.json" || entry.Schema != 1 ||
		entry.ID != "awesome-agent-app-features" ||
		entry.Repository != "https://github.com/timmyagentic/awesome-agent-app-features" ||
		entry.Module != "github.com/timmyagentic/awesome-agent-app-features" ||
		entry.Contract != "v1" || entry.ReleaseStatus != "unreleased" || entry.Since != nil ||
		entry.IntegrationModel != "remote-agent-assisted-code-change" {
		t.Fatalf("unexpected entry identity: %+v", entry)
	}
	if entry.Entrypoint.Path != "features/index.json" ||
		entry.Entrypoint.IntegrationPlanSchema != "features/integration-plan.schema.json" {
		t.Fatalf("unexpected entrypoint: %+v", entry.Entrypoint)
	}
	if entry.Resolution.Provider != "github" ||
		entry.Resolution.Repository != "timmyagentic/awesome-agent-app-features" ||
		entry.Resolution.BootstrapRef != "main" ||
		entry.Resolution.ImmutableRef != "full-commit-sha" ||
		entry.Resolution.CI.Workflow != "CI" ||
		entry.Resolution.CI.RequiredStatus != "completed" ||
		entry.Resolution.CI.RequiredConclusion != "success" ||
		!entry.Resolution.Fetch.SameCommitRequired {
		t.Fatalf("unsafe resolution contract: %+v", entry.Resolution)
	}
	for label, template := range map[string]string{
		"raw URL":      entry.Resolution.Fetch.RawURLTemplate,
		"contents API": entry.Resolution.Fetch.ContentsAPITemplate,
		"archive URL":  entry.Resolution.Fetch.ArchiveURLTemplate,
	} {
		if !strings.Contains(template, "{commit}") {
			t.Errorf("%s does not pin the resolved commit: %q", label, template)
		}
	}
	if entry.Delivery.UserCloneRequired {
		t.Fatal("remote entry unexpectedly requires a user clone")
	}
	assertStringSet(t, "allowed delivery modes", entry.Delivery.AllowedModes, []string{"go-module", "source-subtree"})
	assertStringSet(t, "forbidden integration paths", entry.Delivery.Forbidden, []string{"user-clone", "git-submodule", "local-replace", "floating-main"})
	if len(entry.AgentFlow) < 7 {
		t.Fatalf("agent flow has %d steps", len(entry.AgentFlow))
	}

	manifestPaths, err := filepath.Glob(filepath.Join(root, "features", "*", "feature.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Features) != len(manifestPaths) {
		t.Fatalf("entry has %d features, repository has %d manifests", len(entry.Features), len(manifestPaths))
	}
	seen := make(map[string]struct{})
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
		decodeStrictJSON(t, filepath.Join(root, filepath.FromSlash(feature.Manifest)), &item)
		if item.ID != feature.ID || item.Contract != feature.Contract || item.ReleaseStatus != entry.ReleaseStatus {
			t.Errorf("entry/manifest mismatch for %q", feature.ID)
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(entry.Entrypoint.IntegrationPlanSchema))); err != nil {
		t.Fatalf("integration plan schema: %v", err)
	}
}

func TestIntegrationPlanExampleIsStrictAndPinned(t *testing.T) {
	root := repositoryRoot(t)
	var plan integrationPlan
	decodeStrictJSON(t, filepath.Join(root, "features", "integration-plan.example.json"), &plan)
	if plan.SchemaURL != "./integration-plan.schema.json" || plan.Schema != 1 {
		t.Fatalf("plan schema identity: %+v", plan)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(plan.Source.ResolvedCommit) {
		t.Fatalf("resolved commit = %q", plan.Source.ResolvedCommit)
	}
	if !strings.HasPrefix(plan.Source.CIRunURL, "https://github.com/timmyagentic/awesome-agent-app-features/actions/runs/") ||
		plan.Source.CIConclusion != "success" {
		t.Fatalf("CI evidence = %q, %q", plan.Source.CIRunURL, plan.Source.CIConclusion)
	}
	if plan.Source.EntryPath != "features/index.json" ||
		plan.Source.ManifestPath != "features/"+plan.Feature.ID+"/feature.json" ||
		plan.Feature.Contract != "v1" || len(plan.Feature.Deliveries) == 0 ||
		len(plan.Mappings) == 0 || len(plan.Invariants) == 0 {
		t.Fatalf("incomplete integration plan: %+v", plan)
	}
	var item manifest
	decodeStrictJSON(t, filepath.Join(root, filepath.FromSlash(plan.Source.ManifestPath)), &item)
	allowedDeliveries := make(map[string]string)
	for _, delivery := range item.Delivery {
		for _, packagePath := range delivery.Packages {
			allowedDeliveries[packagePath] = delivery.Mode
		}
		if delivery.Path != "" {
			allowedDeliveries[delivery.Path] = delivery.Mode
		}
	}
	for _, delivery := range plan.Feature.Deliveries {
		if allowedDeliveries[delivery.Source] != delivery.Mode || strings.TrimSpace(delivery.Target) == "" {
			t.Errorf("example selects undeclared delivery: %+v", delivery)
		}
	}
	actualInvariants := make([]string, 0, len(plan.Invariants))
	for _, invariant := range plan.Invariants {
		actualInvariants = append(actualInvariants, invariant.Invariant)
	}
	assertStringSet(t, "example manifest invariants", actualInvariants, item.Invariants)
}

func TestUserDocumentationStartsInTheTargetProject(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{"README.md", "README.en.md", "docs/agent-integration.md", "docs/agent-integration.en.md"} {
		data, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, obsolete := range []string{
			"FULL_REVIEWED_COMMIT_SHA",
			"把仓库和目标项目交给",
			"Give this repository and the target project",
			"git clone ",
		} {
			if strings.Contains(text, obsolete) {
				t.Errorf("%s contains repository-centric onboarding %q", relative, obsolete)
			}
		}
		for _, required := range []string{"features/index.json", "commit SHA"} {
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
		"same_commit_required",
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

func decodeStrictJSON(t *testing.T, path string, destination any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("decode %s: trailing JSON: %v", path, err)
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
