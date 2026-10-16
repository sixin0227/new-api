# Body recording is opt-in (global master + per-token whitelist, both default off)

Recording full request/response content stores users' private conversation data, so the feature ships dark: a global master switch (system settings → log settings, default off) AND a per-token switch (admin-configured on the token edit page, default off) must both be on before any body is recorded. Bodies are viewable only by admin/root, via a dedicated lazy-loaded endpoint (`GET /api/log/body?request_id=…`) — list and detail APIs never carry body payloads. NDJSON export deliberately excludes bodies: an exported file is a second copy of private content that leaves the system's protection boundary.

## Considered Options

- Default-on with per-token opt-out: rejected — every deployment would start persisting sensitive content unknowingly.
- User-self-service toggles: rejected — lets users unilaterally cause their own content to be persisted, bypassing the admin's data-governance intent.
- Body full-text search without bounds: rejected — `LIKE '%…%'` on body columns is a full scan; the two "content contains" filters must be combined with an explicit time range (enforced in frontend and backend).

## Consequences

- Body recording follows consume-log writing: with `LogConsumeEnabled` off there is no log row, hence no body.
- Body search and viewing are admin-only; horizontal-access control is therefore concentrated in one admin gate rather than per-user filtering.
