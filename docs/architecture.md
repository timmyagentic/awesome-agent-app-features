# Feature foundation architecture

This repository is a headless capability foundation, not a product framework. A coding agent maps stable cores and generic infrastructure adapters into a host repository; the host keeps product and channel decisions.

```text
host product
  intent / authorization / cards / CLI / web / i18n / restart
                              |
                       thin host adapter
                              |
                  structured values + events
                              |
              reusable feature foundation v1
              core ports     generic adapters
```

## Ownership

| Foundation | Host |
| --- | --- |
| Provider-neutral data, opaque state, sentinel errors | Product flow, copy, cards, buttons, commands |
| Security policy and deterministic transactions | Intent, authorization, configuration UX |
| HTTP, GitHub Release, and filesystem adapters | Install-kind selection, restart, acknowledgement |
| Neutral events and structured results | Channel SDKs, localization, analytics |
| Remote manifests, lock schema, examples, API and wire gates | Thin glue, visible lock, host regression and E2E tests |

A Feishu card or terminal prompt is always a host renderer. A GitHub Release source or HTTPS transport may live here because it implements a generic port without choosing host experience.

## Dependency direction

```text
host adapter -> feedback             host adapter -> updater
             -> feedback/httpclient               -> updater/github
                                                        |
relay/cloudflare -> Feedback v1 protocol               v
                                             Source port in updater core
```

Core packages import only the Go standard library. Infrastructure adapters depend inward on their core; no core depends outward on an adapter or host.

## Feedback boundary

```text
host-selected context
  -> Builder (allowlist + redaction + bounds + freshness)
  -> Draft -> Report deep copy rendered by host
  -> explicit user action -> opaque Approved
  -> /v1/feedback HTTPS adapter
  -> strict single-tenant Relay
  -> Relay-owned GitHub rendering and destination
```

`Report` deliberately fails JSON encoding. Only `Approved` emits Feedback v1 JSON. The host owns preview and action; the Relay owns issue title, body, label, repository, and credential.

## Updater boundary

```text
Source.LatestStable
  -> Prepare validates stable metadata and exact assets
  -> Prepare downloads and pins the same-release SHA-256
  -> opaque Plan with exact release Notes rendered and authorized by host
  -> Apply exact Plan (no second latest lookup)
  -> lock, checksum, bounded staging, version checks,
     backup, replacement, cleanup or rollback
```

The host owns discovery cadence, authorization, install-kind routing, progress copy, restart, and post-restart confirmation. `updater/github` owns GitHub API and redirect rules. The core owns only the macOS/Linux standalone transaction.

## Agent integration plane

Agent-friendly does not mean a Skill, blind installer, or user-managed foundation checkout:

- `features/index.json` is the single remote entry.
- `features/*/feature.json` declares responsibilities, delivery, invariants, and checks.
- `features/integration-lock.schema.json` defines a small visible host record; `cmd/feature-lock` performs same-commit stateless semantic validation against actual delivery and host files.
- Remote `go run package@commit` examples prove public usage without a checkout.
- `api/v1.txt`, `compat/v1`, JSON Schemas, and shared fixtures prevent accidental drift.
- Host tests prove the last-mile adapter preserves each invariant.

Go packages arrive through the module cache at one CI-successful commit SHA. Infrastructure templates arrive by extracting only declared source subtrees from the same SHA, and each declared subtree must pass its own gates after independent extraction. Git submodules, local replaces, floating `main`, and a user-managed second checkout are outside the model.

`agent-app-features.lock.json` records source, deliveries, relative host files, checks, and uncertainty. It is deliberately not a lifecycle database: Git provides history, current code provides ownership, and host tests provide truth. An agent can inspect, refine, upgrade, or remove ordinary code without a foundation-defined action state machine.

Contract tests reject third-party imports and product/channel terms in core source. They are guardrails; an architectural boundary change still requires review.
