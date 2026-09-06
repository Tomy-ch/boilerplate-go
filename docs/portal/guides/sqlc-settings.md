# sqlc.yaml Options Affecting Go Code Generation (sqlc / PostgreSQL — pinned version in `mise.toml`)

## Overview

This document is a cheat sheet for the `sqlc.yaml` (`version: "2"`) options that materially change the shape of generated Go code in this project. It exists alongside the official sqlc docs because we have specific opinions on which options to use (and avoid) — for example, why we deliberately do NOT emit `json` tags from sqlc, and why we wrap nullable types in pointers. Use this as the first stop before editing `sqlc.yaml`; cross-reference the official sqlc docs only for option details not covered here.

This section summarizes the main `sqlc.yaml` (`version: "2"`) options that **affect generated Go code**.
(Examples assume PostgreSQL + Go, focusing on keys under `gen.go`)

## Minimal Configuration (Base)

```yaml
version: "2"
sql:
- engine: "postgresql"
  schema: "postgresql/schema.sql"
  queries: "postgresql/query.sql"
  gen:
    go:
      package: "db"
      out: "internal/infrastructure/rdb/gen"
      sql_package: "pgx/v5"
```

In `version: "2"`, definitions are placed under **`sql:` instead of `packages:`**.

## [SQL Section](https://docs.sqlc.dev/en/latest/reference/config.html#sql)

### engine

Specifies the database engine:

- `postgresql`
- `mysql`
- `sqlite`

### schema

Used to **load database schema information**.

This determines:

- Generated struct field types
- NULLability
- Enum definitions

You can specify one of the following:

- Path to a migration directory
- List of migration files
- Dumped schema SQL file

### queries

Used to **parse SQL queries and generate Go code**. Points at the DML path.

- Can point to a single SQL file or a directory

### database

Specifies DB connection info to **fetch schema at runtime**.

⚠️ Not recommended:

- May fail to correctly infer types/enums
- Can degrade generated code quality

👉 Prefer using the `schema` section.

### strict_function_checks

Returns an error if a referenced SQL function does not exist.

- Default: `false`

### strict_order_by

Returns an error if `ORDER BY` columns are ambiguous.

- Default: `true`

## 1. Output & Package Configuration

### `package`

Name of the generated Go package.

### `out`

Output directory for generated files.

### `sql_package`

Specifies the DB driver API used by generated code:

- `pgx/v5`
- `pgx/v4`
- `database/sql`

Notes:

- `pgx/v5` → uses pgx-native types/interfaces
- `database/sql` → uses standard library

## 2. Prepared Statements

### `emit_prepared_queries`

If `true`, generated code explicitly prepares and reuses statements.

Notes:

- `pgx/v5` already supports implicit prepared statements → usually unnecessary
- Useful with `database/sql` when you want explicit prepare at startup

## 3. Generated API Shape

### `emit_interface`

Generates an interface (e.g., `Querier`) for easier mocking and DI.

### `emit_methods_with_db_argument`

If `true`, methods take `DB`/`Tx` as arguments instead of storing it internally.

## 4. Struct Tags

### `emit_json_tags`

Adds `json:"..."` tags to generated structs.

### `emit_db_tags`

Adds `db:"..."` tags to generated structs.

## 5. Naming & Struct Generation

### `emit_exact_table_names`

Disables singularization; struct names match table names more closely.

### `rename`

Overrides specific column → Go field names.

```yaml
gen:
  go:
    rename:
      spotify_url: "SpotifyURL"
```

### `initialisms`

Controls handling of initialisms (e.g., ID, URL, API).

### `omit_unused_structs`

If `true`, omits structs for unused tables.

## 6. NULL / Pointer Behavior

### `emit_pointers_for_null_types`

If `true`, nullable columns become pointers (`*string`) instead of `sql.NullString`.

⚠️ Behavior depends on `sql_package`, so verify generated code.

### `emit_result_struct_pointers`

If `true`, query result structs are returned as pointers.

### `emit_params_struct_pointers`

If `true`, parameter structs are passed as pointers.

## 7. Enum Helpers

### `emit_enum_valid_method`

Generates a `Valid()` method for enum types.

### `emit_all_enum_values`

Generates helper to return all enum values.

## 8. Type Mapping (Overrides)

### `overrides`

Overrides DB type → Go type mapping.

```yaml
gen:
  go:
    overrides:
      - db_type: "pg_catalog.int4"
        go_type: "int"
```

## 9. Batch File Naming

### `output_batch_file_name`

Customizes the filename for batch-related generated code (default: `batch.go`). Emitted only when a query carries a batch annotation (`:batchexec` / `:batchmany` / `:batchone`).

## 10. Build Tags & JSON Case Style

### `build_tags`

Adds Go build tags to generated files.

### `json_tags_case_style`

Controls JSON tag casing (e.g., camelCase, snake_case).

## Summary (Recommended Usage)

- **For DI / mocking** → `emit_interface: true`
- **For JSON serialization** → `emit_json_tags: true`
- **For pointer-based NULL handling** → `emit_pointers_for_null_types: true` (verify output)
- **For explicit prepared statements** → `emit_prepared_queries: true` (usually unnecessary with pgx/v5)

## Notes

- Always regenerate via `make gen-query` (which runs sqlc inside `docker/tools/`) so the toolchain version is locked.
- Generated files (`*.sql.go`) are committed to the repo; CI (`gen-db-artifacts-check.yaml`) verifies that the committed output matches a fresh regeneration. Drift here blocks merge.
- Avoid hand-editing generated files — changes will be overwritten on the next `make gen-query`. If a change is needed, edit `sqlc.yaml` or the source SQL and regenerate.
- When introducing a new sqlc option that affects code shape (e.g., `emit_*`), update this document so the rationale stays discoverable.
