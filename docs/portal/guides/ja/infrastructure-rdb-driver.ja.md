# driver

概要: **RDB（PostgreSQL / pgx）接続のための基盤ドライバレイヤー。接続管理・トランザクション境界・sqlc 実行アダプタを提供します。**

このパッケージは **Infrastructure 層の最下位に位置する DB アクセス基盤**です。

Repository 層はこの driver を通して DB にアクセスします。

## アーキテクチャ上の位置

```mermaid
flowchart TB
    Usecase --> Repo["Repository"] --> Driver["Driver (このパッケージ)"] --> DB["PostgreSQL"]
```

Driver は **RDB 接続の最下層アダプタ**です。

## 役割

このディレクトリは次の機能を提供します。

- `pgxpool.Pool` をラップした **DatabaseDriver 抽象化**
- **トランザクション管理 (`tx.Manager`)**
- **pgx ベースの DBTX インターフェース提供（sqlc 互換）**
- **接続プール設定**
- **DB 起動時の疎通確認 (fail fast)**

これにより Repository 層は

```mermaid
flowchart TB
    A["pgxpool.Pool"]
    B["pgx.Tx"]
```

のどちらでも同じクエリコードを実行できます。

## DB 初期化

DB 接続を初期化するコンストラクタは 2 つあります。

```go
func NewDB(...) (DatabaseDriver, error)                         // クエリトレーサーなし
func NewTracedDB(..., tracer pgx.QueryTracer) (DatabaseDriver, error) // クエリトレーサーあり
```

`NewTracedDB` はアプリ本体（DI）が利用し、pgx クエリトレーサーを `poolCfg.ConnConfig.Tracer`
に結線します。`NewDB`（トレーサーなし）は、クエリ計装が不要なツール経路（マイグレーション /
シード等）のために残しています。

処理内容:

1. `pgxpool.NewWithConfig()` による接続初期化
2. 接続プール設定
    - MaxConns
    - MinConns
    - ConnMaxLifetime
    - ConnMaxIdleTime
3. クエリトレーサーを `ConnConfig.Tracer` に結線（指定時のみ）
4. `Ping` による DB 疎通確認

Ping に失敗した場合は **起動時にエラーを返す (fail fast)** 設計です。

`DB_PING_TIMEOUT` は `Ping` 単体ではなく、この一連の処理全体に対する予算です（プール生成と `Ping` が同一の
context を共有します）。さらに `pgxpool` はプール生成の直後に `MinConns` 本の接続を並行して張るため、その
確立も同じ締切を `Ping` と奪い合います。1 つのデータベースに対して多数のプロセスが同時にプールを作る状況
（並列 `go test`、レプリカ群の同時コールドスタート、フェイルオーバー後の再接続）では、データベースが実際に
到達不能なのではなく順番待ちで予算を使い切ることがあります。この形の負荷では失敗を「DB が落ちている」と
読まず、`DBCONN_MIN_CONNS` / `DB_PING_TIMEOUT` を環境ごとに調整してください（`env/README.md` 参照）。

## DatabaseDriver

`DatabaseDriver` は `pgxpool.Pool` を抽象化したインターフェースです。

```go
 type DatabaseDriver interface {
     DBTX

     Begin(ctx context.Context) (pgx.Tx, error)
     Ping(ctx context.Context) error
     Close() error
     Stats() *pgxpool.Stat
 }
```

目的:

- `pgxpool.Pool` への直接依存を避ける
- テスト時に mock 化を可能にする
- トランザクション開始を抽象化する

実装は `dbDriver` が提供します。

## DBTX

`DBTX` は **sqlc が要求する最小インターフェース**です。

```go
 type DBTX interface {
     Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
     Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
     QueryRow(ctx context.Context, query string, args ...any) pgx.Row
 }
```

このインターフェースにより sqlc は

```mermaid
flowchart TB
    A["pgxpool.Pool"]
    B["pgx.Tx"]
```

のどちらでも同じクエリコードを実行できます。

## トランザクション透過レイヤ

`connection.go` の `New()` は **トランザクション透過アダプタ**です。

```go
func New(ctx context.Context, db DatabaseDriver) DBTX
```

挙動:

```mermaid
flowchart TB
    HasTx["context に Tx がある"] --> ReturnTx["pgx.Tx を返す"]
    NoTx["Tx がない"] --> ReturnDB["pgxpool.Pool（DatabaseDriver）を返す"]
```

これにより Repository 層は`DB`と`Tx`の違いを意識せずクエリを実行できます。

## トランザクション管理

`tx.Manager` は Usecase 層のトランザクション境界を提供します。

```go
err := tx.Do(ctx, func(ctx context.Context) error {
    ...
})
```

内部では次の処理を行います。

1. context に Tx が存在するか確認
2. 存在すれば **既存 Tx を再利用**
3. 存在しなければ **新規 Tx を開始**
4. fn 実行
5. 成功 → commit
6. error → rollback

- pgx.Tx を利用したトランザクション管理です。

これにより **ネストトランザクションを安全に扱うことができます。**

## 注意点

### トランザクション cleanup タイムアウト

rollback / commit 実行時は、リクエストの `context` がキャンセルされていても cleanup を必ず試行する必要があります。

そのため、以下のパターンを採用しています。

```go
context.WithTimeout(
    context.WithoutCancel(ctx),
    cleanupTimeout,
)
```

#### なぜ `context.WithoutCancel(ctx)` を使うのか

cleanup は **リクエストライフサイクルに依存してはいけません**。

- リクエストがキャンセル（timeout / client 切断）された場合、元の `ctx` をそのまま使うと次が起きます:
  - rollback / commit がキャンセルされる
  - トランザクションが開いたまま残る
  - connection が pool へ返らない

`context.WithoutCancel(ctx)` を使うことで:

- cleanup は必ず実行される
- trace / logger / correlation ID は維持される

> cleanup は「成功させること」ではなく「安全に試みること」が重要です。

#### cleanupTimeout について

- rollback / commit に許可する最大時間
- 現在は `5秒` に固定

この値は **ビジネス設定ではなくインフラ保護のためのセーフティ値** です。

- 長すぎると:
  - goroutine 詰まり
  - connection pool 枯渇
- 短すぎると:
  - cleanup 未完了

そのため、環境変数ではなく driver 内の定数として管理しています。

### Context を必ず伝搬する

トランザクションは`context.Context`に格納されます。そのため`ctx`を必ず下位レイヤに伝搬してください。

### Repository は driver.New() を使用する

Repository 層では

```go
driver.New(ctx, db)
```

を利用して `DBTX` を取得します。

これにより`Tx`と`DB`を透過的に切り替えることができます。

## DSN ヘルパー（config.go）

DB 接続用の DSN を組み立てるユーティリティです。

|関数|説明|
|---|---|
|`DSN(dbCfg)`|基本の接続 URL を生成|
|`DSNWithTimeZone(dbCfg, osCfg)`|タイムゾーン付き接続 URL を生成|
|`DSNString(dbCfg)`|`DSN` の文字列版|
|`DSNStringWithoutPassword(dbCfg)`|パスワードを含まない `DSN` の文字列版（パスワードは `PGPASSWORD` などで別途渡す）|
|`DSNWithTimeZoneString(dbCfg, osCfg)`|`DSNWithTimeZone` の文字列版|

## NewTransactionManager

```go
func NewTransactionManager(db DatabaseDriver, dbCfg *config.DatabaseConfig, logger logging.Logger, sleeper clock.Sleeper) tx.Manager
```

Usecase 層の `tx.Manager`（`internal/usecase/boundary/tx`）を実装するコンストラクタです。
`Do` は `serialization_failure`(40001) / `deadlock_detected`(40P01) を検出するとトランザクション全体を
有限回まで再試行します（`sleeper` で指数 backoff + full jitter, `pkg/retry`）。再試行回数と backoff は
config（`DB_TX_MAX_RETRIES` / `DB_TX_RETRY_BASE_BACKOFF` / `DB_TX_RETRY_MAX_BACKOFF`）から取得し、
0 以下の場合は組み込み既定値にフォールバックします。`fn` の冪等性契約は `tx` 境界 README を参照してください。

## クエリトレーサー（query_tracer.go）

`NewQueryTracer` は `ConnConfig.Tracer` に結線する `pgx.QueryTracer` を生成します。OpenTelemetry
span のために `otelpgx` を埋め込み、クエリログ（正常終了 Info / スロー Warn / 失敗 Error）を上乗せします。

|型 / 関数|説明|
|---|---|
|`NewQueryTracer`|`pgx.QueryTracer` を生成（DB / Observability 設定、otelpgx トレーサー、`QueryRecorder`、Logger、LogFieldBuilder を受け取る）|
|`queryTracer`|`*otelpgx.Tracer` を埋め込み、`TraceQueryStart` / `TraceQueryEnd` を上書きしてログとクエリメトリクスを付加|

特徴：

- `otelpgx` によるクエリごとの OpenTelemetry span（semconv の DB 属性付き、batch / copy も対象）
- 正常終了時の**Info ログ**（latency 付き）
- クエリ失敗時の**エラーログ**（`span.RecordError` に加えて）
- `DB_SLOW_QUERY_WARN_THRESHOLD` 超過時の**スロークエリ Warn ログ**
- `OBS_MASKED_DB_QUERY_ARGS` によるクエリ引数のマスキング
- `TraceQueryEnd` ごとに、注入された `QueryRecorder`（実装は `metrics` パッケージ）で**クエリメトリクス**を記録

## クエリメトリクス（query_metric.go）

`TraceQueryEnd` は `QueryRecorder` を通じて DB クエリの duration / error を記録します。interface と
値 `QueryAttrs` は消費側である本パッケージに置き、`metrics` パッケージが循環 import なしに実装できる
ようにしています（`metrics` は既に `driver` を import しているため）。

|型 / 関数|説明|
|---|---|
|`QueryRecorder`|クエリ終了ごとに、組み立てた `QueryAttrs` を 1 回受け取る interface|
|`QueryAttrs`|低カーディナリティな観測属性（query name / operation / status / error class / duration）。SQL 本文・bind 値・PII は持たない|
|`WithQueryName(ctx, name)`|メトリクスラベル用の安定名（例: `"user.find_by_id"`）を付与|

属性の導出方法：

- `query_name`: `WithQueryName` から取得。未設定 / 空文字は `unknown`
- `operation`: SQL の先頭トークンのみから分類 → `select` / `insert` / `update` / `delete` / `begin` / `commit` / `rollback` / `copy` / `other`（先頭コメントや `WITH` 句は `select` / `other` に丸める）
- `status`: `success` / `error`。`pgx.ErrNoRows` は `success` 扱いで error には数えない
- `error_class`: `pgerror` を用いて導出 → `constraint` / `timeout` / `retryable` / `connection` / `unknown`（`retryable` は `serialization_failure` (40001) / `deadlock_detected` (40P01)、すなわちリトライ可能なトランザクション競合）。`pgx.ErrNoRows` は `success` 扱いのため、ここには現れない

Prometheus メトリクス定義（`rdb_query_duration_seconds` / `rdb_query_errors_total`）は
`internal/infrastructure/rdb/metrics` にあります。Repository / QueryService は
`driver.WithQueryName(ctx, "...")` で query name を設定するだけで、その他は透過的に計装されます。
