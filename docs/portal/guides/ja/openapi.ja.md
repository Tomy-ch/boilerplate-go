# OpenAPI ガイド（`openapi/`）

このディレクトリには、本プロジェクトで使用する **OpenAPI 定義**が格納されています。

- Redocly による分割構成
- oapi-codegen による Go コード生成
- オニオンアーキテクチャに準拠した境界設計

## ディレクトリ構成

この分割は利便ではなく、spec の組み立て方によって決まっている。

- `openapi.yaml` — エントリポイント。他のファイルはすべてここから `$ref` で辿られる
- `openapi.gen.yaml` — バンドル結果。生成物でありコード生成が読む（編集しない）
- `paths/` — エンドポイント定義。URL の構造を写すように配置する
- `components/` — 定義の再利用される側: `schemas/` / `parameters/` / `requests/` / `responses/`

参考資料はそれらと並ぶ通常の Markdown として置く。主題は各ファイルが自分で述べる。

## ファイルの役割

|ファイル|役割|
|---|---|
|`openapi.yaml`|エントリーポイント — `$ref` で分割ファイルを参照|
|`openapi.gen.yaml`|Redocly でバンドルされた単一ファイル — **oapi-codegen の入力**（編集禁止）|

## コード生成

```bash
make gen-api    # OpenAPI バンドル + Go コード生成
```

生成内容：

- ハンドラインターフェース（`gen/server.gen.go`）
- リクエスト / レスポンス型（`gen/type.gen.go`）
- バリデーション仕様（`gen/validate.gen.go`）

## アーキテクチャ上の位置

```mermaid
flowchart TB
    OpenAPI["OpenAPI（契約）"] --> Controller["Controller（oapi-codegen）"] --> Usecase --> Domain
```

OpenAPI は **Controller 境界の契約**を定義します。入出力形式と HTTP セマンティクスを規定し、Controller が OpenAPI 型とアプリケーション DTO の間を変換します。

## 設計方針

### 1. 分割構成（Redocly）

- エンドポイント → `paths/`
- データ構造 → `components/schemas/`
- パラメータ → `components/parameters/`

### 2. `$ref` は相対パスで統一

```yaml
# 推奨
$ref: '../components/schemas/UserResponse.yaml'

# 禁止
$ref: '#/components/schemas/UserResponse'
```

理由: Redocly バンドルとの互換性。

### 3. 1ファイル = 1責務

- schema → 1ファイル1構造体
- parameter → 1ファイル1定義
- path → 1エンドポイント単位

### 4. 実装との分離

|レイヤー|OpenAPI を知るか|
|---|---|
|Controller|はい — OpenAPI 型 ↔ DTO を変換|
|Usecase|いいえ — DTO のみ受け渡し|
|Domain|いいえ — Entity と Value Object を使用|

## API 設計ポリシー

### REST 設計（Google API Design Guide 準拠）

- リソースは複数形: `/users`
- CRUD は HTTP メソッドで表現: `GET`, `POST`, `PATCH`, `DELETE`
- 非 CRUD アクション: `POST /users/{id}:deactivate`

### 命名 / casing

ロケーション別の意図的な規約（`redocly lint` で強制）：

|ロケーション|casing|例|
|---|---|---|
|リクエスト／レスポンスの**ボディフィールド**|`camelCase`|`firstName`, `postalCode`, `nextCursor`, `hasNext`, `requestId`|
|**クエリ／パスパラメータ**|`camelCase`|`perPage`, `userId`|
|`operationId`|`PascalCase`・動詞始まり|`GetUsers`, `PostUsers`|

HTTP ヘッダーはこの表の対象外です — 慣例に従い `Train-Case`（例 `Idempotency-Key`）を用います。

ボディフィールドとパラメータは意図的に同じ `camelCase` に統一しています（パラメータをボディフィールドと揃えることで、JS/TS フロントエンドや生成 SDK とワイヤー契約を一致させる）。各ロケーション内では統一します。

### 部分更新（PATCH）— 3 状態フィールド

PATCH のリクエストボディでは、フィールドごとに **未送信**（現在値を据え置く）・**null 送信**（値をクリアする）・**値送信**（置き換える）の 3 状態を区別する必要があります。oapi-codegen の既定マッピングは optional かつ nullable なフィールドを `*T` に生成するため、「未送信」と「null」が同じ `nil` に潰れ、クリア要求と省略が区別できなくなります。

null 明示によるクリアをサポートするフィールドには、`x-go-type` 拡張と [`oapi-codegen/nullable`](https://github.com/oapi-codegen/nullable) で生成型を上書きします。`Nullable[T]` は標準の `encoding/json` デコードだけで 3 状態を保持します:

```yaml
description:
  type: string
  nullable: true
  description: 説明。null を指定すると値をクリアします。
  x-go-type: nullable.Nullable[string]
  x-go-type-import:
    path: github.com/oapi-codegen/nullable
  x-go-type-skip-optional-pointer: true   # *Nullable[T] にしない（3 状態は型自身が表現する）
```

ルール:

- 適用するのは「クリア」が意味を持つ PATCH リクエストフィールドのみ。未送信と null の区別が不要な単なる optional フィールドは既定の `*T` のままにします。
- `x-go-type-import` は常に `nullable` パッケージを指します。`T` が別の import を要する場合（例 `time.Time`）でも、`time` は oapi-codegen が自動解決するため、ここで宣言すると生成ファイルで import が重複します。
- 「OpenAPI 生成型を Usecase に渡さない」原則に従い、`nullable.Nullable[T]` は controller 境界でフレームワーク非依存の 3 状態値（`pkg/patch.Field[T]`）へ変換します。内側の層は生成型を見ず、domain には解決済みの確定値のみを渡します。

### バージョニング

URL パスバージョニング: `/v1/<リソース>`

破壊的変更 → `/v1/` と並行して `/v2/` を新設

### セキュリティ

- 認証エンドポイントには JWT（BearerAuth）を使用
- SSE endpoint だけは query の不透明な stream ticket（`StreamTicket`、`apiKey` / `in: query`）— ブラウザの `EventSource` はヘッダを設定できないため（ADR-0074 (query-ticket-stream-authentication)）。ticket は短命で 1 つの destination に束縛され、hash で保存され、`httpstack/redaction` がログから取り除く。Bearer で守れるものにこの scheme を流用しない
- リソース所有権は Usecase / Middleware で `sub` クレームにより検証
- UUID を公開識別子として使用 — セキュリティ評価は `secure-uuid.md` を参照
- IDOR 対策必須
- OpenAPI 経由のエンドポイントでは `security:` 宣言が**強制の source of truth**：`oapi` ミドルウェアの `AuthenticationFunc` は宣言のある操作だけ発火する。**例外：** `/metrics` は ops パスとして OpenAPI 検証パイプラインから skip されるため、宣言された `BasicAuth` はドキュメント上のみで、実際の認証はそのルートに付与した別の Echo `BasicAuth` ミドルウェアが担う。

## 禁止事項

- OpenAPI 定義にビジネスロジックを含めない
- パス定義内でスキーマをインライン定義しない
- ファイル間で構造を重複定義しない
- DB カラム構造を API スキーマに露出しない
- OpenAPI 生成型を Usecase に渡さない — DTO に変換する

## ガイド

- [parameter-guide.ja.md](parameter-guide.ja.md) — パラメータ定義のクイックリファレンス
- [secure-uuid.ja.md](secure-uuid.ja.md) — UUID 公開のセキュリティ評価
- [boundary-ownership.ja.md](boundary-ownership.ja.md) — `min` / `max` / 長さ制約のオーナーシップ：OpenAPI の制約は **ワイヤー契約**であり domain の業務ルールではない（両者は正当に食い違える）

## サブディレクトリのドキュメント

- [paths/README.ja.md](paths/README.ja.md) — エンドポイント定義とバージョニング
- [components/schemas/README.ja.md](components/schemas/README.ja.md) — スキーマ設計ポリシー
- [components/parameters/README.ja.md](components/parameters/README.ja.md) — パラメータ規約
- [components/requests/README.ja.md](components/requests/README.ja.md) — リクエストボディのセマンティクス（content / required）
- [components/responses/README.ja.md](components/responses/README.ja.md) — レスポンスのセマンティクス（status / description）
