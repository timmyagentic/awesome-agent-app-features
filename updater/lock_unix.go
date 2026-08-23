//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package updater

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type unixFileLock struct {
	file *os.File
}

func tryPlatformLock(path string) (heldPlatformLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open update lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrUpdateInProgress
		}
		return nil, fmt.Errorf("acquire update lock: %w", err)
	}
	if err := file.Truncate(0); err == nil {
		_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
		_ = file.Sync()
	}
	return &unixFileLock{file: file}, nil
}

func (lock *unixFileLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release update lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close update lock: %w", closeErr)
	}
	return nil
}
