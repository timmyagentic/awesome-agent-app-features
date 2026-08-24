# Feature foundation architecture

This repository is a headless feature foundation, not a product framework. A coding agent maps stable cores and generic infrastructure adapters into a host repository; the host keeps every product and channel decision.

```text
host product
  intent / authorization / cards / CLI / web / i18n / lifecycle
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
| Provider-neutral data, opaque state, sentinel errors | Product flow, user-facing copy, cards, buttons, commands |
| Security policy and deterministic transactions | Intent detection, administrator policy, configuration UX |
| HTTP, GitHub Release, and filesystem adapters | Install-kind selection, restart, acknowledgement |
| Neutral progress events and structured results | Channel SDKs, localization, analytics, support workflow |
| Manifests, examples, shared fixtures, API/consumer gates | Glue code and host-specific regression/E2E tests |

A Feishu card or terminal prompt is always a host renderer. A GitHub release source or HTTPS transport can live here because it implements a provider port without choosing the host experience.

## Dependency direction

```text
host adapter -> feedback             host adapter -> updater
             -> feedback/httpclient               -> updater/github
                                                        |
relay/cloudflare -> Feedback v1 protocol               v
                                             Source port in updater core
```

Core packages import only the Go standard library. `feedback` knows nothing about HTTP or GitHub. `updater` knows nothing about GitHub. Infrastructure adapters depend inward on their core; no core depends outward on an adapter or host.

## Feedback boundary

```text
host-selected context
  -> Builder (allowlist + default/additional redaction + bounds + freshness)
  -> Draft
  -> Report deep copy rendered by the host
  -> explicit user action
  -> opaque Approved
  -> /v1/feedback HTTPS adapter
  -> strict single-tenant relay
  -> relay-owned GitHub rendering/destination
```

`Report` deliberately fails JSON encoding. Only `Approved` emits schema 1 JSON. The host owns the preview and action; the relay owns issue title/body/label/repository. Shared Go and Worker fixtures freeze the boundary.

## Updater boundary

```text
Source.LatestStable
  -> Prepare validates stable metadata and exact assets
  -> Prepare downloads and pins the same-release SHA-256
  -> opaque Plan rendered/authorized by the host
  -> Apply exact Plan (no second latest lookup)
  -> lock, archive, checksum, staged verify, backup, replace,
     installed verify, cleanup or rollback
```

The host owns discovery cadence, authorization, install-kind routing, progress copy, restart, and post-restart confirmation. `updater/github` owns GitHub API/URL/redirect rules. The core owns only the standalone transaction.

## Agent integration plane

Agent-friendly does not mean a Codex Skill, blind installer, or a user-managed foundation checkout. The user remains in the target project. An agent resolves the remote entry to one CI-successful commit SHA, reads every resource from that SHA, and adapts the feature safely:

- `features/index.json` is the single remote discovery and delivery entry.
- `features/*/feature.json` declares ownership, prerequisites, invariants, and commands.
- `features/integration-plan.schema.json` makes the host mapping and remaining uncertainty reviewable.
- Feature READMEs describe the low-level contract.
- `docs/agent-integration.md` defines the host inventory and mapping process.
- Remote `go run package@commit` examples prove the intended public API without a clone.
- `api/v1.txt` and `compat/v1` prevent accidental public drift.
- Host tests prove the last-mile adapter retained every invariant.

Go packages are delivered through the module cache at the resolved commit. Infrastructure templates are delivered by extracting only declared source subtrees from the same commit. Git submodules, local replaces, floating main references, and a user-managed second checkout are outside the integration model.

The architecture test rejects channel/product terms and third-party imports in core source files. This is a guardrail; an architectural boundary change still requires human review.
