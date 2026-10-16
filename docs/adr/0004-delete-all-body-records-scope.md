# Delete-all removes the filtered set of Body Records, coupled with their usage logs, in server-side batches

The body browser's Delete All removes every recorded exchange matching the current browse filters (the same filter set the list endpoint uses; an empty filter deliberately means the whole table). Each exchange is removed as a unit — its body rows and its usage-log row together — so the two tables never disagree about whether an exchange exists. The sweep runs server-side in batches of 500 request ids and reports one exact total, because the interactive batch endpoint caps a request at 100 ids specifically so no UI click can sweep the table.

## Considered Options

- Body-only deletion (keep the usage logs): rejected — it would split one exchange's lifecycle, make the bodies list and the log list disagree, and contradict the existing single/batch deletion contract.
- A client loop over the existing 100-id batch endpoint: rejected — partial failure leaves an unclear state, the client round-trips once per 100 ids, and the reported count becomes hard to guarantee.
- An asynchronous background cleanup task: rejected — Delete All is an administrator's explicit "now" action; the scheduled log-cleanup task already covers time-based retention.
- Delete-all that always ignores filters (whole table only): rejected — clearing a specific user's or search's records is the common case; the whole table stays reachable by applying no filters.
- Interactive deletion also raising or dropping its 100-id cap: rejected — the cap protects the interactive path; the explicit, counted Delete All is the sanctioned way to exceed it.

## Consequences

- With no filters set, Delete All removes the entire bodies table together with the matching usage-log rows — an accounting-history operation. The confirmation dialog always shows the exact affected count and states that the usage logs go with it.
- On ClickHouse each batch runs `ALTER TABLE … DELETE … SETTINGS mutations_sync = 1`; without the synchronous mutation the next batch query could re-read rows that were only scheduled for deletion.
- The deleted count is the number of exchanges requested per batch, not the driver's affected-rows value (ClickHouse reports none for mutations).
- A record whose usage-log row is already gone is still swept: batching reads request ids from the bodies table only.
