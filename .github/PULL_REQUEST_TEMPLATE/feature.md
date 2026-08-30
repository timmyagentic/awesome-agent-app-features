## Reusable boundary

<!-- What provider-neutral core or self-contained infrastructure subtree belongs in the Foundation? -->

## Host responsibilities

<!-- What UI, authorization, localization, credentials, deployment, restart, and failure UX remain host-owned? -->

## Security and compatibility invariants

<!-- List every invariant introduced or changed by this Feature. -->

## Real adopter

<!-- Link a non-fixture host mapping and its exact agent-app-features.lock.json. -->

## Verification evidence

- [ ] Focused core and adapter tests
- [ ] `go run ./cmd/feature-author sync-docs --root .`
- [ ] `go run ./cmd/feature-author validate --root .`
- [ ] `make verify`
- [ ] Remote no-clone delivery proof
- [ ] Focused host tests and the host's complete normal verification
- [ ] Same-source `feature-lock validate`

## UNVERIFIED

<!-- List every production, credential, deployment, client, paid, restart, or platform boundary without current evidence. Never leave this blank. -->

## Compatibility and release notes

<!-- State whether public API, wire schema, manifests, delivery modes, or introduction metadata changed. -->
