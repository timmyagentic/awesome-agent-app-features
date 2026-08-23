//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package updater

import (
	"errors"
	"fmt"
	"os"
)

// Version 1 does not promise in-place Windows replacement. This fallback still
// prevents concurrent healthy processes; a crash may leave a lock that an
// operator must remove after verifying no updater is active.
type exclusiveFileLock struct {
	file *os.File
	path string
	info os.FileInfo
}

func tryPlatformLock(path string) (heldPlatformLock, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("update lock is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect update lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrUpdateInProgress
		}
		return nil, fmt.Errorf("acquire update lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("inspect acquired update lock: %w", err)
	}
	_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	return &exclusiveFileLock{file: file, path: path, info: info}, nil
}

func (lock *exclusiveFileLock) release() error {
	if lock == nil {
		return nil
	}
	if lock.file != nil {
		if err := lock.file.Close(); err != nil {
			return fmt.Errorf("close update lock: %w", err)
		}
	}
	current, err := os.Lstat(lock.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect update lock before removal: %w", err)
	}
	if lock.info == nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(lock.info, current) {
		return fmt.Errorf("update lock path changed before removal")
	}
	if err := os.Remove(lock.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove update lock: %w", err)
	}
	return nil
}
