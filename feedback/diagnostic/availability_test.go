package diagnostic

import (
	"slices"
	"testing"
)

func TestCaptureMarksTerminalObservationsIndependently(t *testing.T) {
	knownFalse := false
	knownTrue := true
	for _, test := range []struct {
		name                string
		received, delivered *bool
	}{
		{"received_only", &knownTrue, nil},
		{"delivered_only", nil, &knownFalse},
		{"both_known_false", &knownFalse, &knownFalse},
		{"both_unknown", nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := (Builder{}).Capture(Diagnostic{Transport: Transport{TerminalReceived: test.received, TerminalDelivered: test.delivered}})
			if got := slices.Contains(d.Missing, "transport.terminal_received: unavailable"); got != (test.received == nil) {
				t.Fatalf("terminal receipt availability = %v, missing = %v", got, d.Missing)
			}
			if got := slices.Contains(d.Missing, "transport.terminal_delivered: unavailable"); got != (test.delivered == nil) {
				t.Fatalf("terminal delivery availability = %v, missing = %v", got, d.Missing)
			}
		})
	}
}
