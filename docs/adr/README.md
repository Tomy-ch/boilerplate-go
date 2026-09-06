# Architecture Decision Records (ADR)

This directory holds the project's **architecture decisions**, one immutable record per
file, in [MADR-lite](https://adr.github.io/madr/) form. It supersedes the former monolithic
`docs/decisions.md` (now a redirect stub).

An ADR captures a single decision at a point in time: the context, the options weighed,
the choice, and its consequences. Superseding a decision does **not** mean editing its
ADR — it means adding a *new* ADR whose `Status` is `accepted` and marking the old one
`superseded`. The record of *why we once chose X* is preserved.

## What belongs here (and what does not)

| Kind | Example | Home |
| --- | --- | --- |
| **decision** — a choice among alternatives with lasting consequences | "Adopt onion architecture" | this dir (ADR) |
| **exclusion** — a deliberate "we intentionally do NOT do X" | "No in-application rate limiter" | this dir (ADR) |
| **rule** — a day-to-day enforced constraint / consequence of a decision | "Controller must not import infrastructure" | `docs/rules.md` (may link an ADR) |
| **inventory** — a catalog that drifts with the code | the direct-dependency table | `docs/reference/dependencies.md` (living doc) |

The dependency inventory is **not** an ADR: it tracks `go.mod` and changes continuously,
which is the opposite of an immutable record. The *policy* for selecting dependencies is
a decision (an ADR); the *list* of them is a living reference.

## Conventions

- **Filename**: `NNNN-kebab-title.md`, zero-padded 4 digits. Supersession frees no number — a superseded ADR keeps both its number and its file — so a retired decision's number is never handed to a different one.
- **Ordering**: append each new ADR at the next unused number. The number is a stable reference; use the title, tags, and the log to find the applicable decision. This is the general ADR practice described by [AWS](https://docs.aws.amazon.com/prescriptive-guidance/latest/architectural-decision-records/adr-process.html) and [Microsoft](https://learn.microsoft.com/en-us/azure/well-architected/architect-role/architecture-decision-record): preserve accepted records and express a replacement through a later ADR, rather than rearranging the historical sequence for thematic neatness.
- **References**: write `ADR-NNNN (kebab-title)`. The number identifies the current record and the slug detects a silent retarget after an upstream renumbering; `make md-doc-ref-lint` validates both against the filename.
<!-- boilerplate-only:begin -->
- **Upstream ordering exception**: while this repository is distributed as the boilerplate source, numbers follow dependency / foundational order (principles → contract → layers → subsystems → cross-cutting → exclusions), not discovery order. To preserve this order, a new ADR may be **inserted at its thematic position by shifting all subsequent numbers +1** (a pure renumbering: every shifted ADR keeps its content, and all repository-internal references are updated in the same change). This exception is removed when setup creates a project; the kebab title remains the stable identifier across an upstream renumbering.
<!-- boilerplate-only:end -->
- **Status lifecycle**: `proposed` → `accepted` → (`superseded` | `deprecated`).
- **Who may change a decision record**: superseding or deprecating an `accepted` ADR is a decision for this repository's architect or tech lead, taken per change. Finding that an accepted ADR is contradicted by the implementation is a reason to raise it with them, not a licence to change it — the implementation is as likely to be the error. A previous change is not standing authorization for the next.
- **Template**: copy [`template.md`](template.md).
- **Meta**: [`0000-record-architecture-decisions.md`](0000-record-architecture-decisions.md) records the decision to use ADRs and this classification.
- **Translation**: each ADR has a `<name>.ja.md` beside it (via the `canonicalize-doc` flow).
- **Upstream deviations**: while this repository is distributed as a boilerplate it operates under exceptions to the above, recorded in [`docs/get-started/boilerplate-only-conventions.md`](../get-started/boilerplate-only-conventions.md). They do not apply to a project built from it. <!-- boilerplate-only:line -->

## Log

All decisions from `docs/decisions.md` and the latent decisions across the repository have
been materialized as ADRs.

<!-- boilerplate-only:begin -->
Upstream numbering follows dependency / foundational order (principles → contract → HTTP →
persistence → DI/config → async subsystems → observability → toolchain/CI → process →
binary/deploy → exclusions).

Exclusion ADRs (deliberate "we do NOT do X") are tagged `setup-review` so the repository-setup flow
can enumerate them. The tag has no reader once setup is done.

<!-- boilerplate-only:end -->

| # | Decision | Status |
| --- | --- | --- |
| [0000](0000-record-architecture-decisions.md) | Record architecture decisions as ADRs | accepted |
| [0001](0001-avoid-lock-in.md) | Adopt lock-in avoidance as a design principle | accepted |
| [0002](0002-onion-architecture.md) | Adopt pragmatic onion architecture | accepted |
| [0003](0003-interface-based-decoupling.md) | Define boundaries with interfaces for loose coupling (DIP) | accepted |
| [0004](0004-modular-monolith.md) | Adopt a modular monolith (microservices are a non-goal) | accepted |
| [0005](0005-driving-adapters-not-split-axis.md) | REST / Worker / Job are driving adapters, not a service-split axis | accepted |
| [0006](0006-structural-safety-via-tooling.md) | Enforce structural safety with tooling and CI (depguard) | accepted |
| [0007](0007-agents-md-operational-contract.md) | AI-first, manual-compatible development, with AGENTS.md as the operational contract | accepted |
| [0008](0008-agent-environment-alignment.md) | Align the agent environment around declared, checkable properties, and improve it in a closed loop | accepted |
| [0009](0009-long-running-agent-state.md) | Keep durable agent state in its owning canonical form | accepted |
| [0010](0010-development-window-as-feedback-unit.md) | Make the development window the unit the feedback loop observes | accepted |
| [0011](0011-docs-as-canonical-source.md) | Docs-as-canonical-source strategy | accepted |
| [0012](0012-openapi-first.md) | Define the API contract OpenAPI-first | accepted |
| [0013](0013-redocly-modular-spec-pipeline.md) | Author the spec in modular Redocly files, bundle, then generate | accepted |
| [0014](0014-oapi-codegen-strict-server.md) | Generate per tag/handler with oapi-codegen in strict-server mode | accepted |
| [0015](0015-retain-generated-openapi.md) | Retain the bundled openapi.gen.yaml as a committed cross-repo contract artifact | accepted |
| [0016](0016-spec-driven-request-validation.md) | Validate requests and enforce auth from the spec at runtime; do not validate responses | accepted |
| [0017](0017-validation-value-authority.md) | Designate the domain layer as the sole authority for business-validity rules | accepted |
| [0018](0018-boundary-value-ownership.md) | OpenAPI is the wire contract, not the domain rule; request is subset of domain, domain is subset of response | accepted |
| [0019](0019-search-query-parameter-shape.md) | Repeat the parameter name for multi-value filters, and keep free-text input scalar | accepted |
| [0020](0020-metrics-endpoint-auth-exception.md) | /metrics is an auth exception — outside OpenAPI validation, protected by a separate BasicAuth middleware | accepted |
| [0021](0021-optional-authentication-fail-closed.md) | Optional authentication is allowed, and a failed authentication still denies the request | accepted |
| [0022](0022-echo-http-framework.md) | Adopt Echo as the HTTP framework | accepted |
| [0023](0023-priority-ordered-middleware-chain.md) | Build the middleware chain as a priority-ordered, data-driven list | accepted |
| [0024](0024-outbound-http-resilience.md) | Provide an outbound-HTTP resilience foundation (retry / circuit breaker / retry budget / dual timeout) | accepted |
| [0025](0025-egress-ssrf-guard.md) | Adopt an egress SSRF / dial-guard security posture for outbound HTTP | accepted |
| [0026](0026-sql-first-data-access.md) | SQL-first data access | accepted |
| [0027](0027-sqlc-type-safe-sql.md) | Generate type-safe SQL access with sqlc | accepted |
| [0028](0028-merged-dml-schema-as-sqlc-input.md) | Use merged DML and a dumped schema as sqlc's single input | accepted |
| [0029](0029-append-only-immutable-migrations.md) | Treat migrations as append-only and immutable | accepted |
| [0030](0030-sequential-migration-ids.md) | Use sequential 6-digit migration IDs with CI-enforced gap and pair checks | accepted |
| [0031](0031-master-data-via-migration.md) | Ship master data via migration; keep transactional seed out of production | accepted |
| [0032](0032-lightweight-cqrs.md) | Adopt lightweight CQRS — Repository for writes, QueryService for reads | accepted |
| [0033](0033-system-cqrs-dml-category.md) | Introduce system_cqrs as a fourth DML category outside the CQRS split | accepted |
| [0034](0034-commandservice-atomicity-criterion.md) | Cross-aggregate operations: usecase + outbox by default, a synchronous lock when a guard must not go stale, CommandService only for single-tx atomicity | accepted |
| [0035](0035-transaction-retry-idempotent-callers.md) | Retry transactions on serialization conflict; require callers to be idempotent | accepted |
| [0036](0036-ordered-pessimistic-row-locks.md) | Serialize contended writes with ordered pessimistic row locks taken before the guarded condition | accepted |
| [0037](0037-uuidv7-identifiers.md) | Use UUIDv7 (time-ordered) identifiers for all entity primary keys | accepted |
| [0038](0038-two-scale-quantity-model.md) | Hold a quantity in two scales — exact decimal for precision, integer minor unit for settlement | accepted |
| [0039](0039-domain-lexicon.md) | Cross-aggregate value objects live in a curated domain lexicon (`internal/domain/lexicon`) | accepted |
| [0040](0040-uber-fx-di.md) | Adopt Uber Fx for dependency injection and lifecycle | accepted |
| [0041](0041-fx-neutral-di-abstraction.md) | Contain fx behind a neutral DI abstraction (Registrar / Shutdowner) | accepted |
| [0042](0042-env-gated-wiring.md) | Swap implementations per environment via DI (env-gated wiring) | accepted |
| [0043](0043-subsystem-typed-config-loaders.md) | Subsystem-scoped envPrefix typed config loaders | accepted |
| [0044](0044-config-default-vs-required-governance.md) | Governance: default-in-code (immutable) vs required-in-file (variable) | accepted |
| [0045](0045-immutable-fail-fast-config.md) | Config is immutable, loaded once at startup, fail-fast | accepted |
| [0046](0046-embedded-self-contained-binary.md) | go:embed bundles config (.env) and migrations for a self-contained binary | accepted |
| [0047](0047-apperror-protocol-agnostic-errors.md) | Protocol-agnostic aggregated error classification (apperror) | accepted |
| [0048](0048-error-metadata-code-message-details.md) | Protocol-neutral error metadata (code / message / details) on top of apperror | accepted |
| [0049](0049-error-details-opt-in-gate.md) | Opt-in gate for error-response details via schema split (refines 0043) | accepted |
| [0050](0050-broker-agnostic-worker-scaffold.md) | Broker-agnostic pull-ack worker scaffold | accepted |
| [0051](0051-out-of-scope-push-streaming-brokers.md) | Push-type brokers and streaming-log platforms are out of scope for the worker port | accepted (exclusion) |
| [0052](0052-sqs-adapter-opt-in.md) | SQS adapter is opt-in and not linked into the default binary | superseded by [0052](0053-broker-sdk-isolation-measured-as-coupling.md) |
| [0053](0053-broker-sdk-isolation-measured-as-coupling.md) | Measure broker-SDK isolation as coupling, not as linkage | accepted |
| [0054](0054-transactional-outbox.md) | Transactional outbox: emit events within the business transaction | accepted |
| [0055](0055-at-least-once-outbox-poll.md) | At-least-once delivery via polling (transport-level retry disabled) | accepted |
| [0056](0056-skip-locked-outbox-relay.md) | Single-transaction relay using SELECT FOR UPDATE SKIP LOCKED (safe across instances) | accepted |
| [0057](0057-message-id-idempotency-propagation.md) | Propagate the outbox message_id as the receiver's Idempotency-Key | accepted |
| [0058](0058-outbox-dead-on-permanent-error.md) | An outbox row dies on a permanent error; transient failures retry with per-message backoff | accepted |
| [0059](0059-outbox-retention-gc.md) | 7-day retention GC of published rows (batches of 10,000) | accepted |
| [0060](0060-publisher-http-profile-isolation.md) | Isolate the publisher's non-standard HTTP profile inside the relay | accepted |
| [0061](0061-relay-resident-gc-oneshot.md) | The relay is a resident process; GC is a one-shot cron job | accepted |
| [0062](0062-single-tx-at-most-once-idempotency.md) | Run claim, business function, and complete in a single transaction for at-most-once semantics | accepted |
| [0063](0063-idempotency-scope-required.md) | Every Store call requires an explicit scope to prevent cross-user key collisions | accepted |
| [0064](0064-idempotency-fixed-ttl.md) | Fix idempotency key TTL at 24 hours with no per-route configuration | accepted |
| [0065](0065-idempotency-response-persistence.md) | Persist the response body as JSON to enable deterministic replay (accepted PII tradeoff) | accepted |
| [0066](0066-idempotency-gc-separate-job.md) | Run idempotency key garbage collection as a separate one-shot CLI job | accepted |
| [0067](0067-idempotency-orthogonal-concerns.md) | Keep idempotency orthogonal to optimistic locking and rate limiting | accepted (exclusion) |
| [0068](0068-job-fresh-fx-app-per-run.md) | Each job launch constructs a fresh fx.App (one-shot lifecycle) | accepted |
| [0069](0069-job-no-worker-machinery.md) | Jobs deliberately have no broker, circuit breaker, drain, or health machinery | accepted (exclusion) |
| [0070](0070-job-explicit-registration.md) | Jobs are explicitly registered (no auto-discovery) | accepted |
| [0071](0071-realtime-delivery-driving-mechanism.md) | Adopt Realtime Delivery as an independent driving mechanism for server-to-client event delivery | accepted |
| [0072](0072-postgres-state-dynamodb-eventlog.md) | PostgreSQL holds current state; the DynamoDB EventLog is a bounded replay store, not an event-sourced log | accepted |
| [0073](0073-sns-sqs-instance-fanout.md) | Fan out realtime wakeups to serve instances with SNS to per-instance SQS queues | accepted |
| [0074](0074-query-ticket-stream-authentication.md) | Authenticate SSE streams with an opaque query ticket bound to subject, destination, scope, and expiry | accepted |
| [0075](0075-config-driven-observability-gating.md) | Config-driven observability gating | accepted |
| [0076](0076-vendor-neutral-otlp-export.md) | Vendor-neutral OTLP-only export (delegate backend to the Collector) | accepted |
| [0077](0077-official-otel-semconv.md) | Use only official OpenTelemetry semantic conventions; do not invent custom semconv or put vendor keys in typed config | accepted (exclusion) |
| [0078](0078-dual-path-metrics.md) | Metrics travel two paths — OTLP push and Prometheus scrape | accepted |
| [0079](0079-lifecycle-independent-provider.md) | Observability providers are lifecycle-independent (ProviderShutdowner) | accepted |
| [0080](0080-fixed-default-sampling.md) | Fix the SDK default sampling; do not expose sampling as an env knob | accepted (exclusion) |
| [0081](0081-library-selection-policy.md) | Single-responsibility library selection policy | accepted |
| [0082](0082-bridge-instrumentation-exceptions.md) | Bridge / instrumentation libraries as bounded SRP exceptions | accepted |
| [0083](0083-containerized-pinned-toolchain.md) | Use a containerized toolchain pinned by mise for reproducibility | accepted |
| [0084](0084-mise-ssot-drift-gate.md) | mise.toml is the single source of truth for mise-resolved versions; versions propagate downstream with a CI drift gate | accepted |
| [0085](0085-make-single-entrypoint.md) | Make is the single tool entrypoint with .mk registration and self-documenting help | accepted |
| [0086](0086-scripts-in-node-go.md) | Operational scripts live in scripts/ as TypeScript or Go; shell scripting is not used | accepted |
| [0087](0087-docker-compose-dev-environment.md) | Local dev environment is provided via Docker Compose with profile-separated services | accepted |
| [0088](0088-two-layer-golangci-config.md) | Two-layer golangci config: minimal default vs full authoritative gate | accepted |
| [0089](0089-local-hooks-mirror-ci.md) | Local git hooks duplicate the CI contract (local == CI, glob-scoped, bypass-then-verify-once) | accepted |
| [0090](0090-coverage-hard-gate.md) | Total coverage 90% is a CI hard gate, with an exception-governance path | accepted |
| [0091](0091-ci-real-graph-boot-check.md) | CI boots the real fx graph against real Postgres (startup verification) | accepted |
| [0092](0092-generated-artifact-drift-gate.md) | Generated-artifact drift gate + release-branch-centralized auto-generation bot | accepted |
| [0093](0093-multi-layer-security-scanning.md) | Multi-layer security scanning, splitting reporting from gating, on hardened runners | accepted |
| [0094](0094-sha-pinned-actions.md) | Pin GitHub Actions by SHA with a supply-chain quarantine | accepted |
| [0095](0095-malicious-package-detection-via-cooldown.md) | Malicious packages are mitigated by a publication cooldown, with no dedicated detector adopted | accepted |
| [0096](0096-rollback-integration-tests.md) | Run infrastructure integration tests against a real DB with sentinel-error rollback | accepted |
| [0097](0097-multi-model-adversarial-review.md) | Use multi-model adversarial review with finder and verifier subagents | accepted |
| [0098](0098-lean-a-spec-scaffold.md) | Scaffold only domain and usecase from spec files; derive controller and infra from generated code | accepted |
| [0099](0099-cli-humble-object-split.md) | CLI humble-object split (thin cmd/ shell + testable internal/cli core) | accepted |
| [0100](0100-single-multi-command-binary.md) | All roles are one multi-command binary | accepted |
| [0101](0101-single-runtime-image.md) | A single runtime image with command override (no purpose-specific images) | accepted |
| [0102](0102-hardened-alpine-runtime.md) | Use a hardened-alpine runtime base; do NOT use distroless/scratch | accepted (exclusion) |
| [0103](0103-per-environment-images.md) | Per-environment images (.env matrix x APP_ENV build-arg, fixed at build time) | accepted |
| [0104](0104-predeploy-oneshot-migration.md) | Migrations run as a pre-deploy one-shot; do NOT auto-migrate at application startup | accepted (exclusion) |
| [0105](0105-release-image-supply-chain.md) | Release-image supply-chain integrity (cosign signing + provenance + SBOM) | accepted |
| [0106](0106-vendor-neutral-deploy-skeleton.md) | Deploy is a vendor-neutral skeleton (build/sign implemented; cloud CD is a stub; registry not fixed) | accepted |
| [0107](0107-docs-via-github-pages.md) | Publish static docs/ via GitHub Pages (released on production push) | accepted |
| [0108](0108-no-in-app-rate-limiter.md) | Do not provide an in-application rate limiter | accepted (exclusion) |
| [0109](0109-scheduled-job-concurrency-delegated.md) | Do not control scheduled-job concurrency in-app; delegate to the scheduler | accepted (exclusion) |
| [0110](0110-no-generic-cache-abstraction.md) | Do not provide a generic Cache abstraction | accepted (exclusion) |
| [0111](0111-outbox-relay-hardening-delegated.md) | Ship a balanced outbox relay; delegate hardening (multi-layer lease redesign) to operational evidence | accepted (exclusion) |
| [0112](0112-pnpm-as-the-only-node-resolver.md) | Resolve every Node package with pnpm; do not use npm | accepted |
| [0113](0113-outbox-payload-kinds-and-parity-declaration.md) | Declare each outbox payload's kind; check a snapshot's field correspondence against its aggregate | accepted |

Frontmatter fields: `status`, `date`, `deciders`, `supersedes` / `superseded-by`, `tags`.
Consequences follow the MADR standard (`Positive` / `Negative`; optional `Neutral`).
