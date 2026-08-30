# Compatibility policy

The published `v0.1.0` module is a pre-1.0 preview of contract v1. Semantic Versioning applies to the reusable foundation, not to host UI or product-specific adapters, but backward compatibility is not promised across `v0.x` minor releases. Integration Agents resolve the remote entry to an exact reviewed commit SHA after successful CI and pin every resource to that SHA; `main` remains discovery-only, not a compatibility promise. Go consumers should use the immutable `v0.1.0` tag or its exact commit.

## Supported surface

The current public Go API consists only of these import paths:

- `github.com/timmyagentic/awesome-agent-app-features/feedback`
- `github.com/timmyagentic/awesome-agent-app-features/feedback/httpclient`
- `github.com/timmyagentic/awesome-agent-app-features/updater`
- `github.com/timmyagentic/awesome-agent-app-features/updater/github`

`api/v1.txt` records their exported declarations. `compat/v1` compiles and exercises them as an external package. CI rejects an accidental API change. Before module `v1.0.0`, a deliberately reviewed breaking change may ship in a new `v0` minor release with explicit migration notes; patch releases remain compatibility-preserving.

The remote integration surface also includes `features/index.json`, feature manifests, declared delivery modes, manifest invariants, and `integration-lock.schema.json`. Once published, removing a Feature, weakening an invariant, or changing an existing delivery or lock field incompatibly requires an explicit migration.

The minimum toolchain for the `v0.x` preview line is Go 1.25. Consumers should use keyed literals for exported configuration and data structs. A patch release may add an optional field, constant, stage, or helper but will not remove or change an existing one. Before `v1.0.0`, adding a method to a public interface, changing a signature, or changing existing field semantics requires an explicit compatibility review and a new `v0` minor release.

## Behavioral contracts

The following behavior defines contract v1 throughout the current preview line; changing it requires an explicit compatibility review and migration:

- `Draft.Report` returns a deep copy and is not JSON-serializable.
- Only valid `Approved` values emit Feedback schema 1 JSON.
- The v1 wire endpoint is exactly `POST /v1/feedback`.
- Feedback v1 rejects unknown fields and enforces its existing field meanings and limits.
- `Prepare` selects and pins an exact stable release, presentation-neutral Notes, assets, archive entry, and SHA-256.
- `Apply` never resolves latest and either installs that exact plan or fails/rolls back.
- Existing updater stage string values and exported sentinel errors remain valid.
- Every manifest `invariants` item remains a security and compatibility boundary.
- One integration never mixes delivery commits.
- The host lock remains metadata only and never substitutes for current host verification; the stateless validator checks source/delivery/file consistency without turning the lock into lifecycle state.

Hosts should compare sentinel errors with `errors.Is`; complete error text is diagnostic and not a compatibility API. Event consumers must tolerate a new stage added by a future minor release, while existing stages retain their meaning and relative safety boundary.

## Wire evolution

Feedback schema 1 is strict. After release, a field addition, removal, rename, type change, approval change, or limit relaxation that old peers cannot safely interpret will use a new schema number and a new endpoint such as `/v2/feedback`. A future relay should keep the previous endpoint during a documented migration window rather than silently changing the meaning of v1.

Shared fixtures under `protocol/feedback/v1/testdata` are executable cross-language contracts. Go produces the valid full fixture; the Worker accepts valid fixtures and rejects invalid fixtures.

## Outside the stable surface

The following may evolve without changing the contract version, provided the contracts above remain intact:

- Documentation wording and host-rendered UI examples.
- Files under `internal/` and implementation details of opaque values.
- Relay GitHub title/body presentation and best-effort deduplication details.
- CI layout and release packaging.
- Free-text wording in manifests and integration guidance, while declared invariants retain their meaning.

The reference Cloudflare relay is self-hosted. Operators control deployment timing and are responsible for migrating their own instance; this repository never changes a deployed relay automatically.
