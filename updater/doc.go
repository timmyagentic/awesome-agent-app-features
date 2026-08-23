// Package updater installs exact stable GitHub Release binaries through one
// UI-independent transaction: release validation, asset selection, checksum
// verification, staged version verification, atomic replacement, installed
// version verification, and rollback.
//
// The MVP targets standalone command-line binaries on macOS and Linux. Host
// applications own update prompts, authorization, progress copy, restart
// behavior, and channel policy outside the stable path.
package updater
