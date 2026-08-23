package feedback

import (
	"encoding/json"
	"errors"
	"reflect"
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

func TestBuilderCreatesStructuredReportAndDropsStaleError(t *testing.T) {
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
	report := draft.Report()
	if report.Environment.Product != "Example Agent" || report.Environment.Version != "v1.2.3" {
		t.Fatalf("environment = %+v", report.Environment)
	}
	if !strings.Contains(report.Description, "Please add webhook retries") || strings.Contains(report.Description, "do-not-send") {
		t.Fatalf("description = %q", report.Description)
	}
	if report.RecentError == nil || !strings.Contains(report.RecentError.Text, "[REDACTED-PATH]") {
		t.Fatalf("recent error = %+v", report.RecentError)
	}
	wantGaps := []string{"agent.fast_mode", "webhook.retry"}
	if !reflect.DeepEqual(report.CapabilityGaps, wantGaps) {
		t.Fatalf("capability gaps = %v, want %v", report.CapabilityGaps, wantGaps)
	}

	if _, err := json.Marshal(report); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("preview report marshal error = %v", err)
	}
	reportType := reflect.TypeOf(Report{})
	for _, field := range []string{"Title", "Body", "Repository", "IssueURL"} {
		if _, exists := reportType.FieldByName(field); exists {
			t.Fatalf("provider-neutral report exposes presentation/provider field %s", field)
		}
	}

	// Report must be a deep copy so a renderer cannot mutate the draft that
	// is later approved.
	report.CapabilityGaps[0] = "mutated"
	report.RecentError.Text = "mutated"
	again := draft.Report()
	if again.CapabilityGaps[0] == "mutated" || again.RecentError.Text == "mutated" {
		t.Fatal("Report returned mutable draft storage")
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
	if stale.Report().RecentError != nil {
		t.Fatalf("stale error was attached: %+v", stale.Report().RecentError)
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
	draft, err := (Builder{AdditionalRedact: func(value string) string {
		return strings.ReplaceAll(value, "internal-project", "[PRODUCT-REDACTED]")
	}}).Build(Input{
		Description: "internal-project token=must-never-pass",
		Environment: Environment{Product: "Example"},
	})
	if err != nil {
		t.Fatalf("Build with additional redaction: %v", err)
	}
	if got := draft.Report().Description; strings.Contains(got, "internal-project") || strings.Contains(got, "must-never-pass") {
		t.Fatalf("combined redaction leaked input: %q", got)
	}
	_, err = (Builder{AdditionalRedact: func(value string) string {
		if value == "remove all of this" {
			return ""
		}
		return value
	}}).Build(Input{
		RecentError: &RecentError{Text: "remove all of this", At: time.Now()},
		Environment: Environment{Product: "Example"},
	})
	if !errors.Is(err, ErrNothingToReport) {
		t.Fatalf("fully removed recent error = %v", err)
	}
}

func TestBuilderFieldLimitsRemainValidUTF8(t *testing.T) {
	draft, err := (Builder{}).Build(Input{
		Description: strings.Repeat("反馈内容", 1000),
		RecentError: &RecentError{
			Text: strings.Repeat("错误详情", 1000),
			At:   time.Now(),
		},
		CapabilityGaps: []string{strings.Repeat("能力缺口", 1000)},
		Environment:    Environment{Product: "示例应用"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	report := draft.Report()
	values := []struct {
		name  string
		value string
		limit int
	}{
		{"description", report.Description, MaxDescriptionBytes},
		{"recent error", report.RecentError.Text, MaxErrorBytes},
		{"capability gap", report.CapabilityGaps[0], MaxCapabilityGapBytes},
	}
	for _, value := range values {
		if len(value.value) > value.limit || !utf8.ValidString(value.value) {
			t.Errorf("%s is invalid: bytes=%d value=%q", value.name, len(value.value), value.value)
		}
	}
}

func TestApprovalProducesOnlyValidWirePayload(t *testing.T) {
	if _, err := json.Marshal(Approved{}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("zero Approved marshal error = %v", err)
	}
	draft, err := (Builder{}).Build(Input{
		Description: "Please improve startup diagnostics",
		Environment: Environment{Product: "Example Agent", Version: "v1.0.0"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := draft.Approve(false); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("rejected approval error = %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	payload, err := json.Marshal(approved)
	if err != nil {
		t.Fatalf("marshal Approved: %v", err)
	}
	var received struct {
		Schema       int    `json:"schema"`
		UserApproved bool   `json:"user_approved"`
		Description  string `json:"description"`
	}
	if err := json.Unmarshal(payload, &received); err != nil {
		t.Fatalf("decode Approved: %v", err)
	}
	if received.Schema != WireSchemaVersion || !received.UserApproved || received.Description != draft.Report().Description {
		t.Fatalf("approved payload = %+v", received)
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
