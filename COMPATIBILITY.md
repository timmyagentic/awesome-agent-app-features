# Compatibility policy

The repository is stabilizing an unreleased `v1` contract. Once a v1 tag is formally published, it follows Semantic Versioning for the reusable foundation, not for host UI or product-specific adapters. Until then, pin an exact reviewed commit and do not treat `main` as a compatibility promise.

## Supported surface

The stable Go API consists only of these import paths:

- `github.com/timmyagentic/awesome-agent-app-features/feedback`
- `github.com/timmyagentic/awesome-agent-app-features/feedback/httpclient`
- `github.com/timmyagentic/awesome-agent-app-features/updater`
- `github.com/timmyagentic/awesome-agent-app-features/updater/github`

`api/v1.txt` records their exported declarations. `compat/v1` compiles and exercises them as an external package. CI rejects an accidental API change.

The minimum toolchain for all `v1.x` releases is Go 1.25. Consumers should use keyed literals for exported configuration and data structs; a minor release may add an optional field, constant, stage, or helper, but will not remove or change an existing one. Adding a method to a public interface, changing a method signature, or changing existing field semantics requires a new major version.

## Behavioral contracts

The following behavior is stable throughout v1:

- `Draft.Report` returns a deep copy and is not JSON-serializable.
- Only valid `Approved` values emit Feedback schema 1 JSON.
- The v1 wire endpoint is exactly `POST /v1/feedback`.
- Feedback v1 rejects unknown fields and enforces its existing field meanings and limits.
- `Prepare` selects and pins an exact stable release, assets, archive entry, and SHA-256.
- `Apply` never resolves latest and either installs that exact plan or fails/rolls back.
- Existing updater stage string values and exported sentinel errors remain valid.
- Every manifest `invariants` item remains a security and compatibility boundary.

Hosts should compare sentinel errors with `errors.Is`; complete error text is diagnostic and not a compatibility API. Event consumers must tolerate a new stage added by a future minor release, while existing stages retain their meaning and relative safety boundary.

## Wire evolution

Feedback schema 1 is strict. After release, a field addition, removal, rename, type change, approval change, or limit relaxation that old peers cannot safely interpret will use a new schema number and a new endpoint such as `/v2/feedback`. A future relay should keep the previous endpoint during a documented migration window rather than silently changing the meaning of v1.

Shared fixtures under `protocol/feedback/v1/testdata` are executable cross-language contracts. Go produces the valid full fixture; the Worker accepts valid fixtures and rejects invalid fixtures.

## Outside the stable surface

The following may evolve without a Go major version, provided the stable contracts above remain intact:

- Documentation wording and host-rendered UI examples.
- Files under `internal/` and implementation details of opaque values.
- Relay GitHub title/body presentation and best-effort deduplication details.
- CI layout and release packaging.

The reference Cloudflare relay is self-hosted. Operators control deployment timing and are responsible for migrating their own instance; this repository never changes a deployed relay automatically.
