# Feedback integration contract

Start with `feature.json`, then map each step to the host using
`docs/agent-integration.md`.

The reusable core has four phases:

```text
host context -> redacted Draft -> explicit host confirmation -> Approved -> relay
```

The host may trigger the flow from a command, a natural-language request, or a
proactive “report this?” card. Trigger UX can vary; submission consent cannot.

Recent errors should have a short relevance window. Capability gaps should be
deduplicated. Neither becomes a separate report type. The complete final draft,
including outbound metadata, must be visible before approval.

Use a persisted random `feedback.NewInstallID()` only if anonymous per-install
linkability is acceptable for rate limiting. Leaving it empty is supported.
