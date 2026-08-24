package feedback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestApprovedWireMatchesSharedFeedbackV1Fixture(t *testing.T) {
	at := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	draft, err := (Builder{Now: func() time.Time { return at.Add(time.Minute) }}).Build(Input{
		Description: "Improve startup diagnostics",
		RecentError: &RecentError{
			Text: "startup returned a redacted failure",
			At:   at,
		},
		CapabilityGaps: []string{"doctor.explain"},
		Environment: Environment{
			Product: "Example Agent",
			Version: "v1.0.0",
			OS:      "darwin",
			Arch:    "arm64",
			Agent:   "codex",
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	actualJSON, err := json.Marshal(approved)
	if err != nil {
		t.Fatalf("Marshal Approved: %v", err)
	}
	wantJSON, err := os.ReadFile(filepath.Join(repositoryRoot(t), "protocol", "feedback", "v1", "testdata", "valid-full.json"))
	if err != nil {
		t.Fatalf("read shared fixture: %v", err)
	}
	var actual any
	var want any
	if err := json.Unmarshal(actualJSON, &actual); err != nil {
		t.Fatalf("decode actual: %v", err)
	}
	if err := json.Unmarshal(wantJSON, &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("wire payload does not match shared v1 fixture\nactual: %s\nwant:   %s", actualJSON, wantJSON)
	}
}

func TestFeedbackV1SchemaIsValidJSON(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "protocol", "feedback", "v1", "schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("schema type = %v", schema["type"])
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve feedback test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
