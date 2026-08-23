package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// ErrUpdateInProgress is returned when this process or another process
	// already owns the updater lock for the target executable.
	ErrUpdateInProgress = errors.New("another update is already in progress")
)

// Asset is one immutable asset attached to a GitHub Release.
type Asset struct {
	Name        string
	DownloadURL string
	Size        int64
}

// Release contains only the release fields the updater trusts.
type Release struct {
	Tag        string
	URL        string
	Draft      bool
	Prerelease bool
	Assets     []Asset
}

// Source resolves the latest stable candidate and downloads exact assets from
// that same Release object.
type Source interface {
	LatestStable(ctx context.Context) (Release, error)
	Download(ctx context.Context, asset Asset, destination io.Writer) error
}

// AssetNameFunc maps a release tag and platform to the exact archive name.
type AssetNameFunc func(tag, goos, goarch string) string

// VersionVerifier proves that a staged or installed executable identifies as
// the selected release.
type VersionVerifier interface {
	Verify(ctx context.Context, executablePath, expectedTag string) error
}

// VersionVerifierFunc adapts a function to VersionVerifier.
type VersionVerifierFunc func(context.Context, string, string) error

// Verify implements VersionVerifier.
func (function VersionVerifierFunc) Verify(ctx context.Context, path, tag string) error {
	return function(ctx, path, tag)
}

// Stage is a UI-independent update milestone.
type Stage string

const (
	StageChecking           Stage = "checking"
	StageUpToDate           Stage = "up_to_date"
	StageAvailable          Stage = "available"
	StageDownloadingChecks  Stage = "downloading_checksums"
	StageDownloadingArchive Stage = "downloading_archive"
	StageChecksumVerified   Stage = "checksum_verified"
	StageStagedVerified     Stage = "staged_version_verified"
	StageInstalling         Stage = "installing"
	StageInstalledVerified  Stage = "installed_version_verified"
	StageComplete           Stage = "complete"
)

// Event lets a CLI, chat adapter, or UI render progress without owning update
// policy or file mutation.
type Event struct {
	Stage          Stage
	CurrentVersion string
	TargetVersion  string
	Asset          string
	Bytes          int64
	Detail         string
}

// Config defines a standalone updater transaction.
type Config struct {
	Product        string
	CurrentVersion string
	ExecutablePath string
	BinaryName     string
	ChecksumsAsset string
	AssetName      AssetNameFunc
	Source         Source
	Verifier       VersionVerifier
	LockPath       string
	PlatformOS     string
	PlatformArch   string
	MaxArchiveSize int64
	MaxBinarySize  int64
	Progress       func(Event)
}

// CheckResult reports the validated stable candidate without mutating files.
type CheckResult struct {
	Release   Release
	Available bool
}

// Result describes a completed update transaction.
type Result struct {
	Release          Release
	Updated          bool
	ArchiveAsset     string
	BackupRetainedAt string
}

// Updater is immutable after construction and safe to keep in a host service.
type Updater struct {
	config Config
}

const (
	defaultMaxArchiveSize = 256 * 1024 * 1024
	defaultMaxBinarySize  = 128 * 1024 * 1024
)

// New validates and normalizes a standalone updater configuration.
func New(config Config) (*Updater, error) {
	config.Product = strings.TrimSpace(config.Product)
	if config.Product == "" {
		return nil, fmt.Errorf("updater product is required")
	}
	config.CurrentVersion = strings.TrimSpace(config.CurrentVersion)
	if config.CurrentVersion == "" {
		return nil, fmt.Errorf("current version is required")
	}
	if config.Source == nil {
		return nil, fmt.Errorf("release source is required")
	}
	if config.AssetName == nil {
		return nil, fmt.Errorf("archive asset naming function is required")
	}
	if config.Verifier == nil {
		return nil, fmt.Errorf("version verifier is required")
	}
	if strings.TrimSpace(config.ExecutablePath) == "" {
		path, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate current executable: %w", err)
		}
		config.ExecutablePath = path
	}
	absolute, err := filepath.Abs(config.ExecutablePath)
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	config.ExecutablePath = absolute
	info, err := os.Lstat(config.ExecutablePath)
	if err != nil {
		return nil, fmt.Errorf("inspect current executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("standalone updater refuses a symlink executable path; use an install-kind adapter")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("current executable is not a regular file")
	}
	config.BinaryName = strings.TrimSpace(config.BinaryName)
	if config.BinaryName == "" || filepath.Base(config.BinaryName) != config.BinaryName || config.BinaryName == "." {
		return nil, fmt.Errorf("binary name must be one file name")
	}
	if config.ChecksumsAsset == "" {
		config.ChecksumsAsset = "checksums.txt"
	}
	if filepath.Base(config.ChecksumsAsset) != config.ChecksumsAsset {
		return nil, fmt.Errorf("checksums asset must be one file name")
	}
	if config.LockPath == "" {
		config.LockPath = config.ExecutablePath + ".update.lock"
	} else if !filepath.IsAbs(config.LockPath) {
		return nil, fmt.Errorf("lock path must be absolute")
	}
	if config.PlatformOS == "" {
		config.PlatformOS = runtime.GOOS
	}
	if config.PlatformArch == "" {
		config.PlatformArch = runtime.GOARCH
	}
	if config.MaxArchiveSize <= 0 {
		config.MaxArchiveSize = defaultMaxArchiveSize
	}
	if config.MaxBinarySize <= 0 {
		config.MaxBinarySize = defaultMaxBinarySize
	}
	return &Updater{config: config}, nil
}

// ReleaseArchiveName returns the common
// <product>-<tag>-<os>-<arch>.tar.gz/.zip naming policy.
func ReleaseArchiveName(product string) AssetNameFunc {
	product = strings.TrimSpace(product)
	return func(tag, goos, goarch string) string {
		extension := ".tar.gz"
		if goos == "windows" {
			extension = ".zip"
		}
		return fmt.Sprintf("%s-%s-%s-%s%s", product, tag, goos, goarch, extension)
	}
}

func (u *Updater) emit(event Event) {
	if u != nil && u.config.Progress != nil {
		u.config.Progress(event)
	}
}
