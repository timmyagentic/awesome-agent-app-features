package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRedactRemovesSensitiveShapes(t *testing.T) {
	input := strings.Join([]string{
		"api_key=super-secret-value",
		"Authorization: Bearer secret-bearer-token",
		"user_id=ou_1234567890abcdef",
		"owner@example.com",
		"/Users/alice/private/config.toml",
		`C:\Users\Alice\secret.txt`,
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature1234567890",
		"-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----",
	}, "\n")

	redacted := Redact(input)
	for _, leaked := range []string{
		"super-secret-value",
		"secret-bearer-token",
		"ou_1234567890abcdef",
		"owner@example.com",
		"/Users/alice",
		`C:\Users\Alice`,
		"eyJhbGciOiJIUzI1NiJ9",
		"abc123",
	} {
		if strings.Contains(redacted, leaked) {
			t.Errorf("redacted output still contains %q: %s", leaked, redacted)
		}
	}
	if !strings.Contains(redacted, "[REDACTED") {
		t.Fatalf("expected redaction markers, got %s", redacted)
	}
}

func TestBuilderCreatesSingleReportShapeAndDropsStaleError(t *testing.T) {
	now := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	builder := Builder{Now: func() time.Time { return now }}
	draft, err := builder.Build(Input{
		Description: "Please add webhook retries; token=do-not-send",
		RecentError: &RecentError{
			Text: "request failed for /Users/alice/project",
			At:   now.Add(-5 * time.Minute),
		},
		CapabilityGaps: []string{"webhook.retry", "agent.fast_mode", "webhook.retry"},
		Environment: Environment{
			Product: "Example Agent",
			Version: "v1.2.3",
			Agent:   "codex",
		},
		InstallID: "install-123",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	submission := draft.Submission()
	if submission.Schema != 1 || submission.UserApproved {
		t.Fatalf("unexpected draft schema/approval: %+v", submission)
	}
	if !strings.HasPrefix(submission.Title, "[feedback] Please add webhook retries") {
		t.Fatalf("title = %q", submission.Title)
	}
	for _, want := range []string{
		"Please add webhook retries",
		"Most recent error",
		"[REDACTED-PATH]",
		"agent.fast\\_mode",
		"webhook.retry",
		"Product: Example Agent",
		"OS/Arch:",
	} {
		if !strings.Contains(submission.Body, want) {
			t.Errorf("body missing %q:\n%s", want, submission.Body)
		}
	}
	if strings.Contains(submission.Body, "do-not-send") || strings.Count(submission.Body, "webhook.retry") != 1 {
		t.Fatalf("body was not redacted/deduplicated:\n%s", submission.Body)
	}
	preview := draft.Preview()
	if !strings.Contains(preview, "Installation ID: install-123") || !strings.Contains(preview, submission.Body) {
		t.Fatalf("preview omits outbound fields:\n%s", preview)
	}

	stale, err := builder.Build(Input{
		Description: "A different problem",
		RecentError: &RecentError{
			Text: "old failure",
			At:   now.Add(-31 * time.Minute),
		},
		Environment: Environment{Product: "Example Agent"},
	})
	if err != nil {
		t.Fatalf("Build stale input: %v", err)
	}
	if strings.Contains(stale.Submission().Body, "old failure") {
		t.Fatalf("stale error was attached:\n%s", stale.Submission().Body)
	}
}

func TestBuilderRejectsEmptyOrUnsafeMetadata(t *testing.T) {
	builder := Builder{}
	if _, err := builder.Build(Input{Environment: Environment{Product: "Example"}}); !errors.Is(err, ErrNothingToReport) {
		t.Fatalf("empty report error = %v", err)
	}
	if _, err := builder.Build(Input{Description: "hello"}); err == nil || !strings.Contains(err.Error(), "product") {
		t.Fatalf("missing product error = %v", err)
	}
	if _, err := builder.Build(Input{
		Description: "hello",
		Environment: Environment{Product: "Example"},
		InstallID:   "not allowed/id",
	}); err == nil || !strings.Contains(err.Error(), "installation ID") {
		t.Fatalf("unsafe installation ID error = %v", err)
	}
}

func TestBuilderLimitsRemainValidUTF8(t *testing.T) {
	draft, err := (Builder{MaxDescription: 48, MaxBody: 420, MaxTitle: 32}).Build(Input{
		Description: strings.Repeat("反馈内容", 100),
		Environment: Environment{Product: "示例应用"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	submission := draft.Submission()
	if len(submission.Title) > 32 || len(submission.Body) > 420 {
		t.Fatalf("limits exceeded: title=%d body=%d", len(submission.Title), len(submission.Body))
	}
	if !utf8.ValidString(submission.Title) || !utf8.ValidString(submission.Body) {
		t.Fatal("truncation produced invalid UTF-8")
	}
}

func TestApprovalAndClientSubmission(t *testing.T) {
	var received Submission
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/feedback" {
			http.NotFound(writer, request)
			return
		}
		if got := request.Header.Get("User-Agent"); got != "example-agent/1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode submission: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"issue_url":"https://github.com/owner/repo/issues/7","deduplicated":true}`))
	}))
	defer server.Close()

	draft, err := (Builder{}).Build(Input{
		Description: "Please improve startup diagnostics",
		Environment: Environment{Product: "Example Agent", Version: "v1.0.0"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	client := Client{Endpoint: server.URL + "/v1/feedback", UserAgent: "example-agent/1.0"}
	if _, err := client.Submit(context.Background(), Approved{}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("zero Approved error = %v", err)
	}
	if _, err := draft.Approve(false); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("rejected approval error = %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	receipt, err := client.Submit(context.Background(), approved)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if receipt.IssueURL != "https://github.com/owner/repo/issues/7" || !receipt.Deduplicated {
		t.Fatalf("receipt = %+v", receipt)
	}
	if !received.UserApproved || received.Title != draft.Submission().Title {
		t.Fatalf("received submission = %+v", received)
	}
}

func TestClientRejectsInsecureRemoteEndpoint(t *testing.T) {
	client := Client{Endpoint: "http://feedback.example.com/v1/feedback"}
	approved := Approved{valid: true, submission: Submission{
		Schema:       1,
		UserApproved: true,
		Title:        "title",
		Body:         "body",
	}}
	if _, err := client.Submit(context.Background(), approved); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("insecure endpoint error = %v", err)
	}
}

func TestNewInstallIDIsRandomAndValid(t *testing.T) {
	first, err := NewInstallID()
	if err != nil {
		t.Fatalf("NewInstallID: %v", err)
	}
	second, err := NewInstallID()
	if err != nil {
		t.Fatalf("NewInstallID: %v", err)
	}
	if first == second || !validInstallID(first) || len(first) != 32 {
		t.Fatalf("unexpected IDs: %q %q", first, second)
	}
}
