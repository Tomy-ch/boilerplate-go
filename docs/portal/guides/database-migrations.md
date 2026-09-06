# migrations

`database/migrations` stores **database migration files managed by golang-migrate**.

## File Generation

Generate new migration files with:

```bash
make new-migrate-<name>
```

Example:

```bash
make new-migrate-create_orders
```

This auto-generates a numbered up / down pair:

```text
000011_create_orders.up.sql
000011_create_orders.down.sql
```

## File Naming Convention

```text
{6-digit sequence}_{description}.up.sql    # Upgrade (apply)
{6-digit sequence}_{description}.down.sql  # Downgrade (rollback)
```

- Sequence starts at `000001`, zero-padded to 6 digits
- Description uses snake_case to briefly describe the change
- up and down must always be created as a pair

## Running Migrations

|Command|Description|
|---|---|
|`make db-migrate-up DB=<name>`|Apply all pending migrations|
|`make db-migrate-down DB=<name>`|Rollback the last migration|

Also available via CLI:

```bash
./server migrate-up
./server migrate-down
```

## Rules

- **Never modify existing migration files** — changing applied files causes hash mismatches
- **Always create new files** — schema changes must be done via new migrations
- **Create up and down as pairs** — rollback is impossible without down
- **down must be the exact inverse of up** — table creation → DROP, column addition → DROP COLUMN
- **Be idempotent where possible** — use `IF NOT EXISTS` / `IF EXISTS`
- **No gaps in sequence numbers** — CI detects gaps (`migration-check.yaml`)

## Reference-master table shape

A reference master (a table whose rows are fixed by migration and have no write API) carries these columns:

```sql
id UUID, name VARCHAR(100), code SMALLINT, sort_key SMALLINT, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
```

`code` and `sort_key` both hold a small integer, which is exactly why the split has to be stated rather than
inferred. They answer different questions and move for different reasons:

|Column|What it is|Exposed by the API|
|---|---|---|
|`code`|A **static alias** for the row. Application code, SQL and API clients refer to the row by this, and its value never moves once assigned.|**Yes** — this is what a client sends and receives|
|`sort_key`|The **key that makes ordering idempotent**. `ORDER BY sort_key` is what fixes the display order, and it is free to move whenever the intended order changes.|**No** — the response's array order already carries it|

Both are `UNIQUE`. The consequence worth stating: **never order by `code`**. Ordering by `code` fuses the two
roles, so the only way to change the display order becomes renumbering `code` — which silently breaks every
client holding it as a constant and every `WHERE ... code = ...` in `database/dml/`.

A column that is not a reference master's ordering, but an order the *user* chooses and the API returns,
is neither of these — it is named `display_sort` to keep it out of the pair
above.

## CI Check

The `migration-check.yaml` workflow automatically verifies:

- No duplicate sequence numbers
- No gaps in sequence numbers
- up / down pairs are complete

## Notes

- Only place DDL (table definitions, schema changes) in this directory
- DML (data manipulation) belongs in `database/dml/`
- Seed data belongs in `database/seed/`
- Initialization SQL (DB creation, extension setup) belongs in `docker/database/sql/`
