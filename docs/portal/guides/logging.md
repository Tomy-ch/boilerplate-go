# logging

`internal/logging` provides a **structured logging foundation** used across the entire application.

This package is based on `zap`, while providing an abstraction layer that allows application code to handle logging **without directly depending on zap**.

The main purposes are as follows.

- Standardization of log formats
- Safe generation of log fields
- Integration with observability (trace/span)
- Ensuring testability
- Providing a framework-independent logging API

## Logger Interface

Application code uses only the **Logger interface**.

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

Each output method takes a `context.Context` and auto-injects `trace_id` / `span_id` extracted from it. Where no request-scoped span exists (DI startup, fx events, CLI bootstrap), pass `context.Background()`; injection is simply skipped. Callers never pass `trace_id` / `span_id` as explicit fields.

`Named` returns a child logger with the given name appended, and `CallerSkip` returns a logger whose caller reporting skips the given number of stack frames (useful when logging through a wrapper). `CallerSkip` **adds to** the existing skip count rather than setting it, so calling it again on an already-skipped logger stacks. Conversion of `*Field` to `zap.Field` is done internally by the unexported `convertFields` and is not part of the public interface.

This design provides:

- Encapsulation of zap dependency
- Ability to replace with mocks in tests
- No impact on application layer even if logging implementation changes

## Logger Creation

Loggers are created by output format. Pass a `Level` (output level) and the level at which stacktraces start.

The third argument is a `TraceExtractor` that pulls `trace_id` / `span_id` from the log call's `ctx`; pass `nil` to disable trace injection (e.g. CLI bootstrap loggers). At the DI root, fx wires `observability.NewTraceExtractor(obsCfg)` into `provideLogger` as its `TraceExtractor`.

```go
// JSON logger (machine-readable; production-style output)
logger := logging.NewJSONLogger(logging.LevelInfo(), logging.LevelError(), extract)
// Console logger (human-readable; development-style output)
logger := logging.NewConsoleLogger(logging.LevelDebug(), logging.LevelWarn(), extract)
// No trace injection (e.g. CLI bootstrap before DI):
logger := logging.NewJSONLogger(logging.LevelInfo(), logging.LevelError(), nil)
```

`LevelDebug` / `LevelInfo` / `LevelWarn` / `LevelError` are functions that return the corresponding `Level` value.

`Level` wraps the zap level so callers never depend on `zapcore` directly. Parse a level string (`debug` / `info` / `warn` / `error`) with `ParseLevel`:

```go
level, err := logging.ParseLevel("info")
```

Which output format and level a running process uses is decided at the DI composition root, not here: `provideLogger` in `internal/di/module/logging.go` selects the format from `APP_MODE` and the output level from `APP_LOG_LEVEL`.

|Mode|Output format|Stacktrace|
|---|---|---|
|production|JSON logger|Error and above|
|development|console logger|Warn and above|

### Attaching an Additional Core (Observability)

`LogCore` is a type alias for `zapcore.Core`. `WithCore` Tees an extra core onto an
existing `Logger`, gated to the base logger's minimum level, so the same log entries are
also emitted through that core:

```go
logger = logging.WithCore(logger, extraCore)
```

If `core` is `nil`, the original `Logger` is returned unchanged; if the passed `Logger`
is not this package's concrete `*logger` (e.g. a test fake), it is returned as-is.

This is the connection point for OpenTelemetry log export: `internal/observability`
provides `NewLogCore`, which returns an `otelzap` core bridging zap logs to OTLP when
log export is enabled (and `nil` otherwise). `provideLogger` in
`internal/di/module/logging.go` wires the two together via `WithCore`.

## Field

Log fields are created using the `Field` type.

```go
logger.Info(ctx,
    "user created",
    logging.String("user_id", "123"),
    logging.Int("age", 20),
)
```

Supported types

|Function|Type|
|---|---|
|String|string|
|Strings|[]string|
|Int|int|
|Int64|int64|
|Float64|float64|
|Bool|bool|
|Time|time.Time (converted to RFC3339Nano string)|
|DurationMs|time.Duration (converted to float64 in milliseconds)|
|Error|error|
|Stacktrace|error (converted to stack trace lines as []string)|
|Any|any|

`Stacktrace` stores the stack as a `[]string` (one element per line) so JSON viewers such
as Grafana / Loki render line breaks readably. The helper `SplitStackLines(s string) []string`
that performs this splitting is exported and reused elsewhere (e.g. the recovery middleware
splits a raw runtime stack for the `internal_stacktrace` field).

Purpose of this design

- Prevent direct usage of zap.Field
- Ensure safe field generation
- Unify API

## LogFieldBuilder

A component that consolidates log field generation for HTTP / SQL logs.

```go
type LogFieldBuilder interface {
    BuildHTTPRequestFields(req HTTPRequestLogInput) []*Field
    BuildHTTPResponseFields(resp HTTPResponseLogInput) []*Field
    BuildSQLEndFields(sql SQLFieldsEndInput) []*Field
}
```

`trace_id` / `span_id` are not built here — the `Logger` injects them from `ctx` at emit
time. `BuildSQLEndFields` additionally appends `parent_span_id` (which cannot be derived
from `ctx`) when observability is enabled and a parent span ID is present.

Creation

```go
lf := logging.NewLogFields(obsCfg, osCfg)
```

Accepts `config.ObservabilityConfig`, which gates `parent_span_id`, and `config.OperatingSystemConfig`, which supplies the timezone stamped on every event header.

Use cases

- HTTP access logs
- SQL logs

Automatically generates **structured logs**.

### Input Structs

Each Build method receives a dedicated input struct.

|Struct|Use|Key Fields|
|---|---|---|
|`HTTPRequestLogInput`|HTTP request log|EventType, Method, Path, URI, RemoteIP, Host, Scheme, Proto, UserAgent, ContentType, ContentLength, PathParams, QueryParams|
|`HTTPResponseLogInput`|HTTP response log|Method, Path, URI, Status, Latency, RequestID|
|`SQLFieldsEndInput`|SQL end log|Layer, PkgName, FuncName, SpanName, Latency, Query, Args, Err|

All input structs carry `EventAt` (event timestamp). Trace information is not carried here;
see the note under [LogFieldBuilder](#logfieldbuilder) above.

## HTTP Logging

HTTP request / response logs output the following fields.

Example (Request)

- `event_type=start`
- `method=GET`
<!-- sample-api:replace-begin -->
- `path=/v1/users`
<!-- sample-api:replace-with -->
<!-- = - `path=/v1/<resources>` -->
<!-- sample-api:replace-end -->
- `remote_ip=...`
- `trace_id=...`
- `span_id=...`

Example (Response)

- `event_type=end`
- `status=200`
- `latency_ms=12`
- `trace_id=...`
- `span_id=...`

## SQL Logging

SQL logs are emitted at the **end** of a query via `BuildSQLEndFields`.

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
- `args_count=2` (only when arguments are present)
- `internal_error=...` (only when the query failed)

Queries are output in two formats: `raw_query` (as-is) and `query_compact` (newlines /
tabs / repeated spaces collapsed to a single-line form).

## Observability Fields

There is no standalone observability builder. `trace_id` / `span_id` are injected by the
`Logger` from the log call's `ctx` (via the DI-wired `TraceExtractor`), so they appear on
every log emitted with an active span — not only HTTP / SQL logs:

- `trace_id` — injected by the `Logger` from `ctx`
- `span_id` — injected by the `Logger` from `ctx`
- `parent_span_id` — appended by `BuildSQLEndFields` (SQL only), since it cannot be derived
  from `ctx`; only when a parent span ID is present

If observability is disabled (or `ctx` carries no valid span), these fields are not output.
The `layer` / `package` / `function` fields are part of the SQL log output
(from `SQLFieldsEndInput`), not the trace attachment.

## Test Kit

In tests, use `NewTestLogger`.

```go
logger := logging.NewTestLogger(t)
```

Features

- `zaptest.NewLogger`
- Outputs test logs to `testing.T`
- No side effects

To assert on emitted logs (level / presence / caller), use the observed variants, which
return an `*observer.ObservedLogs` alongside the `Logger`:

```go
logger, observed := logging.NewObservedTestLogger(t)
loggerWithCaller, observed := logging.NewObservedTestLoggerWithCaller(t)
```

Test instance for LogFieldBuilder

```go
logging.NewTestLogFieldBuilder(t)
```

## Design Policy

Four policies shape this package. Each is detailed in the section named alongside it.

1. **Do not use zap directly** — application code depends on neither `zap.Logger` nor `zap.Field` (Logger Interface).
2. **Wrap Field** — log fields go through the `Field` type (Field).
3. **Integrate observability** — trace correlation is resolved at the logging layer, not by callers (Observability Fields).
4. **Testability** — `Logger` is an interface, so it can be mocked with `mockgen` (Test Kit).

## Log Key Constants

Log keys defined in `const.go`.

### HTTP

|Constant|Key|
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

|Constant|Key|
|---|---|
|`ErrorKey`|`error`|
|`OriginalErrorKey`|`original_error`|
|`ErrorCodeKey`|`error_code`|
|`ErrorMessageKey`|`error_message`|
|`ErrorDetailsKey`|`error_details`|
|`InternalErrorKey`|`internal_error`|
|`InternalStackTraceKey`|`internal_stacktrace`|

### Query

|Constant|Key|
|---|---|
|`RawQueryKey`|`raw_query`|
|`QueryCompactKey`|`query_compact`|
|`QueryArgsCountKey`|`args_count`|

### Job

|Constant|Key|
|---|---|
|`JobNameKey`|`job_name`|
|`JobArgsKey`|`job_args`|
|`JobErrorKey`|`job_error`|
|`JobResultKey`|`job_result`|
|`JobSkippedKey`|`job_skipped`|
|`JobScannedKey`|`job_scanned`|
|`FilterKey`|`filter`|

### Worker

|Constant|Key|
|---|---|
|`WorkerNameKey`|`worker_name`|
|`MessageIDKey`|`message_id`|
|`ReceiveCountKey`|`receive_count`|
|`PanicKey`|`panic`|

### Realtime

|Constant|Key|
|---|---|
|`StreamIDKey`|`stream_id`|
|`CloseReasonKey`|`close_reason`|
|`OpenConnectionsKey`|`open_connections`|

### Observability

|Constant|Key|
|---|---|
|`TraceIDKey`|`trace_id`|
|`SpanIDKey`|`span_id`|
|`ParentSpanIDKey`|`parent_span_id`|
|`SpanNameKey`|`span_name`|
|`LayerKey`|`layer`|
|`PackageKey`|`package`|
|`FunctionKey`|`function`|

## Security Considerations

Be careful not to output the following information in logs.

- passwords
- authentication tokens
- personal information

If necessary, apply **masking processing**.

## Test Strategy

Logging is a sealed layer — everything else depends on it and nothing here may leak zap into a caller — so its tests verify the *structured* output, never a formatted line.

- **Assert fields, not strings** — drive the subject through `NewObservedTestLogger` and assert on the observed entry's message and `ContextMap()` keys/values. Matching a rendered log line couples the test to the encoder and passes even when a key is wrong.
- **One `TestXxx` per `Field` constructor** — `String` / `Strings` / `Int` / `Int64` / `Float64` / `Bool` / `Time` / `DurationMs` / `Error` / `Stacktrace` / `Any` each get their own test asserting the produced key **and** the value type. These are the primitives every other layer's log assertions rest on.
- **Builders emit the documented key set** — `LogFieldBuilder`'s HTTP request / response and SQL builders are asserted against the key constants, so renaming a key without updating consumers fails here rather than silently changing a dashboard query.
- **Level gating** — a message below the configured level produces no entry; assert the absence, not just the presence at higher levels.
- **Stacktrace shaping** — `SplitStackLines` turns a raw runtime stack into the line array the log schema expects; assert the shape, not the specific frames (they move).
- **No secrets in fields** — this package implements **no** masking; the [Security Considerations](#security-considerations) section places that duty on the caller, which must mask before the value reaches a `Field`. There is therefore nothing to assert here, and a test that appears to verify masking in this package would be verifying something that does not exist — assert it at the call site instead.

Helpers this package offers to other layers are inventoried in [Test Kit](#test-kit); do not restate them here.
