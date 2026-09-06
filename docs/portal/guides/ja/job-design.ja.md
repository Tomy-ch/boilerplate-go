# Job サブシステム設計リファレンス

本書は job scaffold の **役割論・状態遷移・実装箇所・integrator が書く箇所・用語** を、実装を精査して 1 枚にまとめた参照資料です。概要は README、非同期の兄弟である worker は [worker.ja.md](worker.ja.md) を参照。

---

## 1. 役割論（なにが・なんのために）

job は **HTTP handler・worker と同格の「command-in driving adapter」**。新しいアーキテクチャ層ではなく、**CLI（Cobra）経由で Usecase 層に入るもう 1 つの入口**。HTTP が「同期リクエストを受ける口」、worker が「キューメッセージを受ける口」なら、job は「1 回の CLI 実行を受けて終了する口」。

責務の分担（誰が何を持つか）:

| 構成要素 | 層 | 責務 | 持たないもの |
| --- | --- | --- | --- |
| **runner**（`Runner`） | controller | 名前 → `Job` のレジストリ / 名前ディスパッチ / 同名拒否 / `Names()` 列挙 | 業務ロジック・引数解釈 |
| **state**（`State`） | controller | 選択された実行（name / args / `done` チャネル）を保持し lifecycle hook へ受け渡す | ジョブ実行・引数解釈 |
| **seam**（`Job`/`Runner`/`State`） | usecase/boundary | CLI ライフサイクルと業務ジョブの契約 | 実装 |
| **Job**（業務処理） | integrator が実装（usecase を呼ぶ） | 1 回の実行の業務処理（**冪等推奨**） | ディスパッチ・lifecycle・停止 |
| **DI / cli / cmd** | di / cli / cmd(main) | 実行ごとの `fx.App` 合成 / サブコマンド / timeout・graceful stop / detached goroutine / 停止要求 | 業務ロジック |
| **OperatingSystemConfig** | config | 構造化ジョブログ用の TZ | ジョブ挙動 |

設計原則（不変）: **runner と state は seam（`job.Job` / `job.Runner` / `job.State`）のみに依存し実装を import しない。** 実行ごとに **新しい `fx.App` を構築**（常駐プロセスなし）し、ちょうど 1 件のジョブを実行して停止を要求する。worker と違い **broker・サーキットブレーカ・drain・health listener は無い** ——ジョブの失敗は CLI へ非ゼロ終了として返すだけ。

---

## 2. 状態遷移図

### 2.1 実行ライフサイクル（`cli/job.RunJobWith` / `runJob` ＋ DI フック）

```mermaid
stateDiagram-v2
    [*] --> Idle: RunJobWith(provide) → (StartFunc, StopFunc)
    Idle --> Started: start(ctx, name, args) — state.Set ＋ app.Start(ctx)
    Started --> Running: RegisterJobHooks OnStart が runJobAndShutdown goroutine を起動
    Running --> Completed: runner.Run(name, args) → Job.Execute 復帰、結果を done へ送出
    Running --> TimedOut: timeout>0 かつ waitCtx が先に完了（DeadlineExceeded / 親 Canceled）
    Completed --> Stopping: gracefulStop → app.Stop（grace = APP_SHUTDOWN_TIMEOUT）
    TimedOut --> Stopping: gracefulStop → app.Stop（grace = APP_SHUTDOWN_TIMEOUT）
    Stopping --> Stopped: sd.Shutdown 要求 ＋ app.Stop 完了
    Stopped --> [*]: runJob が err を返す（ジョブ結果 / waitCtx.Err()）

    note right of Running
      ジョブは context.WithoutCancel(startCtx) の detached goroutine で走るため、
      OnStart 完了でキャンセルされない。
      done==nil（ジョブ未選択）→ "No job to run" ログ → shutdown。
    end note
    note right of TimedOut
      停止は期限切れの waitCtx ではなく専用 context を作り直し猶予を与える。
    end note
```

### 2.2 状態の受け渡し（`controller/job/state.go`）

```mermaid
stateDiagram-v2
    [*] --> Empty: NewState()
    Empty --> Set: Set(name, args, done)  // mutex 保護、DI start func が呼ぶ
    Set --> Snapshotted: Snapshot() → (name, args, done)  // hook goroutine が読む
    Snapshotted --> Set: 再度 Set（次の実行、スレッドセーフ）

    note right of Set
      done はバッファ付き（cap≥1）。Snapshot 読み手（hook）が送信＋close を所有。
      app.Start 失敗時は別チャネルで返し done の二重送信／close を避ける。
    end note
```

### 2.3 ディスパッチ（`run.Run`、レジストリ参照）

```mermaid
stateDiagram-v2
    [*] --> Lookup: runner.Run(name, args)
    Lookup --> Unknown: 未登録 name → ErrUnknownJob（available 一覧付き）
    Lookup --> Execute: 一致 → Job.Execute(ctx, args)
    Execute --> Success: nil
    Execute --> Failed: error（そのまま CLI へ返す）
    Unknown --> [*]
    Success --> [*]
    Failed --> [*]

    note right of Lookup
      NewRunner は構築時に同名を拒否（ErrDuplicateJob）。
      各 Job は自分の引数を解釈（例: --batch-size=N / --active-only）。
    end note
```

---

## 3. 実装箇所（このアーキテクチャ上のどこに・どう作用するか）

### 3.1 パッケージ配置と依存方向

```mermaid
flowchart TD
    subgraph cmdL["cmd (main)"]
        CMD["cmd/job.go<br/>newJobCommand / args[0]=name ＋ args[1:] / --timeout"]
    end
    subgraph cliL["internal/cli/job"]
        CLI["job.go: RunJobWith / runJob<br/>timeout 分岐 ＋ gracefulStop(grace)"]
    end
    subgraph diL["internal/di"]
        DIJ["job.go: RunJob / NewJobCore<br/>実行ごとの fx.App, state.Set, start/stop 関数"]
        DIR["job/runner.go: ProvideRunner (group:jobs → Runner)"]
        DIH["job/hook: RegisterJobHooks (OnStart → detached goroutine)"]
        DIM["module/job.go: JobModule (provideJobs, group:jobs)"]
    end
    subgraph ctrlL["internal/controller/job  ＝ runner ＋ state"]
        RUN["runner.go: Runner, dispatch, Names, 同名ガード"]
        STATE["state.go: State, mutex 保護の name/args/done"]
        %% sample-api:begin
        UC["usercount/: サンプル Job (sample-api)"]
        %% sample-api:end
        GC["idempotencygc/: サンプル Job (冪等性 GC)"]
    end
    subgraph seamL["internal/usecase/boundary/job  ＝ seam"]
        PORT["job.go: Job / Runner / State (port)"]
        MOCK["mock/: 生成モック"]
    end
    subgraph crossL["横断"]
        SD["di/shutdowner: Shutdowner (app 停止要求)"]
        CFG["config: OperatingSystemConfig (TZ)"]
        LOG["logging: JobNameKey/JobArgsKey/JobResultKey/JobErrorKey"]
        OTEL["observability: TracerFactory / LayerTracer"]
    end

    CMD --> CLI
    CMD --> DIJ
    DIJ --> DIM
    DIM --> DIR
    DIM --> DIH
    DIH --> CLI
    DIR --> RUN
    RUN --> PORT
    STATE --> PORT
    %% sample-api:begin
    UC --> PORT
    %% sample-api:end
    GC --> PORT
    DIH --> SD
    DIH --> STATE
    RUN --> LOG
    %% sample-api:begin
    UC --> OTEL
    %% sample-api:end
    GC --> OTEL
    DIH --> CFG

    classDef done fill:#e6ffed,stroke:#2da44e;
    %% sample-api:replace-begin
    class CMD,CLI,DIJ,DIR,DIH,DIM,RUN,STATE,UC,GC,PORT,MOCK,SD,CFG,LOG,OTEL done;
    %% sample-api:replace-with
    %% = class CMD,CLI,DIJ,DIR,DIH,DIM,RUN,STATE,GC,PORT,MOCK,SD,CFG,LOG,OTEL done;
    %% sample-api:replace-end
```

> 緑＝本プロジェクトの実装済み。依存方向は常に内向き（`controller→usecase/boundary`）。runner/state は業務ロジックも infra import も持たず、ジョブはデータへ usecase 層経由でのみ到達する。

### 3.2 実行 1 回の作用シーケンス

```mermaid
sequenceDiagram
    participant Cobra as cmd/job.go (Cobra)
    participant CLI as cli/job (runJob)
    participant DI as di.RunJob (start/stop)
    participant Hook as RegisterJobHooks (goroutine)
    participant Run as Runner (controller)
    participant J as Job (業務)
    Cobra->>CLI: RunJobWith(ctx, name, args, timeout, provide)
    CLI->>DI: start(ctx, name, args)
    DI->>DI: state.Set(name, args, done) ＋ app.Start(ctx)
    DI-->>Hook: OnStart 発火 → go runJobAndShutdown
    Hook->>Hook: name,args,done := state.Snapshot()
    Hook->>Run: runner.Run(WithoutCancel(ctx), name, args)
    Run->>J: Execute(ctx, args)  // span 開始, usecase 呼び出し
    J-->>Run: nil / error
    Run-->>Hook: result
    Hook->>Hook: done <- result, close(done), sd.Shutdown()
    CLI->>CLI: err := <-done（timeout 時は waitCtx.Done()）
    CLI->>DI: gracefulStop → stop(ctx) ＝ app.Stop（≤ grace）
    CLI-->>Cobra: return err（終了コード）
```

---

## 4. integrator が実装する箇所（本プロジェクトが用意しない部分）

<!-- sample-api:replace-begin -->
本プロジェクトは **runner・state・lifecycle hook・DI 配線・2 つの参考ジョブ**（`usercount`・`idempotencygc`）を提供する。ジョブを追加するには次を用意する（ジョブは明示登録・自動検出なし）。
<!-- sample-api:replace-with -->
<!-- = 本プロジェクトは **runner・state・lifecycle hook・DI 配線・参考ジョブ**（`idempotencygc`）を提供する。ジョブを追加するには次を用意する（ジョブは明示登録・自動検出なし）。 -->
<!-- sample-api:replace-end -->

```mermaid
flowchart LR
    J["① 業務 Job<br/>Name() ＋ Execute(ctx,args) 冪等"]:::need
    C["② コンストラクタ<br/>New(deps...) job.Job"]:::need
    R["③ JobModule に登録<br/>provideJobs(<pkg>.New)"]:::need
    D["④ 必要な依存の DI provide<br/>(usecase は配線済み)"]:::need
    J --> C --> R --> D
    classDef need fill:#fff8c5,stroke:#bf8700;
```

| # | 必要な実装 | 置き場（推奨） | 参考 |
| --- | --- | --- | --- |
<!-- sample-api:replace-begin -->
| ① | 業務 `Job`：`Name()`（kebab-case）＋ `Execute(ctx, args)`（引数解釈・usecase 呼び出し・ログ） | `internal/controller/job/<name>/` | `usercount` / `idempotencygc` |
| ② | `New(...) job.Job` DI コンストラクタ（logging / tracer factory / usecase を受け取る） | ①と同じファイル | `usercount.New` |
<!-- sample-api:replace-with -->
<!-- = | ① | 業務 `Job`：`Name()`（kebab-case）＋ `Execute(ctx, args)`（引数解釈・usecase 呼び出し・ログ） | `internal/controller/job/<name>/` | `idempotencygc` | -->
<!-- = | ② | `New(...) job.Job` DI コンストラクタ（logging / tracer factory / usecase を受け取る） | ①と同じファイル | `idempotencygc.New` | -->
<!-- sample-api:replace-end -->
| ③ | `JobModule()` の `provideJobs(...)` に コンストラクタ追加 | `internal/di/module/job.go` | `provideWorkers` と同形 |
| ④ | ジョブが必要とする追加依存の `fx.Provide`（usecase は提供済み） | `internal/di/module/` | 既存の usecase provider |

> 実行は `<binary> job <name> [args...] [--timeout 30s]`。`cmd/job.go` が `args[0]` をジョブ名、`args[1:]` をジョブ引数へ振り分け済みで、ジョブごとの CLI 配線は不要。

---

## 5. 用語集

| 用語 | 意味 |
| --- | --- |
| **seam** | CLI ライフサイクルと業務ジョブの境界となる port 群。`internal/usecase/boundary/job`。 |
| **port** | seam を構成する interface：`Job` / `Runner` / `State`。 |
| **Job** | CLI から 1 回実行される作業単位。`Name()` ＋ `Execute(ctx, args)`。冪等であるべき（運用者が再実行しうる）。 |
| **runner** | ジョブ名を `Job` に対応づけ `Execute` をディスパッチするレジストリ。構築時に同名を拒否。 |
| **State** | 選択された実行（`name` / `args` / `done`）のスレッドセーフな保持器。DI start func が `Set`、hook goroutine が `Snapshot`。 |
| **done チャネル** | hook がジョブ結果を送って close するバッファ付き（`chan error`, cap≥1）チャネル。読み手（hook）が所有。 |
| **StartFunc / StopFunc** | `di.RunJob` が返す開始/停止のペア。`start` は `state.Set` ＋ `app.Start` 後に `done` を返す、`stop` は `app.Stop`。 |
| **RunJobWith** | provider を差し替え可能にしてから `runJob`（timeout 分岐 ＋ graceful stop）へ委譲する `cli/job` のファサード。 |
| **runJobAndShutdown** | `RegisterJobHooks` OnStart が起動する detached goroutine。state を Snapshot し、ジョブを実行、結果送出、停止を要求。 |
| **context.WithoutCancel** | fx OnStart フック復帰でジョブの context がキャンセルされないようにする（ジョブは OnStart を超えて走る）。 |
| **timeout（`--timeout`）** | 任意の CLI 期限。`>0` ならジョブ完了か期限の早い方を待ち、`≤0` なら無期限に待つ。 |
| **gracefulStop / stopTimeout** | 完了・timeout 後、`app.Stop` に（期限切れでない）新規 context を与え、停止猶予（`APP_SHUTDOWN_TIMEOUT`、既定 65s）の範囲で後始末させる。 |
| **Shutdowner** | ジョブ完了後に hook が呼ぶ fx の停止要求器。これでアプリが終了する。 |
| **provideJobs / `group:"jobs"`** | 全ジョブコンストラクタを `NewRunner` が食うスライスへ束ねる Fx 集約。 |
| **ErrUnknownJob / ErrDuplicateJob** | ディスパッチ時の未知名エラー（available 一覧付き）／構築時の同名エラー。 |
| **idempotencygc** | 失効した冪等性キーを掃除する同梱ジョブ（`--batch-size=N`）。[idempotency.ja.md](idempotency.ja.md) 参照。 |
