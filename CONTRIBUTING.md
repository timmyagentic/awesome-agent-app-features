# Contributing

Awesome Agent App Features accepts reviewed Feature contributions through ordinary pull requests. It is not a dynamic plugin registry: reusable safety policy lives here, while product UI, commands, permissions, localization, credentials, and deployment remain in the host.

Start with [Adding a Feature](docs/adding-a-feature.md). The supported authoring commands are:

```bash
go run ./cmd/feature-author new --id example-feature --name "Example Feature" --kind go
go run ./cmd/feature-author new --id example-adapter --name "Example Adapter" --kind source-subtree --runtime javascript
go run ./cmd/feature-author validate --root .
make verify
```

Generated scaffolds are deliberately unreleased. Replace every placeholder responsibility, invariant, integration step, and verification statement with reviewed Feature-specific truth before requesting publication.

Every Go delivery needs a zero-network `go-run` example. Every source-subtree delivery needs an in-subtree `verify` script that proves the copied delivery independently; successful copying alone is not verification.

Do not add credentials, product identifiers, hosted endpoints, host UI, or a `SKILL.md`. A new delivery mode, wire schema, or compatibility-contract version needs an explicit architecture review rather than a local schema workaround.
