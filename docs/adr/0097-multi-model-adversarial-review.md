---
status: accepted
date: 2026-07-30
deciders: [maintainers]
tags: [process, ai, review]
---

# ADR-0097: Use multi-model adversarial review with finder and verifier subagents

## Status

accepted

## Context

A code review performed by the same AI model that wrote the code has a structural blind
spot: the reviewer carries the same biases, assumptions, and errors as the implementer.
Reviews that only flag what the implementer already considered provide low marginal value.

A review workflow is needed that:

1. Uses a different model than the implementer so blind spots differ between the two.
2. Reduces false positives — a finding that sounds real but is refuted by context should
   not reach the report.
3. Covers runtime behavior (DI wiring, authentication middleware, real DB) that unit tests
   with mocked dependencies cannot exercise.
4. Distinguishes code correctness from comment quality, which requires different review
   criteria and different resolution actions.

A related question surfaced later, from a different direction. Some properties this project
wants reviewed are not decidable by a linter at all — whether a type can only be built through
its validated constructor, whether a design document has interpreted a domain concept under
any wording, whether a divergence from an external standard was declared on purpose. Go's
package-level encapsulation in particular means several of these have no deterministic form:
there is no class-private, so "constructed only via the factory" cannot be checked the way
"this field is exported" can.

Such checks can be approximated by heuristic static analysis, but the approximation is
unreliable at a rate the tool cannot report. Wiring it into CI would make a merge gate depend
on a guess; making it an opt-in linter would not improve the guess, only move the cost of
being wrong onto whoever opted in. The allocation of checks between deterministic tooling and
probabilistic review therefore needs to be a decision, not an accident of what was easy to
build.

## Decision

The `impl-review` skill implements a multi-model adversarial review in the
**finder → verifier** shape:

**Finder stage (concurrent):** Four code lenses (`correctness`, `security`, `architecture`,
`runtime-gap`) each run as an independent `adversarial-reviewer` subagent. A dedicated
`comment-reviewer` subagent covers comment quality. All finders run concurrently. The
orchestrator enforces that reviewer agents run on a different model than the implementer —
the reviewer agents default to `sonnet`, and the orchestrator overrides them when the
session model would otherwise match.

**Verifier stage (concurrent):** Each surviving finding is independently verified by a
`review-verifier` subagent, which re-derives its conclusion from the source code rather than
trusting the finder. Findings are classified `CONFIRMED`, `PLAUSIBLE`, or `REFUTED`.
Refuted findings are dropped before the report.

**Runtime stage (orchestrator):** When a touched endpoint is detected, the orchestrator
performs a live `curl` and observability log check against the running application. This
exercises the Fx DI graph, HTTP middleware (authentication, OpenAPI validation), and the
real database — none of which mocked unit tests reach.

**Comment fix stage:** Confirmed comment quality findings are applied to the working tree by
the orchestrator (not a subagent) after user confirmation, followed by `make go-fix` and
`make go-lint`. The four code lenses are read-only; their surviving findings are posted to the
branch PR as inline review comments by default.

**Allocation between deterministic and probabilistic checks.** A check is placed by whether it
is decidable, not by how valuable it is:

| Decidable from the syntax tree | Requires reading comprehension |
| --- | --- |
| golangci-lint / depguard / custom analyzers, CI-gating | review agents in the finder → verifier shape, never CI-gating |

Nothing straddles the line. A property that can be decided is decided by tooling and gates the
merge; a property that cannot is reviewed by an agent that reports and does not block. Heuristic
static analysis that approximates an undecidable property is not adopted in either position — the
finder → verifier shape above exists precisely because a probabilistic judgment needs an
independent check before a human reads it, and a linter has no equivalent.

The same shape carries checks whose yardstick is external to the repository — `ddd-audit` and
its `ddd-origin-auditor` compare the project's design documents against Evans's DDD patterns,
with the pattern ledger at `.agents/ddd-audit/pattern-ledger.yaml` recording which patterns have
been interpreted and where. Because the yardstick is a book the model cannot open, those agents
must state the premise they judged against so a reader can refute it, and they emit flags rather
than verdicts: this project claims DDD alignment, not Evans-strict compliance, so whether a
divergence is a deliberate design choice or an oversight stays a maintainer decision.

## Consequences

### Positive Consequences

- Bias reduction: reviewer and implementer run on different models, so the review surfaces
  findings the implementer's own model would miss.
- False-positive control: the verifier independently re-derives each finding from the code;
  plausible-but-wrong findings are classified `REFUTED` and dropped before the final report.
- Runtime coverage: the curl and observability stage catches DI misconfiguration, missing
  `security:` declarations, and SQL behavioral gaps that mocked tests do not exercise.
- Comment quality findings are applied automatically (with one confirmation), reducing
  friction for fixing narrating or tautological comments.
- Undecidable properties get reviewed at all, instead of being dropped because no linter could
  hold them — and they get reviewed by a mechanism that is honest about being probabilistic.
- What CI blocks on stays decidable, so a red gate always means a fact and never a guess.

### Negative Consequences

- Multiple concurrent subagent spawns increase cost and latency compared to a single-pass
  review.
- The runtime stage requires a live application environment (`make serve` and `make db-init`);
  it is skipped when no endpoint is touched by the change.
- The model-divergence guarantee depends on the orchestrator correctly detecting and
  overriding when the session model matches the reviewer model default.
- Non-gating checks can be skipped. A reviewer who never runs `ddd-audit` sees none of its
  findings, and nothing in CI will tell them. This is accepted: the alternative is gating a
  merge on a probabilistic judgment, which trades a missed finding for a false block.
- Agents judging against an external standard depend on the model's recall of that standard.
  The stated-premise requirement and the verifier pass reduce the damage but cannot remove it.

## Alternatives Considered

### Single-model review

Simple to invoke, but the same model that wrote the code reviews it. Structural blind spots
are shared between implementer and reviewer. Rejected because the primary value proposition
is bias reduction through model divergence.

### Human-only review

Provides genuine independence but is not available at every commit and does not scale with
AI-driven development pace. The `impl-review` skill is positioned as a pre-PR complement
to human review, not a replacement.

### Automated linting only

`make go-lint` and `make go-fix` catch formatting and static analysis errors but do not reason
about correctness semantics, authorization, architecture violations, or runtime behavior.
Rejected as insufficient for a full code review pass.

### Heuristic static analysis for undecidable properties

An analyzer could approximate "constructed only via the validated constructor" by flagging
composite literals of a type that also has a `New`, or approximate design-document coverage by
grepping for pattern names. Rejected in both the CI-gating and the opt-in form. Gating a merge on
an approximation makes the gate lie in a direction nobody can see; opting in does not make the
approximation any better, it only relocates the consequences of it being wrong onto the person
who trusted it. Where a property is undecidable, this project prefers a mechanism that is openly
probabilistic and can be argued with over one that is quietly probabilistic and cannot.

### Skipping the external-standard audit entirely

Relying on maintainer knowledge alone is cheaper and avoids the model-recall risk. Rejected
because that knowledge is held by individuals and is invisible to a reviewer who has not read the
source material — and the failure it produces is silent: a concept nobody knows exists is never
looked for and never missed. Flagging with a stated premise trades a bounded, visible risk for an
unbounded, invisible one.

## Notes

- Source: `.claude/skills/impl-review/SKILL.md`, `.claude/agents/adversarial-reviewer.md`,
  `.claude/agents/review-verifier.md`.
- The `adversarial-reviewer` and `review-verifier` agent files declare `model: sonnet`
  in their frontmatter; the orchestrator overrides this via the `Agent` tool `model`
  parameter when the session model is also `sonnet`.
- Comment quality findings are the only findings auto-applied in Step 8; the four code
  lenses are reported-only (no auto-fix applied by the skill).
- Findings are posted to the branch PR as inline review comments by default; suppress with
  `--no-comment`, or pass `--no-apply` to suppress the comment auto-fix.
- DDD audit source: `.claude/skills/ddd-audit/SKILL.md`, `.claude/agents/ddd-origin-auditor.md`,
  `.claude/agents/drift-detector-ddd.md`, ledger at `.agents/ddd-audit/pattern-ledger.yaml`.
  `arch-check` runs the auditor in quick mode when domain code or the ADR / README corpus is
  touched; `back-prop` category (D) checks the ledger against that corpus.
- The ledger lives under `.agents/` rather than `.claude/` because it is a skill artifact that
  other assistants (Codex, Cursor) may produce or consume, and nothing about it is Claude-specific.
