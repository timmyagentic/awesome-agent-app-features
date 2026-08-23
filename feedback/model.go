package feedback

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// WireSchemaVersion is the exact JSON protocol version emitted by Approved.
// Wire-incompatible changes require a new protocol version and endpoint.
const WireSchemaVersion = 1

var (
	// ErrApprovalRequired means a host attempted to serialize or submit a report
	// without recording an explicit user confirmation.
	ErrApprovalRequired error = errors.New("explicit user approval is required")
	// ErrNothingToReport means an input contained no description, recent error,
	// or capability gap.
	ErrNothingToReport error = errors.New("feedback has no reportable content")
)

// Environment is the complete allowlist of automatically captured fields.
// Do not add arbitrary environment maps: they make accidental secret capture
// too easy.
type Environment struct {
	Product string
	Version string
	OS      string
	Arch    string
	Agent   string
}

// RecentError is optional context captured by the host application. Builder
// drops it when At falls outside ErrorMaxAge.
type RecentError struct {
	Text string
	At   time.Time
}

// Input describes one feedback report. Errors and capability gaps are context,
// not report categories.
type Input struct {
	Description    string
	RecentError    *RecentError
	CapabilityGaps []string
	Environment    Environment
	InstallID      string
}

// Report is the complete provider-neutral preview a host must render before
// approval. It deliberately cannot be JSON-marshaled; only Approved can cross
// the provided transport boundary.
type Report struct {
	InstallID      string
	Environment    Environment
	Description    string
	RecentError    *RecentError
	CapabilityGaps []string
}

// MarshalJSON prevents a preview report from being mistaken for an approved
// wire payload.
func (Report) MarshalJSON() ([]byte, error) {
	return nil, ErrApprovalRequired
}

// Draft is a fully redacted structured report awaiting a user decision.
type Draft struct {
	report Report
}

// Report returns a deep copy of every outbound field. The host owns how it
// renders this structure and must show an equivalent complete representation
// before asking for approval.
func (d Draft) Report() Report {
	return cloneReport(d.report)
}

// Approve creates the only value serializable by the provided HTTP adapter.
// userConfirmed must come from an explicit user action in the host application.
func (d Draft) Approve(userConfirmed bool) (Approved, error) {
	if !userConfirmed {
		return Approved{}, ErrApprovalRequired
	}
	report := cloneReport(d.report)
	if strings.TrimSpace(report.Environment.Product) == "" || !hasReportableContent(report) {
		return Approved{}, fmt.Errorf("approve invalid draft")
	}
	return Approved{report: report, valid: true}, nil
}

// Approved is intentionally opaque outside this package. Its zero value cannot
// be serialized or submitted by the provided transport adapter.
type Approved struct {
	report Report
	valid  bool
}

// MarshalJSON implements json.Marshaler so transport adapters can serialize an
// approved report without exposing mutable internal state.
func (approved Approved) MarshalJSON() ([]byte, error) {
	if !approved.valid || !hasReportableContent(approved.report) {
		return nil, ErrApprovalRequired
	}
	report := cloneReport(approved.report)
	type wireEnvironment struct {
		Product string `json:"product"`
		Version string `json:"version,omitempty"`
		OS      string `json:"os,omitempty"`
		Arch    string `json:"arch,omitempty"`
		Agent   string `json:"agent,omitempty"`
	}
	type wireRecentError struct {
		Text       string    `json:"text"`
		OccurredAt time.Time `json:"occurred_at"`
	}
	type wireSubmission struct {
		Schema         int              `json:"schema"`
		UserApproved   bool             `json:"user_approved"`
		InstallID      string           `json:"install_id,omitempty"`
		Environment    wireEnvironment  `json:"environment"`
		Description    string           `json:"description,omitempty"`
		RecentError    *wireRecentError `json:"recent_error,omitempty"`
		CapabilityGaps []string         `json:"capability_gaps,omitempty"`
	}
	wire := wireSubmission{
		Schema:       WireSchemaVersion,
		UserApproved: true,
		InstallID:    report.InstallID,
		Environment: wireEnvironment{
			Product: report.Environment.Product,
			Version: report.Environment.Version,
			OS:      report.Environment.OS,
			Arch:    report.Environment.Arch,
			Agent:   report.Environment.Agent,
		},
		Description:    report.Description,
		CapabilityGaps: append([]string(nil), report.CapabilityGaps...),
	}
	if report.RecentError != nil {
		wire.RecentError = &wireRecentError{
			Text:       report.RecentError.Text,
			OccurredAt: report.RecentError.At,
		}
	}
	return json.Marshal(wire)
}

func cloneReport(value Report) Report {
	clone := value
	if value.RecentError != nil {
		recent := *value.RecentError
		clone.RecentError = &recent
	}
	clone.CapabilityGaps = append([]string(nil), value.CapabilityGaps...)
	return clone
}

func hasReportableContent(value Report) bool {
	return strings.TrimSpace(value.Description) != "" ||
		(value.RecentError != nil && strings.TrimSpace(value.RecentError.Text) != "") ||
		len(value.CapabilityGaps) > 0
}
