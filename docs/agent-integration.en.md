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
- Expected receipt path `.agent-app-features/<feature>.json`.

`integration-plan.example.json` demonstrates structure only; it does not contain reusable host decisions.

## Persistent receipt

After code and verification, fetch `features/integration-receipt.schema.json` from the same SHA and write the result to the receipt path declared by the plan. See the complete [host receipt example](../examples/host-receipt/).

The receipt is the maintenance entry for future agents, not runtime configuration. It:

- Records exact source commit, successful CI run, entry, and feature manifest.
- Records selected delivery, resolved module version, and host target.
- Uses only host-relative paths, entry-point names, and configuration key names—not values.
- Copies every manifest invariant with status and sanitized evidence.
- Separates remote, foundation, host, and production verification; failed is never passed.
- Records `UNVERIFIED`, action history, and removal evidence.
- Contains no tokens, cookies, user/chat IDs, payloads, raw logs, absolute paths, or copied source.

States:

- `partial`: implementation or evidence exists, but an invariant is blocked or required proof is missing/failed.
- `active`: no invariant is blocked and remote plus host verification passed.
- `removed`: integration is gone; the receipt remains as a tombstone with removed and retained paths.

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

## Typed lifecycle actions

Actions come from exact-SHA `features/index.json`; they are not Skills, shell installers, or platform plugins.

### `integrate`

Requires an absent or removed receipt. Resolve source, create the plan, apply declared delivery, implement host mappings, verify, then write an `active` or honest `partial` receipt.

### `inspect`

Read-only comparison of the receipt with dependency, source SHA, delivery targets, artifacts, host entry points, invariants, verification, and `UNVERIFIED`. It performs no production call, repair, or receipt mutation.

### `validate`

Keep the receipt's source commit fixed and rerun applicable remote and host checks. Update only sanitized evidence, invariant status, `UNVERIFIED`, timestamps, and history.

### `refine`

Keep the source commit unchanged while closing host mapping, UX, fallback, recovery, and test gaps. Record every changed artifact, rerun verification, and append history.

### `upgrade`

Resolve a new CI-successful commit and compare contract, public API, wire schema, delivery, and invariants before mutation. Move every selected delivery to the same new SHA; mixed-source upgrades are forbidden. Retain the prior SHA in history.

### `remove`

Disable product entry points first. Delete only unshared `integration-managed/candidate` artifacts. Retain shared files, configuration containers, and dependencies still used elsewhere. Verify the host and keep a `removed` receipt tombstone.

### `list`

Read local `.agent-app-features/*.json` and report feature, state, contract, source commit, last action, and drift without network access.

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
- `.agent-app-features/<feature>.json` passes the exact-source receipt schema and matches the manifest, actual delivery, and host files.

A user-managed clone, local `replace`, floating `main`, temporary output, historical receipt, or foundation-only test run is not current host verification.
