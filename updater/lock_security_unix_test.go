//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package updater

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateLockRefusesSymlinkWithoutTouchingTarget(t *testing.T) {
	harness := newUpdateHarness(t, VersionVerifierFunc(func(context.Context, string, string) error { return nil }))
	plan, err := harness.updater.Prepare(context.Background())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	victim := filepath.Join(t.TempDir(), "must-not-change")
	if err := os.WriteFile(victim, []byte("important"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	lockPath := filepath.Join(filepath.Dir(victim), "lock-link")
	if err := os.Symlink(victim, lockPath); err != nil {
		t.Fatalf("create lock symlink: %v", err)
	}
	harness.updater.lockPath = lockPath
	if _, err := harness.updater.Apply(context.Background(), plan); err == nil {
		t.Fatal("Apply accepted a symlink lock path")
	}
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(data) != "important" {
		t.Fatalf("lock symlink target changed to %q", data)
	}
}
