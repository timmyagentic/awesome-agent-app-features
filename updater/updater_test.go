package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	release        Release
	data           map[string][]byte
	latest         func(context.Context) (Release, error)
	beforeDownload func(Asset)
	downloads      []string
	mutex          sync.Mutex
}

func (source *fakeSource) LatestStable(ctx context.Context) (Release, error) {
	if source.latest != nil {
		return source.latest(ctx)
	}
	return source.release, nil
}

func (source *fakeSource) Download(_ context.Context, asset Asset, destination io.Writer) error {
	if source.beforeDownload != nil {
		source.beforeDownload(asset)
	}
	source.mutex.Lock()
	source.downloads = append(source.downloads, asset.Name)
	source.mutex.Unlock()
	data, ok := source.data[asset.Name]
	if !ok {
		return fmt.Errorf("missing fake asset %s", asset.Name)
	}
	_, err := destination.Write(data)
	return err
}

type updateHarness struct {
	updater     *Updater
	source      *fakeSource
	target      string
	archiveName string
	archive     []byte
}

func newUpdateHarness(t *testing.T, verifier VersionVerifier) updateHarness {
	t.Helper()
	directory := t.TempDir()
	target := filepath.Join(directory, "example-agent")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	archiveName := "example-agent-v1.2.3-darwin-arm64.tar.gz"
	archive := tarGzArchive(t, "example-agent", []byte("new binary"))
	digest := sha256.Sum256(archive)
	manifest := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(digest[:]), archiveName))
	source := &fakeSource{
		data: map[string][]byte{
			archiveName:     archive,
			"checksums.txt": manifest,
		},
	}
	source.release = Release{
		Tag: "v1.2.3",
		Assets: []Asset{
			{Name: archiveName, DownloadURL: "https://example.invalid/archive", Size: int64(len(archive))},
			{Name: "checksums.txt", DownloadURL: "https://example.invalid/checksums", Size: int64(len(manifest))},
		},
	}
	instance, err := New(Config{
		Product:        "Example Agent",
		CurrentVersion: "v1.0.0",
		ExecutablePath: target,
		BinaryName:     "example-agent",
		AssetName: func(tag, goos, goarch string) string {
			if tag != "v1.2.3" || goos != "darwin" || goarch != "arm64" {
				t.Fatalf("unexpected asset inputs: %s %s %s", tag, goos, goarch)
			}
			return archiveName
		},
		Source:       source,
		Verifier:     verifier,
		PlatformOS:   "darwin",
		PlatformArch: "arm64",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return updateHarness{
		updater:     instance,
		source:      source,
		target:      target,
		archiveName: archiveName,
		archive:     archive,
	}
}

func TestUpdateVerifiesChecksumStagedAndInstalledBinary(t *testing.T) {
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(_ context.Context, path, tag string) error {
		verifierCalls++
		if tag != "v1.2.3" {
			return fmt.Errorf("tag = %s", tag)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "new binary" {
			return fmt.Errorf("content = %q", data)
		}
		return nil
	}))
	var stages []Stage
	harness.updater.config.Progress = func(event Event) {
		stages = append(stages, event.Stage)
	}

	result, err := harness.updater.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !result.Updated || result.Release.Tag != "v1.2.3" || result.ArchiveAsset != harness.archiveName {
		t.Fatalf("result = %+v", result)
	}
	if verifierCalls != 2 {
		t.Fatalf("verifier calls = %d, want staged + installed", verifierCalls)
	}
	assertContent(t, harness.target, "new binary")
	if _, err := os.Stat(harness.target + ".update-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected backup after success: %v", err)
	}
	wantStages := []Stage{
		StageChecking,
		StageAvailable,
		StageDownloadingChecks,
		StageDownloadingArchive,
		StageChecksumVerified,
		StageStagedVerified,
		StageInstalling,
		StageInstalledVerified,
		StageComplete,
	}
	if fmt.Sprint(stages) != fmt.Sprint(wantStages) {
		t.Fatalf("stages = %v, want %v", stages, wantStages)
	}
	if fmt.Sprint(harness.source.downloads) != fmt.Sprint([]string{"checksums.txt", harness.archiveName}) {
		t.Fatalf("downloads = %v", harness.source.downloads)
	}
}

func TestUpdateRefusesPrereleaseBeforeDownload(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.source.release.Tag = "v1.2.3-beta.1"
	harness.source.release.Prerelease = true
	_, err := harness.updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "non-stable") {
		t.Fatalf("Update error = %v", err)
	}
	if len(harness.source.downloads) != 0 {
		t.Fatalf("downloaded assets for prerelease: %v", harness.source.downloads)
	}
	assertContent(t, harness.target, "old binary")
}

func TestUpdateRefusesChecksumMismatchBeforeVersionOrMutation(t *testing.T) {
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		verifierCalls++
		return nil
	}))
	harness.source.data["checksums.txt"] = []byte(strings.Repeat("0", 64) + "  " + harness.archiveName + "\n")
	_, err := harness.updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Update error = %v", err)
	}
	if verifierCalls != 0 {
		t.Fatalf("verifier called %d times before checksum refusal", verifierCalls)
	}
	assertContent(t, harness.target, "old binary")
}

func TestUpdateRefusesStagedVersionBeforeMutation(t *testing.T) {
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		verifierCalls++
		return fmt.Errorf("simulated staged version mismatch")
	}))
	_, err := harness.updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "verify staged binary") {
		t.Fatalf("Update error = %v", err)
	}
	if verifierCalls != 1 {
		t.Fatalf("verifier calls = %d", verifierCalls)
	}
	assertContent(t, harness.target, "old binary")
	if _, err := os.Stat(harness.target + ".update-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup exists before replacement: %v", err)
	}
}

func TestUpdateRollsBackInstalledVersionMismatch(t *testing.T) {
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		verifierCalls++
		if verifierCalls == 2 {
			return fmt.Errorf("simulated installed version mismatch")
		}
		return nil
	}))
	_, err := harness.updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("Update error = %v", err)
	}
	if verifierCalls != 2 {
		t.Fatalf("verifier calls = %d", verifierCalls)
	}
	assertContent(t, harness.target, "old binary")
	if _, err := os.Stat(harness.target + ".update-backup"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup remains after rollback: %v", err)
	}
}

func TestUpdateRefusesExistingRecoveryBackup(t *testing.T) {
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		verifierCalls++
		return nil
	}))
	backup := harness.target + ".update-backup"
	if err := os.WriteFile(backup, []byte("important prior backup"), 0o755); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	_, err := harness.updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite existing update backup") {
		t.Fatalf("Update error = %v", err)
	}
	if verifierCalls != 0 {
		t.Fatalf("verifier calls = %d before backup preflight", verifierCalls)
	}
	if len(harness.source.downloads) != 0 {
		t.Fatalf("downloads occurred before backup preflight: %v", harness.source.downloads)
	}
	assertContent(t, harness.target, "old binary")
	assertContent(t, backup, "important prior backup")
}

func TestUpdateReturnsDeterministicBusyForConcurrentCaller(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.source.latest = func(context.Context) (Release, error) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return harness.source.release, nil
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := harness.updater.Update(context.Background())
		firstDone <- err
	}()
	<-started
	_, err := harness.updater.Update(context.Background())
	if !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("concurrent error = %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first update: %v", err)
	}
}

func TestUpdateHonorsPlatformFileLock(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	held, err := tryPlatformLock(harness.updater.config.LockPath)
	if err != nil {
		t.Fatalf("tryPlatformLock: %v", err)
	}
	defer held.release()
	_, err = harness.updater.Update(context.Background())
	if !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("file-lock error = %v", err)
	}
}

func TestCheckReportsUpToDateWithoutDownload(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.updater.config.CurrentVersion = "v1.2.3"
	result, err := harness.updater.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Available {
		t.Fatalf("result = %+v", result)
	}
	if len(harness.source.downloads) != 0 {
		t.Fatalf("Check downloaded assets: %v", harness.source.downloads)
	}
}

func TestUpdateRequiresExactAssetsFromSelectedRelease(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.source.release.Assets[0].Name = harness.archiveName + ".other"
	_, err := harness.updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not contain exact asset") {
		t.Fatalf("Update error = %v", err)
	}
	if len(harness.source.downloads) != 0 {
		t.Fatalf("downloaded non-selected assets: %v", harness.source.downloads)
	}
}

func TestUpdateEnforcesDeclaredAndStreamingAssetLimits(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.updater.config.MaxArchiveSize = int64(len(harness.archive) - 1)
	_, err := harness.updater.Update(context.Background())
	if !errors.Is(err, errAssetTooLarge) {
		t.Fatalf("declared-size error = %v", err)
	}

	harness = newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.updater.config.MaxArchiveSize = int64(len(harness.archive) - 1)
	harness.source.release.Assets[0].Size = 0
	_, err = harness.updater.Update(context.Background())
	if !errors.Is(err, errAssetTooLarge) {
		t.Fatalf("stream-size error = %v", err)
	}
}

func TestValidateStableReleaseAndVersionComparison(t *testing.T) {
	for _, release := range []Release{
		{Tag: "v1.2.3-beta.1", Prerelease: true},
		{Tag: "v1.2.3-beta.1"},
		{Tag: "v1.2.3", Draft: true},
		{Tag: " v1.2.3"},
		{Tag: "v1.2"},
	} {
		if err := ValidateStableRelease(release); err == nil {
			t.Fatalf("accepted release %+v", release)
		}
	}
	if err := ValidateStableRelease(Release{Tag: "v1.2.3"}); err != nil {
		t.Fatalf("stable release: %v", err)
	}
	for _, item := range []struct {
		candidate string
		current   string
		want      bool
	}{
		{"v1.2.3", "v1.2.2", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.3", "v1.2.4", false},
		{"v1.2.3", "v1.2.3-beta.2", true},
		{"v1.2.3", "dev", true},
	} {
		got, err := IsNewerStable(item.candidate, item.current)
		if err != nil || got != item.want {
			t.Errorf("IsNewerStable(%q, %q) = %v, %v; want %v", item.candidate, item.current, got, err, item.want)
		}
	}
	if _, err := IsNewerStable("v1.2.3", "unknown"); err == nil {
		t.Fatal("accepted unknown current version")
	}
}

func TestChecksumManifestRequiresOneExactSHA256Entry(t *testing.T) {
	asset := "example.tar.gz"
	valid := strings.Repeat("a", 64)
	checksum, err := parseChecksumManifest(valid+"  "+asset+"\n", asset)
	if err != nil || checksum != valid {
		t.Fatalf("valid manifest = %q, %v", checksum, err)
	}
	for _, manifest := range []string{
		valid + "  other.tar.gz\n",
		"bad  " + asset + "\n",
		valid + "  " + asset + "\n" + valid + "  " + asset + "\n",
	} {
		if _, err := parseChecksumManifest(manifest, asset); err == nil {
			t.Fatalf("accepted manifest %q", manifest)
		}
	}
}

func TestArchiveExtractionSupportsZipAndRejectsTraversalOnlyEntry(t *testing.T) {
	directory := t.TempDir()
	zipPath := filepath.Join(directory, "release.zip")
	writeZipArchive(t, zipPath, "bin/example-agent", []byte("zip binary"))
	staged, err := extractBinary(zipPath, "release.zip", "example-agent", directory, 1024)
	if err != nil {
		t.Fatalf("extract zip: %v", err)
	}
	defer os.Remove(staged)
	assertContent(t, staged, "zip binary")

	unsafePath := filepath.Join(directory, "unsafe.zip")
	writeZipArchive(t, unsafePath, "../../example-agent", []byte("unsafe"))
	if _, err := extractBinary(unsafePath, "unsafe.zip", "example-agent", directory, 1024); err == nil {
		t.Fatal("accepted traversal-only archive entry")
	}
}

func TestGitHubSourceValidatesLatestRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/owner/repository/releases/latest" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
          "tag_name":"v1.2.3",
          "html_url":"https://github.com/owner/repository/releases/tag/v1.2.3",
          "draft":false,
          "prerelease":false,
          "assets":[{
            "name":"checksums.txt",
            "browser_download_url":"https://github.com/owner/repository/releases/download/v1.2.3/checksums.txt",
            "size":64
          }]
        }`))
	}))
	defer server.Close()
	source := GitHubSource{
		Repository: " owner/repository ",
		APIBase:    server.URL,
		Client:     server.Client(),
	}
	release, err := source.LatestStable(context.Background())
	if err != nil {
		t.Fatalf("LatestStable: %v", err)
	}
	if release.Tag != "v1.2.3" || len(release.Assets) != 1 || release.Assets[0].Name != "checksums.txt" {
		t.Fatalf("release = %+v", release)
	}
}

func TestGitHubSourceRejectsAssetOutsideConfiguredRepository(t *testing.T) {
	source := GitHubSource{Repository: "owner/repository"}
	for _, raw := range []string{
		"http://github.com/owner/repository/releases/download/v1.2.3/file",
		"https://example.com/owner/repository/releases/download/v1.2.3/file",
		"https://github.com/attacker/repository/releases/download/v1.2.3/file",
	} {
		if err := source.validateDownloadURL(raw, "v1.2.3"); err == nil {
			t.Errorf("accepted URL %s", raw)
		}
	}
	if err := source.validateDownloadURL(
		"https://github.com/owner/repository/releases/download/v1.2.3/file",
		"v1.2.3",
	); err != nil {
		t.Fatalf("valid URL: %v", err)
	}
}

func TestCommandVersionVerifierRequiresExactFirstLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "example-agent")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'Example Agent v1.2.3\\ncommit: abc\\n'\n"), 0o755); err != nil {
		t.Fatalf("write version fixture: %v", err)
	}
	verifier := ExactVersionLine("Example Agent")
	if err := verifier.Verify(context.Background(), binary, "v1.2.3"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if err := verifier.Verify(context.Background(), binary, "v1.2.4"); err == nil {
		t.Fatal("accepted mismatched version output")
	}
}

func TestUpdateRunsStrictVersionProbeOnStagedAndInstalledExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "example-agent")
	oldScript := []byte("#!/bin/sh\nprintf 'example-agent v1.0.0\\n'\n")
	newScript := []byte("#!/bin/sh\nprintf 'example-agent v1.2.3\\n'\n")
	if err := os.WriteFile(target, oldScript, 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	archiveName := "example-agent-v1.2.3-darwin-arm64.tar.gz"
	archive := tarGzArchive(t, "example-agent", newScript)
	digest := sha256.Sum256(archive)
	manifest := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(digest[:]), archiveName))
	source := &fakeSource{
		release: Release{
			Tag: "v1.2.3",
			Assets: []Asset{
				{Name: archiveName, Size: int64(len(archive))},
				{Name: "checksums.txt", Size: int64(len(manifest))},
			},
		},
		data: map[string][]byte{
			archiveName:     archive,
			"checksums.txt": manifest,
		},
	}
	service, err := New(Config{
		Product:        "example-agent",
		CurrentVersion: "v1.0.0",
		ExecutablePath: target,
		BinaryName:     "example-agent",
		AssetName:      func(string, string, string) string { return archiveName },
		Source:         source,
		Verifier:       ExactVersionLine("example-agent"),
		PlatformOS:     "darwin",
		PlatformArch:   "arm64",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result, err := service.Update(context.Background())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !result.Updated {
		t.Fatalf("result = %+v", result)
	}
	if err := ExactVersionLine("example-agent").Verify(context.Background(), target, "v1.2.3"); err != nil {
		t.Fatalf("verify installed fixture: %v", err)
	}
}

func TestReleaseArchiveName(t *testing.T) {
	namer := ReleaseArchiveName("example-agent")
	if got := namer("v1.2.3", "darwin", "arm64"); got != "example-agent-v1.2.3-darwin-arm64.tar.gz" {
		t.Fatalf("darwin name = %q", got)
	}
	if got := namer("v1.2.3", "windows", "amd64"); got != "example-agent-v1.2.3-windows-amd64.zip" {
		t.Fatalf("windows name = %q", got)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	source := &fakeSource{}
	verifier := VersionVerifierFunc(func(context.Context, string, string) error { return nil })
	executable := filepath.Join(t.TempDir(), "example")
	if err := os.WriteFile(executable, []byte("example"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	base := Config{
		Product:        "Example",
		CurrentVersion: "v1.0.0",
		ExecutablePath: executable,
		BinaryName:     "example",
		AssetName:      ReleaseArchiveName("example"),
		Source:         source,
		Verifier:       verifier,
	}
	if _, err := New(base); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	invalid := base
	invalid.BinaryName = "../example"
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted path-shaped binary name")
	}
	invalid = base
	invalid.CurrentVersion = ""
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted empty current version")
	}
	if runtime.GOOS != "windows" {
		symlink := filepath.Join(t.TempDir(), "example-link")
		if err := os.Symlink(executable, symlink); err != nil {
			t.Fatalf("create executable symlink: %v", err)
		}
		invalid = base
		invalid.ExecutablePath = symlink
		if _, err := New(invalid); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink executable error = %v", err)
		}
	}
}

func tarGzArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func writeZipArchive(t *testing.T, destination, name string, content []byte) {
	t.Helper()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("content of %s = %q, want %q", path, data, want)
	}
}

func TestContextCancellationReleasesLock(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.source.latest = func(ctx context.Context) (Release, error) {
		<-ctx.Done()
		return Release{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := harness.updater.Update(ctx); err == nil {
		t.Fatal("cancelled update succeeded")
	}
	harness.source.latest = nil
	if _, err := harness.updater.Update(context.Background()); err != nil {
		t.Fatalf("lock was not released after cancellation: %v", err)
	}
}
