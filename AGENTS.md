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
- Manifests must distinguish the v1 contract from publication truth:
  `release_status: unreleased` requires `since: null` until a tag actually exists.
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
- The lock is maintenance metadata only. Git owns history, current code owns
  file relationships, and host tests own truth. Do not build another action or
  lifecycle state machine into the foundation.

## Verification

Run before handing off a change:

```bash
make verify
```
