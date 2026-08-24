# Host receipt example

This directory demonstrates the persistent target-project artifact created by the remote `integrate` action:

```text
.agent-app-features/
  feedback.json
```

The JSON is a complete schema-valid example, not evidence for a real deployed host. Agents must replace its source, host mappings, artifacts, invariant evidence, verification, timestamps, and unverified boundaries with facts from the target project.

Receipts contain no credentials, configuration values, payloads, logs, user identifiers, absolute paths, or copied source code. A later `inspect`, `validate`, `refine`, `upgrade`, or `remove` action uses the receipt as the durable starting point. Removal retains the receipt with `state: removed` as an audit tombstone.
