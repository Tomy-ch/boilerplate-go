# error response

このパッケージは、HTTPエラーレスポンスの生成と管理を行います。

## 役割

- **統一フォーマットの提供**: APIで返すエラーレスポンスを一貫した構造で返し、クライアント側の取り扱いを簡単にします
- **ハンドラ / ミドルウェアからの再利用**: 各エンドポイントのハンドラやミドルウェアは、公開関数を呼び出してエラーを返します
- **型安全なレスポンス**: `gen/type.gen.go`（OpenAPI から自動生成）の型を利用してレスポンスを組み立てます

## ファイル構成

|ファイル|役割|
|---|---|
|`error_response.go`|レスポンス組み立て、公開関数|
|`http_error.go`|エラーコード / メッセージ定数、apperror → HTTP マッピング|
|`gen/type.gen.go`|OpenAPI から自動生成された型定義（編集不可）|

## エラーレスポンスの構造

```json
{
  "code": "INTERNAL_ERROR",
  "message": "サーバーで予期しないエラーが発生しました。時間をおいて再度お試しください。",
  "details": ["具体的なエラーの説明"],
  "requestId": "123e4567-e89b-12d3-a456-426614174000"
}
```

|フィールド|説明|
|---|---|
|`code`|アプリケーションレベルのエラー識別子|
|`message`|エンドユーザー向けのメッセージ|
|`details`|エラー詳細を示す公開可能な識別子（例: 不正フィールド名）。理由文や入力値そのものは入れない|
|`requestId`|リクエスト追跡用のユニークID|

HTTPステータスコードはレスポンスヘッダで返し、スタックトレース等の内部情報はログにのみ出力します。

## 公開関数

|関数|説明|
|---|---|
|`NewHTTPErrorFromAppError`|`apperror` から対応するHTTPエラーレスポンスを生成（未知のエラーは 500 にフォールバック）|
|`NewHTTPErrorFromStatus`|HTTPステータスコードから対応するエラーレスポンスを生成（未知のステータスは 500 にフォールバック）|

### `apperror.Meta` による上書き

エラーが `apperror.Meta` を運んでいる場合（`internal/apperror` README 参照）、`NewHTTPErrorFromAppError` はセンチネル分類で解決した既定値の上に Meta を重ねます。

|レスポンスフィールド|Meta なし|Meta あり|
|---|---|---|
|HTTP ステータス|センチネル分類から解決|センチネル分類から解決（**Meta では変わらない**）|
|`code`|ステータス既定値|`Meta.Code()` が非空ならそれ、空なら既定値|
|`message`|ステータス既定値|`Meta.Message()` が非空ならそれ、空なら既定値|
|`details`|明示引数 `details`|明示引数があればそれ、無ければ `Meta.Details()`|

この builder は **request 非依存**です。常に superset エンベロープ（`gen.ErrorResponseWithDetails`）を
組み立て、エラーが持つ `details` を付与します。その `details` が実際にクライアントへ届くかは、
下流の `errorhandler` のエンドポイントごとの opt-in ゲート（fail-closed）が決めます。`requestId` を
ここで空にして edge で埋めるのと同じ構図です。`errorhandler` README と
[ADR-0049 (error-details-opt-in-gate)](../../../../docs/adr/0049-error-details-opt-in-gate.md) を参照。

## エラーコードとHTTPステータスの対応

### apperror → HTTP マッピング

HTTP ステータス列は [`internal/apperror/README.ja.md`](../../../apperror/README.ja.md) の対応表が所有するセンチネル分類を写したものです。エラーコード列は本層独自の追加です。

|apperror|HTTPステータス|エラーコード|
|---|---|---|
|`ErrInvalidArgument`|400 Bad Request|`BAD_REQUEST`|
|`ErrUnauthenticated`|401 Unauthorized|`UNAUTHORIZED`|
|`ErrPermissionDenied`|403 Forbidden|`ACCESS_DENIED`|
|`ErrNotFound`|404 Not Found|`NOT_FOUND`|
|`ErrConflict`|409 Conflict|`RESOURCE_CONFLICT`|
|`ErrGone`|410 Gone|`GONE`|
|`ErrPayloadTooLarge`|413 Payload Too Large|`PAYLOAD_TOO_LARGE`|
|`ErrUnsupportedMediaType`|415 Unsupported Media Type|`UNSUPPORTED_MEDIA_TYPE`|
|`ErrValidation`|422 Unprocessable Entity|`VALIDATION_FAILED`|
|`ErrTooManyRequests`|429 Too Many Requests|`TOO_MANY_REQUESTS`|
|`ErrCanceled`|499 Client Closed Request|`CLIENT_CLOSED_REQUEST`|
|`ErrUnimplemented`|501 Not Implemented|`NOT_IMPLEMENTED`|
|`ErrUnavailable`|503 Service Unavailable|`SERVICE_UNAVAILABLE`|
|その他|500 Internal Server Error|`INTERNAL_ERROR`|

405 Method Not Allowed を意図的に載せていません。リクエストメソッドはアプリケーションが選ぶものではなく、
対応する sentinel が存在しないためです。ステータス単独で解決されます
（[`errorMeta` の収録基準](#errormeta-の収録基準)を参照）。

### エラーコード一覧

|コード|デフォルトメッセージ|
|---|---|
|`BAD_REQUEST`|入力内容に誤りがあります。再度ご確認ください。|
|`UNAUTHORIZED`|ログインが必要です。ログインして再度お試しください。|
|`ACCESS_DENIED`|この操作を行う権限がありません。|
|`NOT_FOUND`|お探しの情報が見つかりませんでした。|
|`METHOD_NOT_ALLOWED`|許可されていないリクエスト方法です。|
|`RESOURCE_CONFLICT`|既に同じ情報が登録されています。|
|`PAYLOAD_TOO_LARGE`|ファイルサイズが大きすぎます。上限を超えないファイルで再度お試しください。|
|`UNSUPPORTED_MEDIA_TYPE`|サポートされていないファイル形式です。形式をご確認のうえ再度お試しください。|
|`VALIDATION_FAILED`|入力内容の検証に失敗しました。修正して再度お試しください。|
|`TOO_MANY_REQUESTS`|リクエストが多すぎます。しばらくしてから再度お試しください。|
|`CLIENT_CLOSED_REQUEST`|リクエストがキャンセルされました。|
|`INTERNAL_ERROR`|サーバーで予期しないエラーが発生しました。時間をおいて再度お試しください。|
|`NOT_IMPLEMENTED`|この機能は提供されていません。|
|`SERVICE_UNAVAILABLE`|現在この機能はご利用いただけません。しばらくしてから再度お試しください。|

### `errorMeta` の収録基準

`errorMeta` は HTTP ステータスをキーに持ち、未収録のステータスは 500 のエントリへフォールバックします。
このときステータス自体も 500 になるため、未収録のステータスはそのままではなく
`500` / `INTERNAL_ERROR` としてクライアントに届きます。収録の基準は「アプリケーションが実際に出しうるか」です。

1. **`apperror` sentinel から到達するステータス** — 上のマッピング表にあるものすべて
2. **常時オンの HTTP スタックが sentinel 無しで出すステータス** — 現状は 405 のみ。
   パスは一致したがメソッドが一致しない場合にルータが送出する（`echo.ErrMethodNotAllowed`）

いずれにも該当しないステータスは真に想定外であり、500 へのフォールバックが正しい応答です。
どちらの経路でも生成されないステータスを足すのは、根拠のない先回りになります。

## 設定変更方法

- **レスポンスロジックの修正**: `error_response.go` を編集
- **エラーコード / メッセージの追加**: `http_error.go` に定数とマッピングを追加
- **型定義の更新**: OpenAPI 仕様を更新後、`make gen-api` で再生成

## テスト戦略

この package は adapter ではなく写像表なので、[`../../README.ja.md`](../../README.ja.md) の観点は当てはまらない。観点はここで持つ。

- **全エントリを全列で** — 写像にある `apperror` の sentinel ごとに status・`code`・message を一度に表明する（`lookupErrorMetaByAppError`）。status ごとにも同じ 3 つ組を status から引いて表明する（`lookupErrorMetaByHTTPStatus`）。status を足すとき（`410` がそうだった）は両方のケースを足す。
- **fallback は独立したケース** — 未登録のエラーは 500 のメタに落ちる。sentinel の登録忘れは黙ってここへ落ちるので、明示的に表明する。
- **Meta が置き換えるのは code だけ** — エラーに載った `apperror.Meta` は `code`（与えられれば message も）を置き換えるが、sentinel が決めた status は変えない。両側を表明する。

## 注意点

- `Details` にスタックトレースや機密情報を含めないこと。内部情報はログにのみ出力する
- エラーコードとHTTPステータスの整合性を保つこと
- `gen/` 配下のファイルは手動で編集しないこと
