# Updater v1 integration contract

Agents discover this contract through `features/index.json`, resolve one CI-successful commit SHA, and fetch this README, `feature.json`, and the Go module from that same SHA. The user does not clone the foundation.

Run the complete updater transaction remotely without a Release repository:

```bash
GOWORK=off go run github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@<resolved-commit-sha>
```

The demo mutates only a temporary fake executable. Read [the full transaction contract](../../docs/updater-contract.md) before host implementation.

Interactive entry points must use:

```text
Prepare -> render exact Plan -> authorize -> Apply(the same Plan)
```

`Prepare` validates one exact stable release, selects the archive/checksum and archive entry, downloads the bounded checksum manifest, and pins SHA-256. `Apply` never queries latest. A new successful `Prepare` supersedes older plans; a failed Apply can retry the same plan, while a successful one is consumed.

`updater/github.Source` is an infrastructure adapter, not part of the core. A custom source must return internally consistent release metadata and download only the supplied exact `Asset`.

Static archives use `BinaryName`. Archives whose entry changes by tag/OS/arch use `ArchiveBinaryName`. Neither option can bypass checksum, bounded extraction, staged verification, installed verification, locking, backup, or rollback.

All host entry points share one updater configuration. They may render events differently but cannot duplicate stable selection or replacement policy. `UpdateLatest` is for an already-authorized non-interactive action. Restart and acknowledgement always remain host-owned.

The standalone adapter supports macOS/Linux regular executable paths. npm, Homebrew, symlink installations, and Windows require explicit install-kind adapters with honest verification and recovery behavior.

After integration, record the exact source, module version, host-relative files, successful checks, and `UNVERIFIED` boundaries in the target's visible `agent-app-features.lock.json`. Future agents combine that locator with current references, Git history, and host tests; the lock does not own shared files or recovery material.
