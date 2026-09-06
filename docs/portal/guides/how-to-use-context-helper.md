# ctxhelper

ctxhelper is a "boundary layer that controls the usage of context".

This package provides helper functions for carrying request-scoped values on `context.Context`.

## Implementation Method

Simple value keys are created through code generation. For details on the generation mechanism, refer to the following:

- `scripts/genctxkey/README.md`

The `Authn` helpers (`authn.go`) are hand-written: since the OpenAPI `AuthenticationFunc` cannot propagate context forward, `Authn` is carried through a mutable slot installed by the middleware before authentication.

## Provided helpers

Hand-written (`authn.go`) — the `Authn` slot:

- `WithAuthn(ctx) context.Context` — install an empty `Authn` slot (call before authentication)
- `SetAuthn(ctx, authn) bool` — write into the slot; returns `false` when no slot is present
- `GetAuthn(ctx) (auth.Authn, bool)` — read from the slot; `ok=false` when unset
- `RequireAuthn(ctx) (auth.Authn, error)` — read from the slot, returning `ErrUnauthenticatedUser` when unset
- `RequireUserID(ctx) (uuid.UUID, error)` — read the resolved internal user ID out of the slot's `Authn`
- `SetAuthnFailure(ctx, err) bool` — record an authentication failure; returns `false` when no slot is present
- `AuthnFailure(ctx) error` — read the recorded failure; `nil` when authentication did not fail

Hand-written (`stream_grant.go`) — the `StreamGrant` slot, filled by the `StreamTicket` security scheme (ADR-0074 (query-ticket-stream-authentication)) with the verified ticket's bindings. A rejected ticket is recorded through `SetAuthnFailure`, so the same fail-closed stage denies it:

- `WithStreamGrant(ctx) context.Context` — install an empty slot (call before authentication)
- `SetStreamGrant(ctx, grant) bool` — write the verified `realtime.StreamGrant`; `false` when no slot is present
- `GetStreamGrant(ctx) (realtime.StreamGrant, bool)` — read; `ok=false` when unset
- `RequireStreamGrant(ctx) (realtime.StreamGrant, error)` — read, returning `ErrStreamGrantMissing` when unset
- `SetStreamRevalidator(ctx, fn) bool` / `GetStreamRevalidator(ctx) (fn, bool)` — the same slot also carries a way to verify the *same* ticket again. Establishing a stream connection runs ticket verification, then external I/O, then registration, and a revocation arriving in between misses a connection that is not indexed yet; re-verifying once after registration closes that window. The credential itself stays inside the security scheme — what travels is the closure

The slot carries failures as well as successes because a spec may declare authentication
optional, and validation of such an operation succeeds even when a presented credential was
rejected. The failure survives in the slot, so a later stage can still deny the request.

Generated (`genctxkey`, defined in `generate.go`) — boolean request-scoped flags. Each name exposes a `context.Context` pair plus an `*echo.Context` pair:

- `ErrorHandled` — `SetErrorHandled` / `GetErrorHandled`, `SetErrorHandledToEcho` / `GetErrorHandledFromEcho`
- `Recovered` — `SetRecovered` / `GetRecovered`, `SetRecoveredToEcho` / `GetRecoveredFromEcho`

## Usage

When adding a ctxkey, add a definition to `generate.go` as follows.

```go
//go:generate go run ../../../scripts/genctxkey --name UserID --type string --out .
```

When using external types:

```go
//go:generate go run ../../../scripts/genctxkey --name Actor --type "auth.Authn" --import go-boilerplate/internal/usecase/boundary/auth --out .
```

The example above only illustrates the external-type syntax. The real `Authn` slot in this package is hand-written (`authn.go`) and is **not** produced by this command.

Then execute the following.

```bash
make gen-go-code
```

## How to specify type

### Primitive types / same-package types

```bash
--type string
--type UserID
```

- import is not required
- handled directly as a type

### Types from external packages

```bash
--type "auth.Authn"
--import go-boilerplate/internal/usecase/boundary/auth
```

- `--type` is specified in Go type format
- package is explicitly specified with `--import`
- `--alias` is optional

### Complex types

```bash
--type "*[]auth.Authn"
--type "map[string]auth.Authn"
```

- supports pointer / slice / map / generic

## Notes

- specifying only import path (e.g., `github.com/foo/bar`) is not allowed
- external types must always specify both `--type` and `--import`

## Test Strategy

The package is neither an inbound adapter nor a middleware, so nothing in [`../README.md`](../README.md) applies; the viewpoints here are its own.

- **Three slot states, both sides** — every accessor is asserted with no slot installed, a slot installed but unset, and a slot written: `With*` seeds an *empty* slot (the immediate `Get*` must report unset), `Set*` reports `false` without a slot and `true` with one, `Get*` / `Require*` distinguish unset from written. The unset-after-install case is the one that matters: a slot that reads as set with a zero value is how an unauthenticated request would pass as authenticated.
- **Failure carried beside success** — for the `Authn` slot, a recorded failure survives a later read; assert it independently of the success path.
- **Generated flags** — the `genctxkey` outputs are pinned by their generated tests; do not duplicate them here.

## About editing

Files with `.gen.go` in this directory are automatically generated code.

- manual editing is prohibited in principle
- make changes through `scripts/genctxkey`

Hand-written helpers (such as `authn.go`) are edited directly.
