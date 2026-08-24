# Coding-agent remote integration protocol

[中文](agent-integration.md)

This is the remote construction contract for an integration agent. The user stays in the target project and does not clone or maintain a working copy of `awesome-agent-app-features`.

## Single entry

Public discovery URL:

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`features/index.json` is the only machine-readable entry. `main` helps discover the repository; it is not a dependency version. The agent resolves and pins one full commit SHA before fetching any feature resource.

## Resolution protocol

The agent performs these steps internally from the target project:

1. Resolve `main` to a 40-character commit SHA through the GitHub Commit API.
2. Require the `CI` workflow for that SHA to be `completed/success`.
3. Refetch `features/index.json` from that SHA instead of trusting floating discovery content.
4. Select a feature and fetch its manifest, README, schemas, and delivery items from the same SHA.
5. Stop if any resource drifts to another SHA, CI is not successful, or a path is not declared by the entry.

Reference GitHub CLI commands:

```bash
foundation_repository="timmyagentic/awesome-agent-app-features"
resolved_commit="$(gh api "repos/${foundation_repository}/commits/main" --jq .sha)"

gh run list \
  --repo "${foundation_repository}" \
  --commit "${resolved_commit}" \
  --workflow CI \
  --limit 20 \
  --json headSha,status,conclusion \
  | jq -e --arg commit "${resolved_commit}" \
      'any(.[]; .headSha == $commit and .status == "completed" and .conclusion == "success")'

curl --fail --silent --show-error --location --proto '=https' \
  "https://raw.githubusercontent.com/${foundation_repository}/${resolved_commit}/features/index.json"
```

An equivalent GitHub HTTPS API route is acceptable when `gh` is unavailable. Mixing files read from floating `main` at different times is not.

## Map the host before editing

Fetch `features/integration-plan.schema.json` from the same SHA and produce a reviewable plan. Keep it in the conversation unless the host explicitly wants a persistent audit file.

The plan records:

- Resolved commit, successful CI run URL, feature, contract, and delivery mode.
- Host runtime, existing UI/commands/configuration, install kinds, and lifecycle.
- The location of every foundation, adapter, and host responsibility.
- Evidence that each invariant is `preserved`, `not-applicable`, or `blocked`.
- Target files, focused tests, full verification, and `UNVERIFIED` boundaries.

`integration-plan.example.json` demonstrates structure only; it does not contain reusable host decisions.

## Delivery modes

### `go-module`

Run in the target project:

```bash
go get "github.com/timmyagentic/awesome-agent-app-features@${resolved_commit}"
GOWORK=off go test github.com/timmyagentic/awesome-agent-app-features/compat/v1
```

Go stores the module in its cache and records an immutable pseudo-version in the target `go.mod`. It does not create a foundation working copy next to the target.

### `source-subtree`

Download the same-SHA GitHub archive or use the Contents API in a temporary directory. Extract only the path declared by the manifest, such as `relay/cloudflare`, into host-owned infrastructure.

The agent must inspect archive entries, reject traversal and symlinks, avoid copying the whole foundation, remove temporary material, and leave deployment and credentials under host ownership. Without explicit production authorization, run tests and dry-run only.

## Remote proof before host changes

```bash
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/feedback@${resolved_commit}"
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@${resolved_commit}"
```

The Feedback example only renders a preview. The Updater example runs a full transaction against a temporary fake executable, with no Release request or installed-product mutation.

## Feedback host inventory

Record the trigger, complete preview surface, explicit approval action, product-specific redaction, trusted environment sources, optional install identity, relay configuration, and public fallback.

Never capture arbitrary environment maps, transcripts, reasoning, tool payloads, raw logs, user/chat IDs, or credentials. The host may render with cards, text, CLI, or web UI, but every `Report` field must be visible and only a confirmation callback may call `Approve(true)`.

Host tests cover cancellation with zero requests, preview/outbound equivalence, default and product redaction, stale-error filtering, exact v1 endpoint behavior, secret-safe errors, and fallback.

## Updater host inventory

Record current-version truth, strict non-mutating version output, exact archive/checksum and archive-entry naming, executable/install kind, every update entry point, authorization, restart, acknowledgement, and separate beta/nightly policy.

All entry points share one updater configuration. Interactive paths run `Prepare -> render exact Plan -> authorize -> Apply(plan)`. Only already-authorized non-interactive paths may call `UpdateLatest`.

Host tests cover prompt/apply identity, prerelease and asset refusal before mutation, checksum and staged mismatch, installed mismatch rollback, concurrency errors, install-kind routing, restart, and acknowledgement.

Package-manager installation requires a separate host adapter with honest stable selection, post-install version truth, and recovery. Standalone guarantees do not transfer to npm, Homebrew, or Windows.

## Completion

The agent reports completion only when:

- Dependency and every fetched resource use the same commit SHA.
- Every mapping and invariant has evidence or a named blocker.
- Applicable `verification.remote` and `verification.host` steps passed.
- The target project's normal complete verification passed.
- Unavailable client, credential, deployment, restart, paid, or production checks are marked `UNVERIFIED`.

A user-managed clone, local `replace`, floating `main`, temporary output, or foundation-only test run is not host verification.
