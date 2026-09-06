# OpenAPI スキーマ

`openapi/components/schemas/` は、**再利用可能な OpenAPI スキーマ定義**（リクエスト・レスポンス・セキュリティスキームのデータ構造）を格納するディレクトリです。

## ディレクトリ内容

|ファイル|種別|説明|
|---|---|---|
|`ErrorResponse.yaml`|レスポンス|統一エラーレスポンススキーマ（code / message / details / requestId）|
|`errors/`|レスポンスオブジェクト|`ErrorResponse` をラップする再利用エラーレスポンス — `apperror` 全種を網羅した HTTP ステータス別（`<理由句><コード>.yaml`）|
|`PaginationMetadataResponse.yaml`|レスポンス|オフセットページネーションのメタデータ（total / limit / offset）|
|`CursorPaginationMetadataResponse.yaml`|レスポンス|カーソル（keyset）ページネーションのメタデータ（nextCursor / hasNext）|
|`BasicAuth.yaml`|セキュリティ|HTTP Basic 認証スキーム|
|`BearerAuth.yaml`|セキュリティ|HTTP Bearer（JWT）認証スキーム|
|`StreamCursor.yaml`|Value|10 進文字列の stream 位置（SSE `id`・`after`・`Last-Event-ID`）|
|`DeliveryEvent.yaml`|Event|feature 中立な SSE business event の封筒|
|`ControlEvent.yaml`|Event|in-band の SSE 制御指示（`action` / `reason` / `retryAfterMs`）|
|`UserBaseInputRequest.yaml`|リクエスト|ユーザー入力フィールド（サンプル）|
|`UserResponse.yaml`|レスポンス|ユーザーレスポンスフィールド（サンプル）|

> `User*` ファイルは**サンプル実装**です。独自のスキーマを構築する際の命名・構造の参考として使用してください。

## 設計ポリシー

### 1ファイル = 1スキーマ

各ファイルは単一のトップレベルスキーマを定義します。1ファイルに複数のスキーマを含めないでください。

```yaml
# Good: UserResponse.yaml
type: object
required:
  - name
  - email
properties:
  name:
    type: string
  email:
    type: string
    format: email
```

### 命名規則

|要素|規則|例|
|---|---|---|
|ファイル名|PascalCase|`ErrorResponse.yaml`, `UserBaseInputRequest.yaml`|
|リクエストスキーマ|`*Request.yaml`|`UserBaseInputRequest.yaml`|
|レスポンススキーマ|`*Response.yaml`|`UserResponse.yaml`, `ErrorResponse.yaml`|
|セキュリティスキーム|説明的な名前|`BasicAuth.yaml`, `BearerAuth.yaml`（`StreamTicket` scheme だけは Spectral の override を JSON pointer で当てるため `openapi.yaml` に inline で宣言）|

### ペイロードは schema として定義し、役割で3フォルダに分ける

`redocly` のバンドルと `oapi-codegen` の生成の制約により、リクエスト／レスポンスのペイロードはすべて **schema** として定義します（OpenAPI の `requestBodies` / `responses` の **component object** 型は使いません）。パスからは `content.<media>.schema.$ref` で直接参照します。

これらは種別ではなく**役割**で3フォルダに分かれます：

|フォルダ|格納するもの|例|
|---|---|---|
|`schemas/`|基底・再利用スキーマ＋セキュリティスキーム|`UserResponse.yaml`, `ErrorResponse.yaml`, `PaginationMetadataResponse.yaml`|
|`requests/`|エンドポイントの**リクエストボディ**スキーマ（多くは `allOf` で基底を合成）|`UsersPostRequest.yaml` = `UserBaseInputRequest` ＋ `required` リスト|
|`responses/`|エンドポイントの**レスポンスボディ**スキーマ（多くは `allOf` で基底を合成）|`UsersResponse.yaml` = `UserResponse[]` ＋ ページネーションメタ|

目安：再利用できる小さな部品は `schemas/`、それを合成したエンドポイント固有の形は `requests/` / `responses/`。詳細は [`requests/README.ja.md`](../requests/README.ja.md) と [`responses/README.ja.md`](../responses/README.ja.md) を参照。

### $ref の使い方

すべての `$ref` 参照は `redocly bundle` との互換性のため、**相対 YAML パス**を使用します（`#/components/...` フラグメント形式は使用しない）。

```yaml
# パス定義から
schema:
  $ref: '../components/schemas/UserResponse.yaml'
```

### PATCH 対応

PATCH 操作には `allOf` を使って専用のラッパースキーマを作成します：

```yaml
# UserPatchRequest.yaml
allOf:
  - $ref: './UserBaseInputRequest.yaml'
```

入力構造と操作のセマンティクスを分離します。

`additionalProperties: false` は、properties を宣言している `UserBaseInputRequest.yaml` の側に置いたままにします。
ラッパーに置くと**全て**のフィールドが拒否されます。`additionalProperties` が見るのは同じスキーマオブジェクトで
宣言された properties だけで、ラッパーは 1 つも宣言していないためです。

## コアスキーマ

### ErrorResponse / ErrorResponseWithDetails

2 つのエラーエンベロープ。`ErrorResponse` は base（`details` なし）で大半のエラーステータスが
使う。`ErrorResponseWithDetails` は `details` を追加し、意図的に露出するレスポンスだけが参照する。
どの operation が `ErrorResponseWithDetails` を参照するかが、details 露出の**エンドポイントごとの
opt-in スイッチ**（edge で fail-closed に強制 — [ADR-0049 (error-details-opt-in-gate)](../../../docs/adr/0049-error-details-opt-in-gate.md) 参照）。

```yaml
# ErrorResponse.yaml (base)
type: object
required: [code, message, requestId]
properties:
  code:       # 機械可読なエラーコード（例: BAD_REQUEST）
  message:    # ユーザー向けエラーメッセージ
  requestId:  # リクエスト追跡 ID

# ErrorResponseWithDetails.yaml (base + details)
#   ...同じフィールドに加えて:
#   details:  # 公開して安全な識別子（例: 不正フィールド名）
```

Go 側の builder（`response.HTTPErrorResponse`）は `ErrorResponseWithDetails` superset を埋め込む
— 詳細は `internal/controller/error/response/` を参照。

### errors/ — 再利用エラーレスポンスオブジェクト（DRY）

各パスでブロックを複製せず、`schemas/errors/` に **HTTP ステータスごとの再利用レスポンスオブジェクトを1つずつ（`apperror` の全種を網羅）** 置き、パスはステータス項目ごと参照します：

```yaml
# パス側
responses:
  '401':
    $ref: '../../../components/schemas/errors/Unauthorized401.yaml'
```

```yaml
# schemas/errors/Unauthorized401.yaml
description: 認証が必要です。
content:
  application/json:
    schema:
      $ref: '../ErrorResponse.yaml'
```

これらは厳密には OpenAPI の**レスポンスオブジェクト**（plain schema が持てない `description` ＋ `content` を持つ）で、エラー定義をまとめるため `ErrorResponse` の隣に置いています。
各ファイルを `#/components/responses/<ファイル名>` へホイストし、`oapi-codegen` がそれを `<ファイル名>JSONResponse` という Go 型にします。
したがって**ファイル名は有効な Go 識別子（Pascal の理由句 ＋ HTTP コードのサフィックス。数字始まりは不可）**である必要があります。description がオペレーション固有の場合（＝そのオペレーションだけで意味を持ち共有できない文言のとき）のみ **inline** で残します。

**全集合（`apperror` 1種につき1つ）。** すべてのフラグメントを用意しておき、エンドポイントが必要になった瞬間に `$ref` できる状態にしています。パスには**そのオペレーションが実際に返しうるステータスだけ**を宣言します（`internal/controller/error/response/http_error.go` ＋ `internal/infrastructure/rdb/pgerror` から導出）：

|フラグメント|ステータス|`apperror`|到達経路|
|---|---|---|---|
|`BadRequest400`|400|`ErrInvalidArgument`|OpenAPI リクエスト検証（param/body のスキーマ違反）|
|`Unauthorized401`|401|`ErrUnauthenticated`|認証ミドルウェア|
|`Forbidden403`|403|`ErrPermissionDenied`|認証ミドルウェア|
|`NotFound404`|404|`ErrNotFound`|リソース不在|
|`Conflict409`|409|`ErrConflict`|`ErrAlreadyDeleted`（削除）または unique 違反 `23505`（作成・更新、例：email 重複）|
|`PayloadTooLarge413`|413|`ErrPayloadTooLarge`|アップロードサイズ上限を超えた場合の usecase 検証|
|`UnsupportedMediaType415`|415|`ErrUnsupportedMediaType`|許可しない `Content-Type` に対する usecase 検証|
|`UnprocessableEntity422`|422|`ErrValidation`|OpenAPI スキーマで捕まらない domain 検証（例：email 形式）|
|`TooManyRequests429`|429|`ErrTooManyRequests`|レートリミット|
|`ClientClosedRequest499`|499|`ErrCanceled`|リクエスト中のクライアント切断|
|`InternalServerError500`|500|`ErrInternal`|予期しないサーバエラー|
|`NotImplemented501`|501|`ErrUnimplemented`|未実装オペレーション|
|`ServiceUnavailable503`|503|`ErrUnavailable`|DB の一時障害（`40001`/`40P01`/`57014`/接続）を `pgerror` 経由|

どのオペレーションからも `$ref` されていないフラグメントは `redocly bundle` がバンドルに含めないため `no-unused-components` にも掛かりません。そのステータスを返す経路ができたら、オペレーションの `responses` に `'<コード>': { $ref: ... }` を足すだけで使えます。

### PaginationMetadataResponse

一覧エンドポイントで返される**オフセット**ページネーションのメタデータ：

```yaml
type: object
required: [total, limit, offset]
properties:
  total:   # 全件数
  limit:   # 1ページあたりの件数
  offset:  # 現在のオフセット
```

### CursorPaginationMetadataResponse

**カーソル（keyset）**ページネーションのメタデータ — オフセットに対するもう一方の戦略：

```yaml
type: object
required: [nextCursor, hasNext]
properties:
  nextCursor:  # 次ページ用の不透明カーソル。最終ページは null
  hasNext:     # 次ページが存在するか
```

再利用パターン：カーソル部品は**リソース非依存で共有**です。カーソルページネーションのエンドポイントを足すとき、新しいページネーションコンポーネントは**作りません**。既存のものを再利用します：

- クエリパラメータ：`parameters/pagination/CursorAfterParam.yaml`（`after`）＋ `parameters/pagination/CursorFirstParam.yaml`（`first`）
- レスポンス：リソースごとのラッパーで、items 配列とこのメタデータを `allOf` で合成する。feature 固有なのはそのラッパーだけ：

```yaml
# responses/users/UsersFeedResponse.yaml
allOf:
  - type: object
    required: [users]
    properties:
      users:
        type: array
        items:
          $ref: '../../schemas/UserResponse.yaml'
  - $ref: '../../schemas/CursorPaginationMetadataResponse.yaml'
```

## ルール

- パス定義内でインラインにスキーマを定義しない — 必ず `schemas/` に切り出す
- リクエストとレスポンスの兼用スキーマは極力避ける
- すべてのプロパティに `description` と `example` を記述する
- `required` で必須フィールドを明示する
- `additionalProperties: false` は、properties を持つリクエストスキーマオブジェクト自身に置く — properties が見えない `allOf` のラッパーには置かない（[PATCH 対応](#patch-対応) を参照）
- `maxLength` などの境界値は**ワイヤー契約**であり domain の業務ルールではない（オーナーが別） — [入力境界値のオーナーシップ](../../boundary-ownership.ja.md) を参照

## チェックリスト

- [ ] 1ファイル = 1スキーマになっているか
- [ ] ファイル名がスキーマの目的と一致しているか（PascalCase）
- [ ] `$ref` が相対パス形式で統一されているか
- [ ] PATCH には専用ラッパースキーマを作成しているか
- [ ] すべてのプロパティに `description` と `example` があるか
