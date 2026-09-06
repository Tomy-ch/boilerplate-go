# error response

This package handles the generation and management of HTTP error responses.

## Role

- **Unified format**: Returns error responses from the API in a consistent structure, simplifying client-side handling
- **Reusable from handlers / middleware**: Endpoint handlers and middleware call public functions to return errors
- **Type-safe responses**: Responses are built using types from `gen/type.gen.go` (auto-generated from OpenAPI)

## File Structure

|File|Role|
|---|---|
|`error_response.go`|Response assembly, public functions|
|`http_error.go`|Error code / message constants, apperror → HTTP mapping|
|`gen/type.gen.go`|Type definitions auto-generated from OpenAPI (do not edit)|

## Error Response Structure

```json
{
  "code": "INTERNAL_ERROR",
  "message": "An unexpected error occurred on the server. Please try again later.",
  "details": ["Specific error description"],
  "requestId": "123e4567-e89b-12d3-a456-426614174000"
}
```

|Field|Description|
|---|---|
|`code`|Application-level error identifier|
|`message`|User-friendly message for end users|
|`details`|Public-safe identifiers giving error specifics (e.g., invalid field names). Never reason texts or raw input values|
|`requestId`|Unique ID for request tracing|

HTTP status codes are returned in the response header, and internal information such as stack traces is output only to logs.

## Public Functions

|Function|Description|
|---|---|
|`NewHTTPErrorFromAppError`|Generate HTTP error response from `apperror` (unknown errors fall back to 500)|
|`NewHTTPErrorFromStatus`|Generate error response from HTTP status code (unknown status falls back to 500)|

### `apperror.Meta` Overrides

When the error carries an `apperror.Meta` (see `internal/apperror` README), `NewHTTPErrorFromAppError` overlays it on the defaults resolved from the sentinel classification:

|Response field|Without Meta|With Meta|
|---|---|---|
|HTTP status|From sentinel classification|From sentinel classification (**Meta never changes it**)|
|`code`|Status default|`Meta.Code()` if non-empty, else status default|
|`message`|Status default|`Meta.Message()` if non-empty, else status default|
|`details`|Explicit `details` argument|Explicit `details` argument if given, else `Meta.Details()`|

This builder is **request-agnostic**: it always constructs the superset envelope
(`gen.ErrorResponseWithDetails`) and attaches whatever `details` the error holds. Whether
those `details` actually reach the client is decided downstream by the `errorhandler`'s
per-endpoint opt-in gate (fail-closed), the same way `requestId` is left empty here and filled
at the edge. See the `errorhandler` README and [ADR-0049 (error-details-opt-in-gate)](../../../../docs/adr/0049-error-details-opt-in-gate.md).

## Error Code and HTTP Status Mapping

### apperror → HTTP Mapping

The HTTP Status column mirrors the sentinel classification owned by [`internal/apperror/README.md`](../../../apperror/README.md) § Mapping Table; the Error Code column is this layer's own addition.

|apperror|HTTP Status|Error Code|
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
|Other|500 Internal Server Error|`INTERNAL_ERROR`|

405 Method Not Allowed is absent from this table on purpose: the application never chooses the
request method, so there is no sentinel for it. It is resolved by status alone — see
[Statuses Covered by `errorMeta`](#statuses-covered-by-errormeta).

### Error Code List

|Code|Default Message|
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

### Statuses Covered by `errorMeta`

`errorMeta` is keyed by HTTP status and an unlisted status falls back to the 500 entry — the
status included, so an unmapped status reaches the client as `500` / `INTERNAL_ERROR` rather than
as itself. A status belongs in the table when the application can actually produce it:

1. **Reachable from an `apperror` sentinel** — every status in the mapping table above
2. **Produced by the always-on HTTP stack with no sentinel behind it** — currently 405 alone,
   raised by the router (`echo.ErrMethodNotAllowed`) when the path matches but the method does not

Any other status is genuinely unexpected, and the 500 fallback is the right answer for it. Adding
a status that neither source can produce would be speculative.

## Configuration Changes

- **Modify response logic**: Edit `error_response.go`
- **Add error codes / messages**: Add constants and mapping to `http_error.go`
- **Update type definitions**: After updating the OpenAPI spec, regenerate with `make gen-api`

## Test Strategy

This package is a mapping table, not an adapter, so nothing in [`../../README.md`](../../README.md) applies; the viewpoints are its own.

- **Every entry, all columns** — for each `apperror` sentinel in the mapping, one case asserts status, `code` and message together (`lookupErrorMetaByAppError`), and for each status one case asserts the same triple by status (`lookupErrorMetaByHTTPStatus`). Adding a status (as `410` was) means adding both cases.
- **The fallback is its own case** — an unmapped error lands on the 500 meta; assert it explicitly, since a sentinel forgotten here falls into it silently.
- **Meta overrides only the code** — an `apperror.Meta` on the error replaces the `code` (and the message when given) but never the status the sentinel decided; assert both halves.

## Notes

- Do not include stack traces or sensitive information in `Details`. Internal information should only be output to logs
- Maintain consistency between error codes and HTTP status codes
- Do not manually edit files under `gen/`
