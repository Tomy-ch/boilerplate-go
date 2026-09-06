# logging

> このファイルは canonical な英語版 [README.md](README.md) の翻訳です。直接編集せず、更新は英語版から反映してください。

`internal/logging` は、アプリケーション全体で使用する**構造化ログ基盤**を提供します。

本パッケージは `zap` をベースにしつつ、アプリケーションコードが **zap に直接依存せず**にロギングを扱えるよう抽象化レイヤーを提供します。

主な目的は以下のとおりです。

- ログフォーマットの標準化
- ログフィールドの安全な生成
- オブザーバビリティ（trace/span）との統合
- テスタビリティの確保
- フレームワーク非依存なロギング API の提供

## Logger Interface

アプリケーションコードは **Logger インターフェース**のみを使用します。

```go
type Logger interface {
    Debug(ctx context.Context, msg string, fields ...*Field)
    Info(ctx context.Context, msg string, fields ...*Field)
    Warn(ctx context.Context, msg string, fields ...*Field)
    Error(ctx context.Context, msg string, fields ...*Field)

    Named(name string) Logger
    CallerSkip(skip int) Logger
}
```

各出力メソッドは `context.Context` を受け取り、そこから抽出した `trace_id` / `span_id` を自動注入します。リクエストスコープの span が存在しない箇所（DI 起動時、fx イベント、CLI ブートストラップ）では `context.Background()` を渡します。その場合、注入は単にスキップされます。呼び出し側が `trace_id` / `span_id` を明示的なフィールドとして渡すことはありません。

`Named` は指定した名前を付与した子ロガーを返し、`CallerSkip` は caller 情報の出力時に指定した段数のスタックフレームをスキップするロガーを返します（ラッパー経由でログを出す場合に有用）。`CallerSkip` は既存のスキップ数へ**加算**するもので絶対値の設定ではないため、スキップ済みのロガーへ重ねて呼ぶと積み上がります。`*Field` から `zap.Field` への変換は、非公開の `convertFields` が内部で行い、公開インターフェースには含まれません。

この設計により、以下を実現します。

- zap 依存のカプセル化
- テストでのモック差し替え
- ロギング実装が変わってもアプリケーション層に影響を与えない

## Logger Creation

ロガーは出力フォーマットごとに生成します。`Level`（出力レベル）と、スタックトレースを付与し始めるレベルを渡します。

第 3 引数はログ呼び出しの `ctx` から `trace_id` / `span_id` を抽出する `TraceExtractor` です。`nil` を渡すと trace 注入を無効化します（例: CLI ブートストラップ用ロガー）。DI ルートでは fx が `observability.NewTraceExtractor(obsCfg)` を `provideLogger` の `TraceExtractor` として配線します。

```go
// JSON ロガー（機械可読・本番向け出力）
logger := logging.NewJSONLogger(logging.LevelInfo(), logging.LevelError(), extract)
// console ロガー（人間可読・開発向け出力）
logger := logging.NewConsoleLogger(logging.LevelDebug(), logging.LevelWarn(), extract)
// trace 注入なし（例: DI 前の CLI ブートストラップ）
logger := logging.NewJSONLogger(logging.LevelInfo(), logging.LevelError(), nil)
```

`LevelDebug` / `LevelInfo` / `LevelWarn` / `LevelError` は、対応する `Level` 値を返す関数です。

`Level` は zap のレベルをラップしており、呼び出し側が `zapcore` に直接依存しないようにします。レベル文字列（`debug` / `info` / `warn` / `error`）は `ParseLevel` でパースします。

```go
level, err := logging.ParseLevel("info")
```

実行中のプロセスがどの出力フォーマット・レベルを使うかは、ここではなく DI コンポジションルートで決定されます。`internal/di/module/logging.go` の `provideLogger` が、フォーマットを `APP_MODE` から、出力レベルを `APP_LOG_LEVEL` から選択します。

|モード|出力フォーマット|スタックトレース|
|---|---|---|
|production|JSON ロガー|Error 以上|
|development|console ロガー|Warn 以上|

### Attaching an Additional Core (Observability)

`LogCore` は `zapcore.Core` の型エイリアスです。`WithCore` は既存の `Logger` に追加の core を、ベースロガーの最小レベルでゲートしつつ Tee し、同じログエントリをその core にも出力させます。

```go
logger = logging.WithCore(logger, extraCore)
```

`core` が `nil` の場合は元の `Logger` をそのまま返します。渡された `Logger` が本パッケージの具象 `*logger` でない場合（例: テスト用の fake）も、そのまま返します。

これは OpenTelemetry ログエクスポートの接続点です。`internal/observability` が `NewLogCore` を提供し、ログエクスポートが有効なとき zap ログを OTLP へ橋渡しする `otelzap` core を返します（無効時は `nil`）。`internal/di/module/logging.go` の `provideLogger` が、両者を `WithCore` で結線します。

## Field

ログフィールドは `Field` 型で生成します。

```go
logger.Info(ctx,
    "user created",
    logging.String("user_id", "123"),
    logging.Int("age", 20),
)
```

サポートする型

|関数|型|
|---|---|
|String|string|
|Strings|[]string|
|Int|int|
|Int64|int64|
|Float64|float64|
|Bool|bool|
|Time|time.Time（RFC3339Nano 文字列へ変換）|
|DurationMs|time.Duration（ミリ秒の float64 へ変換）|
|Error|error|
|Stacktrace|error（スタックトレース行を []string へ変換）|
|Any|any|

`Stacktrace` はスタックを `[]string`（1 行につき 1 要素）として格納するため、Grafana / Loki などの JSON ビューアで改行が読みやすく表示されます。この分割を行うヘルパー `SplitStackLines(s string) []string` は公開されており、他箇所でも再利用されます（例: recovery ミドルウェアが `internal_stacktrace` フィールド向けに生ランタイムスタックを分割）。

この設計の目的

- zap.Field の直接使用を防ぐ
- 安全なフィールド生成を保証する
- API を統一する

## LogFieldBuilder

HTTP / SQL ログのフィールド生成を集約するコンポーネントです。

```go
type LogFieldBuilder interface {
    BuildHTTPRequestFields(req HTTPRequestLogInput) []*Field
    BuildHTTPResponseFields(resp HTTPResponseLogInput) []*Field
    BuildSQLEndFields(sql SQLFieldsEndInput) []*Field
}
```

`trace_id` / `span_id` はここでは構築しません。`Logger` が出力時に `ctx` から注入します。`BuildSQLEndFields` は、オブザーバビリティが有効かつ親 span ID が存在する場合に、`ctx` から導出できない `parent_span_id` のみを追加で付与します。

生成

```go
lf := logging.NewLogFields(obsCfg, osCfg)
```

`config.ObservabilityConfig`（`parent_span_id` の付与を制御）と `config.OperatingSystemConfig`（全イベントヘッダに刻むタイムゾーンを供給）を受け取ります。

ユースケース

- HTTP アクセスログ
- SQL ログ

**構造化ログ**を自動生成します。

### Input Structs

各 Build メソッドは専用の入力構造体を受け取ります。

|構造体|用途|主なフィールド|
|---|---|---|
|`HTTPRequestLogInput`|HTTP リクエストログ|EventType, Method, Path, URI, RemoteIP, Host, Scheme, Proto, UserAgent, ContentType, ContentLength, PathParams, QueryParams|
|`HTTPResponseLogInput`|HTTP レスポンスログ|Method, Path, URI, Status, Latency, RequestID|
|`SQLFieldsEndInput`|SQL 終了ログ|Layer, PkgName, FuncName, SpanName, Latency, Query, Args, Err|

すべての入力構造体は `EventAt`（イベント発生時刻）を持ちます。trace 情報はここでは保持しません。上の [LogFieldBuilder](#logfieldbuilder) の注記を参照してください。

## HTTP Logging

HTTP リクエスト / レスポンスログは以下のフィールドを出力します。

例（Request）

- `event_type=start`
- `method=GET`
<!-- sample-api:replace-begin -->
- `path=/v1/users`
<!-- sample-api:replace-with -->
<!-- = - `path=/v1/<リソース>` -->
<!-- sample-api:replace-end -->
- `remote_ip=...`
- `trace_id=...`
- `span_id=...`

例（Response）

- `event_type=end`
- `status=200`
- `latency_ms=12`
- `trace_id=...`
- `span_id=...`

## SQL Logging

SQL ログは、クエリの**終了時点**に `BuildSQLEndFields` を通じて出力されます。

### SQL End

- `event_type=end`
- `event_at=...`
- `event_tz=...`
- `layer=repository`
- `package=...`
- `function=...`
- `span_name=FindUser`
- `latency_ms=4`
- `raw_query=SELECT ...`
- `query_compact=SELECT ...`
- `args_count=2`（引数が存在する場合のみ）
- `internal_error=...`（クエリが失敗した場合のみ）

クエリは 2 つのフォーマットで出力されます。`raw_query`（そのまま）と `query_compact`（改行 / タブ / 連続空白を 1 行形式に畳んだもの）です。

## Observability Fields

独立したオブザーバビリティ用ビルダーはありません。`trace_id` / `span_id` は `Logger` がログ呼び出しの `ctx`（DI で配線された `TraceExtractor` 経由）から注入するため、アクティブな span を持つすべてのログに現れます。HTTP / SQL ログに限りません。

- `trace_id` — `Logger` が `ctx` から注入
- `span_id` — `Logger` が `ctx` から注入
- `parent_span_id` — `BuildSQLEndFields` が付与（SQL のみ）。`ctx` から導出できないため。親 span ID が存在する場合のみ

オブザーバビリティが無効な場合（または `ctx` が有効な span を持たない場合）、これらのフィールドは出力されません。`layer` / `package` / `function` フィールドは（`SQLFieldsEndInput` 由来の）SQL ログ出力の一部であり、trace の付与ではありません。

## Test Kit

テストでは `NewTestLogger` を使用します。

```go
logger := logging.NewTestLogger(t)
```

特徴

- `zaptest.NewLogger`
- テストログを `testing.T` へ出力
- 副作用なし

出力されたログ（レベル / 有無 / caller）を検証する場合は、`*observer.ObservedLogs` を `Logger` と併せて返す observed 版を使用します。

```go
logger, observed := logging.NewObservedTestLogger(t)
loggerWithCaller, observed := logging.NewObservedTestLoggerWithCaller(t)
```

LogFieldBuilder 用のテストインスタンス

```go
logging.NewTestLogFieldBuilder(t)
```

## Design Policy

本パッケージを形づくる方針は 4 つです。詳細はそれぞれ併記した節にあります。

1. **zap を直接使わない** — アプリケーションコードは `zap.Logger` にも `zap.Field` にも依存しない（Logger Interface）
2. **Field をラップする** — ログフィールドは `Field` 型を経由する（Field）
3. **オブザーバビリティを統合する** — trace の相関はロギング層で解決し、呼び出し側には持たせない（Observability Fields）
4. **テスタビリティ** — `Logger` はインターフェースなので `mockgen` でモック化できる（Test Kit）

## Log Key Constants

`const.go` で定義されるログキーです。

### HTTP

|定数|キー|
|---|---|
|`EventTypeKey`|`event_type`|
|`EventTypeStart`|`start`|
|`EventTypeEnd`|`end`|
|`EventTypeError`|`error`|
|`EventTypePanic`|`panic`|
|`EventAtKey`|`event_at`|
|`EventTzKey`|`event_tz`|
|`StatusKey`|`status`|
|`MethodKey`|`method`|
|`URIKey`|`uri`|
|`PathKey`|`path`|
|`QueryParamsKey`|`query_params`|
|`PathParamsKey`|`path_params`|
|`UserAgentKey`|`user_agent`|
|`HostKey`|`host`|
|`SchemeKey`|`scheme`|
|`ProtoKey`|`proto`|
|`RemoteIPKey`|`remote_ip`|
|`ContentTypeKey`|`content_type`|
|`ContentLengthKey`|`content_length`|
|`LatencyKey`|`latency_ms`|
|`RequestIDKey`|`request_id`|

### Error

|定数|キー|
|---|---|
|`ErrorKey`|`error`|
|`OriginalErrorKey`|`original_error`|
|`ErrorCodeKey`|`error_code`|
|`ErrorMessageKey`|`error_message`|
|`ErrorDetailsKey`|`error_details`|
|`InternalErrorKey`|`internal_error`|
|`InternalStackTraceKey`|`internal_stacktrace`|

### Query

|定数|キー|
|---|---|
|`RawQueryKey`|`raw_query`|
|`QueryCompactKey`|`query_compact`|
|`QueryArgsCountKey`|`args_count`|

### Job

|定数|キー|
|---|---|
|`JobNameKey`|`job_name`|
|`JobArgsKey`|`job_args`|
|`JobErrorKey`|`job_error`|
|`JobResultKey`|`job_result`|
|`JobSkippedKey`|`job_skipped`|
|`JobScannedKey`|`job_scanned`|
|`FilterKey`|`filter`|

### Worker

|定数|キー|
|---|---|
|`WorkerNameKey`|`worker_name`|
|`MessageIDKey`|`message_id`|
|`ReceiveCountKey`|`receive_count`|
|`PanicKey`|`panic`|

### Realtime

|定数|キー|
|---|---|
|`StreamIDKey`|`stream_id`|
|`CloseReasonKey`|`close_reason`|
|`OpenConnectionsKey`|`open_connections`|

### Observability

|定数|キー|
|---|---|
|`TraceIDKey`|`trace_id`|
|`SpanIDKey`|`span_id`|
|`ParentSpanIDKey`|`parent_span_id`|
|`SpanNameKey`|`span_name`|
|`LayerKey`|`layer`|
|`PackageKey`|`package`|
|`FunctionKey`|`function`|

## Security Considerations

以下の情報をログに出力しないよう注意してください。

- パスワード
- 認証トークン
- 個人情報

必要に応じて**マスキング処理**を適用してください。

## テスト戦略

logging は sealed layer である —— 他の全てが依存し、zap を呼び出し側へ漏らしてはならない。したがってテストは **構造化された** 出力を検証し、整形済みの行を検証しない。

- **文字列でなくフィールドを検証する** — 対象を `NewObservedTestLogger` 経由で駆動し、observed エントリのメッセージと `ContextMap()` のキー／値を検証する。レンダリング済みのログ行との照合はテストをエンコーダに結合させ、キーが誤っていても通ってしまう。
- **`Field` コンストラクタ 1 つにつき `TestXxx` 1 つ** — `String` / `Strings` / `Int` / `Int64` / `Float64` / `Bool` / `Time` / `DurationMs` / `Error` / `Stacktrace` / `Any` はそれぞれ独立したテストを持ち、生成されるキー **と** 値の型を検証する。他の全層のログ検証はこの基本要素の上に乗っている。
- **ビルダが文書化されたキー集合を出すこと** — `LogFieldBuilder` の HTTP リクエスト／レスポンスおよび SQL 用ビルダはキー定数に対して検証する。これにより、利用側を更新せずにキー名を変えた場合、ダッシュボードのクエリが黙って壊れる前にここで落ちる。
- **レベルによる抑止** — 設定レベル未満のメッセージはエントリを生成しないこと。上位レベルでの出力だけでなく、この **出ないこと** を検証する。
- **スタックトレースの整形** — `SplitStackLines` は生のランタイムスタックをログスキーマが期待する行配列へ変換する。フレームの中身（変動する）ではなく形を検証する。
- **フィールドに機密を載せない** — 本パッケージはマスキングを **実装していない**。[セキュリティ注意点](#security-considerations) はその責務を呼び出し側に置いており、値が `Field` へ到達する前にマスクするのは呼び出し側である。したがってここに検証すべきものは無く、本パッケージでマスキングを検証しているように見えるテストは存在しないものを検証していることになる。検証は呼び出し側で行う。

他層へ提供する補助は [Test Kit](#test-kit) に一覧がある。ここへ再掲しないこと。
