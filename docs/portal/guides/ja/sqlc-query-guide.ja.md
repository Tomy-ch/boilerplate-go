# database/dml

`database/dml` は **sqlc によるコード生成の元となる SQL ファイル**を格納するディレクトリです。

ここに配置された SQL は `make gen-query` で Go コード（`internal/infrastructure/rdb/sqlc/gen/`）に変換されます。

## ディレクトリ構成

DML の区分ごとに 1 つのサブディレクトリを置き、それぞれが自分の README で受け持ちを述べる。

- `repository/` — 集約の永続化と単一集約の読み取り（CRUD）
- `query_service/` — 集約を跨ぐ読み取り投影と集計
- `command_service/` — 他の集約の状態と原子的に行う必要のある書き込み
- `system_cqrs/` — 業務ではなくシステム自身の運用のためのクエリ

## サブディレクトリとオニオンアーキテクチャの対応

|ディレクトリ|Infrastructure 実装先|interface 配置|用途|
|---|---|---|---|
|`repository/`|`internal/infrastructure/rdb/repository/`|Domain 層|Aggregate の CRUD|
|`query_service/`|`internal/infrastructure/rdb/query_service/`|Usecase 層|ユースケース固有の検索|
|`command_service/`|（将来拡張）|Usecase 層|書き込み専用コマンド|
|`system_cqrs/`|`internal/infrastructure/rdb/system_cqrs/`|Usecase 層|システム運用クエリ|

## SQL ファイルの配置ルール

- 1集約 = 1ディレクトリ（例: `repository/user/`）
- ファイル内の各クエリには `-- name: QueryName :type` を必ず付ける
- パラメータは `sqlc.arg()` または `@param` で命名する
- 生成コードは手動編集禁止

## sqlc ベストプラクティス

`sqlc` でのコード生成を前提に、**PostgreSQL + Go** でよく使う記法をまとめます。

## 1. `-- name:` と実行種別

各クエリの先頭に「クエリ名 + 実行種別」をコメントで付与します。

```sql
-- name: GetUser :one
SELECT * FROM users WHERE id = sqlc.arg(id);

-- name: CreateUser :exec
INSERT INTO users (name, email)
VALUES (sqlc.arg(name), sqlc.arg(email));
```

代表的な種別：

- `:one`     … 単一レコードを返す  
- `:many`    … 複数レコードを返す  
- `:exec`    … 結果なし（INSERT/UPDATE/DELETE）  
- `:execrows`… `RowsAffected` を返す  
- `:batch`   … 複数クエリをバッチ実行  

## 2. `sqlc.arg()` でパラメータ名を固定する

`sqlc.arg()` を使うと、生成される構造体のフィールド名を制御できます。  
`@param_name` 形式も同じ意味で利用可能です。

```sql
WHERE age > sqlc.arg(min_age)
```

```go
type GetUsersParams struct {
    MinAge int
}
```

ページングなど、nullable を許容したいパラメータでも `sqlc.arg()` を使います。

```sql
LIMIT  sqlc.arg(limit_param)
OFFSET sqlc.arg(offset_param)
```

また、PostgreSQL では、@を使うことでも同様にパラメータ名を指定できます。

```sql
WHERE age > @min_age
```

## 3. `sqlc.embed()` で JOIN 結果をネスト

JOIN 結果をネストした構造体で受け取りたい場合に使います。

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

## 4. `sqlc.narg()` で NULL 許容パラメータ

NULL を取り得る条件には `sqlc.narg()` を使います。

```sql
WHERE deleted_at IS sqlc.narg(deleted_at)
```

```go
type GetUsersParams struct {
    DeletedAt sql.NullTime
}
```

## 5. CAST で Go 側の型を補強する

PostgreSQL 側で明示的に型キャストすると、生成される Go の型も揃えやすくなります。

```sql
WHERE id = sqlc.arg(user_id)::uuid
```

```go
type GetUserParams struct {
    UserID uuid.UUID
}
```

## 6. `overrides` で生成型を上書きする

`sqlc.yaml`（例: `database/sqlc/sqlc.template.yaml`）で DB 型と Go 型の対応を上書きできます。

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

## 7. 配列パラメータは `ANY()` と組み合わせる

複数 ID をまとめて渡したい場合は、スライス + `ANY()` を使います。

```sql
WHERE id = ANY(sqlc.arg(user_ids)::uuid[])
```

```go
type GetUsersParams struct {
    UserIDs []uuid.UUID
}
```

1 行ごとに 1 文を発行する代わりに配列でまとめることで、行数に比例する往復を避けられます。呼び出し側が
トランザクションの間ずっと行ロックを保持している場合は、その保持時間の短縮にもなります。

## 8. SELECT カラム名 = Go フィールド名

SELECT するカラム名が、そのまま `Row` 構造体のフィールド名になります。

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

## 9. 複雑な検索はサブクエリ / CTE で整理

長くなりがちな検索クエリは、サブクエリや CTE で分割して可読性を確保します。

```sql
-- name: SearchUsers :many
SELECT * FROM (
    SELECT id, name FROM users
    WHERE name ILIKE '%' || sqlc.arg(keyword) || '%'
) sub
ORDER BY name;
```

## 推奨ルール（超要約）

1. **必須**: すべてのクエリに `-- name:` + 種別を付ける  
2. **必須**: パラメータは必ず `sqlc.arg()` / `@param` で命名する  
3. **必須**: NULL 許容は `sqlc.narg()` を使う  
4. **推奨**: JOIN は `sqlc.embed()` でネストする  
5. **推奨**: 型を合わせたいところは CAST を明示  
6. **推奨**: 配列は `ANY()` + `[]T` で扱う  
7. **推奨**: 複雑なクエリはサブクエリ/CTE で切り出す  

## 参考リンク

- [sqlc 公式ドキュメント](https://docs.sqlc.dev/en/latest/)
- [PostgreSQL 型一覧](https://www.postgresql.org/docs/current/datatype.html)
- [Go `database/sql` パッケージ](https://pkg.go.dev/database/sql)
