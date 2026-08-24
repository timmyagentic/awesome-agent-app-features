# Security model

## Remote Agent integration

| Threat | Control | Residual risk |
| --- | --- | --- |
| Discovery content changes during integration | Resolve `main` once, require a full commit SHA, then refetch the entry and every resource from that SHA | A compromised repository can still publish a malicious commit |
| Foundation code and documentation come from different revisions | `features/index.json` requires same-commit manifest, docs, module, examples, and source-subtree delivery | The Agent must preserve the resolved SHA when invoking its own tools |
| Floating or local dependency bypasses review | Delivery forbids floating `main`, local `replace`, submodules, and a user-managed foundation checkout; Go records an immutable pseudo-version | A target maintainer can later change its dependency intentionally |
| Source archive writes outside the intended host path | Only declared subtrees may be extracted in a temporary directory after entry inspection; traversal and symlinks are rejected | The copied subtree becomes host-owned and needs normal review thereafter |
| Green CI is mistaken for publisher identity | Agents require successful CI as a quality gate | GitHub account/repository compromise remains in the trust root; higher-assurance users should require signed commits or independently rooted provenance |
| A future Agent cannot tell what was integrated | A strict host receipt records exact source, deliveries, artifacts, invariants, evidence, uncertainty, and history | Receipt evidence can become stale; `inspect` and `validate` must compare it with current host state |
| Receipt becomes a secret or telemetry dump | Schema permits configuration key names and relative paths, never values, payloads, logs, user IDs, credentials, or absolute machine paths | Free-text evidence still requires Agent redaction and review |
| Remove deletes shared host code | Artifacts declare `integration-managed` or `host-shared`; only unshared candidates may be removed and a tombstone records retained paths | Incorrect ownership classification remains a host-review risk |

The discovery URL on `main` is not an immutable dependency and must never be copied into a target lockfile as one. A no-clone workflow removes user-managed checkout drift; it does not remove the need to review the resolved source and target-project changes.

An `active` receipt is allowed only when every invariant is preserved or not applicable and remote plus host verification passed. A receipt with blockers must remain `partial`. Historical evidence, including a previously active receipt, is not proof of current behavior.

## Feedback

| Threat | v1 control | Residual risk |
| --- | --- | --- |
| Silent reporting | `Report` cannot be JSON-marshaled; provided transport accepts only opaque `Approved`; relay requires `user_approved: true` | A malicious host can bypass its own dependency; review host wiring |
| Credential or identity leakage | Default redaction always runs before and after `AdditionalRedact`; fixed environment allowlist; UTF-8 byte limits; stale-error window | Product-specific secrets still require host tests; redaction cannot prove absence of all PII |
| Approved POST replayed elsewhere | Client requires exact `/v1/feedback`, remote HTTPS, and refuses all redirects, including custom-client redirect policies | The configured relay receives the approved payload by design |
| Client controls GitHub | v1 rejects title/body/repository/label fields; token, renderer, and repository are server-side | Relay operator must scope and rotate its credential |
| Memory abuse | Request and GitHub response streams are bounded before JSON parsing | Valid maximum payloads still consume Worker resources |
| Spam | Required Cloudflare rate-limit binding prefers connecting IP, then optional install ID | Data-center-local/eventually consistent counters are an abuse brake, not billing accounting; NAT can over-limit |
| Duplicate issue flood | Fingerprint derives from provider-neutral report content; open matches receive comments | GitHub search is eventually consistent, so uniqueness is best effort |

Use a fine-grained GitHub token restricted to exactly one repository with Issues read/write. Store it only with `wrangler secret put`; never place it in source, `wrangler.jsonc`, application config, logs, fixtures, or client binaries.

## Updater

| Threat | v1 control | Residual risk |
| --- | --- | --- |
| Confirmation target drifts | `Prepare` deep-copies release metadata, resolves exact names, downloads checksum, and returns opaque `Plan`; `Apply` never resolves latest | A source can remove the asset; Apply then fails rather than switching |
| Beta/RC reaches stable users | Exact SemVer stable tag without leading zero plus draft/prerelease rejection | A trusted publisher can still publish bad code as stable |
| Archive/checksum replaced together | SHA-256 is pinned during `Prepare`, before confirmation | A compromised publisher can serve malicious content before Prepare; checksum is not identity |
| Download corruption or archive bomb | Declared and streaming bounds, SHA-256 before extraction, one bounded regular-file entry | Host-chosen limits must fit its environment |
| Wrong binary | Exact static or release-derived entry plus staged and installed version probes; content hashes must remain unchanged across both probes | The version probe executes code from the verified archive and must still be trusted and strict |
| Broken replacement | Same-directory staging, directory sync, no-clobber hard-link backup, installed verification, rollback | Power-loss guarantees depend on the filesystem and platform |
| Concurrent mutation | Per-updater operation gate, per-target process mutex, and OS file lock | Non-Unix fallback can leave a stale crash lock; operators must verify before removal |
| Lock/target link attack | New and preflight checks reject executable symlinks; Unix lock open uses no-follow; non-regular locks fail | Parent-directory integrity remains an operator responsibility |
| Recovery evidence overwritten | Existing `.update-backup` stops the transaction; backup creation cannot replace it | A retained backup requires deliberate operator cleanup |
| Custom HTTP redirect escapes source | GitHub API redirects are refused; asset redirects require HTTPS and allowed GitHub/enterprise hosts | An explicitly trusted allowed host can still serve malicious bytes |

For stronger publisher identity, add independently rooted Sigstore/SLSA provenance, minisign, TUF, or equivalent verification. Do not describe a checksum published beside its archive as a signature.

## Disclosure

Do not send vulnerability details, secrets, exploit payloads, or real identifiers through the public Feedback relay. Use GitHub private vulnerability reporting as described in [SECURITY.md](../SECURITY.md).
