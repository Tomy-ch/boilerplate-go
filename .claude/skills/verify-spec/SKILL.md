---
name: verify-spec
description: >-
  Integrator skill for spec validation. Confirms the target package path (or receives it from `scaffold-endpoint`), then validates whichever of `docs/spec/domain/<pkgpath>.md` / `docs/spec/usecase/<pkgpath>.md` exist — format, internal consistency, entity-to-SQL correspondence for domain, cross-spec references and naming convention for usecase — and reports in Japanese. It does NOT check OpenAPI operationId coverage: that would violate dependency direction, and `scaffold-controller` verifies the mapping at scaffold time. Read-only. Use it before implementing from a spec, or after editing one. Do NOT use it to write a spec (`new-spec`) or to audit implementation code (`arch-check`).
---

# Verify Spec

Integrator for spec validation. Fans out per-spec **read-only validator subagents** in parallel based on which of the two spec trees — `docs/spec/domain/**/*.md` and `docs/spec/usecase/**/*.md` — hold a spec for the target package path.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- Before invoking `scaffold-endpoint` to catch spec inconsistencies upfront (`scaffold-endpoint` auto-chains this).
- Standalone after editing specs, to confirm all checks pass.
- During spec authoring as a quick check.

To validate a single spec, run this integrator — it detects which of the domain / usecase specs exist for the target package path and fans out only the matching validator.

Do NOT use for:

- Verifying generated code — that's `make go-test`.
- Implementation ↔ spec drift — that's `arch-check`.
- Fixing inconsistencies — read-only, reports only.

## Architecture: parallel validator subagents

Validation is delegated to two **read-only worker subagents** under `.claude/agents/`, one per spec file. The integrator runs them concurrently via the Agent tool (`subagent_type`):

| Validator subagent | Spec | Checks |
| --- | --- | --- |
| `spec-validator-domain` | `docs/spec/domain/<pkgpath>.md` | format + path ↔ `package:` + entity ↔ SQL soft + internal consistency |
| `spec-validator-usecase` | `docs/spec/usecase/<pkgpath>.md` | format + path ↔ `package:` + cross-spec to the domain specs it depends on + 命名規約 + Workflow consistency |

lean A 構成では controller / infra の spec ツリーが存在しないため spec 検証は不要（controller / infra は実装時に OpenAPI + sqlc gen から導出され、verify は `arch-check`（controller / infra 監査）が implementation 側で実施）。

The validators are the per-spec validation workers and are **strictly read-only** (no auto-fix, no writes). The two validators read independently — `spec-validator-usecase` opens the domain specs it needs itself, resolving them from its own `## Dependencies` — so there is **no write dependency** between them and they can run in parallel.

## First Step: Confirm Target Package Path

Spec identity is the **package path**, not a feature directory: `docs/spec/<layer>/<rest>.md` corresponds to `internal/<layer>/<rest>`.

This skill **MUST call `AskUserQuestion` immediately after invocation** (unless invoked from `scaffold-endpoint` with the package path already in context):

- 質問: 「検証対象のパッケージパスを選んでください」
- 選択肢: `docs/spec/domain/**/*.md` と `docs/spec/usecase/**/*.md` を列挙し、`docs/spec/<layer>/` 接頭辞と `.md` を落としたパッケージパスの和集合 + 規約外パス用のフリーテキスト

`glossary.md` sits directly under `docs/spec/` and belongs to neither tree, so this enumeration excludes it without a special case.

If neither tree holds a spec for the confirmed path, abort with a clear message.

## Step 1. Detect Existing Spec Files

For the confirmed package path, check existence of:

- `docs/spec/domain/<pkgpath>.md`
- `docs/spec/usecase/<pkgpath>.md`

If neither exists → abort with message. If only one exists → fan out only the matching validator. **A usecase spec with no domain spec at the same path is normal**, not a finding: a usecase that owns no aggregate has none, and `spec-validator-usecase` resolves the domain specs it actually references from its own `## Dependencies`.

## Step 2. Fan Out Validator Subagents IN PARALLEL

For the existing spec files, spawn the matching validators with the **Agent tool**, all in **a single message with multiple tool calls** so they run concurrently. Pass each validator:

- `pkgpath` — the confirmed package path
- `specPath` — the spec file path (`docs/spec/domain/<pkgpath>.md` or `docs/spec/usecase/<pkgpath>.md`)

Each validator's final message **is** its findings (Japanese), ending in a machine-readable `SUMMARY violations=<v> suggestions=<s>` line. Collect them with their spec label and parse the SUMMARY counts.

> If the `spec-validator-*` subagents cannot be spawned in the current environment, follow each `spec-validator-<layer>.md` procedure inline instead (domain first, since usecase references it).

## Step 3. Aggregate Report (Japanese)

```text
verify-spec 統合結果（package: <pkgpath>）

[domain] violations: N, suggestions: K
  - <findings from spec-validator-domain>

[usecase] violations: N, suggestions: K
  - <findings from spec-validator-usecase>

総計: violations <sum>, suggestions <sum>
```

All clean:

```text
verify-spec 統合結果（package: <pkgpath>）
全 spec で違反は検出されませんでした（チェック済み: <spec list>）。
```

## Step 4. Closing

- **Standalone invocation**: print the report and exit. Even with violations, exit status is 0 (informational).
- **Chained from `scaffold-endpoint`**: when aggregated `violations > 0`, signal the parent to abort the downstream chain with a clear "scaffold can not safely proceed" message. (Suggestions do not abort.)

## AI Modification Scope

Strictly read-only. The integrator and all validator subagents touch no spec or source files. The integrator only runs `AskUserQuestion` (package-path confirmation, standalone) and spawns read-only validators.

## Constraints

- ❌ validator を逐次起動（必ず1メッセージ内で複数 Agent 呼び出し＝並列）
- ❌ Hardcode rules — validators read `.claude/scaffold-spec/<layer>-spec.md` + `verify-rules.md` every run
- ❌ Auto-fix violations / modify any file
- ❌ Skip the target-confirmation `AskUserQuestion` (unless supplied by `scaffold-endpoint`)
- ❌ Fan out a validator when its target spec file is missing
- ✅ Japanese aggregated report
- ✅ Fan out only existing spec files
- ✅ Per-spec validator / skill が独立 standalone 動作可能であることを維持
- ✅ Run all per-spec checks in one pass (no fail-fast); abort downstream only when chained from `scaffold-endpoint` with violations

## Checklist

- [ ] Target package path confirmed via `AskUserQuestion` (or supplied by `scaffold-endpoint`)
- [ ] Existing spec files detected (domain / usecase tree)
- [ ] Matching `spec-validator-*` を **1メッセージ内で並列起動**（pkgpath / specPath を渡す）
- [ ] 各 validator の SUMMARY を集約
- [ ] Aggregated Japanese report emitted
- [ ] scaffold-endpoint から chain 時のみ violations>0 で downstream abort
- [ ] No file modifications
