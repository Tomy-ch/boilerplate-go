# database/dml

`database/dml` stores **SQL source files for sqlc code generation**.

SQL files placed here are converted to Go code (`internal/infrastructure/rdb/sqlc/gen/`) via `make gen-query`.

## Directory Structure

One subdirectory per DML category; each has its own README stating what belongs in it.

- `repository/` — Aggregate persistence and single-aggregate reads (CRUD)
- `query_service/` — Read projections that span aggregates, and aggregation
- `command_service/` — Writes that must be atomic with another aggregate's state
- `system_cqrs/` — Queries for the system's own operation rather than the business

## Subdirectory Mapping to Onion Architecture

|Directory|Infrastructure Implementation|Interface Placement|Purpose|
|---|---|---|---|
|`repository/`|`internal/infrastructure/rdb/repository/`|Domain layer|Aggregate CRUD|
|`query_service/`|`internal/infrastructure/rdb/query_service/`|Usecase layer|Usecase-specific search|
|`command_service/`|(future extension)|Usecase layer|Write-only commands|
|`system_cqrs/`|`internal/infrastructure/rdb/system_cqrs/`|Usecase layer|System operational queries|

## SQL File Placement Rules

- 1 aggregate = 1 directory (e.g., `repository/user/`)
- Each query must have `-- name: QueryName :type` annotation
- Parameters must be named with `sqlc.arg()` or `@param`
- Generated code must not be manually edited

## sqlc Best Practices

A summary of commonly used patterns for **PostgreSQL + Go** with `sqlc` code generation.

## 1. `-- name:` and Execution Type

Add a comment with "query name + execution type" at the beginning of each query.

```sql
-- name: GetUser :one
SELECT * FROM users WHERE id = sqlc.arg(id);

-- name: CreateUser :exec
INSERT INTO users (name, email)
VALUES (sqlc.arg(name), sqlc.arg(email));
```

Representative types:

- `:one`     … returns a single record  
- `:many`    … returns multiple records  
- `:exec`    … no result (INSERT/UPDATE/DELETE)  
- `:execrows`… returns `RowsAffected`  
- `:batch`   … executes multiple queries in batch  

## 2. Fix Parameter Names with `sqlc.arg()`

Using `sqlc.arg()` allows you to control the field names of generated structs.  
The `@param_name` format can also be used with the same meaning.

```sql
WHERE age > sqlc.arg(min_age)
```

```go
type GetUsersParams struct {
    MinAge int
}
```

Use `sqlc.arg()` even for parameters that allow nullable values such as pagination.

```sql
LIMIT  sqlc.arg(limit_param)
OFFSET sqlc.arg(offset_param)
```

Also, in PostgreSQL, you can specify parameter names using `@` as well.

```sql
WHERE age > @min_age
```

## 3. Nest JOIN Results with `sqlc.embed()`

Use this when you want to receive JOIN results as nested structs.

```sql
-- name: GetUserWithProfile :one
SELECT sqlc.embed(u), sqlc.embed(p)
FROM users u
JOIN profiles p ON p.user_id = u.id
WHERE u.id = sqlc.arg(id);
```

```go
type GetUserWithProfileRow struct {
    User    User
    Profile Profile
}
```

## 4. Nullable Parameters with `sqlc.narg()`

Use `sqlc.narg()` for conditions that can take NULL.

```sql
WHERE deleted_at IS sqlc.narg(deleted_at)
```

```go
type GetUsersParams struct {
    DeletedAt sql.NullTime
}
```

## 5. Reinforce Go Types with CAST

Explicit type casting on the PostgreSQL side helps align generated Go types.

```sql
WHERE id = sqlc.arg(user_id)::uuid
```

```go
type GetUserParams struct {
    UserID uuid.UUID
}
```

## 6. Override Generated Types with `overrides`

You can override DB type to Go type mappings in `sqlc.yaml` (e.g. `database/sqlc/sqlc.template.yaml`).

```yaml
version: "2"
sql:

- engine: postgresql
    gen:
      go:
        package: gen
        out: internal/infrastructure/rdb/...
        overrides:
          - db_type: "pg_catalog.int4"
            go_type: "int"
```

## 7. Combine Array Parameters with `ANY()`

To pass multiple IDs at once, use slices + `ANY()`.

```sql
WHERE id = ANY(sqlc.arg(user_ids)::uuid[])
```

```go
type GetUsersParams struct {
    UserIDs []uuid.UUID
}
```

Batching rows into arrays instead of issuing one statement per row avoids the round trips that scale
with the row count. Where the caller holds a row lock for the duration of a transaction, it also
shortens how long that lock is held.

## 8. SELECT Column Names = Go Field Names

Column names selected become field names of the `Row` struct as-is.

```sql
-- name: GetUserEmailAndName :one
SELECT email, name FROM users WHERE id = sqlc.arg(id);
```

```go
type GetUserEmailAndNameRow struct {
    Email string
    Name  string
}
```

## 9. Organize Complex Queries with Subqueries / CTE

Split complex queries using subqueries or CTEs to maintain readability.

```sql
-- name: SearchUsers :many
SELECT * FROM (
    SELECT id, name FROM users
    WHERE name ILIKE '%' || sqlc.arg(keyword) || '%'
) sub
ORDER BY name;
```

## Recommended Rules (Concise Summary)

1. **Required**: Add `-- name:` + type to all queries  
2. **Required**: Always name parameters using `sqlc.arg()` / `@param`  
3. **Required**: Use `sqlc.narg()` for nullable parameters  
4. **Recommended**: Use `sqlc.embed()` for JOIN nesting  
5. **Recommended**: Explicitly use CAST where types need alignment  
6. **Recommended**: Use `ANY()` + `[]T` for arrays  
7. **Recommended**: Split complex queries with subqueries/CTE  

## Reference Links

- [sqlc official documentation](https://docs.sqlc.dev/en/latest/)
- [PostgreSQL data types](https://www.postgresql.org/docs/current/datatype.html)
- [Go `database/sql` package](https://pkg.go.dev/database/sql)
