# Awesome Agent App Features

[中文](README.md) · [Agent integration guide](docs/agent-integration.md) · [Security model](docs/security.md)

Reusable product-feature code for agent applications. Give this repository to a coding agent and it can adapt proven Feedback and safe-update mechanisms to your codebase.

This is not an awesome link list or a Codex Skills collection. It ships runnable packages, explicit security invariants, machine-readable feature manifests, integration steps, and regression tests.

## MVP contents

| Feature | Problem solved | Current implementation |
| --- | --- | --- |
| Feedback | Submit redacted in-product feedback without shipping a GitHub token to clients | Go draft/redaction/approval/client package plus a single-tenant Cloudflare GitHub relay |
| Updater | Give chat, CLI, and UI adapters one trusted stable update transaction | Go standalone updater with stable-only selection, same-release checksums, two version checks, cross-process locking, and rollback |

The MVP targets Go-based agent CLIs and daemons. In-place replacement is currently promised only on macOS and Linux. The host still owns cards, buttons, natural-language intent, administrator gates, localization, restart behavior, and final UI.

## Fastest path: give it to an agent

```text
Read https://github.com/timmyagentic/awesome-agent-app-features .
Inspect my existing architecture first, then integrate Feedback and Updater by
following features/feedback/feature.json and features/updater/feature.json.
Preserve every invariant, adapt the UI, permissions, version output, and release
asset naming to my project, and run each feature's verification commands.
```

The agent should inventory the existing feedback entry points, version source, release assets, authorization, and restart lifecycle before editing. See [docs/agent-integration.md](docs/agent-integration.md).

## Feedback core

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

showToUser(draft.Preview()) // or an equivalent complete rendering
if !userClickedSubmit() {
    return nil
}

approved, err := draft.Approve(true)
if err != nil {
    return err
}
receipt, err := (feedback.Client{
    Endpoint: "https://your-relay.example/v1/feedback",
}).Submit(ctx, approved)
```

Only `Approved` values can enter the client submission API, and the relay independently requires `user_approved: true`. Automatically captured environment fields are a fixed allowlist, never an arbitrary environment map. See [relay/cloudflare](relay/cloudflare/README.md).

## Updater core

```go
service, err := updater.New(updater.Config{
    Product:        "my-agent-app",
    CurrentVersion: currentVersion,
    ExecutablePath: executablePath,
    BinaryName:     "my-agent-app",
    AssetName:      updater.ReleaseArchiveName("my-agent-app"),
    Source: updater.GitHubSource{
        Repository: "owner/my-agent-app",
    },
    Verifier: updater.ExactVersionLine("my-agent-app"),
    Progress: renderProgress,
})
if err != nil {
    return err
}

result, err := service.Update(ctx)
```

Default release asset convention:

```text
my-agent-app-v1.2.3-darwin-arm64.tar.gz
my-agent-app-v1.2.3-linux-amd64.tar.gz
my-agent-app-v1.2.3-windows-amd64.zip
checksums.txt
```

You may replace `AssetName` and `VersionVerifier`, but not this order: exact stable release → exact assets on that release → SHA-256 → staged version → backup/replace → installed version → rollback on failure.

## Repository layout

```text
feedback/               Go Feedback core
updater/                Go stable-only standalone updater
relay/cloudflare/       Self-hosted single-tenant GitHub issue relay
features/*/feature.json Agent-readable integration contracts
examples/               Minimal compiling integrations
docs/                   Architecture, security, and agent guidance
```

## Security boundaries

- Clients never hold the GitHub token or choose the relay's destination repository.
- Feedback has one shape; errors and capability gaps are user-approved context.
- Ordinary updates never select beta/rc releases; tags must exactly match `v?X.Y.Z`.
- The checksum manifest and archive must be exact assets on the selected Release.
- A checksum protects download integrity, not release-publisher identity. High-risk products should extend `Source` or their release pipeline with signatures/provenance.
- An existing `.update-backup` is never overwritten.

See [docs/security.md](docs/security.md) for the full threat model.

## Current boundary

This MVP does not include npm rollback, a multi-tenant feedback SaaS, dashboard, accounts, guaranteed Windows in-place replacement, package publication, or a hosted relay. No Go module tag is published yet; pin an audited commit when integrating.

The standalone updater refuses symlink executable paths. npm, Homebrew, and other package-manager installs need explicit install-kind detection plus a dedicated host adapter.

## Verify

```bash
gofmt -l .
go test -race ./...
go vet ./...
npm test --prefix relay/cloudflare
npm run check --prefix relay/cloudflare
npm run validate:worker --prefix relay/cloudflare
```

The project was extracted from the proven Feedback and unified updater paths in [CC Connect Next](https://github.com/timmyagentic/cc-connect-next), with Feishu, chat-command, CLI-copy, and repository-specific release policy removed.

MIT License
