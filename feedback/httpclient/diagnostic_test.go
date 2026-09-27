package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
	"github.com/timmyagentic/awesome-agent-app-features/feedback/diagnostic"
)

func diagnosticApproval(t *testing.T) diagnostic.Approved {
	t.Helper()
	draft, err := (diagnostic.Builder{}).Build(diagnostic.Input{Description: "one immutable report", Environment: feedback.Environment{Product: "Example"}})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

func TestDiagnosticClientRetriesSamePayloadAndNeverDowngrades(t *testing.T) {
	for _, status := range []int{503, 502, 400, 404, 409, 429, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			var first string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != DiagnosticEndpointPath {
					t.Errorf("protocol fallback: %s", r.URL.Path)
				}
				payload, _ := io.ReadAll(r.Body)
				if calls.Add(1) == 1 {
					first = string(payload)
					w.Header().Set("Location", "/v1/feedback")
					w.WriteHeader(status)
					return
				}
				if string(payload) != first {
					t.Error("retry changed the approved body")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"reference_url":"https://github.com/owner/repo/issues/1","deduplicated":true}`)
			}))
			defer server.Close()
			client := Client{Endpoint: server.URL + DiagnosticEndpointPath}
			if _, err := client.SubmitDiagnostic(context.Background(), diagnostic.Approved{}); !errors.Is(err, feedback.ErrApprovalRequired) {
				t.Fatalf("unapproved error: %v", err)
			}
			if calls.Load() != 0 {
				t.Fatal("unapproved network request")
			}
			receipt, err := client.SubmitDiagnostic(context.Background(), diagnosticApproval(t))
			want := int32(1)
			if status == 502 || status == 503 {
				want = 2
				if err != nil || !receipt.Deduplicated {
					t.Fatalf("retry receipt=%#v error=%v", receipt, err)
				}
			} else if err == nil {
				t.Fatal("rejected request reported success")
			}
			if calls.Load() != want {
				t.Fatalf("calls=%d want=%d", calls.Load(), want)
			}
		})
	}
}

func TestDiagnosticClientBoundsRetryByCallerDeadline(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (Client{Endpoint: server.URL + DiagnosticEndpointPath}).SubmitDiagnostic(ctx, diagnosticApproval(t))
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second || calls.Load() != 1 {
		t.Fatalf("deadline was not preserved: calls=%d err=%v", calls.Load(), err)
	}
}
