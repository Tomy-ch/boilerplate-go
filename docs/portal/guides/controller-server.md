# server

`server` is the package that **creates and configures the HTTP server** and provides HTTP request logging / parameter-extraction utilities for the Echo context.

It builds the Echo instance and the `http.Server` that serves it with the timeouts from `ServerConfig`; middleware application and DI-lifecycle (start / shutdown) registration are handled by other packages (see Role).

## Role

- Echo instance creation (`NewAppServer`)
- HTTP server creation (`NewHTTPServer`) -- serves the Echo instance and applies the timeouts from `ServerConfig`
- Build HTTP request log input (`BuildHTTPRequestLogInput`) -- the shared construction point for error / recovery log paths
- Provide Echo context parameter extraction utilities (`ExtractPathParams` / `ExtractQueryParams`) and Echo response access (`ResponseOf`)

This package **does not define middleware directly**. Middleware application is handled by `internal/controller/httpstack` and `internal/di/server/extension`. Registering the server's start / shutdown with the DI lifecycle (`lifecycle.Registrar`) is handled by `internal/di/server/hook`, not this package.

## Test Strategy

The package holds two kinds of subject, tested differently.

### Echo context utilities

`BuildHTTPRequestLogInput` / `ExtractPathParams` / `ExtractQueryParams` / `ResponseOf` are driven against a real context (`echo.New().NewContext(httptest.NewRequestWithContext(...), httptest.NewRecorder())`) — there is nothing to mock:

- **Empty vs populated** — no path values / no query yields a **non-nil empty** map, not nil (callers range over it unconditionally); populated input is extracted in full, including a repeated query key keeping every value.
- **Field mapping** — `BuildHTTPRequestLogInput` copies each request attribute into the matching log-input field for the given event type. Assert per field; `EventAt` is clock-derived, so assert it is non-zero rather than pinning a literal.
- **`ResponseOf` unwrap chain** — a response writer wrapped by a middleware still resolves to the *same* `*echo.Response` (`assert.Same`), and a writer that does not lead back to Echo returns `nil`. Every caller's degradation path (`logging` / `redmetrics` / `forcejson` / `cookie` / `errorhandler`) rests on that nil branch, and the production stack never produces it — so it is pinned here, at the definition, not only at the call sites.

### Server construction

- `NewHTTPServer` copies each timeout from `ServerConfig` onto the `http.Server` and keeps the given Echo as `Handler`. Assert per field — a mis-mapped timeout is silent at runtime.
- Listening, serving, and graceful shutdown are **not** exercised here; they belong to the lifecycle hooks in [`internal/di/server/hook`](../../di/server/hook/README.md).

## Notes

- The Echo instance created by `NewAppServer` receives middleware from subsequent extensions -- this package **does not define middleware directly**
- Echo v5 concentrates server start / stop in `echo.StartConfig`, whose blocking model does not fit the DI container's separate start / stop hooks; this project therefore owns an `http.Server` (`NewHTTPServer`) and that is also where the request timeouts live (see [ADR-0022 (echo-http-framework)](../../../docs/adr/0022-echo-http-framework.md))
- `Context.Response()` returns an `http.ResponseWriter` in Echo v5; use `ResponseOf` when the Echo-specific status or `Before` / `After` hooks are needed
- Use `logging.Logger` for logging; direct use of zap is prohibited (sealed layer)
- Graceful shutdown timeout follows `ServerConfig` -- ensure the configuration is correct
- The recovered-panic flag previously exposed here (`MarkRecovered` / `IsRecovered`) has moved to `internal/controller/ctxhelper` as the typed helpers `SetRecoveredToEcho` / `GetRecoveredFromEcho`; depend on `ctxhelper` directly rather than going through this package
