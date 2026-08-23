// Package updater installs exact stable standalone binaries through a
// UI-independent Prepare/Apply transaction: immutable release and checksum
// planning, bounded download, staged version verification, no-clobber backup,
// replacement, installed version verification, and rollback.
//
// The v1 implementation targets standalone command-line binaries on macOS and
// Linux. Host applications own update prompts, authorization, progress copy,
// restart behavior, and channel policy outside the stable path.
package updater
