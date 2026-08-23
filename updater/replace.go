package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// replaceExecutable returns a retained backup path only when installation
// succeeded but cleanup of the verified backup failed.
func replaceExecutable(target, staged string, verify func(string) error) (string, error) {
	if err := preflightReplacement(target); err != nil {
		return "", err
	}
	if filepath.Clean(filepath.Dir(target)) != filepath.Clean(filepath.Dir(staged)) {
		return "", fmt.Errorf("staged binary must be on the target filesystem")
	}
	if err := prepareStagedExecutable(target, staged); err != nil {
		return "", err
	}

	backup := target + ".update-backup"
	if err := os.Rename(target, backup); err != nil {
		return "", fmt.Errorf("create update backup: %w", err)
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return "", rollbackExecutable(target, backup, "sync update backup", err)
	}
	if err := os.Rename(staged, target); err != nil {
		return "", rollbackExecutable(target, backup, "install staged binary", err)
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return "", rollbackExecutable(target, backup, "sync installed binary", err)
	}
	if verify != nil {
		if err := verify(target); err != nil {
			return "", rollbackExecutable(target, backup, "verify installed binary", err)
		}
	}
	if err := os.Remove(backup); err != nil {
		return backup, nil
	}
	// The new executable is already verified. A directory-sync failure while
	// persisting backup cleanup must not be reported as a failed installation.
	_ = syncDirectory(filepath.Dir(target))
	return "", nil
}

func preflightReplacement(target string) error {
	targetInfo, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("inspect current executable: %w", err)
	}
	if !targetInfo.Mode().IsRegular() {
		return fmt.Errorf("current executable is not a regular file")
	}
	backup := target + ".update-backup"
	if _, err := os.Lstat(backup); err == nil {
		return fmt.Errorf("refusing to overwrite existing update backup %s", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect update backup: %w", err)
	}
	return nil
}

func prepareStagedExecutable(target, staged string) error {
	targetInfo, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("inspect current executable mode: %w", err)
	}
	mode := targetInfo.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}
	if err := os.Chmod(staged, mode); err != nil {
		return fmt.Errorf("apply executable mode to staged binary: %w", err)
	}
	return nil
}

func rollbackExecutable(target, backup, action string, updateErr error) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w; rollback failed to remove new binary: %v; backup remains at %s", action, updateErr, err, backup)
	}
	if err := os.Rename(backup, target); err != nil {
		return fmt.Errorf("%s: %w; rollback failed: %v; backup remains at %s", action, updateErr, err, backup)
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return fmt.Errorf("%s: %w; rollback restored the executable but directory sync failed: %v", action, updateErr, err)
	}
	return fmt.Errorf("%s: %w (rolled back)", action, updateErr)
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
