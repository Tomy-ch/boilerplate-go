# clock

現在時刻を取得する `Clock` インターフェースと、待機のための `Sleeper` インターフェースを提供します。

```go
type Clock interface {
    Now() time.Time
}

type Sleeper interface {
    Sleep(ctx context.Context, d time.Duration) error
}
```

## なぜ抽象化するのか

Domain / Usecase から `time.Now()` へ直接アクセスすることは禁止されています。理由は
[Time Handling Policy](../../README.ja.md#time-handling-policy) を参照してください。

- `Sleeper` により backoff の待機を注入可能にし、実時間 sleep なしでリトライをテストできる。利用者: レジリエント HTTP クライアント（`internal/infrastructure/httpclient`）とトランザクションマネージャのリトライ（`internal/infrastructure/rdb/driver`）。

## 実装

`internal/infrastructure/system/` に `time.Now()` / `time.NewTimer`（ctx 対応）を呼ぶ具体実装が配置されています。
