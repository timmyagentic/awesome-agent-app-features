# Repository instructions

This repository is a headless feature foundation, not a product application,
UI framework, or Codex Skills collection.

## Change rules

- Keep product UI, chat commands, permissions, localization, and restart UX in
  the target repository. Core packages must remain provider- and UI-independent.
- Generic infrastructure adapters may connect HTTP, GitHub, release sources,
  or filesystems, but they must not choose host UX or product policy.
- Do not add product-specific repository names, tokens, paths, binary names, or
  hosted endpoints to reusable packages.
- Feedback core must remain structured: downstream issue title/body rendering
  belongs to a relay adapter, while cards/text/CLI/web rendering belongs to the
  host.
- Every feature manifest must declare `foundation.core`, provided `adapters`,
  `foundation.host`, and `foundation.excludes`.
- Manifests must distinguish their compatibility contract from publication truth:
  `release_status: unreleased` requires `since: null` until a tag actually exists. Released and unreleased Features may coexist; existing Features preserve their historical `since` when a later Feature is published.
- Treat every `invariants` entry in `features/*/feature.json` as a compatibility
  and security boundary.
- Feedback submission must continue to require an opaque `Approved` value.
- Feedback v1 changes must preserve the non-serializable `Report`, exact
  `/v1/feedback` endpoint, shared Go/Worker fixtures, strict unknown-field
  rejection, server-side destination, and no-redirect client.
- Updater changes must retain immutable Prepare/Apply plans, stable-only
  selection, same-release exact assets, checksum pinned before approval,
  checksum-before-extract, staged and installed version checks, per-target
  locking, non-mutating version probes, no-clobber backup, and rollback.
- `api/v1.txt`, `compat/v1`, manifest invariants, and protocol fixtures are v1
  compatibility gates; update them only after an explicit compatibility review.
- Do not add `SKILL.md`; integration instructions belong in feature manifests
  and `docs/agent-integration.md`.
- Preserve `features/index.json` as the single remote entry. User onboarding
  must start in the target project and must not require a foundation clone,
  Git submodule, local replace, or floating main dependency.
- Every remote resource and delivery item in one integration must resolve from
  the same full commit SHA after successful CI. Keep entry, feature, lock, and
  delivery schemas under contract tests.
- Host integrations leave one visible `agent-app-features.lock.json` with exact
  source, deliveries, relative files, checks, and `UNVERIFIED` boundaries. It
  must not contain values, credentials, payloads, logs, identifiers, absolute
  paths, copied source, lifecycle state, or history.
- Validate that lock with the same-commit `cmd/feature-lock` command against a
  temporary exact-source extraction and the real host root. The validator is a
  stateless consistency check, not an integration database or completion proof.
- A declared source subtree must pass its own install, test, generated-type,
  syntax, dry-run, and audit gates after independent extraction; foundation-root
  files or dependencies cannot make a delivery appear self-contained.
- The lock is maintenance metadata only. Git owns history, current code owns
  file relationships, and host tests own truth. Do not build another action or
  lifecycle state machine into the foundation.

## Verification

Run before handing off a change:

```bash
make verify
```

For a new Feature, start with `docs/adding-a-feature.md` and use `cmd/feature-author`; do not copy the current Feedback/Updater cohort into a new hard-coded test list.

## Repository workflow and branch protection

- Confirm the current GitHub default branch before starting; it is currently `main`. Fetch it, use a separate `codex/` branch in an isolated worktree, and keep the primary checkout read-only. Under Tingjun Workspace, activate the Task and Session, acknowledge the current Context revision and hold the worktree's unique writer claim.
- All changes to the GitHub default branch must go through a PR. Direct pushes, force pushes and deletion of that branch are prohibited. Administrators and apps have no bypass. Never use `--admin`, disable protection or add a bypass actor to land a change.
- Only squash merging is allowed. Inspect the final diff, current head, review threads and actual verification results before an authorized merge. Resolve all review conversations and address substantive findings; zero mandatory approvals supports the single-maintainer workflow. Merge with `gh pr merge <PR> --squash --match-head-commit <verified-full-head-SHA>` and verify the merged state afterward.
- Keep `delete_branch_on_merge = true` and verify that GitHub automatically removes the merged PR's remote task branch when eligible. Do not use `--delete-branch`, which also removes the local branch. Under Tingjun Workspace, completion only releases the writer claim; local branches and worktrees remain for the existing archive workflow.
- The default-branch ruleset declaration is [`.github/rulesets/default-branch.json`](.github/rulesets/default-branch.json); repository merge and deletion settings are declared in [`.github/repository-settings.json`](.github/repository-settings.json). These files do not apply themselves. After an authorized configuration update, read back the GitHub repository settings, effective rules and any classic branch protection. Protect the actual default branch rather than assuming it is named `main`.
- Keep CI aligned with the minimal checks documented below. Workflow or required-check changes need explicit task authorization; preserve app bindings and strictness unless that change is authorized. Existing stronger protections and immutable release tags remain in force. An empty or legitimately path-filtered checks list does not prove application tests passed; report the checks actually run and mark unavailable verification `UNVERIFIED`. Follow this repository's existing verification requirements and use proportionate document/configuration checks for documentation changes.

## Continuous integration

- Daily `CI` has three required GitHub Actions contexts: `Go checks`, `Remote no-clone consumer` and `Worker checks`. Keep API contracts, manifest/lock validation, cross compilation, remote exact-SHA consumption and independently extracted Worker verification.
- Main checks use Go 1.27 and Node 24; there is no daily Cartesian version matrix. Manual `CI` dispatch additionally runs Go 1.25/Linux and Go 1.27/macOS compatibility plus active fuzzing before publication.
- Fork PRs cannot fetch an upstream-only unmerged SHA for the no-clone test; its existing fork restriction remains. Same-repository PRs and default-branch pushes must actually run it.
- Keep the `CI` workflow name: it is part of the immutable remote-consumer delivery contract. Release publication remains a separately authorized manual workflow.
