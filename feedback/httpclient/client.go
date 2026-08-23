// Package httpclient provides a reusable HTTPS adapter for approved feedback
// reports. It has no knowledge of the relay's downstream issue tracker.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
)

const (
	// EndpointPath is the only wire path supported by this v1 client.
	EndpointPath          = "/v1/feedback"
	maxRelayResponseBytes = 64 * 1024
)

// Client sends approved reports to an author-operated relay.
type Client struct {
	Endpoint   string
	HTTPClient *http.Client
	UserAgent  string
}

// Receipt is returned by a successful relay submission.
type Receipt struct {
	ReferenceURL string `json:"reference_url"`
	Deduplicated bool   `json:"deduplicated,omitempty"`
}

// Submit sends an Approved report. It refuses zero-value or unapproved input
// before making a network request.
func (c Client) Submit(ctx context.Context, approved feedback.Approved) (Receipt, error) {
	payload, err := json.Marshal(approved)
	if err != nil {
		return Receipt{}, fmt.Errorf("encode feedback: %w", err)
	}
	endpoint, err := validateEndpoint(c.Endpoint)
	if err != nil {
		return Receipt{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return Receipt{}, fmt.Errorf("create feedback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	userAgent := strings.TrimSpace(c.UserAgent)
	if userAgent == "" {
		userAgent = "awesome-agent-app-features-feedback/1"
	}
	if len(userAgent) > 256 || strings.ContainsFunc(userAgent, func(character rune) bool {
		return character < 0x20 || character == 0x7f
	}) {
		return Receipt{}, fmt.Errorf("feedback user agent is invalid")
	}
	req.Header.Set("User-Agent", userAgent)

	client := http.Client{Timeout: 10 * time.Second}
	if c.HTTPClient != nil {
		client = *c.HTTPClient
		if client.Timeout <= 0 {
			client.Timeout = 10 * time.Second
		}
	}
	// A feedback POST is approved for exactly the configured endpoint. Preserve
	// custom transports and timeouts, but never inherit a redirect policy that
	// can replay the body to a different destination.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return Receipt{}, fmt.Errorf("submit feedback: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRelayResponseBytes+1))
	if err != nil {
		return Receipt{}, fmt.Errorf("read relay response: %w", err)
	}
	if len(body) > maxRelayResponseBytes {
		return Receipt{}, fmt.Errorf("relay response exceeded %d bytes", maxRelayResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Receipt{}, fmt.Errorf("relay returned HTTP %d", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return Receipt{}, fmt.Errorf("relay returned a non-JSON response")
	}
	var receipt Receipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return Receipt{}, fmt.Errorf("decode relay response: %w", err)
	}
	if receipt.ReferenceURL != strings.TrimSpace(receipt.ReferenceURL) {
		return Receipt{}, fmt.Errorf("relay returned an invalid reference URL")
	}
	referenceURL, err := url.Parse(receipt.ReferenceURL)
	if err != nil || referenceURL.Host == "" || referenceURL.User != nil || referenceURL.Fragment != "" ||
		(referenceURL.Scheme != "https" && !(referenceURL.Scheme == "http" && isLoopbackHost(referenceURL.Hostname()))) {
		return Receipt{}, fmt.Errorf("relay returned an invalid reference URL")
	}
	return receipt, nil
}

func validateEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" {
		return nil, fmt.Errorf("feedback endpoint is invalid")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.Opaque != "" {
		return nil, fmt.Errorf("feedback endpoint must not contain credentials, query parameters, or fragments")
	}
	if endpoint.EscapedPath() != EndpointPath {
		return nil, fmt.Errorf("feedback endpoint path must be %s", EndpointPath)
	}
	if endpoint.Scheme == "https" {
		return endpoint, nil
	}
	if endpoint.Scheme == "http" && isLoopbackHost(endpoint.Hostname()) {
		return endpoint, nil
	}
	return nil, fmt.Errorf("feedback endpoint must use HTTPS (HTTP is allowed only for loopback development)")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
