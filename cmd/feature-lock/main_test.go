package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/timmyagentic/awesome-agent-app-features/internal/clioutput"
)

func TestRunJSONFailureHasStableRemediationFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"validate", "--json", "--source", "/missing", "--source-commit", "not-a-sha"}, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%s", exitCode, stderr.String())
	}
	var result clioutput.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Code != "feature_lock_invalid" || result.What == "" || result.Why == "" || result.Remediation == "" || result.NextCommand == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, required := range []string{"--source \"/missing\"", "--source-commit \"not-a-sha\"", "--host \".\"", "--lock \"agent-app-features.lock.json\"", "--json"} {
		if !strings.Contains(result.NextCommand, required) {
			t.Errorf("next command %q is missing %q", result.NextCommand, required)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON mode wrote stderr: %q", stderr.String())
	}
}

func TestRunJSONInvalidArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"unknown", "--json"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	var result clioutput.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != "invalid_arguments" || result.Command != "feature-lock validate" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
