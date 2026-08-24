package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type integrationReceipt struct {
	Schema      int    `json:"schema"`
	SchemaPath  string `json:"schema_path"`
	ReceiptPath string `json:"receipt_path"`
	State       string `json:"state"`
	Source      struct {
		Repository     string `json:"repository"`
		ResolvedCommit string `json:"resolved_commit"`
		CIRunURL       string `json:"ci_run_url"`
		CIConclusion   string `json:"ci_conclusion"`
		EntryPath      string `json:"entry_path"`
		ManifestPath   string `json:"manifest_path"`
	} `json:"source"`
	Feature struct {
		ID            string  `json:"id"`
		Contract      string  `json:"contract"`
		ReleaseStatus string  `json:"release_status"`
		Since         *string `json:"since"`
	} `json:"feature"`
	Host struct {
		Root              string   `json:"root"`
		Runtime           []string `json:"runtime"`
		EntryPoints       []string `json:"entry_points"`
		ConfigurationKeys []string `json:"configuration_keys"`
	} `json:"host"`
	Deliveries []struct {
		Mode            string `json:"mode"`
		Source          string `json:"source"`
		Target          string `json:"target"`
		ResolvedVersion string `json:"resolved_version,omitempty"`
	} `json:"deliveries"`
	Artifacts []struct {
		Path      string `json:"path"`
		Ownership string `json:"ownership"`
		Role      string `json:"role"`
		Removal   string `json:"removal"`
	} `json:"artifacts"`
	Invariants []struct {
		Invariant string `json:"invariant"`
		Status    string `json:"status"`
		Evidence  string `json:"evidence"`
	} `json:"invariants"`
	Verification []struct {
		Scope    string `json:"scope"`
		Check    string `json:"check"`
		Status   string `json:"status"`
		Evidence string `json:"evidence"`
		At       string `json:"at"`
	} `json:"verification"`
	Unverified []struct {
		Boundary string `json:"boundary"`
		Reason   string `json:"reason"`
	} `json:"unverified"`
	History []struct {
		Action     string  `json:"action"`
		FromCommit *string `json:"from_commit"`
		ToCommit   string  `json:"to_commit"`
		At         string  `json:"at"`
		Summary    string  `json:"summary"`
	} `json:"history"`
	Removal struct {
		Status        string   `json:"status"`
		RemovedPaths  []string `json:"removed_paths"`
		RetainedPaths []string `json:"retained_paths"`
		Evidence      string   `json:"evidence"`
	} `json:"removal"`
	UpdatedAt string `json:"updated_at"`
}

func TestIntegrationReceiptExampleMatchesPinnedFeatureContract(t *testing.T) {
	root := repositoryRoot(t)
	examplePath := filepath.Join(root, "examples", "host-receipt", ".agent-app-features", "feedback.json")
	var receipt integrationReceipt
	decodeStrictJSON(t, examplePath, &receipt)
	if receipt.Schema != 1 ||
		receipt.SchemaPath != "features/integration-receipt.schema.json" ||
		receipt.ReceiptPath != ".agent-app-features/"+receipt.Feature.ID+".json" ||
		receipt.State != "active" ||
		receipt.Host.Root != "." {
		t.Fatalf("invalid receipt identity: %+v", receipt)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(receipt.Source.ResolvedCommit) ||
		receipt.Source.Repository != "timmyagentic/awesome-agent-app-features" ||
		receipt.Source.CIConclusion != "success" ||
		!strings.HasPrefix(receipt.Source.CIRunURL, "https://github.com/timmyagentic/awesome-agent-app-features/actions/runs/") ||
		receipt.Source.EntryPath != "features/index.json" ||
		receipt.Source.ManifestPath != "features/"+receipt.Feature.ID+"/feature.json" {
		t.Fatalf("invalid receipt source: %+v", receipt.Source)
	}

	var item manifest
	decodeStrictJSON(t, filepath.Join(root, filepath.FromSlash(receipt.Source.ManifestPath)), &item)
	if receipt.Feature.ID != item.ID || receipt.Feature.Contract != item.Contract ||
		receipt.Feature.ReleaseStatus != item.ReleaseStatus ||
		(receipt.Feature.Since == nil) != (item.Since == nil) {
		t.Fatalf("receipt feature does not match manifest: %+v vs %+v", receipt.Feature, item)
	}

	allowedDeliveries := make(map[string]string)
	for _, delivery := range item.Delivery {
		for _, packagePath := range delivery.Packages {
			allowedDeliveries[packagePath] = delivery.Mode
		}
		if delivery.Path != "" {
			allowedDeliveries[delivery.Path] = delivery.Mode
		}
	}
	for _, delivery := range receipt.Deliveries {
		if allowedDeliveries[delivery.Source] != delivery.Mode {
			t.Errorf("receipt selects undeclared delivery: %+v", delivery)
		}
		if delivery.Mode == "go-module" {
			if delivery.Target != "go.mod" ||
				!strings.HasSuffix(delivery.ResolvedVersion, "-"+receipt.Source.ResolvedCommit[:12]) {
				t.Errorf("Go delivery is not pinned to the receipt commit: %+v", delivery)
			}
		} else if delivery.ResolvedVersion != "" || !safeRelativePath(delivery.Target) {
			t.Errorf("invalid source-subtree delivery: %+v", delivery)
		}
	}

	seenArtifacts := make(map[string]struct{})
	for _, artifact := range receipt.Artifacts {
		if !safeRelativePath(artifact.Path) {
			t.Errorf("unsafe receipt artifact path %q", artifact.Path)
		}
		if _, exists := seenArtifacts[artifact.Path]; exists {
			t.Errorf("duplicate receipt artifact %q", artifact.Path)
		}
		seenArtifacts[artifact.Path] = struct{}{}
		if artifact.Ownership == "host-shared" && artifact.Removal != "retain" {
			t.Errorf("host-shared artifact is removable: %+v", artifact)
		}
	}

	actualInvariants := make([]string, 0, len(receipt.Invariants))
	for _, invariant := range receipt.Invariants {
		actualInvariants = append(actualInvariants, invariant.Invariant)
		if invariant.Status == "blocked" || strings.TrimSpace(invariant.Evidence) == "" {
			t.Errorf("active receipt has unresolved invariant: %+v", invariant)
		}
	}
	assertStringSet(t, "receipt manifest invariants", actualInvariants, item.Invariants)

	passedScopes := make(map[string]bool)
	for _, verification := range receipt.Verification {
		if verification.Status == "failed" {
			t.Errorf("active receipt contains failed verification: %+v", verification)
		}
		if verification.Status == "passed" {
			passedScopes[verification.Scope] = true
		}
	}
	if !passedScopes["remote"] || !passedScopes["host"] {
		t.Errorf("active receipt lacks remote/host proof: %v", passedScopes)
	}
	if len(receipt.History) == 0 ||
		receipt.History[0].Action != "integrate" ||
		receipt.History[0].FromCommit != nil ||
		receipt.History[0].ToCommit != receipt.Source.ResolvedCommit {
		t.Fatalf("invalid initial receipt history: %+v", receipt.History)
	}
	if receipt.Removal.Status != "not-requested" || len(receipt.Removal.RemovedPaths) != 0 {
		t.Fatalf("active receipt has removal state: %+v", receipt.Removal)
	}
}

func TestReceiptExampleContainsNoSecretOrMachineSpecificFields(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "examples", "host-receipt", ".agent-app-features", "feedback.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	for _, forbidden := range []string{
		"\"configuration_values\"",
		"\"environment\"",
		"\"payload\"",
		"\"logs\"",
		"\"user_id\"",
		"\"chat_id\"",
		"\"access_token\"",
		"\"authorization\"",
		"\"absolute_path\"",
		"/users/",
		"/home/",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("receipt contains forbidden machine/secret field %q", forbidden)
		}
	}
}

func safeRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." || part == "" {
			return false
		}
	}
	return true
}
