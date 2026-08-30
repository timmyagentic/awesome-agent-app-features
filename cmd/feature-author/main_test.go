package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/timmyagentic/awesome-agent-app-features/internal/clioutput"
)

func TestValidateJSONSuccess(t *testing.T) {
	root := repositoryRoot(t)
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"validate", "--root", root, "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr=%s", exitCode, stderr.String())
	}
	var result clioutput.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Code != "feature_catalog_valid" || result.NextCommand != "make verify" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestValidateJSONFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"validate", "--root", filepath.Join(t.TempDir(), "missing"), "--json"}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	var result clioutput.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Code != "feature_catalog_invalid" || result.What == "" || result.Why == "" || result.Remediation == "" || result.NextCommand == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON mode wrote stderr: %q", stderr.String())
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
