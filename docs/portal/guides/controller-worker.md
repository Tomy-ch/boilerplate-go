# Worker Engine Guide (`internal/controller/worker`)

## Role in Onion Architecture

- A **message-in driving adapter**, on par with the HTTP handler — it is **another entry point into the Usecase layer**, not a new architectural layer.
- Consumes a pull-ack queue and dispatches each message to a business `Handler`.
- Depends only on the seam ports in `internal/usecase/boundary/worker` (`Consumer` / `Handler` / `FailureHandler` / `Worker` / `State`); it never imports `internal/infrastructure/queue/*` (enforced by depguard `maintain_a_sound_controller`).

> The ports live in `internal/usecase/boundary/worker` (not here) because that is the only package both the engine (controller) and the broker adapters (infrastructure) can import under the layer rules — same reason `job` keeps its ports there.

## Pull-type premise & first-class platforms

- This worker is **pull-type**: the consumer **pulls** messages via `Receive`. The interface is designed first and foremost for **AWS SQS** and **GCP Pub/Sub (pull)**.
- Other pull-ack platforms (Azure Service Bus, Cloudflare Queues HTTP pull, ...) are **illustrative examples**: they generally fit by **writing an adapter only** (no interface change).
- **Rewriting the interface itself is only needed for platforms that fundamentally do not fit pull-ack** (push delivery, streaming-log).
- **Push-type brokers (e.g. RabbitMQ) are out of scope** — push delivery (e.g. Pub/Sub push, webhooks) is the HTTP controller's domain. Rationale: in worker workloads pull is the majority and lets the consumer own backpressure.

## "Stopping" — three distinct mechanisms

These are easy to conflate. The circuit breaker is applied to the **intake side** (whether to keep pulling), which is less common than the usual "protect a downstream call" framing — so the distinction is documented here.

| Mechanism | What it stops | Recovery | Process |
| --- | --- | --- | --- |
| **Backoff** | speed control only (never stops intake) — realized as the Open cooldown's exponential growth (`pkg/backoff`), not a standalone runtime state | automatic | alive |
| **Circuit Open** | **stops calling `Receive`** (intake) on continued downstream failure | **automatic** (Open → cooldown → Half-open → Closed) | alive |
| **Fatal** | drains and **stops the engine** | manual (restart) | exits |

- **Open ↔ Fatal boundary**: continued Retryable failures escalate the circuit (Open → cooldown grows on each Open→Half-open→Open cycle); the engine is taken down (Fatal) only when a `Handler` returns `apperror.ErrFatal` (e.g. unrecoverable config error). Circuit Open is a temporary, self-healing pause; Fatal is terminal.
- **Circuit (engine-wide) vs redelivery backoff (per-message)**: the circuit throttles the whole poll loop (how much to pull from the queue); per-message redelivery delay is a **first-class port capability** — the engine owns the backoff policy (exponential from `ReceiveCount` + full jitter via `pkg/retry`) and calls `Consumer.NackWithBackoff(ctx, m, d)`, which the adapter honours through its native mechanism (e.g. SQS `ChangeMessageVisibility`). They remain different layers and coexist: the circuit is broker-agnostic intake backpressure; the redelivery backoff is per-message and broker-honoured.

## Invariants (acceptance criteria)

The engine is **completed against the in-memory fake** (`internal/usecase/boundary/worker/testkit`, the `Fake` test double); all engine tests are green without a real broker. Test names map to invariant IDs A1–A7 / B1–B4 / C1 (engine) plus D1–D3 (O11Y). Key ones:

- A1/A2: `Ack` only after success; `NackWithBackoff` (per-message exponential + jitter) on Retryable.
- A5: Permanent → `FailureHandler` → `Ack`; Fatal → stop.
- A6: a single message's panic is recovered and does not take down the engine.
- B1/B2/B3: concurrency cap / in-flight cap / `PartitionKey` serialization.
- B4: circuit breaker (Open pauses intake; Half-open recovers).
- SIGTERM/SIGINT drains in-flight; unfinished messages are not `Ack`ed (redelivered).
- D1–D3: traceparent continuation / engine-owned metrics / structured logs.

## Files

- `runner.go` — `Engine` (registry, `Run`, `Healthy`), `run.go` — per-run poll loop / dispatch / drain.
- `circuit.go` — 3-state breaker (cooldown via `pkg/backoff`). `classify.go` — error → category. `settings.go` — engine-core `Settings`. `dispatch.go` — `PartitionKey` keyed serialization. `state.go` — `worker.State` impl. `errors.go` — registry sentinels. `telemetry.go` — O11Y (traceparent continuation / structured-log fields; the engine-owned metrics themselves live in `observability.WorkerMetrics`).

The SQS reference adapter (`internal/infrastructure/queue/sqs`) may be wired as part of the **removable sample set**. Broker-SDK isolation is measured as **coupling** rather than as linkage — a concrete broker is named only by its adapter package and the wiring that selects it — see E3 in [ADR-0053 (broker-sdk-isolation-measured-as-coupling)](../../../docs/adr/0053-broker-sdk-isolation-measured-as-coupling.md).

<!-- sample-api:begin -->
[`withdrawalarchive/`](withdrawalarchive/README.md) is the bundled sample worker — the worked example of everything `docs/design/worker.md` §4 asks an integrator to supply. It consumes the withdrawal event the outbox emits, and is removed with the rest of the sample set.
<!-- sample-api:end -->

## Config clamping (safe defaults, not silent)

`Settings.normalize()` (`settings.go`) **clamps** out-of-range engine-core values to safe defaults rather than failing startup — a resilience choice so a misconfigured worker still runs instead of crash-looping. Clamped fields: `Concurrency` / `MaxInFlight` / `BatchSize` (coerced into `Concurrency <= MaxInFlight` and `1 <= BatchSize <= MaxInFlight`), `DrainTimeout`, `CircuitHalfOpenProbe`, `CircuitOpenBackoffInitial` / `CircuitOpenBackoffMax` (so an enabled breaker never degenerates to a zero cooldown), `ProgressStaleAfter`, and `NackBackoffInitial` / `NackBackoffMax`. The `WORKER_*` env vars carry non-zero `envDefault`s, so a clamp only triggers when an operator explicitly sets `0` / a negative value. Documenting it here (and in the setup review, see [`docs/get-started/setup-repository.md`](../../../docs/get-started/setup-repository.md)) keeps the clamping reviewable rather than silent.

> Design deep-dive (state transitions / implementation map / glossary): [docs/design/worker.md](../../../docs/design/worker.md).
