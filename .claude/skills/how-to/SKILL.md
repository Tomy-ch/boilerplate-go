---
name: how-to
description: >-
  Find the sanctioned way to carry out an operational goal in this repository and hand it back as a runnable procedure — prerequisites, the exact commands, how to tell it worked, how to undo it, and what is destructive about it. Use whenever someone wants to DO something and does not know the blessed route: 「integration test だけ回したい」「リリースはどうやる」「新しい環境変数を足すには」「マイグレーションを取り消したい」「この検証はどのコマンド？」. It is goal-driven, which separates it from the symptom-driven `repo-ops`, and it routes to the skill that already owns a procedure rather than re-deriving it. Read-only knowledge — it surfaces the command and warns before anything destructive, and runs it only when explicitly asked to perform the operation. Do NOT use it for a symptom or a failing gate (`repo-ops`), to explain how something works rather than how to do it (`repo-truth`), to compare undecided options (`research`), or to carry out a code change (`impl-issue`).
argument-hint: '[goal] [--mode=lookup|run] [--dry-run]'
---

# How To

Answer "what is the sanctioned way to do this here" with a procedure that can actually be run.

A Japanese reference translation lives at `SKILL.ja.md` in this directory (for human reference only;
not loaded as a skill).

## When to Use

- Someone wants to perform an operation and does not know the blessed route.
- Someone has a command in mind and wants to know whether it is the sanctioned one.
- Someone needs the prerequisites, the success signal, or the rollback for a procedure they already
  know the command for.

Do NOT use it for a symptom or a failing gate (`repo-ops`), to explain how something works rather
than how to do it (`repo-truth`), to compare undecided options (`research`), or to carry out a code
change (`impl-issue`).

## Contract

| | |
| --- | --- |
| **Owns** | 目標 → 正規手順（前提 / 成功判定 / 復旧 / 警告つき）、および所有スキルへの routing |
| **Never** | コマンドの発明 / 所有スキルの手順の再記述 / 破壊的操作の無断実行 / ツールの独断インストール |
| **Starts when** | 実行したい操作があり、正規経路が不明なとき |
| **Stops when** | 索引を通読しても手順が無い（UNDEFINED）、候補が複数で決着しない（AMBIGUOUS）、権限超過（BLOCKED） |

## Why this exists, and why it is not `repo-ops`

The two are different doors, and merging them breaks both.

`repo-ops` is **symptom-driven**: something behaved unexpectedly, you match it against a curated index
of known gotchas, and section N is the fix. Its value is that everything in it was actually hit by
someone. It cannot conclude "no procedure exists", and teaching it to would make its silence
indistinguishable from an answer.

This skill is **goal-driven**: you know what you want to achieve and not how it is done here. That
question has no index. There are 44 skills, 52 `.mk` files, a hook config, a workflow directory, and
`docs/maintenance/` — and nothing enumerates "the sanctioned way to do X" across them. `tool-map`
inventories skills, `make help` lists targets, and neither answers a goal.

The failure this skill is shaped around is specific: **a plausible command is indistinguishable from
a documented one.** `make test-integration` is exactly the kind of target that ought to exist, reads <!-- skill-lint-ignore -->
as authoritative when written down, and will be run by whoever asked — and it does not exist. That
it had to be marked as a deliberate non-reference for this repo's own lint to accept the sentence is
the point. Producing the sanctioned command or producing nothing are the only acceptable outcomes.

## Arguments, and the door check

| Argument | Effect |
| --- | --- |
| `--mode=lookup` *(default)* | Produce the procedure. Run nothing |
| `--mode=run` | Carry the operation out, confirming each destructive step individually |
| `--dry-run` | Prefer the `DRY_RUN=1` form of every step that offers one |

`--mode` exists for safety, not convenience. Without it, "did they want this run or explained?" is
inferred from phrasing — and the phrasing that means *do it* and the phrasing that means *show me*
differ by a particle. That inference is acceptable for a read; for `make db-test-reinit` it is not.
Absent the flag, assume `lookup`: showing a command to someone who wanted it run costs one more turn,
and the reverse costs a database.

**Then check you are the right door.** The neighbouring skills take the same nouns and differ only in
what the user is describing:

| The user is describing | Door |
| --- | --- |
| something that broke, or a gate that failed | `repo-ops` |
| how something works, what the rule is, whether it exists | `repo-truth` |
| an operation they want to perform | here |

A symptom arriving here is the case worth catching: assembling a procedure for a system that is
already in a broken state produces steps whose prerequisites do not hold. Say so and point at
`repo-ops` instead of proceeding.

## Step 1 — Route to the owning skill first

Before assembling anything, check whether a skill already owns this procedure. Many do — adding an
environment variable is `new-env`, committing is `commit`, opening a PR is `submit-pr`, upgrading Go
is `go-upgrade`, generating a layer is the `scaffold-*` family, upgrading pins is `actions-pin` /
`images-pin` / `tools-upgrade` / `dep-vuln-upgrade`.

When one does, **say so and stop.** Do not re-derive its steps. A procedure that exists in two places
diverges, and the copy is the one that rots — this is the same README > Code > SKILL precedence that
`back-prop` enforces, applied to procedures.

```bash
ls .claude/skills/                       # what exists
grep -l "<goal keywords>" .claude/skills/*/SKILL.md
```

Run `/tool-map` when the inventory itself is the question.

## Step 2 — Assemble from the registries, by concern

When no skill owns it, the procedure lives in one of these. Read the **index**, not a keyword search:
a target is named for what it does, not for how the goal was phrased.

| Registry | What it holds | Index |
| --- | --- | --- |
| make targets | almost every sanctioned operation | `.makefiles/README.md`, `make help` |
| git hooks | what runs at commit / push time | `.lefthook.yaml` |
| CI | what runs on a PR, and with what env | `.github/workflows/` |
| operational docs | local topology, DB pool, upgrade procedures | `docs/maintenance/` |
| change procedures | API / DB / business-logic flows | `docs/development-flow.md` |

`.claude/skills/repo-ops/SKILL.md` section 0 carries the answer-target → source table and the
noise-free search invocation; read it at runtime rather than duplicating it here. Its sections 1-21
are also worth scanning even for a goal question — a known gotcha attached to the procedure belongs
in Warnings.

Keyword search comes last, as a net for what the indexes missed.

Graphify, when the checkout has it, reaches structure that text search cannot; `.claude/README.md`
records what actually pays off here. It indexes code and docs, not the make graph, so for this skill
it is a supplement to the registries above rather than a replacement.

## Step 3 — Establish the operational envelope

A command alone is not a procedure. Each of these is a separate question, and each has a source:

- **Prerequisites.** Does infra need to be up? Is a DB slot required (`make slot-acquire` in a
  worktree)? Does it need `vendor/`, a generated artifact, or a specific branch? Read the target's
  recipe, not its name.
- **Expected result.** How does the caller know it worked — exit status, a file that changes, a
  check that turns green? A procedure whose success cannot be recognized will be declared successful.
- **Recovery.** What undoes it, and is undoing it even possible? Say plainly when it is not.
- **Warnings.** Destructive to shared state, environment-dependent, slow, or touching secrets.

Two properties of this repository make the envelope non-obvious and are worth checking every time:

- **Shared infrastructure.** One Postgres (`gobp-shared`) is shared by every checkout, so an
  operation that looks local can affect other worktrees. `docs/maintenance/db-worktree-pool.md`
  governs.
- **A dry-run convention.** Several targets honour `DRY_RUN=1`. When one does, put it in the
  procedure — it converts an irreversible step into an inspectable one.

## Step 4 — Answer in this contract

Always this shape, in Japanese. Omit a field only when it genuinely does not apply, and say so
rather than dropping it silently.

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
<UNDEFINED のときは「なし」と書き、なぜ近そうなコマンドを出さないのかを 1 行添える>

## 近いもの
- `<target / 手順>` — <ただし〜の点で目的が異なる>   ← FOUND 以外のときに書く

## 成功の判定
<どうなれば成功か>

## 復旧 / 切り戻し
<既存資料に定義されたもの。無いなら「定義なし」と書く>

## 注意
- <破壊性 / 環境差 / 共有インフラへの影響 / 未検証の点>

## 探索範囲
<通読したレジストリ / 確認した所有スキル / 検索した glob / 回さなかった掃引とその理由>
← UNDEFINED / AMBIGUOUS のときは必須

## エスカレーション
<未定義・矛盾・権限超過のとき、誰が何を決めれば進むか>
````

The four states are not interchangeable:

| State | Meaning |
| --- | --- |
| `FOUND` | A sanctioned procedure exists and is reproduced above |
| `AMBIGUOUS` | Several candidates exist and the sources do not settle which governs — present all, pick none |
| `UNDEFINED` | The owning indexes were read in full and no sanctioned procedure exists |
| `BLOCKED` | A procedure exists but the requested operation exceeds what was authorized here |

`UNDEFINED` carries the same bar as `repo-truth`'s: it requires the owning registries read **in full**,
and the frontier published with it. Short of that the state is `AMBIGUOUS` or the answer is 確認できず
— naming which index was left unread. Downgrading costs nothing; a wrong `UNDEFINED` reads as a fact
about the repository.

Under `UNDEFINED` the `コマンド` field says **なし**, and says in one line why the obvious-looking
command is not being offered. Leaving the field empty invites the next reader to supply the command
themselves, which is the failure this skill exists to prevent, one step removed. What *does* belong is
the near miss — under `近いもの`, named together with the respect in which its purpose differs. A
reader who is told only "there is no procedure" will go looking, and the nearest thing they find is
the one this skill already examined and rejected.

## Step 5 — Running it

Under `--mode=lookup` this step does not happen: the procedure is the deliverable. Under
`--mode=run`, carry it out — and note that the flag authorizes the *procedure*, not each destructive
step inside it. Those are still confirmed one at a time.

Before anything destructive, say what it will destroy and wait. `AGENTS.md` and `repo-ops` both draw
this line, and shared infrastructure is why: dropping a database, `chown -R` over a tree, or stopping
the shared compose project reaches every other checkout on the machine, and the person who asked was
usually thinking only about their own.

Never install anything on your own initiative. A missing tool is a finding to report; `AGENTS.md`
governs, and this repository usually already ships the capability behind a `make` target.

## Standalone by design

This skill hands over a procedure and stops. It names what to run next — `repo-ops` when the
procedure fails on a known gotcha, `repo-truth` when the goal turned out to be a knowledge question,
`new-issue` when the gap is worth tracking — and the user decides.

It also does not absorb the skills it routes to. Routing to `new-env` is the whole answer; restating
`new-env`'s steps here would create the second copy this skill exists to prevent.

## Do / Do NOT

- ✅ Treat a missing `--mode` as `lookup`; never infer authorization to run from phrasing.
- ✅ Check for an owning skill first, and stop there when one exists.
- ✅ Read the registries by index, not by keyword.
- ✅ Establish prerequisites, success signal, recovery, and warnings as separate questions.
- ✅ Cite the file and the target / section each step came from.
- ✅ Check shared-infrastructure impact and `DRY_RUN=1` availability every time.
- ✅ Report `UNDEFINED` with the frontier, or downgrade to `AMBIGUOUS` / 確認できず.
- ✅ Under `UNDEFINED`, write `なし` in `コマンド` with the reason, and name the near miss under
  `近いもの` — including the respect in which its purpose differs.
- ✅ Record a sweep you deliberately did **not** run, with its reason.
- ✅ Warn before anything destructive and wait.
- ✅ Answer in Japanese.
- ❌ Invent a command, a flag, or a target that you did not read in a registry.
- ❌ Restate the steps of a skill that owns the procedure.
- ❌ Report `UNDEFINED` from a keyword search or from indexes not read in full.
- ❌ Pick between conflicting sources — present both and escalate.
- ❌ Run a destructive command without saying what it destroys, or any command the user asked only to
  be shown.
- ❌ Install a tool because one is missing.

## Checklist

- [ ] Door check done — a symptom goes to `repo-ops`, a knowledge question to `repo-truth`.
- [ ] `--mode` resolved; absent the flag, treated as `lookup` and nothing was run.
- [ ] Owning skill checked; routed and stopped if one exists.
- [ ] Registries read by index; `repo-ops` section 0 read at runtime; keyword search used last.
- [ ] Prerequisites, expected result, recovery, and warnings each established from a source.
- [ ] Shared-infrastructure impact and `DRY_RUN=1` availability checked.
- [ ] Contract emitted in full, in Japanese, with an explicit state.
- [ ] `UNDEFINED` only on exhausted indexes, with the frontier published in `探索範囲`.
- [ ] Under `UNDEFINED`: `コマンド` says なし with a reason, and `近いもの` names the near miss.
- [ ] Nothing invented; every command traced to the file that defines it.
- [ ] Destructive steps flagged and confirmed before running; nothing run that was only to be shown.
