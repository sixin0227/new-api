# new-api — Domain Glossary

AI API gateway: relays client requests to upstream providers, with billing, quotas and audit logging.

## Language

**Body Record**:
The recorded request body and response body of one relay exchange, stored as a pair and associated with its usage log through the request id. The record and its usage log are deleted together, so neither side survives the other.
_Avoid_: payload dump, raw log

**Body Skip**:
The state in which one side of a Body Record exceeds the size threshold and is therefore not recorded at all (never truncated); the usage log still exists and marks the side as skipped.
_Avoid_: truncation, partial body

**Request Id**:
The gateway-generated correlation id that ties a relay exchange to its usage log and, when recorded, to its Body Record.
_Avoid_: trace id

**Log Database (LOG_DB)**:
The optionally separate database that stores runtime usage/error logs and Body Records; may be a different engine (even ClickHouse) than the primary database.
_Avoid_: analytics DB

**Body Search**:
A list-level filter matching a substring against recorded request or response bodies; requires an explicit time range so it never scans unbounded history.
_Avoid_: full-text search (no index or analyzer is involved)
