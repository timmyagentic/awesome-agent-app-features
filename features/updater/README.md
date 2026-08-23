# Updater v1 integration contract

Read `feature.json` and [the full transaction contract](../../docs/updater-contract.md) first.

Interactive entry points must use:

```text
Prepare -> render exact Plan -> authorize -> Apply(the same Plan)
```

`Prepare` validates one exact stable release, selects the archive/checksum and archive entry, downloads the bounded checksum manifest, and pins SHA-256. `Apply` never queries latest. A new successful `Prepare` supersedes older plans; a failed Apply can retry the same plan, while a successful one is consumed.

`updater/github.Source` is an infrastructure adapter, not part of the core. A custom source must return internally consistent release metadata and download only the supplied exact `Asset`.

Static archives use `BinaryName`. Archives whose entry changes by tag/OS/arch use `ArchiveBinaryName`. Neither option can bypass checksum, bounded extraction, staged verification, installed verification, locking, backup, or rollback.

All host entry points share one updater configuration. They may render events differently but cannot duplicate stable selection or replacement policy. `UpdateLatest` is for an already-authorized non-interactive action. Restart and acknowledgement always remain host-owned.

The standalone adapter supports macOS/Linux regular executable paths. npm, Homebrew, symlink installations, and Windows require explicit install-kind adapters with honest verification and recovery behavior.
