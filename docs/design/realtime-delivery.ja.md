# Realtime Delivery サブシステム設計リファレンス

本書は Realtime Delivery サブシステムの**役割理論・状態遷移・実装位置・integrator が実装すべきもの・用語集**を 1 つのリファレンスに統合したものである。実装がこれに合わせて作られる設計であり、§3 は現在のコードの読解である。本書がシンボル名・既定値・手順の順序を挙げている箇所は意図された具体として扱い、それが属する機構——ordering chain、connection の状態機械、ticket の契約——が統べる。採択の根拠は realtime の ADR 4 本: [ADR-0071 (realtime-delivery-driving-mechanism)](../adr/0071-realtime-delivery-driving-mechanism.ja.md)、[ADR-0072 (postgres-state-dynamodb-eventlog)](../adr/0072-postgres-state-dynamodb-eventlog.ja.md)、[ADR-0073 (sns-sqs-instance-fanout)](../adr/0073-sns-sqs-instance-fanout.ja.md)、[ADR-0074 (query-ticket-stream-authentication)](../adr/0074-query-ticket-stream-authentication.ja.md)。outbox 行が dead になる条件は [ADR-0058 (outbox-dead-on-permanent-error)](../adr/0058-outbox-dead-on-permanent-error.ja.md)。

---

## 1. 役割理論（何を、何のために）

Realtime Delivery は、**feature が commit 済みの event を、それを待っているクライアントへ push し、切断されていたクライアントが喪失も重複もなく追いつけるようにする**ために存在する。REST / Worker / Job と並ぶ第 4 の駆動機構——短命の request でも queue consumer でも one-shot プロセスでもなく、長寿命の Server-Sent Events（SSE）応答——であり、既存の onion の内側に新しい層なしで置かれる。

これは意図的に、チャット機構でも通知機構でも event-sourcing 基盤でも**ない**。見えるのは **destination** 宛ての、**stream** 上の、**subject** のための、**sequence** 位置の、opaque な **payload** を持つ **event** である。会話・メッセージ・operator・受信箱——それらの語は event を emit する feature に属し、このサブシステムの型・分岐・package 名には決して現れない。

機構は同時には走らない 3 つの半身に分かれる。

- **emit** — 同期。feature の業務 transaction の内側。feature の realtime adapter が stream-local sequence を採番し、destination ごとに 1 行の outbox 行を書く。
- **relay** — 非同期。realtime relay がそれらの行を順序どおり EventLog へ流し、全 serve instance を起こす。
- **stream** — 長寿命。serve instance が SSE 接続を保持し、各クライアントの cursor から EventLog を replay し、届いたものを push する。

責務の分割（誰が何を持つか）:

| 部品 | 層 | 責務 | 持たないもの |
| --- | --- | --- | --- |
| **realtime adapter**（`usecase/<feature>/`） | usecase（feature 側） | commit 済みの feature の変更を `DeliveryEvent` に変換し、destination を選び、業務 tx の中で sequence allocator から stream-local sequence を得て、outbox 経由で emit する | transport、replay、connection の状態、sequence そのもの（機構の状態であり feature の aggregate の field には決してならない） |
| **ticket 発行 usecase**（`usecase/<feature>/`） | usecase（feature 側） | subject × destination を認可してから Realtime Delivery に ticket を求める | ticket の形式や保存 |
| **boundary/realtime** | usecase/boundary | 継ぎ目: `DeliveryEvent`、`SequenceAllocator`、`EventLogStore`、`StreamTicketStore`、`InstanceLeaseStore`、`RevocationNotifier` | 実装、feature の語彙 |
| **usecase/realtime** | usecase | ticket の発行 / 検証、cursor の検証と失効、replay の読み出し、lease heartbeat、orphan cleanup の所有権 | HTTP transport、poll loop |
| **realtime relay**（`controller/outbox` + realtime publisher） | controller / infrastructure | realtime channel の行を stream 順に claim し、EventLog へ append し、wakeup を publish する | 業務判断、[ADR-0058] を超える retry 方針 |
| **Streamer**（`controller/stream/`） | controller | connection registry（subject で索引）、capacity gate、replay / catch-up のスケジューリング、heartbeat、backpressure、drain、control-event protocol | 認可、feature の語彙 |
| **EventLog / ticket / lease store**（`infrastructure/eventlog`、`streamticket`、`instancelease`） | infrastructure | boundary store の DynamoDB 実装 | 業務判断 |
| **fan-out substrate**（`infrastructure/realtime/`） | infrastructure | SNS publish（realtime publisher と revocation notifier）、instance ごとの SQS queue / subscription の lifecycle、instance 自身の queue に対する receive と delete | wakeup の意味 |
| **consumer engine**（`controller/realtime/`） | controller | instance 自身の queue を drain し、batch ごとに wakeup を coalesce して、それと revocation を Streamer の sink へ渡す。instance lease の heartbeat loop | wakeup をどう扱うか（Streamer が持つ）・transport |
| **orphan-cleanup job** | controller/job + cli | crash した instance の queue / subscription / lease を conditional な所有権の下で回収する | スケジューリング（scheduler が持つ。[ADR-0109 (scheduled-job-concurrency-delegated)](../adr/0109-scheduled-job-concurrency-delegated.ja.md)） |
| **DI module** | di | feature adapter が 1 つ以上あるときだけ runtime を結線する | 業務ロジック |
| **RealtimeConfig** | config | deployment 依存の knob のみ（§3.3） | 固定の protocol 値 |

設計の不変条件:

- **ordering chain は 1 つ。** feature の commit 順 → outbox → EventLog 可視化 → client cursor は単一の不変条件である（[ADR-0072]）: sequence に gap は無く、client-visible な sequence は連続 prefix を成し、terminal failure は sequence を飛ばさず stream を止める。
- **クライアントに配るのは EventLog だけ。** wakeup は状態を運ばない。重複 wakeup は同じ catch-up read であり、欠落は periodic catch-up が覆う。
- **feature 非依存は依頼ではなく強制。** Realtime Delivery は `internal/domain/<feature>` も `internal/usecase/<feature>` も import しない。`depguard` と architecture test が build を落とす。
- **adapter ゼロなら runtime ゼロ。** feature adapter が結線されていなければ、serve graph は Streamer を起動せず EventLog も fan-out substrate も要求しない——sample feature は機構を残したまま削除できる。
- **認可は端で行い、Streamer では行わない。** feature が ticket 発行時に認可し、identity resolver が全 REST request を認可する。Streamer は ticket を検証し失効に従うだけである。
- **capacity は instance ごとに守り、rate は守らない。** connection と replay の capacity はプロセス内で有界にする。クライアントがどれだけ頻繁に再接続してよいかは edge の関心である（[ADR-0108 (no-in-app-rate-limiter)](../adr/0108-no-in-app-rate-limiter.ja.md)）。

---

## 2. 状態遷移

### 2.1 配送の lifecycle（1 つの event の端から端まで）

```mermaid
flowchart LR
  cmd["Feature command<br/>(business tx)"]
  seq["allocate sequence<br/>UPDATE sequence row (system_cqrs) … RETURNING<br/>lock held to commit"]
  outbox["outbox row<br/>delivery_channel=realtime<br/>ordering_key / ordering_sequence"]
  relay["realtime relay<br/>claim in stream order"]
  log["EventLog append<br/>conditional on (streamId, sequence)"]
  sns["SNS publish<br/>eventId / streamId / sequence"]
  sqs["SQS · one queue per serve instance"]
  streamer["Streamer<br/>coalesce wakeup → read after cursor"]
  sse["SSE write<br/>id: sequence"]
  cmd --> seq --> outbox --> relay --> log --> sns --> sqs --> streamer --> sse
  streamer -. "periodic catch-up (30 s + jitter)" .-> log
```

- sequence は機構の **sequence allocator** が採番する——Realtime Delivery が所有する `system_cqrs` の table に stream ごと 1 行を持ち、feature 自身の書き込みと同じ transaction の中で `UPDATE … RETURNING` し、行ロックを commit まで保持する。この行は outbox 行と並ぶ機構の状態であり、sequence を field に持つ aggregate も採番する Repository も無い。したがって採番順 = commit 順で、rollback は増分を戻す: **gap なし**。
- 複数の destination（たとえば会話 stream と operator feed）に届ける feature は、同じ transaction で destination ごとに 1 行の outbox 行を emit する。各行は自分の ordering key と sequence を持ち、機構は 1 つの event を複数 stream に分割しない。
- EventLog の append は `(streamId, sequence)` が存在しないことを条件とするので、部分的な失敗の後の outbox retry は冪等である。

### 2.2 ordering chain を状態機械として（stream ごと、outbox 内）

```mermaid
stateDiagram-v2
    [*] --> Pending: emit (in business tx)
    Pending --> Pending: predecessor on same ordering_key unpublished<br/>→ not claimable (head-of-line)
    Pending --> Claimed: claim — FOR UPDATE SKIP LOCKED<br/>AND next_attempt_at <= now()<br/>AND no earlier unpublished sequence on this key
    Claimed --> Published: append OK → publish wakeup → MarkPublished
    Claimed --> Pending: retryable error → next_attempt_at += backoff (full jitter, cap 60 s)
    Claimed --> Dead: permanent error → MarkDead
    Dead --> Pending: ReplayDead (operator)
    note right of Dead
      A dead head halts its stream: successors stay Pending
      (never claimed). Surfaced as realtime_blocked_streams.
      The realtime channel produces no permanent error of
      its own — payload validity is checked before emit —
      so this state is reached only through a systemic fault.
    end note
```

retryable か permanent かは attempt 回数ではなく error 分類で決まる（[ADR-0058]）。backoff 中の行は claim 述語に選ばれないだけで、lock されることがないため `SKIP LOCKED` の意味論は変わらない。

### 2.3 connection の lifecycle（1 つの SSE 接続）

```mermaid
stateDiagram-v2
    [*] --> Verify: GET stream?ticket=…&after=…<br/>Last-Event-ID (on reconnect)
    Verify --> Rejected400: cursor malformed / negative / overflow → 400 invalid_stream_cursor
    Verify --> Rejected401: ticket unknown, expired, or revoked → 401
    Verify --> Rejected410: cursor below replay floor → 410 stream_cursor_expired
    Verify --> Rejected503: instance connection cap reached,<br/>or initial-replay slot not acquired within the bounded wait,<br/>or Realtime dependencies degraded → 503 + Retry-After
    Verify --> Streaming: 200 committed<br/>Cache-Control: no-store · X-Accel-Buffering: no
    Streaming --> Streaming: replay after cursor → business events (id: sequence)
    Streaming --> Streaming: wakeup or periodic catch-up → read after cursor
    Streaming --> Streaming: heartbeat every 15 s (comment, no id)
    Streaming --> Closed: client EOF / write deadline (10 s) exceeded
    Streaming --> Closed: buffer full (64 events) → close, no event dropped (replay covers it)
    Streaming --> Closed: control STOP (revoked) · REAUTHENTICATE (1 h lifetime) · RECONNECT (drain) · RETRY_LATER · RESYNC
    Closed --> [*]: capacity returned
```

cursor の解決順: 有効な `Last-Event-ID` が勝つ。無ければ `after`。それも無ければ ticket の初期 cursor。初期 cursor は開始位置であって認可の下限ではない — client は replay floor がまだ覆う範囲なら、それより前の位置からも再開できる。可視範囲を狭めたい feature は destination を分ける。cursor は ticket の destination 以外に対して決して有効ではない。

拒否できるものはすべて response の commit **前**に拒否する。commit 後にクライアントへ戻る経路は in-band の control event と close だけである（§4.3）。

### 2.4 ticket の lifecycle

```mermaid
stateDiagram-v2
    [*] --> Issued: feature authorizes subject × destination → 256-bit opaque value<br/>stored as hash, bound to subject / destination / scope / expiry (TTL 5 min)
    Issued --> Issued: presented on connect (reusable within TTL)
    Issued --> Expired: TTL elapsed → new connections refused
    Issued --> Revoked: revocation seam (subject × destination) → new connections refused,<br/>open connections receive STOP
    Expired --> [*]
    Revoked --> [*]
```

ticket の TTL は *新しい* 接続を開始できる期限を定める。確立済みの接続は別途 **maximum connection lifetime（1 時間）**で有界であり、到達するとサーバーは `REAUTHENTICATE` を送って閉じる。クライアントは feature の認可経路で新しい ticket を得る。この service 内での失効（membership の soft-delete、destination アクセスの剥奪）は即時——ticket を無効化し、fan-out 経由で該当接続を閉じる。identity provider での失効は観測せず、lifetime だけで収束する（[ADR-0074]、`auth.md` §2）。

### 2.5 serve instance の lifecycle と instance lease

```mermaid
stateDiagram-v2
    [*] --> Provisioning: Start — verify EventLog reachable → write the instance lease
    Provisioning --> Subscribed: create SQS queue → subscribe to SNS topic → start consumer
    Subscribed --> Ready: HTTP listen; lease heartbeat every 30 s (expiry 2 min)
    Ready --> Draining: Stop — refuse new connections; send SERVER_DRAINING / RECONNECT; drain SSE
    Draining --> Unsubscribing: stop consumer → unsubscribe → delete queue → delete lease
    Unsubscribing --> [*]: HTTP shutdown
    Ready --> Crashed: process dies without Stop
    Crashed --> Orphaned: lease expires (2 min) + safety margin (5 min)
    Orphaned --> Reclaimed: orphan-cleanup job takes ownership by conditional update<br/>unsubscribe → delete queue → delete lease
```

起動順は provisioning → subscription → consumer → HTTP listen / ready であり、instance lease は queue が存在するより**先**に書く。lease に名指される前に死んだプロセスが queue を残した場合、orphan-cleanup job は期限切れの lease 経由でしか resource に辿り着けないため、その queue は二度と回収できないからである。停止はその逆順で、SSE の drain を consumer 停止と `http.Server.Shutdown` の**前**に行うため、長寿命接続が shutdown を塞ぐことはない。そして lease は queue と subscription が消えた後に**最後**に削除する（teardown に失敗しているのに lease を先に消すと、残骸が cleanup job から見えなくなる）。

instance の識別子はプロセス起動ごとに新しく採番し、再起動をまたいで残る値（hostname や pod 名）から導かない。instance ごとの queue 名は `<prefix>-<instance id>`、lease も同じ識別子をキーにするため、再起動したプロセスが前世代の識別子を使い回すと、前世代の lease を heartbeat で延命しつつ、まだ生きているプロセスが consume 中の queue を片付けてしまう。起動ごとの新しい識別子は前世代の残骸をただの orphan にし、cleanup job が回収できる唯一の状態へ倒す。

instance は生きたまま受信先を失うこともある。cleanup の安全余裕はこれを起こりにくくするだけで、無くしはしない
——停滞した instance の lease を引き受けた回収は、後で再開するプロセスの queue を消す。subscription は、消えた
queue への受信も、まだ作っていない受信先への受信も、同じ `ErrReceivingEndGone` として報告する。どちらも復旧が
同一だからであり、分けると作り直しに失敗した次の周回が復旧の対象から外れる。consumer の loop はこれを受けて
作り直しを依頼し、順序は上の起動順序と同じく lease が先である。理由も起動順序と同じ。その順序は lease と
subscription が出会う場所——DI module——で合成する。subscription 自身は lease を知らないので、そこには置かない。
作り直しの失敗はその場で再試行しない。次の受信が同じように失敗して再び依頼するので、AWS が queue の削除から
同名 queue の作成まで要求する 60 秒は、その繰り返しが越える。

### 2.6 degraded 時の動作

| 条件 | 新規 SSE 接続への影響 | 既存接続への影響 | `/ready` への影響 |
| --- | --- | --- | --- |
| 起動時に EventLog または fan-out へ到達不能 | runtime は起動しない。起動は fail-fast | — | not ready |
| 稼働中に EventLog または fan-out へ到達不能 | `503` + `Retry-After` | 維持する。依存が戻れば periodic catch-up が配送を再開する（一斉 `RETRY_LATER` は回復を reconnect storm に変えるため送らない） | REST は healthy のまま。realtime は degraded として報告。これだけを理由に instance を load balancer から外さ**ない** |
| instance の connection 上限到達 | `503` + `Retry-After` | 影響なし | 変わらない |
| replay の並行数が飽和 | 初回 replay は有界時間待ってから `503` | catch-up は順番を待つ（cancellation を尊重） | 変わらない |

---

## 3. 実装位置（どこに在るか）

本節は**計画された**配置を述べる。依存方向と allowlist が統べる部分で、package 名は意図された具体である。

### 3.1 package の配置と依存方向

| package | 内容 |
| --- | --- |
| `internal/usecase/boundary/realtime/` | `DeliveryEvent`（eventId / streamId / 10 進文字列の sequence / type / occurredAt / schemaVersion / payload ≤ 64 KiB）、`SequenceAllocator`（`Allocate(streamID)` — 次の sequence、行ロックは commit まで / `Current(streamID)` — stream の現在位置。History の cursor に使う）、`EventLogStore`（conditional append、cursor 以降の `ConsistentRead` 読み出し、降順 1 件の latest）、`StreamTicketStore`（hash 化 ticket、bind、無効化）、`InstanceLeaseStore`（heartbeat、expiry、conditional な cleanup 所有権）、`RevocationNotifier`（subject が destination を失ったことを全 instance へ伝える） |
| `internal/usecase/realtime/` | ticket の発行 / 検証、失効 seam `AccessRevoker`（ticket を無効化してから通知）、cursor の検証と replay floor の導出、replay の読み取り、lease の heartbeat と orphan cleanup の引き受け |
| `internal/controller/stream/` | subject で索引する connection registry、capacity gate、初回 replay の admission、replay / catch-up の semaphore と jitter 付き scheduler、heartbeat、write deadline、buffer と満杯時 close、drain、control-event writer、非 strict の SSE handler |
| `internal/infrastructure/rdb/system_cqrs/realtime/` | PostgreSQL の `system_cqrs` table（`stream_id`、`last_sequence`）上の `SequenceAllocator`——outbox / idempotency の table と同じ区分（[ADR-0033 (system-cqrs-dml-category)](../adr/0033-system-cqrs-dml-category.ja.md)） |
| `internal/infrastructure/eventlog/dynamodb/`、`internal/infrastructure/streamticket/dynamodb/`、`internal/infrastructure/instancelease/dynamodb/` | DynamoDB の store。idempotent な one-shot table initializer（application 起動時には作らない） |
| `internal/infrastructure/realtime/aws/` | realtime publisher（EventLog append → SNS publish）、revocation notifier、`InstanceSubscription` port（provision / receive / delete / teardown）の背後にある instance ごとの SQS queue / subscription の lifecycle。`internal/infrastructure/realtime/local/` は emulator の queue attribute の集合だけ——wire 非互換が判明した唯一の呼び出しの組 |
| `internal/controller/realtime/` | `InstanceSubscription` を drain し、wakeup / revocation を `controller/stream` が実装する sink へ渡す consumer engine（`controller/outbox` や `controller/worker` と同じ driving adapter の形）。lease の heartbeat loop |
| `internal/controller/job/<orphan-cleanup>/` + `internal/cli/` | cleanup job の入口 |
| `internal/di/module/realtime.go` | feature adapter が 1 つ以上 provide されたときだけ結線する。`internal/di/server/hook` が §2.5 の順序で走らせる serve lifecycle の participant（startup probe、lease と queue の provisioner、consumer と heartbeat の runner）を提供する |
| `internal/usecase/<feature>/` | feature の realtime adapter と ticket 発行 usecase |

```mermaid
flowchart LR
  feature["usecase/&lt;feature&gt;<br/>adapter · ticket issue · revoke"] --> boundary["usecase/boundary/realtime"]
  feature --> ucrt
  ucrt["usecase/realtime"] --> boundary
  stream["controller/stream"] --> ucrt
  consumer["controller/realtime"] --> ucrt
  consumer --> boundary
  infra["infrastructure/eventlog · streamticket · instancelease · realtime"] --> boundary
  feature -. "never" .-x stream
  stream -. "never" .-x feature
```

architecture rule（いずれも機械的に検査される）:

1. `boundary/realtime`、`usecase/realtime`、`controller/stream`、`controller/realtime`、5 つの infrastructure package は `internal/domain/<feature>` も `internal/usecase/<feature>` も import しない。
2. `InstanceLeaseStore` を import できるのは realtime package 群、realtime DI module、orphan-cleanup job の入口だけ。

規約 1 は意図的に二重で検査される: `depguard`（`maintain_realtime_feature_neutrality`、`.golangci.yaml`——`make go-lint` と CI が実際に走らせる config——に宣言され、editor でも表示されるよう `.golangci.yaml` にも複製されている。[ADR-0088 (layered-golangci-config)](../adr/0088-layered-golangci-config.ja.md)）が他の layer rule と並んで lint 時点で落とし、`internal/architest/realtime_isolation_test.go` が test 時点で落とすのに加えて、走査対象の package 一覧が今も存在することまで assert する——対象がリネームで消えたルールは、linter には報告できない唯一の失敗である。規約 2 は architecture test にしか存在しない: これは単一の**シンボル**を制約するものであり、`depguard` は package 丸ごとしか拒否できないため、そこで表現すると同じ package からの正当な `EventLogStore` / `StreamTicketStore` の import まで拒否してしまう。

この二重検査には既知の穴が 1 つある。`depguard` は import path を素の prefix で照合するため、`usecase/realtime` を機構へ戻す `allow` 記述は、その文字列で始まるだけの将来の兄弟 package まで通してしまう。architecture test の `isFeatureImport` も同じ導出をしているため、二つの検査は互いを補えず同じ弱点を共有している。現時点でそのような package は存在しない——制約は、`internal/usecase/` に `realtime` で始まる名前の feature package を作らないことである。

### 3.2 このサブシステムが依拠する outbox の追加

outbox の delivery-channel の作業で追加され、全 channel で共有される。

- `delivery_channel` — `NOT NULL`、default なし（channel を忘れた emit は黙って HTTP に落ちるのではなく失敗する）。
- `ordering_key` / `ordering_sequence` — nullable。順序を持たない channel は `NULL`。
- `next_attempt_at` — message ごとの backoff（[ADR-0058]）。
- channel ごとに 1 つの relay 実行 loop。claim 述語は channel、`next_attempt_at <= now()`、§2.2 の head-of-line 規則を加える。

### 3.3 設定と固定値

| 種別 | 値 |
| --- | --- |
| **typed config**（deployment 依存） | EventLog endpoint / region、各 store の table 名を導く table suffix（`<store>_<suffix>`。並行する checkout が 1 つの DynamoDB を共有できるのはこれによる）、credentials（空 → SDK default chain）、fan-out endpoint、SNS topic、SQS resource prefix、DLQ（空 → redrive しない）、いずれも AWS では ARN、**instance あたりの最大 SSE 接続数**、**replay / catch-up の並行数** |
| **code 上の固定値**（それぞれ単一の正規定義） | write deadline 10 s、heartbeat 15 s、connection ごとの buffer = replay 1 ページ分（64。1 ページが buffer を溢れさせないため）、catch-up 間隔 30 s、jitter 比率（間隔の 5 分の 1）、初回 replay の枠を待つ上限 2 s、drain の予算 10 s、503 の `Retry-After` と `retryAfterMs` が共用する目安 5 s、ticket TTL 5 分、maximum connection lifetime 1 時間、lease heartbeat 30 s / expiry 2 分 / cleanup margin 5 分、payload 上限 64 KiB、EventLog 保持 7 日、backoff 上限 60 s、instance queue の visibility timeout 30 s / long polling 20 s / redrive の `maxReceiveCount` 5 |

### 3.4 observability の契約

metric は feature 非依存（`realtime.*` という名前で、Prometheus exporter が `realtime_*` として出す）で、subject / user / stream / destination / event / message / trace / ticket の識別子を label に**持たない**。個別の相関は trace と structured log で行う。

close は閉じ方ごとに metric を分けず、`reason` を持つ 1 本で表す——遅い client も通常の EOF も、その label で読み分ける同じ系列である。`result` と `outcome` も同様で、実行とその成功は別々の counter ではない。

| 群 | metric（`internal/observability/realtime_metrics.go`） |
| --- | --- |
| connection | `connections.active`、`connections.accepted`、`connections.rejected`（`reason`: capacity / degraded / draining）、`connections.closed`（`reason`。遅い client はここに計上される）、`connections.reconnects`（`Last-Event-ID` または `after` を伴う接続）、`connections.duration_ms` |
| replay / catch-up | `replay.executions`（`trigger`）、`replay.events`、`replay.depth`、`replay.failures`、`replay.in_flight`（並行数の枠の占有）、`replay.admission_timeouts`（上限内に枠を取れなかった回数）、`catchup.lag_ms` |
| delivery | `eventlog.appends`（`result`）、**`delivery.latency_ms`** = `occurredAt` → SSE write 成功（2 つの instance を跨ぐため clock skew を含む。構造上の近似値）、**`eventlog.lag_ms`** = outbox `created_at` → append、`wakeup.publish_failures`、`recovery.executions`（`result`） |
| cleanup | `lease.heartbeat_failures`、`cleanup.executions`（`result`）、`cleanup.instances`（`outcome`） |
| outbox | delivery channel ごとの lag、channel ごとの最古 pending 行の age（attempt 回数に代わる alert。[ADR-0058]）。これらは realtime ではなく outbox の meter に載る——**blocked stream**（先頭が dead）もそこから読む |

trace: command → outbox → relay → EventLog は outbox header を通じて 1 つの trace を共有する。長寿命接続は元の command の child span にはならない。各 delivery / replay 操作は短い span で、event の origin trace へ span link を張る。公式の OpenTelemetry semantic convention のみを使う（[ADR-0077 (official-otel-semconv)](../adr/0077-official-otel-semconv.ja.md)）。payload、ticket、query credential は span attribute にも log field にも決して現れない。

---

## 4. integrator が実装するもの（サブシステムが提供しない部分）

### 4.1 feature 側

| 成果物 | 契約 |
| --- | --- |
| **destination の対応付け** | この feature にとって stream が*何であるか*を決める（会話、user ごとの受信箱、組織の feed）。サブシステムは何も解釈しない。 |
| **event type と payload schema** | `<feature>.<noun>.<verb>.vN` + `schemaVersion`。どちらも維持する。client generator が型付けできるよう OpenAPI component として宣言する。payload は 64 KiB 以下で自己完結し、credential・token・ticket・cookie・binary・feature が既に公開している範囲を超える生の個人情報を含まない。 |
| **sequence の採番** | 業務 transaction の中で `SequenceAllocator.Allocate(streamID)` を呼ぶ——機構自身の stream 行を `UPDATE … RETURNING` し、commit まで lock を保持する。aggregate に sequence の field を持たせず、Repository で採番しない。 |
| **emit** | destination ごとに 1 行の outbox 行。`delivery_channel = realtime`、`ordering_key` = stream、`ordering_sequence` = 採番した sequence。 |
| **ticket 発行 endpoint** | subject × destination を認可してから ticket を得る。destination の種類ごとに 1 endpoint。scope は feature が定義する。 |
| **正規の recovery 経路** | 返す行と整合する `streamCursor` を response に持つ読み出し endpoint（History、一覧）。cursor を**先に**読み（`SequenceAllocator.Current`）、次に `sequence <= cursor` の行を読む: 採番が行ロックを commit まで保持するため、読めた cursor 以下の行はすべて commit 済みで、それより上は除外される——既定の `READ COMMITTED` のままで単一 snapshot と等価になり、分離レベルの変更も単一 SQL の join も要らない。`RESYNC` と `410` はどちらもクライアントをここへ送る。 |
| **失効の呼び出し** | feature が subject の destination アクセスを取り消すとき、および membership の soft-delete 経路から、失効 seam を呼ぶ。 |
| **command の idempotency** | timeout 後のクライアント再送で効果が重複する変更 endpoint を idempotency middleware に opt-in する。 |

### 4.2 deployment 側

- **DynamoDB**: 3 table（`occurredAt` 由来の失効で TTL 7 日の EventLog、TTL 付きの StreamTicket、InstanceLease）。partition = stream。at-rest 暗号化。point-in-time recovery、backup、alarm はこの repository の外で provisioning する（[ADR-0106 (vendor-neutral-deploy-skeleton)](../adr/0106-vendor-neutral-deploy-skeleton.ja.md)）。単一の hot stream は 1 partition と 1 PostgreSQL 行で有界であり、機構はそれを shard しない。
- **SNS / SQS**: standard topic を 1 つ。deployment が provisioning し、`REALTIME_TOPIC` でアプリケーションに名前を渡す（ローカルでは `make realtime-init` が作る）。アプリケーション自身が作る resource は instance ごとの standard queue だけで、serve 起動時に `<REALTIME_QUEUE_PREFIX>-<instance id>` として作る。属性は次のとおり: `aws:SourceArn` がその topic のときに限り `sns.amazonaws.com` からの `sqs:SendMessage` を許可する access policy、subscription の `RawMessageDelivery=true`、long polling（`ReceiveMessageWaitTimeSeconds=20`）、`VisibilityTimeout=30`、`SqsManagedSseEnabled=true`（customer-managed な KMS 鍵は機構が公開する調整点ではない）、そして `REALTIME_DLQ` が設定されているときは共有 DLQ への `maxReceiveCount=5` の `RedrivePolicy`——DLQ 自体は deployment が provisioning するものであり、wakeup を 1 つ失っても定期 catch-up が拾うので、正しさの要件ではなく運用上の安全網である。最小 IAM: relay は topic への `sns:Publish`。serve instance は `<prefix>-*` への `sqs:CreateQueue` / `sqs:GetQueueAttributes` / `sqs:SetQueueAttributes` / `sqs:ReceiveMessage` / `sqs:DeleteMessage` / `sqs:DeleteQueue` と、topic への `sns:Subscribe` / `sns:SetSubscriptionAttributes` / `sns:Unsubscribe`、および revocation のための topic への `sns:Publish`。orphan-cleanup job はこれに加えて、topic への `sns:ListSubscriptionsByTopic` と `<prefix>-*` への `sqs:GetQueueUrl` が要る——死んだ instance の queue へは、キャッシュした URL ではなく名前から辿るためで、そこでの拒否は「既に無い」と区別が付かないため、queue が残ったまま job が毎回失敗し続ける。`sns:CreateTopic` はどこも必要としない。
- **edge**: HTTPS。固定の CORS origin。クライアントの CSP `connect-src`。load balancer / proxy の idle timeout は heartbeat 間隔より長く、stream path では response buffering を無効化。reconnect の rate limiting があるなら edge で。**stream path の query string を edge / proxy / load balancer の access log から除外または redact する**——ticket は query parameter で運ばれ、プロセス内の除去はプロセス外で書かれる log には届かない。
- **local**: shared infrastructure profile の DynamoDB Local と SNS/SQS emulator（GoAWS）。table と topic は idempotent な initializer が作り、table 名・topic・queue prefix のすべてに worktree slot ごとの suffix を付ける——emulator を共有する checkout どうしが状態を共有しないようにするためである。GoAWS は queue policy を受け付けず、redrive / 暗号化の属性も尊重しないため、emulator 用の queue attribute の集合（`infrastructure/realtime/local`）は timing の属性だけを設定する。DI module は emulator の環境ではこちらを、`dev` 以降では完全な集合を選ぶ。

**Operate.** threshold は deployment が持つ——これらは traffic、instance 数、business が必要とする retention に従うものであり、いずれもこの repository の知るところではない——しかし*何を見るか*は deployment の持ち物ではない。それは機構から導かれるからである。以下の signal はいずれも §3.4 の metric 群である。上記の resource を provisioning しても、これらを配線しない deployment はサブシステムを盲目のまま運用することになる。

| signal | 意味 | 最初の runbook 対応 |
| --- | --- | --- |
| **blocked stream**（先頭が dead） | ordering chain が stream 上で停止している: sequence を skip したのではなく terminal failure が止めたので、dead な先頭より後は何も配送されない。自動では解消しない唯一の alert。 | dead な outbox 行を見つけ、原因を修正するか破棄したうえで `outbox-relay replay --message-id=<uuid>` を実行する。動くまではその stream は全クライアントに対して止まったままになる。 |
| **EventLog lag**（outbox `created_at` → append） | relay が遅れている。クライアントには commit 済みの変更が遅れて見え、relay が追いつくまで gap が広がり続ける。 | relay が動いているか（`outbox-relay --channel=realtime`）、DynamoDB が append を受け付けているかを確認する。死んだ relay は error ではなく lag として現れる。 |
| **delivery latency**（`occurredAt` → SSE write） | EventLog lag が横ばいのまま上昇しているなら、ボトルネックは relay ではなく stream 側にある: replay の飽和、slow client、または instance が capacity 上限に達している、のいずれか。 | scale する前に replay の並行数の飽和と rejected connection 数を比較する。slow client の問題は instance を増やしても直らない。 |
| **rejected connection**（capacity） | instance が `REALTIME_MAX_CONNECTIONS` に達している。クライアントは `503` で追い返され、どれだけ強く retry するかは edge の reconnect policy が決める。 | instance を追加するか、process に memory と file descriptor の余裕があれば instance あたりの上限を引き上げる（§3.3）。 |
| **heartbeat 失敗 / expired instance** | lease が更新されていない。死んだ instance は queue と subscription を残したままになり、費用がかかるうえ誰も読まない message を受信し続ける。 | orphan-cleanup job が scheduled され成功しているか確認する。cleanup の実行が無いまま expired 件数が増えているなら、job が動いていないということである。 |
| **cleanup 失敗** | job は動いているが回収できていない。多くは IAM が原因: `<prefix>-*` への `sns:ListSubscriptionsByTopic` と `sqs:GetQueueUrl` が要り、拒否は「既に無い」と区別が付かない。 | ロジックより先に job の permission を確認する——これは上の IAM の一覧がまさに防ごうとしている失敗である。 |
| **append 失敗 / recovery 失敗** | EventLog 自体が書き込みを拒否している: throttling か、table がコードの想定と一致しなくなっている。 | DynamoDB の capacity と、table の key schema・TTL が initializer が作るものと一致しているかを確認する。 |

DLQ が溜まっていくことはこの一覧に含めていない。それは正しさの signal ではないからである: wakeup を 1 つ失っても定期 catch-up が拾う。replay しなければならない配送としてではなく、fan-out の健全性についての証拠として扱う。

### 4.3 クライアントの契約

クライアントはこの状態機械を実装しなければならない。サーバーはそれを前提にする。

```mermaid
stateDiagram-v2
    [*] --> Connecting: EventSource(stream?ticket=…&after=…)
    Connecting --> Connected: 200
    Connecting --> Backoff: 503 → wait Retry-After (+ jitter)
    Connecting --> Resync: 410 → re-read canonical state, new streamCursor
    Connecting --> Authenticating: 401 → new ticket
    Connected --> Reconnecting: EOF / network error (Last-Event-ID kept by the browser)
    Connected --> Reconnecting: control RECONNECT
    Connected --> Backoff: control RETRY_LATER (retryAfterMs, advisory; add jitter)
    Connected --> Authenticating: control REAUTHENTICATE
    Connected --> Resync: control RESYNC
    Connected --> Stopped: control STOP
    Reconnecting --> Connecting
    Backoff --> Connecting
    Authenticating --> Connecting: new ticket obtained
    Resync --> Connecting: after=streamCursor, new ticket
```

- control event は `event: control`、JSON 本文 `{action, reason, retryAfterMs?}`、**SSE `id` なし**。business event だけが sequence を `id` に持つので、`Last-Event-ID` が control plane に汚されることはない。`retryAfterMs` は milliseconds、commit 前の `Retry-After` header は秒。
- `STOP`、`REAUTHENTICATE`、`RESYNC` は**同期的に**処理しなければならない: クライアントはサーバーの EOF が届く前に自分で `EventSource` を閉じる。さもなければブラウザが自動再接続して reconnect loop になる。
- reason code は安定した machine-readable な値（`SERVER_DRAINING`、`TEMPORARILY_OVERLOADED`、`AUTH_REFRESH_REQUIRED`、`AUTHORIZATION_REVOKED`、`CURSOR_TOO_OLD`、`STREAM_RECOVERY_FAILED`）。クライアントは human-readable な message で分岐しない。feature 固有の reason はこの層には存在せず、feature が自分の event payload で表現する。
- control event の配送は保証されない。クライアントは裸の EOF からも回復しなければならない——control event は回復を改善するものであり、その correctness ではない。
- 試験専用の Go 参照クライアントを integration test に置き、この契約を固定する。出荷する SDK ではない。

---

## 5. 用語集

| 語 | 意味 |
| --- | --- |
| **Realtime Delivery** | このサブシステム: emit → relay → stream、replay 付き。何をするかで名付ける。`core`、`platform` 等の容器名では呼ばない。 |
| **DeliveryEvent** | feature 非依存の封筒: `eventId`（= outbox `message_id`）、`streamId`、`sequence`、`type`、`occurredAt`、`schemaVersion`、`payload`。 |
| **destination** | stream が*何のためか*。feature が決める（会話、受信箱、feed）。サブシステムは何も解釈しない。 |
| **stream** | 順序と replay の単位。`streamId` は EventLog の partition key。1 つの event はちょうど 1 つの stream に属する。 |
| **subject** | ticket が bind され connection registry が索引する feature 非依存の principal。サブシステムはそれが user か operator かを知らない。 |
| **sequence** | feature の業務 transaction で採番される、stream-local で gap の無い単調増加の event 位置。wire では 10 進文字列。 |
| **sequence 行** | 機構が stream ごとに持つ行（`stream_id`、`last_sequence`）。`system_cqrs` の table にあり、feature の transaction の中で allocator が更新し commit まで lock する。outbox 行と並ぶ機構の状態で、aggregate の一部ではない。 |
| **ordering key / ordering sequence** | relay が head-of-line 規則を保てるよう stream と sequence を運ぶ outbox の列。 |
| **head-of-line blocking** | claim 規則: 同じ ordering key でより小さい sequence が未 published の間、行は claim できない。 |
| **contiguous prefix** | クライアントが stream 上で見られるものはすべて穴の無い `1..N` である、という不変条件。 |
| **cursor** | クライアントが見た sequence。ブラウザ再接続時は `Last-Event-ID`、明示的な resume 時は `after`。 |
| **replay** | 接続が開いたときに cursor 以降の EventLog を読むこと。 |
| **catch-up** | wakeup を受けて、または periodic（30 s、jitter 付き）schedule で、現在の cursor 以降の EventLog を読むこと。 |
| **replay floor** | まだ replay できる最古の sequence。ログと append watermark から導出する: `cursor + 1` が無く後続が存在する、または存在しても retention より古い → `410`。cursor より後ろに何も無く `cursor < watermark` → `410`（続きは追記され、その後に期限切れした）。後ろに何も無く `cursor >= watermark` → replay 可能（追いついているか、relay がまだ cursor に到達していない）。cursor *自身*の item が残っているかは関与せず、初期位置は watermark `0` に対する cursor `0` である。 |
| **append watermark** | relay が stream の EventLog へ書いた最大の sequence。stream ごとに保持し、後戻りしない。event そのものより長く残るため、item が無いときに「retention で消えた」のか「まだ書かれていない」のかを分けられる。 |
| **wakeup** | SNS → SQS の通知「stream S を cursor の後から読み直せ」。状態を持たず、重複は畳まれ、欠落は catch-up が覆う。 |
| **blocked stream** | 先頭行が dead の stream。replay されるまで停止。`realtime_blocked_streams` で数える。 |
| **ticket** | 接続時に提示する opaque な 256-bit credential。hash で保存し、subject / destination / scope / expiry に bind し、5 分の TTL の間は再利用可。 |
| **connection maximum lifetime** | 確立済み接続の 1 時間の上限。到達で `REAUTHENTICATE` を送る。ticket TTL とは独立。 |
| **grant** | 検証を通った ticket が接続に与える束縛（`boundary/realtime.StreamGrant`）: subject、destination、scope、initial cursor。ticket は credential、grant はそれを検証して得られるもので、request の context に載せて stream handler へ渡す。 |
| **revocation seam** | この service 内で subject が destination へのアクセスを失ったときに feature が呼ぶ呼び出し（`usecase/realtime.AccessRevoker`）。先に ticket を無効化し、次に fan-out（`boundary/realtime.RevocationNotifier`）経由で接続を閉じる。 |
| **control event** | `event: control`、`id` なし。action は `RECONNECT` / `RETRY_LATER` / `REAUTHENTICATE` / `RESYNC` / `STOP`。 |
| **instance lease** | serve instance の生存記録（heartbeat 30 s、expiry 2 分）。crash 後に queue と subscription を回収するためだけに使う。lock でも leader election でもない。 |
| **orphan** | instance lease が safety margin を超えて失効した queue / subscription。 |
| **capacity protection** | instance ごとの connection と並行 replay 読み出しの上限。commit 前に `503` で強制する。edge の関心である rate limiting とは別物。 |
