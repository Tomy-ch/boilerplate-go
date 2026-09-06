# Idempotency Subsystem Design Reference

This document consolidates the idempotency (`Idempotency-Key`) subsystem's **role theory, state transitions, implementation locations, what an integrator must implement, and glossary** into a single reference, derived from a close reading of the implementation. For the overview see the README; for the HTTP path it plugs into see [rest.md](rest.md), and the GC side runs as a [job](job.md).

---

## 1. Role theory (what, and what for)

A database transaction guarantees atomicity **within one request**; it does **not** deduplicate client retries (network timeout, double-submit, auto-retry). When a write has no natural unique key — a `POST` that allocates its own id, a balance increment, a charge, an email send — a retry runs the side effect **again**.

Idempotency makes such writes safe via a client-supplied **`Idempotency-Key` header**: the side effect runs **at most once**, and a retry of a completed operation **replays the stored response**. It is a cross-layer mechanism (entry middleware → usecase orchestration → persistence), **opt-in per handler**, and **orthogonal** to optimistic locking (lost-update prevention) and rate limiting (edge concern).

Responsibility split (who owns what):

| Component | Layer | Responsibility | Does NOT hold |
| --- | --- | --- | --- |
| **middleware** (`httpstack/idempotency`) | controller | extract + validate `Idempotency-Key`, require authn, compute the request **fingerprint**, thread `Request` into ctx | transaction, persistence, replay decision |
| **Run[T]** orchestrator | usecase (`usecase/idempotency`) | `claim → businessFn → complete` in **one tx** / replay·409·422 routing / TTL stamping / metrics | HTTP parsing, SQL, business rules |
| **Store** (seam) | usecase/boundary | persistence contract: `Claim` / `Get` / `Complete` / `DeleteExpired` + `Status` / `Record` / `ErrLockTimeout` | implementation, business policy |
| **store** impl | infrastructure (`rdb/system_cqrs`) | sqlc wrap / `SET LOCAL lock_timeout` / `ON CONFLICT DO NOTHING` / `pgerror.NormalizeError` | replay decision, HTTP |
| **GCUsecase + `idempotencygc` job** | usecase + controller/job | batch-delete expired keys (TTL housekeeping) | the request path |
| **`idempotency_keys` table** | database | persisted state (scope / key / fingerprint / status / response / `expires_at`) | — |

Design principles (invariants):

- **At most once per `(scope, key)`.** Claim, `businessFn`, and Complete share **one transaction**, so a business failure rolls the claim back too — the key is **auto-released** for a clean retry.
- **Scope is mandatory.** Every `Store` method takes `scope` (the authenticated principal); there is **no id-only lookup**, which prevents cross-scope key collision / IDOR. The DB enforces `UNIQUE(scope, idempotency_key)`.
- **Fail-closed fingerprint.** If the request cannot be marshalled the middleware returns an error rather than forging a weak fingerprint.

---

## 2. State transitions

### 2.1 record lifecycle (one row in `idempotency_keys`)

```mermaid
stateDiagram-v2
    [*] --> Claimed: Claim — INSERT ... ON CONFLICT DO NOTHING (status='claimed', expires_at=now+24h)
    Claimed --> Completed: Complete — UPDATE claimed→completed (+response_status, +response_payload)
    Claimed --> Released: businessFn / Complete fails → tx ROLLBACK (row never persisted = key free again)
    Completed --> Expired: expires_at < now → swept by the GC job
    Released --> [*]
    Expired --> [*]

    note right of Claimed
      a concurrent duplicate sees status='claimed' (or a lock timeout) → 409.
    end note
    note right of Completed
      a retry with the same fingerprint replays the stored response (no businessFn).
      a retry with a different fingerprint → 422.
    end note
```

### 2.2 per-request decision (`Run[T]`)

```mermaid
stateDiagram-v2
    [*] --> KeyPresent: Idempotency-Key in ctx AND Scope≠""?
    KeyPresent --> Passthrough: no → businessFn directly → (res, replayed=false, err)
    KeyPresent --> OpenTx: yes → Txm.Do (one tx)
    Passthrough --> [*]

    OpenTx --> Claim: Store.Claim(scope,key,fingerprint,expires_at)
    Claim --> Business: claimed=true (new key)
    Claim --> Existing: claimed=false (key exists)
    Claim --> Conflict: ErrLockTimeout → IncConflict → 409 (ErrConflict)
    Claim --> TxErr: other error → return (tx rollback)

    Business --> Complete: businessFn ok → marshal(T) → Store.Complete(successStatus, payload)
    Business --> BizErr: businessFn err → return err (tx rollback, claim released)
    Complete --> CommitOK: COMMIT → (result, replayed=false, nil)

    Existing --> Get: Store.Get(scope,key)
    Get --> RaceGone: nil → IncConflict → 409 (ErrConflict)
    Get --> FpCheck: record found
    FpCheck --> Mismatch: fingerprint ≠ → IncFingerprintMismatch → 422 (ErrValidation)
    FpCheck --> StillClaimed: status≠completed → IncConflict → 409 (ErrConflict)
    FpCheck --> Replay: status=completed → Unmarshal → IncHit → (result, replayed=true, nil)

    CommitOK --> [*]
    Conflict --> [*]
    TxErr --> [*]
    BizErr --> [*]
    RaceGone --> [*]
    Mismatch --> [*]
    StillClaimed --> [*]
    Replay --> [*]

    note right of Claim
      Claim is INSERT ... ON CONFLICT DO NOTHING under SET LOCAL lock_timeout='3s'.
      Lock unavailable within 3s → ErrLockTimeout (concurrent in-flight claim).
    end note
```

> Branch → status mapping: **409 `ErrConflict`** = a concurrent/in-flight claim (lock timeout, `status='claimed'`, or a vanished record); **422 `ErrValidation`** = same key reused with a different request body; **replay** = same key + same fingerprint on a completed op (response restored, `businessFn` not called). `Run` returns `(T, replayed bool, error)`; on replay only the saved body `T` is restored, not the stored status code (the op is assumed single-success-status, e.g. 201).

### 2.3 TTL & GC

```mermaid
stateDiagram-v2
    [*] --> Live: row inserted with expires_at = claimed_at + 24h (ttl)
    Live --> Stale: expires_at < now
    Stale --> Deleted: idempotency-gc job → SweepExpired(batchSize) loops DeleteExpired until a short batch
    Deleted --> [*]

    note right of Stale
      after TTL a retry is treated as a fresh operation (no cached state).
      DeleteExpired removes ≤ limit rows ordered by expires_at; the index keeps it cheap.
    end note
```

---

## 3. Implementation locations (where in the architecture it lives and acts)

### 3.1 Package placement and dependency direction

```mermaid
flowchart TD
    subgraph ctrlL["internal/controller"]
        MW["httpstack/idempotency: Middleware / StrictMiddleware<br/>key validate + fingerprint + WithRequest"]
        JOB["job/idempotencygc: SweepExpired CLI job"]
    end
    subgraph ucL["internal/usecase/idempotency"]
        RUN["run.go: Run[T], decideExisting, Metrics, ttl=24h"]
        CTX["context.go: Request, WithRequest / requestFromContext"]
        DEPS["deps.go: Deps{Txm,Store,Clock,Metrics}, NewDeps"]
        GC["gc.go: GCUsecase.SweepExpired, DefaultGCBatchSize=10000"]
    end
    subgraph seamL["internal/usecase/boundary"]
        PORT["idempotency/store.go: Store, Status, Record, ClaimParams, CompleteParams, ErrLockTimeout"]
        TXP["tx.Manager (transaction boundary)"]
        CLK["clock.Clock (now)"]
        MOCK["idempotency/mock/: generated mock"]
    end
    subgraph infraL["internal/infrastructure/rdb"]
        IMPL["system_cqrs/idempotency: store impl<br/>SET LOCAL lock_timeout='3s' + ON CONFLICT + pgerror"]
        SQLC["sqlc/gen: ClaimIdempotencyKey/Get/Complete/DeleteExpired"]
    end
    subgraph dbL["database"]
        DML["dml/system_cqrs/idempotency/*.sql"]
        MIG["migrations: idempotency_keys (UNIQUE(scope,key), expires_at idx)"]
    end
    subgraph crossL["cross-cutting"]
        APPERR["apperror: ErrConflict / ErrValidation / ErrInvalidArgument / ErrInternal"]
        AUTHN["ctxhelper: GetAuthn → Subject() = Scope"]
    end

    MW --> CTX
    MW --> AUTHN
    MW -.400/500.-> APPERR
    RUN --> CTX
    RUN --> DEPS
    RUN --> PORT
    RUN --> TXP
    RUN --> CLK
    RUN -.classify.-> APPERR
    GC --> PORT
    GC --> CLK
    JOB --> GC
    IMPL -- implements --> PORT
    IMPL --> SQLC
    SQLC --> DML
    DML --> MIG

    classDef done fill:#e6ffed,stroke:#2da44e;
    class MW,JOB,RUN,CTX,DEPS,GC,PORT,TXP,CLK,MOCK,IMPL,SQLC,DML,MIG,APPERR,AUTHN done;
```

> Dependencies point inward (`controller→usecase`, `infrastructure→usecase/boundary`). The orchestrator (`Run`) knows nothing about SQL — it depends only on the `Store` seam and the `tx.Manager`; the RDB `store` implements `Store` and is the only place that touches sqlc and `pgerror`.

### 3.2 Per-request action sequence (a `POST` adopting idempotency)

```mermaid
sequenceDiagram
    participant C as Client
    participant MW as Middleware (controller)
    participant H as Handler.PostX
    participant R as Run[T] (usecase)
    participant S as Store (rdb)
    participant U as businessFn (usecase)
    C->>MW: POST /v1/x (Idempotency-Key: k)
    MW->>MW: validate key, GetAuthn→Scope, fingerprint=sha256(method · path · json(req))
    MW->>H: next(ctx with Request{Scope,Key,Fingerprint,...})
    H->>R: Run(ctx, deps, 201, businessFn)
    R->>S: Claim(scope,k,fp,now+24h)  // BEGIN tx, SET LOCAL lock_timeout=3s
    alt claimed=true (new)
        R->>U: businessFn(ctx)
        U-->>R: dto
        R->>S: Complete(scope,k, 201, json(dto))  // COMMIT
        R-->>H: (dto, replayed=false, nil) → 201
    else claimed=false (exists)
        R->>S: Get(scope,k)
        alt completed & fingerprint match
            R-->>H: (stored dto, replayed=true, nil) → 201 (replay)
        else status=claimed / lock timeout
            R-->>H: 409 ErrConflict
        else fingerprint mismatch
            R-->>H: 422 ErrValidation
        end
    end
```

---

## 4. What an integrator implements (adoption is opt-in, two steps)

This project provides the **middleware, `Run[T]` orchestrator, `Store` seam + RDB impl, schema, GC usecase/job, and a reference adoption** on one sample resource-creating endpoint. A handler is idempotent **only if both steps are done** — otherwise it behaves normally.

```mermaid
flowchart LR
    M["① slot the middleware<br/>StrictMiddleware in NewStrictHandler"]:::need
    W["② wrap the usecase call<br/>idempotency.Run(ctx, deps, status, fn)"]:::need
    O["③ (optional) metrics / scope<br/>Deps.Metrics, scope composition"]:::need
    M --> W --> O
    classDef need fill:#fff8c5,stroke:#bf8700;
```

| # | Required implementation | Location | Reference |
| --- | --- | --- | --- |
| ① | add `idempotency.StrictMiddleware[gen.StrictHandlerFunc]()` to the handler's `NewStrictHandler` middleware slice; take `idempotency.Deps` in `BindHandler` | `internal/controller/handler/<path>/*_handler.go` | the sample POST handler |
| ② | wrap the usecase call: `idempotency.Run(ctx, s.idem, http.StatusCreated, func(ctx) (T, error) { return s.uc.Create(...) })` | same handler method | its `Post<Resource>` method |
| ③ (opt) | inject `Deps.Metrics` when an o11y backend exists; widen `Scope` (e.g. `subject:operationID`) in the middleware if per-endpoint isolation is wanted | DI / middleware | `NewDeps`, `WithRequest` |

Operational notes (no per-route config flags — these are coded constants):

- **TTL = 24h**, **header = `Idempotency-Key`**, **key ≤ 255 printable-ASCII chars**, **GC default batch = 10,000** (overridable via the job's `--batch-size=N`).
- Schedule the GC: `<binary> job idempotency-gc --batch-size=10000` on an external cron / k8s CronJob (hourly is plenty for a 24h TTL).
- **PII caveat:** the response body is stored as JSON; for PII-bearing DTOs, dumps/backups expose it (mitigated by the 24h TTL).

---

## 5. Glossary

| Term | Meaning |
| --- | --- |
| **Idempotency-Key** | Client-supplied request header (≤255 printable-ASCII chars) identifying one logical operation within a scope. |
| **scope** | The namespace for key uniqueness = the authenticated principal (`authn.Subject()`). `UNIQUE(scope, key)` prevents cross-user collision / IDOR. Every `Store` call requires it. |
| **fingerprint** | `SHA-256(method + "\n" + path + "\n" + json(request))`. Detects the same key reused with a different body (→ 422). Computed fail-closed by the middleware. |
| **Claim** | `INSERT ... ON CONFLICT DO NOTHING` under `SET LOCAL lock_timeout='3s'`. Returns `claimed=true` (new), `false` (exists), or `ErrLockTimeout`. Runs inside the business tx. |
| **claimed / completed** | The two `status` values. `claimed` = reserved, result not yet saved; `completed` = `businessFn` succeeded and the response is stored. |
| **Complete** | `UPDATE claimed→completed` saving `response_status` + `response_payload` (JSON of `T`), in the same tx. |
| **replay** | Returning the stored response for a same-`(scope,key,fingerprint)` completed op; `businessFn` is not run. `Run` returns `replayed=true`. Counter `IncHit`. |
| **409 `ErrConflict`** | A concurrent / in-flight claim — lock timeout, `status='claimed'`, or a record that vanished after a claim collision. Client should retry later. Counter `IncConflict`. |
| **422 `ErrValidation`** | Same key reused with a different request body (fingerprint mismatch). Client bug. Counter `IncFingerprintMismatch`. |
| **ErrLockTimeout** | Boundary sentinel from `Claim` when the row lock is unavailable after 3s; the usecase maps it to 409. |
| **Run[T]** | The orchestrator. `Run(ctx, deps, successStatus, businessFn) (T, bool, error)`. No key in ctx (or empty scope) → `businessFn` runs directly. |
| **Deps** | Injected bundle for `Run`: `Txm` (`tx.Manager`), `Store`, `Clock`, optional `Metrics` (nil = no-op). |
| **Request (context)** | In-flight metadata threaded by the middleware: `Scope` / `Key` / `Fingerprint` / `Method` / `Path` / `OperationID`. |
| **ttl** | `24 * time.Hour`. `expires_at = now + ttl`. After it, a retry is a fresh op. |
| **GCUsecase / idempotencygc** | `SweepExpired(batchSize)` loops `Store.DeleteExpired` until a short batch; run from the bundled CLI [job](job.md). Default batch 10,000. |
| **Store** | The persistence seam (`internal/usecase/boundary/idempotency`): `Claim` / `Get` / `Complete` / `DeleteExpired`, all scope-mandatory. |
| **Metrics** | Optional o11y counters labelled by `operationID`: `IncHit` / `IncMiss` / `IncConflict` / `IncFingerprintMismatch` / `IncClaimFailure` / `IncCompleteFailure`. Default no-op. |
