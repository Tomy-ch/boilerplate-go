# seed

`database/seed` stores **transactional data (seed data) for non-production environments** (local / CI / staging).

## Role

Used to insert **initial data** needed to verify application behavior in development, test, and demo environments.

<!-- sample-api:replace-begin -->
- User data
- Product data
- Other business data
<!-- sample-api:replace-with -->
<!-- = - Business data -->
<!-- sample-api:replace-end -->

Targets **transactional data**, not master data.

## Execution

```bash
make db-seed
```

Or via CLI:

```bash
./server db-seed
```

## Naming Convention

```text
{6-digit sequence}_{description}.sql
```

Example:

<!-- sample-api:replace-begin -->
```text
000001_users.sql
000002_users_additional.sql
000007_products_electronic_equipment_01.sql
```
<!-- sample-api:replace-with -->
<!-- = ```text -->
<!-- = 000001_<aggregate>.sql -->
<!-- = 000002_<aggregate>_additional.sql -->
<!-- = 000007_<aggregate>_<group>_01.sql -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

- Executed in ascending order of sequence number
<!-- sample-api:replace-begin -->
- Control order via sequence numbers when dependencies exist (e.g., users → products → purchases)
<!-- sample-api:replace-with -->
<!-- = - Control order via sequence numbers when dependencies exist (a referenced row is seeded before the row referencing it) -->
<!-- sample-api:replace-end -->
- A group that does not fit in one file is split with a two-digit suffix (`_01`, `_02`, …)

## Placeholders

A seed file may write an environment-dependent value as `${NAME}`; the runner expands it at execution
time. Use it wherever a literal would be correct in one environment only — the port a local provider
publishes, for instance, shifts per worktree slot.

<!-- sample-api:replace-begin -->
```sql
-- issuer follows the environment (the mock auth server's published port shifts per worktree slot)
INSERT INTO user_identities (id, user_id, issuer, subject) VALUES
('...', '...', '${AUTH_ISSUER}', 'user-john-doe') ON CONFLICT (id) DO NOTHING;
```
<!-- sample-api:replace-with -->
<!-- = ```sql -->
<!-- = -- issuer follows the environment (the mock auth server's published port shifts per worktree slot) -->
<!-- = INSERT INTO <table> (id, issuer, subject) VALUES -->
<!-- = ('...', '${AUTH_ISSUER}', '<subject>') ON CONFLICT (id) DO NOTHING; -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

The available names are supplied by the `db-seed` command (`cmd/seed.go`) — currently `AUTH_ISSUER`,
the JWT issuer taken from the configuration. A placeholder with no value (undefined or empty) is **not**
filled with an empty string: the run errors out and **the whole file is left unexecuted** — rows in it
that use no placeholder are skipped too. Data holding an empty environment value would leave a state
where seeding succeeded yet only the runtime path fails, which is hard to trace back, so the file is the
unit of fail-closed. A value containing a single quote is rejected the same way: the expansion is textual,
so a value that can escape its string literal would run as a statement of its own.

## Difference from Migrations

|Aspect|migrations|seed|
|---|---|---|
|Target|DDL (schema definitions)|DML (data insertion)|
|Production use|Required|Not recommended|
|Rollback|up / down pairs|None (delete or re-insert data)|
|Idempotency|Required|Recommended (`ON CONFLICT`, etc.)|

## Notes

- **Not intended for production execution** — seed data is for development and testing
<!-- sample-api:replace-begin -->
- Master data (prefectures, status definitions, etc.) should be managed via migrations
<!-- sample-api:replace-with -->
<!-- = - Master data (a fixed reference table with no independent write lifecycle) should be managed via migrations -->
<!-- sample-api:replace-end -->
- When tables have foreign key constraints, pay attention to insertion order (sequence numbers)
- For large datasets, split into multiple files and manage with sequence numbers. Keep each file
  under 20000 bytes: `make sql-lint` skips a larger file instead of parsing it, so the whole file
  silently loses lint coverage
- Make idempotent where possible (`INSERT ... ON CONFLICT DO NOTHING`, etc.)
