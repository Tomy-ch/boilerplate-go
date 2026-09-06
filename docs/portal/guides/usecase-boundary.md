# boundary

`internal/usecase/boundary` defines the **interfaces that the Usecase layer requires from external layers (Infrastructure)**.

## Position in Onion Architecture

```mermaid
flowchart TB
    subgraph "Usecase Layer"
        Boundary["boundary (interface definitions)"]
        UC["Usecase impl"]
    end
    subgraph "Infrastructure Layer"
        Impl["Concrete impl"]
    end

    UC --> Boundary
    Impl -. implements .-> Boundary
```

boundary is the mechanism for achieving the **Dependency Inversion Principle (DIP)**.

- Usecase depends only on boundary interfaces
- Infrastructure provides concrete implementations
- Usecase has no knowledge of Infrastructure implementation details

### Difference from Domain Repository Interface

|Aspect|Domain Repository|Usecase Boundary|
|---|---|---|
|Definition location|Domain layer|Usecase layer|
|Purpose|Aggregate persistence abstraction|Abstraction of external capabilities needed by Usecase|
|Scope|Persistence (CRUD)|Auth / encryption / time / transaction / job, etc.|

Domain Repository abstracts "how to persist Aggregates", while Usecase Boundary abstracts "external capabilities Usecase needs to execute business flows".

## Package List

|Package|Interface|Description|Implementation|
|---|---|---|---|
|`address`|`Gateway`|Semantic gateway to an external postal-address lookup service (sample of the `<service>.Gateway` pattern)|`internal/infrastructure/webapi/address/`<!-- sample-api:line -->|
|`auth`|`Authenticator`|Obtain auth info (`Authn`) from token|`internal/infrastructure/auth/`|
|`authz`|`Authorizer`|Decide whether a subject may perform an action on a resource|`internal/infrastructure/authz/`|
|`clock`|`Clock`|Retrieve current time|`internal/infrastructure/system/`|
|`exchangerate`|`Gateway`|Semantic gateway to an external exchange-rate service (sample of the `<service>.Gateway` pattern)|`internal/infrastructure/webapi/exchangerate/`<!-- sample-api:line -->|
|`idempotency`|`Store`|Idempotency-key persistence boundary (claim / replay / conflict)|`internal/infrastructure/rdb/system_cqrs/idempotency/`|
|`job`|`Job`, `Runner`, `State`|Job definition, execution, state management|`internal/controller/job/`|
|`objectstorage`|`Storage`|Substrate-agnostic object-storage boundary (`Put` / `List` / `Delete` objects by key)|`internal/infrastructure/objectstorage/s3/`|
|`outbox`|`Store`|Transactional outbox table persistence boundary|`internal/infrastructure/rdb/system_cqrs/outbox/`|
|`publisher`|`Publisher`|Substrate-agnostic outbound message publish boundary|`internal/infrastructure/publisher/`|
|`realtime`|`SequenceAllocator`, `EventLogStore`, `StreamTicketStore`, `InstanceLeaseStore`, `SecretGenerator`, `RevocationNotifier`|Realtime Delivery seam: the feature-neutral `DeliveryEvent` envelope plus the replay store, ticket store, instance lease, ticket-secret and revocation-notification boundaries (see the package README)|`internal/infrastructure/{eventlog,streamticket,instancelease}/dynamodb/`, `internal/infrastructure/realtimesecret/`, the SNS fan-out publisher (Phase 7)|
|`token`|`Generator`|Generate unguessable opaque token strings|`internal/infrastructure/token/`<!-- sample-api:line -->|
|`tx`|`Manager`|Transaction boundary management|`internal/infrastructure/rdb/driver/`|
|`worker`|`Consumer`, `Handler`, `FailureHandler`, `Worker`, `State`|Broker-agnostic worker seam (pull-ack)|`internal/infrastructure/queue/sqs/`|

## Package Details

### auth

Provides interfaces and value objects for authentication.

|Type / Function|Description|
|---|---|
|`Authenticator`|Interface to generate `Authn` from `Credential`|
|`Authn`|Authentication result (subject / userID / issuer / scopes / claims)|
|`New(subject, issuer, scopes, claims)`|Create `Authn` with the UserID unresolved (empty subject returns `ErrUnauthenticatedSubjectMissing`)|
|`WithUserID(userID)`|Return a copy of `Authn` with the internal UserID resolved (a zero-value UUID returns `ErrUserIDZero`)|
|`Credential`|Value object holding the auth scheme + token|
|`NewCredential(scheme, token)`|Create `Credential` (empty token returns `ErrTokenMissing`)|
|`IdentityResolver`|Interface to resolve an authenticated external identity (issuer + subject) to an internal user; applied after authentication succeeds|

Errors:

|Error|Description|
|---|---|
|`ErrUnauthenticatedSubjectMissing`|Subject is empty|
|`ErrUserIDUnresolved`|Internal UserID is unresolved|
|`ErrUserIDZero`|`WithUserID()` was given a zero-value UUID|
|`ErrTokenMissing`|Token is empty|
|`ErrIdentityNotFound`|No internal user corresponds to the authenticated identity|
|`ErrUserUnavailable`|The corresponding internal user exists but is unusable (e.g. deleted)|

### authz

Provides the interface and value objects for authorization — the counterpart to `auth`. The Usecase layer is the enforcement point (PEP): it calls `Authorize(...)` and maps a deny to `apperror.ErrPermissionDenied` (403).

|Type / Function|Description|
|---|---|
|`Authorizer`|Interface deciding whether `authn` may perform `action` on `resource` (`Authorize(ctx, *auth.Authn, Action, *Resource) error`)|
|`Action`|Operation being authorized (e.g. `ActionUserDelete` = `"user:delete"`)|
|`Resource`|Target resource carrying `Kind()` and optional `OwnerID()`, so ownership-based (object-level) decisions are expressible|
|`NewResource(kind, ownerID)`|Create a `Resource`|

Errors:

|Error|Description|
|---|---|
|`ErrForbidden`|Authorization denied (wraps `apperror.ErrPermissionDenied`, HTTP 403)|

Passing the full `auth.Authn` (subject / scopes / claims) plus the target `Resource` lets both RBAC (roles from claims) and ownership (subject == OwnerID) models be expressed. The default implementation is allow-all and restricted to non-production environments.

Passing `ownerID = nil` to `NewResource` declares an **ownerless** resource — the caller is stating that no ownership claim applies, not that the owner is unknown. What an `Authorizer` makes of that is its own policy, but no ownership comparison can succeed against such a resource, so any ownership-based rule can only narrow access, never widen it. Omitting the owner is therefore the safe direction to fail in.

### clock

```go
type Clock interface {
    Now() time.Time
}

type Sleeper interface {
    Sleep(ctx context.Context, d time.Duration) error
}
```

Abstraction to prevent Domain / Usecase from depending directly on `time.Now()`. Allows mock substitution in tests. `Sleeper` does the same for waiting, so backoff and retry can be exercised without real sleeping.

<!-- sample-api:begin -->
### exchangerate

Sample Gateway boundary: a semantic port to an external exchange-rate service (`<service>.Gateway` pattern). Keeps Usecase depending on a semantic port rather than `net/http` or a vendor SDK, and translates transport failures into `apperror` sentinels at the boundary.

|Type / Function|Description|
|---|---|
|`Gateway`|Fetch a conversion rate via `GetRate(ctx, base, quote)`|
|`Rate`|Output DTO (`Base` / `Quote` / `Value`)|
<!-- sample-api:end -->

### idempotency

Persistence boundary for idempotency keys. Every method requires a `scope` (no id-only lookup, to prevent cross-boundary access).

|Type / Function|Description|
|---|---|
|`Store`|Idempotency-key persistence boundary interface|
|`Claim(ctx, p)`|Create a claimed row; returns `claimed=true` when new, `false` when the key already exists (`ErrLockTimeout` on lock-wait timeout)|
|`Get(ctx, scope, key)`|Return the stored state for `(scope, key)` (nil when absent)|
|`Complete(ctx, p)`|Transition `claimed` → `completed` and store the response|
|`DeleteExpired(ctx, cutoff, limit)`|Delete rows older than `cutoff` up to `limit` (GC), returning the count|

Input / output value objects: `ClaimParams` / `CompleteParams` (inputs) and `Record` (stored state). Sentinel: `ErrLockTimeout` (mapped to 409 by the usecase).

### job

|Interface|Description|
|---|---|
|`Job`|Job definition with `Name()` + `Execute(ctx, args)`|
|`Runner`|Execute and list jobs via `Run(ctx, jobName, args)` + `Names()`|
|`State`|Manage job execution state via `Set(name, args, done)` + `Snapshot()`|

### objectstorage

Substrate-agnostic object-storage boundary. Usecase depends only on this port; the S3-compatible adapter (infrastructure) implements it, and vendor vocabulary (bucket / region / etag) never leaks across the boundary.

|Type / Function|Description|
|---|---|
|`Storage`|`Put(ctx, PutObject) (Path, error)` stores an object under its key. `List(ctx, ListQuery) (ListResult, error)` enumerates one page of matching objects. `Delete(ctx, keys []string) error` removes objects in bulk — an empty slice is a no-op, absent keys are not an error, and re-running with the same keys changes nothing. Failures return an `apperror` sentinel (e.g. `ErrUnavailable`)|
|`PutObject`|Input DTO (`Key` / `Body` / `ContentType` / `CacheControl`); the caller assigns `Key` (e.g. `<prefix>/{uuid}.png`) and decides `CacheControl`, since cacheability follows from how the caller numbers keys (empty leaves it unset)|
|`ListQuery`|Input DTO (`Prefix` / `Cursor` / `Limit`); an empty `Prefix` enumerates everything, `Cursor` is the opaque, adapter-defined boundary taken from a previous `ListResult.NextCursor`, and a `Limit` of zero or less leaves the page size to the adapter's default|
|`ListResult`|One page of objects plus `NextCursor`; a non-empty `NextCursor` means more remain and is fed back as the next `ListQuery.Cursor`|
|`Object`|A single enumerated object as the boundary describes it|
|`Path`|The stored object path (key); the display URL is composed separately by the caller|

### outbox

Persistence boundary for the transactional outbox table. The emit usecase and the relay engine (controller layer) both depend on it.

|Type / Function|Description|
|---|---|
|`Store`|Outbox table persistence boundary interface|
|`Channel`|The delivery lane a row belongs to (`ChannelHTTP` / `ChannelRealtime`); one relay process drains exactly one|
|`ParseChannel(s)`|Convert a string to a `Channel`, rejecting anything unknown (`ErrUnknownChannel`)|
|`Insert(ctx, p)`|INSERT one outbox row within the business tx, returning the assigned `message_id`|
|`ClaimPending(ctx, channel, limit)`|Claim up to `limit` of the channel's pending rows whose retry time has come (`FOR UPDATE SKIP LOCKED`), skipping any row an earlier unpublished sequence on its ordering key still blocks|
|`MarkPublished(ctx, id)`|Transition a published row to `published` (no-op unless still pending)|
|`MarkFailed(ctx, id, lastErr, nextAttemptAt)`|Record `last_error` and push the row's next claimable time out; the row stays `pending`|
|`MarkDead(ctx, id)`|Transition a row to `dead` (no-op unless still pending)|
|`ReplayDead(ctx, messageID)`|Return `dead` rows to `pending` (all dead rows when `messageID` is nil), returning the count|
|`DeletePublished(ctx, cutoff, limit)`|Delete published rows older than `cutoff` up to `limit` (GC), returning the count|
|`OldestPendingCreatedAt(ctx, channel)`|Return the channel's oldest pending row's `created_at` for the outbox-lag SLI (`ok=false` when none)|
|`CountBlockedStreams(ctx, channel)`|Count the streams whose head row is `dead`, so nothing behind them can be claimed|

Input / output value objects: `EmitParams` (INSERT input, carrying the channel and the optional ordering key / sequence) and `PendingMessage` (claimed unpublished row).

### publisher

Outbound publish boundary for domain events plus a substrate-agnostic message envelope. Both the relay engine (controller layer) and the publish adapter (infrastructure layer) depend on it.

|Type / Function|Description|
|---|---|
|`Publisher`|Boundary that sends a message to its destination|
|`Publish(ctx, m)`|Send `m` to the destination; on failure returns an error and the relay re-sends on its next poll (at-least-once)|
|`Message`|Substrate-agnostic message envelope built from an outbox row (exposes no `net/http` types)|

<!-- sample-api:begin -->
### token

```go
type Generator interface {
    Generate() (string, error)
}
```

Produces an unguessable, opaque token string. Randomness is an effect, so a caller that reaches
for it directly stops being reproducible — the same reason `clock` exists for time. The value's
length and alphabet belong to the implementation; whether a string is acceptable as a particular
kind of token is a rule of the type that holds it.
<!-- sample-api:end -->

### tx

|Type / Function|Description|
|---|---|
|`Manager`|Manage transaction boundaries via `Do(ctx, fn)`|
|`DoWithResult[T](ctx, m, fn)`|Generic helper to return a value from within a transaction|

### worker

|Type / Function|Description|
|---|---|
|`Consumer`|Broker-agnostic message intake — `Receive` / `Ack` / `Nack` / `NackWithBackoff` / `Extend` (implemented by a broker adapter)|
|`Handler`|Per-message business processing (must be idempotent)|
|`FailureHandler`|Dead-letter sink for permanent failures|
|`Worker`|Bundles Name / Consumer / Handler / FailureHandler|
|`State`|Selected-worker state shared with the engine|
|`QueueStatsProvider`|Optional queue depth / DLQ stats source for metrics|

## Design Policy

- boundary contains no business logic (only interfaces and value objects)
- Importing Infrastructure is prohibited (dependency direction violation)
- All interfaces have `mockgen` auto-generation configured
