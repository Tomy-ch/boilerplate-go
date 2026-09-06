---
status: accepted
date: 2026-08-05
deciders: [maintainers]
tags: [persistence, cqrs, architecture, concurrency]
---

# ADR-0034: Reserve CommandService for multi-aggregate writes that require single-transaction atomicity

## Status

accepted

## Context

[ADR-0032](0032-lightweight-cqrs.md) defines the *structure* of the CommandService
construct: its interface lives in the usecase layer (`internal/usecase/<workflow>/command/`),
its implementation lives under `internal/infrastructure/rdb/command_service/<aggregate>/`, it
returns DTOs, and it is registered in `persistenceModule` and injected via DI. What ADR-0032
deliberately leaves open is the *placement criterion*: when does a write operation actually
warrant a CommandService instead of a regular usecase composed of Repository calls?

Without such a criterion, the placement of multi-aggregate write operations falls back to
implementer judgment about inter-aggregate semantics, and decisions vary by implementer. The
naive heuristic — "it touches more than one aggregate, so it needs a CommandService" — widens the
aggregate boundary whenever that is convenient, and a boundary that widens on convenience
constrains nothing. A related ambiguity
exists in ADR-0032 itself: its consequences note that CommandService "can freely optimize
flexible updates, deletes", which could be misread as write-shape flexibility or performance
being sufficient grounds for introducing one.

That question is about *writes*, and it does not reach every way an operation crosses an aggregate
boundary. An operation may also **read** another aggregate to decide whether it is permitted at all
— a **guard**. A guard writes nothing, so an atomicity criterion never triggers on it and the
operation lands on the default path by omission rather than by decision. Under READ COMMITTED,
however, a condition that was merely read is not held: a concurrent write can invalidate it between
the check and the commit ([ADR-0036](0036-ordered-pessimistic-row-locks.md)). "It spans aggregates
but it is a read, so no write atomicity is required" is therefore a true sentence with a right
conclusion — a regular usecase — and an incomplete answer, because the read may still have to be
serialized against the write that would invalidate it. Left without a branch of its own, the choice
between holding a cross-aggregate condition synchronously and letting it go stale falls back to
exactly the implementer judgment this ADR exists to remove.

The goal is to decide where an operation that crosses an aggregate boundary belongs — for its
writes and for the conditions it reads — based on a verifiable criterion rather than implementer
judgment.

## Decision

**CommandService is defined as the write-side counterpart of QueryService.** Here,
CommandService is not a layer; it is a processing category contrasted with QueryService.

### Definition

QueryService and CommandService are defined as the same kind of construct:

- **QueryService** — the residue that remains when read performance requirements forbid
  decomposition into per-aggregate reads.
- **CommandService** — the residue that remains when write atomicity (immediacy) requirements
  forbid decomposition into eventual consistency.

Neither is a semantic domain category. **Both are constructs that stand where non-functional
requirements forbid aggregate decomposition.**

The underlying fact: given unlimited latency and resources, every read decomposes into
per-aggregate Repository reads joined in application code, and every write decomposes into a
single-aggregate write followed by an eventually consistent cascade. QueryService and
CommandService are needed only where non-functional requirements forbid that decomposition.

### Criterion

Two independent questions decide where a cross-aggregate operation belongs. They are independent
because one is about reading and the other about writing, and a single operation may answer yes to
both:

1. **Does a condition read from another aggregate have to hold for the rest of the transaction?**
   Equivalently: can a concurrent operation invalidate the checked condition between the check and
   the commit? This asks whether a *read may go stale*, not whether a write is atomic, which is why
   the second question never reaches it.
2. **Does the multi-aggregate write require single-transaction atomicity?** Immediacy — all effects
   being visible at API response time — is the typical reason this requirement arises.
3. **Can the rows to be written be named by identity?** A write whose targets are fixed only by a
   predicate cannot be enumerated or locked and has no upper bound, so decomposing it would issue one
   round trip per row. Added by
   [ADR-0114](0114-predicate-defined-set-writes-on-commandservice.md); this ADR did not reach it
   because it is asked of every write, not only of one that crosses a boundary.

### Decision procedure

The default is decomposition. A guard that must hold until commit takes a synchronous row lock and
stays a regular usecase; CommandService is reached only when single-transaction atomicity of the
multi-aggregate *write* remains as a requirement (branch 3a) — or, per ADR-0114, when the rows to be
written are named only by a predicate (branch 3b). Two justifications are not acceptable: "it spans
multiple aggregates, therefore CommandService", and — the failure mode branch 2 exists to close —
"it is only a read, therefore nothing is needed".

The procedure itself, and the eligibility gate it composes with, are stated once in
[`docs/design/data-access-pattern.md`](../design/data-access-pattern.md).

### Departure from "1 Aggregate = 1 Transaction Boundary"

Branches 2 and 3a both put rows belonging to more than one aggregate inside a single transaction, so
both depart from the principle [`internal/domain/README.md`](../../internal/domain/README.md)
(§ Aggregate Boundary) states as "1 Aggregate = 1 Transaction Boundary" — Vernon's formulation of
Evans's argument that the aggregate is the unit of consistency. The departure is stated here rather
than left implicit:

- **What Evans holds.** The aggregate is the boundary of *immediate* consistency. One transaction
  changes one aggregate; anything beyond it is reconciled afterwards. A boundary that may be widened
  whenever it is convenient constrains nothing, so the discipline is the point of the pattern.
- **What this repository does instead.** Two named widenings, and only these two. A guard locks a row
  belonging to another aggregate and holds it until commit (branch 2). A write that must be atomic
  across aggregates runs in one transaction through a CommandService (branch 3a). The second path into
  a CommandService — branch 3b, a write whose target rows are named only by a predicate
  ([ADR-0114](0114-predicate-defined-set-writes-on-commandservice.md)) — is **not** a third widening:
  the write it admits may stay inside one aggregate. The consequence for a reader is that the presence
  of a CommandService no longer signals which boundary was crossed; branch 3a does.
- **Why.** Evans's argument is about *change*: the hazard in a wide boundary is mutating several
  aggregates through one loaded graph until no one can say which invariant belongs to which root.
  Branch 2 does not mutate the other aggregate at all — it observes one row and blocks the writer
  that would invalidate the observation, so that aggregate's root keeps sole authority over its own
  changes. Branch 3a does mutate more than one, which is why it is the narrow exception: it is
  admitted only where the requirements say the intermediate state must never be observable, and the
  condition it enforces is still authored by the domain
  ([ADR-0032](0032-lightweight-cqrs.md) § Derivation). What both branches refuse is the option a
  strict reading would otherwise leave as the default — deciding a cross-aggregate question from a
  read that nothing holds, which is not eventual consistency but no consistency at all.

### Worked instances

<!-- 撤去後にこの箇所へ自分の例を置くための指針。
     目的: 判定手順は分岐の形しか述べておらず、実際にどう当てはめたかは実例が無いと読めない。
     意義: 効くのは「どの分岐に当たったか」ではなく「なぜその分岐だと判断したか」を残すこと。
     書き方: 自分の書き込み操作から分岐 1 / 2 / 3 に当たる例を 1 件ずつ挙げ、それぞれ
             『何が同時に書かれるか』『途中状態が観測されてよいか』を明記する。 -->
<!-- sample-api:replace-begin -->
- **Purchase creation — branch 2 for the guard, and a write that still decomposes.** Stock validation
  and decrement must be atomic with purchase confirmation: a state where "a purchase succeeded
  without stock" must never be observable, even momentarily (no overselling). That atomicity is
  supplied by the transaction the usecase already owns, not by a CommandService — the rows to be
  written are named by identity (the products the details reference), so they are locked in id order
  first and the write decomposes into `product.Repository.UpdateStock` plus
  `purchase.Repository.Create`. This is the instance to read against the one below: three aggregates
  are touched and the count still decides nothing. Independently, the same transaction guards on the
  purchaser still being a member, and that condition can be invalidated by a concurrent withdrawal,
  so the user row is locked first (branch 2). The outbox insert joins the same transaction as usual
  per [ADR-0054](0054-transactional-outbox.md), and never justifies a CommandService — a regular
  usecase writes the outbox in its own transaction too. Specified in
  `docs/spec/usecase/purchase.md`.
- **User withdrawal — branch 2 for the guard, branch 1 for the cascade.** The core of withdrawal is
  a single-aggregate write to `users.deleted_at`. The cascade — cancelling pending purchases and
  restoring stock — requires no immediacy and is eventually consistent via outbox events (branch 1).
  The check "cannot withdraw with purchases in progress" spans aggregates and is a read, so branch 3a
  does not apply and the operation stays a usecase; but a concurrent purchase creation would
  invalidate it, so branch 2 does apply and the user row is locked exclusively before the check.
  This is the shape the two-way procedure could not describe: a usecase that nonetheless takes a
  lock. Specified in `docs/spec/usecase/user.md`.
- **Product unpublication — branch 1, with no guard at all.** Clearing a product's publication date
  is a single-aggregate write, and the cross-aggregate condition it might appear to threaten — the
  in-progress purchases that reference the product — is deliberately not guarded. A purchase records
  a unit-price snapshot when it is created, and no step of its lifecycle re-reads the product's
  publication state, so the product going unpublished cannot make the purchase wrong. The condition
  is therefore allowed to go stale, and the update takes no lock on any purchase row. This is the
  contrasting instance to the withdrawal guard: same shape (one aggregate's write, another
  aggregate's state), opposite answer to question 1. Specified in `docs/spec/usecase/purchase.md`.
- **Product discontinuation — branch 3a, and the instance that reaches it by atomicity.** Discontinuing a
  product unpublishes it and issues a compensating coupon to everyone who was holding it in a cart.
  The recipients are defined by a predicate — a join through `cart_items` to `carts`, excluding
  withdrawn users — so they cannot be named by identity, cannot be locked, and have no upper bound.
  Decomposing the write would mean reconstructing one `Coupon` aggregate per recipient, which is the
  write-side mirror of `docs/design/data-access-pattern.md` §3.3. The requirement that settles it is
  that "coupons handed out while the product is still purchasable" must never be observable: the
  coupon is compensation for losing the product, so a window in which someone holds both is wrong.
  The product write and the bulk issuance therefore share one transaction through a CommandService.
  **Read this against purchase creation above:** that operation also touches three aggregates and
  still decomposes, because its rows are named by identity. The count is the same; the answer is not.
  Two things this instance is deliberately *not*. The cart items are **not** removed — an item that
  stays with a `discontinued` issue keeps the reason visible, and unpublication alone is branch 1 by
  the instance above, so removal would carry no part of the argument. And the two statements the
  implementation uses (select recipients, then bulk insert) are **not** a departure: the criterion for
  a set operation is that round trips do not grow with the population, not that there is exactly one
  statement. Splitting is forced by [ADR-0037](0037-uuidv7-identifiers.md), which puts key generation
  in the domain. Specified in `docs/spec/usecase/product.md`.
  **This instance answers both questions yes,** which is why one branch could carry it alone for so
  long. Its rows are named by a predicate, so branch 3b would admit it on its own; the product write is
  what makes branch 3a the settling reason. Read it against the promotional issuance below, which
  answers only 3b.

- **Promotional bulk issuance — branch 3b, and the instance that reaches it without atomicity.**
  Issuing one promotional coupon to every user who has not withdrawn writes `coupons` and nothing else.
  No second aggregate is written, so branch 3a never triggers: there is no intermediate state spanning
  two aggregates for an observer to catch, and no part of this operation must be immediate. It still
  cannot decompose. The recipients are defined by a predicate over `users`, so they cannot be named by
  identity, cannot be locked, and have no upper bound; expressing the write as load-mutate-save would
  mean constructing one `Coupon` per recipient and issuing one round trip per recipient. That is branch
  3b, and it is the whole reason this operation is a CommandService.
  **Read this against the discontinuation above:** both issue coupons in bulk by predicate, and only one
  of them widens a transaction boundary. The pair is what separates the two questions — "must these
  writes commit together" and "can these rows be named" — that a single branch 3 used to answer at once.
  Two things this instance is deliberately *not*. It does **not** record the distribution: nothing marks
  that a campaign ran, so running the operation twice issues to everyone twice. Where discontinuation
  has `IsDiscontinued` as a natural re-entry guard, this has none, and the duplicate-run defence is the
  caller's `Idempotency-Key` alone. And the recipient cap is **not** a domain invariant — it is an
  application policy checked in the usecase before the write, so the CommandService still enforces no
  condition the domain did not author ([ADR-0032](0032-lightweight-cqrs.md) § Derivation). Decided in
  [ADR-0114](0114-predicate-defined-set-writes-on-commandservice.md); specified in
  `docs/spec/usecase/coupon.md`.

- **Coupon redemption — branch 1, and the lock is on a row this transaction writes.** Applying one
  coupon at checkout writes both `purchases` and `coupons`, and a state where the purchase is
  confirmed at the discounted price while the coupon is still spendable must never be observable —
  the coupon would be spent twice. That atomicity is supplied by the transaction the usecase already
  owns, exactly as in purchase creation above: the rows are named by identity, since the caller
  supplies the coupon id, so the write decomposes into `coupon.Repository.LockByID` +
  `coupon.Repository.UpdateUsed` alongside `purchase.Repository.Create`.
  **The coupon lock is not a branch-2 guard.** A guard writes nothing and holds a row against the
  writer that would invalidate the observation; here this transaction *is* that writer, so
  `LockByID` is the ordinary ordered pessimistic lock [ADR-0036](0036-ordered-pessimistic-row-locks.md)
  requires before writing a row whose condition was read — the same role the product lock plays in
  purchase creation, which is likewise not called a guard. The guard in this workflow is the
  purchaser row, and purchase creation above already records it.
  **Read this against the discontinuation above:** both write the coupon aggregate, and they land on
  opposite sides of the criterion. What separates them is not how many aggregates are touched, nor how
  important the operation is, but whether the target rows can be named — which is why the pair sits in
  one aggregate rather than in two unrelated features. Specified in `docs/spec/usecase/purchase.md`.
<!-- sample-api:replace-with -->
<!-- = No instance has been recorded for this project yet. -->
<!-- sample-api:replace-end -->

### Recording discipline

This classification depends on **requirements**, not on the structure of the operation. The
<!-- sample-api:replace-begin -->
same operation changes classification when requirements change (e.g., if "stock restored by
response time" becomes a requirement, withdrawal moves to CommandService).
<!-- sample-api:replace-with -->
<!-- = same operation changes classification when requirements change: a tolerance that used to be -->
<!-- = acceptable becoming a requirement moves the operation to CommandService. -->
<!-- sample-api:replace-end -->

Therefore, record the **tolerance judgment as the rationale**, not the conclusion alone. Both
questions are recorded, because an operation that answers them differently is a different operation:

<!-- sample-api:replace-begin -->
- Bad — "Withdrawal is a usecase."
- Good — "The withdrawal cascade tolerates eventual consistency; therefore it is a usecase."
- Bad — "Withdrawal locks the user row."
- Good — "A concurrent purchase would invalidate the withdrawal guard; therefore the user row is
  locked before the check."
- Good — "A product going unpublished cannot invalidate a purchase that already snapshotted its unit
  price; therefore that condition is left unguarded."
<!-- sample-api:replace-with -->
<!-- = - Bad — "<operation> is a usecase." -->
<!-- = - Good — "<operation>'s cascade tolerates eventual consistency; therefore it is a usecase." -->
<!-- = - Bad — "<operation> locks the <aggregate> row." -->
<!-- = - Good — "A concurrent <other operation> would invalidate <operation>'s guard; therefore the -->
<!-- =   <aggregate> row is locked before the check." -->
<!-- = - Good — "<other operation> cannot invalidate what <operation> already snapshotted; therefore -->
<!-- =   that condition is left unguarded." -->
<!-- sample-api:replace-end -->

## Consequences

### Positive Consequences

- Placement decisions shift from subjective interpretation of inter-aggregate semantics to
  verifiable requirement checks.
- The direction of default (decomposition) vs. exception (CommandService) is explicit,
  suppressing misuse at the policy level.
- The definition is symmetric with QueryService, so both sides of CQRS are explained in the
  same grammar.
- Because the rationale is recorded as a requirement, the places that need re-evaluation are
  identifiable when requirements change.
- A cross-aggregate read now has a branch of its own, so "it is only a read" can no longer route an
  unheld condition onto the default path by omission.
- Every widening of the transaction boundary is named and bounded, and its relation to the
  aggregate-as-consistency-boundary principle is stated rather than left for a reader to reconstruct
  from the code.

### Negative Consequences

- The criterion requires an explicit requirements judgment ("can this cascade tolerate
  eventual consistency?") before implementation; the answer cannot be derived from code
  structure alone.
- Classification is requirement-relative: when requirements change, an operation may need to
  migrate between usecase + outbox and CommandService.
- The atomicity judgment and its recorded rationale must be maintained by review; there is no
  compiler enforcement for the distinction.
- The default path presumes the outbox machinery (relay process, at-least-once delivery), so
  its eventual-consistency effects must be acceptable to downstream consumers.
- Branch 2 costs throughput on the guarded row: the operation now blocks for the duration of any
  concurrent writer of that row ([ADR-0036](0036-ordered-pessimistic-row-locks.md)).
- Three branches are harder to hold than two, and two of them produce the same construct (a regular
  usecase), so the presence of a lock — not the class of the file — is what distinguishes branch 2
  from branch 1 at a glance.

## Alternatives Considered

### Classification by derivability

Classifying by "whether the change to secondary aggregates is derivable as a consequence of
the primary operation." This inserts a subjective judgment about inter-aggregate semantics,
<!-- sample-api:replace-begin -->
so decisions vary by implementer. It also classifies purchase creation as a regular usecase
and withdrawal as a CommandService — the reverse of what the atomicity criterion yields —
evidence that the criterion is not grounded in requirements. Rejected.
<!-- sample-api:replace-with -->
<!-- = so decisions vary by implementer. It also inverts the classification the atomicity criterion -->
<!-- = yields on real operations — evidence that the criterion is not grounded in requirements. Rejected. -->
<!-- sample-api:replace-end -->

### Classifying every multi-aggregate write as CommandService

Simpler to decide, but the count of aggregates a write touches says nothing about whether the
write decomposes. Purchase creation touches three and still decomposes, because its target rows
are named by identity and can be locked; what forbids decomposition is a target set defined by a
predicate, which can be neither enumerated nor locked. Classifying by count would therefore admit
a CommandService wherever widening the boundary is convenient, and a boundary that widens on
convenience constrains nothing (§ Departure from "1 Aggregate = 1 Transaction Boundary"). Rejected.

### Treating a cross-aggregate read as needing no mechanism

Keeping the two-way procedure and letting every guard fall to the default path, on the grounds that
a read changes nothing. Rejected. It is the reading that produced the gap: it answers a question
about atomicity that a guard never asks, and leaves the guard deciding from a condition nothing
holds — the interleaving [ADR-0036](0036-ordered-pessimistic-row-locks.md) documents. It also makes
the two directions of one invariant look like unrelated operations, since only the writing side gets
a mechanism.

### Promoting a guarded operation to CommandService

Routing anything that touches a second aggregate's row — lock included — through a CommandService,
so that "crosses the boundary" and "is a CommandService" coincide. Rejected. It restates the
already-rejected "spans multiple aggregates, therefore CommandService" with the lock as the trigger,
and it misplaces the work: taking a lock is a transaction-boundary responsibility, which the usecase
already owns, and the condition the lock protects belongs to the domain
([ADR-0036](0036-ordered-pessimistic-row-locks.md) § 5), not to a write-optimization construct.

### Raising the isolation level instead of adding a branch

Running every transaction at SERIALIZABLE, so that a stale guard surfaces as a serialization failure
and no per-operation judgment is needed. Rejected here for the reason
[ADR-0036](0036-ordered-pessimistic-row-locks.md) records: the isolation level is a property of every
transaction in the application, and its retry-rate cost is not a decision any single invariant should
make.

### Introducing Saga / process manager

The general solution for multi-aggregate writes, but at this project's scale, outbox
choreography is sufficient and an orchestration layer does not justify its complexity.
Rejected for now; not excluded as a future evolution.

## Notes

- **Relationship to [ADR-0032](0032-lightweight-cqrs.md)**: complementary, not conflicting.
  ADR-0032 defines the construct's structure (interface placement, implementation directory,
  DTO returns, DI registration); this ADR supplies the placement criterion ADR-0032 left
  open. One clarification: ADR-0032's consequence that CommandService "can freely optimize
  flexible updates, deletes" describes what the construct may do once placed, not when to
  place it. Under this ADR, write-shape flexibility or performance alone is NOT sufficient
  grounds for a CommandService; the atomicity requirement is the sole criterion, and this ADR
  governs placement going forward. ADR-0032 is not edited or superseded.
- **Relationship to [ADR-0036](0036-ordered-pessimistic-row-locks.md)**: this ADR decides *whether* a
  cross-aggregate condition needs to be held; ADR-0036 decides *how* it is held — lock order,
  acquisition point, lock mode, and who is allowed to author the guarded condition. Branch 2 is the
  entry point into it, and nothing in branch 2 overrides it.
<!-- sample-api:replace-begin -->
- Which rows a given workflow locks, and which business rule each lock protects, is feature content
  rather than an architectural decision, so it is specified with the feature — in this repository the
  removable sample set (`docs/spec/usecase/purchase.md`, `docs/spec/usecase/user.md`), referenced by path rather than
  linked, because those files are deleted by `make setup-remove-sample-api` while this ADR stays.
<!-- sample-api:replace-with -->
<!-- = - Which rows a given workflow locks, and which business rule each lock protects, is feature content -->
<!-- =   rather than an architectural decision, so it is specified with the feature. -->
<!-- sample-api:replace-end -->
- Related: [ADR-0033](0033-system-cqrs-dml-category.md) (`system_cqrs` category that carries
  the outbox DML, outside the CQRS split); [ADR-0054](0054-transactional-outbox.md) (the
  outbox pattern, which addresses the dual-write anomaly between the database and an external
  endpoint — it is how a decomposed cascade is *delivered*, not what makes decomposition
  correct inside one database);
  [ADR-0035](0035-transaction-retry-idempotent-callers.md) (retry semantics of the single
  transaction an atomic write runs in).
