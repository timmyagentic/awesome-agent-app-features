# Cloudflare Feedback v1 relay

This is a self-hosted, single-tenant adapter from provider-neutral Feedback v1 to GitHub Issues. It owns GitHub title/body rendering, label, repository, token, rate limiting, and best-effort deduplication. Product binaries contain none of those credentials or destination controls.

The foundation's `v0.1.1` tag versions this source subtree. Its private npm metadata intentionally remains `0.0.0-unreleased`: the Relay is copied into host-owned infrastructure and is not published as an npm package.

## Configure

1. Let the integration Agent resolve `features/index.json` to one CI-successful commit SHA, then extract only the declared `relay/cloudflare` source subtree from that same-SHA GitHub archive or Contents API into host-owned infrastructure. The user does not clone the foundation.
2. Change the Worker `name`, `GITHUB_REPO`, and optional `GITHUB_LABEL` in `wrangler.jsonc`.
3. Replace the example rate-limit `namespace_id` with a positive integer unique in the Cloudflare account.
4. Create a fine-grained GitHub token restricted to that repository with Issues read/write.
5. Regenerate the committed binding types after configuration changes, install locked dependencies, set the secret, verify, and deploy:

```bash
npm ci --ignore-scripts
npm run types
npm exec wrangler secret put GITHUB_TOKEN
npm test
npm run check
npm run typecheck
npm run types:check
npm run validate:worker
npm audit --audit-level=high
npm exec wrangler deploy
```

Never put `GITHUB_TOKEN` in `wrangler.jsonc`, source control, client configuration, fixtures, or logs. The repository ships no production route/domain and does not deploy this Worker automatically.

## Contract

Only `POST /v1/feedback` with `Content-Type: application/json` is accepted. The payload must match [Feedback v1](../../docs/protocol-feedback-v1.md), include `schema: 1` and `user_approved: true`, contain no unknown fields, and stay inside byte limits.

Clients cannot send issue title/body, label, repository, or token. The Worker validates structured data, renders the issue, bounds GitHub responses, validates returned issue URLs, and returns:

```json
{
  "reference_url": "https://github.com/owner/repository/issues/7",
  "deduplicated": false
}
```

The fingerprint uses provider-neutral report content. Matching open issues receive a `+1` environment comment. GitHub search is eventually consistent, so this is best effort rather than a uniqueness guarantee.

The rate-limit key uses Cloudflare's connecting IP when available and one shared fallback bucket otherwise. The included binding allows five requests per 60 seconds. Counters are data-center local/eventually consistent and shared IPs can over-limit; treat it as an abuse brake.

## Verification

```bash
npm ci --ignore-scripts
npm test                 # self-contained unit tests + real workerd runtime smoke tests
npm run check
npm run typecheck        # check JavaScript against generated Worker binding/runtime types
npm run types:check      # prove worker-configuration.d.ts matches wrangler.jsonc
npm run validate:worker  # Wrangler dry run against the committed config
npm audit --audit-level=high
```

The Worker has structured error logging, bounded request/upstream reads, a 15-second GitHub timeout, no global request state, and no floating promises. `worker-configuration.d.ts` is generated from `wrangler.jsonc`, while the Workers runtime types are version-locked through `@cloudflare/workers-types`; the only extra binding is the `GITHUB_TOKEN` secret set through Wrangler. Compatibility date, Rate Limiting binding, observability, and non-secret vars live in `wrangler.jsonc`.

Cross-language Feedback fixtures and foundation manifest/schema checks intentionally live outside this copied subtree under the foundation's `internal/contract` gate. They run in foundation CI; the commands above prove the declared delivery itself has no hidden dependency on the foundation checkout.

The copied subtree becomes host-owned infrastructure. Future updates must repeat the same immutable-ref extraction, regenerate binding types after config changes, and review the host diff rather than following floating `main`; no production deployment is implied by adding the files.

The host's `agent-app-features.lock.json` may record the subtree source and target, relative files, and verification commands. It must never record `GITHUB_TOKEN`, endpoint values, repository credentials, issue payloads, or raw Worker logs.
