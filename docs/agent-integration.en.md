# Coding-agent remote integration protocol

[中文](agent-integration.md)

This is the implementation protocol for an integrating agent. The user stays in the target project and does not maintain a local copy of `awesome-agent-app-features`.

## 1. Pin the remote source

The single machine-readable entry is `features/index.json`:

```text
https://raw.githubusercontent.com/timmyagentic/awesome-agent-app-features/main/features/index.json
```

`main` is discovery-only. The agent must:

1. Resolve `main` to a 40-character commit SHA through the GitHub API.
2. Require the `CI` workflow for that SHA to be `completed/success`.
3. Refetch the entry and selected Feature manifest, README, schema, and delivery from that SHA.
4. Use the same SHA for every resource, dependency, and template; stop on drift.

Reference commands:

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

An equivalent GitHub HTTPS API is acceptable when `gh` is unavailable. Never mix files read from floating `main` at different times.

## 2. Map the host first

Before editing, write a short plan in the conversation or work tracker containing:

- Feature, contract, resolved commit SHA, and actual delivery;
- foundation, generic-adapter, and host responsibilities;
- existing host entry points, configuration, install kinds, and reusable code;
- the host location of every invariant;
- expected relative files, focused tests, full verification, and `UNVERIFIED` boundaries.

This is a plan for the current codebase, not another generic plan JSON. Cards, commands, authorization, localization, restart, and product flow map into the host. This repository supplies only low-level values, state machines, ports, and infrastructure adapters.

## 3. Apply declared delivery

### `go-module`

Run inside the target project:

```bash
go get "github.com/timmyagentic/awesome-agent-app-features@${resolved_commit}"
GOWORK=off go test github.com/timmyagentic/awesome-agent-app-features/compat/v1
```

Go stores the module in its cache and records an immutable pseudo-version in `go.mod`. Do not use a local `replace`, submodule, or floating `main`.

### `source-subtree`

Download the same-SHA GitHub archive into a temporary directory and extract only the manifest-declared path, such as `relay/cloudflare`. Inspect archive entries first and reject traversal, symlinks, and files outside the declared path. Remove temporary material afterwards. The copied configuration, credentials, deployment, and maintenance become host-owned. Without production authorization, run tests and dry-run only.

A declared `source-subtree` must remain self-contained after it leaves the foundation root. For the Relay, run `npm ci --ignore-scripts`, `npm test`, `npm run check`, `npm run typecheck`, `npm run types:check`, `npm run validate:worker`, and `npm audit --audit-level=high` in the final host target. Schemas, fixtures, or `node_modules` that happen to exist in a foundation checkout are not delivery proof. The manifest's `host_owned_files` explicitly names configuration and derived files that may change after copying; every other delivered target file must byte-match the same-SHA source subtree. Extra host-created files are not foundation provenance and remain subject to host review and tests.

## 4. Run proof

Before host changes, the agent may run same-SHA zero-configuration examples:

```bash
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/feedback@${resolved_commit}"
GOWORK=off go run "github.com/timmyagentic/awesome-agent-app-features/examples/updater-demo@${resolved_commit}"
```

The Feedback example renders a preview only. The Updater example mutates only a temporary fake executable and makes no real Release request. Afterwards, run applicable remote checks from the manifest, focused host tests, and the target project's complete verification.

## 5. Write the minimal lock

After integration, write a visible `agent-app-features.lock.json` at the target root. Validate its structure with the same-SHA [integration-lock.schema.json](../features/integration-lock.schema.json), then run the same-commit stateless semantic validator:

```bash
GOWORK=off go run \
  "github.com/timmyagentic/awesome-agent-app-features/cmd/feature-lock@${resolved_commit}" \
  validate \
  --source "${temporary_exact_source_root}" \
  --source-commit "${resolved_commit}" \
  --host "${target_project_root}" \
  --lock "${target_project_root}/agent-app-features.lock.json"
```

`temporary_exact_source_root` is the temporary full extraction of the same-SHA archive. The validator stores no lifecycle state, does not run host checks, and does not replace the remote CI gate. It rejects mixed sources, duplicate or unknown Features, undeclared deliveries, mismatched Go module version or content, mismatched non-configuration subtree content, missing subtree targets, and nonexistent claimed host files.

The lock records only:

- source repository and full commit SHA;
- Feature ID, contract, actual delivery, and resolved module version;
- host-relative files changed by the agent;
- successful checks, `verified_at`, and honest `unverified` boundaries.

Do not store configuration values, tokens, cookies, user or chat IDs, payloads, raw logs, absolute paths, copied source, or runtime state. The lock is a locator for future agents, not runtime configuration, an audit database, or proof of completion.

## Feedback host inventory

- User actions, errors, or capability gaps that offer feedback.
- The host surface that displays every `Draft.Report()` field.
- The explicit approval action; only its callback may call `Approve(true)`.
- Product-specific secret, identifier, and path shapes covered by `AdditionalRedact` tests.
- Relay endpoint configuration, unavailable UX, and public fallback.

Never capture arbitrary environment maps, transcripts, reasoning, tool payloads, raw logs, identities, or credentials. Host tests cover cancellation with zero requests, preview/outbound equivalence, redaction, stale errors, the exact endpoint, and fallback.

## Updater host inventory

- Current-version truth and strict, non-mutating version output.
- Archive, checksum, and archive-entry naming.
- Executable path, install kind, and every update entry point.
- Authorization, progress renderer, restart, and post-restart acknowledgement.
- Beta/nightly policy kept separate from the stable-only path.

Every entry point shares one Updater configuration. Interactive paths use `Prepare -> render exact Plan -> authorize -> Apply(the same Plan)`. Only already-authorized non-interactive paths may call `UpdateLatest`. npm, Homebrew, Windows, and other install kinds require host adapters and do not inherit standalone replacement guarantees.

## Maintenance

- Inspect: keep the lock source fixed, reacquire that exact source, run the validator, then compare host wiring and manifest invariants read-only.
- Validate: keep the lock source fixed and rerun applicable checks; historical success is not current proof.
- Refine: close host UX, fallback, or test gaps at the same source.
- Upgrade: resolve a new CI-successful commit, compare API, manifests, and invariants, move every delivery together, update the lock, and validate against the new source; never mix sources.
- Remove: inspect current references and Git history, delete only known-unshared integration code, then remove the Feature from the lock; delete an empty lock.

These are ordinary agent operations on a codebase, not a foundation-owned action state machine. Git owns history; host tests own current truth.

## Completion

Report completion only when:

- the dependency and every remote resource use the same commit SHA;
- each responsibility and invariant has a host location or named blocker;
- applicable remote checks, focused tests, and complete target verification passed;
- unavailable client, credential, deployment, paid, restart, or production checks are `UNVERIFIED`;
- `agent-app-features.lock.json` passes the exact-source schema and matches actual delivery and host files.
- the same-SHA stateless validator passes, and every source subtree independently passes its gates in the copied host target.

Foundation-only tests, temporary output, or a historical lock never replace current host verification.
