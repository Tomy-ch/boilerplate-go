# Goコード生成に影響する `sqlc.yaml` の主なオプション（sqlc / PostgreSQL — バージョンは `mise.toml` に固定）

## 概要

本ドキュメントは、本プロジェクトで生成される Go コードの形に実質的な影響を与える `sqlc.yaml`（`version: "2"`）オプションのチートシートです。sqlc 公式ドキュメントと並行して用意する理由は、本プロジェクト独自の判断（例: なぜ `json` タグを生成しないか、なぜ nullable をポインタで包むか）を残すためです。`sqlc.yaml` を編集する前にまずここを読み、本書でカバーされていない詳細は公式ドキュメントを当たってください。

このセクションでは、`sqlc.yaml`（`version: "2"`）において **Goコードの生成結果に影響する**主な設定をまとめます。
（例は PostgreSQL + Go を前提。`gen.go` 配下のキーを中心に記載）

## 最小構成（ベース）

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

`version: "2"` では `packages:` ではなく **`sql:` 配下に定義**します。

## [sqlセクション](https://docs.sqlc.dev/en/latest/reference/config.html#sql)

### engine

利用する DB エンジンを指定します。

- `postgresql`
- `mysql`
- `sqlite`

### schema

ここのセクションの情報を元に **DBスキーマ情報を取得** します。

ここの情報を元にして、次が決まります。

- 生成される構造体のフィールド型
- NULL 可否
- Enum 定義

下記の三つのうちいずれかを指定します：

- マイグレーションディレクトリへのパス
- 特定のマイグレーションファイルのリスト
- dumpしたスキーマSQLファイルなどへのパス

### queries

ここのセクションの情報を元に **SQLクエリを解析** し、Goコードの生成を行います。

DML のパスを指定します。

- SQL ファイル単位でもディレクトリ単位でも指定可能

### database

DB接続情報を指定し、**実行時にDBへ接続してスキーマ情報を取得** する場合に使います。

⚠️ 非推奨:

- 型情報や Enum 情報を正しく推論できないことがある
- 生成コードの品質が落ちる可能性がある

👉 `schema` セクションを使う方法を推奨します。

### strict_function_checks

呼び出されたSQL関数が存在しない場合にエラーを返します。

- デフォルト: `false`

### strict_order_by

order by列が曖昧な場合にエラーを返します。

- デフォルト: `true`

## 1. 出力先とパッケージ構成

### `package`

生成される Go の `package` 名。

### `out`

生成ファイルの出力先ディレクトリ。

### `sql_package`

生成コードが利用する DB ドライバの API を指定します。

- `pgx/v5`
- `pgx/v4`
- `database/sql`

補足:

- `pgx/v5` → pgx ネイティブの型・インターフェースを使う
- `database/sql` → 標準ライブラリを使う

## 2. Prepared Statements（明示Prepare）

### `emit_prepared_queries`

`true` の場合、**生成コードに「PrepareしてStmtを保持して実行する」実装**が入ります。

- **pgx/v5 は暗黙的に prepared statement を扱える**ため、追加設定不要（= 必要性は相対的に低い）と明記されています。
- `database/sql` など「暗黙preparedが無い/弱い」ドライバで、起動時に prepare して使い回したい場合に選択肢になります。

## 3. 生成されるAPI形状（インターフェース/メソッド形）

### `emit_interface`

`true` にすると `Querier` のようなインターフェースが生成され、モックやDIがしやすくなります。

### `emit_methods_with_db_argument`

`true` にすると、`Queries` が内部にDBを保持する形ではなく、**各メソッドがDB（またはTx）を引数で受け取る**形になります。

## 4. タグ出力（json/db）

### `emit_json_tags`

`true` にすると、生成structに `json:"..."` が付きます。

### `emit_db_tags`

`true` にすると、生成structに `db:"..."` が付きます。

## 5. 命名・構造体生成の制御

### `emit_exact_table_names`

`true` にすると、テーブル名の単数化などの変換を抑制し、テーブル名寄りの構造体名になります。

### `rename`

特定カラム名 → Goフィールド名 を個別に上書きします。

```yaml
gen:
  go:
    rename:
      spotify_url: "SpotifyURL"
```

### `initialisms`

頭字語（例：ID/URL/API など）の扱いを調整します。

### `omit_unused_structs`

`true` で、クエリで参照されないテーブル構造体等の生成を省略します。

## 6. NULL・ポインタ系の生成方針

### `emit_pointers_for_null_types`

`true` で、NULL可能カラムを `sql.NullString` 系ではなく `*string` などのポインタで表現します。  
（※どの `sql_package` で効くかはドライバ依存のため、採用時は生成結果の確認推奨）

### `emit_result_struct_pointers`

`true` で、`Row` 構造体を値ではなくポインタとして返す形に寄せます。

### `emit_params_struct_pointers`

`true` で、`Params` 構造体を値ではなくポインタで受け取る形に寄せます。

## 7. Enum補助コード

### `emit_enum_valid_method`

`true` で、Enum型に `Valid()` 的な検証メソッドが生成されます。

### `emit_all_enum_values`

`true` で、Enumの全値を返すヘルパーが生成されます。

## 8. 型マッピング（オーバーライド）

### `overrides`

DB型 → Go型 を上書きできます（`db_type` 指定やカラム単位指定など）。  
例：

```yaml
gen:
  go:
    overrides:
      - db_type: "pg_catalog.int4"
        go_type: "int"
```

## 9. バッチ生成ファイル名（`batch.go` の制御）

### `output_batch_file_name`

バッチ関連の生成物のファイル名を変えられます（デフォルトが `batch.go`）。  
※バッチ系アノテーションを使っていると生成される、という挙動とセットで覚えると良いです。

## 10. ビルドタグ・JSONタグケース

### `build_tags`

生成ファイルに Go ビルドタグを付けたいときに使います。

### `json_tags_case_style`

JSONタグのケース（camel/snake等）を指定します。

## まとめ（採用の指針）

- **DI/モック重視** → `emit_interface: true`
- **構造体に json を付与したい** → `emit_json_tags: true`
- **NULL表現をポインタ寄りにしたい** → `emit_pointers_for_null_types: true`（生成結果を確認）
- **preparedを明示したい** → `emit_prepared_queries: true`（pgx/v5 なら基本は不要）

## 補足

- 再生成は必ず `make gen-query`（`docker/tools/` 経由で sqlc を実行）で行い、ツールチェインバージョンを固定する
- 生成ファイル（`*.sql.go`）はリポジトリにコミットされており、CI（`gen-db-artifacts-check.yaml`）が「コミット内容 = 再生成結果」を検証。drift があればマージブロック
- 生成ファイルを手で編集しない — 次の `make gen-query` で上書きされる。変更が必要なら `sqlc.yaml` か元の SQL を編集して再生成する
- Go コード形に影響する新しい sqlc オプション（例: `emit_*` 系）を採用する際は、本ドキュメントにも理由を追記して情報を散在させないこと
