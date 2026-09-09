---
name: arch-check
description: Audit this repository's architectural compliance before review or merge. Use for changed Go files, one or more specified layers, or the full repository; a full-repository or domain-layer scope also fans out `ddd-modeling-reviewer` without a diff, making this the entry point for questions about the current aggregate boundaries or rule placement when no change exists. Run the shared lint baseline once, inspect the applicable onion-architecture layers against their runtime READMEs, and report violations separately from advisory suggestions. For changed-files review, leave DDD modeling to `impl-review` so the same finding is not reported twice. Make source changes only when the user explicitly asks to add TODO hand-offs.
---

# Architecture Compliance Check

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

Use this skill for an architecture-focused review, not for formatting, general code review, or specification validation.

## Contract

| | |
| --- | --- |
| **Owns** | Code against this repository's rules through five layer auditors, plus domain-modeling review when there is no diff |
| **Never** | Treat general preferences as violations or repair the implementation; optional `// TODO:` hand-offs are the only write |
| **Starts when** | An auditable change or existing structure is present |
| **Stops when** | The governing rule cannot be identified; a missing README belongs to `back-prop` |

## Auditor architecture

Five read-only auditors apply the repository's documented rules to their respective layers:

| Path | Layer | Agent |
| --- | --- | --- |
| `internal/domain/` | domain | `.codex/agents/arch-auditor-domain.toml` |
| `internal/usecase/` | usecase | `.codex/agents/arch-auditor-usecase.toml` |
| `internal/controller/` | controller | `.codex/agents/arch-auditor-controller.toml` |
| `internal/infrastructure/` | infrastructure | `.codex/agents/arch-auditor-infra.toml` |
| `pkg/` | shared package | `.codex/agents/arch-auditor-pkg.toml` |

Two further auditors are keyed to the subject rather than to a layer:

| Agent | Trigger | Checks |
| --- | --- | --- |
| `.codex/agents/ddd-origin-auditor.toml` | Domain code or the ADR / README corpus is changed | Differences between the repository's documented DDD interpretation and Evans, including whether deviations are declared |
| `.codex/agents/ddd-modeling-reviewer.toml` | `internal/domain/**` is in scope and the scope is the full repository or one or more named layers | Aggregate and transaction boundaries; whether rules belong to an entity, value object, Domain Service, or usecase; cross-aggregate reference discipline; ubiquitous language against `docs/spec/glossary.md` |

The `ddd-origin-auditor` judgment is different from the five layer audits. The layer auditors compare
code with this repository's own rules; this auditor compares the documentation with an external
yardstick, Evans. Its output is a three-state flag rather than a violation and contains no verdict.
Use the `ddd-audit` skill for a deep audit; this skill uses only its change-related `quick` scope.

The scope restriction on `ddd-modeling-reviewer` is deliberate. `impl-review` already owns this lens
as tier 1 for diff reviews, so running both against one change would duplicate findings without making
their provenance clear. All three `impl-review` scopes require a diff, however, and therefore cannot
answer a question such as whether the current aggregate boundary is sound when no change exists.
This skill supplies that entry point and silently defers to `impl-review` whenever the scope is changed
files.

This lens also does not overlap the five layer auditors. `arch-auditor-domain` enforces mechanical
rules such as forbidden imports, direct `time.Now()` calls, and advisory entity-to-SQL correspondence;
none of those auditors decides whether the boundary itself was drawn in the right place.

## Scope and write permission

Default to changed production Go files, excluding generated files, mocks, and tests. For an existing pull request, its `baseRefName` is authoritative; otherwise resolve the base with `make base-branch` (the latest `release/*` from `origin`'s live state), never `gh repo view --json defaultBranchRef`; if the base cannot be resolved, stop and report that failure in Japanese, separately from the no-changed-production-files case.

Use the full repository only when requested. A request naming one or more layers limits the audit to those layers:

This skill is read-only by default. Do not add `TODO` comments merely because the source skill offered that option. Add them only after explicit user approval, and never add one for a violation that should be fixed.

## Procedure

1. Read `AGENTS.md`, then resolve the in-scope files and layers. Report changed Go files outside the listed layers separately; do not pretend they were audited.
2. Run `make go-lint` once. Attribute its findings to the appropriate layer. If the baseline is broken by unrelated errors, report the failure and do not continue semantic review.
3. For each in-scope layer, read the agent definition named in the auditor table above, the layer README, and any applicable nearest package README. The agent definition gives the audit role and output contract; `AGENTS.md` and the READMEs remain the source of truth for repository rules.
4. Review layer-specific concerns:

   - **domain:** purity; inward-only dependencies; no framework, persistence, I/O, direct time/randomness/ID generation, or inappropriate context use. Treat entity-to-table correspondence as an advisory check, allowing value-object and computed-method representations.
   - **usecase:** orchestration and transaction ownership; dependency only on domain interfaces; boundary abstractions for time, randomness, and external I/O; no business invariants that belong in domain.
   - **controller:** OpenAPI operation-to-handler correspondence; request/response adaptation only; no infrastructure imports or business orchestration. Treat handler size and non-trivial branching as suggestions unless a documented rule is violated.
   - **infrastructure:** implementation of domain interfaces; data orchestration only; generated-query use and error normalization according to the RDB and `pgerror` READMEs. Treat one-to-one repository/query correspondence as advisory because joins, multi-query operations, and dispatch are valid.
   - **pkg:** no `internal/` dependency; framework-agnostic and reusable; no feature-specific business logic. Honor an explicit subpackage README exception when present.

5. Fan out independent in-scope roles concurrently when delegation is available; otherwise execute their instructions inline. Pass each layer auditor the resolved files and shared lint output, and never run `make go-lint` more than once. When the audited change touches domain code or the ADR / README corpus, also run `ddd-audit` with its `quick` scope preset through `.codex/agents/ddd-origin-auditor.toml`; it must not ask for scope again. Keep this separate from the per-layer audits: it compares the repository's documented DDD interpretation with Evans, flags divergences for a human, and never arbitrates or fixes them.

   When the scope is the full repository or named layers and includes `internal/domain/**`, also fan out `.codex/agents/ddd-modeling-reviewer.toml`. Pass the resolved current domain file list and no `baseRef`; passing a diff would reduce this lens to a smaller `impl-review`. Do not run it for changed-files scope. Add one report line stating that the DDD-modeling lens was skipped because diff review belongs to `impl-review`.
6. Report in Japanese using this form:

   ```text
   arch-check 結果（スコープ: <changed | full | layers>）

   [lint]
   - <file:line>: <message>

   [<layer>]
   - violation: <file:line> — <problem>
     source: <AGENTS.md or README location>
     remediation: <action>
   - suggestion: <file:line> — <advisory>
     source: <location>

   [ddd-modeling]
   - <finding grounded in the repository's DDD documents, or the changed-files skip reason>

   総計: violations <n>, suggestions <n>
   ```

   Do not manufacture findings. If clean, name the audited layers and say no violations were found.

## Optional TODO hand-off

Only after explicit approval, add a plain Japanese `// TODO:` comment for a suggestion in `internal/domain`, `internal/controller`, or `internal/infrastructure`.

Before insertion, inspect the three preceding lines. If a comment block already exists, skip it. Describe the observed deviation and the decision left to the human. Do not use an AI-specific marker, and do not write TODOs for violations.

## Completion constraints

- Do not auto-fix, stage, commit, or push.
- Cite the governing README or `AGENTS.md` for every finding.
- Keep generated files untouched.
- Report TODO additions and skips separately if the optional hand-off was approved.
