package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errAssetTooLarge = errors.New("asset exceeds configured size limit")

// Check resolves and validates the newest stable release without acquiring the
// install lock or mutating the executable.
func (u *Updater) Check(ctx context.Context) (CheckResult, error) {
	if u == nil {
		return CheckResult{}, fmt.Errorf("updater is nil")
	}
	u.emit(Event{Stage: StageChecking, CurrentVersion: u.config.CurrentVersion})
	release, err := u.config.Source.LatestStable(ctx)
	if err != nil {
		return CheckResult{}, fmt.Errorf("resolve latest stable release: %w", err)
	}
	if err := ValidateStableRelease(release); err != nil {
		return CheckResult{}, err
	}
	available, err := IsNewerStable(release.Tag, u.config.CurrentVersion)
	if err != nil {
		return CheckResult{}, err
	}
	stage := StageUpToDate
	if available {
		stage = StageAvailable
	}
	u.emit(Event{
		Stage:          stage,
		CurrentVersion: u.config.CurrentVersion,
		TargetVersion:  release.Tag,
	})
	return CheckResult{Release: release, Available: available}, nil
}

// Update performs one locked stable update transaction. Both checksum and
// archive must be exact assets on the Release returned by Source.LatestStable.
func (u *Updater) Update(ctx context.Context) (result Result, resultErr error) {
	if u == nil {
		return Result{}, fmt.Errorf("updater is nil")
	}
	releaseLock, err := acquireUpdateLock(u.config.LockPath)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if releaseErr := releaseLock(); releaseErr != nil {
			if resultErr == nil {
				resultErr = releaseErr
			} else {
				resultErr = fmt.Errorf("%w; additionally failed to release update lock: %v", resultErr, releaseErr)
			}
		}
	}()

	check, err := u.Check(ctx)
	if err != nil {
		return Result{}, err
	}
	result.Release = check.Release
	if !check.Available {
		return result, nil
	}
	if err := preflightReplacement(u.config.ExecutablePath); err != nil {
		return result, err
	}

	archiveName := strings.TrimSpace(u.config.AssetName(
		check.Release.Tag,
		u.config.PlatformOS,
		u.config.PlatformArch,
	))
	if archiveName == "" || filepath.Base(archiveName) != archiveName {
		return result, fmt.Errorf("archive asset naming function returned an invalid file name")
	}
	if !strings.HasSuffix(archiveName, ".tar.gz") && !strings.HasSuffix(archiveName, ".tgz") && !strings.HasSuffix(archiveName, ".zip") {
		return result, fmt.Errorf("archive asset %q must be .tar.gz, .tgz, or .zip", archiveName)
	}
	archiveAsset, err := exactAsset(check.Release, archiveName)
	if err != nil {
		return result, err
	}
	checksumsAsset, err := exactAsset(check.Release, u.config.ChecksumsAsset)
	if err != nil {
		return result, err
	}
	result.ArchiveAsset = archiveName

	u.emit(Event{
		Stage:         StageDownloadingChecks,
		TargetVersion: check.Release.Tag,
		Asset:         checksumsAsset.Name,
	})
	manifest, err := u.downloadBytes(ctx, checksumsAsset, 1024*1024)
	if err != nil {
		return result, fmt.Errorf("download checksum manifest: %w", err)
	}
	expectedChecksum, err := parseChecksumManifest(string(manifest), archiveName)
	if err != nil {
		return result, err
	}

	u.emit(Event{
		Stage:         StageDownloadingArchive,
		TargetVersion: check.Release.Tag,
		Asset:         archiveName,
	})
	archivePath, archiveBytes, err := u.downloadFile(ctx, archiveAsset, u.config.MaxArchiveSize)
	if err != nil {
		return result, fmt.Errorf("download release archive: %w", err)
	}
	defer os.Remove(archivePath)
	if err := verifyFileChecksum(archivePath, expectedChecksum); err != nil {
		return result, err
	}
	u.emit(Event{
		Stage:         StageChecksumVerified,
		TargetVersion: check.Release.Tag,
		Asset:         archiveName,
		Bytes:         archiveBytes,
	})

	targetDirectory := filepath.Dir(u.config.ExecutablePath)
	stagedPath, err := extractBinary(
		archivePath,
		archiveName,
		u.config.BinaryName,
		targetDirectory,
		u.config.MaxBinarySize,
	)
	if err != nil {
		return result, fmt.Errorf("extract release binary: %w", err)
	}
	defer os.Remove(stagedPath)
	if err := prepareStagedExecutable(u.config.ExecutablePath, stagedPath); err != nil {
		return result, fmt.Errorf("prepare staged binary: %w", err)
	}
	if err := u.config.Verifier.Verify(ctx, stagedPath, check.Release.Tag); err != nil {
		return result, fmt.Errorf("verify staged binary: %w", err)
	}
	u.emit(Event{
		Stage:         StageStagedVerified,
		TargetVersion: check.Release.Tag,
		Asset:         archiveName,
	})

	u.emit(Event{
		Stage:         StageInstalling,
		TargetVersion: check.Release.Tag,
		Asset:         archiveName,
	})
	backupPath, err := replaceExecutable(u.config.ExecutablePath, stagedPath, func(path string) error {
		return u.config.Verifier.Verify(ctx, path, check.Release.Tag)
	})
	if err != nil {
		return result, fmt.Errorf("install release binary: %w", err)
	}
	result.BackupRetainedAt = backupPath
	u.emit(Event{
		Stage:         StageInstalledVerified,
		TargetVersion: check.Release.Tag,
		Asset:         archiveName,
		Detail:        backupPath,
	})
	result.Updated = true
	u.emit(Event{
		Stage:         StageComplete,
		TargetVersion: check.Release.Tag,
		Asset:         archiveName,
	})
	return result, nil
}

func exactAsset(release Release, name string) (Asset, error) {
	var match Asset
	found := false
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		if found {
			return Asset{}, fmt.Errorf("release %s contains duplicate asset %s", release.Tag, name)
		}
		match = asset
		found = true
	}
	if !found {
		return Asset{}, fmt.Errorf("release %s does not contain exact asset %s", release.Tag, name)
	}
	return match, nil
}

func (u *Updater) downloadBytes(ctx context.Context, asset Asset, maximum int64) ([]byte, error) {
	if asset.Size > maximum {
		return nil, fmt.Errorf("%w: %s declares %d bytes (maximum %d)", errAssetTooLarge, asset.Name, asset.Size, maximum)
	}
	var buffer bytes.Buffer
	writer := &boundedWriter{destination: &buffer, remaining: maximum}
	if err := u.config.Source.Download(ctx, asset, writer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (u *Updater) downloadFile(ctx context.Context, asset Asset, maximum int64) (string, int64, error) {
	if asset.Size > maximum {
		return "", 0, fmt.Errorf("%w: %s declares %d bytes (maximum %d)", errAssetTooLarge, asset.Name, asset.Size, maximum)
	}
	directory := filepath.Dir(u.config.ExecutablePath)
	file, err := os.CreateTemp(directory, "."+filepath.Base(u.config.ExecutablePath)+".download-*")
	if err != nil {
		return "", 0, err
	}
	path := file.Name()
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	writer := &boundedWriter{destination: file, remaining: maximum}
	if err := u.config.Source.Download(ctx, asset, writer); err != nil {
		return "", 0, err
	}
	if err := file.Sync(); err != nil {
		return "", 0, fmt.Errorf("sync downloaded archive: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", 0, fmt.Errorf("close downloaded archive: %w", err)
	}
	remove = false
	return path, writer.written, nil
}

type boundedWriter struct {
	destination io.Writer
	remaining   int64
	written     int64
}

func (writer *boundedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		if writer.remaining > 0 {
			written, err := writer.destination.Write(data[:writer.remaining])
			writer.written += int64(written)
			writer.remaining -= int64(written)
			if err != nil {
				return written, err
			}
			return written, errAssetTooLarge
		}
		return 0, errAssetTooLarge
	}
	written, err := writer.destination.Write(data)
	writer.written += int64(written)
	writer.remaining -= int64(written)
	return written, err
}

func parseChecksumManifest(manifest, assetName string) (string, error) {
	var checksum string
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		if checksum != "" {
			return "", fmt.Errorf("checksum manifest contains more than one entry for %s", assetName)
		}
		candidate := strings.ToLower(fields[0])
		decoded, err := hex.DecodeString(candidate)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("checksum manifest contains an invalid SHA-256 for %s", assetName)
		}
		checksum = candidate
	}
	if checksum == "" {
		return "", fmt.Errorf("checksum manifest does not contain %s", assetName)
	}
	return checksum, nil
}

func verifyFileChecksum(path, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("expected checksum is not a SHA-256")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open downloaded archive: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash downloaded archive: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch for release archive; refusing to install")
	}
	return nil
}
