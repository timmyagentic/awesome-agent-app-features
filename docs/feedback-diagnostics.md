# Turn-bound feedback diagnostics (unreleased)

`feedback/diagnostic` adds a typed diagnostic report to Feedback without changing
the original `feedback` package's v1 payload or endpoint. Use it when an incident
must retain its original request after a long turn or a host restart.

## Capture and approval

The host captures the initiating participant's input before injecting prompts,
quoted messages or shared conversation context. Bind it to the project,
conversation, turn and participant. Freeze the incident before cleanup or error
presentation. A diagnostic timestamp is an observation, not a freshness-based
reason to discard an explicitly bound incident.

`Builder.Capture` returns a deep-copied, redacted, bounded value for local storage.
`Builder.Build` creates an opaque immutable `Draft`. `Draft.Report` is the full
preview and refuses JSON serialization; only `Draft.Approve(true)` produces the
serializable `Approved`. The host owns authorization, rendering and retention.
Retain `Draft.PreparedAt()` alongside the copied input when restoring a draft:
use it as `Builder.Now` to preserve the exact approved payload, including the
legacy recent-error freshness decision. Preserve `ReportID` for retries.

Runtime fields are a fixed allowlist. `settings_source` distinguishes configured
settings, launch/turn requests and backend observations. A requested model is
not proof of the model chosen by a server. Unknown CLI versions, transport facts
and timestamps belong in `missing`; absent counters are unknown, not zero.
There is no raw log, reasoning, arbitrary map, tool argument/output, native
session identifier or installation identifier field.

The original request and error each have 4,000 UTF-8 bytes, the visible response
has 1,200 bytes, activity keeps at most 24 labels/timings, and missing/truncation
lists have at most 24 entries. Text is redacted before truncation. The host must
not attach unowned history or another participant's input; the library cannot
infer provenance from text.

## Delivery and recovery

`httpclient.Client.SubmitDiagnostic` accepts only `diagnostic.Approved` and uses
exactly `POST /v2/feedback` with schema 2. It never follows redirects or falls
back to v1. Up to two attempts share one 15-second maximum budget and the exact
same body. A shorter caller deadline still wins. Validation errors do not retry.

The reference Worker supports both v1 and v2. Configure `FEEDBACK_REPORTS` as a
SQLite-backed `FeedbackReport` Durable Object with the supplied migration. The
object binds each random report ID to the canonical payload hash and durably
writes intent before calling GitHub. Concurrent attempts share the result;
confirmed receipts survive eviction. Stored state contains only the hash,
outcome and receipt, never the report text or GitHub credential.

If an Issue POST might have succeeded, an explicit retry searches for its exact
report-ID/hash marker. A positive match restores the receipt. An empty search
does not authorize another POST because GitHub search is eventually consistent.
An ambiguous result therefore remains an error until positively confirmed; this
is not an exactly-once guarantee across independent services. Only a definite
GitHub rejection permits a new create attempt for the same report. There is no
polling, alarm, background submission or later user-visible status update.

The ID deduplicates one approved report. It is not a semantic bug grouping key
or a user/install identifier. Existing v1 duplicate grouping remains available
to legacy clients.

## Compatibility decision

This is an additive, unreleased extension under the existing Feedback feature.
The v1 `Input`, `Draft`, `Report`, `Approved`, `Client.Submit`, fixtures, strict
unknown-field behavior, endpoint and no-redirect policy retain their contracts.
The new package, `Client.SubmitDiagnostic`, `DiagnosticEndpointPath` and typed
HTTP `ResponseError` are additions; response error text stays unchanged.
`api/v1.txt` records the expanded public API after this explicit review. The
feature keeps its historical `since: v0.1.0`; that does not claim v2 was released
then. Source consumers must pin a CI-successful immutable commit.

Deploy the same-commit dual-protocol Relay and its migration before releasing a
host that sends v2. Keep the old route for old clients. Deploying, publishing a
Foundation tag and releasing the host are separate operational actions. This
source change does not perform any of them. Hosts can preserve their existing
commands, cards, approval rules and result messages while changing collection
and transport internally.
