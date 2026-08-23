package github

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/timmyagentic/awesome-agent-app-features/updater"
)

var _ updater.Source = Source{}

func TestSourceValidatesLatestStableRelease(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/owner/repository/releases/latest" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{
          "tag_name":"v1.2.3",
          "html_url":%q,
          "draft":false,
          "prerelease":false,
          "assets":[{
            "name":"checksums.txt",
            "browser_download_url":%q,
            "size":64
          }]
        }`, server.URL+"/owner/repository/releases/tag/v1.2.3", server.URL+"/owner/repository/releases/download/v1.2.3/checksums.txt")
	}))
	defer server.Close()

	source := Source{
		Repository: " owner/repository ",
		APIBaseURL: server.URL,
		HTTPClient: server.Client(),
	}
	release, err := source.LatestStable(context.Background())
	if err != nil {
		t.Fatalf("LatestStable: %v", err)
	}
	if release.Tag != "v1.2.3" || len(release.Assets) != 1 || release.Assets[0].Name != "checksums.txt" {
		t.Fatalf("release = %+v", release)
	}
}

func TestSourceRejectsAssetOutsideConfiguredRepository(t *testing.T) {
	config, err := (Source{Repository: "owner/repository"}).configuration()
	if err != nil {
		t.Fatalf("configuration: %v", err)
	}
	for _, raw := range []string{
		"http://github.com/owner/repository/releases/download/v1.2.3/file",
		"https://example.com/owner/repository/releases/download/v1.2.3/file",
		"https://github.com/attacker/repository/releases/download/v1.2.3/file",
		"https://github.com/owner/repository/releases/download/v1.2.4/file",
		"https://github.com/owner/repository/releases/download/v1.2.3/file?token=unexpected",
		"https://github.com:444/owner/repository/releases/download/v1.2.3/file",
	} {
		if err := config.validateAssetURL(raw, "v1.2.3", false); err == nil {
			t.Errorf("accepted URL %s", raw)
		}
	}
	if err := config.validateAssetURL(
		"https://github.com/owner/repository/releases/download/v1.2.3/file",
		"v1.2.3",
		false,
	); err != nil {
		t.Fatalf("valid URL: %v", err)
	}
	if err := config.validateAssetURL(
		"https://release-assets.githubusercontent.com/github-production-release-asset/file?signature=redacted",
		"",
		true,
	); err != nil {
		t.Fatalf("valid public GitHub CDN redirect: %v", err)
	}
}

func TestSourceRejectsAPIAndAssetRedirectDowngrades(t *testing.T) {
	var reached atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		_, _ = writer.Write([]byte("unexpected"))
	}))
	defer sink.Close()

	api := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer api.Close()
	apiClient := api.Client()
	apiClient.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	source := Source{
		Repository: "owner/repository",
		APIBaseURL: api.URL,
		HTTPClient: apiClient,
	}
	if _, err := source.LatestStable(context.Background()); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("API redirect error = %v", err)
	}
	if reached.Load() != 0 {
		t.Fatalf("API redirect reached downgrade sink %d time(s)", reached.Load())
	}

	assetServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, sink.URL, http.StatusTemporaryRedirect)
	}))
	defer assetServer.Close()
	assetClient := assetServer.Client()
	assetClient.CheckRedirect = func(*http.Request, []*http.Request) error { return nil }
	assetSource := Source{
		Repository: "owner/repository",
		APIBaseURL: api.URL,
		HTTPClient: assetClient,
	}
	asset := updater.Asset{
		Name:        "archive.tar.gz",
		DownloadURL: assetServer.URL + "/owner/repository/releases/download/v1.2.3/archive.tar.gz",
		Size:        10,
	}
	if err := assetSource.Download(context.Background(), asset, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("asset downgrade error = %v", err)
	}
	if reached.Load() != 0 {
		t.Fatalf("asset redirect reached downgrade sink %d time(s)", reached.Load())
	}
}

func TestSourceBoundsReleaseResponseAndValidatesConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(bytes.Repeat([]byte("x"), maxReleaseResponseBytes+1))
	}))
	defer server.Close()
	source := Source{Repository: "owner/repository", APIBaseURL: server.URL, HTTPClient: server.Client()}
	if _, err := source.LatestStable(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("oversized response error = %v", err)
	}

	for _, invalid := range []Source{
		{Repository: "../repository"},
		{Repository: "owner"},
		{Repository: "owner/repository", APIBaseURL: "http://github.example"},
		{Repository: "owner/repository", AllowedAssetHosts: []string{"host/path"}},
		{Repository: "owner/repository", UserAgent: "invalid\nvalue"},
	} {
		if _, err := invalid.configuration(); err == nil {
			t.Fatalf("accepted invalid source %+v", invalid)
		}
	}
	zeroTimeout := Source{HTTPClient: &http.Client{}}
	if got := zeroTimeout.baseClient(defaultAPITimeout).Timeout; got != defaultAPITimeout {
		t.Fatalf("API timeout = %s", got)
	}
	if got := zeroTimeout.baseClient(defaultDownloadTimeout).Timeout; got != defaultDownloadTimeout {
		t.Fatalf("download timeout = %s", got)
	}
}
