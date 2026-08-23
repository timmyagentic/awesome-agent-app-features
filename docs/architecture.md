# Architecture

The project separates three layers so products can vary without duplicating
security policy.

```text
host UI / chat / CLI
        |
        v
thin product adapter
        |
        +---- feedback core ---- HTTPS ---- self-hosted relay ---- GitHub Issues
        |
        +---- updater core ----- GitHub Release assets ----- executable
```

## Feedback boundary

The host supplies a fixed allowlist of environment facts plus optional user
description, fresh error, and capability gaps. `Builder` redacts and bounds the
report. The host renders the complete draft and records a user decision.
`Client` accepts only the opaque `Approved` type.

The relay is intentionally single tenant. Its GitHub token and repository live
server-side. The wire payload cannot select a repository. GitHub search-based
deduplication is best effort and not part of submission correctness.

## Updater boundary

`Source` owns release metadata and asset bytes. `Updater` owns the complete
installation transaction. UI adapters receive progress events only.

The selected release is revalidated as an exact stable version. Archive and
checksum names are looked up exactly on that same release object. The updater
downloads with size bounds, verifies SHA-256, extracts only one configured
binary into the target directory, verifies it, backs up the installed binary,
renames the staged binary into place, verifies again, and rolls back on failure.

An in-process gate plus a target lock serializes callers. Unix builds use an
advisory file lock, which is automatically released after process death. The
non-Unix fallback uses an exclusive lock file; Windows replacement is not an
MVP guarantee.
