# Repository instructions

This repository is a reusable feature kit, not a product application and not a
Codex Skills collection.

## Change rules

- Keep product UI, chat commands, permissions, localization, and restart UX in
  adapters or examples. Core packages must remain UI-independent.
- Do not add product-specific repository names, tokens, paths, binary names, or
  hosted endpoints to reusable packages.
- Treat every `invariants` entry in `features/*/feature.json` as a compatibility
  and security boundary.
- Feedback submission must continue to require an opaque `Approved` value.
- Updater changes must retain stable-only selection, same-release exact assets,
  checksum-before-extract, staged and installed version checks, locking, backup,
  and rollback.
- Do not add `SKILL.md`; integration instructions belong in feature manifests
  and `docs/agent-integration.md`.

## Verification

Run before handing off a change:

```bash
gofmt -l .
go test -race ./...
go vet ./...
npm test --prefix relay/cloudflare
npm run check --prefix relay/cloudflare
npm run validate:worker --prefix relay/cloudflare
```
