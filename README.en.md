# Awesome Agent App Features

[中文](README.md) · [Agent integration](docs/agent-integration.en.md) · [Compatibility](COMPATIBILITY.md) · [Security](docs/security.md)

Headless feature-foundation code for agent applications. The user stays in the target project while a coding agent integrates low-level capabilities from remote contracts. Cards, commands, authorization, localization, install kinds, and product flow remain host-owned.

This is not an awesome list, Codex Skills collection, UI framework, or hosted SaaS. The currently unreleased `v1` covers three capabilities:

| Capability | This repository provides | The host provides |
| --- | --- | --- |
| Agent-friendly integration | Remote entry, manifests, exact-SHA delivery, minimal lock, contract tests | Thin adapters fitted to the existing architecture and host tests |
| Feedback | Redacted Draft, explicit Approved, HTTPS client, single-tenant Relay | Trigger, preview UI, confirmation, and failure UX |
| Updater | Exact plan, stable-only selection, checksum, two version checks, locks, backup, rollback | Prompt, authorization, install-kind routing, restart, acknowledgement |

## No-clone integration

The single machine-readable entry is [features/index.json](features/index.json):

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`main` is discovery-only. The agent resolves a full commit SHA, requires successful `CI` for that commit, then fetches the entry, manifest, documentation, dependency, and source subtree from the same SHA. The target must not use a Git submodule, local `replace`, or floating `main`.

Give the agent this instruction from inside the target project:

```text
Integrate the feedback (or updater) feature from awesome-agent-app-features
into the current project. Official entry:
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json

Do not ask me to clone the repository. Resolve main to a full commit SHA and
require successful CI for that SHA. Pin every resource to the same SHA. First
inventory the host and map feature responsibilities, invariants, and checks;
then implement the thinnest adapter. Mark unavailable client, credential,
deployment, or restart checks UNVERIFIED. Finally write a secret-free
agent-app-features.lock.json.
```

No version has been published. Evaluate the current code with Go 1.25 or newer and an exact reviewed commit SHA:

```bash
go get github.com/timmyagentic/awesome-agent-app-features@<resolved-commit-sha>
go run github.com/timmyagentic/awesome-agent-app-features/examples/feedback@<resolved-commit-sha>
go run github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@<resolved-commit-sha>
```

Go uses its module cache and creates no working copy of this repository. The Updater demo replaces only a fake executable in a temporary directory; it contacts no Release and touches no installed product.

The public Go packages are:

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
_, err = (httpclient.Client{Endpoint: feedbackEndpoint}).Submit(ctx, approved)
```

`Report` is a deep copy and cannot be JSON-marshaled; only opaque `Approved` emits Feedback v1 JSON. The core creates no issue title, Markdown, card, or copy. The reference Relay fixes the GitHub repository and token server-side. See [protocol/feedback/v1](protocol/feedback/v1) for the wire contract.

## Updater

```go
service, err := updater.New(updater.Config{
    Product:        "my-agent-app",
    CurrentVersion: currentVersion,
    ExecutablePath: executablePath,
    BinaryName:     "my-agent-app",
    AssetName:      updater.ReleaseArchiveName("my-agent-app"),
    Source: updatergithub.Source{Repository: "owner/my-agent-app"},
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
_, err = service.Apply(ctx, plan)
```

`Prepare` pins the release, archive, archive binary name, and checksum. `Apply` executes only that plan and never resolves latest again. Standalone in-place replacement supports macOS/Linux only; npm, Homebrew, Windows, and other install kinds require host adapters. See the [Updater contract](docs/updater-contract.md).

## Integration record

After integration, the agent maintains one visible `agent-app-features.lock.json` in the target root. [integration-lock.schema.json](features/integration-lock.schema.json) records only:

- this repository and the full commit SHA;
- feature, contract, and actual deliveries;
- relative files changed by the agent;
- checks run, verification time, and `UNVERIFIED` boundaries.

It stores no configuration values, credentials, payloads, logs, user IDs, absolute paths, runtime state, or removal history. Update it during upgrades; combine it with current code and Git history when inspecting or removing an integration. The lock is a maintenance clue, not a substitute for host tests or deployment authorization.

## Boundaries and gates

- A Feishu card or other product UI is a host renderer for `Report`, `Plan`, or `Event`; it never belongs here.
- Feedback is never submitted in the background; the Relay validates schema, approval, and byte limits again.
- Updater accepts stable tags only, pins checksum before confirmation, and refuses or rolls back version mismatches.
- Core packages use only the Go standard library. Infrastructure adapters depend inward; cores do not depend outward.
- Source-subtree delivery extracts only a declared directory and rejects traversal and symlinks.

```text
api/v1.txt                         public API snapshot
compat/v1                          external-consumer contract
features/index.json                single remote Agent entry
features/integration-lock.schema.json  minimal host lock
features/*/feature.json            Feature manifests
feedback/                          Feedback core
updater/                           Updater core
relay/cloudflare/                  single-tenant GitHub Issues Relay
```

```bash
make verify
```

See [architecture](docs/architecture.md) for ownership details. This project extracts only the reusable Feedback and unified updater foundation from [CC Connect Next](https://github.com/timmyagentic/cc-connect-next). MIT License.
