---
status: accepted
date: 2026-09-07
deciders: [maintainers]
tags: [persistence, cqrs, architecture]
---

# ADR-0114: Admit a write whose target rows are named only by a predicate to CommandService

## Status

accepted

## Context

[ADR-0034](0034-commandservice-atomicity-criterion.md) supplies the placement criterion for an
operation that crosses an aggregate boundary, and reserves CommandService for a multi-aggregate write
that requires single-transaction atomicity. Its worked instances state that product discontinuation is
the only operation reaching that branch.

An operation exists that the criterion cannot place.
<!-- sample-api:replace-begin -->
Issuing one promotional coupon to every user who has not withdrawn writes a single aggregate —
`coupons` — and nothing else. Nothing about it must be immediate, and there is no second aggregate
whose intermediate state an observer could catch, so the atomicity question answers no. It nonetheless
does not decompose: the recipients are fixed by a predicate over another aggregate's rows, so they
cannot be enumerated, cannot be locked, and have no upper bound. Expressing the write as
load-mutate-save would construct one `Coupon` per recipient and issue one round trip per recipient.
<!-- sample-api:replace-with -->
<!-- = A write touches one aggregate and nothing else, so no atomicity requirement arises, and it still -->
<!-- = does not decompose: its target rows are fixed by a predicate rather than named, so they cannot be -->
<!-- = enumerated, cannot be locked, and have no upper bound. Expressing it as load-mutate-save would -->
<!-- = construct one aggregate per row and issue one round trip per row. -->
<!-- sample-api:replace-end -->

Both gates in [`docs/design/data-access-pattern.md`](../design/data-access-pattern.md) §4 must pass for
a write to reach CommandService. Gate 1 passes: a set-based write is named there as exactly what
CommandService exists for. Gate 2 fails, because it asks only about multi-aggregate atomicity. The
write therefore belongs to neither construct, and the procedure returns no answer rather than a wrong
one.

The gap is in the document, not in the operation. §1 states the underlying fact — QueryService and
CommandService are the residue that remains when a non-functional requirement forbids decomposing an
operation into per-aggregate work — and §2 tabulates the four constructs on that single axis. A write
whose round trips grow with the population is squarely on the "decomposition is forbidden" side of
that axis. What is missing is a branch expressing it. The read side already has one: §3.3 admits a
QueryService when decomposition would materialize an aggregate the operation does not need.
<!-- sample-api:replace-begin -->
ADR-0034 itself calls the discontinuation bulk issuance "the write-side mirror of §3.3" — naming a
mirror that §4 never provided.

Discontinuation could carry a single branch 3 because it answers both questions yes: its rows are
predicate-defined *and* its write must be atomic with another aggregate's write. With only that
instance on record, the two reasons were indistinguishable, and the narrower one was written down.
<!-- sample-api:replace-with -->
<!-- = ADR-0034 already names a set write "the write-side mirror of §3.3" — naming a mirror that §4 -->
<!-- = never provided. A first instance that answers both questions yes hides the distinction, because -->
<!-- = either reason alone explains it; the narrower one is then the one written down. -->
<!-- sample-api:replace-end -->

## Decision

**A write reaches CommandService when its target rows cannot be named by identity, independently of
whether it crosses an aggregate boundary.**

`docs/design/data-access-pattern.md` §4 Gate 2 splits branch 3 into two paths, either of which admits
an operation on its own:

- **3a — multi-aggregate atomicity.** Single-transaction atomicity of a multi-aggregate write remains a
  requirement. Unchanged from ADR-0034.
- **3b — predicate-defined set write.** The rows to be written are fixed only by a predicate, so they
  can be neither enumerated nor locked and have no upper bound, and decomposition would make round
  trips grow with the population.

The test for 3b is whether the rows can be **named**, not how many there are. A thousand rows named by
identity still decompose; three rows chosen by a predicate do not.

**3b is not a widening of the transaction boundary.** The write it admits may touch one aggregate only.
The two named widenings ADR-0034 records — the guard (branch 2) and multi-aggregate atomicity (3a) —
remain exactly two. The consequence is that the presence of a CommandService no longer tells a reader
that a boundary was crossed; branch 3a does.

Gate 1 is unchanged and still composes: a write that can be expressed as load-mutate-save belongs on
the Repository whatever else is true of it.

## Consequences

### Positive Consequences

- An operation whose rows are predicate-defined has a placement, decided by a stated criterion rather
  than by implementer judgment about which existing construct is least wrong.
- The write side gains the branch that mirrors §3.3, so the read and write halves of the document are
  now symmetric on the axis §1 declares.
- The two reasons discontinuation reaches CommandService are separated, so a later operation answering
  only one of them is placed correctly instead of being read against a precedent that answers both.
- Widening the transaction boundary and being unable to name the rows are now distinguishable in review;
  previously both surfaced as "it is a CommandService".

### Negative Consequences

- The decision procedure has one more branch, and two of its paths produce the same construct for
  different reasons — so the construct alone no longer identifies why an operation was placed there.
  The recorded rationale ADR-0034 § Recording discipline already requires is what carries the
  difference, and it now has to be read rather than inferred.
- 3b admits a CommandService without any atomicity requirement, which removes atomicity as the single
  story for why the construct exists. A reviewer who learned the criterion as "CommandService means
  atomic" has to relearn it.
- The boundary of 3b rests on "cannot be named by identity", which is a property of the operation's
  requirements rather than of its code, so it is not mechanically checkable — the same limitation
  ADR-0034 records for its own criterion.

## Alternatives Considered

### Widening Gate 2 to atomicity in general

Restating Gate 2 as "does the write require atomicity" rather than "does the multi-aggregate write
require single-transaction atomicity", so that a single-aggregate set write passes on the grounds that
its statements must commit together. Rejected. Every write in this application already runs in a
transaction, so an atomicity test that does not say *across what* admits everything, and Gate 2 would
stop excluding anything. It also duplicates Gate 1: the relative updates and set operations Gate 1
routes to CommandService would then be re-argued under Gate 2 in different words.

### Admitting the set write to the Repository

Relaxing Gate 1 so a Repository method may perform a predicate-defined set write. Rejected on two
counts. It contradicts the sentence in Gate 1 that names set-based operations as what CommandService
exists for, and Gate 1 exists precisely to stop the seam degrading into "where I put SQL I want to write
directly". Independently, the predicate reaches another aggregate's rows, so the query violates
[`docs/rules.md`](../rules.md) § Repository / QueryService Rules — "Writing join / aggregation queries
across *independent* Aggregates in Repository", whose only exemption is a uniquely-determined JOIN to a
context-nested reference master.

### Bundling the write with a second aggregate so 3a applies

Requiring that a set write always accompany a write to a companion aggregate, so the operation becomes
multi-aggregate and passes the existing Gate 2 unchanged. Rejected as a *placement* answer. It leaves
the criterion unable to place the operation and instead requires every future single-aggregate set
write to invent a companion aggregate, which is the convenience-driven boundary widening ADR-0034
§ Departure refuses. Such a companion may still be worth building for its own reasons — it can supply
a natural re-entry guard and an audit trail — but building one to satisfy a gate is the wrong order.

### Leaving the gap unrecorded

Implementing the operation on a CommandService and noting in passing that the procedure does not cover
it. Rejected. An undocumented exception is indistinguishable from a misplacement at review time, and
the next operation of this shape would re-derive the argument from scratch or land somewhere else.

## Notes

- The criterion and its procedure are stated once, in
  [`docs/design/data-access-pattern.md`](../design/data-access-pattern.md) §4; this ADR records the
  decision and the options weighed. Per [`docs/rules.md`](../rules.md) § Repository / QueryService
  Rules, the criterion is not restated here.
- [ADR-0034](0034-commandservice-atomicity-criterion.md) is amended rather than superseded: branch 3
  becomes 3a, its Criterion gains the third question, and its worked instances record both the
  discontinuation (3a, which also answers 3b) and the promotional issuance (3b alone). Its decision —
  that atomicity across aggregates admits a write to CommandService — is unchanged.
<!-- sample-api:replace-begin -->
- The two coupon issuance journeys are being built concurrently, and the placement of each depends on
  whether its rows can be named. Once they have settled, this record is expected to be folded back into
  ADR-0034 so that a project created from this repository reads one criterion rather than a criterion
  and its later amendment.
<!-- sample-api:replace-with -->
<!-- sample-api:replace-end -->
- Related: [ADR-0032](0032-lightweight-cqrs.md) (the CommandService construct and the rule that the
  conditions it enforces are authored by the domain);
  [ADR-0037](0037-uuidv7-identifiers.md) (key generation in the domain, which is why a set insert takes
  two statements without that being a departure).
