package clioutput

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteStableEnvelope(t *testing.T) {
	result := New(false, "feature-lock validate", "feature_lock_invalid", "The host lock is invalid.", "source mismatch", "Regenerate the lock.", "feature-lock validate --json")
	var output bytes.Buffer
	if err := Write(&output, result); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema", "ok", "command", "code", "what", "why", "remediation", "next_command"} {
		if _, exists := decoded[field]; !exists {
			t.Errorf("missing stable field %q", field)
		}
	}
	if decoded["schema"] != float64(SchemaVersion) || decoded["code"] != "feature_lock_invalid" {
		t.Fatalf("unexpected envelope: %#v", decoded)
	}
}
