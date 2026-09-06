# Data Access Pattern

## Role

This document is the single place the **placement criterion** for a data access lives: given an
operation, which construct does it belong to, and why.

The decisions are recorded in the ADRs —
[ADR-0032 (lightweight-cqrs)](../adr/0032-lightweight-cqrs.md) (the constructs, their structure, the
CommandService eligibility and derivation rules),
[ADR-0033 (system-cqrs-dml-category)](../adr/0033-system-cqrs-dml-category.md) (the fourth category
outside the split), and
[ADR-0034 (commandservice-atomicity-criterion)](../adr/0034-commandservice-atomicity-criterion.md)
(the cross-aggregate decision procedure). Those record *what was decided and what was rejected*; this
document states *how to decide a given case*, and is the one that grows as the criterion is sharpened.
`docs/rules.md` § Repository / QueryService Rules carries only the enforceable prohibitions and links
here.

## 1. The underlying fact

> Given unlimited latency and resources, every read decomposes into per-aggregate Repository reads
> joined in application code, and every write decomposes into a single-aggregate write followed by an
> eventually consistent cascade. QueryService and CommandService are needed only where non-functional
> requirements forbid that decomposition.

There is **one axis** — whether a non-functional requirement forbids decomposing the operation into
per-aggregate work — and the direction of the operation decides which construct the residue lands on.

Neither QueryService nor CommandService is a semantic domain category. Both are the residue that
remains when decomposition is not available. Latency and resource cost are exactly the requirements
that remove it; what is *not* a justification is optimization with no requirement behind it (§7).

## 2. The four constructs

| direction | decomposition is available | decomposition is forbidden |
| --- | --- | --- |
| **read** | Repository | QueryService |
| **write** | Repository | CommandService |

Repository is the default in both directions. It owns the aggregate's system-of-record state:
persistence, fetch by ID, and simple filter / list / count over the aggregate's own attributes. On the
write side "decomposition is available" also covers a regular usecase composed of several Repository
calls — see §5, which is a branch of its own.

**`system_cqrs` sits outside this table entirely.** Health verification, idempotency enforcement and
outbox delivery are infrastructure-operational queries: not driven by a user-facing use case, with no
aggregate owner and no usecase interface. They are not a harder case of the read/write question — they
are not in the question. Placing them in `repository/` would break its 1:1 correspondence with domain
aggregates. See [ADR-0033](../adr/0033-system-cqrs-dml-category.md); DML lives in
`database/dml/system_cqrs/` and implementations in `internal/infrastructure/rdb/system_cqrs/`.

## 3. Read side: when is decomposition forbidden

Ask these in order. The first that applies decides.

### 3.1 Is the aggregate even the source of record for what is being read?

If the queried data is a **derived projection** — a search index, a separate search store, a
denormalized column maintained as its own read surface, a materialized cross-aggregate view — the
aggregate cannot be reconstructed from it, so decomposition is not merely expensive but impossible.
**QueryService.**

The clean image is a system of record in PostgreSQL with a projection in Elasticsearch. What decides is
the **relationship between the record and its projection**, not the storage engine: PostgreSQL does not
imply Repository and a document store does not imply QueryService.

Full-text search is a *typical* QueryService case because it usually rides on such a projection — not
because "search" is inherently a read-side concern. A plain filter over the aggregate's own columns
stays in Repository, and so does a filter that uses a generated column of the same row as an index
while still returning the full aggregate.

### 3.2 Does the read span independent aggregates?

Two shapes, landing differently.

- **Attaching a field** from another independent aggregate (a name, a label) — batch-fetch that
  aggregate through its own Repository (`FindByIDs`) and merge by key **in the usecase layer**. Each
  read stays a single-aggregate Repository read. **Not** a QueryService case.
- **Reading the joined shape itself** — a view or an aggregation whose rows belong to no single
  aggregate. Decomposition would mean materializing every participating aggregate to discard most of
  it. **QueryService.**

**Reference masters are not independent aggregates.** Fixed lookup data with no independent write or
transactional lifecycle, reached through a mandatory and uniquely-determined FK, is part of the owning
aggregate's semantic set. JOINing it to project a display attribute is a single-aggregate Repository
read — no QueryService, no usecase merge. The criterion is the joined data's *nature*, not whether it
happens to have a Go type of its own — a master resolved purely by JOIN with no domain type
of its own qualifies just as one that also has a sub-package.

<!-- 撤去後にこの箇所へ自分の例を置くための指針。
     目的: 「参照マスタ」だけでは読者が自分のテーブルを当てはめられない。
     意義: 判定は独立した書き込みライフサイクルの有無で決まり、テーブルの名前や規模ではない。
     書き方: 列挙型に相当する固定テーブルを 1〜2 個挙げる。 -->
<!-- sample-api:begin -->
（サンプルでの例は `purchase_statuses` / `product_statuses`）
<!-- sample-api:end -->

### 3.3 Would decomposition materialize an aggregate the operation does not need?

A subset or reshape of a **heavy** aggregate — the read needs a few fields and reconstructing the whole
aggregate discards the rest — is a QueryService case even though nothing crosses an aggregate boundary
and nothing is denormalized.

**This branch has no threshold, deliberately.** What counts as "heavy enough" depends on the shape of
the aggregate and on the operational reality of the system being built, so this document sets no number.
What it does fix is the axis you weigh on and the default:

| weigh | against |
| --- | --- |
| the cost of materializing what the operation discards — field count × row count, JOIN depth, how many aggregates get expanded | the cost of a second read path — duplicated SQL, duplicated DTOs, another surface to keep in sync and to test |

**The default is Repository.** Use this branch only when the left side clearly outweighs the right.
Stating the default matters: without one, every judgment call resolves toward adding a QueryService,
because adding a path is always the nearer move.

## 4. Write side

A write reaches CommandService only by passing **both**. They ask different questions and neither
implies the other.

### Gate 1 — Eligibility: can the write be expressed as load-mutate-save?

If the write can be expressed as loading an aggregate, mutating it, and saving it, it belongs on the
**Repository**, regardless of anything else.

CommandService exists for the writes that *cannot* be expressed that way without changing their
concurrency properties: **relative updates, set-based operations, and operations that obtain atomicity
without taking a lock**. Returning a reserved quantity when a reservation is released is the shape — a
relative update that takes no lock on the counted row at all; expressing it as load-add-save would
introduce a lock the cancel path does not take, adding contention and a deadlock surface that does not
exist today.

Without this gate the seam degrades into "where I put SQL I want to write directly".

### Gate 2 — Atomicity: does the multi-aggregate write require a single transaction?

For an operation that crosses an aggregate boundary, two independent questions decide. They are
independent because one is about reading and the other about writing, and one operation may answer yes
to both.

1. Does a condition read from another aggregate have to **hold for the rest of the transaction**? — can
   a concurrent operation invalidate it between the check and the commit?
2. Does the multi-aggregate write require **single-transaction atomicity**? Immediacy — all effects
   visible at API response time — is the typical reason this arises.

The procedure:

1. **Decomposition (default).** If the consequence for the other aggregate may be eventually consistent
   and a condition read from it may go stale, implement it as a regular usecase and propagate the
   consequence as an outbox event ([ADR-0054](../adr/0054-transactional-outbox.md)). No other aggregate
   is held inside the transaction.
2. **Guard (synchronous row lock; still a regular usecase).** See §5.
3. **Atomicity (CommandService; exception, must be justified).** Only when single-transaction atomicity
   of the multi-aggregate *write* remains as a requirement.

Two justifications are not acceptable: "it spans multiple aggregates, therefore CommandService", and
"it is only a read, therefore nothing is needed".

### The Repository write contract

The gates decide *where* a write goes. This decides what a write method that landed on the Repository
has to guarantee.

> **A Repository write method must leave the aggregate consistent on its own. It must not require
> another Repository call.**

Concretely: if a doc comment says "call B after A, in the same transaction", then **A and B are one
method**. The criterion is not how many methods an aggregate has — it is what each method's contract
guarantees. An ordering obligation written in prose does not stop the next caller; only a method that
cannot be half-invoked does.

That makes the rule mechanically checkable: search write-method doc comments for an ordering
obligation imposed on the caller. Each one is a split that has to be folded back.

Three consequences follow, and the third is where the rule is most often misread.

1. **A child collection is synchronized inside the parent's write**, keyed by identity — remove the
   rows the aggregate no longer holds, add the ones it does not have yet. Any ordering the storage
   engine forces (a partial unique index that must be cleared before a key is reused, for instance) is
   an invariant *of the method*, not an instruction for the caller.
2. **Delete-everything-then-reinsert is not synchronization.** It changes the identity of every child
   on every write, so an update that touched nothing else still rewrites the whole collection.
3. **Several write methods per aggregate are fine** when each is consistent on its own — a
   single-column update, or one method per state transition. Folding those together buys no
   guarantee and costs write amplification, which is the opposite of what this rule is for. What is
   forbidden is a split that hands the caller an ordering obligation, not a split as such.

<!-- 撤去後にこの箇所へ自分の例を置くための指針。
     目的: 「単独で整合させる」だけでは、どの分割が適法でどれが違反なのか読者が自分の Repository を
     当てはめられない。適合と違反の両方が要る。
     書き方: 適合例は「単独で完結する部分更新」と「状態遷移ごとのメソッド」を 1 つずつ、違反例は
     「doc コメントが呼び出し順序を課していた分割」を 1 つ、いずれもメソッド名で挙げる。 -->
<!-- sample-api:begin -->
Examples in the sample set: `product.UpdateStock` (a single-column update that is consistent on its
own) and `purchase.UpdatePaid` / `UpdateShipped` / `UpdateDelivered` (one method per state transition)
are compliant. `product.Update` writes its images in the same call, and `cart.Update` its items —
both by keyed synchronization. The shape this rule rejects would be splitting that product write in
two: an `Update` that excludes the images, plus a separate `ReplaceImages` the caller has to invoke
afterwards in the same transaction.
<!-- sample-api:end -->

## 5. The guard branch

Branch 2 above is the one shape that does not appear in the §2 table. It crosses an aggregate boundary,
it is decided by a non-functional requirement, and it still lands on a regular usecase.

If a condition read from another aggregate must hold until commit, take a row lock before evaluating it,
in the global lock order ([ADR-0036](../adr/0036-ordered-pessimistic-row-locks.md)). Where the rule
spans aggregates it lives in a Domain Service. This buys immediate consistency for a **read**; it makes
no write atomic and is **never on its own a reason to introduce a CommandService**.

The branch exists because an operation that only *reads* another aggregate never triggers a
write-atomicity criterion, so without a branch of its own it would reach the default **by omission
rather than by decision** — and under READ COMMITTED a condition that was merely read is not held. "It
spans aggregates but it is only a read, so nothing is needed" is a true sentence with the right
conclusion and an incomplete answer.

## 6. Derivation: what a CommandService may enforce

Any condition a CommandService enforces must be **derived from a domain invariant, never authored
independently**. A guard in a decrement statement restates the domain's own sufficiency rule as
a fail-closed second net; it is downstream of that rule, so a change to the domain rule obliges a change
here and never the reverse. Two independently written copies of one rule diverge silently the first time
only one of them moves.

This is why a CommandService and a Domain Service are different things even when they concern the same
rule: **a Domain Service is a rule and owns no transaction; a CommandService is a transaction tool** and
is owned by whoever opens the transaction.

Two shape rules follow from the same reasoning:

- A CommandService method **receives the decided aggregate** — `CreateX(ctx, *x.X)` — symmetric to how a
  Repository returns one, rather than a decomposed parameter bag that scatters the write intent across a
  signature. This is not a DTO-boundary violation: that rule targets what the *controller* is exposed
  to, not what infrastructure receives.
- After the write, the usecase **calls back through the Repository** for the affected aggregate to
  validate correctness.

## 7. What is not a criterion

Each of these has been mistaken for the criterion at least once. None decides anything.

- **Returning a DTO.** Every read is eventually mapped to a response DTO. What matters is whether the
  operation needed the aggregate, not the shape it is returned in.
- **Being a "search".** §3.1 — the projection decides, not the word.
- **Spanning more than one aggregate.** §3.2 and §4 — span alone pulls work that could be eventually
  consistent into synchronous transactions.
- **Write-shape flexibility.** "CommandService can optimize flexible updates and deletes" describes what
  the seam happens to permit, not what admits an operation to it. Gate 1 does.
- **The storage engine.** §3.1.
- **Returning many rows.** Volume is not crossing; a thousand aggregates are still aggregates.
  Listing every row of one table is a single-aggregate `list` and stays in Repository.
  <!-- 撤去後にこの箇所へ自分の例を置くための指針。
       目的: 行数の多さが集約横断の証拠だと読まれやすいため、単一テーブル全件の具体例が要る。
       意義: 効くのは「1 つのテーブルに閉じていること」であって、返る行数ではない。
       書き方: 全件取得が自然な参照マスタを 1 つ選び、SELECT 文の形で示す。 -->
  <!-- sample-api:begin -->
  Example: `SELECT * FROM prefectures ORDER BY code`.
  <!-- sample-api:end -->

## 8. Where each construct lives

| construct | interface | implementation | DML |
| --- | --- | --- | --- |
| Repository | domain — `internal/domain/<aggregate>/` | `internal/infrastructure/rdb/repository/<aggregate>/` | `database/dml/repository/` |
| QueryService | usecase — `internal/usecase/<aggregate>/query/` | `internal/infrastructure/rdb/query_service/<aggregate>/` | `database/dml/query_service/` |
| CommandService | usecase — `internal/usecase/<workflow>/command/` | `internal/infrastructure/rdb/command_service/<aggregate>/` | `database/dml/command_service/` |
| system_cqrs | — (no usecase interface) | `internal/infrastructure/rdb/system_cqrs/` | `database/dml/system_cqrs/` |

Both Service interfaces live in the usecase layer rather than the domain, because the read model and the
atomic write shape are usecase concerns, not aggregate invariants — so the domain layer keeps exactly one
persistence contract, the Repository.

**CommandService ownership is decided on the workflow axis, not the aggregate axis.** One real write can
enforce the invariants of several aggregates at once, so "the aggregate whose invariant it enforces" is a
relation, not a function. A transaction always has exactly one initiator, and the usecase layer already
owns transaction boundaries — so `internal/usecase/<workflow>/command/` names a workflow, and "why this
aggregate and not the other one it also writes?" does not arise.

All of these are registered in `persistenceModule` (`internal/di/module/persistence.go`) and injected via
Fx. The `command_service` sub-module **may legitimately hold zero providers**: a system with no
cross-aggregate write of its own has nothing to register, and an empty sub-module is not a defect.

## 9. Departure from "1 Aggregate = 1 Transaction Boundary"

The guard branch (§5) and CommandService (§4 gate 2, branch 3) both put rows belonging to more than one
aggregate inside a single transaction, departing from the principle
[`internal/domain/README.md`](../../internal/domain/README.md) (§ Aggregate Boundary) states. Exactly
those two widenings are admitted, and no others; the reasoning is recorded in
[ADR-0034](../adr/0034-commandservice-atomicity-criterion.md) § Departure from "1 Aggregate = 1
Transaction Boundary".
