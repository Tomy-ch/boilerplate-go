# system

`internal/infrastructure/system` は、時刻取得やコンテキスト対応の待機などの **システム依存処理の Infrastructure 実装**を提供するパッケージです。

`internal/usecase/boundary/clock` の 2 つのインターフェースを実装します。

- `clock.Clock`（`NewClock`）— `Now()` は現在時刻を返す
- `clock.Sleeper`（`NewSleeper`）— `Sleep(ctx, d)` は `d` 経過まで待機し、先に context がキャンセルされた場合は `ctx.Err()` を返す（`d` が非正の場合は即座に `ctx.Err()` を返す）

いずれも同一の非公開型 `systemClock` が実体です。

## アーキテクチャ上の位置づけ

```mermaid
flowchart TB
    subgraph "Usecase 層"
        IF["clock.Clock / clock.Sleeper interface"]
    end
    subgraph "Infrastructure 層"
        Impl["systemClock 実装 (Clock + Sleeper)"]
    end
    subgraph "Domain 層"
        Domain["Domain Entity"]
    end

    Impl -. implements .-> IF
    Domain -. uses .-> IF
```

Domain / Usecase が `time.Now()` を直接呼ぶと、テストで時刻を制御できなくなります。`clock.Clock` インターフェース（`internal/usecase/boundary/clock`）を介することで、テスト時にモック差し替えが可能になります。

## なぜ抽象化するのか

- Domain / Usecase の **決定論性（determinism）** を守る — テストで時刻を固定できる
- オニオンアーキテクチャの原則に従い、**システム依存を外側に押し出す**
- `time.Now()` への直接依存は Domain 層で禁止されている

## DI 登録

`internal/di/module/clock.go` の `clockModule()` で登録します（`InfrastructureModule()` に集約）。`Clock` と `Sleeper` の両実装をここで提供します。

```go
fx.Provide(
    system.NewClock,
    system.NewSleeper,
)
```

## Test Strategy

ここに DB は無いため、infrastructure 層の実 DB 戦略は適用されません。またこのパッケージは、リポジトリの中で実時刻を読むことが正当な唯一の場所です。他のコードが `time.Now()` を直接呼ぶ代わりに注入する実装が、まさにこれだからです。その代わりに注入できるものはもう残っていません。「テストを実時間に依存させない」という規則は、満たせる場所すべてで成り立ちます。ここでは満たせないため、戦略は「実時間が無いふりをする」ことではなく「露出を小さく、境界づけて保つ」ことです。

- **実時間は使う。ただし契約を示せる最小量に留める。** 待機は数十ミリ秒で測り、下限（`elapsed >= d`）で assert します。上限では assert しません。上限は、スケジューラのゆらぎと負荷のかかった CI マシンを、そのまま赤いビルドに変えてしまいます。`Now()` を `time.Now()` と比較する窓を広く取っているのも同じ理由です。
- **キャンセルは待たずに検証する。** context は呼び出しの **前に** キャンセルします。これにより「キャンセルが待機時間に優先する」ことを固定するケースが、渡した duration を待たずに即座に完了します。
- **非正の待機時間は edge case ではなく分岐である。** `d <= 0` は即座に返りますが、それでもキャンセル済み context を報告せねばなりません。さもなくば backoff が 0 のとき、キャンセルが無言で無視されます。この経路はキャンセル済みと生存中の両方の context で固定します。

これらの interface の利用側は逆を行います。`clock` testkit の代替物を注入し、制御されたタイムライン上で assert します。この非対称こそが抽象化の目的であり、実時間への露出がこのパッケージで止まる理由です。

## 拡張する場合

時刻以外のシステム依存処理（乱数生成、ホスト名取得等）を追加する場合：

1. `internal/usecase/boundary/` に interface を定義
2. このパッケージに実装を配置
3. `internal/di/module/infrastructure.go` に DI 登録を追加
