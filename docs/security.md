# Security model

## Remote Agent integration

| Threat | Control | Residual risk |
| --- | --- | --- |
| Discovery content changes during integration | Resolve `main` once, require a full commit SHA, then refetch the entry and every resource from that SHA | A compromised repository can still publish a malicious commit |
| Foundation code and documentation come from different revisions | `features/index.json` requires same-commit manifest, docs, module, examples, and source-subtree delivery | The Agent must preserve the resolved SHA when invoking its own tools |
| Floating or local dependency bypasses review | Delivery forbids floating `main`, local `replace`, submodules, and a user-managed foundation checkout; Go records an immutable pseudo-version | A target maintainer can later change its dependency intentionally |
| Source archive writes outside the intended host path | Only declared subtrees may be extracted in a temporary directory after entry inspection; traversal and symlinks are rejected | The copied subtree becomes host-owned and needs normal review thereafter |
| Green CI is mistaken for publisher identity | Agents require successful CI as a quality gate | GitHub account/repository compromise remains in the trust root; higher-assurance users should require signed commits or independently rooted provenance |
| A future Agent cannot locate the integration | A visible minimal lock records exact source, deliveries, relative files, checks, and uncertainty; a same-commit stateless validator checks the declared source, module content, non-configuration subtree provenance, targets, and host files | Manifest-declared host-owned configuration and extra host files still require current code review, Git history, and fresh host tests |
| Lock becomes a secret or telemetry dump | Its strict schema has no configuration values, payloads, logs, identifiers, credentials, or absolute paths | Free-text check names and uncertainty still require Agent review |
| Removal deletes shared host code | The Agent inspects current references and Git history before deleting ordinary host files | Ownership remains a host-review decision; the lock does not claim authority |

The discovery URL on `main` is not an immutable dependency and must never be copied into a target lockfile as one. A no-clone workflow removes user-managed checkout drift; it does not remove the need to review the resolved source and target-project changes. The lock validator consumes a temporary same-SHA source extraction and stores no lifecycle state.

`agent-app-features.lock.json` is maintenance metadata, not runtime state or completion proof. Historical success is never proof of current behavior.

## Feedback

| Threat | v1 control | Residual risk |
| --- | --- | --- |
| Silent reporting | `Report` cannot be JSON-marshaled; provided transport accepts only opaque `Approved`; relay requires `user_approved: true` | A malicious host can bypass its own dependency; review host wiring |
| Credential or identity leakage | Default redaction always runs before and after `AdditionalRedact`; fixed environment allowlist; UTF-8 byte limits; stale-error window | Product-specific secrets still require host tests; redaction cannot prove absence of all PII |
| Approved POST replayed elsewhere | Client requires exact `/v1/feedback`, remote HTTPS, and refuses all redirects, including custom-client redirect policies | The configured relay receives the approved payload by design |
| Client controls GitHub | v1 rejects title/body/repository/label fields; token, renderer, and repository are server-side | Relay operator must scope and rotate its credential |
| Memory abuse | Request and GitHub response streams are bounded before JSON parsing | Valid maximum payloads still consume Worker resources |
| Spam | Required Cloudflare rate-limit binding uses connecting IP when available and one shared fallback bucket otherwise | Data-center-local/eventually consistent counters are an abuse brake, not billing accounting; NAT can over-limit |
| Duplicate issue flood | Fingerprint derives from provider-neutral report content; open matches receive comments | GitHub search is eventually consistent, so uniqueness is best effort |

Use a fine-grained GitHub token restricted to exactly one repository with Issues read/write. Store it only with `wrangler secret put`; never place it in source, `wrangler.jsonc`, application config, logs, fixtures, or client binaries.

## Updater

| Threat | v1 control | Residual risk |
| --- | --- | --- |
| Confirmation target drifts | `Prepare` deep-copies release metadata, resolves exact names, downloads checksum, and returns opaque `Plan`; `Apply` never resolves latest | A source can remove the asset; Apply then fails rather than switching |
| Release notes inject host behavior | `Release.Notes` is read-only metadata pinned in the Plan; hosts render it as untrusted text and never execute it or derive authorization from it | A trusted publisher controls the displayed release copy by design |
| Beta/RC reaches stable users | Exact SemVer stable tag without leading zero plus draft/prerelease rejection | A trusted publisher can still publish bad code as stable |
| Archive/checksum replaced together | SHA-256 is pinned during `Prepare`, before confirmation | A compromised publisher can serve malicious content before Prepare; checksum is not identity |
| Download corruption or archive bomb | Declared and streaming bounds, SHA-256 before extraction, one bounded regular-file entry | Host-chosen limits must fit its environment |
| Wrong binary | Exact static or release-derived entry plus staged and installed version probes; content hashes must remain unchanged across both probes | The version probe executes code from the verified archive and must still be trusted and strict |
| Broken replacement | Same-directory staging, directory sync, no-clobber hard-link backup, installed verification, rollback | Power-loss guarantees depend on the filesystem and platform |
| Concurrent mutation | Per-updater operation gate, per-target process mutex, and OS file lock on supported macOS/Linux targets | Unsupported operating systems are rejected; network-filesystem lock semantics remain outside the promise |
| Lock/target link attack | New and preflight checks reject executable symlinks; lock open uses no-follow and rejects non-regular files | Parent-directory integrity remains an operator responsibility |
| Recovery evidence overwritten | Existing `.update-backup` stops the transaction; backup creation cannot replace it | A retained backup requires deliberate operator cleanup |
| Custom HTTP redirect escapes source | GitHub API redirects are refused; asset redirects require HTTPS and allowed GitHub/enterprise hosts | An explicitly trusted allowed host can still serve malicious bytes |

For stronger publisher identity, add independently rooted Sigstore/SLSA provenance, minisign, TUF, or equivalent verification. Do not describe a checksum published beside its archive as a signature.

## Disclosure

Do not send vulnerability details, secrets, exploit payloads, or real identifiers through the public Feedback relay. Use GitHub private vulnerability reporting as described in [SECURITY.md](../SECURITY.md).
