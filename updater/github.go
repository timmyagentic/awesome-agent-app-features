package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// GitHubSource reads a public repository's `/releases/latest` API and
// downloads assets listed on that exact response.
type GitHubSource struct {
	Repository string
	Client     *http.Client
	APIBase    string
	UserAgent  string
}

// LatestStable implements Source.
func (source GitHubSource) LatestStable(ctx context.Context) (Release, error) {
	repository := strings.TrimSpace(source.Repository)
	if !githubRepositoryPattern.MatchString(repository) {
		return Release{}, fmt.Errorf("GitHub repository must be owner/name")
	}
	source.Repository = repository
	apiBase := strings.TrimRight(strings.TrimSpace(source.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	baseURL, err := url.Parse(apiBase)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "https" && !isLoopbackHTTP(baseURL)) {
		return Release{}, fmt.Errorf("GitHub API base must use HTTPS")
	}
	requestURL := apiBase + "/repos/" + repository + "/releases/latest"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("create release request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", source.userAgent())
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := source.client().Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("request latest GitHub Release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("latest GitHub Release returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024))
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
	if err := decoder.Decode(&wire); err != nil {
		return Release{}, fmt.Errorf("decode latest GitHub Release: %w", err)
	}
	release := Release{
		Tag:        wire.TagName,
		URL:        wire.HTMLURL,
		Draft:      wire.Draft,
		Prerelease: wire.Prerelease,
		Assets:     make([]Asset, 0, len(wire.Assets)),
	}
	seen := make(map[string]struct{})
	for _, item := range wire.Assets {
		if strings.TrimSpace(item.Name) == "" {
			return Release{}, fmt.Errorf("release contains an unnamed asset")
		}
		if _, exists := seen[item.Name]; exists {
			return Release{}, fmt.Errorf("release contains duplicate asset %q", item.Name)
		}
		seen[item.Name] = struct{}{}
		if err := source.validateDownloadURL(item.BrowserDownloadURL, release.Tag); err != nil {
			return Release{}, fmt.Errorf("asset %s: %w", item.Name, err)
		}
		release.Assets = append(release.Assets, Asset{
			Name:        item.Name,
			DownloadURL: item.BrowserDownloadURL,
			Size:        item.Size,
		})
	}
	if err := ValidateStableRelease(release); err != nil {
		return Release{}, err
	}
	return release, nil
}

// Download implements Source.
func (source GitHubSource) Download(ctx context.Context, asset Asset, destination io.Writer) error {
	if err := source.validateDownloadURL(asset.DownloadURL, ""); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create asset request: %w", err)
	}
	request.Header.Set("User-Agent", source.userAgent())
	response, err := source.client().Do(request)
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

func (source GitHubSource) client() *http.Client {
	if source.Client != nil {
		return source.Client
	}
	return &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if request.URL.Scheme != "https" {
				return fmt.Errorf("refusing asset redirect to non-HTTPS URL")
			}
			return nil
		},
	}
}

func (source GitHubSource) userAgent() string {
	if value := strings.TrimSpace(source.UserAgent); value != "" {
		return value
	}
	return "awesome-agent-app-features-updater/1"
}

func (source GitHubSource) validateDownloadURL(raw, expectedTag string) error {
	downloadURL, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || downloadURL.Host == "" || downloadURL.User != nil || downloadURL.RawQuery != "" || downloadURL.Fragment != "" {
		return fmt.Errorf("invalid asset download URL")
	}
	if downloadURL.Scheme != "https" {
		return fmt.Errorf("asset download URL must use HTTPS")
	}
	if strings.TrimSpace(source.APIBase) != "" {
		return nil
	}
	if !strings.EqualFold(downloadURL.Hostname(), "github.com") {
		return fmt.Errorf("asset download URL must be hosted by github.com")
	}
	prefix := "/" + strings.Trim(source.Repository, "/") + "/releases/download/"
	if !strings.HasPrefix(downloadURL.EscapedPath(), prefix) {
		return fmt.Errorf("asset download URL is outside the configured repository")
	}
	if expectedTag != "" && !strings.HasPrefix(downloadURL.EscapedPath(), prefix+url.PathEscape(expectedTag)+"/") {
		return fmt.Errorf("asset download URL is outside release %s", expectedTag)
	}
	return nil
}

func isLoopbackHTTP(value *url.URL) bool {
	host := strings.ToLower(value.Hostname())
	return value.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
}
