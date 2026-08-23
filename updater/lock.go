package updater

import "sync"

var processUpdateLock sync.Mutex

func acquireUpdateLock(path string) (func() error, error) {
	if !processUpdateLock.TryLock() {
		return nil, ErrUpdateInProgress
	}
	platformLock, err := tryPlatformLock(path)
	if err != nil {
		processUpdateLock.Unlock()
		return nil, err
	}
	return func() error {
		err := platformLock.release()
		processUpdateLock.Unlock()
		return err
	}, nil
}

type heldPlatformLock interface {
	release() error
}
