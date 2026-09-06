---
status: accepted
date: 2026-09-06
deciders: [maintainers]
tags: [outbox, architecture, testing, contracts]
---

# ADR-0113: Declare each outbox payload's kind, and check a snapshot's field correspondence

## Status

accepted

## Context

An outbox payload is written once, next to the usecase that emits it, and then read forever by
subscribers this repository does not own. The aggregate it is copied from keeps changing.

Nothing connected the two. A field added to an aggregate left every payload exactly as it was, and
no check in the repository looked at the pair. The failure that produced this record is the shape
the gap takes in practice: `purchase.created.v1` gained a discount to its aggregate but not to its
payload, so a subscriber adding the payload's own numbers got `subtotal + tax + shipping ≠ total`.
Every layer of review passed it — the unit test asserted the fields that were present, the reviewers
compared code against code, and one real HTTP request returned a correct response because the
response DTO was never wrong. It surfaced only by reading an `outbox` row directly.

The obvious rule does not work. Every payload's doc comment called itself a *self-contained
snapshot*, so the phrase could not separate the payloads that owed the aggregate's state from the
ones that did not. `purchase.paid.v1` carries five of the aggregate's seventeen fields and is
correct: it reports that a payment happened, to which purchase, and when. A check requiring the full
field set would have failed six of the eight payloads for being what they are supposed to be.

So the question is not "does this payload carry everything" but **what did this payload promise**,
and that is not derivable from the code. It is a decision the emitting layer makes — the same layer
that already owns the wire format, since the event's name belongs to the domain while its
representation does not (`internal/domain/README.md`, *Domain events*).

## Decision

**Every outbox payload declares its kind, and a `snapshot` additionally classifies every field of
the aggregate it copies.** The declaration lives in `payload_parity.yaml` beside the `Build*` that
produces the payload; `TestOutboxPayloadParity` reconciles it with the code.

| Kind | Promises | Obligation |
| --- | --- | --- |
| `snapshot` | the aggregate's state at the moment of the fact, so a subscriber need not call back | every field of the source struct is carried (named by its JSON key) or omitted (with a reason) |
| `notification` | what happened, when, and to which identifier | none at field level — the declaration itself records that this payload does not track the aggregate |

The check reads both directions of every set it reconciles: a package with a `Build*` and no
declaration fails, and a declaration with no `Build*` fails; a payload absent from the declaration
fails, and a declared payload that no longer exists fails; for a `snapshot`, an unclassified
aggregate field fails, and a classified field the aggregate no longer has fails.

**What is enforced is that a decision was recorded, not which decision it was.** Adding a field to
an aggregate turns red until someone writes down whether the payload carries it, and either answer
clears the check. That is the whole mechanism: the failure mode was never a wrong choice, it was a
choice nobody was asked to make.

A `snapshot` whose fields form a breakdown — amounts a subscriber is expected to add up — also pins
the arithmetic in its own unit test with every term non-zero. Field-level parity proves the terms
are present; only the arithmetic proves they still agree, which is the specific way
`purchase.created.v1` broke.

Two shapes are deliberately excluded. The check reads only the **top level** of the source struct,
so a decision inside a value object stays with that value object. And it is **one-directional**: a
payload may carry fields that come from elsewhere — another aggregate, a derived value — without
declaring them, because the risk this record addresses is the aggregate moving out from under the
payload, not the payload growing.

## Consequences

### Positive

- The failure that produced this record becomes a red test at the moment the aggregate changes,
  rather than a wrong number in a subscriber's ledger some time later.
- The two archetypes are now separable in the code. `notification` is a stated position rather than
  an absence, so a thin payload no longer has to be defended as an exception every time it is read.
- The declaration sits beside the code it describes and is exhaustive, so it fails when it goes
  stale. That is what distinguishes it from an allowlist, which is silent when it rots
  (`internal/architest/README.md`, *Notes*).
- A project that instantiates this template and removes the sample APIs keeps the gate. The
  declarations leave with the sample packages, the check tolerates zero subjects, and the first
  `Build*` the integrator writes fails until it is declared.

### Negative

- Adding a field to an aggregate now requires editing a second file. This is the intended cost —
  the edit *is* the decision being recorded — but it is a real one, and it lands on changes that
  have nothing to do with the outbox.
- The check is text scanning over gofmt-normalised sources, because `depguard` forbids `go/ast`
  here. Field shapes it cannot parse (embedded fields, multi-name declarations) are reported as
  unclassified rather than skipped, which is safe but noisy if such a shape is ever introduced
  deliberately.
- `payload_parity.yaml` is this repository's first non-Go declaration file living inside a Go
  package. It reads naturally beside the payload it describes, but it is a new precedent.

### Neutral

- Nothing about the wire format changes. Existing subscribers see the same payloads, except for
  `purchase.created.v1` gaining `orderedAt` — additive, and previously absent only because the
  emitting code passed an entity the database had not yet stamped.
- The word "self-contained" is retired from payload doc comments. It was never wrong so much as
  unable to distinguish anything.
