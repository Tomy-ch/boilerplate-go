## `testkit` パッケージ

概要: **RDB を利用するテストのためのユーティリティを提供するパッケージです。**

主に次の用途で利用します。

- テスト用の `DatabaseDriver` を簡単に生成する
- トランザクション内でテストを実行し、必ずロールバックする

このパッケージは **Repository や Infrastructure のテストを簡単に書くための補助ツール**です。

## 目的

RDB を利用するテストでは次の問題が発生します。

- DB 初期化コードが毎回長くなる
- トランザクション管理が煩雑になる
- テスト後のデータクリーンアップが必要

`testkit` はこれらを解決するために次の機能を提供します。

- テスト用 DB 初期化
- 自動ロールバックトランザクション

## アーキテクチャ上の位置

```mermaid
flowchart TD
    A[Repository Test]
    B[testkit]
    C[driver]
    D[(PostgreSQL)]

    A --> B --> C --> D
```

`testkit` は **テスト専用の Infrastructure ヘルパー層**です。

## 提供 API

### NewTestDB

```go
func NewTestDB(t *testing.T) driver.DatabaseDriver
```

テスト用の `DatabaseDriver`（共有シングルトン）を生成します。Repository / QueryService の
コンストラクタへ直接渡してください。SQL のログ / トレースは driver の接続層で付与されます。

### NewTestTransactionRunner

```go
func NewTestTransactionRunner(t *testing.T) TransactionRunner
```

テスト用トランザクションマネージャーを生成します。

内部では

```mermaid
flowchart TD
    A[config.MockConfigForTest]
    B[driver.NewTransactionManager]

    A --> B
```

を利用します。

### HoldSuiteSerialization

```go
func HoldSuiteSerialization(t *testing.T, db driver.DatabaseDriver)
```

呼び出したテストが終わるまで、スイート全体の直列化を占有します。占有している間、他パッケージの
テスト（別プロセス）が張る `CASCADE TRUNCATE` は走りません。解放は `t.Cleanup` で行われます。

占有は専用のトランザクションで行い、テスト自身のトランザクションはこの直列化に**参加しません**。
参加させると自分の占有を待つことになり、進みません。

通常のテストは `WithinTx` が同じ直列化を内部で行うため、これを呼ぶ必要はありません。呼ぶのは、
トランザクションを 2 本同時に生かす検証（ロック競合の再現など）のように、`WithinTx` の
「1 本 + ロールバック」では表現できないテストに限られます。占有はシードする行の作成を含めて
テスト全体を覆ってください。一部しか守らないと、シードした行が競合の手前で truncate される窓が残ります。

## トランザクション実行

### TransactionRunner

```go
type TransactionRunner interface {
    WithinTx(fn func(ctx context.Context))
    WithinTxE(fn func(ctx context.Context) error)
}
```

### WithinTx

```go
func (t *testTxRunner) WithinTx(fn func(ctx context.Context))
```

指定された関数を **トランザクション内で実行**します。

処理の流れ

```mermaid
flowchart TD
    A[Transaction Begin]
    B["fn(ctx) 実行"]
    C[errRollbackForTest を返す]
    D[Rollback]

    A --> B --> C --> D
```

内部では `tx.Manager.Do` を利用しています。

```mermaid
flowchart TD
    A["Do(fn)"]
    B[error を返すことで rollback]

    A --> B
```

### WithinTxE

```go
func (t *testTxRunner) WithinTxE(fn func(ctx context.Context) error)
```

`WithinTx` と同じですが、`fn` がエラーを返せます。これで得られるのは **リトライ** です。
`pgerror.IsRetryableTxError` が宣言しているとおり、deadlock（`40P01`）と serialization failure
（`40001`）はトランザクションマネージャーが tx ごと再試行します。

`WithinTx` はそのリトライへ到達できません。`fn` の中で `require` が失敗すると `FailNow` が
`runtime.Goexit` で巻き戻すため、トランザクションマネージャーは戻り値を受け取れず、
リトライ可能かどうかを評価する機会がありません。リポジトリが復帰可能と宣言済みの一時障害が、
恒久的なテスト失敗として露出することになります。

失敗が契約違反ではなくインフラ由来の雑音である文に使ってください。`CASCADE TRUNCATE` が該当します
（依存表ごと `ACCESS EXCLUSIVE` を取るため、スイート直列化の外側で走るトランザクションと
deadlock しうる）。

```go
txm.WithinTxE(func(ctx context.Context) error {
    if _, err := driver.New(ctx, testDB).Exec(ctx, "TRUNCATE some_table CASCADE"); err != nil {
        return err // ここでの 40P01 は再試行される。テストは赤くならない
    }

    actual, err := repo.FindAll(ctx)
    require.NoError(t, err)
    assert.Empty(t, actual)

    return nil
})
```

`nil` を返してもトランザクションはロールバックされます。戻り値が運ぶのは commit の意思ではなく
失敗です。通常の検証は `require` / `assert` のままにし、再試行させたい文だけを戻り値へ回してください。

## ロールバックの仕組み

`WithinTx` は次の仕組みでロールバックを実現しています。

```mermaid
flowchart TD
    A[fn 実行]
    B[errRollbackForTest を返す]
    C[tx.Manager が rollback]

    A --> B --> C
```

この特殊エラーは

```go
var errRollbackForTest = xerrors.New("rollback for test")
```

として定義されています。

このエラーは **テストでは成功扱い**として扱われます。

## 並列実行

Repository テストは `t.Parallel()` を使用して並列実行できます。

ただし、トランザクションは内部で直列化されます。

```mermaid
flowchart LR
    A[テスト実行] -->|並列| B
    C[トランザクション] -->|直列| D
```

これは次の実装によって保証されています。

```go
txLock sync.Mutex
```

```go
txLock.Lock()
defer txLock.Unlock()
```

これにより

- DB状態の競合を防止
- テスト間の干渉を防止

が保証されます。

## DB インスタンス

テスト用 DB はシングルトンで管理されます。

```go
var (
    testDB driver.DatabaseDriver
    dbOnce sync.Once
)
```

**プロセス内で1つのDBインスタンスを共有** することで

- 接続コスト削減
- テスト高速化

が実現されています。

## 使用例

### トランザクションを利用したテスト

```go
txm := testkit.NewTestTransactionRunner(t)

txm.WithinTx(func(ctx context.Context) {
    repo.Create(ctx, ...)
})
```

### Repository テスト

```go
db := testkit.NewTestDB(t)

repo := repository.NewRepository(db)
```

## テスト設計ポリシー

`testkit` を使うことで次の設計が可能になります。

```mermaid
flowchart TD
    A[Repository Test]
    B[Real DB]
    C[Transaction Rollback]

    A --> B --> C
```

つまり

```mermaid
flowchart TD
    A[実DBを使う]
    B[テスト後に状態を戻す]

    A --> B
```

という **安全な Integration Test** を実現できます。

## 注意点

### 実際の DB に接続する

このパッケージは **実際の PostgreSQL に接続します。**

そのため

- テスト用 DB
- CI 用 DB

の設定が必要です。

### WithinTx の設計

`WithinTx` は fn の戻り値を受け取りません。

```go
fn(ctx)
return errRollbackForTest
```

そのためテスト内の検証は

```go
require / assert
```

を使用してください。

この選択はリトライを失わせます。`require` は `runtime.Goexit` で巻き戻すため、リトライ可能な
DB エラーがトランザクションマネージャーへ届きません。テストを落とすのではなく再試行させたい文には
`WithinTxE` を使い、エラーを返してください。

### トランザクションは必ずロールバックされる

`WithinTx` を使用した場合、トランザクションは **必ず rollback されます。**

そのため、次のようなテストには使用できません。

```txt
永続データを残すテスト
```
