# REST Subsystem Design Reference

This document consolidates the REST (HTTP) scaffold's **role theory, state transitions, implementation locations, what an integrator must implement, and glossary** into a single reference, derived from a close reading of the implementation. For the handler-authoring detail see the [handler README](../../internal/controller/handler/README.md); the worker and job are its async / CLI siblings — see [worker.md](worker.md) and [job.md](job.md).

---

## 1. Role theory (what, and what for)

REST is the **"request-in driving adapter," the synchronous HTTP entry point into the Usecase layer**, and the original peer that the worker ("message-in") and job ("command-in") were modelled after. The Echo server is the transport; the handler is a thin template that adapts HTTP I/O to a usecase call. **All business logic stays in the usecase layer** — the handler binds, calls one usecase method, and shapes the response.

Responsibility split (who owns what):

| Component | Layer | Responsibility | Does NOT hold |
| --- | --- | --- | --- |
| **HTTP server** (`server.NewAppServer` / `server.NewHTTPServer`) | controller | Echo instance + the `http.Server` that serves it: read·write·idle timeouts / route table via generated `RegisterHandlers` | business logic, middleware policy |
| **middleware chain** (`httpstack/*`) | controller | cross-cutting concerns in a fixed order: uri(pre) → timeout(pre) → bodyLimit(pre) → requestID → observability → recovery → cors → security → openapi → forcejson → httpredmetrics → logging → cookie | business logic |
| **handler** (`controller/handler/**`) | controller | parse typed request (`StrictHandler`) → call **one** usecase method → convert DTO → `gen` response | business logic, persistence, tx |
| **error handler** (`httpstack/errorhandler`) | controller | map `apperror` / Echo / OpenAPI errors → HTTP status + code, unified error body | business policy |
| **usecase** | usecase | **all** business logic, transaction boundaries, domain orchestration, error policy | HTTP, framework, presentation |
| **DI server + hook** (`di/server`) | di | compose Echo + ordered middleware (by priority) + lifecycle (listen / graceful shutdown) | business logic |
| **ServerConfig** | config | host / port / read-header·read·write·idle timeouts, per-request deadline (`SERVER_REQUEST_TIMEOUT`), body-size limit (`SERVER_BODY_LIMIT_MB`) | business logic |

Design principles (invariants):

- **Contract-first.** Routes, request, and response types are generated from OpenAPI (`make gen-api`); handlers implement the generated `StrictServerInterface`. Handlers and usecases must not precede the contract.
- **Thin handler.** A handler is a template (bind → usecase → present); it holds no business logic and **does not import infrastructure** (depguard `maintain_a_sound_controller`).
- **Ordered middleware by priority.** Each middleware declares an integer priority in its `*_di.go`; the extension engine sorts and applies `Pre`/`Use` deterministically, so the chain order is data, not call-site ordering.

---

## 2. State transitions

### 2.1 server lifecycle (`cli/server.RunServer` + fx + signal)

```mermaid
stateDiagram-v2
    [*] --> Building: NewApplicationCore() → fx.New (config→logging→o11y→db→infra→usecase→controller→server)
    Building --> Wired: BindHandler×N (RegisterHandlers) + ApplyExtends (sort & apply middleware) + RegisterHTTPServerHooks
    Wired --> Listening: fx OnStart → net listen port → go srv.Serve(ln)
    Listening --> Serving: request → middleware chain → handler → response (resident)
    Serving --> Listening: response sent
    Listening --> Draining: SIGINT/SIGTERM → ctx.Done() in RunServer
    Draining --> Stopped: srv.Shutdown(stopCtx) drains in-flight within shutdownTimeout
    Stopped --> [*]: fx OnStop done → process exits

    note right of Draining
      stopCtx is a fresh context timed from shutdown start (not consumed by uptime).
    end note
    note right of Building
      a metrics server is started in non-production mode only (ResolveMetricsStop).
    end note
```

Realtime Delivery extends this lifecycle rather than replacing it. When at least one feature adapter
is wired, its DI module contributes serve-lifecycle participants — a startup probe, the lease and
queue provisioner, the consumer and heartbeat runners, and a drainer — which `internal/di/server/hook`
runs in the phases above ([realtime-delivery.md §2.5](realtime-delivery.md#25-serve-instance-lifecycle-and-the-instance-lease)).
Two consequences matter to REST. `Draining` now has long-lived SSE responses to close, so it sends the
control events that tell clients to reconnect elsewhere instead of severing them and waiting for the
timeout. And a Realtime substrate that is unreachable degrades `/ready` without taking REST down: the
subsystem is a separate driving mechanism, so ordinary request serving continues while the stream half
reports itself unhealthy ([§2.6](realtime-delivery.md#26-degraded-operation)). With no feature adapter
wired, none of this exists and the lifecycle is exactly the diagram above.

### 2.2 per-request flow (middleware order → handler → usecase → present)

```mermaid
stateDiagram-v2
    [*] --> Pre: e.Pre 1 — uri (path normalization)
    Pre --> TimeoutPre: e.Pre 2 — timeout (SERVER_REQUEST_TIMEOUT)
    TimeoutPre --> BodyLimitPre: e.Pre 3 — bodyLimit (SERVER_BODY_LIMIT_MB)
    BodyLimitPre --> RequestID: 1 requestID (X-Request-ID)
    RequestID --> Observability: 2 observability (OTel span, traceparent)
    Observability --> Recovery: 3 recovery (defer/recover → 500)
    Recovery --> CORS: 4 cors (origin / preflight)
    CORS --> Security: 5 security (HSTS, X-Frame-Options, …)
    Security --> OpenAPI: 6 openapi (request schema + auth validation)
    OpenAPI --> ForceJSON: 7 forcejson (Content-Type)
    ForceJSON --> HTTPREDMetrics: 8 httpredmetrics (RED metrics)
    HTTPREDMetrics --> Logging: 9 logging (start time + deferred response log)
    Logging --> Cookie: 10 cookie (Secure/SameSite enforcement)
    Cookie --> Handler: StrictHandler.<Op>
    Handler --> Usecase: parse typed request → s.uc.<Method>(ctx, …)
    Usecase --> Present: DTO → gen.<Op><Status>JSONResponse
    Present --> Respond: VisitResponse → c.JSON(status, body)
    Respond --> [*]

    note right of OpenAPI
      validation/auth failure short-circuits here → error handler (400/401).
    end note
    note right of Handler
      adopting handlers add StrictMiddleware (e.g. idempotency) in NewStrictHandler.
    end note
```

### 2.3 error path (`apperror` → HTTP)

```mermaid
stateDiagram-v2
    [*] --> HandlerErr: handler / usecase returns error (or middleware short-circuits)
    HandlerErr --> EchoCore: error propagates to Echo core
    EchoCore --> Recovered: recovery already logged a panic? → skip re-log
    EchoCore --> Normalize: else → HTTPErrorHandler
    Normalize --> MapAppError: apperror → status+code (lookupErrorMetaByAppError)
    Normalize --> MapEcho: status-carrying error (echo / openapi validation) → normalizeEchoHTTPError
    MapAppError --> Write: write HTTPErrorResponse (JSON) + headers
    MapEcho --> Write
    Write --> LogIf: log if status ∈ ObservabilityConfig target set
    Recovered --> Write
    LogIf --> [*]

    note right of Normalize
      the deferred logging middleware still records the final status + latency.
    end note
```

---

## 3. Implementation locations (where in the architecture it lives and acts)

### 3.1 Package placement and dependency direction

```mermaid
flowchart TD
    subgraph cmdL["cmd (main)"]
        CMD["cmd/serve.go<br/>newServeCommand / config + signals + RunServer"]
    end
    subgraph cliL["internal/cli/server"]
        CLI["server.go: RunServer / ResolveMetricsStop<br/>start → wait signal → graceful stop"]
    end
    subgraph diL["internal/di"]
        DIA["server.go: NewApplicationCore / NewApplicationServer (fx.App)"]
        DISRV["server/server.go: Module / MiddlewareModule / HookModule"]
        DIEXT["server/extension: ApplyExtends (priority sort, Pre/Use/SrvCfg)"]
        DIHOOK["server/hook: RegisterHTTPServerHooks (listen + shutdown)"]
        DICTRL["module/controller.go: ControllerModule (fx.Invoke BindHandler×N)"]
    end
    subgraph srvL["internal/controller/server"]
        APPSRV["app_server.go: NewAppServer (echo) / NewHTTPServer (http.Server + timeouts)"]
        ECHOH["echo.go: request/response extraction helpers"]
    end
    subgraph mwL["internal/controller/httpstack  = middleware + errors"]
        MW["requestid / observability / recovery / cors / security / oapi / logging / cookie / idempotency"]
        EH["errorhandler: HTTPErrorHandler, apperror→status"]
    end
    subgraph hdlL["internal/controller/handler/**"]
        HDL["<path>/*_handler.go: BindHandler + server{} + one method per operationId"]
        GEN["<path>/gen: server.gen.go (ServerInterface, RegisterHandlers, NewStrictHandler) + type.gen.go"]
    end
    subgraph ucL["internal/usecase/**"]
        UC["Usecase interfaces + Application Services (business logic)"]
    end
    subgraph crossL["cross-cutting"]
        APPERR["apperror: error taxonomy"]
        CFG["config: ServerConfig / SecurityConfig / ApplicationConfig"]
        OTEL["observability: TracerFactory"]
        LOG["logging: HTTP request/response fields"]
    end

    CMD --> CLI
    CMD --> DIA
    DIA --> DISRV
    DISRV --> DIEXT
    DISRV --> DIHOOK
    DISRV --> APPSRV
    DIA --> DICTRL
    DICTRL --> HDL
    HDL --> GEN
    HDL --> UC
    DIEXT --> MW
    APPSRV --> ECHOH
    EH -.maps.-> APPERR
    HDL -.returns.-> APPERR
    APPSRV --> CFG
    HDL --> OTEL
    MW --> LOG

    classDef done fill:#e6ffed,stroke:#2da44e;
    class CMD,CLI,DIA,DISRV,DIEXT,DIHOOK,DICTRL,APPSRV,ECHOH,MW,EH,HDL,GEN,UC,APPERR,CFG,OTEL,LOG done;
```

> Dependencies point inward (`controller→usecase`). The handler depends on its generated `gen` package and a usecase interface only; it never imports infrastructure. Middleware ordering is owned by the DI extension engine, not by handlers.

### 3.2 Per-request action sequence

```mermaid
sequenceDiagram
    participant C as Client
    participant E as Echo (router + middleware)
    participant H as StrictHandler.<Op> (handler)
    participant U as Usecase
    participant EH as ErrorHandler
    C->>E: HTTP request
    E->>E: middleware chain (uri→…→cookie), span started, request validated
    E->>H: typed <Op>RequestObject
    H->>H: tracer.Start, parse/convert request
    H->>U: s.uc.<Method>(ctx, params)
    alt success
        U-->>H: DTO
        H->>H: convert DTO → gen.<Op><Status>JSONResponse
        H-->>E: response object → VisitResponse → c.JSON(status, body)
        E-->>C: 2xx + JSON
    else error (apperror)
        U-->>H: error
        H-->>E: return error
        E->>EH: HTTPErrorHandler
        EH->>EH: apperror → status + code, write HTTPErrorResponse
        EH-->>C: 4xx/5xx + JSON
    end
    Note over E: deferred logging middleware records status + latency
```

---

## 4. What an integrator implements (contract-first endpoint flow)

This project provides the **server bootstrap, ordered middleware chain, error handler, DI wiring, and the `scaffold-*` skills**. To add an endpoint, follow the contract-first order (OpenAPI changes must precede handler/usecase code).

```mermaid
flowchart LR
    O["① OpenAPI source<br/>openapi/**/*.yaml"]:::need
    G["② make gen-api<br/>server.gen.go / type.gen.go"]:::need
    H["③ handler<br/>BindHandler + one method/operationId"]:::need
    U["④ usecase<br/>business logic (if new)"]:::need
    R["⑤ register in DI<br/>fx.Invoke(<pkg>.BindHandler)"]:::need
    O --> G --> H --> U --> R
    classDef need fill:#fff8c5,stroke:#bf8700;
```

| # | Required implementation | Location | Reference |
| --- | --- | --- | --- |
| ① | define the path / operation / schemas in OpenAPI source, re-bundle | `openapi/**/*.yaml` → `openapi/openapi.gen.yaml` | existing paths |
| ② | regenerate the server interface + types | `make gen-api` → `internal/controller/handler/<path>/gen/` | — |
| ③ | implement `BindHandler(echo, tracerFactory, usecase, …)` + one method per `operationId` (tracer span → parse → usecase → response) | `internal/controller/handler/<path>/*_handler.go` | `scaffold-controller`, existing handlers |
| ④ | implement the usecase method (if it does not exist), mapping domain → DTO | `internal/usecase/<feature>/` | `scaffold-usecase` |
| ⑤ | wire the handler: `fx.Invoke(<pkg>.BindHandler)` | `internal/di/module/controller.go` | existing invokes |

> Use the `scaffold-endpoint` orchestrator (or per-layer `scaffold-*` skills) to generate domain → infra → usecase → controller from the spec. A new cross-cutting middleware is added as a `*_di.go` with a priority constant under `internal/di/server/extension/` — the engine sorts it into the chain.

---

## 5. Glossary

| Term | Meaning |
| --- | --- |
| **driving adapter** | An entry point that drives the usecase layer. REST (HTTP) is the synchronous one; the [worker](worker.md) (queue) and [job](job.md) (CLI) are its siblings. |
| **Echo** | The HTTP framework. `server.NewAppServer()` builds the `*echo.Echo` and `server.NewHTTPServer(e, ServerConfig)` wraps it in an `http.Server` carrying the timeouts; routes are registered by generated code. |
| **ServerInterface / StrictServerInterface** | The OpenAPI-generated route interface / its strongly-typed (request-object, response-object) variant. Handlers implement the strict form. |
| **RegisterHandlers / NewStrictHandler** | Generated functions: register routes on Echo / wrap the strict handler with a `StrictMiddlewareFunc` slice (e.g. idempotency). |
| **BindHandler** | The handler package's constructor: builds the `server{}` (tracer + usecase) and calls `RegisterHandlers(e, NewStrictHandler(...))`. Wired via `fx.Invoke`. |
| **handler / `server{}`** | The thin controller type with one method per `operationId`: tracer span → parse request → call one usecase method → convert DTO → `gen` response. |
| **presenter** | The DTO → `gen.<Op><Status>JSONResponse` conversion, implemented inline in the handler method. |
| **middleware (Use) / Pre** | Per-request cross-cutting functions applied in priority order (`Use`), plus `Pre` (path normalization, request deadline, body-size limit) that runs before routing. |
| **priority** | The integer in each middleware's `*_di.go` that the extension engine sorts on (uri-pre 1, timeout-pre 2, bodyLimit-pre 3; requestID 1, observability 2, recovery 3, cors 4, security 5, openapi 6, forcejson 7, httpredmetrics 8, logging 9, cookie 10). |
| **extension engine** (`ApplyExtends`) | Collects `Pre`/`Use`/`SrvCfg` providers, sorts by priority, and applies them to Echo; also applies non-middleware configurators (IP extractor, error handler). |
| **error handler** | `HTTPErrorHandler` set on Echo; normalizes `apperror` and status-carrying errors (`echo.HTTPError`, OpenAPI validation failures) into a unified `HTTPErrorResponse` with the mapped status + code. |
| **apperror** | The framework-agnostic error taxonomy; the error handler maps it to HTTP status (e.g. `ErrConflict`→409, `ErrValidation`→422, `ErrInvalidArgument`→400). |
| **graceful shutdown** | On SIGINT/SIGTERM, `RunServer` stops accepting and calls `srv.Shutdown(stopCtx)` to drain in-flight within `shutdownTimeout` (timed from shutdown start). |
| **lifecycle / Registrar** | The fx hook seam: `RegisterHTTPServerHooks` registers OnStart (listen + serve) and OnStop (shutdown). |
| **ServerConfig** | Host / port / read-header / read / write / idle timeouts (`SERVER_*`). Injected into `NewHTTPServer`. |
| **idempotency middleware** | A `StrictMiddleware` slot adopting handlers add to make non-idempotent writes safe. See [idempotency.md](idempotency.md). |
