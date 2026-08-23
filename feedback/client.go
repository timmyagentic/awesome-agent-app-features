package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRelayResponseBytes = 64 * 1024

// Client sends approved reports to a relay. The GitHub credential belongs in
// the relay, never in this client.
type Client struct {
	Endpoint   string
	HTTPClient *http.Client
	UserAgent  string
}

// Receipt is returned by a successful relay submission.
type Receipt struct {
	IssueURL     string `json:"issue_url"`
	Deduplicated bool   `json:"deduplicated,omitempty"`
}

// Submit sends an Approved report. It refuses zero-value or unapproved input
// before making a network request.
func (c Client) Submit(ctx context.Context, approved Approved) (Receipt, error) {
	if !approved.valid || !approved.submission.UserApproved {
		return Receipt{}, ErrApprovalRequired
	}
	endpoint, err := validateEndpoint(c.Endpoint)
	if err != nil {
		return Receipt{}, err
	}
	payload, err := json.Marshal(approved.submission)
	if err != nil {
		return Receipt{}, fmt.Errorf("encode feedback: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return Receipt{}, fmt.Errorf("create feedback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	userAgent := strings.TrimSpace(c.UserAgent)
	if userAgent == "" {
		userAgent = "awesome-agent-app-features-feedback/1"
	}
	req.Header.Set("User-Agent", userAgent)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
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
	var receipt Receipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return Receipt{}, fmt.Errorf("decode relay response: %w", err)
	}
	issueURL, err := url.Parse(receipt.IssueURL)
	if err != nil || issueURL.Host == "" || (issueURL.Scheme != "https" && !(issueURL.Scheme == "http" && isLoopbackHost(issueURL.Hostname()))) {
		return Receipt{}, fmt.Errorf("relay returned an invalid issue URL")
	}
	return receipt, nil
}

func validateEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" {
		return nil, fmt.Errorf("feedback endpoint is invalid")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("feedback endpoint must not contain credentials, query parameters, or fragments")
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
