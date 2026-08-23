# Awesome Agent App Features

[中文](README.md) · [Agent integration](docs/agent-integration.md) · [Compatibility](COMPATIBILITY.md) · [Security](docs/security.md)

Headless feature-foundation code for agent applications. Give this repository and the target project to a coding agent: it first understands the host architecture, then integrates the reliable low-level capability. Cards, commands, authorization, localization, restart behavior, and product flow remain in the host.

This is not an awesome list, Codex Skills collection, UI framework, or hosted SaaS. The currently unreleased `v1` contract stabilizes three integration outcomes:

| Capability | This repository provides | The host product provides |
| --- | --- | --- |
| Agent-friendly integration | Machine-readable manifests, boundaries, examples, API/consumer contracts, and verification commands | Glue code fitted to the existing architecture plus host tests |
| Feedback | Redacted `Draft`, non-serializable preview, explicit `Approved`, Feedback v1 HTTPS client, single-tenant Cloudflare relay | Trigger timing, Feishu/Slack/CLI/web rendering, confirmation, and failure UX |
| Updater | Immutable exact plan, stable-only selection, same-release checksum, two version checks, locks, no-clobber backup, and rollback | Update prompt, authorization, install-kind detection, restart, and acknowledgement |

## Current integration status

No version has been published yet. To evaluate the current v1 contract, use Go 1.25 or newer and pin an exact reviewed commit SHA rather than floating `main`:

```bash
go get github.com/timmyagentic/awesome-agent-app-features@FULL_REVIEWED_COMMIT_SHA
```

The supported v1 packages are:

```text
feedback
feedback/httpclient
updater
updater/github
```

## Feedback

```go
draft, err := (feedback.Builder{}).Build(feedback.Input{
    Description: "Startup failed; doctor should explain why",
    Environment: feedback.Environment{
        Product: "my-agent-app",
        Version: "v1.4.0",
        Agent:   "codex",
    },
})
if err != nil {
    return err
}

renderEveryField(draft.Report()) // host-owned card, text, CLI, or web UI
if !userExplicitlyConfirmed() {
    return nil
}

approved, err := draft.Approve(true)
if err != nil {
    return err
}
receipt, err := (httpclient.Client{
    Endpoint: "https://feedback.example/v1/feedback",
}).Submit(ctx, approved)
```

`Report` is a deep copy and standard JSON encoding returns `ErrApprovalRequired`; only opaque `Approved` emits a schema 1 wire payload. The core creates no issue title, Markdown body, card, or copy. The reference relay fixes the GitHub repository and token server-side, so a client cannot choose the destination. Shared protocol fixtures live in [protocol/feedback/v1](protocol/feedback/v1).

## Updater

```go
service, err := updater.New(updater.Config{
    Product:        "my-agent-app",
    CurrentVersion: currentVersion,
    ExecutablePath: executablePath,
    BinaryName:     "my-agent-app",
    AssetName:      updater.ReleaseArchiveName("my-agent-app"),
    Source: updatergithub.Source{
        Repository: "owner/my-agent-app",
    },
    Verifier: updater.ExactVersionLine("my-agent-app"),
    Progress: renderProgress,
})
if err != nil {
    return err
}

plan, err := service.Prepare(ctx)
if err != nil || !plan.Available() {
    return err
}
renderExactUpdate(plan.Release(), plan.ArchiveAsset())
if !userExplicitlyConfirmed() {
    return nil
}
result, err := service.Apply(ctx, plan)
```

Import both `updater` and `updater/github`. `Prepare` pins the release, archive, archive binary name, and checksum. `Apply` executes only that plan and never resolves latest again. A later successful `Prepare` makes an older plan return `ErrPlanSuperseded`. An already-authorized, non-interactive CLI or administrator action may use `UpdateLatest`.

The default archive name is `<product>-<tag>-<os>-<arch>.tar.gz` (zip on Windows), with `checksums.txt` as the default manifest. Use `ArchiveBinaryName` when the binary inside the archive changes by tag or platform. In-place standalone replacement is supported on macOS/Linux and rejects symlink executables. npm, Homebrew, and Windows require host-owned install adapters.

## Prompt for a coding agent

```text
No version is published yet. Pin a reviewed full commit SHA; do not use main,
a local replace, or another floating reference. Move to the immutable v1 tag
only after it is formally published.
Read features/<feature>/feature.json, its README, and
docs/agent-integration.md. Inventory my existing UI, authorization, version
truth, release assets, install kind, and restart lifecycle before editing.
Reuse the foundation core/adapters, implement presentation and product flow in
the host, preserve every invariant, and run both verification suites.
```

A host Feishu card is only a renderer for `Report`, `Plan`, or `Event`; it does not belong in this repository. See [docs/architecture.md](docs/architecture.md) for the ownership model.

## Security boundaries

- Feedback is never sent in the background; `Approved` must follow an explicit user action.
- Default redaction, fixed environment allowlists, and UTF-8 byte limits are enforced in both Go and the relay.
- Ordinary updates accept only exact `v?X.Y.Z` stable tags; drafts, prereleases, and leading-zero versions fail closed.
- The checksum is pinned during `Prepare`; the archive is verified before extraction, execution, or replacement.
- Both staged and installed binaries must report the exact target version, otherwise mutation is refused or rolled back.
- Lock-file symlinks, executable symlinks, and existing recovery backups fail closed.
- A checksum proves content consistency, not publisher identity; high-risk products still need an independent signature or provenance root.

## Layout and gates

```text
api/v1.txt                    v1 public API snapshot
compat/v1                     external-consumer compile contract
features/*                    agent-readable integration manifests
feedback/                     provider-neutral Feedback core
feedback/httpclient/          Feedback v1 HTTPS adapter
protocol/feedback/v1/         JSON Schema and shared Go/JS fixtures
updater/                      stable standalone transaction core
updater/github/               GitHub Releases source adapter
relay/cloudflare/             runnable single-tenant GitHub Issues relay
```

```bash
make verify
```

Once published, `v1` follows SemVer. Rules for public APIs, wire protocols, and manifest invariants are in [COMPATIBILITY.md](COMPATIBILITY.md).

This project was extracted from the Feedback and unified updater paths in [CC Connect Next](https://github.com/timmyagentic/cc-connect-next), retaining only the reusable foundation. MIT License.
