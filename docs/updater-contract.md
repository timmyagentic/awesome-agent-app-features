# Updater v1 transaction contract

The updater is a headless standalone-binary transaction, not an update UI or restart manager.

## Interactive flow

```text
Prepare
  latest stable metadata
  -> exact archive and checksum assets
  -> bounded checksum download
  -> pinned SHA-256
  -> opaque Plan

host renders Plan.Release + Plan.ArchiveAsset
  -> explicit authorization

Apply(the same Plan)
  target lock
  -> exact archive download
  -> pinned SHA-256
  -> bounded extraction of one exact binary
  -> staged version probe + unchanged-content check
  -> no-clobber backup + replacement
  -> installed version probe + unchanged-content check
  -> success or rollback
```

`Apply` never calls `Source.LatestStable`. A successful later `Prepare` supersedes older plans. A failed `Apply` leaves its plan retryable after the cause is fixed; a successful plan returns `ErrPlanConsumed` if replayed. A plan from another updater or the zero value returns `ErrInvalidPlan`.

The updater serializes its own operations and same-target installation, but injected `Source`, `VersionVerifier`, and progress callbacks remain host-owned dependencies. They must be safe for the host's concurrency model and must not be reconfigured while an operation is running.

Use `UpdateLatest` only when the caller is already authorized and does not need to show an exact target before execution.

## Progress stages

Available update:

```text
checking
downloading_checksums
available
downloading_archive
checksum_verified
staged_version_verified
installing
installed_version_verified
complete
```

Up-to-date check:

```text
checking
up_to_date
```

The callback is synchronous. `Product` and `CurrentVersion` are populated on every event; `TargetVersion`, `Asset`, and `Bytes` are populated when meaningful. Hosts own copy, cards, logs, and progress UI and must tolerate new stage values in a compatible minor release.

## Stable release policy

- Candidate tags must match exact `v?X.Y.Z` without leading-zero components, prerelease, or build metadata.
- Draft and prerelease releases are rejected even if the tag looks stable.
- The current version may contain a valid prerelease or build suffix; a stable release supersedes the same-version prerelease.
- Archive and checksum asset names must be unique exact matches on the selected release.

The built-in `updater/github.Source` refuses API redirects, validates public GitHub release and asset paths, bounds release metadata, applies 15-second metadata and 5-minute download defaults, and permits asset redirects only to HTTPS GitHub release hosts (or explicitly configured enterprise hosts).

## Filesystem and recovery

- The standalone executable and its derived lock path must not be symlinks.
- Downloads and staging occur in the executable directory for same-filesystem replacement.
- The target's permissions are copied to the staged binary; content hashes before and after each version probe must remain identical.
- A hard-link backup is created with no-clobber semantics; an existing `.update-backup` stops the transaction.
- Installed-version failure removes the new file and restores the backup.
- A retained backup path in `Result` means recovery evidence still exists after either a successful installation or a failed rollback; the host should surface an operator action even when `Apply` also returned an error.
- Operations for one updater or executable target serialize. Unrelated target paths remain independent; hosts cannot choose an alternate lock path to bypass this boundary.

macOS and Linux are the supported in-place targets. Package managers and Windows require separate host adapters with their own verification and recovery truth.

## Trust boundary

SHA-256 detects content changes between `Prepare` and `Apply`; it is not a publisher signature. The staged version probe executes code from the verified archive. Only use a trusted release source, and add independently rooted signature or provenance verification for higher-assurance products.
