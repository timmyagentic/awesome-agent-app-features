// Package v1_test is an external-consumer compile contract. It must use only
// exported symbols from the four supported v1 packages.
package v1_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/timmyagentic/awesome-agent-app-features/feedback"
	"github.com/timmyagentic/awesome-agent-app-features/feedback/httpclient"
	"github.com/timmyagentic/awesome-agent-app-features/updater"
	updatergithub "github.com/timmyagentic/awesome-agent-app-features/updater/github"
)

type stableSource struct{}

func (stableSource) LatestStable(context.Context) (updater.Release, error) {
	return updater.Release{
		Tag:   "v1.0.0",
		URL:   "https://example.invalid/releases/tag/v1.0.0",
		Notes: "Consumer release notes",
	}, nil
}

func (stableSource) Download(context.Context, updater.Asset, io.Writer) error {
	return errors.New("up-to-date consumer fixture must not download")
}

var _ updater.Source = stableSource{}
var _ updater.Source = updatergithub.Source{}
var _ updater.VersionVerifier = updater.VersionVerifierFunc(nil)
var _ updater.VersionVerifier = updater.CommandVersionVerifier{}

func TestFeedbackV1ConsumerContract(t *testing.T) {
	if feedback.WireSchemaVersion != 1 || httpclient.EndpointPath != "/v1/feedback" {
		t.Fatal("unexpected Feedback v1 constants")
	}
	if feedback.MaxDescriptionBytes <= 0 || feedback.MaxErrorBytes <= 0 ||
		feedback.MaxMetadataBytes <= 0 || feedback.MaxCapabilityGapBytes <= 0 ||
		feedback.MaxCapabilityGaps <= 0 || feedback.DefaultErrorMaxAge <= 0 {
		t.Fatal("invalid Feedback limits")
	}
	draft, err := (feedback.Builder{
		Now: func() time.Time { return time.Now().UTC() },
		AdditionalRedact: func(value string) string {
			return value
		},
		ErrorMaxAge: time.Minute,
	}).Build(feedback.Input{
		Description: "Improve diagnostics",
		RecentError: &feedback.RecentError{
			Text: "failure",
			At:   time.Now().UTC(),
		},
		CapabilityGaps: []string{"doctor.explain"},
		Environment: feedback.Environment{
			Product: "Consumer",
			Version: "v1.0.0",
			OS:      "test-os",
			Arch:    "test-arch",
			Agent:   "test-agent",
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	report := draft.Report()
	if report.Environment.Product == "" || report.Description == "" ||
		report.RecentError == nil || len(report.CapabilityGaps) == 0 {
		t.Fatalf("incomplete report: %+v", report)
	}
	if _, err := json.Marshal(report); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatalf("Report MarshalJSON: %v", err)
	}
	if _, err := draft.Approve(false); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatalf("Approve(false): %v", err)
	}
	approved, err := draft.Approve(true)
	if err != nil {
		t.Fatalf("Approve(true): %v", err)
	}
	if _, err := json.Marshal(approved); err != nil {
		t.Fatalf("Approved MarshalJSON: %v", err)
	}
	if _, err := json.Marshal(feedback.Approved{}); !errors.Is(err, feedback.ErrApprovalRequired) {
		t.Fatalf("zero Approved MarshalJSON: %v", err)
	}
	_ = feedback.Redact("token=secret")
	_ = feedback.ErrNothingToReport
	_ = httpclient.Client{
		Endpoint:   "https://relay.example/v1/feedback",
		HTTPClient: &http.Client{Timeout: time.Second},
		UserAgent:  "consumer/1",
	}
	_ = httpclient.Receipt{ReferenceURL: "https://example.invalid/feedback/1", Deduplicated: true}
}

func TestUpdaterV1ConsumerContract(t *testing.T) {
	target := filepath.Join(t.TempDir(), "consumer")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	var events []updater.Event
	service, err := updater.New(updater.Config{
		Product:        "Consumer",
		CurrentVersion: "v1.0.0",
		ExecutablePath: target,
		BinaryName:     "consumer",
		ChecksumsAsset: "checksums.txt",
		AssetName:      updater.ReleaseArchiveName("consumer"),
		Source:         stableSource{},
		Verifier: updater.VersionVerifierFunc(func(context.Context, string, string) error {
			return nil
		}),
		MaxArchiveSize: 1024,
		MaxBinarySize:  512,
		Progress: func(event updater.Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plan, err := service.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.Available() || plan.Release().Tag != "v1.0.0" || plan.Release().Notes != "Consumer release notes" ||
		plan.ArchiveAsset() != (updater.Asset{}) {
		t.Fatalf("plan accessors returned unexpected values")
	}
	result, err := service.Apply(context.Background(), plan)
	if err != nil || result.Updated || result.Release.Tag != "v1.0.0" {
		t.Fatalf("Apply = %+v, %v", result, err)
	}
	if len(events) != 2 || events[0].Product != "Consumer" || events[0].Stage != updater.StageChecking || events[1].Stage != updater.StageUpToDate {
		t.Fatalf("events = %+v", events)
	}
	if _, err := service.Apply(context.Background(), plan); !errors.Is(err, updater.ErrPlanConsumed) {
		t.Fatalf("consumed plan error = %v", err)
	}
	if result, err := service.UpdateLatest(context.Background()); err != nil || result.Updated {
		t.Fatalf("UpdateLatest up-to-date = %+v, %v", result, err)
	}
	if newer, err := updater.IsNewerStable("v1.0.1", "v1.0.0"); err != nil || !newer {
		t.Fatalf("IsNewerStable = %v, %v", newer, err)
	}
	if err := updater.ValidateStableRelease(updater.Release{Tag: "v1.0.0"}); err != nil {
		t.Fatalf("ValidateStableRelease: %v", err)
	}
	_ = updater.CommandVersionVerifier{
		Args:         []string{"--version"},
		ExpectedLine: func(string) string { return "consumer v1.0.0" },
		Timeout:      time.Second,
	}
	_ = updater.ExactVersionLine("consumer")
	_ = updater.BinaryNameFunc(func(string, string, string) string { return "consumer" })
	_ = updater.AssetNameFunc(func(string, string, string) string { return "consumer.tar.gz" })
	_ = []updater.Stage{
		updater.StageChecking,
		updater.StageUpToDate,
		updater.StageAvailable,
		updater.StageDownloadingChecksums,
		updater.StageDownloadingArchive,
		updater.StageChecksumVerified,
		updater.StageStagedVerified,
		updater.StageInstalling,
		updater.StageInstalledVerified,
		updater.StageComplete,
	}
	_ = updater.ErrInvalidPlan
	_ = updater.ErrPlanSuperseded
	_ = updater.ErrUpdateInProgress
	_ = updater.Result{ArchiveAsset: "archive.tar.gz", BackupRetainedAt: "backup"}
	_ = updatergithub.Source{
		Repository:        "owner/repository",
		HTTPClient:        &http.Client{Timeout: time.Second},
		UserAgent:         "consumer/1",
		APIBaseURL:        "https://api.github.com",
		AllowedAssetHosts: []string{"release-assets.githubusercontent.com"},
	}
}
