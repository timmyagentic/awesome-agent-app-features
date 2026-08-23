//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package updater

import (
	"errors"
	"fmt"
	"os"
)

// The MVP does not promise in-place Windows replacement. This fallback still
// prevents concurrent healthy processes; a crash may leave a lock that an
// operator must remove after verifying no updater is active.
type exclusiveFileLock struct {
	file *os.File
	path string
}

func tryPlatformLock(path string) (heldPlatformLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrUpdateInProgress
		}
		return nil, fmt.Errorf("acquire update lock: %w", err)
	}
	_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	return &exclusiveFileLock{file: file, path: path}, nil
}

func (lock *exclusiveFileLock) release() error {
	if lock == nil {
		return nil
	}
	if lock.file != nil {
		_ = lock.file.Close()
	}
	if err := os.Remove(lock.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove update lock: %w", err)
	}
	return nil
}
