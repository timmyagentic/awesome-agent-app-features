package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestApplyInstallsExactlyThePreparedReleaseWithoutRefetching(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(_ context.Context, path, tag string) error {
		if tag != "v1.2.3" {
			return fmt.Errorf("unexpected tag %s", tag)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "new binary" {
			return fmt.Errorf("unexpected content %q", data)
		}
		return nil
	}))
	var latestCalls atomic.Int32
	harness.source.latest = func(context.Context) (Release, error) {
		call := latestCalls.Add(1)
		if call == 1 {
			return harness.source.release, nil
		}
		return Release{Tag: "v9.9.9"}, errors.New("Apply must not resolve latest again")
	}

	plan, err := harness.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if !plan.Available() || plan.Release().Tag != "v1.2.3" || plan.ArchiveAsset().Name != harness.archiveName {
		t.Fatalf("plan = available:%v release:%+v archive:%+v", plan.Available(), plan.Release(), plan.ArchiveAsset())
	}

	// Mutating a value returned for presentation must not mutate the exact plan.
	presented := plan.Release()
	presented.Tag = "v9.9.9"
	presented.Assets[0].Name = "attacker-controlled.tar.gz"

	result, err := harness.updater.Apply(context.Background(), plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Updated || result.Release.Tag != "v1.2.3" || latestCalls.Load() != 1 {
		t.Fatalf("result = %+v; latest calls = %d", result, latestCalls.Load())
	}
	assertContent(t, harness.target, "new binary")
}

func TestPreparePinsChecksumManifestBeforeApproval(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	plan, err := harness.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got := fmt.Sprint(harness.source.downloads); got != fmt.Sprint([]string{"checksums.txt"}) {
		t.Fatalf("Prepare downloads = %v", harness.source.downloads)
	}

	// Replacing the release checksum after the plan was shown cannot authorize a
	// different archive. Apply must use the checksum captured in the plan.
	harness.source.data["checksums.txt"] = []byte(strings.Repeat("0", 64) + "  " + harness.archiveName + "\n")
	if _, err := harness.updater.Apply(context.Background(), plan); err != nil {
		t.Fatalf("Apply exact plan: %v", err)
	}
	if got := fmt.Sprint(harness.source.downloads); got != fmt.Sprint([]string{"checksums.txt", harness.archiveName}) {
		t.Fatalf("all downloads = %v", harness.source.downloads)
	}
}

func TestPlanOwnershipSupersessionAndRetrySemantics(t *testing.T) {
	first := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	second := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))

	if _, err := first.updater.Apply(context.Background(), Plan{}); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("zero plan error = %v", err)
	}
	foreign, err := first.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare foreign plan: %v", err)
	}
	if _, err := second.updater.Apply(context.Background(), foreign); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("foreign plan error = %v", err)
	}

	stale, err := first.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare stale plan: %v", err)
	}
	current, err := first.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare current plan: %v", err)
	}
	if _, err := first.updater.Apply(context.Background(), stale); !errors.Is(err, ErrPlanSuperseded) {
		t.Fatalf("superseded plan error = %v", err)
	}

	failedOnce := false
	originalDownload := first.source.beforeDownload
	first.source.beforeDownload = func(asset Asset) {
		if originalDownload != nil {
			originalDownload(asset)
		}
		if asset.Name == first.archiveName && !failedOnce {
			failedOnce = true
			delete(first.source.data, asset.Name)
		}
	}
	if _, err := first.updater.Apply(context.Background(), current); err == nil {
		t.Fatal("first Apply unexpectedly succeeded")
	}
	first.source.data[first.archiveName] = first.archive
	first.source.beforeDownload = nil
	if _, err := first.updater.Apply(context.Background(), current); err != nil {
		t.Fatalf("retry exact plan: %v", err)
	}
	if _, err := first.updater.Apply(context.Background(), current); !errors.Is(err, ErrPlanConsumed) {
		t.Fatalf("consumed plan error = %v", err)
	}
}

func TestUpdateLatestIsTheExplicitNonInteractiveConvenience(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	result, err := harness.updater.UpdateLatest(context.Background())
	if err != nil {
		t.Fatalf("UpdateLatest: %v", err)
	}
	if !result.Updated {
		t.Fatalf("result = %+v", result)
	}
}

func TestDifferentTargetsDoNotShareAnInProcessLock(t *testing.T) {
	first := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	second := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	firstPlan, err := first.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("prepare first: %v", err)
	}
	secondPlan, err := second.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("prepare second: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	first.source.beforeDownload = func(asset Asset) {
		if asset.Name == first.archiveName {
			close(started)
			<-release
		}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := first.updater.Apply(context.Background(), firstPlan)
		firstDone <- err
	}()
	<-started
	if _, err := second.updater.Apply(context.Background(), secondPlan); err != nil {
		close(release)
		t.Fatalf("independent target Apply: %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Apply: %v", err)
	}
}

func TestDifferentUpdaterValuesForOneTargetShareTheDerivedLock(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	second, err := New(harness.updater.config)
	if err != nil {
		t.Fatalf("New second updater: %v", err)
	}
	firstPlan, err := harness.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare first: %v", err)
	}
	secondPlan, err := second.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare second: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var blocked atomic.Bool
	harness.source.beforeDownload = func(asset Asset) {
		if asset.Name == harness.archiveName && blocked.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := harness.updater.Apply(context.Background(), firstPlan)
		firstDone <- err
	}()
	<-started
	if _, err := second.Apply(context.Background(), secondPlan); !errors.Is(err, ErrUpdateInProgress) {
		close(release)
		t.Fatalf("same-target Apply error = %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Apply: %v", err)
	}
}

func TestPrepareRejectsInvalidSelectedAssetMetadata(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	harness.source.release.Assets[0].Size = -1
	if _, err := harness.updater.Prepare(context.Background()); err == nil || !strings.Contains(err.Error(), "negative size") {
		t.Fatalf("negative asset size error = %v", err)
	}
}
