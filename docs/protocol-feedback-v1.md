# Feedback v1 protocol

Feedback v1 carries provider-neutral, already-redacted report data from a host application to an author-operated relay.

## Transport

- Method and path: `POST /v1/feedback`
- Remote scheme: HTTPS; the Go client allows HTTP only for loopback development.
- Request content type: `application/json`
- Success content type: `application/json`
- Redirects: the Go client never follows a redirect for an approved POST.
- Maximum request body at the reference relay: 96 KiB.

The authoritative structural schema is [schema.json](../protocol/feedback/v1/schema.json). Implementations additionally enforce UTF-8 byte limits because JSON Schema `maxLength` counts characters.

## Payload

```json
{
  "schema": 1,
  "user_approved": true,
  "install_id": "optional-random-install-id",
  "environment": {
    "product": "Example Agent",
    "version": "v1.0.0",
    "os": "darwin",
    "arch": "arm64",
    "agent": "codex"
  },
  "description": "Improve startup diagnostics",
  "recent_error": {
    "text": "a redacted recent failure",
    "occurred_at": "2026-08-23T09:00:00Z"
  },
  "capability_gaps": ["doctor.explain"]
}
```

`schema`, `user_approved`, and `environment.product` are required. At least one of `description`, `recent_error`, or non-empty `capability_gaps` is required. Unknown fields are rejected. Empty optional fields must be omitted rather than sent as `null`, an empty string, or an empty array.

| Field | Limit |
| --- | ---: |
| Each environment field | 160 UTF-8 bytes |
| `install_id` | 1–64 ASCII letters/digits/dot/underscore/hyphen |
| `description` | 4,000 UTF-8 bytes |
| `recent_error.text` | 4,000 UTF-8 bytes |
| One capability gap | 160 UTF-8 bytes |
| Capability gaps | 20 unique values |

The Go `Builder` applies those limits before approval. The relay validates them again. A host must render every field returned by `Draft.Report`; calling `Draft.Approve(true)` is permitted only after the explicit user action represented by that host UI.

## Response

Successful submission returns:

```json
{
  "reference_url": "https://github.com/owner/repository/issues/7",
  "deduplicated": false
}
```

The reference relay uses `400` for invalid payloads, `405` for the wrong method, `413` for size limits, `415` for content type, `429` for rate limiting, `500` for missing relay configuration, and `502` for a GitHub upstream failure. Clients should treat any non-2xx response as a failed submission and offer a safe product-owned fallback.

## Privacy and identity

The protocol has no transcript, reasoning, tool payload, arbitrary metadata map, title, body, label, repository, or token field. `install_id` is optional and should be a persisted random identifier only when anonymous per-install rate limiting is acceptable. The reference relay prefers Cloudflare's connecting IP for abuse limiting when available and never includes it in the issue body.

## Evolution

Schema 1 is strict and will be frozen when the v1 contract is released. After that point, an incompatible shape uses a new schema number and endpoint; it is never introduced by changing `/v1/feedback` in place.
