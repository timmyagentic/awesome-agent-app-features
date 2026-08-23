package updater

import "sync"

// processUpdateLocks prevents two Updater values in this process from racing on
// the same target while allowing unrelated products or installations to update
// independently. Entries are intentionally retained: configured lock paths are
// a small, process-lifetime set and deleting an unlocked entry creates races.
var processUpdateLocks sync.Map

func acquireUpdateLock(path string) (func() error, error) {
	value, _ := processUpdateLocks.LoadOrStore(path, &sync.Mutex{})
	processLock := value.(*sync.Mutex)
	if !processLock.TryLock() {
		return nil, ErrUpdateInProgress
	}
	platformLock, err := tryPlatformLock(path)
	if err != nil {
		processLock.Unlock()
		return nil, err
	}
	return func() error {
		err := platformLock.release()
		processLock.Unlock()
		return err
	}, nil
}

type heldPlatformLock interface {
	release() error
}
