# Cloudflare feedback relay

This is a self-hosted, single-tenant relay. It accepts an explicitly approved,
redacted report and creates (or deduplicates onto) a GitHub issue without
putting a GitHub token in the product binary.

The client cannot choose the destination repository. `GITHUB_REPO` and the
fine-grained token are configured on the Worker.

## Configure

1. Copy this directory or deploy it from the repository.
2. Change `name`, `GITHUB_REPO`, and optionally `GITHUB_LABEL` in
   `wrangler.jsonc`.
3. Change the `RATE_LIMITER` `namespace_id` from the example `1001` to a
   positive integer unique within your Cloudflare account.
4. Create a fine-grained GitHub token restricted to that repository with
   Issues read/write permission.
5. Install the pinned Wrangler version, store the token as a Worker secret,
   validate, and deploy:

```bash
npm install
npm exec wrangler secret put GITHUB_TOKEN
npm run validate:worker
npm exec wrangler deploy
```

Do not put `GITHUB_TOKEN` in `wrangler.jsonc`, source control, application
configuration, or the client binary.

## Contract

`POST /v1/feedback` accepts feedback schema 1 from the Go `feedback` package.
The relay requires `user_approved: true`, validates a strict field allowlist,
limits title/body sizes by UTF-8 bytes, and always uses its server-side target.

Success:

```json
{
  "issue_url": "https://github.com/owner/repository/issues/7",
  "deduplicated": false
}
```

Identical open reports are deduplicated on product + title and become `+1`
comments. This is best effort because GitHub issue search is eventually
consistent.

`RATE_LIMITER` is required. The included Wrangler template configures five
requests per 60 seconds per installation ID, falling back to source IP only
when the client omits its anonymous install ID. Cloudflare's counters are local
to a data center and eventually consistent, so this is an abuse brake rather
than billing-grade accounting. Shared IP fallback can limit unrelated users.

## Verify locally

```bash
npm test
npm run check
npm run validate:worker
```
