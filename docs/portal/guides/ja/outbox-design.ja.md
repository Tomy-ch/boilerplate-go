# Outbox サブシステム設計リファレンス

本書は transactional outbox サブシステムの **役割論・状態遷移・実装箇所・integrator が書く箇所・用語** を、実装を精査して 1 枚にまとめた参照資料です。各パッケージの概要は README、採用判断は outbox ADR（[ADR-0054](../adr/0054-transactional-outbox.ja.md) 以降）、バランス型の relay を出荷しハードニングは運用で得た事実に委ねる決定（多層ハードニングの設計図込み）は [ADR-0111](../adr/0111-outbox-relay-hardening-delegated.ja.md) を参照。

---

## 1. 役割論（なにが・なんのために）

transactional outbox は **dual-write 異常の排除** のために存在します。ドメイン状態を変更し、かつ外部へ通知する必要があるとき、「DB への書き込み」と「外部エンドポイントへの publish」は別々の失敗点です。commit 後に publish すれば lost し、commit 前に publish すれば phantom になる。outbox は「publish する意図」を **ドメイン変更と同一 DB トランザクション** に畳み込み（outbox 行を 1 行 INSERT）、その後に別の **relay** が **at-least-once** で非同期送出します。

サブシステムは、決して同じ呼吸で走らない 2 つの半身に分かれます。

- **emit** — 同期。呼び出し側の業務 tx 内。イベントを 1 行として記録する。
- **relay / gc / replay** — 非同期。専用の常駐プロセス内。行を捌き・刈り・回復する。

責務分担（誰がなにを持つか）:

| コンポーネント | 層 | 責務 | 持たないもの |
| --- | --- | --- | --- |
| **EmitUsecase**（`emit.go`） | usecase | 呼び出し側 tx 内で 1 行 INSERT、`traceparent` を headers へ capture | tx 制御（呼び出し側が持つ）・送出 |
| **RelayUsecase**（`relay.go`） | usecase | **1 つの配送チャネル**について、バッチ単位で claim → publish → mark（`published` / バックオフ付き `pending` / `dead`）を 1 tx で・lag と停止ストリーム数の記録 | ループ周期・broker 詳細 |
| **GCUsecase** / **ReplayUsecase**（`gc.go` / `replay.go`） | usecase | 古い `published` 行の剪定 / `dead` 行を `pending` へ戻す | スケジューリング・CLI パース |
| **Store** / **Publisher** / **tx.Manager** | usecase/boundary | seam: 永続化ポート / 送出ポート / トランザクションポート | 実装 |
| **Engine**（`controller/outbox/relay.go`） | controller | poll ループの統括: 周期・sleep/backoff・ctx 完了での drain・span | claim/publish/mark の業務（usecase へ委譲） |
| **outbox-gc job**（`controller/job/outboxgc`） | controller | 外部スケジューラ向けの one-shot GC 入口 | ループ自体（daemon ではなく cron） |
| **httpPublisher**（`infrastructure/publisher`） | infrastructure | `Publisher` の HTTP 実装: POST + `Idempotency-Key` + 非標準 client profile | retry（poll ループが retry 本体） |
| **realtime publisher**（`infrastructure/realtime`） | infrastructure | `realtime` チャネル向けの `Publisher` 実装: EventLog への append（`eventId` で冪等）→ SNS への wakeup publish。sequence の衝突なら恒久、基盤の障害なら一時と分類する（[Realtime Delivery](realtime-delivery.ja.md)） | retry・Streamer |
| **outbox store**（`infrastructure/rdb/system_cqrs/outbox`） | infrastructure | sqlc gen + `pgerror.NormalizeError` 上の `Store` 実装 | 業務判断 |
| **DI / cli / cmd** | di / cli / cmd(main) | relay プロセスの合成 / サブコマンド / ライフサイクル | 業務ロジック |
| **OutboxConfig** | config | relay チューニング（`OUTBOX_*`） | broker/endpoint の内部 |

設計上の不変条件:

- **dual-write 回避**: `Emit` はドメイン変更と同じ `tx.Manager.Do` の **中で** 呼ばれるため、業務 tx が巻き戻れば outbox 行も破棄される — lost も phantom も起きない。
- **at-least-once / retry-by-poll**: publish 失敗は巻き戻さない。行は `pending` のまま `next_attempt_at` が先へ進み、*その時刻以降の poll が retry 本体*。よって transport 層 retry は二重化を避けるため無効（`MaxAttempts=1`、D10）。
- **dead は回数でなく種別で決まる**: 行が `dead` になるのは publisher が失敗を恒久的と分類したときだけ（[ADR-0058](../adr/0058-outbox-dead-on-permanent-error.md)）。一時失敗は上限 60 秒の指数バックオフで再試行し続け、dead にはならない。下流障害の代償は遅延であって、手作業で replay する滞留ではない。
- **チャネル隔離**: 各行は `delivery_channel` を持ち、relay プロセスはちょうど 1 チャネルを捌く。claim も失敗時の進行もチャネルをまたがないため、あるチャネルの停止が別のチャネルを止めない。
- **ストリーム内の順序**: 行は `ordering_key`（ストリーム）と `ordering_sequence`（位置）を持てる。claim 述語は同一キーの先行 sequence が未 publish の間その行を選ばないため、consumer から見える範囲は穴の無い連続 prefix になる（[ADR-0072](../adr/0072-postgres-state-dynamodb-eventlog.md)）。順序キーを持たない行は影響を受けない。
- **多インスタンス安全**: `ClaimPending` は `FOR UPDATE SKIP LOCKED` を用い、claim→publish→mark 全体を 1 tx で回すため、行ロックは送出が確定するまで保持される — 2 つの relay が同一行を publish しない。
- **受信側冪等**: 各行は INSERT 時採番の安定 `message_id` を持ち、HTTP `Idempotency-Key` として伝搬。送出側 at-least-once を受信側 dedup で実効 exactly-once 化する。
- **trace 継続**: `Emit` は現在 span の `traceparent` を `headers` へ capture し、publisher が再生するため受信側が起点 trace に繋がる。

---

## 2. 状態遷移

### 2.1 outbox 行ライフサイクル（`status` 列: `pending` / `published` / `dead`）

```mermaid
stateDiagram-v2
    [*] --> Pending: Emit INSERT（業務 tx 内）<br/>status=pending, attempts=0, next_attempt_at=NOW
    Pending --> Published: publish 成功 → MarkPublished<br/>published_at=NOW（status=pending に対し冪等）
    Pending --> Pending: 一時失敗 → MarkFailed<br/>attempts++, last_error 記録, next_attempt_at を先送り<br/>（pending のまま・その時刻まで claim されない）
    Pending --> Dead: 恒久失敗 → MarkFailed の後 MarkDead<br/>（IncDead メトリクス + warn ログ）
    Dead --> Pending: ReplayDead（運用者）→ attempts=0, last_error=NULL, next_attempt_at=NOW
    Published --> [*]: GC が now-Retention(7d) より古い行を削除

    note right of Pending
      failed は status 値ではない。attempts / last_error / next_attempt_at が
      進んだ pending 行のこと。status は 3 値のみ
      （CHECK 制約 = pending / published / dead）。
    end note
    note right of Dead
      replay されるまで終端。種別は publisher（apperror.ErrPermanent）が決め、
      試行回数では決まらない（ADR-0058）。outbox.dead カウンタで顕在化し、
      CLI replay サブコマンドで回復する。
    end note
```

### 2.2 relay poll ループ（`Engine.Run` + `RelayUsecase.RelayBatch`）

```mermaid
stateDiagram-v2
    [*] --> Polling: Engine.Run(ctx)
    Polling --> Batch: RelayBatch(BatchSize) を 1 tx で<br/>ClaimPending(channel) → 各 deliver → mark
    Batch --> RecordLag: 成功 → observeLag（best-effort・失敗時もループ継続）<br/>lag と停止ストリーム数をチャネル別に記録
    RecordLag --> Continue: Claimed が BatchSize と等しく Published が 1 以上
    RecordLag --> WaitPoll: 空振り / 部分消化 / 満杯だが publish 0 件
    Batch --> WaitBackoff: RelayBatch エラー（DB レベルのみ）→ log
    Continue --> Polling: 待機なし（仕事が残る間は積極的に捌く）
    WaitPoll --> Polling: Sleeper.Sleep(PollInterval = 1s)
    WaitBackoff --> Polling: Sleeper.Sleep(ErrorBackoff = 5s)
    Polling --> [*]: ctx 完了 → nil を返す（graceful）

    note right of WaitPoll
      publish 0 件の満杯バッチは必ず待機させる。さもないと
      下流停止時にホットループしてバッチ全体を即時 dead 化する。
    end note
    note right of Batch
      publish 失敗は tx を巻き戻さない（行は次 poll 用に保持）。
      DB / mark 失敗のみ巻き戻し、ループエラーとして返す。
    end note
```

---

## 3. 実装箇所（アーキテクチャ上のどこに在り、どう働くか）

### 3.1 パッケージ配置と依存方向

```mermaid
flowchart TD
    subgraph cmdL["cmd (main)"]
        CMD["cmd/outbox_relay.go<br/>outbox-relay + replay サブコマンド / SIGTERM"]
        CMDJOB["cmd/job.go → job outbox-gc"]
    end
    subgraph cliL["internal/cli/outbox"]
        CLIR["relay.go: RunRelay（start → ctx 待ち → grace 付き stop）"]
        CLIP["replay.go: RunReplayWith（UUID parse → di.RunOutboxReplay）"]
    end
    subgraph diL["internal/di"]
        DIM["module/outboxrelay.go: OutboxRelayModule（relay 専用プロセス）"]
        DIP["module/outboxpublisher.go: outboxPublisherModule（非標準 profile）"]
        DIPER["module/persistence.go: outbox Store（共有）"]
        DIJOB["module/job.go: outbox-gc job（共有・main server）"]
        DIH["outboxrelay/hook: RegisterRelayHooks → SupervisedRunner"]
        DIREP["outboxrelay.go: RunOutboxReplay（一時 app・relay ループ無し）"]
    end
    subgraph ctrlL["internal/controller"]
        ENG["outbox/relay.go: Engine（poll ループ・sleep/backoff・span）"]
        GCJ["job/outboxgc: GC job（one-shot）"]
    end
    subgraph ucL["internal/usecase/outbox"]
        EMIT["emit.go: EmitUsecase"]
        REL["relay.go: RelayUsecase + deliver"]
        GC["gc.go: GCUsecase"]
        RPL["replay.go: ReplayUsecase"]
    end
    subgraph seamL["internal/usecase/boundary"]
        STORE["outbox.Store（Insert/Claim/Mark*/Replay/Delete/Oldest）"]
        PUB["publisher.Publisher（Publish）"]
        TXM["tx.Manager（Do / DoWithResult）"]
        CLK["clock.Clock / Sleeper"]
    end
    subgraph infraL["internal/infrastructure"]
        SQ["rdb/system_cqrs/outbox: Store 実装（sqlc gen + pgerror）"]
        HTTP["publisher/http_publisher.go: httpPublisher（POST, Idempotency-Key）"]
    end
    subgraph crossL["cross-cutting"]
        OBS["observability: OutboxMetrics（outbox.lag_seconds / outbox.dead / outbox.blocked_streams）"]
        CFG["config: OutboxConfig（OUTBOX_*）"]
        HC["infrastructure/httpclient（downstream=outbox）"]
    end

    CMD --> CLIR
    CMD --> CLIP
    CMDJOB --> GCJ
    CLIR --> DIM
    CLIP --> DIREP
    DIM --> DIP
    DIM --> ENG
    DIM --> DIH
    DIH --> ENG
    DIPER --> SQ
    DIJOB --> GCJ
    ENG --> REL
    GCJ --> GC
    DIREP --> RPL
    EMIT --> STORE
    REL --> STORE
    REL --> PUB
    REL --> TXM
    REL --> OBS
    GC --> STORE
    RPL --> STORE
    SQ -- implements --> STORE
    HTTP -- implements --> PUB
    HTTP --> HC
    DIP --> HTTP
    ENG --> CLK
    DIM --> CFG

    classDef done fill:#e6ffed,stroke:#2da44e;
    class CMD,CMDJOB,CLIR,CLIP,DIM,DIP,DIPER,DIJOB,DIH,DIREP,ENG,GCJ,EMIT,REL,GC,RPL,STORE,PUB,TXM,CLK,SQ,HTTP,OBS,CFG,HC done;
```

> 緑 = サブシステムが提供。依存は内向き: `controller`/`infrastructure` → `usecase`/`boundary`、逆は無い。relay の非標準 HTTP profile（`MaxAttempts=1`・`PropagateTrace=false`・`AllowPrivateNetwork=false`）は `outboxPublisherModule` が寄与し、これは **`OutboxRelayModule` の中に入れ子** にしてあるため他プロセスへ漏れない。

### 3.2 バッチ単位のアクション列（relay プロセス）

```mermaid
sequenceDiagram
    participant E as Engine (controller)
    participant U as RelayUsecase
    participant T as tx.Manager
    participant S as Store (infra/rdb)
    participant P as httpPublisher (infra)
    participant R as Receiver (外部)
    E->>U: RelayBatch(BatchSize)
    U->>T: DoWithResult(fn)  // バッチ全体を 1 tx で囲う
    T->>S: ClaimPending(channel, limit)  // FOR UPDATE SKIP LOCKED + バックオフ + head-of-line
    S-->>T: []PendingMessage
    loop 各行
        U->>P: Publish(Message{MessageID, EventType, Payload, Headers})
        P->>R: POST endpoint (Content-Type json, Idempotency-Key=message_id, traceparent)
        alt 2xx
            R-->>P: ok
            U->>S: MarkPublished(id)
        else 非 2xx / transport 失敗
            R-->>P: error (apperror)
            U->>S: MarkFailed(id, err, nextAttemptAt)
            opt 失敗が恒久的
                U->>S: MarkDead(id) + IncDead + warn
            end
        end
    end
    T-->>U: RelayResult{Claimed, Published}
    U-->>E: result
    E->>U: RecordLag() + RecordBlockedStreams()  // best-effort SLI
    E->>E: sleep 判定（continue / PollInterval / ErrorBackoff）
```

---

## 4. integrator が書く箇所（サブシステムが提供しない部分）

サブシステムは **機構一式** を同梱します: emit/relay/gc/replay の各 usecase、RDB `Store`、HTTP `Publisher`、relay `Engine`、GC job、DI 結線、`outbox-relay` / `replay` / `job outbox-gc` の各入口。既定ではイベントは流れません — integrator が両端（生産する呼び出しと消費するエンドポイント）を結線し、プロセスを運用します。

> **Evans からの逸脱 — この面に公開言語が無い。** 同期 HTTP 面には存在する。OpenAPI は、別リポジトリの
> 利用者がこのリポジトリのツールチェーン無しで読める解決済み契約としてコミットされ、drift gate が
> 鮮度を保証している。非同期面には無い。[ADR-0057](../adr/0057-message-id-idempotency-propagation.md)
> が定めているのは*転送*規約（`Idempotency-Key`）であって言語ではない。イベント payload のスキーマも
> `event_type` の語彙も、ここでは定義も公開もされていないため、受信側は両方をこのリポジトリのソースを
> 読んで知ることになる。この非対称は、下記②が payload と `event_type` を integrator に委ねている限りに
> おいては意図的である。自分が所有していないイベントの言語は、誰も公開できない。**まだ提供
> できていない**のは、integrator がそれらのイベントを定義した後、その公開がどういう形を取るべきかの型である。

```mermaid
flowchart LR
    EM["① 業務 tx 内で Emit を呼ぶ<br/>（ドメイン変更と一緒に）"]:::need
    PL["② payload + event_type を定義<br/>（snapshot + version の自己完結）"]:::need
    RC["③ 受信エンドポイントを作る<br/>（Idempotency-Key で冪等）"]:::need
    CF["④ ENDPOINT_OUTBOX を設定（+ tuning）"]:::need
    DP["⑤ relay プロセスをデプロイ<br/>（cmd outbox-relay --channel・チャネルごとに常駐）"]:::need
    GC["⑥ GC をスケジュール<br/>（cron → cmd job outbox-gc）"]:::need
    OP["⑦ dead 行を運用<br/>（outbox.dead 監視 → replay）"]:::need
    EM --> PL --> RC --> CF --> DP --> GC --> OP
    classDef need fill:#fff8c5,stroke:#bf8700;
```

| # | 必要な実装 | 箇所 / やり方 | 参照 |
| --- | --- | --- | --- |
| ① | ドメイン書き込みと同じ `tx.Manager.Do` の中で `EmitUsecase.Emit` を呼び、`Channel` を明示する（既定値は無い。順序を持つチャネルでは `OrderingKey` + `OrderingSequence` も渡す） | 集約を変更する usecase | `emit.go` `EmitInput` |
| ② | `EventType`（`+version`）を選び、**自己完結 snapshot** payload を marshal。`Headers` に `Authorization`/`Cookie` を入れない（既知の機微ヘッダ名は送出前に denylist で落とすが、それは defense-in-depth であって契約ではない） | `Emit` の呼び出し側 | `EmitInput.Payload` / `.Headers` doc |
| ③ | **`Idempotency-Key`（= `message_id`）で dedup** し、永続受理時のみ 2xx を返す受信エンドポイント | 外部サービス | `httpPublisher.Publish` |
| ④ | `ENDPOINT_OUTBOX`（必須。空/不正 URL は relay が起動拒否）+ 任意の `OUTBOX_POLL_INTERVAL` / `OUTBOX_ERROR_BACKOFF` / `OUTBOX_BATCH_SIZE` | `env/` & IaC | `OutboxConfig` 既定値 |
| ⑤ | `cmd outbox-relay --channel=<channel>` を常駐プロセスとして、使用する配送チャネルごとに 1 つ起動（SIGTERM まで常駐・stop で drain） | デプロイ / IaC | `cmd/outbox_relay.go` |
| ⑥ | `cmd job outbox-gc [--batch-size=N]` をスケジュール（k8s CronJob / cron）し retention 超過の `published` 行を剪定 | スケジューラ | `controller/job/outboxgc` |
| ⑦ | `outbox.dead`・チャネル別の `outbox.lag_seconds`（旧来の試行回数の歯止めを置き換える経過時間 alert。[ADR-0058](../adr/0058-outbox-dead-on-permanent-error.md)）・`outbox.blocked_streams` で alert し `outbox-relay replay [--message-id=<uuid>]` で回復 | runbook | `cmd outbox-relay replay` |

> relay・GC・replay は共有 infra 結線を再利用し、非標準 HTTP publisher profile を持つのは常駐 relay プロセスだけ。GC job は main server の job group に在るため `cmd job outbox-gc` は同一バイナリから使える — 単独では走らない。

---

## 5. 用語

| 用語 | 意味 |
| --- | --- |
| **transactional outbox** | 「publish する意図」をドメイン変更と同一 tx の DB 行として記録し、非同期に送出するパターン。dual-write 異常を回避する。 |
| **emit** | 同期側の半身: `EmitUsecase.Emit` が呼び出し側の業務 tx 内で 1 行 INSERT する（`internal/usecase/outbox/emit.go`）。 |
| **relay** | 非同期側の半身: pending 行を claim・publish・mark する常駐 `Engine` poll ループ（`controller/outbox` + `usecase/outbox/relay.go`）。 |
| **Store** | outbox テーブルの永続化ポート（`usecase/boundary/outbox`）。`infrastructure/rdb/system_cqrs/outbox` で sqlc gen 上に実装。 |
| **Publisher** | 送出ポート（`usecase/boundary/publisher`）。HTTP 実装が `ENDPOINT_OUTBOX` へ POST する。 |
| **status** | 行のライフサイクル列 — 厳密に `pending` / `published` / `dead`（CHECK 制約）。`failed` という status は無く、publish 失敗は `pending` のまま。 |
| **attempts / last_error** | publish 試行回数と直近失敗理由。`MarkFailed` が両方を進める。`attempts` は診断のためだけに残り、何かの判定基準ではない。 |
| **next_attempt_at** | その行を再び claim してよくなる時刻。一時失敗のたびに、上限 60 秒の指数バックオフ + full jitter だけ先へ進む。claim 述語は時刻の来ていない行を単に選ばないので、バックオフ中の行はロックもされない。 |
| **delivery channel** | 行を配送するレーン（`delivery_channel`、`NOT NULL`・既定値なし — `http` / `realtime`）。relay プロセスはちょうど 1 チャネルを捌き、チャネル間で何も共有しない。 |
| **ordering key / ordering sequence** | 行が属するストリームと、その中の位置（順序を持たないチャネルではいずれも `NULL`）。 |
| **head-of-line blocking** | ストリームの順序を保つ claim 規則: 同一 ordering key の先行 sequence が未 publish の間、その行は claim できない。 |
| **blocked stream** | 先頭行が `dead` になり、後続が claim されなくなったストリーム。`outbox.blocked_streams` で計上し、先頭を replay して復旧する。 |
| **dead** | publisher が恒久失敗と分類した行（[ADR-0058](../adr/0058-outbox-dead-on-permanent-error.md)）。運用者が replay するまで終端。`outbox.dead` で計上。 |
| **replay** | `dead` 行を `pending` へ戻す（`attempts=0`・`last_error=NULL`・`next_attempt_at=NOW`）。`ReplayUsecase` / `outbox-relay replay [--message-id]`。 |
| **GC（SweepPublished）** | `Retention`（`DefaultRetention = 7d`）より古い `published` 行を `DefaultGCBatchSize = 10000` 件ずつ削除。`cmd job outbox-gc` で実行。 |
| **message_id** | 行ごとの安定 UUID（INSERT 時採番）。受信側 dedup 用に HTTP `Idempotency-Key` として伝搬。 |
| **traceparent** | W3C trace context。emit 時に `headers` へ capture し publisher が再生するため、受信側が起点 trace に繋がる。 |
| **FOR UPDATE SKIP LOCKED** | 多インスタンス relay を安全にする claim 機構 — ロック中の行は保持 tx が確定するまで他インスタンスがスキップする。 |
| **retry-by-poll** | relay の at-least-once 機構: publish 失敗は `pending` のまま残し、`next_attempt_at` 以降の最初の poll で再送する。二重化回避のため transport retry は無効（`MaxAttempts=1`、D10）。 |
| **PollInterval / ErrorBackoff** | 進捗なし poll の後の待機（`OUTBOX_POLL_INTERVAL = 1s`）/ DB レベルの `RelayBatch` エラー後の待機（`OUTBOX_ERROR_BACKOFF = 5s`）。満杯だが publish 0 件のバッチは必ず待機させる。 |
| **BatchSize** | 1 poll で claim する行数（`OUTBOX_BATCH_SIZE = 100`。≤ 0 なら `DefaultBatchSize = 100` へ clamp）。 |
| **outbox.lag_seconds / outbox.dead / outbox.blocked_streams** | サブシステム固有 SLI（meter `go-boilerplate/outbox`、いずれも `channel` 属性を持つ）: 最古 pending 行の経過秒数 / dead 化した行数 / 先頭が dead で停止しているストリーム数。publish の latency/error は `httpclient` downstream=`outbox` 計装が賄う。 |
| **SupervisedRunner** | relay ループを OnStart で起動し OnStop で cancel + drain するライフサイクルヘルパ（`di/lifecycle`）。 |
| **OutboxRelayModule** | relay 専用プロセスの fx module。`outboxPublisherModule` を入れ子にして非標準 HTTP profile が他へ漏れないようにする。 |
