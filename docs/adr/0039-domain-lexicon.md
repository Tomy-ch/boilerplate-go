---
status: accepted
date: 2026-07-24
deciders: [maintainers]
tags: [architecture]
---

# ADR-0039: Cross-aggregate value objects live in a curated domain lexicon

## Status

accepted

## Context

<!-- sample-api:replace-begin -->
Introducing `money.Price` ([ADR-0038](0038-two-scale-quantity-model.md)) surfaced a gap in the
layer rules. `Price` is a business-semantic value object (non-negativity, minor-unit conversion)
shared by **more than one aggregate**.
<!-- sample-api:replace-with -->
<!-- = Introducing a settlement-scale value object ([ADR-0038](0038-two-scale-quantity-model.md)) -->
<!-- = surfaced a gap in the layer rules. It carries business semantics of its own and is shared by -->
<!-- = **more than one aggregate**. -->
<!-- sample-api:replace-end -->
<!-- 撤去後にこの箇所へ自分の例を置くための指針。
     目的: 「複数の集約から使われる」が抽象のままだと、入場基準を満たす実例が示せない。
     意義: 効くのは利用者が 2 つ以上あることで、型そのものの複雑さではない。
     書き方: その値オブジェクトを使う集約側のフィールドを 2 つ以上挙げる。 -->
<!-- sample-api:begin -->
サンプルでの利用者は `product.price` と `purchase_details.unit_price` / `purchases.*_amount`。
<!-- sample-api:end -->
It therefore cannot live in a single
aggregate package (that would force other aggregates to reach into it, or duplicate the VO), and
it cannot live in `pkg/` (which forbids business logic and must stay context-independent).

The existing rule — "the domain layer's only permitted `internal/` dependency is
`internal/apperror`" — did not anticipate a value object shared *across domain aggregates*.
Read literally it forbids the natural placement; and the depguard rule was `lax` about
domain→domain, so it silently permitted *any* cross-aggregate import (one aggregate reaching
directly into another),
a latent coupling hole. A decision is needed on where such shared value objects live and how the
boundary is enforced.

## Decision

Cross-aggregate, business-semantic value objects live in a **curated domain lexicon** at
`internal/domain/lexicon/`. Other domain packages **may** import it; the lexicon itself depends only
on `pkg/**` and `internal/apperror`, never on an aggregate.

The name carries the admission question: what belongs there is a **word of the business**, not merely
something used twice. Admission is deliberately narrow — a type belongs there only when **all** hold:

- it is a **value object / domain concept**, not an aggregate or a service tied to one aggregate;
- it is genuinely used by **two or more aggregates** (or is imminently so) — not "might be reused someday";
- it carries **business semantics** that bar it from `pkg/` (currency, non-negativity,
  minor-unit, tax rules, …);
- adding it is a **jointly-owned** decision — a change here ripples to every aggregate that
  depends on it, so it is made conservatively.

The boundary is enforced by depguard (`maintain_a_sound_domain`): `internal/domain/` is denied
for domain files, with an explicit allow for `internal/domain/lexicon`. So domain→lexicon is
permitted while domain→other-aggregate is now forbidden (closing the prior lax hole).

Placement is resolved **`pkg/` first**: its bar is machine-enforced (depguard `independent_pkg`) while
this one is prose, and a prose bar cannot push a type across a boundary a linter draws. Failing `pkg/`
is not an argument for the lexicon — the lexicon asks its own questions, and treating it as the
fallback would make `pkg/` the junk drawer instead. A type that clears neither stays in its aggregate.

## Consequences

### Positive Consequences

- The path itself signals intent: `internal/domain/lexicon/<vo>` reads as "shared, importable",
  so a cross-aggregate import no longer looks like a violation (the review confusion that
  prompted this ADR).
- depguard now both **permits** the lexicon and **forbids** ad-hoc aggregate-to-aggregate
  coupling — a stricter, clearer boundary than the previous lax domain→domain.
- `money` semantics live once, in the domain, reused across aggregates without
  duplication and without leaking business logic into `pkg/`.

### Negative Consequences

- A shared lexicon is a coupling point: a change to a lexicon package can affect every dependent
  aggregate, so it must be evolved conservatively (this is the cost the admission bar manages).
- One more placement concept for contributors to learn (aggregate vs. lexicon), documented here,
  in `docs/rules.md`, and in `internal/domain/lexicon/README.md`.

## Alternatives Considered

### Keep `money` as a flat `internal/domain/money` aggregate-level package

Rejected: its path is indistinguishable from an aggregate, so the domain→domain import keeps
reading as a violation, and depguard cannot cleanly allow it without either enumerating each
shared package (brittle) or re-opening domain→domain entirely (the lax hole).

### A generic `internal/domain/shared` (or `common` / `util`) package

Rejected: the deciding factor is **what question the name asks at the door**. `shared` / `common` /
`util` ask "is this used in more than one place?", which anything reused can answer yes to — so the
package fills with unrelated code and becomes a junk drawer. `lexicon` asks "is this a word of the
business?", which a generic helper cannot answer yes to. The strict criteria above then hold the line,
but the name is what makes them stick.

### `internal/domain/kernel`, naming the DDD *Shared Kernel*

Rejected on reflection (this ADR originally chose it). In Evans, Shared Kernel is a **Context Map
relationship**: a model subset that two Bounded Contexts jointly own, which presupposes more than one
Bounded Context. This repository has a single model and shares across *aggregates*, so the premise
does not hold and the term would be claimed under a meaning it does not have. The discipline the term
carries — keep it small, change it as a joint decision — is retained above on its own terms, and
`kernel` is left free for whoever actually introduces that structure.

### Put `money` in `pkg/`

Rejected: `pkg/` forbids business logic and must be context-independent. Currency / non-negative
/ minor-unit are domain semantics; only the generic decimal container (`pkg/decimal`,
[ADR-0038](0038-two-scale-quantity-model.md)) belongs in `pkg/`.

## Notes

<!-- sample-api:replace-begin -->
- Lexicon package: `internal/domain/lexicon/money` (`Price`).
<!-- sample-api:replace-with -->
<!-- = - Lexicon package: none admitted yet; `internal/domain/lexicon/` holds only its admission bar. -->
<!-- sample-api:replace-end -->
- Enforcement: depguard `maintain_a_sound_domain` in `.golangci.yaml` (deny
  `internal/domain/`, allow `internal/domain/lexicon`).
- Admission bar: `internal/domain/lexicon/README.md`; layer rule: `docs/rules.md`.
- Two-scale quantity model this enables: [ADR-0038](0038-two-scale-quantity-model.md).
