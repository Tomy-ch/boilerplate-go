# genctxkey

`genctxkey` は、`context.Context` および `*echo.Context` に対する値の受け渡しを型安全に行うためのコードを生成するツールです。

## 概要

context に対する値の格納・取得を、以下のような問題を避けながら統一的に扱うことを目的としています。

- stringキーによる衝突
- 型安全性の欠如
- 実装のばらつき

本ツールはこれらを解決するためのヘルパー関数群を自動生成します。

## 生成されるコード

- context.Context 用
  - `SetXxx`
  - `GetXxx`
- *echo.Context 用
  - `SetXxxToEcho`
  - `GetXxxFromEcho`

## 使用方法

### 1. generate.go に定義

```go
package ctxhelper

//go:generate go run ../../../scripts/genctxkey --name ErrorHandled --type bool --out .
```

### 2. コード生成

```sh
make gen-go-code
```

## type の指定方法

### 基本型 / 同一パッケージ型

```sh
--type string
--type UserID
```

- import は不要
- Goの型としてそのまま扱われます

### 外部パッケージの型

```sh
--type "auth.Authn"
--import go-boilerplate/internal/usecase/boundary/auth
```

- `--type` には Goの型式を指定します
- `--import` でパッケージを明示します
- `--alias` は省略可能（未指定時は自動生成）
- `--test-value` は省略可能で、生成されるテストが成功値として使う Go の式を渡します。基本型でない型は
  アサーションに使えるゼロ値が無いため、ここで与えます

### 複雑な型（対応）

```sh
--type "*[]auth.Authn"
--type "map[string]auth.Authn"
```

- pointer / slice / map / generic に対応
- 型は文字列としてではなく Go の型式として扱われます

## 無効な例

```sh
--type github.com/foo/bar
```

- import path のみは無効です
- 外部型を使う場合は `--type` と `--import` を組み合わせて指定してください

## 出力仕様

- ファイル名はすべて小文字
  - 例: `errorhandled_ctx.gen.go`
- テストファイルも自動生成
  - 例: `errorhandled_ctx_test.go`

## 設計方針

### 1. generator の責務

- コード生成に専念
- 型は Go の型式として扱い、解析は行わない
- import は CLI 入力に基づいて処理

### 2. template は最小責務

- ロジックは持たない
- 表示のみ

### 3. deterministic（再現可能）

- 推測処理なし（goimports未使用）
- 常に同一結果を生成

## 編集について

生成される `.gen.go` ファイルは自動生成コードです。

- 原則として手動編集は禁止
- 変更は generator を通して行う

### 例外

依存解決の都合で以下は許可されます：

- import の調整
- alias の変更

ただし、

- 再生成時に上書きされる可能性あり
- 恒久対応は generator 側で行うこと

## CIとの関係

- 生成はローカルで実施し、結果をコミットする前提です
- CI は `make gen-go-code` を回し直し、コミット済みの出力と差があればプルリクエストを失敗させます
  （`.github/workflows/gen-go-artifacts-check.yaml`）。再生成の忘れはここで捕まります

## 補足

本ツールは以下の思想に基づいています：

- context は「制御された境界」で使用する
- 直接操作は禁止し、必ずラッパーを通す
- 生成によって一貫性を担保する

## まとめ

|項目|方針|
|------|------|
|型指定|Goの型式|
|import|CLIで明示|
|生成|再現可能|
|編集|原則禁止（例外あり）|

本ツールは、context利用の統一と安全性を保証するための基盤コンポーネントです。
