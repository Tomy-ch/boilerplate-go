---
name: how-to
description: >-
  Find the sanctioned way to perform an operational goal in this repository and return an executable, source-backed procedure with prerequisites, exact commands, success criteria, recovery, and destructive-operation warnings. Use whenever someone wants to do something but does not know the blessed route, wants to verify whether a remembered command is canonical, or needs the operational envelope around a known command. Routes first to any skill that already owns the procedure, and never invents a command to fill a gap. Defaults to read-only lookup unless `--mode=run` is explicit. Do NOT use for symptoms or failed gates (`repo-ops`), explanations of how the repository works (`repo-truth`), undecided comparisons (`research`), or implementing code changes (`impl-issue`).
argument-hint: '[goal] [--mode=lookup|run] [--dry-run]'
---

# How To

Answer "what is the sanctioned way to do this here" with a procedure that can actually be run.

A Japanese reference translation lives at `SKILL.ja.md` in this directory. It is for human
reference and is not loaded as a skill.

## When to Use

- Use when someone wants to perform an operation but does not know the sanctioned route.
- Use when someone has a command in mind and wants to verify that it is canonical.
- Use when someone knows a command but still needs its prerequisites, success signal, recovery, or
  warnings.

Do not use for a symptom or failed gate (`repo-ops`), an explanation of how something works
(`repo-truth`), a comparison whose choice is still open (`research`), or execution of a code change
(`impl-issue`). If a named neighboring skill is absent from `.codex/skills/`, state that routing could
not be completed; do not absorb its responsibility here.

## Contract

| | |
| --- | --- |
| **Owns** | 目標から、前提 / 成功判定 / 復旧 / 警告を含む正規手順を特定し、所有スキルへ振り分けること |
| **Never** | コマンドの発明 / 所有スキルの手順の再記述 / 破壊的操作の無断実行 / ツールの独断インストール |
| **Starts when** | 実行したい操作があり、このリポジトリでの正規経路が不明なとき |
| **Stops when** | 所有索引を通読しても手順がない（UNDEFINED）、候補を決着できない（AMBIGUOUS）、権限を超える（BLOCKED） |

## Why This Exists and Why It Is Not `repo-ops`

Keep the two doors separate.

`repo-ops` is symptom-driven: match unexpected behavior against its curated inventory of known
operational failures. Its silence cannot prove that no sanctioned procedure exists.

This skill is goal-driven: the desired outcome is known, but the repository-specific route is not.
That answer may be distributed across skills, make targets, hooks, workflows, and maintenance
documentation. `tool-map` inventories skills and `make help` inventories targets, but neither alone
maps an operational goal to its complete procedure.

The central failure to prevent is that a plausible command looks exactly like a documented command.
For example, `make test-integration` sounds authoritative but is an intentional non-reference here, <!-- skill-lint-ignore -->
not evidence that the target exists. Produce a sourced sanctioned command or produce no command.
Never bridge missing evidence with a guess.

## Arguments and Door Check

| Argument | Effect |
| --- | --- |
| `--mode=lookup` (default) | Return the procedure and execute nothing |
| `--mode=run` | Perform the procedure, while confirming every destructive step separately |
| `--dry-run` | Prefer the `DRY_RUN=1` form for each step that explicitly supports it |

`--mode` is a safety boundary, not a convenience. If it is absent, always choose `lookup`; do not
infer permission to execute from phrasing. Showing commands to someone who wanted execution costs
one additional turn. Executing commands for someone who wanted an explanation can cost a database.

Then verify that this is the correct door:

| What the user is describing | Route |
| --- | --- |
| A symptom, broken behavior, or failed gate | `repo-ops` |
| How something works, what a rule says, or whether something exists | `repo-truth` |
| An operation they want to perform | Continue here |

Do not assemble a normal procedure for a system already known to be broken; its prerequisites may
not hold. Route that case to `repo-ops`.

## Step 1 — Route to the Owning Skill First

Inspect `.codex/skills/` before assembling any procedure. In particular:

- adding an environment variable belongs to `new-env`;
- committing belongs to `commit`;
- creating or updating a pull request belongs to `submit-pr`;
- upgrading Go belongs to `go-upgrade`;
- generating architectural layers belongs to the relevant `scaffold-*` skill;
- pin and vulnerable-dependency updates belong to `actions-pin`, `images-pin`, `tools-upgrade`, or
  `dep-vuln-upgrade` as applicable.

When a skill owns the procedure, name it and stop. Routing is the complete answer. Do not restate its
steps: duplicated procedures diverge, and the copied procedure is the one that rots.

Use the Codex-side inventory, not Claude-specific skill-discovery commands:

```bash
ls .codex/skills/
rg -l "<goal keywords>" .codex/skills/*/SKILL.md
```

Run the `tool-map` skill when the inventory itself is the question.

Codex-side skill copies can lag their Claude-side canonical counterparts during synchronization. If
a referenced `.codex/skills/<name>/SKILL.md` is missing or shows concrete evidence of stale content,
state that limitation and stop rather than silently substituting `.claude/skills/<name>/SKILL.md` or
reconstructing the owner skill's procedure. A human can decide whether to run `sync-ai`.

## Step 2 — Assemble from Registries by Concern

When no skill owns the goal, read the relevant registry indexes in full before using keyword search.
A target is named for what it does, not necessarily for the user's wording.

| Registry | What it owns | Index |
| --- | --- | --- |
| Make targets | Most sanctioned repository operations | `.makefiles/README.md`, `make help` |
| Git hooks | Commit-time and push-time checks | `.lefthook.yaml` |
| CI | Pull-request jobs and their environments | `.github/workflows/` |
| Operational documents | Local topology, DB pool, and upgrade procedures | `docs/maintenance/` |
| Change procedures | API, DB, and business-logic flows | `docs/development-flow.md` |

Read section 0 of `.codex/skills/repo-ops/SKILL.md` at runtime for its answer-target-to-source table
and noise-reduced search method; do not copy that table here. Scan its known-gotcha sections as well,
because a gotcha attached to the procedure belongs under `注意`.

The Codex copy of `repo-ops` may be behind its Claude-side source during a synchronization window. If
the expected section or evidence is absent, disclose that limitation instead of assuming equivalence.

Use keyword search last, only as a net for what the indexes missed.

When Graphify output exists, use it only as a supplement for structural relationships. The
repository-specific observations in the Claude-environment README at `.claude/README.md` are facts
about this repository and may be consulted as such, but Graphify indexes code and documentation, not
the make graph, so it does not replace the registries above.

## Step 3 — Establish the Operational Envelope

A command alone is not a procedure. Establish each item independently from a source:

- **Prerequisites:** required infrastructure, worktree DB slot (`make slot-acquire`), generated
  artifacts, `vendor/`, branch state, credentials, or permissions. Read the recipe, not just its name.
- **Expected result:** exit status, changed artifact, observable state, or check that turns green.
- **Recovery:** the documented undo path and whether recovery is possible. Say `定義なし` when no
  recovery is defined; do not invent one.
- **Warnings:** destructive or shared-state effects, environment dependence, duration, secrets, and
  any unverified assumption.

Check these repository-specific concerns every time:

- One Postgres service (`gobp-shared`) is shared across checkouts. A command that looks local can
  affect other worktrees. `docs/maintenance/db-worktree-pool.md` governs the topology.
- Some targets explicitly support `DRY_RUN=1`. Include that form only when the target or its owning
  documentation says it is supported.

## Step 4 — Answer in This Contract

Always answer in Japanese with this structure. If a field genuinely does not apply, say so instead
of silently omitting it.

````markdown
## 状態
FOUND | AMBIGUOUS | UNDEFINED | BLOCKED

## 手順
<何をする手順か、1 行>

## 出典
- `<path>` の `<target / 節 / job 名>`

## 前提条件
- <満たしていなければならないこと、必要な権限>

## コマンド
```bash
<正規に定義されたコマンド。存在するものだけ>
```
<UNDEFINED の場合は「なし」と書き、もっともらしい候補を提示しない理由を 1 行添える>

## 近いもの
- `<target / 手順>` — <ただし〜の点で目的が異なる>（FOUND 以外の場合に記載）

## 成功の判定
<どうなれば成功か>

## 復旧 / 切り戻し
<既存資料に定義されたもの。無いなら「定義なし」と書く>

## 注意
- <破壊性 / 環境差 / 共有インフラへの影響 / 未検証の点>

## 探索範囲
<通読したレジストリ / 確認した所有スキル / 検索した glob / 回さなかった掃引とその理由>
<UNDEFINED / AMBIGUOUS の場合は必須>

## エスカレーション
<未定義・矛盾・権限超過のとき、誰が何を決めれば進むか>
````

Interpret the four states exactly:

| State | Meaning |
| --- | --- |
| `FOUND` | A sanctioned procedure exists and is reproduced or routed above |
| `AMBIGUOUS` | Multiple candidates exist and sources do not select one; present all and choose none |
| `UNDEFINED` | Every owning registry was read in full and no sanctioned procedure exists |
| `BLOCKED` | A procedure exists, but the requested operation exceeds current authorization |

Use `UNDEFINED` only after reading every owning registry in full and publishing the exact search
frontier. Otherwise report `AMBIGUOUS` or `確認できず`, and name each unread index. A false
`UNDEFINED` is presented as a repository fact, so downgrade whenever the frontier is incomplete.

For `UNDEFINED`, write **なし** under `コマンド` and add one line explaining why the
obvious-looking command is not being offered. An empty field invites the next reader to fill in a
command independently, recreating the exact failure this skill prevents one step later. Record the
rejected near miss under `近いもの`, including how its purpose differs. A reader told only that no
procedure exists will search again and is likely to find the same candidate already inspected and
rejected here.

## Step 5 — Run Only in Run Mode

In `--mode=lookup`, do not execute any procedure command. The procedure is the deliverable.

In `--mode=run`, execute non-destructive authorized steps. The flag authorizes the procedure, not
each destructive step. Before every destructive step, explain in Japanese exactly what it will
destroy or mutate and present numbered choices in the conversation body, for example:

1. 実行する
2. このステップを中止する

Wait for the user's numbered response before continuing. Do not claim that a modal or a
Claude-specific confirmation tool will appear. If no interactive user is attached, report the step
as `BLOCKED`; never infer approval.

Treat dropping a database, recursive ownership changes, stopping shared compose services, and other
shared or irreversible mutations as destructive. Never install a missing tool on your own initiative.
A missing tool is a finding, and this repository usually exposes existing capabilities through a
sanctioned `make` target.

## Standalone by Design

Hand over the procedure and stop. Name the next owning skill when appropriate: `repo-ops` if a known
gotcha blocks the procedure, `repo-truth` if the request becomes a knowledge question, or `new-issue`
if the missing procedure may deserve tracking. The user chooses whether to invoke it.

Do not absorb any routed skill. Routing to `new-env`, for example, is the entire answer; copying its
steps here would create the duplicate this skill exists to prevent.

## Do / Do Not

- Do treat an absent `--mode` as `lookup` and run nothing.
- Do route to an owning skill first and stop when one exists.
- Do read registries through their indexes before keyword search.
- Do source prerequisites, success criteria, recovery, and warnings separately.
- Do cite the file and target, section, or job behind every step.
- Do check shared-infrastructure impact and explicit `DRY_RUN=1` support every time.
- Do publish the search frontier for `UNDEFINED`; otherwise downgrade the result.
- Do write `なし` with a reason under `コマンド` for `UNDEFINED`, and list the rejected near miss
  under `近いもの` with the purpose mismatch.
- Do record any sweep deliberately omitted from `探索範囲`, together with the reason.
- Do warn and wait with numbered choices before every destructive step.
- Do answer in Japanese.
- Do not invent a command, flag, target, rollback, or success signal.
- Do not restate a procedure owned by another skill.
- Do not report `UNDEFINED` from keyword search or partially read indexes.
- Do not choose between conflicting authoritative sources; present both and escalate.
- Do not execute a command requested only as a lookup.
- Do not install a tool merely because it is missing.

## Checklist

- [ ] Door check completed: symptoms go to `repo-ops`, knowledge questions to `repo-truth`.
- [ ] `--mode` resolved; an absent flag became `lookup`, and no command was run.
- [ ] Owning skills checked; routed and stopped when one owns the goal.
- [ ] Registries read by index; `repo-ops` section 0 checked at runtime; keyword search used last.
- [ ] Prerequisites, expected result, recovery, and warnings each came from a source.
- [ ] Shared-infrastructure impact and explicit `DRY_RUN=1` support checked.
- [ ] Full Japanese output contract emitted with an explicit state.
- [ ] `UNDEFINED` used only after exhausting indexes and publishing the frontier in `探索範囲`.
- [ ] Under `UNDEFINED`, `コマンド` says `なし` with a reason and `近いもの` identifies the
  rejected near miss.
- [ ] Every command traced to the source that defines it; nothing was invented.
- [ ] Every destructive step was described and separately confirmed; lookup-only commands were not run.
