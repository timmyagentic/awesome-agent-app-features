package diagnostic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

func TestApprovedWireMatchesSharedFeedbackV2Fixtures(t *testing.T) {
	for _, name := range []string{"valid-full.json", "valid-minimal.json"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "protocol", "feedback", "v2", "testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Description string               `json:"description"`
				Environment feedback.Environment `json:"environment"`
				Diagnostic  *Diagnostic          `json:"diagnostic"`
				ReportID    string               `json:"report_id"`
			}
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			draft, err := (Builder{Now: func() time.Time { return time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC) }}).Build(Input{Description: fixture.Description, Environment: fixture.Environment, Diagnostic: fixture.Diagnostic, ReportID: fixture.ReportID})
			if err != nil {
				t.Fatal(err)
			}
			approved, err := draft.Approve(true)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(approved)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Go wire differs from shared Worker fixture\ngot %s\nwant %s", encoded, data)
			}
		})
	}
}
