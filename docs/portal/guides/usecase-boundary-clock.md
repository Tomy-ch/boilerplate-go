# clock

Provides a `Clock` interface for retrieving the current time and a `Sleeper` interface for waiting.

```go
type Clock interface {
    Now() time.Time
}

type Sleeper interface {
    Sleep(ctx context.Context, d time.Duration) error
}
```

## Why Abstract?

Direct `time.Now()` access from Domain / Usecase is prohibited; see
[Time Handling Policy](../../README.md#time-handling-policy) for the rationale.

- `Sleeper` makes backoff waits injectable so retries can be tested without real sleeping. Consumers: the resilient HTTP client (`internal/infrastructure/httpclient`) and the transaction manager's retry (`internal/infrastructure/rdb/driver`).

## Implementation

`internal/infrastructure/system/` provides the concrete implementations that call `time.Now()` / `time.NewTimer` (ctx-aware).
