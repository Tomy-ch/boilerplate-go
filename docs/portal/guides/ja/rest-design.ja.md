# REST サブシステム設計リファレンス

本書は REST（HTTP）scaffold の **役割論・状態遷移・実装箇所・integrator が書く箇所・用語** を、実装を精査して 1 枚にまとめた参照資料です。handler 実装の詳細は [handler README（日本語）](../../internal/controller/handler/README.ja.md)、非同期 / CLI の兄弟は [worker.ja.md](worker.ja.md)・[job.ja.md](job.ja.md) を参照。

---

## 1. 役割論（なにが・なんのために）

REST は **「request-in driving adapter」、Usecase 層への同期 HTTP 入口**であり、worker（message-in）・job（command-in）が手本とした元祖の同格。Echo サーバが transport、handler は HTTP I/O を usecase 呼び出しへ橋渡しする薄いテンプレート。**業務ロジックはすべて usecase 層に留まる**——handler は bind し、usecase を 1 メソッド呼び、応答を整形するだけ。

責務の分担（誰が何を持つか）:

| 構成要素 | 層 | 責務 | 持たないもの |
| --- | --- | --- | --- |
| **HTTP サーバ**（`server.NewAppServer` / `server.NewHTTPServer`） | controller | Echo インスタンスとそれを配信する `http.Server`：read·write·idle タイムアウト / 生成 `RegisterHandlers` によるルート表 | 業務ロジック・middleware 方針 |
| **middleware チェーン**（`httpstack/*`） | controller | 固定順の横断関心事：uri(pre) → timeout(pre) → bodyLimit(pre) → requestID → observability → recovery → cors → security → openapi → forcejson → httpredmetrics → logging → cookie | 業務ロジック |
| **handler**（`controller/handler/**`） | controller | 型付きリクエスト解釈（`StrictHandler`）→ usecase を**1 メソッド**呼ぶ → DTO → `gen` 応答へ変換 | 業務ロジック・永続化・tx |
| **error handler**（`httpstack/errorhandler`） | controller | `apperror` / Echo / OpenAPI エラー → HTTP ステータス＋コード、統一エラーボディ | 業務方針 |
| **usecase** | usecase | **すべての**業務ロジック・トランザクション境界・ドメインオーケストレーション・エラー方針 | HTTP・フレームワーク・表現 |
| **DI server ＋ hook**（`di/server`） | di | Echo ＋ 順序付き middleware（priority）＋ lifecycle（listen / graceful shutdown）の合成 | 業務ロジック |
| **ServerConfig** | config | host / port / read-header·read·write·idle タイムアウト、リクエスト期限（`SERVER_REQUEST_TIMEOUT`）、ボディサイズ上限（`SERVER_BODY_LIMIT_MB`） | 業務ロジック |

設計原則（不変）:

- **Contract-first。** ルート・リクエスト・レスポンス型は OpenAPI から生成（`make gen-api`）し、handler は生成された `StrictServerInterface` を実装する。handler・usecase は契約に先行してはならない。
- **薄い handler。** handler はテンプレート（bind → usecase → present）で、業務ロジックを持たず **infrastructure を import しない**（depguard `maintain_a_sound_controller`）。
- **priority による順序付き middleware。** 各 middleware は `*_di.go` に整数 priority を宣言し、extension エンジンが `Pre`/`Use` を決定的にソート・適用する。チェーン順は呼び出し位置ではなくデータ。

---

## 2. 状態遷移図

### 2.1 サーバライフサイクル（`cli/server.RunServer` ＋ fx ＋ signal）

```mermaid
stateDiagram-v2
    [*] --> Building: NewApplicationCore() → fx.New (config→logging→o11y→db→infra→usecase→controller→server)
    Building --> Wired: BindHandler×N (RegisterHandlers) ＋ ApplyExtends (middleware ソート＆適用) ＋ RegisterHTTPServerHooks
    Wired --> Listening: fx OnStart → net listen port → go srv.Serve(ln)
    Listening --> Serving: request → middleware チェーン → handler → 応答（常駐）
    Serving --> Listening: 応答送出
    Listening --> Draining: SIGINT/SIGTERM → RunServer の ctx.Done()
    Draining --> Stopped: srv.Shutdown(stopCtx) が in-flight を shutdownTimeout 内で drain
    Stopped --> [*]: fx OnStop 完了 → プロセス終了

    note right of Draining
      stopCtx は停止開始時点から計る新規 context（稼働時間に消費されない）。
    end note
    note right of Building
      メトリクスサーバは非本番モードでのみ起動（ResolveMetricsStop）。
    end note
```

Realtime Delivery はこの lifecycle を置き換えるのではなく拡張する。feature adapter が 1 つ以上
結線されていると、その DI module が serve lifecycle の participant——startup probe、lease と queue
の provisioner、consumer と heartbeat の runner、そして drainer——を提供し、
`internal/di/server/hook` が上記のフェーズの中でそれらを走らせる
（[realtime-delivery.ja.md §2.5](realtime-delivery.ja.md#25-serve-instance-の-lifecycle-と-instance-lease)）。
REST にとって重要な帰結が 2 つある。`Draining` は閉じるべき長寿命の SSE 応答を抱えるようになったため、
切断してタイムアウトを待つのではなく、クライアントへ他所への再接続を促す control event を送る。そして
Realtime の基盤に到達できなくても REST を道連れにせず `/ready` だけが degrade する——このサブシステムは
別個の driving mechanism であるため、stream 側が degraded を報告している間も通常のリクエスト処理は続く
（[§2.6](realtime-delivery.ja.md#26-degraded-時の動作)）。feature adapter が 1 つも結線されていなければ
これらは何一つ存在せず、lifecycle は上の図そのままである。

### 2.2 リクエスト 1 件の流れ（middleware 順 → handler → usecase → present）

```mermaid
stateDiagram-v2
    [*] --> Pre: e.Pre 1 — uri（パス正規化）
    Pre --> TimeoutPre: e.Pre 2 — timeout（SERVER_REQUEST_TIMEOUT）
    TimeoutPre --> BodyLimitPre: e.Pre 3 — bodyLimit（SERVER_BODY_LIMIT_MB）
    BodyLimitPre --> RequestID: 1 requestID（X-Request-ID）
    RequestID --> Observability: 2 observability（OTel span, traceparent）
    Observability --> Recovery: 3 recovery（defer/recover → 500）
    Recovery --> CORS: 4 cors（origin / preflight）
    CORS --> Security: 5 security（HSTS, X-Frame-Options, …）
    Security --> OpenAPI: 6 openapi（リクエストスキーマ＋認証検証）
    OpenAPI --> ForceJSON: 7 forcejson（Content-Type）
    ForceJSON --> HTTPREDMetrics: 8 httpredmetrics（RED メトリクス）
    HTTPREDMetrics --> Logging: 9 logging（開始時刻＋遅延応答ログ）
    Logging --> Cookie: 10 cookie（Secure/SameSite 強制）
    Cookie --> Handler: StrictHandler.<Op>
    Handler --> Usecase: 型付きリクエスト解釈 → s.uc.<Method>(ctx, …)
    Usecase --> Present: DTO → gen.<Op><Status>JSONResponse
    Present --> Respond: VisitResponse → c.JSON(status, body)
    Respond --> [*]

    note right of OpenAPI
      検証/認証失敗はここで短絡 → error handler（400/401）。
    end note
    note right of Handler
      採用 handler は NewStrictHandler に StrictMiddleware（例: 冪等性）を差す。
    end note
```

### 2.3 エラー経路（`apperror` → HTTP）

```mermaid
stateDiagram-v2
    [*] --> HandlerErr: handler / usecase が error 返却（または middleware が短絡）
    HandlerErr --> EchoCore: error が Echo コアへ伝播
    EchoCore --> Recovered: recovery が既に panic を記録済み? → 再ログ省略
    EchoCore --> Normalize: それ以外 → HTTPErrorHandler
    Normalize --> MapAppError: apperror → status+code (lookupErrorMetaByAppError)
    Normalize --> MapEcho: ステータスを持つエラー（echo / openapi 検証）→ normalizeEchoHTTPError
    MapAppError --> Write: HTTPErrorResponse（JSON）＋ヘッダを書く
    MapEcho --> Write
    Write --> LogIf: status ∈ ObservabilityConfig 対象集合 ならログ
    Recovered --> Write
    LogIf --> [*]

    note right of Normalize
      遅延 logging middleware が最終 status ＋ latency を記録する。
    end note
```

---

## 3. 実装箇所（このアーキテクチャ上のどこに・どう作用するか）

### 3.1 パッケージ配置と依存方向

```mermaid
flowchart TD
    subgraph cmdL["cmd (main)"]
        CMD["cmd/serve.go<br/>newServeCommand / config ＋ signals ＋ RunServer"]
    end
    subgraph cliL["internal/cli/server"]
        CLI["server.go: RunServer / ResolveMetricsStop<br/>start → signal 待ち → graceful stop"]
    end
    subgraph diL["internal/di"]
        DIA["server.go: NewApplicationCore / NewApplicationServer (fx.App)"]
        DISRV["server/server.go: Module / MiddlewareModule / HookModule"]
        DIEXT["server/extension: ApplyExtends (priority ソート, Pre/Use/SrvCfg)"]
        DIHOOK["server/hook: RegisterHTTPServerHooks (listen ＋ shutdown)"]
        DICTRL["module/controller.go: ControllerModule (fx.Invoke BindHandler×N)"]
    end
    subgraph srvL["internal/controller/server"]
        APPSRV["app_server.go: NewAppServer (echo) / NewHTTPServer (http.Server ＋ timeouts)"]
        ECHOH["echo.go: request/response 抽出ヘルパ"]
    end
    subgraph mwL["internal/controller/httpstack  ＝ middleware ＋ errors"]
        MW["requestid / observability / recovery / cors / security / oapi / logging / cookie / idempotency"]
        EH["errorhandler: HTTPErrorHandler, apperror→status"]
    end
    subgraph hdlL["internal/controller/handler/**"]
        HDL["<path>/*_handler.go: BindHandler ＋ server{} ＋ operationId ごと 1 メソッド"]
        GEN["<path>/gen: server.gen.go (ServerInterface, RegisterHandlers, NewStrictHandler) ＋ type.gen.go"]
    end
    subgraph ucL["internal/usecase/**"]
        UC["Usecase interface ＋ Application Service（業務ロジック）"]
    end
    subgraph crossL["横断"]
        APPERR["apperror: エラー分類"]
        CFG["config: ServerConfig / SecurityConfig / ApplicationConfig"]
        OTEL["observability: TracerFactory"]
        LOG["logging: HTTP request/response フィールド"]
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

> 依存方向は内向き（`controller→usecase`）。handler は自身の生成 `gen` パッケージと usecase interface のみに依存し、infrastructure を import しない。middleware 順序は DI extension エンジンが所有し、handler は持たない。

### 3.2 リクエスト 1 件の作用シーケンス

```mermaid
sequenceDiagram
    participant C as Client
    participant E as Echo (router ＋ middleware)
    participant H as StrictHandler.<Op> (handler)
    participant U as Usecase
    participant EH as ErrorHandler
    C->>E: HTTP リクエスト
    E->>E: middleware チェーン（uri→…→cookie）, span 開始, リクエスト検証
    E->>H: 型付き <Op>RequestObject
    H->>H: tracer.Start, リクエスト解釈/変換
    H->>U: s.uc.<Method>(ctx, params)
    alt 成功
        U-->>H: DTO
        H->>H: DTO → gen.<Op><Status>JSONResponse へ変換
        H-->>E: response object → VisitResponse → c.JSON(status, body)
        E-->>C: 2xx ＋ JSON
    else エラー（apperror）
        U-->>H: error
        H-->>E: return error
        E->>EH: HTTPErrorHandler
        EH->>EH: apperror → status ＋ code, HTTPErrorResponse を書く
        EH-->>C: 4xx/5xx ＋ JSON
    end
    Note over E: 遅延 logging middleware が status ＋ latency を記録
```

---

## 4. integrator が実装する箇所（contract-first のエンドポイント手順）

本プロジェクトは **サーバ起動・順序付き middleware チェーン・error handler・DI 配線・`scaffold-*` スキル**を提供する。エンドポイント追加は contract-first 順（OpenAPI 変更が handler/usecase コードに先行する）に従う。

```mermaid
flowchart LR
    O["① OpenAPI ソース<br/>openapi/**/*.yaml"]:::need
    G["② make gen-api<br/>server.gen.go / type.gen.go"]:::need
    H["③ handler<br/>BindHandler ＋ operationId ごと 1 メソッド"]:::need
    U["④ usecase<br/>業務ロジック（新規なら）"]:::need
    R["⑤ DI に登録<br/>fx.Invoke(<pkg>.BindHandler)"]:::need
    O --> G --> H --> U --> R
    classDef need fill:#fff8c5,stroke:#bf8700;
```

| # | 必要な実装 | 置き場 | 参考 |
| --- | --- | --- | --- |
| ① | OpenAPI ソースにパス / operation / スキーマを定義し再 bundle | `openapi/**/*.yaml` → `openapi/openapi.gen.yaml` | 既存パス |
| ② | サーバ interface ＋ 型を再生成 | `make gen-api` → `internal/controller/handler/<path>/gen/` | — |
| ③ | `BindHandler(echo, tracerFactory, usecase, …)` ＋ `operationId` ごと 1 メソッド（tracer span → 解釈 → usecase → 応答）を実装 | `internal/controller/handler/<path>/*_handler.go` | `scaffold-controller`, 既存のハンドラ |
| ④ | usecase メソッドを実装（無ければ）、domain → DTO へ写像 | `internal/usecase/<feature>/` | `scaffold-usecase` |
| ⑤ | handler を配線：`fx.Invoke(<pkg>.BindHandler)` | `internal/di/module/controller.go` | 既存の invoke |

> `scaffold-endpoint` オーケストレータ（または各層の `scaffold-*` スキル）で spec から domain → infra → usecase → controller を生成できる。新しい横断 middleware は `internal/di/server/extension/` 配下に priority 定数付きの `*_di.go` として追加すると、エンジンがチェーンへソートして組み込む。

---

## 5. 用語集

| 用語 | 意味 |
| --- | --- |
| **driving adapter** | usecase 層を駆動する入口。REST（HTTP）が同期、[worker](worker.ja.md)（キュー）・[job](job.ja.md)（CLI）が兄弟。 |
| **Echo** | HTTP フレームワーク。`server.NewAppServer()` が `*echo.Echo` を構築し、`server.NewHTTPServer(e, ServerConfig)` が timeouts を持つ `http.Server` で包む。ルートは生成コードが登録。 |
| **ServerInterface / StrictServerInterface** | OpenAPI 生成のルート interface / その強型（request-object, response-object）版。handler は strict 版を実装。 |
| **RegisterHandlers / NewStrictHandler** | 生成関数：Echo にルート登録 / strict handler を `StrictMiddlewareFunc` スライス（例: 冪等性）で包む。 |
| **BindHandler** | handler パッケージのコンストラクタ：`server{}`（tracer ＋ usecase）を作り `RegisterHandlers(e, NewStrictHandler(...))` を呼ぶ。`fx.Invoke` で配線。 |
| **handler / `server{}`** | `operationId` ごと 1 メソッドの薄い controller 型：tracer span → リクエスト解釈 → usecase 1 メソッド → DTO → `gen` 応答へ変換。 |
| **presenter** | DTO → `gen.<Op><Status>JSONResponse` 変換。handler メソッド内にインラインで実装。 |
| **middleware (Use) / Pre** | priority 順に適用される per-request 横断関数（`Use`）と、ルーティング前に走る `Pre`（パス正規化・リクエスト期限・ボディサイズ上限）。 |
| **priority** | 各 middleware の `*_di.go` の整数で extension エンジンがソート（uri-pre 1, timeout-pre 2, bodyLimit-pre 3; requestID 1, observability 2, recovery 3, cors 4, security 5, openapi 6, forcejson 7, httpredmetrics 8, logging 9, cookie 10）。 |
| **extension エンジン**（`ApplyExtends`） | `Pre`/`Use`/`SrvCfg` provider を集約し priority でソートして Echo に適用。非 middleware の構成器（IP extractor, error handler）も適用。 |
| **error handler** | Echo に設定する `HTTPErrorHandler`。`apperror` と、ステータスを持つエラー（`echo.HTTPError` / OpenAPI 検証エラー）を統一 `HTTPErrorResponse`（status ＋ code 写像付き）へ正規化。 |
| **apperror** | フレームワーク非依存のエラー分類。error handler が HTTP ステータスへ写像（例 `ErrConflict`→409, `ErrValidation`→422, `ErrInvalidArgument`→400）。 |
| **graceful shutdown** | SIGINT/SIGTERM で `RunServer` が受付停止し `srv.Shutdown(stopCtx)` で in-flight を `shutdownTimeout` 内に drain（停止開始時点から計る）。 |
| **lifecycle / Registrar** | fx フックの seam：`RegisterHTTPServerHooks` が OnStart（listen ＋ serve）と OnStop（shutdown）を登録。 |
| **ServerConfig** | host / port / read-header / read / write / idle タイムアウト（`SERVER_*`）。`NewHTTPServer` に注入。 |
| **冪等性 middleware** | 採用 handler が非冪等書き込みを安全にするために差す `StrictMiddleware` スロット。[idempotency.ja.md](idempotency.ja.md) 参照。 |
