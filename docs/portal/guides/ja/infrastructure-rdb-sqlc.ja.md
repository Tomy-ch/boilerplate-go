## sqlc パッケージ

このディレクトリは、[sqlc](https://docs.sqlc.dev/) を使用して SQL から Go コードを生成した結果と、
それらを補助する **SQL 実行関連ユーティリティ** を提供します。

このパッケージの役割は次の2つです。

1. **sqlc によって生成されたコードの配置**
2. **生成コードを実用的に扱うための補助関数の提供**

## 役割

本プロジェクトの DB アクセスは、手書きの SQL 文字列ではなく sqlc が生成した型安全なクエリコードを経由します。これにより、スキーマと形が合わなくなったクエリは実行時ではなく生成・コンパイル時に失敗します。本パッケージは、その生成コード（`gen/`）と、DB 固有の SQL の癖（特に LIKE / ILIKE のメタ文字エスケープとパターン生成）を吸収する薄い実行ヘルパーをまとめて配置する唯一の置き場です。両者をここに閉じ込めることで、生の SQL 仕様やエスケープ規則が domain / usecase 層へ漏れず（上位層は型付きクエリ関数だけを見る）、SQL やスキーマ変更時の再生成ポイントを一本化できます。

## 生成コードについて

- `gen/` ディレクトリ以下には、sqlc によって生成された Go コードが含まれます。
- これらのコードは **手動で編集してはいけません。**
- SQL クエリやスキーマを変更した場合は、`sqlc generate` を実行してコードを再生成してください。

生成コードは次の機能を提供します。

- SQL クエリの型安全な実行
- パラメータのバインディング
- DB レコードの Go 型へのマッピング

## 補助ユーティリティ

このディレクトリには、sqlc 生成コードを補完するための **SQL 実行補助関数** が含まれます。

主な用途は次の通りです。

- LIKE / ILIKE 検索用パターン生成
- PostgreSQL LIKE エスケープ

これらは **Infrastructure 層のユーティリティ**として利用されます。

## ディレクトリ構成

## LIKE 検索ヘルパー

PostgreSQL の `LIKE` / `ILIKE` 検索で利用するパターン生成を補助する関数を提供します。

### EscapeForLike

`LIKE` 検索で特別な意味を持つ次の文字をエスケープします。

- `%`
- `_`
- エスケープ文字自身

```go
escaped := EscapeForLike(keyword, DefaultLikeEscapeChar)
```

`DefaultLikeEscapeChar` は次の値です。

```go
const DefaultLikeEscapeChar = "\\"
```

SQL 側では次のように使用します。

```sql
WHERE name ILIKE $1 ESCAPE '\\'
```

### LIKE パターン生成

LIKE 検索用のパターン生成ヘルパーです。

#### 前方一致

```go
pattern := WrapPrefixLikePattern(token)
```

結果: `token%`

#### 後方一致

```go
pattern := WrapSuffixLikePattern(token)
```

結果: `%token`

#### 部分一致

```go
escaped := EscapeForLike(keyword, DefaultLikeEscapeChar)
pattern := WrapContainsLikePattern(escaped)
```

結果: `%escaped%`

##### **重要**

`LIKE` 検索では必ず `EscapeForLike` を実行してからパターンを生成してください。

## 設計方針

このパッケージは **Infrastructure 層専用の補助ライブラリ**です。

次の責務を持ちます。

- SQL 実行に関するユーティリティ
- sqlc 生成コードの補助処理
- DB 固有仕様の吸収

一方で、次の責務は持ちません。

- ドメインロジック
- ユースケースロジック
- HTTP / Controller 処理

これらはそれぞれ

- Domain 層
- Usecase 層
- Controller 層

に配置します。
