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
	archiveName := ReleaseArchiveName("example-agent")("v1.2.3", runtime.GOOS, runtime.GOARCH)
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
			if tag != "v1.2.3" || goos != runtime.GOOS || goarch != runtime.GOARCH {
				t.Fatalf("unexpected asset inputs: %s %s %s", tag, goos, goarch)
			}
			return archiveName
		},
		Source:   source,
		Verifier: verifier,
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

	result, err := harness.updater.UpdateLatest(context.Background())
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
		StageDownloadingChecksums,
		StageAvailable,
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
	_, err := harness.updater.UpdateLatest(context.Background())
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
	_, err := harness.updater.UpdateLatest(context.Background())
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
	_, err := harness.updater.UpdateLatest(context.Background())
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

func TestUpdateRejectsVersionProbeThatModifiesStagedBinary(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(_ context.Context, path, _ string) error {
		return os.WriteFile(path, []byte("probe-mutated binary"), 0o755)
	}))
	_, err := harness.updater.UpdateLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "staged version probe modified") {
		t.Fatalf("Update error = %v", err)
	}
	assertContent(t, harness.target, "old binary")
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
	_, err := harness.updater.UpdateLatest(context.Background())
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

func TestUpdateRollsBackVersionProbeThatModifiesInstalledBinary(t *testing.T) {
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(_ context.Context, path, _ string) error {
		verifierCalls++
		if verifierCalls == 2 {
			return os.WriteFile(path, []byte("probe-mutated binary"), 0o755)
		}
		return nil
	}))
	_, err := harness.updater.UpdateLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "installed version probe modified") || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("Update error = %v", err)
	}
	assertContent(t, harness.target, "old binary")
}

func TestUpdateReturnsStructuredBackupWhenRollbackCannotFinish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission fixture is Unix-only")
	}
	var verifierCalls int
	harness := newUpdateHarness(t, VersionVerifierFunc(func(_ context.Context, path, _ string) error {
		verifierCalls++
		if verifierCalls == 2 {
			if err := os.Chmod(filepath.Dir(path), 0o500); err != nil {
				return err
			}
			return fmt.Errorf("simulated installed version mismatch")
		}
		return nil
	}))
	directory := filepath.Dir(harness.target)
	defer os.Chmod(directory, 0o700)
	result, err := harness.updater.UpdateLatest(context.Background())
	if restoreErr := os.Chmod(directory, 0o700); restoreErr != nil {
		t.Fatalf("restore fixture permissions: %v", restoreErr)
	}
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("Update error = %v", err)
	}
	wantBackup := harness.target + ".update-backup"
	if result.BackupRetainedAt != wantBackup {
		t.Fatalf("BackupRetainedAt = %q, want %q", result.BackupRetainedAt, wantBackup)
	}
	assertContent(t, wantBackup, "old binary")
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
	_, err := harness.updater.UpdateLatest(context.Background())
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
	plan, err := harness.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	harness.source.beforeDownload = func(asset Asset) {
		if asset.Name != harness.archiveName {
			return
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := harness.updater.Apply(context.Background(), plan)
		firstDone <- err
	}()
	<-started
	_, err = harness.updater.Apply(context.Background(), plan)
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
	held, err := tryPlatformLock(harness.updater.lockPath)
	if err != nil {
		t.Fatalf("tryPlatformLock: %v", err)
	}
	defer held.release()
	_, err = harness.updater.UpdateLatest(context.Background())
	if !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("file-lock error = %v", err)
	}
}

func TestCheckReportsUpToDateWithoutDownload(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.updater.config.CurrentVersion = "v1.2.3"
	result, err := harness.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if result.Available() {
		t.Fatalf("plan = %+v", result.Release())
	}
	if len(harness.source.downloads) != 0 {
		t.Fatalf("Prepare downloaded assets for up-to-date release: %v", harness.source.downloads)
	}
}

func TestUpdateRequiresExactAssetsFromSelectedRelease(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.source.release.Assets[0].Name = harness.archiveName + ".other"
	_, err := harness.updater.UpdateLatest(context.Background())
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
	_, err := harness.updater.UpdateLatest(context.Background())
	if !errors.Is(err, errAssetTooLarge) {
		t.Fatalf("declared-size error = %v", err)
	}

	harness = newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.updater.config.MaxArchiveSize = int64(len(harness.archive) - 1)
	harness.source.release.Assets[0].Size = 0
	_, err = harness.updater.UpdateLatest(context.Background())
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
		{Tag: "v01.2.3"},
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
		{"v1.2.3", "v1.2.3+build.7", false},
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
	if _, err := IsNewerStable("v1.2.3", "v1.2.2-01"); err == nil {
		t.Fatal("accepted invalid prerelease numeric identifier")
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

	symlinkPath := filepath.Join(directory, "symlink.zip")
	writeZipSymlink(t, symlinkPath, "example-agent", "bin/example-agent")
	if _, err := extractBinary(symlinkPath, "symlink.zip", "example-agent", directory, 1024); err == nil {
		t.Fatal("accepted symlink-mode zip entry")
	}

	duplicatePath := filepath.Join(directory, "duplicate.tar.gz")
	if err := os.WriteFile(duplicatePath, tarGzArchiveWithNames(t, []string{"bin/example-agent", "example-agent"}, []byte("duplicate")), 0o600); err != nil {
		t.Fatalf("write duplicate archive: %v", err)
	}
	if _, err := extractBinary(duplicatePath, "duplicate.tar.gz", "example-agent", directory, 1024); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("duplicate archive error = %v", err)
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
	noisy := filepath.Join(directory, "noisy-agent")
	noisyScript := "#!/bin/sh\nprintf '" + strings.Repeat("x", 70*1024) + "'\n"
	if err := os.WriteFile(noisy, []byte(noisyScript), 0o755); err != nil {
		t.Fatalf("write noisy version fixture: %v", err)
	}
	if err := verifier.Verify(context.Background(), noisy, "v1.2.3"); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("oversized version output error = %v", err)
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
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	service.platformOS = "darwin"
	service.platformArch = "arm64"
	result, err := service.UpdateLatest(context.Background())
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

func TestUpdateSupportsReleaseSpecificArchiveBinaryName(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "example-agent")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	archiveName := "example-agent-v1.2.3-darwin-arm64.tar.gz"
	archivedBinaryName := "example-agent-v1.2.3-darwin-arm64"
	archive := tarGzArchive(t, archivedBinaryName, []byte("new binary"))
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
		ArchiveBinaryName: func(tag, goos, goarch string) string {
			return fmt.Sprintf("example-agent-%s-%s-%s", tag, goos, goarch)
		},
		AssetName: func(string, string, string) string { return archiveName },
		Source:    source,
		Verifier:  VersionVerifierFunc(func(context.Context, string, string) error { return nil }),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	service.platformOS = "darwin"
	service.platformArch = "arm64"
	if _, err := service.UpdateLatest(context.Background()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	assertContent(t, target, "new binary")
}

func TestUpdateRejectsInvalidReleaseSpecificBinaryNameBeforeDownload(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error {
		return nil
	}))
	harness.updater.config.BinaryName = ""
	harness.updater.config.ArchiveBinaryName = func(string, string, string) string {
		return "../example-agent"
	}
	if _, err := harness.updater.UpdateLatest(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid file name") {
		t.Fatalf("Update error = %v", err)
	}
	if len(harness.source.downloads) != 0 {
		t.Fatalf("downloaded assets for invalid binary name: %v", harness.source.downloads)
	}
	assertContent(t, harness.target, "old binary")
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
	invalid.ArchiveBinaryName = func(string, string, string) string { return "example" }
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted both static and dynamic archive binary names")
	}
	invalid = base
	invalid.ChecksumsAsset = `..\checksums.txt`
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted Windows-style path-shaped checksums asset")
	}
	invalid = base
	invalid.ChecksumsAsset = "checksums final.txt"
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted whitespace in checksums asset name")
	}
	invalid = base
	invalid.CurrentVersion = ""
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted empty current version")
	}
	invalid = base
	invalid.CurrentVersion = "not-a-version"
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted invalid current version")
	}
	invalid = base
	invalid.CurrentVersion = "developer-build"
	if _, err := New(invalid); err == nil {
		t.Fatal("accepted an ambiguous development version")
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
	return tarGzArchiveWithNames(t, []string{name}, content)
}

func tarGzArchiveWithNames(t *testing.T, names []string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range names {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
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

func writeZipSymlink(t *testing.T, destination, name, target string) {
	t.Helper()
	file, err := os.Create(destination)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	header := &zip.FileHeader{Name: name}
	header.SetMode(os.ModeSymlink | 0o777)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatalf("create zip symlink entry: %v", err)
	}
	if _, err := entry.Write([]byte(target)); err != nil {
		t.Fatalf("write zip symlink entry: %v", err)
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
	if _, err := harness.updater.UpdateLatest(ctx); err == nil {
		t.Fatal("cancelled update succeeded")
	}
	harness.source.latest = nil
	if _, err := harness.updater.UpdateLatest(context.Background()); err != nil {
		t.Fatalf("lock was not released after cancellation: %v", err)
	}
}
