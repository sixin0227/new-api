# Request/response bodies live in a dedicated `request_bodies` table, not in `logs`

Relay request/response bodies can reach megabytes (long contexts, base64 media), while the `logs` table is queried list-wise on every dashboard page load. We decided to store bodies in a separate `request_bodies` table on the log database (SQLite/MySQL/PostgreSQL via AutoMigrate, plus a ClickHouse DDL that reuses `LOG_SQL_CLICKHOUSE_TTL_DAYS`), keyed by `request_id`, instead of adding `request_body`/`response_body` columns to `logs`.

## Considered Options

- Extra columns on `logs`: rejected — every list query would pay for wide rows, ClickHouse TTL semantics would entangle bodies with log metadata, and the two data lifecycles (audit row vs. privacy-sensitive content) would be welded together.

## Consequences

- Body reads/deletes/searches go through `request_id` joins or subqueries on the same log database connection; no cross-database join is possible when LOG_DB differs from the primary DB.
- Bulk delete and the `log-cleanup` scheduled task must cascade-delete body rows so no orphan bodies survive; ClickHouse TTL expires both tables at the same age.
