package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

func TestClientSubmitsOnlyApprovedStructuredReports(t *testing.T) {
	var received struct {
		Schema       int    `json:"schema"`
		UserApproved bool   `json:"user_approved"`
		Description  string `json:"description"`
	}
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
		_, _ = writer.Write([]byte(`{"reference_url":"https://github.com/owner/repo/issues/7","deduplicated":true}`))
	}))
	defer server.Close()

	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: "Please improve startup diagnostics",
		Environment: feedback.Environment{Product: "Example Agent", Version: "v1.0.0"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	client := Client{Endpoint: server.URL + "/v1/feedback", UserAgent: "example-agent/1.0"}
	if _, err := client.Submit(context.Background(), feedback.Approved{}); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatalf("zero Approved error = %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	receipt, err := client.Submit(context.Background(), approved)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if receipt.ReferenceURL != "https://github.com/owner/repo/issues/7" || !receipt.Deduplicated {
		t.Fatalf("receipt = %+v", receipt)
	}
	if received.Schema != feedback.WireSchemaVersion || !received.UserApproved || received.Description != draft.Report().Description {
		t.Fatalf("received submission = %+v", received)
	}
}

func TestClientRejectsInsecureRemoteEndpoint(t *testing.T) {
	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: "hello",
		Environment: feedback.Environment{Product: "Example"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	client := Client{Endpoint: "http://feedback.example.com/v1/feedback"}
	if _, err := client.Submit(context.Background(), approved); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("insecure endpoint error = %v", err)
	}
}

func TestClientRejectsWrongProtocolPathBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: "hello",
		Environment: feedback.Environment{Product: "Example"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	client := Client{Endpoint: server.URL + "/v2/feedback"}
	if _, err := client.Submit(context.Background(), approved); err == nil || !strings.Contains(err.Error(), EndpointPath) {
		t.Fatalf("wrong-path error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("wrong protocol path made %d request(s)", requests.Load())
	}
}

func TestClientRefusesRedirectEvenWithFollowingHTTPClient(t *testing.T) {
	var forwarded atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		forwarded.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"reference_url":"https://example.com/feedback/1"}`))
	}))
	defer sink.Close()
	relay := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer relay.Close()

	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: "hello",
		Environment: feedback.Environment{Product: "Example"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	client := Client{Endpoint: relay.URL + "/v1/feedback", HTTPClient: relay.Client()}
	if _, err := client.Submit(context.Background(), approved); err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("redirect response error = %v", err)
	}
	if forwarded.Load() != 0 {
		t.Fatalf("approved report followed redirect %d time(s)", forwarded.Load())
	}
}

func TestClientBoundsAndValidatesRelayResponse(t *testing.T) {
	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: "hello",
		Environment: feedback.Environment{Product: "Example"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        string
	}{
		{"oversized", http.StatusOK, "application/json", strings.Repeat("x", maxRelayResponseBytes+1), "exceeded"},
		{"non-json", http.StatusOK, "text/plain", `{}`, "non-JSON"},
		{"invalid-reference", http.StatusOK, "application/json", `{"reference_url":"https://example.com/value#fragment"}`, "invalid reference"},
		{"upstream-error", http.StatusServiceUnavailable, "application/json", `{}`, "HTTP 503"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			client := Client{Endpoint: server.URL + EndpointPath}
			if _, err := client.Submit(context.Background(), approved); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Submit error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateEndpointRejectsAmbiguousURLs(t *testing.T) {
	for _, raw := range []string{
		"https://user@example.com/v1/feedback",
		"https://example.com/v1/feedback?target=other",
		"https://example.com/v1/feedback#fragment",
		"https://example.com/v1%2ffeedback",
		"https://example.com/v1/feedback/",
	} {
		if _, err := validateEndpoint(raw); err == nil {
			t.Errorf("accepted endpoint %q", raw)
		}
	}
}

func TestClientRejectsInvalidUserAgentBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	draft, err := (feedback.Builder{}).Build(feedback.Input{
		Description: "hello",
		Environment: feedback.Environment{Product: "Example"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	client := Client{Endpoint: server.URL + EndpointPath, UserAgent: "invalid\nvalue"}
	if _, err := client.Submit(context.Background(), approved); err == nil || !strings.Contains(err.Error(), "user agent") {
		t.Fatalf("invalid user-agent error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid user agent made %d request(s)", requests.Load())
	}
}
