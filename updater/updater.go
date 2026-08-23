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

const maxChecksumManifestSize = 1024 * 1024

// Prepare resolves the newest stable release, selects its exact assets, and
// pins the archive checksum without mutating the executable. A later successful
// Prepare supersedes previously returned plans from this Updater.
func (u *Updater) Prepare(ctx context.Context) (Plan, error) {
	if u == nil {
		return Plan{}, fmt.Errorf("updater is nil")
	}
	if !u.operation.TryLock() {
		return Plan{}, ErrUpdateInProgress
	}
	defer u.operation.Unlock()
	u.emit(Event{Stage: StageChecking, CurrentVersion: u.config.CurrentVersion})
	release, err := u.config.Source.LatestStable(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve latest stable release: %w", err)
	}
	release = cloneRelease(release)
	if err := ValidateStableRelease(release); err != nil {
		return Plan{}, err
	}
	available, err := IsNewerStable(release.Tag, u.config.CurrentVersion)
	if err != nil {
		return Plan{}, err
	}
	state := &planState{
		owner:     u,
		available: available,
		release:   release,
	}
	if !available {
		state.generation = u.planGeneration.Add(1)
		u.emit(Event{
			Stage:          StageUpToDate,
			CurrentVersion: u.config.CurrentVersion,
			TargetVersion:  release.Tag,
		})
		return Plan{state: state}, nil
	}
	// Fail before downloading even the checksum manifest when the target cannot
	// currently participate in a safe replacement. Apply repeats this check
	// under the transaction lock because filesystem state can change later.
	if err := preflightReplacement(u.config.ExecutablePath); err != nil {
		return Plan{}, err
	}

	archiveName, binaryName, archiveAsset, checksumsAsset, err := u.resolvePlanAssets(release)
	if err != nil {
		return Plan{}, err
	}
	state.archiveName = archiveName
	state.binaryName = binaryName
	state.archiveAsset = archiveAsset
	state.checksumsAsset = checksumsAsset

	u.emit(Event{
		Stage:         StageDownloadingChecksums,
		TargetVersion: release.Tag,
		Asset:         checksumsAsset.Name,
	})
	manifest, err := u.downloadBytes(ctx, checksumsAsset, maxChecksumManifestSize)
	if err != nil {
		return Plan{}, fmt.Errorf("download checksum manifest: %w", err)
	}
	state.expectedChecksum, err = parseChecksumManifest(string(manifest), archiveName)
	if err != nil {
		return Plan{}, err
	}
	state.generation = u.planGeneration.Add(1)
	u.emit(Event{
		Stage:          StageAvailable,
		CurrentVersion: u.config.CurrentVersion,
		TargetVersion:  release.Tag,
		Asset:          archiveName,
		Bytes:          archiveAsset.Size,
	})
	return Plan{state: state}, nil
}

// Apply performs one locked transaction using exactly the release, assets,
// archive name, binary name, and checksum captured by Prepare. It never resolves
// latest again. A failed transaction may be retried with the same plan; a
// successful plan is consumed.
func (u *Updater) Apply(ctx context.Context, plan Plan) (result Result, resultErr error) {
	if u == nil {
		return Result{}, fmt.Errorf("updater is nil")
	}
	if !u.operation.TryLock() {
		return Result{}, ErrUpdateInProgress
	}
	defer u.operation.Unlock()
	state, err := u.validatePlan(plan)
	if err != nil {
		return Result{}, err
	}
	result.Release = cloneRelease(state.release)
	if !state.available {
		if err := u.beginPlan(state); err != nil {
			return result, err
		}
		state.status.Store(planConsumed)
		return result, nil
	}

	releaseLock, err := acquireUpdateLock(u.lockPath)
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
	if err := u.beginPlan(state); err != nil {
		return result, err
	}
	defer func() {
		if resultErr == nil {
			state.status.Store(planConsumed)
		} else {
			state.status.Store(planReady)
		}
	}()
	if err := preflightReplacement(u.config.ExecutablePath); err != nil {
		return result, err
	}

	result.ArchiveAsset = state.archiveName

	u.emit(Event{
		Stage:         StageDownloadingArchive,
		TargetVersion: state.release.Tag,
		Asset:         state.archiveName,
	})
	archivePath, archiveBytes, err := u.downloadFile(ctx, state.archiveAsset, u.config.MaxArchiveSize)
	if err != nil {
		return result, fmt.Errorf("download release archive: %w", err)
	}
	defer os.Remove(archivePath)
	if err := verifyFileChecksum(archivePath, state.expectedChecksum); err != nil {
		return result, err
	}
	u.emit(Event{
		Stage:         StageChecksumVerified,
		TargetVersion: state.release.Tag,
		Asset:         state.archiveName,
		Bytes:         archiveBytes,
	})

	targetDirectory := filepath.Dir(u.config.ExecutablePath)
	stagedPath, err := extractBinary(
		archivePath,
		state.archiveName,
		state.binaryName,
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
	stagedChecksum, err := fileSHA256(stagedPath)
	if err != nil {
		return result, fmt.Errorf("hash staged binary: %w", err)
	}
	if err := u.config.Verifier.Verify(ctx, stagedPath, state.release.Tag); err != nil {
		return result, fmt.Errorf("verify staged binary: %w", err)
	}
	if err := requireFileChecksum(stagedPath, stagedChecksum, "staged version probe modified the binary"); err != nil {
		return result, err
	}
	u.emit(Event{
		Stage:         StageStagedVerified,
		TargetVersion: state.release.Tag,
		Asset:         state.archiveName,
	})

	u.emit(Event{
		Stage:         StageInstalling,
		TargetVersion: state.release.Tag,
		Asset:         state.archiveName,
	})
	backupPath, err := replaceExecutable(u.config.ExecutablePath, stagedPath, func(path string) error {
		if err := requireFileChecksum(path, stagedChecksum, "installed binary changed before version verification"); err != nil {
			return err
		}
		if err := u.config.Verifier.Verify(ctx, path, state.release.Tag); err != nil {
			return err
		}
		return requireFileChecksum(path, stagedChecksum, "installed version probe modified the binary")
	})
	result.BackupRetainedAt = backupPath
	if err != nil {
		return result, fmt.Errorf("install release binary: %w", err)
	}
	u.emit(Event{
		Stage:         StageInstalledVerified,
		TargetVersion: state.release.Tag,
		Asset:         state.archiveName,
	})
	result.Updated = true
	u.emit(Event{
		Stage:         StageComplete,
		TargetVersion: state.release.Tag,
		Asset:         state.archiveName,
	})
	return result, nil
}

// UpdateLatest is the explicit non-interactive convenience for already
// authorized CLI or administrative flows. Interactive hosts should Prepare,
// render the exact plan, obtain confirmation, and then Apply that plan.
func (u *Updater) UpdateLatest(ctx context.Context) (Result, error) {
	plan, err := u.Prepare(ctx)
	if err != nil {
		return Result{}, err
	}
	return u.Apply(ctx, plan)
}

func (u *Updater) validatePlan(plan Plan) (*planState, error) {
	if plan.state == nil || plan.state.owner != u {
		return nil, ErrInvalidPlan
	}
	if plan.state.generation != u.planGeneration.Load() {
		return nil, ErrPlanSuperseded
	}
	switch plan.state.status.Load() {
	case planApplying:
		return nil, ErrUpdateInProgress
	case planConsumed:
		return nil, ErrPlanConsumed
	default:
		return plan.state, nil
	}
}

func (u *Updater) beginPlan(state *planState) error {
	if state == nil || state.owner != u {
		return ErrInvalidPlan
	}
	if state.generation != u.planGeneration.Load() {
		return ErrPlanSuperseded
	}
	if state.status.CompareAndSwap(planReady, planApplying) {
		return nil
	}
	if state.status.Load() == planConsumed {
		return ErrPlanConsumed
	}
	return ErrUpdateInProgress
}

func (u *Updater) resolvePlanAssets(release Release) (string, string, Asset, Asset, error) {
	archiveName := strings.TrimSpace(u.config.AssetName(release.Tag, u.platformOS, u.platformArch))
	if !validBinaryName(archiveName) {
		return "", "", Asset{}, Asset{}, fmt.Errorf("archive asset naming function returned an invalid file name")
	}
	if !strings.HasSuffix(archiveName, ".tar.gz") && !strings.HasSuffix(archiveName, ".tgz") && !strings.HasSuffix(archiveName, ".zip") {
		return "", "", Asset{}, Asset{}, fmt.Errorf("archive asset %q must be .tar.gz, .tgz, or .zip", archiveName)
	}
	binaryName := u.config.BinaryName
	if u.config.ArchiveBinaryName != nil {
		binaryName = strings.TrimSpace(u.config.ArchiveBinaryName(release.Tag, u.platformOS, u.platformArch))
	}
	if !validBinaryName(binaryName) {
		return "", "", Asset{}, Asset{}, fmt.Errorf("archive binary naming function returned an invalid file name")
	}
	archiveAsset, err := exactAsset(release, archiveName)
	if err != nil {
		return "", "", Asset{}, Asset{}, err
	}
	checksumsAsset, err := exactAsset(release, u.config.ChecksumsAsset)
	if err != nil {
		return "", "", Asset{}, Asset{}, err
	}
	if err := validateAssetSize(archiveAsset, u.config.MaxArchiveSize); err != nil {
		return "", "", Asset{}, Asset{}, err
	}
	if err := validateAssetSize(checksumsAsset, maxChecksumManifestSize); err != nil {
		return "", "", Asset{}, Asset{}, err
	}
	return archiveName, binaryName, archiveAsset, checksumsAsset, nil
}

func validateAssetSize(asset Asset, maximum int64) error {
	if asset.Size < 0 {
		return fmt.Errorf("asset %s declares a negative size", asset.Name)
	}
	if asset.Size > maximum {
		return fmt.Errorf("%w: %s declares %d bytes (maximum %d)", errAssetTooLarge, asset.Name, asset.Size, maximum)
	}
	return nil
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
	if err := validateAssetSize(asset, maximum); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := &boundedWriter{destination: &buffer, remaining: maximum}
	if err := u.config.Source.Download(ctx, asset, writer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (u *Updater) downloadFile(ctx context.Context, asset Asset, maximum int64) (string, int64, error) {
	if err := validateAssetSize(asset, maximum); err != nil {
		return "", 0, err
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
	actual, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("hash downloaded archive: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("checksum mismatch for release archive; refusing to install")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func requireFileChecksum(path, expected, message string) error {
	actual, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("hash executable: %w", err)
	}
	if actual != expected {
		return errors.New(message)
	}
	return nil
}
