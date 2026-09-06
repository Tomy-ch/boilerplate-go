# Infrastructure Layer (`internal/infrastructure`) Guide

## Role

The Infrastructure layer is responsible for **implementing access to external technologies (DB, external APIs, authentication, etc.)**.

This layer has the following responsibilities.

- Implementation of external I/O (RDB / API / authentication / system)
- Implementation of interfaces defined by the Domain
- Encapsulation of technical details (connection, retry, drivers, logging, etc.)
- Error normalization
- Provision of Observability (logging / tracing)

Upper layers (Domain / Usecase) **must not be aware of Infrastructure implementation details at all.**

## Position in Onion Architecture

```mermaid
flowchart TB
    Infra["Infrastructure"]
    Usecase["Usecase"]
    Domain["Domain"]

    Infra --> Usecase --> Domain
```

- Domain / Usecase define abstractions only
- Infrastructure provides concrete implementations

## Dependencies

```mermaid
flowchart LR
    Infra["Infrastructure"]
    Domain["Domain（interface）"]
    Usecase["Usecase"]

    Infra --> Domain
    Infra --> Usecase
```

- Infrastructure depends on Domain
- Domain / Usecase must not depend on Infrastructure

## Relationship with Usecase

- Transaction boundaries are managed by the Usecase layer
- Infrastructure must not start transactions
- Transactions are propagated via `context.Context`

```mermaid
flowchart TB
    UC["Usecase（Tx start）"]
    Repo["Repository / QueryService"]
    Driver["driver（use Tx）"]

    UC --> Repo --> Driver
```

## Error Handling

The Infrastructure layer must not return external technology errors as-is,  
but must **convert them into application-wide errors**.

Examples:

- PostgreSQL errors → pgerror.NormalizeError
- External API errors → converted to apperror

## Observability

The Infrastructure layer provides the following observability.

- SQL / external I/O log output
- Tracing using OpenTelemetry
- Execution time measurement (slow query)

Mainly implemented by a pgx query tracer wired at the driver connection level (`otelpgx` spans, with log output limited to query failures and slow queries).

In addition to the driver-level tracer, every I/O component (Repository / QueryService / SystemQuery / external gateways / queue / publisher) opens an application-level span per public method: it holds an `observability.LayerTracer` field initialized from `tf.Infra()` in its constructor, and each method begins with `ctx, endSpan := r.tracer.Start(ctx); defer endSpan()`. Pure in-memory components with no real I/O are exempt.

## Prohibited Practices

The following must not be done in the Infrastructure layer.

- Implementation of business logic
- Branching based on Domain rules
- Decision making of Usecase
- Introducing HTTP / framework-dependent code
- Starting transactions

## Implementation Rules

- Use sqlc for SQL execution
- Split Repository vs QueryService by what the read targets, not by whether it is a "search":
  the aggregate's system-of-record state (full aggregate reconstructable) stays in Repository, a
  derived projection / read model goes to QueryService. See
  [ADR-0032 (lightweight-cqrs)](../../docs/adr/0032-lightweight-cqrs.md)
- Acquire the DBTX via `driver.New(ctx, db)` (logging / tracing is applied at the driver connection level)
- Always propagate context
- Always normalize external errors

### Doc comments may name technical detail

Encapsulating technical detail (see *Design Principles Summary § 1*) means the outer layers must not
**see** it — not that this layer must not **document** it. A Repository / QueryService /
CommandService doc comment is read by whoever maintains the SQL, so it should name the mechanics that
carry a guarantee: the lock it takes (`FOR UPDATE OF p`), the predicate that enforces ownership, the
keyset ordering that makes pagination stable, the fixed query count that avoids N+1.

The boundary is directional — this detail stays **here**. A Usecase or Domain doc comment that
repeats it has leaked the layer; see
[`internal/usecase/README.md`](../usecase/README.md) § Doc comments: interface vs implementation.

The converse is the failure mode to watch for here. Because the inward interface states the guarantee
in application vocabulary, an implementation doc that only paraphrases that interface adds **nothing**
— it is a duplicate that rots in two places. Deleting it is not the remedy: Go does not carry an
interface's doc to its implementations, so a bare method body leaves whoever maintains the SQL with
less than the caller has, and the guarantee they are about to change is nowhere in sight.

So an implementation doc **states the guarantee through the mechanism that carries it** (`FindByID`
reads without taking a lock, unlike `LockByID`; `SearchByKeyword` dispatches to one of three fixed
queries on the `active` filter; `Update` normalizes zero affected rows to NotFound). Each of those
says what the interface says *and* how this implementation gets there, in one breath — which is why
it neither duplicates nor omits. Paraphrasing the interface and leaving the body bare are the two
options that are never right.

## Directory Structure

```mermaid
flowchart TB
    Root["internal/infrastructure"]
    Auth["auth/"]
    Authz["authz/"]
    AwsClient["awsclient/"]
    DDBClient["dynamodbclient/"]
    EventLog["eventlog/"]
    StreamTicket["streamticket/"]
    InstanceLease["instancelease/"]
    RealtimeSecret["realtimesecret/"]
    Realtime["realtime/"]
    HTTP["httpclient/"]
    ObjStorage["objectstorage/"]
    Pub["publisher/"]
    Queue["queue/"]
    RDB["rdb/"]
    Sys["system/"]
    %% sample-api:begin
    Token["token/"]
    %% sample-api:end
    Web["webapi/"]

    Root --> Auth
    Root --> Authz
    Root --> AwsClient
    Root --> DDBClient
    Root --> EventLog
    Root --> StreamTicket
    Root --> InstanceLease
    Root --> RealtimeSecret
    Root --> Realtime
    Root --> HTTP
    Root --> ObjStorage
    Root --> Pub
    Root --> Queue
    Root --> RDB
    Root --> Sys
    %% sample-api:begin
    Root --> Token
    %% sample-api:end
    Root --> Web
```

## Subdirectories

|Directory|Description|Interface Placement|Details|
|---|---|---|---|
|`auth/`|Authentication infrastructure (environment-specific Authenticator impl)|Usecase boundary|[README](auth/README.md)|
|`authz/`|Authorization infrastructure (Authorizer impl; default `allowall` for non-production)|Usecase boundary|[README](authz/README.md)|
|`awsclient/`|AWS credential resolution shared by `objectstorage/s3`, `queue/sqs` and `dynamodbclient`|— (substrate, no domain/usecase IF)|[README](awsclient/README.md)|
|`dynamodbclient/`|DynamoDB client substrate shared by the Realtime Delivery stores: endpoint override, fixed retry bound, error normalization, idempotent `EnsureTable`; `testkit/` for the contract tests|— (substrate, no domain/usecase IF)|[README](dynamodbclient/README.md)|
|`eventlog/`|Realtime Delivery EventLog adapter (DynamoDB impl of `realtime.EventLogStore`; bounded replay store)|Usecase boundary|[README](eventlog/README.md)|
|`streamticket/`|Realtime Delivery stream-ticket adapter (DynamoDB impl of `realtime.StreamTicketStore`)|Usecase boundary|[README](streamticket/README.md)|
|`instancelease/`|Realtime Delivery instance-lease adapter (DynamoDB impl of `realtime.InstanceLeaseStore`)|Usecase boundary|[README](instancelease/README.md)|
|`realtimesecret/`|Ticket-secret generation from the OS randomness source (impl of `realtime.SecretGenerator`; independent of the sample-only `token/`)|Usecase boundary|[README](realtimesecret/README.md)|
|`realtime/`|Realtime Delivery fan-out substrate (SNS / SQS impl of the `realtime` channel's `boundary.Publisher`, `realtime.RevocationNotifier` and `realtime.InstanceSubscription`; `local/` for the emulator's queue attributes)|Usecase boundary|[README](realtime/README.md)|
|`httpclient/`|Resilient HTTP client substrate (retry / circuit breaker / tracing); shared driver-level base consumed by `webapi/` and `publisher/`|— (substrate, no domain/usecase IF)|[README](httpclient/README.md)|
|`objectstorage/`|Object storage adapter (impl of `boundary.Storage`; endpoint / credential swap connects to Garage / MinIO / production S3)|Usecase boundary|[README](objectstorage/README.md)|
|`publisher/`|Transactional outbox publish destination (HTTP impl of `boundary.Publisher`)|Usecase boundary|[README](publisher/README.md)|
|`queue/`|Message queue worker seam impl (AWS SQS impl of `worker.Consumer` / `FailureHandler`)|Usecase boundary (worker seam)|[README](queue/sqs/README.md)|
|`rdb/`|RDB subsystem (Repository / QueryService / driver / sqlc, etc.)|Domain / Usecase|[README](rdb/README.md)|
|`system/`|System-dependent operations (time retrieval, etc.)|Usecase boundary|[README](system/README.md)|
|`token/`|Opaque token generation from the OS randomness source (impl of `boundary/token.Generator`)|Usecase boundary|[README](token/README.md)<!-- sample-api:line -->|
|`webapi/`|External web API gateways (impl of `boundary.Gateway`)|Usecase boundary|[README](webapi/README.md)|

## Test Strategy

These bullets govern the subsystems whose substrate **is** the database. A subsystem built on a different substrate, or with no real I/O at all, declares its own *Test Strategy* in its package README; walking up to this section from such a package is a documentation gap to close there, not a licence to require a real database of it. Every non-database subsystem now declares one, so this section is reached only by the subsystems it was written for, and a subsystem added on a new substrate is expected to declare its own rather than inherit these bullets by default.

<!-- sample-api:replace-begin -->
The one package that reaches this section from further down is `auth/useridentity`, which reads `user_identities` through the RDB driver. `auth/`'s own section names it explicitly, so the carve-out is visible from both directions rather than only by walking up.
<!-- sample-api:replace-with -->
<!-- = A package further down that reaches this section names the carve-out in its own section too, so it is visible from both directions rather than only by walking up. -->
<!-- sample-api:replace-end -->

- Integration Test using real DB
- State isolation using transaction rollback
- Use testkit

```mermaid
flowchart TB
    DB["real DB"]
    Rollback["rollback"]
    Parallel["parallel execution"]
    Serial["Tx is serialized"]

    DB --> Rollback --> Parallel --> Serial
```

## Design Principles Summary

### 1. Encapsulation of Technical Details

DB / API / authentication  
→ encapsulated in Infrastructure

### 2. Dependency Inversion

Domain defines interfaces  
Infrastructure implements them

### 3. Separation of Responsibilities

```txt
Persistence → Repository  
Query       → QueryService
```

### 4. Transaction Management

Usecase manages transactions  
Infrastructure does not participate

### 5. Unified Error Handling

External errors → application errors

### 6. Observability

logging / tracing / metrics
