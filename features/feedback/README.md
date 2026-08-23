# Feedback v1 integration contract

Read `feature.json` first. The reusable flow is:

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
