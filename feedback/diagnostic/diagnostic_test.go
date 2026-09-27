package diagnostic

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

func TestLongTurnDiagnosticRetainsCausalContextAndDeepCopy(t *testing.T) {
	now := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	exit := 1
	input := Input{Environment: feedback.Environment{Product: "Example"}, Diagnostic: &Diagnostic{
		StartedAt: now.Add(-26 * time.Hour), OccurredAt: now.Add(-time.Hour), Phase: "turn", ErrorCode: "eof",
		Request: "Export the entire project", Error: "unexpected EOF", Transport: Transport{ExitCode: &exit},
		Activity: []Activity{{Kind: "tool_start", Name: "export", AfterMS: 5}},
	}}
	draft, err := (Builder{Now: func() time.Time { return now }}).Build(input)
	if err != nil {
		t.Fatal(err)
	}
	exit = 99
	input.Diagnostic.Request = "changed input"
	preview := draft.Report()
	if preview.Diagnostic.Request != "Export the entire project" || *preview.Diagnostic.Transport.ExitCode != 1 {
		t.Fatalf("snapshot mutated: %+v", preview)
	}
	preview.Diagnostic.Activity[0].Name = "changed preview"
	*preview.Diagnostic.Transport.ExitCode = 98
	if draft.Report().Diagnostic.Activity[0].Name != "export" || *draft.Report().Diagnostic.Transport.ExitCode != 1 {
		t.Fatal("preview mutated draft")
	}
	if _, err := json.Marshal(preview); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatalf("unapproved report marshaled: %v", err)
	}
	if _, err := draft.Approve(false); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatal(err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"schema":2`, `"report_id":`, "Export the entire project", `"error_code":"eof"`, now.Add(-26 * time.Hour).Format(time.RFC3339)} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("approved payload missing %q: %s", want, data)
		}
	}
}

func TestDiagnosticBoundsRedactsBeforeTruncationAndReportsMissingFacts(t *testing.T) {
	now := time.Now()
	d := (Builder{}).Capture(Diagnostic{StartedAt: now.Add(-time.Second), OccurredAt: now, Phase: "turn",
		Request:  strings.Repeat("中", 4000) + ` api_key="never publish this credential"`,
		Error:    `Authorization: Bearer never-send-this`,
		Runtime:  Runtime{Model: `/Users/alice/private-model`, Backend: `token=secret-backend`},
		Activity: make([]Activity, 100),
	})
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(d.Request) || len(d.Request) > MaxRequestBytes || len(d.Activity) != MaxActivity {
		t.Fatal("invalid field limits")
	}
	for _, secret := range []string{"never publish", "never-send-this", "/Users/alice", "secret-backend"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("diagnostic leaked %q", secret)
		}
	}
	if !strings.Contains(strings.Join(d.Truncated, ","), "request") || !strings.Contains(strings.Join(d.Truncated, ","), "activity") {
		t.Fatal("truncation was hidden")
	}
	if !strings.Contains(strings.Join(d.Missing, ","), "agent_version") || !strings.Contains(strings.Join(d.Missing, ","), "protocol") {
		t.Fatal("unknown adapter facts not identified")
	}
}

func TestDiagnosticReportIdentitySurvivesExplicitRestore(t *testing.T) {
	input := Input{Description: "please improve diagnostics", Environment: feedback.Environment{Product: "Example"}}
	draft, err := (Builder{}).Build(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ReportID = draft.Report().ReportID
	restored, err := (Builder{}).Build(input)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := draft.Approve(true)
	b, _ := restored.Approve(true)
	before, _ := json.Marshal(a)
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatalf("restored report changed: %s != %s", before, after)
	}
	input.ReportID = "client-selected-path/../identity"
	if _, err := (Builder{}).Build(input); err == nil {
		t.Fatal("accepted invalid identity")
	}
	if _, err := json.Marshal(Approved{}); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatal("zero approval was accepted")
	}
}
