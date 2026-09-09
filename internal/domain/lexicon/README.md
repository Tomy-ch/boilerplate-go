# lexicon

The domain's shared **lexicon**: a deliberately small set of business-semantic value objects used by
more than one aggregate. Other `internal/domain/**` packages may import `lexicon/`; `lexicon/` itself
depends only on `pkg/**` and `internal/apperror`, never on an aggregate.

The name states the entry question. What gets in is a **word of the business** — not merely something
used in two places. A name whose only bar is "reused" attracts unrelated code and turns into a junk
drawer; asking whether a type is part of the business vocabulary keeps that out by construction.

> `kernel` is deliberately **not** used here. In Evans it names a Context Map relationship — a model
> subset two Bounded Contexts jointly own — which presupposes more than one Bounded Context. This
> repository has a single model, so the term is reserved for whoever actually introduces that
> structure.

## Why this exists

A value object that more than one aggregate speaks in belongs here.
<!-- 撤去後にこの箇所へ自分の例を置くための指針。
     目的: 入場基準（複数集約から使われること）を満たす実例が無いと、何を入れてよいか判断できない。
     意義: 効くのは利用者が 2 つ以上あることで、型の複雑さではない。
     書き方: その値オブジェクトを使う集約側のフィールドを 2 つ以上挙げる。 -->
<!-- sample-api:begin -->
サンプルでの利用者は `product.price` と `purchase_details.unit_price` / `purchases.*_amount`。
<!-- sample-api:end -->
It cannot live in one aggregate (the
others would have to reach in), and it cannot live in `pkg/` (which forbids business logic). So it
lives here — see [ADR-0039 (domain-lexicon)](../../../docs/adr/0039-domain-lexicon.md).

## Where a type goes

Resolve placement in this order. **`pkg/` is decided first**, because its bar is machine-enforced
(depguard `independent_pkg`) while this one is prose, and a prose bar cannot push a type across a
boundary a linter draws.

1. **Does it meet `pkg/`'s bar?** The authority is [`pkg/README.md`](../../../pkg/README.md) —
   its Policy and its "`pkg/` vs application-wide cross-cutting concerns" section. Do not restate
   "generic" here; two definitions drift, and the gap between them becomes the dumping ground.
   → yes: `pkg/`
2. **If not, does it clear every one of the bars below?** → yes: `lexicon/`
3. **Otherwise** keep it in its owning aggregate.

Failing step 1 is **not** an argument for step 2. `lexicon/` is not where `pkg/`-rejects land; it is a
separate gate that asks its own questions. Treating it as a fallback is what would turn `pkg/` into
the junk drawer instead.

## Admission bar

Add a package here **only when all** hold:

- it is a **value object / domain concept**, not an aggregate or an aggregate-specific service;
- it is genuinely used by **two or more aggregates** (or imminently so) — not "might be reused";
- it carries **business semantics** that bar it from `pkg/` (currency, non-negativity, minor-unit,
  tax, …);
- adding it is a **jointly-owned** decision — a change here ripples to every dependent aggregate, so
  evolve it conservatively.

## Enforcement

depguard (`maintain_a_sound_domain` in `.golangci.yaml`) denies `internal/domain/` for domain
files but allows `internal/domain/lexicon`. So domain→lexicon is permitted while
domain→other-aggregate is forbidden.

## Packages

<!-- 撤去後にこの節へ自分の語を並べるための指針。
     目的: 占有者の一覧が無いと、入場基準を満たした語が実際にどう見えるのか分からない。
     意義: 1 行に「語の名前 — 何を保証する値オブジェクトか」が要る。所有する不変条件が書けない語は
           そもそも入場基準を満たしていない。
     書き方: `- <package> — <型> 値オブジェクト（<保証する不変条件>）。` の形で 1 語 1 行。 -->
<!-- sample-api:begin -->
- `money` — `Price` value object (non-negative price-scale decimal; owns minor-unit conversion).
  The exact decimal container is `pkg/decimal` ([ADR-0038 (two-scale-quantity-model)](../../../docs/adr/0038-two-scale-quantity-model.md)).
<!-- sample-api:end -->

**サンプル撤去後、この節は空になります。** 器と入場基準だけが残り、最初の語を待ちます。
