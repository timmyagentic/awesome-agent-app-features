# Feedback v1 integration contract

Agents discover this contract through `features/index.json`, resolve one CI-successful commit SHA, and fetch this README, `feature.json`, and every delivery item from that same SHA. The user does not clone the foundation.

The zero-configuration remote preview is:

```bash
GOWORK=off go run github.com/timmyagentic/awesome-agent-app-features/examples/feedback@<resolved-commit-sha>
```

The reusable flow is:

```text
host context -> Builder -> redacted Draft -> host renders Draft.Report
                                              -> explicit action
                                              -> Approved
                                              -> /v1/feedback HTTPS client
                                              -> relay-owned downstream adapter
```

The host may start this flow from natural language, a command, an error prompt, or a proactive “report this?” card. Trigger UX can vary; explicit submission consent cannot.

Every field in `Draft.Report()` must be visible before approval. `Report` is a deep copy and intentionally fails JSON encoding. The reference HTTP adapter accepts only `Approved`, requires the exact v1 endpoint, and refuses redirects.

Recent errors are optional context with a short freshness window. Capability gaps are normalized, sorted, and deduplicated. Neither is a different report type. Default redaction always runs; `AdditionalRedact` can only add product rules.

Cards, buttons, localized text, fallback UX, and intent belong in the host. GitHub title/body/label/repository/token belong in the relay. The core owns neither presentation layer.

Use [Feedback v1](../../docs/protocol-feedback-v1.md) for the exact wire contract and [security.md](../../docs/security.md) for residual risk.

The Go core and HTTPS client use `go-module` delivery. The optional Cloudflare relay uses `source-subtree` delivery: the Agent extracts only `relay/cloudflare` from the same resolved commit into host-owned infrastructure.

After integration, keep `.agent-app-features/feedback.json` aligned with this manifest. Use the manifest's `lifecycle` steps for inspect, validate, same-source refinement, exact-source upgrade, and safe removal. Receipt evidence names configuration keys but never endpoint values, tokens, payloads, logs, or user identifiers.
