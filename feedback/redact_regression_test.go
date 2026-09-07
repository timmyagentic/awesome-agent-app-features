package feedback

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRedactQuotedCredentialsAndHTTPContext(t *testing.T) {
	for _, test := range []struct {
		name   string
		input  string
		secret string
	}{
		{"json key", `{"api_key":"test-secret"}`, "test-secret"},
		{"escaped quote", `{"password":"first\"last-secret"}`, "last-secret"},
		{"quoted identifier", `{"user_id":"private-user"}`, "private-user"},
		{"single quoted key", `'client_secret': 'test-secret'`, "test-secret"},
		{"access token", `access_token=test-secret`, "test-secret"},
		{"refresh token", `{"refresh_token":"test-secret"}`, "test-secret"},
		{"provider environment variable", `OPENAI_API_KEY=test-secret`, "test-secret"},
		{"prefixed JSON token", `{"github_token":"test-secret"}`, "test-secret"},
		{"camel case token", `{"accessToken":"test-secret"}`, "test-secret"},
		{"authorization JSON", `{"Authorization":"Bearer test-secret","status":401}`, "test-secret"},
		{"cookie JSON", `{"Cookie":"sid=test-secret; other=test-cookie"}`, "test-secret"},
		{"cookie header", "Cookie: sid=test-secret; other=test-cookie\nstatus=401", "test-secret"},
		{"set cookie", "Set-Cookie: sid=test-secret; HttpOnly", "test-secret"},
		{"URL password", `https://test-user:test-secret@example.invalid/path`, "test-secret"},
		{"URL username", `https://test-user:test-secret@example.invalid/path`, "test-user"},
		{"URL loopback password", `http://test-user:test-secret@localhost/path`, "test-secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Redact(test.input)
			if strings.Contains(got, test.secret) {
				t.Fatal("redaction retained the synthetic sensitive value")
			}
			if !strings.Contains(got, "[REDACTED") {
				t.Fatalf("missing redaction marker: %q", got)
			}
		})
	}
}

func TestBuilderRedactsQuotedCredentialsBeforeApprovalAndTruncation(t *testing.T) {
	secret := strings.Repeat("private with spaces ", 300)
	encoded, err := json.Marshal(map[string]string{"password": secret})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := (Builder{}).Build(Input{
		Description:    string(encoded),
		RecentError:    &RecentError{Text: string(encoded), At: time.Now()},
		CapabilityGaps: []string{`{"token":"test-secret"}`},
		Environment:    Environment{Product: "example", Agent: `{"token":"test-secret"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(approved)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{"private with spaces", "test-secret"} {
		if strings.Contains(string(wire), sensitive) {
			t.Fatal("approved wire payload retained a synthetic sensitive value")
		}
	}
}
