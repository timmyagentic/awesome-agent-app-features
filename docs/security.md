# Security model

## Feedback threats

| Threat | MVP control | Residual risk |
| --- | --- | --- |
| Silent background reporting | Opaque `Approved` type plus relay `user_approved` check | A malicious host can bypass its own library; review host wiring |
| Credential/PII leakage | Aggressive default redaction, bounded text, fixed environment allowlist | Product-specific secret shapes require extra host redaction tests |
| Client steals or redirects GitHub token | Token and target repository exist only in the single-tenant relay | Relay operator credential scope still matters |
| Spam | Payload bounds and required Cloudflare Rate Limiting binding keyed by install ID (IP fallback) | Limits are per data center/eventually consistent; install IDs can rotate and shared IPs can over-limit |
| Issue flood from duplicates | Best-effort product+title fingerprint and open-issue comments | GitHub search is eventually consistent |

Use a fine-grained GitHub token restricted to one repository and Issues
read/write. Never expose the relay token in client configuration or logs.

## Updater threats

| Threat | MVP control | Residual risk |
| --- | --- | --- |
| Beta/RC reaches stable users | Exact `v?X.Y.Z` tag plus draft/prerelease rejection | A maintainer can still publish bad code as stable |
| Archive/checksum mix-and-match | Both are exact assets on one selected Release | A compromised release account can replace both |
| Download corruption | SHA-256 before extraction | SHA-256 is integrity, not publisher identity |
| Wrong executable in valid archive | One exact binary name and staged `--version` verification | Version probe must itself be strict and product-specific |
| Broken replacement | Same-filesystem staging, backup, installed version check, rollback | Power loss at filesystem boundaries still depends on filesystem durability |
| Concurrent updates | In-process gate and target file lock | Non-Unix crash can leave a stale fallback lock |
| Loss of recovery evidence | Existing `.update-backup` is never overwritten | Operator must inspect/remove a stale backup deliberately |
| Package-manager install mutated as standalone | Symlink executable paths are refused | Hosts still need explicit install-kind detection and adapters |

For a stronger publisher identity guarantee, extend the release pipeline and
`Source` with Sigstore/SLSA provenance, minisign, or another separately rooted
signature. Do not describe a checksum published beside its archive as a
signature.

The updater executes the staged binary's version command before install. Only
consume releases from a repository whose publishers you trust; the version
probe is code execution from the verified archive.
