# Agent Environment

This document explains how the repository's agent environment turns its declared properties into daily practice. It is an interpretation of [ADR-0008 (agent-environment-alignment)](../adr/0008-agent-environment-alignment.md) — both the properties a control must satisfy and the loop that later retires it — not a checklist or a second source of rules. Why AI assistance is the standard path at all is [ADR-0007 (agents-md-operational-contract)](../adr/0007-agents-md-operational-contract.md).

```mermaid
flowchart TD
    A["AGENTS.md · development flow · owning README"] --> B["Implement the change"]
    B --> C{"Can the property be decided mechanically?"}
    C -->|yes| D["Lint · architecture tests · generation / drift checks"]
    C -->|no| E["Independent finder-to-verifier review"]
    D --> F{"Changes an approved decision, crosses a boundary, or has no reliable signal?"}
    E --> F
    F -->|yes| G["Escalate to the responsible human"]
    F -->|no| H["Existing check or review signal is sufficient"]
    I["ci-first load band"] -. "delegate heavy local gates; CI remains authoritative" .-> D
```

## Guide before action

[AGENTS.md](../../AGENTS.md), the development flow, and the package README nearest to the target provide the governing and local context before an action. `CLAUDE.md` is AGENTS.md's symlink, not a second contract. Skills make recurring procedures explicit, but read and link to these same sources rather than replacing them.

## Correct after action

Decidable properties fail mechanically: `depguard` and architecture tests protect layer boundaries; generators and drift checks protect derived artifacts; focused linters protect workflow and documentation conventions. Reading-comprehension judgments remain independent review, using the finder-to-verifier shape in [ADR-0097 (multi-model-adversarial-review)](../adr/0097-multi-model-adversarial-review.md).

## Escalation and load-aware verification

Escalate a matter when it changes an approved decision, crosses an architectural boundary, or cannot reappear through an existing check. Do not create a durable task merely because a mechanism already reports the same condition reliably.

The `ci-first` load band delegates heavy local gates to CI when host saturation would make their failures unreliable. It does not permit verification to be skipped: the authoritative check still runs remotely, while local work keeps fast, trustworthy checks.

## Signals whose meaning depends on a human sending them

Some artifacts carry weight only because a human chose to create them, so an agent producing the identical artifact destroys that weight rather than supplying it. A public cross-reference to another repository's issue is the worked example: it tells upstream maintainers that a real project is watching an issue and needs it resolved, and they weigh it when prioritizing. Now that agents can file issues and gather references at scale, a reference emitted by tooling is indistinguishable from one a maintainer chose to send, and the count degrades from signal into spam.

A control over such a signal cannot be satisfied by a better-informed agent, and a standing grant of autonomy does not transfer it. That is why [AGENTS.md](../../AGENTS.md) requires a per-case human confirmation rather than a rule an agent can evaluate, and why the default is the form that leaves no upstream trace.

## Improving the environment itself

The steps above describe one change. The environment that guides and checks it is under a loop of
its own, because a control is added far more readily than it is removed: a skill nobody invokes and
a rule whose case no longer occurs both produce no failure, so the signal to retire them is the
absence of a signal.

```mermaid
flowchart LR
    A["Observe AI sessions"] --> B["Compress friction into feedback"]
    B --> C["Attribute it to a skill · rule · doc · CI step · tool"]
    C --> D["Generate improvement candidates"]
    D --> E["Human decides the design questions"]
    E --> F["Land the improvement"]
    F --> G["Re-evaluate its effect after an interval"]
    G --> H{"Keep · Simplify · Revise · Delete · Revert"}
    H --> A
```

In practice the loop is bounded on three sides:

- **What is observed** is friction, not activity, and it is kept **outside the repository** — the
  compressed finding (what was hit, which control it implicates, what the change to that control
  measurably did) goes to its issue, never to a commit. A per-run narrative is not kept at all, and
  [`.agents/`](../../.agents/README.md) holds only the loop's configuration;
  [ADR-0009 (long-running-agent-state)](../adr/0009-long-running-agent-state.md) draws both lines.
- **Who decides** is a human wherever the candidate touches architecture, domain, or policy. The
  loop surfaces the decision and its options; it does not take one.
- **What may be edited** follows the same scope as any other change. The AI-tool configuration
  directories stay outside an agent's default scope and are reachable only through an explicitly
  invoked skill, bounded by that skill's own procedure, per the skill-execution exception in
  [AGENTS.md](../../AGENTS.md). `AGENTS.md` itself is human-maintained and is never edited this way.

Skills carry the same lifecycle as any other control, inventoried on how often they are invoked, the
occasions on which they would have applied but were not, the friction they themselves caused, and
the measured effect of the last change to them.

## Keeping this interpretation current

This is a describing document. Update it only when the relationship it describes changes. Governing documents and accepted ADRs are not silently corrected from implementation drift; raise conflicts as [docs/rules.md](../rules.md) requires.
