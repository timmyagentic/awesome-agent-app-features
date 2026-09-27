package diagnostic

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

const (
	MaxRequestBytes  = 4000
	MaxResponseBytes = 1200
	MaxActivity      = 24
	MaxMissing       = 24
	maxElapsedMS     = int64((30 * 24 * time.Hour) / time.Millisecond)
)

type Builder struct {
	Now              func() time.Time
	AdditionalRedact func(string) string
}

func (b Builder) Build(input Input) (Draft, error) {
	now := time.Now().UTC()
	if b.Now != nil {
		now = b.Now().UTC()
	}
	b.Now = func() time.Time { return now }
	id := input.ReportID
	if id == "" {
		var err error
		id, err = newID()
		if err != nil {
			return Draft{}, fmt.Errorf("feedback identity: %w", err)
		}
	} else if !validID(id) {
		return Draft{}, fmt.Errorf("invalid feedback report identity")
	}
	var diagnostic *Diagnostic
	if input.Diagnostic != nil {
		value := b.Capture(*input.Diagnostic)
		diagnostic = &value
	}
	description := input.Description
	if strings.TrimSpace(description) == "" && diagnostic != nil && diagnostic.Error != "" {
		description, _, _ = strings.Cut(diagnostic.Error, "\n")
	}
	base, err := (feedback.Builder{Now: b.Now, AdditionalRedact: b.AdditionalRedact}).Build(feedback.Input{
		Description: description, RecentError: input.RecentError,
		CapabilityGaps: input.CapabilityGaps, Environment: input.Environment,
	})
	if err != nil {
		return Draft{}, err
	}
	return Draft{base: base, diagnostic: diagnostic, id: id, preparedAt: now}, nil
}

// Capture returns a deep-copied, sanitized, bounded snapshot suitable for the
// host's local store. It does not grant approval and performs no I/O.
func (b Builder) Capture(input Diagnostic) Diagnostic {
	d := clone(&input)
	d.CaptureVersion = 1
	d.Truncated = boundedList(d.Truncated, b.redact)
	d.Missing = boundedList(d.Missing, b.redact)
	bound := func(value, field string, maximum int, oneLine bool) string {
		value = b.redact(value)
		value = strings.Map(func(r rune) rune {
			if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
				return -1
			}
			return r
		}, strings.ToValidUTF8(value, "�"))
		if oneLine {
			value = strings.Join(strings.Fields(value), " ")
		}
		value = strings.TrimSpace(value)
		if len(value) > maximum {
			d.Truncated = appendUnique(d.Truncated, field)
			value = bounded(value, maximum)
		}
		return value
	}
	d.Request = bound(d.Request, "request", MaxRequestBytes, false)
	d.Response = bound(d.Response, "response", MaxResponseBytes, false)
	d.Error = bound(d.Error, "error", feedback.MaxErrorBytes, false)
	if d.Request == "" {
		d.Missing = appendUnique(d.Missing, "request: unavailable")
	}
	d.Phase = bound(d.Phase, "phase", 64, true)
	d.ErrorCode = bound(d.ErrorCode, "error_code", 64, true)
	if d.Phase == "" {
		d.Phase = "unknown"
		d.Missing = appendUnique(d.Missing, "phase: unavailable")
	}
	if d.ErrorCode == "" {
		d.ErrorCode = "unknown"
	}
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"backend", &d.Runtime.Backend}, {"agent_version", &d.Runtime.AgentVersion},
		{"platform", &d.Runtime.Platform}, {"model", &d.Runtime.Model},
		{"effort", &d.Runtime.Effort}, {"service_tier", &d.Runtime.ServiceTier}, {"mode", &d.Runtime.Mode},
		{"settings_source", &d.Runtime.SettingsSource},
	} {
		*field.value = bound(*field.value, "runtime."+field.name, 160, true)
		if *field.value == "" {
			d.Missing = appendUnique(d.Missing, "runtime."+field.name+": unavailable")
		}
	}
	d.Transport.ProcessState = bound(d.Transport.ProcessState, "transport.process_state", 64, true)
	d.Transport.ReadState = bound(d.Transport.ReadState, "transport.read_state", 64, true)
	if d.Transport.ProcessState == "" {
		d.Missing = appendUnique(d.Missing, "transport.process_state: unavailable")
	}
	if d.Transport.LastProtocolMS == nil {
		d.Missing = appendUnique(d.Missing, "transport.protocol: unavailable")
	}
	if d.Transport.TerminalReceived == nil {
		d.Missing = appendUnique(d.Missing, "transport.terminal: unavailable")
	}
	if d.Transport.ReadState == "" {
		d.Missing = appendUnique(d.Missing, "transport.read_state: unavailable")
	}
	for _, field := range []struct {
		name  string
		value *int
	}{
		{"exit_code", d.Transport.ExitCode}, {"pending_rpc", d.Transport.PendingRPC},
		{"queue_depth", d.Transport.QueueDepth}, {"queue_high_water", d.Transport.QueueHighWater}, {"dropped_events", d.Transport.DroppedEvents},
	} {
		if field.value == nil {
			d.Missing = appendUnique(d.Missing, "transport."+field.name+": unavailable")
		}
	}
	if d.Transport.LastCoreMS == nil {
		d.Missing = appendUnique(d.Missing, "transport.core_event: unavailable")
	}
	if d.Transport.LastDeliveryMS == nil {
		d.Missing = appendUnique(d.Missing, "transport.delivery: unavailable")
	}
	d.Runtime.IdleTimeoutMS = clamp(d.Runtime.IdleTimeoutMS)
	d.Runtime.MaxTurnTimeMS = clamp(d.Runtime.MaxTurnTimeMS)
	for _, elapsed := range []*int64{d.Transport.LastProtocolMS, d.Transport.LastCoreMS, d.Transport.LastDeliveryMS} {
		if elapsed != nil {
			*elapsed = clamp(*elapsed)
		}
	}
	for _, count := range []*int{d.Transport.PendingRPC, d.Transport.QueueDepth, d.Transport.QueueHighWater, d.Transport.DroppedEvents} {
		if count != nil {
			*count = max(0, min(*count, 1_000_000_000))
		}
	}
	if d.Transport.ExitCode != nil {
		*d.Transport.ExitCode = max(-65535, min(*d.Transport.ExitCode, 65535))
	}
	if len(d.Activity) > MaxActivity {
		d.Activity = append([]Activity(nil), d.Activity[len(d.Activity)-MaxActivity:]...)
		d.Truncated = appendUnique(d.Truncated, "activity")
	}
	for i := range d.Activity {
		d.Activity[i].AfterMS = clamp(d.Activity[i].AfterMS)
		d.Activity[i].Kind = bound(d.Activity[i].Kind, "activity.kind", 64, true)
		if d.Activity[i].Kind == "" {
			d.Activity[i].Kind = "unknown"
		}
		d.Activity[i].Name = bound(d.Activity[i].Name, "activity.name", 96, true)
	}
	now := time.Now()
	if b.Now != nil {
		now = b.Now()
	}
	if d.StartedAt.IsZero() || d.StartedAt.After(now) || d.OccurredAt.IsZero() || d.OccurredAt.After(now) || d.OccurredAt.Before(d.StartedAt) {
		d.StartedAt, d.OccurredAt = time.Time{}, time.Time{}
		d.Missing = appendUnique(d.Missing, "timestamps: unavailable or invalid")
	} else {
		d.StartedAt, d.OccurredAt = d.StartedAt.UTC(), d.OccurredAt.UTC()
	}
	d.Missing = boundedList(d.Missing, b.redact)
	d.Truncated = boundedList(d.Truncated, b.redact)
	return *d
}

func (b Builder) redact(value string) string {
	value = feedback.Redact(value)
	if b.AdditionalRedact != nil {
		value = b.AdditionalRedact(value)
	}
	return feedback.Redact(value)
}

func bounded(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum-len(" [truncated]")]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + " [truncated]"
}

func boundedList(values []string, redact func(string) string) []string {
	var result []string
	for _, value := range values {
		value = strings.Map(func(r rune) rune {
			if (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f {
				return -1
			}
			return r
		}, strings.ToValidUTF8(redact(value), "�"))
		value = bounded(strings.Join(strings.Fields(value), " "), 160)
		if value != "" {
			result = appendUnique(result, value)
		}
		if len(result) == MaxMissing {
			break
		}
	}
	return result
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func clamp(value int64) int64 { return max(0, min(value, maxElapsedMS)) }
