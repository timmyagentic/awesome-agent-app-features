# Adding a Feature

This guide is for contributors adding a new reusable capability to the Foundation. Consumers integrating an existing Feature should follow [the remote integration protocol](agent-integration.en.md) instead.

## 1. Choose the boundary

A Feature belongs here only when it has a provider-neutral core or a reusable, self-contained infrastructure subtree. Keep product intent, authorization, cards, commands, localization, restart UX, credentials, and deployment policy in the host.

Every Feature must state its reusable core, generic adapters, host responsibilities, exclusions, invariants, prerequisites, integration steps, and verification at remote/Foundation/host layers.

## 2. Scaffold

For a Go core:

```bash
go run ./cmd/feature-author new --id diagnostics --name "Diagnostics" --kind go
```

For a source-subtree-only capability:

```bash
go run ./cmd/feature-author new \
  --id diagnostics-export \
  --name "Diagnostics export" \
  --kind source-subtree \
  --runtime javascript
```

The command creates `features/<id>/feature.json`, `features/<id>/README.md`, the declared delivery skeleton, and one sorted entry in `features/index.json`. It refuses to overwrite an existing Feature and synchronizes the generated catalog blocks in both public READMEs from that manifest truth.

Source-subtree-only Features do not need a synthetic Go package or `go-run` example. Every source-subtree delivery declares a repository-relative `verify` script inside the subtree; no-checkout CI runs that script after copying the delivery into the temporary host. Replace the scaffold's minimal `verify.sh` with the runtime-appropriate install/test/type/generated-output/dry-run/audit gate before release.

## 3. Develop while unreleased

New scaffolds use `release_status: unreleased` and `since: null`. Released and unreleased Features may coexist. Remote consumers read the selected manifest and skip unreleased Features unless explicitly evaluating that source revision.

Do not assign `since` before the introduction tag exists. In the release commit, change only the new Feature to `released` and set `since` to that exact tag. Existing Features preserve their original introduction tag.

## 4. Required implementation

- Keep the core package on the Go standard library unless an architecture review approves otherwise.
- Keep adapters dependent inward on their core.
- Add focused tests for every invariant and failure boundary.
- Add a zero-configuration, no-side-effect remote example when the Feature has a Go delivery.
- Add compatibility/API coverage for public packages.
- Add a self-contained target gate for every source-subtree delivery.
- Update `CHANGELOG.md` and user-facing integration guidance when catalog behavior changes.

## 5. Validate

```bash
go run ./cmd/feature-author sync-docs --root .
go run ./cmd/feature-author validate --root . --json
make verify
make fuzz
```

The author validator discovers all manifests; it contains no fixed Feature list. It also rejects a public README whose generated catalog block differs from `features/index.json` and the selected manifests. No-checkout CI dynamically tests released Go deliveries, runs zero-network examples, materializes source-subtrees into a temporary host, creates a host lock, and runs the same-SHA validator.

Before publication, the manual workflow runs:

```bash
go run ./cmd/feature-author release-check --root . --tag vX.Y.Z --json
```

This accepts historical `since` tags on the release history, accepts a new Feature introduced by the current tag, and keeps unreleased Features at `since: null`.

JSON mode always returns the stable fields `code`, `what`, `why`, `remediation`, and `next_command` together with `schema`, `ok`, and `command`. It changes presentation only; the commands remain stateless and retain exit code 0 for success, 1 for a validation failure, and 2 for invalid arguments.

## 6. Completion evidence

A valid manifest is not completion. Provide focused core/adapter tests, complete Foundation verification, no-checkout remote consumer proof, at least one real host mapping with an exact lock, and honest production/credential/client/deployment `UNVERIFIED` boundaries.
