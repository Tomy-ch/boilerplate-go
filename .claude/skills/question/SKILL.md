---
name: question
description: >-
  The front door for any question about this repository whose intended reading is not obvious. It answers nothing itself: it resolves what the asker actually meant along three axes — which world (the industry outside / this repository as it stands / the diff in this window), which intent (a symptom / an operation / knowledge / an undecided choice), which subject (implementation / tests / comments / docs / vocabulary) — confirms that reading with the human whenever an axis genuinely splits, and hands off to the skill that owns the answer. Use it when a question could reasonably be taken more than one way: 「今のこれってどうなの？」 turns on which *now* is meant; 「テストどう？」 splits across `test-review` and `repo-truth`; 「これ大丈夫？」 splits across nearly everything. Also use it when the asker does not know which skill exists for what, or explicitly asks where a question should go. Do NOT use it when the reading is already unambiguous — go straight to the owning skill (`repo-truth` for repository facts, `repo-ops` for a symptom, `how-to` for an operation, `research` for an open choice, `impl-review` / `test-review` / `comment-sweep` for a diff) — and never use it to produce an answer, a procedure, a comparison, or a review of its own.
argument-hint: '[question] [--to=<skill>] [--explain]'
---

# Question

Work out what the asker actually meant, confirm it, and hand the question to whoever owns the answer.

A Japanese reference translation lives at `SKILL.ja.md` in this directory (for human reference only;
not loaded as a skill).

## When to Use

- A question could reasonably be read more than one way.
- The asker does not know which skill covers what, or asks where a question should go.
- A previous run answered the wrong reading and it needs re-routing.

Do NOT use it when the reading is already unambiguous — go straight to the owning skill — and never
use it to produce an answer, a procedure, a comparison, or a review of its own.

## Contract

| | |
| --- | --- |
| **Owns** | 問いの意味論的解釈（世界 / 意図 / 対象の 3 軸）、人間への確認、所有スキルへの引き渡し |
| **Never** | 自分で答える / 手順を組む / 比較する / レビューする / 軸が割れているのに黙って片方を選ぶ |
| **Starts when** | 問いの読み方が一意でないとき、どのスキルが担当か分からないとき |
| **Stops when** | 引き渡し先が決まったとき、または該当するスキルが無いと分かったとき |

## Why this exists

Every other skill here has a door check that redirects a mis-triggered question. That catches the
easy case — the one where the wrong door is *visible* from inside it. It does not catch the case this
skill is for: a question that is genuinely well-formed under two different readings, where each
destination would produce a competent, confident, and differently-scoped answer, and nothing in the
output would reveal that the other reading existed.

The commonest form is a bare 「今」: it can mean the world outside this repository, this repository as
it stands, or the diff in this window. Each reading has a different owner, each owner would answer
competently, and nothing in the sentence chooses. A skill that picks one produces an answer that
reads as complete — which is exactly why picking is the wrong move and asking is the right one.

**The ambiguity is in the asker, so the asker resolves it.** That is not a fallback for a weak
heuristic; it is where the information actually lives. `AGENTS.md` puts design and policy decisions
behind a human gate for the same reason, and a routing decision that silently reframes the question
is one of those.

## The three axes

Resolve the question along these. Most questions are unambiguous on all three; the ones that reach
here split on one, occasionally two.

| Axis | Splits into | Signal that it is split |
| --- | --- | --- |
| **World** | 外の世界 / このリポジトリ全体 / この窓の差分 | a bare 「今」「最近」「普通」, or a comparison with no stated baseline |
| **Intent** | 症状 / 目標 / 知識 / 未決の選択 | 「どう」「大丈夫」「どうなってる」 with no verb naming an action or a failure |
| **Subject** | 実装 / テスト / コメント / ドキュメント / 語彙 / 運用 | a noun that names all of them at once（「品質」「これ」「この機能」） |

The axes are independent, so state each one's reading separately rather than jumping to a skill name.
A reader can correct 「差分ではなくリポジトリ全体の話」 far more easily than 「`arch-check` ではなく
`impl-review`」, because the first is about their own question and the second is about a tool they may
not know.

## Step 1 — Read the question and resolve what you can

Restate the question in one line, with your reading of each axis. Resolve every axis the sentence
actually settles — most of them.

Do this from the sentence and its context, not by reading the codebase. Investigating to disambiguate
is doing the destination's job, and it is also usually futile: no file says which *now* the asker
meant.

Two contextual facts legitimately settle an axis, and both should be stated when used:

- **An open diff.** Unmerged commits on the branch make the *this window* reading live; a clean tree
  makes it nearly impossible. Check, and say what you found.
- **The immediately preceding conversation.** A question following a review is usually about that
  change. Say that you inferred it from context so it can be corrected.

## Step 2 — Ask, but only about what is actually split

If every axis resolved, skip to Step 3 and say the reading in one line. Do not manufacture a modal for
an unambiguous question; the friction would fall on the common case to serve the rare one.

When an axis is genuinely split, call `AskUserQuestion` — one question per split axis, at most two.
Phrase the options as **readings of their question**, never as skill names:

```text
質問: 「今」はどの範囲を指していますか？
選択肢:
  - 外の世界と比べて — 一般的なやり方として今も妥当か
  - このリポジトリ全体 — コードベース全体が筋の通った状態か
  - この窓の変更 — いま書いた差分がどうか
  - どれでもない — 別の意味だった
```

Include a "どれでもない" option. A router that offers only its own three readings will collect one of
them regardless of whether any was right.

## Step 3 — Resolve the destination at runtime

**Do not hardcode a routing table.** Skills are added, renamed, and retired, and a table written into
this file would drift silently — routing to a skill that no longer exists, or missing one that now
owns the answer. Read the installed set instead:

```bash
ls .claude/skills/
head -20 .claude/skills/*/SKILL.md      # frontmatter: name + description + argument-hint
```

Each skill's `description` states what it owns and, at the end, what it explicitly does *not* — that
`Do NOT use it for…` clause is the most reliable routing signal in the frontmatter, because it was
written by whoever owned the boundary. Several skills also carry a `## Contract` table naming Owns /
Never / Starts when / Stops when; read it when the description leaves the call close.

Pass the resolved reading along with the question, plus the destination's own arguments where the
reading determines one — `repo-truth --depth=full` when the asker needs to know whether something
exists at all, `research --stage=dissolve` when they are asking whether the question is even open,
`how-to --mode=lookup` when they wanted the procedure and not the operation.

If no installed skill owns the question, say so plainly and stop. That is a finding — a gap in the
skill set — not a reason to answer it here.

## Step 4 — Hand off

State the destination and why, in one line, then invoke it. The user has already approved the reading;
re-confirming the skill choice is a second gate on a decision they just made.

```text
「この窓の変更」の読みで受け取りました。差分のドメインモデリングは /impl-review が所有します。
```

Handing off is this skill's only output. Do not summarise what the destination will find, do not
pre-answer part of it, and do not run two destinations to compare — the second is not thoroughness, it
is the reading question left unresolved.

## Arguments

| Argument | Effect |
| --- | --- |
| `--to=<skill>` | The caller already knows the destination; skip Steps 1-3 and hand off |
| `--explain` | Show the axis resolution and the destination, and stop without invoking |

`--explain` is the mode for "where would this go?" — a question about the skill set rather than about
the repository. It is also how to inspect a routing decision that felt wrong.

## Not a chain

Routing is the opposite of chaining, and the difference is where the decision sits. A skill that
chains into another decides for the user; this one exists to put that decision in front of them and
then carry it out. That is why the confirmation is not optional when an axis is split, and why the
destination is never invoked twice.

It also means the destinations stay independently invocable. Nothing here makes `question` the
required entry point — a clearly-scoped question should go straight to its owner, and this skill is
for the ones that are not.

## Do / Do NOT

- ✅ Resolve every axis the sentence settles; ask only about the ones genuinely split.
- ✅ Phrase options as readings of the question, not as skill names.
- ✅ Always offer 「どれでもない」.
- ✅ Read the installed skills' frontmatter at runtime to resolve the destination.
- ✅ Pass the resolved reading and the destination's arguments along with the question.
- ✅ Say when an open diff or the preceding conversation settled an axis for you.
- ✅ Report "no skill owns this" as a finding and stop.
- ✅ Answer in Japanese.
- ❌ Answer the question, build a procedure, compare options, or review anything.
- ❌ Pick a reading when an axis is split.
- ❌ Hardcode a routing table, or route to a skill without confirming it is installed.
- ❌ Read the codebase to disambiguate — the ambiguity is in the asker, not the files.
- ❌ Produce a modal for an unambiguous question.
- ❌ Invoke two destinations to cover both readings.

## Checklist

- [ ] Question restated in one line with a reading per axis.
- [ ] Diff state and preceding context checked; any axis they settled stated as such.
- [ ] `AskUserQuestion` used only for genuinely split axes, at most two, with 「どれでもない」.
- [ ] Options phrased as readings of the question, not as skill names.
- [ ] Installed skills' frontmatter read at runtime; destination confirmed to exist.
- [ ] Destination arguments derived from the resolved reading and passed along.
- [ ] Handed off with a one-line reason, or reported that no skill owns it.
- [ ] Nothing answered, compared, or reviewed here.
