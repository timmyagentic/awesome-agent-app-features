# Changelog

All notable changes are documented here. This project follows Semantic Versioning.

## [1.0.0] - 2026-08-30

### Added

- Stateless same-commit host-lock validation for Feature identity, declared deliveries, Go module version/content, source-subtree targets, and claimed host files.
- Presentation-neutral Release Notes on exact Updater plans, allowing hosts to preserve release copy without a second floating latest lookup.
- Independent extracted-subtree Relay verification with generated Worker binding types, type checking, workerd tests, Wrangler dry-run, and dependency audit.
- An opt-in, temporary-files-only public GitHub Release updater probe for real host release layouts.
- Stable Agent integration manifests for Feedback and Updater.
- Provider-neutral Feedback core with fixed allowlists, default plus product redaction, bounded fields, stale-error filtering, deep-copy preview, and opaque explicit approval.
- Feedback v1 JSON Schema and shared Go/JavaScript contract fixtures.
- HTTPS Feedback v1 client with approval-before-network, exact endpoint, HTTPS, no-redirect, bounded-response, and receipt validation.
- Self-hosted Cloudflare Worker adapter with strict v1 validation, rate limiting, bounded I/O, server-side GitHub credentials/destination, rendering, and best-effort deduplication.
- Stable standalone updater with immutable `Prepare`/`Apply` plans, pinned checksums, stable-only selection, bounded archives, non-mutating staged and installed version checks, per-target locks, no-clobber backups, structured recovery evidence, and rollback.
- Separate `updater/github` source adapter with repository, release-page, asset-host, response-size, and redirect validation.
- Public API snapshot, external-consumer compile contract, fuzz seeds, race tests, Worker-runtime tests, vulnerability checks, and reproducible source-release workflow.

### Compatibility

- This release defines the first stable contract. Pre-v1 commit consumers remain unsupported; migrate them to `v1.0.0` or its exact commit.
- The minimum Go version is 1.25.
- Feedback wire schema 1 is served only at `POST /v1/feedback`.
