# migrations

`database/migrations` は、**golang-migrate によるデータベースマイグレーションファイル**を格納するディレクトリです。

## ファイル生成

新しいマイグレーションファイルは以下のコマンドで生成します。

```bash
make new-migrate-<name>
```

例：

```bash
make new-migrate-create_orders
```

これにより、連番付きの up / down ペアが自動生成されます。

```text
000011_create_orders.up.sql
000011_create_orders.down.sql
```

## ファイル命名規則

```text
{6桁連番}_{説明}.up.sql    # アップグレード（適用）
{6桁連番}_{説明}.down.sql  # ダウングレード（ロールバック）
```

- 連番は `000001` から始まる6桁ゼロ埋め
- 説明はスネークケースで内容を簡潔に表す
- up と down は必ずペアで作成する

## マイグレーション実行

|コマンド|説明|
|---|---|
|`make db-migrate-up DB=<name>`|すべての未適用マイグレーションを適用|
|`make db-migrate-down DB=<name>`|直前のマイグレーションをロールバック|

CLI からも実行可能です。

```bash
./server migrate-up
./server migrate-down
```

## ルール

- **既存のマイグレーションファイルを変更しない** — 適用済みのファイルを変更するとハッシュ不整合が発生する
- **必ず新しいファイルを作成する** — スキーマ変更は常に新規マイグレーションで行う
- **up と down はペアで作成する** — down がないとロールバックできない
- **down は up の逆操作を正確に記述する** — テーブル作成なら DROP、カラム追加なら DROP COLUMN
- **冪等性を意識する** — `IF NOT EXISTS` / `IF EXISTS` を活用する
- **連番に欠番を作らない** — CI でギャップ検出される（`migration-check.yaml`）

## 参照マスタの基本形

参照マスタ（行を migration が固定し、書き込み API を持たないテーブル）は次の列を持ちます。

```sql
id UUID, name VARCHAR(100), code SMALLINT, sort_key SMALLINT, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ
```

`code` と `sort_key` はどちらも小さな整数を持つため、区別は推測に任せず明示しておく必要があります。
両者は答えている問いが違い、動く理由も違います。

|列|何であるか|API へ出すか|
|---|---|---|
|`code`|行の**静的な別名**。アプリケーションコード・SQL・API クライアントはこの値で行を指し、一度割り当てたら動かさない。|**出す**——クライアントが送受信するのはこちら|
|`sort_key`|**並び順を冪等にするためのキー**。表示順を決めるのは `ORDER BY sort_key` であり、意図する順序が変われば自由に動かしてよい。|**出さない**——レスポンスの配列順が既に表現している|

どちらも `UNIQUE`。ここから出る帰結は、**`code` で並べてはいけない**ということです。`code` で並べると
2 つの役割が癒着し、表示順を変える唯一の手段が `code` の振り直しになります。それは `code` を定数として
持つ全クライアントと、`database/dml/` の `WHERE ... code = ...` を同時に、しかも静かに壊します。

参照マスタの並び順ではなく、**利用者が決めて API が返す**順序は、このどちらでもありません。
上の対から外すために `display_sort` と名付けます。

## CI チェック

`migration-check.yaml` ワークフローにより、以下が自動検証されます。

- 連番の重複がないこと
- 連番に欠番がないこと
- up / down のペアが揃っていること

## 注意点

- このディレクトリには DDL（テーブル定義・スキーマ変更）のみ配置する
- DML（データ操作）は `database/dml/` に配置する
- シードデータは `database/seed/` に配置する
- 初期化用 SQL（DB 作成・拡張有効化）は `docker/database/sql/` に配置する
