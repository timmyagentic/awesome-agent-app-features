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
	"sync"
	"sync/atomic"
	"unicode"
)

var (
	// ErrUpdateInProgress is returned when this process or another process
	// already owns the updater lock for the target executable.
	ErrUpdateInProgress error = errors.New("another update is already in progress")
	// ErrInvalidPlan means Apply received a zero plan or a plan prepared by a
	// different Updater instance.
	ErrInvalidPlan error = errors.New("update plan is invalid or belongs to another updater")
	// ErrPlanSuperseded means a successful later Prepare replaced this plan.
	ErrPlanSuperseded error = errors.New("update plan was superseded by a newer plan")
	// ErrPlanConsumed means this exact plan already completed successfully.
	ErrPlanConsumed error = errors.New("update plan was already applied")
)

// Asset is one immutable asset attached to a source release.
type Asset struct {
	Name        string
	DownloadURL string
	Size        int64
}

// Release contains immutable metadata from the exact release selected by the
// updater. Notes are presentation-neutral publisher text: hosts may localize,
// truncate, or ignore them, but Apply never re-resolves them or the release.
type Release struct {
	Tag        string
	URL        string
	Notes      string
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

// BinaryNameFunc maps a release tag and platform to the exact executable file
// name inside the archive. It exists for projects whose release pipeline puts
// versioned or platform-qualified binaries in otherwise conventional archives.
type BinaryNameFunc func(tag, goos, goarch string) string

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
	StageChecking             Stage = "checking"
	StageUpToDate             Stage = "up_to_date"
	StageAvailable            Stage = "available"
	StageDownloadingChecksums Stage = "downloading_checksums"
	StageDownloadingArchive   Stage = "downloading_archive"
	StageChecksumVerified     Stage = "checksum_verified"
	StageStagedVerified       Stage = "staged_version_verified"
	StageInstalling           Stage = "installing"
	StageInstalledVerified    Stage = "installed_version_verified"
	StageComplete             Stage = "complete"
)

// Event lets a CLI, chat adapter, or UI render progress without owning update
// policy or file mutation.
type Event struct {
	Product        string
	Stage          Stage
	CurrentVersion string
	TargetVersion  string
	Asset          string
	Bytes          int64
}

// Config defines a standalone updater transaction.
type Config struct {
	Product        string
	CurrentVersion string
	ExecutablePath string
	BinaryName     string
	// ArchiveBinaryName is an optional alternative to the static BinaryName.
	// Configure exactly one of them.
	ArchiveBinaryName BinaryNameFunc
	ChecksumsAsset    string
	AssetName         AssetNameFunc
	Source            Source
	Verifier          VersionVerifier
	MaxArchiveSize    int64
	MaxBinarySize     int64
	Progress          func(Event)
}

// Plan is an opaque, exact update decision returned by Prepare. Its zero value
// is invalid. Presentation accessors return copies and cannot change what Apply
// will download or install.
type Plan struct {
	state *planState
}

// Available reports whether the prepared release is newer than the configured
// current version.
func (plan Plan) Available() bool {
	return plan.state != nil && plan.state.available
}

// Release returns a deep copy of the exact release selected by Prepare.
func (plan Plan) Release() Release {
	if plan.state == nil {
		return Release{}
	}
	return cloneRelease(plan.state.release)
}

// ArchiveAsset returns the exact archive asset selected by Prepare. It is zero
// when Available reports false.
func (plan Plan) ArchiveAsset() Asset {
	if plan.state == nil {
		return Asset{}
	}
	return plan.state.archiveAsset
}

// Result describes an update transaction. BackupRetainedAt may be populated on
// either success or failure when recovery evidence still needs operator action.
type Result struct {
	Release          Release
	Updated          bool
	ArchiveAsset     string
	BackupRetainedAt string
}

// Updater does not reconfigure itself after construction and is safe to keep in
// a host service when injected Source, Verifier, and Progress implementations
// are themselves safe for the host's concurrency model.
type Updater struct {
	config         Config
	lockPath       string
	platformOS     string
	platformArch   string
	operation      sync.Mutex
	planGeneration atomic.Uint64
}

type planState struct {
	owner            *Updater
	generation       uint64
	status           atomic.Uint32
	available        bool
	release          Release
	archiveAsset     Asset
	checksumsAsset   Asset
	archiveName      string
	binaryName       string
	expectedChecksum string
}

const (
	planReady uint32 = iota
	planApplying
	planConsumed
)

const (
	defaultMaxArchiveSize = 256 * 1024 * 1024
	defaultMaxBinarySize  = 128 * 1024 * 1024
)

// New validates and normalizes a standalone updater configuration.
func New(config Config) (*Updater, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, fmt.Errorf("standalone updater is supported only on macOS and Linux; use an install-kind adapter")
	}
	config.Product = strings.TrimSpace(config.Product)
	if config.Product == "" {
		return nil, fmt.Errorf("updater product is required")
	}
	config.CurrentVersion = strings.TrimSpace(config.CurrentVersion)
	if config.CurrentVersion == "" {
		return nil, fmt.Errorf("current version is required")
	}
	if !developmentPattern.MatchString(config.CurrentVersion) {
		if _, err := parseVersion(config.CurrentVersion); err != nil {
			return nil, fmt.Errorf("current version is invalid: %w", err)
		}
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
	if config.ArchiveBinaryName != nil && config.BinaryName != "" {
		return nil, fmt.Errorf("configure either binary name or archive binary naming function, not both")
	}
	if config.ArchiveBinaryName == nil && !validBinaryName(config.BinaryName) {
		return nil, fmt.Errorf("binary name must be one file name")
	}
	if config.ChecksumsAsset == "" {
		config.ChecksumsAsset = "checksums.txt"
	}
	config.ChecksumsAsset = strings.TrimSpace(config.ChecksumsAsset)
	if !validBinaryName(config.ChecksumsAsset) {
		return nil, fmt.Errorf("checksums asset must be one file name")
	}
	if config.MaxArchiveSize <= 0 {
		config.MaxArchiveSize = defaultMaxArchiveSize
	}
	if config.MaxBinarySize <= 0 {
		config.MaxBinarySize = defaultMaxBinarySize
	}
	return &Updater{
		config:       config,
		lockPath:     config.ExecutablePath + ".update.lock",
		platformOS:   runtime.GOOS,
		platformArch: runtime.GOARCH,
	}, nil
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

func validBinaryName(value string) bool {
	if value == "" || value == "." || value == ".." || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) {
		return false
	}
	return !strings.ContainsFunc(value, func(character rune) bool {
		return unicode.IsSpace(character) || character < 0x20 || character == 0x7f
	})
}

func cloneRelease(value Release) Release {
	clone := value
	clone.Assets = append([]Asset(nil), value.Assets...)
	return clone
}

func (u *Updater) emit(event Event) {
	if u != nil && u.config.Progress != nil {
		if event.Product == "" {
			event.Product = u.config.Product
		}
		if event.CurrentVersion == "" {
			event.CurrentVersion = u.config.CurrentVersion
		}
		u.config.Progress(event)
	}
}
