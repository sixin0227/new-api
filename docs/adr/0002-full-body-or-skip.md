# Bodies are stored whole or skipped, never truncated

When a relay request/response body exceeds the configured size threshold (default 1 MiB, configurable), that side is skipped entirely and flagged (`body_skipped`), rather than being truncated to a fixed prefix. We chose all-or-nothing because a half-truncated body is useless for debugging and misleading for search: `LIKE '%…%'` over truncated content silently misses tails, and reviewers mistake prefixes for the whole exchange.

## Considered Options

- Hard cap with truncation (e.g. store first 64 KiB): rejected — breaks the "what exactly was sent/received" contract the feature exists for.
- Unlimited storage: rejected — one pathological 1 MiB+ multimodal request per token would bloat the log database without bound.

## Consequences

- The log row is still written when bodies are skipped; only the body payload is omitted, plus a per-side skip flag so the admin UI can say "body not recorded (too large)".
- Streaming responses are reassembled into the equivalent non-stream JSON before the size check, so the threshold applies to the logical payload, not to SSE framing.
