# Awesome Agent App Features

[中文](README.md) · [Agent integration](docs/agent-integration.en.md) · [Compatibility](COMPATIBILITY.md) · [Security](docs/security.md)

Headless feature-foundation code for agent applications and an agent-driven feature integrator. The user stays in the target project and tells a coding agent which feature to integrate, inspect, refine, upgrade, or remove. The agent resolves the official remote entry to one commit SHA, adds only the declared dependency or template, and continuously maintains the host-specific adapter. Cards, commands, authorization, localization, restart behavior, and product flow remain in the host.

This is not an awesome list, Codex Skills collection, UI framework, or hosted SaaS. The currently unreleased `v1` contract stabilizes three integration outcomes:

| Capability | This repository provides | The host product provides |
| --- | --- | --- |
| Agent-friendly integration | Remote entry, typed actions, manifests, host plan/receipt schemas, examples, API/consumer contracts, and verification commands | Glue code fitted to the existing architecture, durable receipt, and host tests |
| Feedback | Redacted `Draft`, non-serializable preview, explicit `Approved`, Feedback v1 HTTPS client, single-tenant Cloudflare relay | Trigger timing, Feishu/Slack/CLI/web rendering, confirmation, and failure UX |
| Updater | Immutable exact plan, stable-only selection, same-release checksum, two version checks, locks, no-clobber backup, and rollback | Update prompt, authorization, install-kind detection, restart, and acknowledgement |

## No-clone integration

The canonical machine-readable entry is [features/index.json](features/index.json). Its public discovery URL is:

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`main` is discovery-only. The agent must resolve it to a full commit SHA, require successful `CI` for that commit, then refetch the entry, feature manifest, documentation, and source subtree from that same SHA. The user does not clone this repository, and the target project must not use a Git submodule, local `replace`, or floating `main`.

Give the agent this instruction from inside the target project:

```text
Integrate the feedback (or updater) feature from awesome-agent-app-features
into the current project. Official entry:
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json

Do not ask me to clone the repository. Resolve main to a full commit SHA and
require successful CI for that SHA. Pin the entry, manifest, documentation,
dependency, and template to the same SHA. First map the host using the
integration-plan schema, then implement the thin adapter and tests. Preserve
every invariant and mark unavailable client, credential, deployment, or
restart checks as UNVERIFIED. Write the sanitized integration record to
.agent-app-features/<feature>.json.
```

No version has been published yet. To evaluate the current v1 contract, use Go 1.25 or newer and pin an exact reviewed commit SHA rather than floating `main`:

```bash
go get github.com/timmyagentic/awesome-agent-app-features@<agent-resolved-commit-sha>
```

The agent runs this inside the target project. Go downloads the module into its cache; it does not create a working copy of this repository. Before changing the host, the agent can run both zero-configuration examples remotely:

```bash
go run github.com/timmyagentic/awesome-agent-app-features/examples/feedback@<agent-resolved-commit-sha>
go run github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@<agent-resolved-commit-sha>
```

The updater demo replaces only a fake executable in a temporary directory. It does not contact GitHub Releases or touch an installed product. See the [agent integration guide](docs/agent-integration.en.md) and [integration plan schema](features/integration-plan.schema.json) for the complete protocol.

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
submissionReceipt, err := (httpclient.Client{
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

## Agent integration protocol

The agent uses [features/index.json](features/index.json) as the only entry and fetches the selected `features/<id>/feature.json`, README, and delivery items from the same commit SHA. It first maps the host's UI, authorization, version truth, release assets, install kinds, and lifecycle with [integration-plan.schema.json](features/integration-plan.schema.json), then changes the target project.

A `go-module` delivery adds the exact SHA as a dependency. A `source-subtree` delivery extracts only the declared directory from the same-SHA GitHub archive or Contents API into host infrastructure. The agent may use temporary downloads or language package caches internally, but the user never manages a second checkout of this repository.

After a complete or partial integration, the agent writes `.agent-app-features/<feature>.json` against [integration-receipt.schema.json](features/integration-receipt.schema.json). The receipt records exact source/CI, selected delivery, host entry points and configuration key names, integration files, every invariant, verification evidence, `UNVERIFIED` boundaries, and history. It never stores configuration values, credentials, payloads, logs, user IDs, or absolute paths.

## Lifecycle

The remote entry defines seven typed Agent actions:

| Action | Purpose | Mutation scope |
| --- | --- | --- |
| `integrate` | First integration or re-integration from a removed tombstone | Host + receipt |
| `inspect` | Compare receipt, dependencies, files, and wiring for drift | Read-only |
| `validate` | Rerun exact-source remote/host checks and refresh evidence | Receipt only |
| `refine` | Close host UX, mapping, and test gaps at the same source commit | Host + receipt |
| `upgrade` | Compare contracts and move every delivery to one new SHA | Host + receipt |
| `remove` | Remove only unshared integration-managed artifacts | Host + removed receipt |
| `list` | Inventory active, partial, and removed local receipts | Read-only |

An `active` receipt cannot contain a blocked invariant and must include successful remote and host verification. Work with blockers remains `partial`. Removal retains a `state: removed` audit tombstone. See the complete [host receipt example](examples/host-receipt/).

A host Feishu card is only a renderer for `Report`, `Plan`, or `Event`; it does not belong in this repository. See [docs/architecture.md](docs/architecture.md) for the ownership model.

## Security boundaries

- Resolve the remote entry to a full commit SHA; entry, manifest, dependency, examples, and templates must use that same CI-successful SHA.
- Receipts contain only relative paths, configuration key names, and sanitized evidence—never values, credentials, identifiers, payloads, or raw logs.
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
features/index.json           single no-clone remote agent entry
features/*.schema.json        entry, feature, host-plan, and receipt contracts
features/*/feature.json       agent-readable integration manifests
examples/host-receipt/        target .agent-app-features receipt example
feedback/                     provider-neutral Feedback core
feedback/httpclient/          Feedback v1 HTTPS adapter
protocol/feedback/v1/         JSON Schema and shared Go/JS fixtures
updater/                      stable standalone transaction core
updater/github/               GitHub Releases source adapter
relay/cloudflare/             runnable single-tenant GitHub Issues relay
examples/updater-demo/        offline transaction against temporary files
```

```bash
make verify
```

Once published, `v1` follows SemVer. Rules for public APIs, wire protocols, and manifest invariants are in [COMPATIBILITY.md](COMPATIBILITY.md).

This project was extracted from the Feedback and unified updater paths in [CC Connect Next](https://github.com/timmyagentic/cc-connect-next), retaining only the reusable foundation. MIT License.
