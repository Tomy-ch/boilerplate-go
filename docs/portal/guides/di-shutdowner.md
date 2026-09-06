# shutdowner DI Wrapper

This package provides a `Shutdowner` interface that abstracts `fx.Shutdowner` from `go.uber.org/fx`. It wraps the `fx.Shutdowner` obtained from the DI container to make it easier to use from application code and tests.

## Why Abstract?

- Using `fx.Shutdowner` directly couples application code to the fx framework
- The `Shutdowner` interface allows easy mock injection in tests
- Keeps the fx dependency confined to the DI layer

## Notes

- The wrapper is extremely thin — it simply holds `fx.Shutdowner` and delegates `Shutdown` calls
- `Shutdown` triggers process stop and cleanup, so callers should be aware of side effects
- `mock/` contains auto-generated mocks via `mockgen`
