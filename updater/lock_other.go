//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package updater

import "errors"

func tryPlatformLock(string) (heldPlatformLock, error) {
	return nil, errors.New("platform file lock is unavailable on this unsupported operating system")
}
