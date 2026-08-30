package contract

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFeatureContributionTemplatesKeepEvidenceBoundaries(t *testing.T) {
	root := contributionRepositoryRoot(t)
	files := map[string][]string{
		".github/ISSUE_TEMPLATE/feature-proposal.yml": {
			"Problem and reusable value",
			"Reusable boundary",
			"Host responsibilities",
			"Security and compatibility invariants",
			"Real adopter",
			"Verification evidence",
			"UNVERIFIED boundaries",
		},
		".github/PULL_REQUEST_TEMPLATE/feature.md": {
			"## Reusable boundary",
			"## Host responsibilities",
			"## Security and compatibility invariants",
			"## Real adopter",
			"## Verification evidence",
			"## UNVERIFIED",
		},
	}
	for relative, required := range files {
		content, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, text := range required {
			if !strings.Contains(string(content), text) {
				t.Errorf("%s does not require %q", relative, text)
			}
		}
	}
}

func contributionRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
