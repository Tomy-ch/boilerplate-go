---
status: accepted
date: 2026-07-04
deciders: [maintainers]
tags: [contract, openapi, tooling]
---

# ADR-0013: 仕様をモジュラーな Redocly ファイルで作成し、バンドルしてから生成する

## ステータス

accepted

## 背景

[ADR-0012](0012-openapi-first.ja.md) はワイヤー契約の唯一の真実のソースとして OpenAPI を確立している。仕様が大きくなると、単一のフラットな YAML ファイルを保守することは現実的ではなくなる。パス、スキーマ、パラメーター、レスポンスオブジェクトがすべて一箇所に混在し、コードレビュー、再利用、ナビゲーションが困難になる。それでも、バンドルされた単一ファイルの出力は oapi-codegen（コード生成）と Redoc（ドキュメントレンダリング）のダウンストリームでは引き続き必要である。

## 決定

仕様を**モジュラー Redocly プロジェクト**として作成する。エントリポイントは `openapi/openapi.yaml` で、`paths/`、`components/schemas/`、`components/parameters/`、`components/requests/`、`components/responses/` 配下のファイルへの `$ref` ポインターを使用する。1 ファイル = 1 責任（スキーマ 1 件につき 1 ファイル、エンドポイント 1 件につき 1 ファイル、パラメーター 1 件につき 1 ファイル）。

ビルドパイプラインは以下の通り：

1. **Lint** — `redocly lint openapi/openapi.yaml` がモジュラーソースを `redocly.yaml`（命名規約、必須メタデータ、未使用コンポーネントなし）に対して検証する。
2. **Bundle** — `redocly bundle openapi/openapi.yaml -o openapi/openapi.gen.yaml` がすべての `$ref` ポインターを単一のフラットファイルに解決する。
3. **Generate** — oapi-codegen が `openapi/openapi.gen.yaml` を読み取り Go コードを生成する（[ADR-0014](0014-oapi-codegen-strict-server.ja.md) 参照）。
4. **Docs** — `redocly build-docs openapi/openapi.yaml --output docs/openapi/index.html` が静的 API ドキュメントを生成する。

すべての `$ref` 値は相対パスを使用しなければならない（例: `../components/schemas/UserResponse.yaml`）。Redocly のバンドラーが相対ファイル参照を正しく解決するため、インラインコンポーネント参照（`#/components/...`）は禁止される。

## 影響

### ポジティブな影響

- 分割されたファイルは `$ref` によって独立してレビュー可能かつ再利用可能である。
- 命名規約（ボディフィールドはキャメルケース、パラメーターはキャメルケース、`operationId` はパスカルケース）がコード生成実行前のリント時に強制される。
- `make gen-api` はバンドル・ドキュメント・コード生成を順に実行する単一のコマンドである（`redocly lint` は `make oapi-lint` で別途実行され、CI ゲートでもある）。
- ドキュメントはコードと同じソースから生成される。
- ハンドラーコードはバンドルされた仕様から生成されるため、YAML 定義が常に実装に先行する形になり、定義と実装の乖離が本番環境に流れ込まない。

### ネガティブな影響

- コントリビューターは Redocly の分割ファイル規約を習得し、常に相対 `$ref` パスを使用しなければならない。フラットな OpenAPI YAML と比較して非標準の作成スタイルである。
- バンドルされた `openapi.gen.yaml` ファイルは生成された出力であり、手動で編集してはならない。部分的な編集の際に 2 つのファイルが乖離すると混乱を招く可能性がある。（CI の `gen-oapi-artifacts-check` ワークフローが乖離を検知して PR を失敗させるため、手動編集による混乱はマージ前に発見可能である。）

## 検討した代替案

### 単一のフラット OpenAPI ファイル

Redocly ツールチェーンの依存関係を回避できる。却下：単一ファイルはスケールで保守不可能になり、`$ref` によるオブジェクト単位の再利用が禁止される。

### JSON ポインターフラグメントを使用したインライン $ref

`$ref: '#/components/schemas/UserResponse'` はすべてを 1 ファイルにまとめ、標準的な OpenAPI の慣行である。却下：Redocly バンドラーはモジュラー構造全体で相対 `$ref` ポインターを正しく解決するために別ファイルを必要とする。

## 補足

- ビルドターゲットの定義: [`.makefiles/openapi/gen.mk`](../../.makefiles/openapi/gen.mk)。
- リントルールと命名規約の強制: [`redocly.yaml`](../../redocly.yaml)。
- モジュラーディレクトリ構造の説明: [`openapi/README.md`](../../openapi/README.ja.md)。
- `openapi/openapi.gen.yaml` は生成された出力であり、手動で編集してはならない（[`docs/rules.md`](../rules.ja.md) の生成ファイルルール参照）。
- 親の決定: [ADR-0012](0012-openapi-first.ja.md)（OpenAPI ファースト）。
