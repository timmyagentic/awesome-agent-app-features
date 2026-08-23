package feedback

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrApprovalRequired means a host attempted to submit without recording an
	// explicit user confirmation.
	ErrApprovalRequired = errors.New("explicit user approval is required")
	// ErrNothingToReport means an input contained no description, recent error,
	// or capability gap.
	ErrNothingToReport = errors.New("feedback has no reportable content")
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

// Submission is schema 1 of the relay wire format. A Draft exposes this value
// for preview, but Client.Submit accepts only Approved.
type Submission struct {
	Schema       int    `json:"schema"`
	UserApproved bool   `json:"user_approved"`
	InstallID    string `json:"install_id,omitempty"`
	Product      string `json:"product"`
	Version      string `json:"version,omitempty"`
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	Agent        string `json:"agent,omitempty"`
	Title        string `json:"title"`
	Body         string `json:"body"`
}

// Draft is a fully redacted report awaiting a user decision.
type Draft struct {
	submission Submission
}

// Submission returns a copy of the exact outbound fields before approval.
func (d Draft) Submission() Submission {
	return d.submission
}

// Preview returns a Markdown preview containing the issue and all metadata
// sent to the relay. The host should display this, or an equivalent complete
// rendering, before asking for approval.
func (d Draft) Preview() string {
	s := d.submission
	installID := s.InstallID
	if installID == "" {
		installID = "(not sent)"
	}
	return fmt.Sprintf(
		"## %s\n\n%s\n\n---\n\n**Outbound metadata**\n\n- Schema: %d\n- Product: %s\n- Version: %s\n- OS/Arch: %s/%s\n- Agent: %s\n- Installation ID: %s\n",
		s.Title,
		s.Body,
		s.Schema,
		previewValue(s.Product),
		previewValue(s.Version),
		previewValue(s.OS),
		previewValue(s.Arch),
		previewValue(s.Agent),
		installID,
	)
}

func previewValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(not sent)"
	}
	return value
}

// Approve creates the only value accepted by Client.Submit. userConfirmed
// must come from an explicit user action in the host application.
func (d Draft) Approve(userConfirmed bool) (Approved, error) {
	if !userConfirmed {
		return Approved{}, ErrApprovalRequired
	}
	s := d.submission
	if s.Schema != 1 || strings.TrimSpace(s.Title) == "" || strings.TrimSpace(s.Body) == "" {
		return Approved{}, fmt.Errorf("approve invalid draft")
	}
	s.UserApproved = true
	return Approved{submission: s, valid: true}, nil
}

// Approved is intentionally opaque outside this package. Its zero value is
// rejected by Client.Submit.
type Approved struct {
	submission Submission
	valid      bool
}
