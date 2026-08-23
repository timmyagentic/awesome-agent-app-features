package feedback

import (
	"encoding/json"
	"testing"
	"time"
	"unicode/utf8"
)

func FuzzBuilderProducesBoundedValidApprovedJSON(f *testing.F) {
	f.Add("Improve diagnostics", "failure", "doctor.explain")
	f.Add("token=secret", "/Users/alice/private", "feature.missing")
	f.Add("反馈内容", "错误详情", "能力缺口")
	f.Fuzz(func(t *testing.T, description, recentError, gap string) {
		now := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
		draft, err := (Builder{Now: func() time.Time { return now }}).Build(Input{
			Description: description,
			RecentError: &RecentError{
				Text: recentError,
				At:   now,
			},
			CapabilityGaps: []string{gap, "always-reportable"},
			Environment: Environment{
				Product: "Fuzz Product",
				OS:      "test-os",
				Arch:    "test-arch",
			},
		})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		report := draft.Report()
		if len(report.Description) > MaxDescriptionBytes || !utf8.ValidString(report.Description) {
			t.Fatalf("invalid description bytes=%d", len(report.Description))
		}
		if report.RecentError != nil && (len(report.RecentError.Text) > MaxErrorBytes || !utf8.ValidString(report.RecentError.Text)) {
			t.Fatalf("invalid recent error bytes=%d", len(report.RecentError.Text))
		}
		if len(report.CapabilityGaps) > MaxCapabilityGaps {
			t.Fatalf("capability gap count=%d", len(report.CapabilityGaps))
		}
		for _, value := range report.CapabilityGaps {
			if len(value) > MaxCapabilityGapBytes || !utf8.ValidString(value) {
				t.Fatalf("invalid capability gap bytes=%d", len(value))
			}
		}
		approved, err := draft.Approve(true)
		if err != nil {
			t.Fatalf("Approve: %v", err)
		}
		wire, err := json.Marshal(approved)
		if err != nil || !json.Valid(wire) {
			t.Fatalf("approved JSON = %q, %v", wire, err)
		}
	})
}

func FuzzRedactAlwaysReturnsValidUTF8(f *testing.F) {
	f.Add("Authorization: Bearer secret")
	f.Add("owner@example.com /Users/alice/private")
	f.Fuzz(func(t *testing.T, value string) {
		if result := Redact(value); !utf8.ValidString(result) {
			t.Fatalf("Redact returned invalid UTF-8")
		}
	})
}
