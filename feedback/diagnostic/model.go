// Package diagnostic adds turn-bound, bounded diagnostic reports and retry
// identities to Feedback. The original feedback package and v1 wire contract
// remain available to existing consumers.
package diagnostic

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

const WireSchemaVersion = 2

// Runtime is a fixed allowlist of effective settings, never a configuration
// map. Unknown values must be explained in Diagnostic.Missing.
type Runtime struct {
	Backend        string `json:"backend,omitempty"`
	AgentVersion   string `json:"agent_version,omitempty"`
	Platform       string `json:"platform,omitempty"`
	Model          string `json:"model,omitempty"`
	Effort         string `json:"effort,omitempty"`
	ServiceTier    string `json:"service_tier,omitempty"`
	Mode           string `json:"mode,omitempty"`
	SettingsSource string `json:"settings_source,omitempty"`
	IdleTimeoutMS  int64  `json:"idle_timeout_ms"`
	MaxTurnTimeMS  int64  `json:"max_turn_time_ms"`
}

// Activity contains labels and relative timing only. Tool arguments, outputs,
// reasoning and raw protocol messages deliberately have no representation.
type Activity struct {
	AfterMS int64  `json:"after_ms"`
	Kind    string `json:"kind"`
	Name    string `json:"name,omitempty"`
}

// Transport distinguishes observations from unavailable adapter facts. A nil
// pointer is unknown, not a false/zero measurement.
type Transport struct {
	ProcessState      string `json:"process_state,omitempty"`
	ReadState         string `json:"read_state,omitempty"`
	ExitCode          *int   `json:"exit_code,omitempty"`
	LastProtocolMS    *int64 `json:"last_protocol_ms,omitempty"`
	LastCoreMS        *int64 `json:"last_core_ms,omitempty"`
	LastDeliveryMS    *int64 `json:"last_delivery_ms,omitempty"`
	PendingRPC        *int   `json:"pending_rpc,omitempty"`
	QueueDepth        *int   `json:"queue_depth,omitempty"`
	QueueHighWater    *int   `json:"queue_high_water,omitempty"`
	DroppedEvents     *int   `json:"dropped_events,omitempty"`
	TerminalReceived  *bool  `json:"terminal_received,omitempty"`
	TerminalDelivered *bool  `json:"terminal_delivered,omitempty"`
}

// Diagnostic describes one causally bound turn. Its lifetime is independent
// of the v1 recent-error time window. The host is responsible for ownership,
// capture at failure time, local retention and explicit submission consent.
type Diagnostic struct {
	CaptureVersion int        `json:"capture_version"`
	StartedAt      time.Time  `json:"started_at"`
	OccurredAt     time.Time  `json:"occurred_at"`
	Phase          string     `json:"phase"`
	ErrorCode      string     `json:"error_code"`
	Request        string     `json:"request,omitempty"`
	Response       string     `json:"response,omitempty"`
	Error          string     `json:"error,omitempty"`
	Runtime        Runtime    `json:"runtime"`
	Transport      Transport  `json:"transport"`
	Activity       []Activity `json:"activity,omitempty"`
	Missing        []string   `json:"missing,omitempty"`
	Truncated      []string   `json:"truncated,omitempty"`
}

type Input struct {
	Description    string
	RecentError    *feedback.RecentError
	CapabilityGaps []string
	Environment    feedback.Environment
	Diagnostic     *Diagnostic
	// ReportID is a random per-report identity, never an installation/user ID.
	// Leave it empty for a new report; preserve it for an exact stored retry.
	ReportID string
}

// Report is the complete preview. Like Feedback v1 it cannot be marshaled as
// a submission. Only Approved can cross the transport boundary.
type Report struct {
	Description    string
	RecentError    *feedback.RecentError
	CapabilityGaps []string
	Environment    feedback.Environment
	Diagnostic     *Diagnostic
	ReportID       string
}

func (Report) MarshalJSON() ([]byte, error) { return nil, feedback.ErrApprovalRequired }

type Draft struct {
	base       feedback.Draft
	diagnostic *Diagnostic
	id         string
	preparedAt time.Time
}

// PreparedAt is the immutable instant used for all freshness and timestamp
// validation. Persist it with the preview to restore exactly the same report.
func (d Draft) PreparedAt() time.Time { return d.preparedAt }

func (d Draft) Report() Report {
	base := d.base.Report()
	return Report{Description: base.Description, RecentError: base.RecentError,
		CapabilityGaps: base.CapabilityGaps, Environment: base.Environment,
		Diagnostic: clone(d.diagnostic), ReportID: d.id}
}

func (d Draft) Approve(userConfirmed bool) (Approved, error) {
	base, err := d.base.Approve(userConfirmed)
	if err != nil {
		return Approved{}, err
	}
	if !validID(d.id) {
		return Approved{}, fmt.Errorf("invalid feedback report identity")
	}
	return Approved{base: base, diagnostic: clone(d.diagnostic), id: d.id}, nil
}

type Approved struct {
	base       feedback.Approved
	diagnostic *Diagnostic
	id         string
}

func (a Approved) MarshalJSON() ([]byte, error) {
	if !validID(a.id) {
		return nil, feedback.ErrApprovalRequired
	}
	data, err := json.Marshal(a.base)
	if err != nil {
		return nil, err
	}
	// Reuse v1's exact bounded environment/error encoding. This private map
	// is constructed only from the opaque approved value, never host maps.
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	wire["schema"] = json.RawMessage("2")
	wire["report_id"], err = json.Marshal(a.id)
	if err != nil {
		return nil, err
	}
	if a.diagnostic != nil {
		wire["diagnostic"], err = json.Marshal(a.diagnostic)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(wire)
}

func validID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && strings.ToLower(id) == id
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func clone(d *Diagnostic) *Diagnostic {
	if d == nil {
		return nil
	}
	copy := *d
	copy.Activity = append([]Activity(nil), d.Activity...)
	copy.Missing = append([]string(nil), d.Missing...)
	copy.Truncated = append([]string(nil), d.Truncated...)
	copy.Transport.ExitCode = pointerCopy(d.Transport.ExitCode)
	copy.Transport.LastProtocolMS = pointerCopy(d.Transport.LastProtocolMS)
	copy.Transport.LastCoreMS = pointerCopy(d.Transport.LastCoreMS)
	copy.Transport.LastDeliveryMS = pointerCopy(d.Transport.LastDeliveryMS)
	copy.Transport.PendingRPC = pointerCopy(d.Transport.PendingRPC)
	copy.Transport.QueueDepth = pointerCopy(d.Transport.QueueDepth)
	copy.Transport.QueueHighWater = pointerCopy(d.Transport.QueueHighWater)
	copy.Transport.DroppedEvents = pointerCopy(d.Transport.DroppedEvents)
	copy.Transport.TerminalReceived = pointerCopy(d.Transport.TerminalReceived)
	copy.Transport.TerminalDelivered = pointerCopy(d.Transport.TerminalDelivered)
	return &copy
}

func pointerCopy[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
