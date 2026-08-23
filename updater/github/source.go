// Package github provides a public GitHub Releases adapter for updater.Source.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/updater"
)

const (
	maxReleaseResponseBytes = 2 * 1024 * 1024
	defaultAPITimeout       = 15 * time.Second
	defaultDownloadTimeout  = 5 * time.Minute
)

var repositoryPartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Source reads one repository's latest stable GitHub Release and downloads
// only assets represented by updater.Asset values. The zero HTTP settings use
// public github.com with conservative timeouts and redirect validation.
type Source struct {
	Repository        string
	HTTPClient        *http.Client
	APIBaseURL        string
	UserAgent         string
	AllowedAssetHosts []string
}

// LatestStable implements updater.Source.
func (source Source) LatestStable(ctx context.Context) (updater.Release, error) {
	config, err := source.configuration()
	if err != nil {
		return updater.Release{}, err
	}
	requestURL, err := url.JoinPath(config.apiBase.String(), "repos", config.owner, config.name, "releases", "latest")
	if err != nil {
		return updater.Release{}, fmt.Errorf("build GitHub Release URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return updater.Release{}, fmt.Errorf("create release request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", source.userAgent())
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := source.apiClient().Do(request)
	if err != nil {
		return updater.Release{}, fmt.Errorf("request latest GitHub Release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return updater.Release{}, fmt.Errorf("latest GitHub Release returned HTTP %d", response.StatusCode)
	}
	body, err := readBounded(response.Body, maxReleaseResponseBytes)
	if err != nil {
		return updater.Release{}, fmt.Errorf("read latest GitHub Release: %w", err)
	}
	var wire struct {
		TagName    string `json:"tag_name"`
		HTMLURL    string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return updater.Release{}, fmt.Errorf("decode latest GitHub Release: %w", err)
	}
	release := updater.Release{
		Tag:        wire.TagName,
		URL:        wire.HTMLURL,
		Draft:      wire.Draft,
		Prerelease: wire.Prerelease,
		Assets:     make([]updater.Asset, 0, len(wire.Assets)),
	}
	if err := updater.ValidateStableRelease(release); err != nil {
		return updater.Release{}, err
	}
	if err := config.validateReleasePageURL(release.URL, release.Tag); err != nil {
		return updater.Release{}, fmt.Errorf("release page: %w", err)
	}
	seen := make(map[string]struct{}, len(wire.Assets))
	for _, item := range wire.Assets {
		if item.Name != strings.TrimSpace(item.Name) || item.Name == "" || strings.ContainsFunc(item.Name, invalidControl) {
			return updater.Release{}, fmt.Errorf("release contains an invalid asset name")
		}
		if _, exists := seen[item.Name]; exists {
			return updater.Release{}, fmt.Errorf("release contains duplicate asset %q", item.Name)
		}
		seen[item.Name] = struct{}{}
		if item.Size < 0 {
			return updater.Release{}, fmt.Errorf("asset %s declares a negative size", item.Name)
		}
		if err := config.validateAssetURL(item.BrowserDownloadURL, release.Tag, false); err != nil {
			return updater.Release{}, fmt.Errorf("asset %s: %w", item.Name, err)
		}
		release.Assets = append(release.Assets, updater.Asset{
			Name:        item.Name,
			DownloadURL: item.BrowserDownloadURL,
			Size:        item.Size,
		})
	}
	return release, nil
}

// Download implements updater.Source.
func (source Source) Download(ctx context.Context, asset updater.Asset, destination io.Writer) error {
	config, err := source.configuration()
	if err != nil {
		return err
	}
	if destination == nil {
		return fmt.Errorf("asset destination is required")
	}
	if asset.Name == "" || asset.Size < 0 {
		return fmt.Errorf("asset metadata is invalid")
	}
	if err := config.validateAssetURL(asset.DownloadURL, "", false); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create asset request: %w", err)
	}
	request.Header.Set("User-Agent", source.userAgent())
	response, err := source.downloadClient(config).Do(request)
	if err != nil {
		return fmt.Errorf("download asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download returned HTTP %d", response.StatusCode)
	}
	if _, err := io.Copy(destination, response.Body); err != nil {
		return fmt.Errorf("copy asset: %w", err)
	}
	return nil
}

type sourceConfig struct {
	repository   string
	owner        string
	name         string
	apiBase      *url.URL
	publicGitHub bool
	allowedHosts map[string]struct{}
}

func (source Source) configuration() (sourceConfig, error) {
	userAgent := strings.TrimSpace(source.UserAgent)
	if len(userAgent) > 256 || strings.ContainsFunc(userAgent, invalidControl) {
		return sourceConfig{}, fmt.Errorf("GitHub user agent is invalid")
	}
	repository := strings.TrimSpace(source.Repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || !validRepositoryPart(parts[0]) || !validRepositoryPart(parts[1]) {
		return sourceConfig{}, fmt.Errorf("GitHub repository must be owner/name")
	}
	publicGitHub := strings.TrimSpace(source.APIBaseURL) == ""
	apiBaseRaw := strings.TrimRight(strings.TrimSpace(source.APIBaseURL), "/")
	if publicGitHub {
		apiBaseRaw = "https://api.github.com"
	}
	apiBase, err := url.Parse(apiBaseRaw)
	if err != nil || apiBase.Host == "" || apiBase.User != nil || apiBase.RawQuery != "" || apiBase.Fragment != "" {
		return sourceConfig{}, fmt.Errorf("GitHub API base URL is invalid")
	}
	if apiBase.Scheme != "https" && !isLoopbackHTTP(apiBase) {
		return sourceConfig{}, fmt.Errorf("GitHub API base URL must use HTTPS")
	}
	allowedHosts := map[string]struct{}{}
	if publicGitHub {
		allowedHosts["github.com"] = struct{}{}
	} else {
		allowedHosts[strings.ToLower(apiBase.Hostname())] = struct{}{}
	}
	for _, raw := range source.AllowedAssetHosts {
		host, err := normalizeHostname(raw)
		if err != nil {
			return sourceConfig{}, fmt.Errorf("allowed asset host %q is invalid", raw)
		}
		allowedHosts[host] = struct{}{}
	}
	return sourceConfig{
		repository:   repository,
		owner:        parts[0],
		name:         parts[1],
		apiBase:      apiBase,
		publicGitHub: publicGitHub,
		allowedHosts: allowedHosts,
	}, nil
}

func validRepositoryPart(value string) bool {
	return value != "." && value != ".." && repositoryPartPattern.MatchString(value)
}

func normalizeHostname(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || strings.ContainsAny(raw, "/:@?#") {
		return "", fmt.Errorf("invalid host")
	}
	parsed, err := url.Parse("https://" + raw)
	if err != nil || parsed.Hostname() != raw {
		return "", fmt.Errorf("invalid host")
	}
	return raw, nil
}

func (source Source) apiClient() *http.Client {
	client := source.baseClient(defaultAPITimeout)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("refusing GitHub API redirect")
	}
	return client
}

func (source Source) downloadClient(config sourceConfig) *http.Client {
	client := source.baseClient(defaultDownloadTimeout)
	configuredRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many asset redirects")
		}
		if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && request.URL.Scheme != "https" {
			return fmt.Errorf("refusing asset redirect: asset download URL must use HTTPS")
		}
		if err := config.validateAssetURL(request.URL.String(), "", true); err != nil {
			return fmt.Errorf("refusing asset redirect: %w", err)
		}
		if configuredRedirect != nil {
			return configuredRedirect(request, via)
		}
		return nil
	}
	return client
}

func (source Source) baseClient(defaultTimeout time.Duration) *http.Client {
	client := http.Client{Timeout: defaultTimeout}
	if source.HTTPClient != nil {
		client = *source.HTTPClient
		if client.Timeout <= 0 {
			client.Timeout = defaultTimeout
		}
	}
	return &client
}

func (source Source) userAgent() string {
	if value := strings.TrimSpace(source.UserAgent); value != "" {
		return value
	}
	return "awesome-agent-app-features-updater/1"
}

func (config sourceConfig) validateAssetURL(raw, expectedTag string, redirect bool) error {
	trimmed := strings.TrimSpace(raw)
	value, err := url.Parse(trimmed)
	if err != nil || trimmed == "" || value.Host == "" || value.User != nil || value.Fragment != "" {
		return fmt.Errorf("invalid asset download URL")
	}
	if !redirect && value.RawQuery != "" {
		return fmt.Errorf("asset download URL must not contain a query")
	}
	if value.Scheme != "https" && !(isLoopbackHTTP(value) && !config.publicGitHub) {
		return fmt.Errorf("asset download URL must use HTTPS")
	}
	if config.publicGitHub && value.Port() != "" {
		return fmt.Errorf("public GitHub asset URL must use the default HTTPS port")
	}
	host := strings.ToLower(value.Hostname())
	if !config.hostAllowed(host, redirect) {
		return fmt.Errorf("asset download host is not allowed")
	}
	if redirect {
		return nil
	}
	if config.publicGitHub && host != "github.com" {
		return fmt.Errorf("public GitHub asset URL must be hosted by github.com")
	}
	prefix := "/" + config.repository + "/releases/download/"
	if !strings.HasPrefix(value.EscapedPath(), prefix) {
		return fmt.Errorf("asset download URL is outside the configured repository")
	}
	if expectedTag != "" && !strings.HasPrefix(value.EscapedPath(), prefix+url.PathEscape(expectedTag)+"/") {
		return fmt.Errorf("asset download URL is outside release %s", expectedTag)
	}
	return nil
}

func (config sourceConfig) validateReleasePageURL(raw, expectedTag string) error {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return fmt.Errorf("invalid release page URL")
	}
	if value.Scheme != "https" && !(isLoopbackHTTP(value) && !config.publicGitHub) {
		return fmt.Errorf("release page URL must use HTTPS")
	}
	if config.publicGitHub && value.Port() != "" {
		return fmt.Errorf("public GitHub release page must use the default HTTPS port")
	}
	host := strings.ToLower(value.Hostname())
	if config.publicGitHub && host != "github.com" {
		return fmt.Errorf("public GitHub release page must be hosted by github.com")
	}
	if !config.publicGitHub {
		if _, ok := config.allowedHosts[host]; !ok {
			return fmt.Errorf("release page host is not allowed")
		}
	}
	want := "/" + config.repository + "/releases/tag/" + url.PathEscape(expectedTag)
	if value.EscapedPath() != want {
		return fmt.Errorf("release page URL is outside release %s", expectedTag)
	}
	return nil
}

func (config sourceConfig) hostAllowed(host string, redirect bool) bool {
	if _, ok := config.allowedHosts[host]; ok {
		return true
	}
	return redirect && config.publicGitHub && strings.HasSuffix(host, ".githubusercontent.com")
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("response exceeded %d bytes", maximum)
	}
	return data, nil
}

func isLoopbackHTTP(value *url.URL) bool {
	if value.Scheme != "http" {
		return false
	}
	host := strings.ToLower(value.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func invalidControl(character rune) bool {
	return character < 0x20 || character == 0x7f
}
