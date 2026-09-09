---
name: question
description: >-
  Route ambiguous questions without answering them. Resolve every meaning the wording and immediate conversation actually establish across three independent axes — world (outside practice / the repository as a whole / the current change window), intent (symptom / goal / knowledge / undecided choice), and subject (implementation / tests / comments / documentation / vocabulary / operations) — ask the user only about axes that genuinely remain split, and then hand the resolved question to the installed Codex skill that owns the answer. Use when a question admits two or more reasonable readings, uses scope-free words such as 「今」「どうなの」「大丈夫」, follows an earlier answer based on the wrong reading, or asks which skill should receive it. Do NOT use when the reading is already unique (invoke the owning skill directly), or to answer, explain a procedure, compare options, review work, or inspect the codebase for clues about the asker's intent.
argument-hint: '[question] [--to=<skill>] [--explain]'
---

# Question

Interpret what the asker means, confirm only genuine ambiguity, and hand the question to the skill
that owns the answer. Never answer the question here.

A Japanese reference translation is available at `SKILL.ja.md` in this directory (for human
reference only; it is not loaded as a skill).

## When to Use

Use this skill when:

- A question has two or more reasonable readings.
- The asker does not know which skill owns the subject, or asks where the question belongs.
- A previous run answered a different reading and the question needs re-routing.
- Scope-free language such as 「今」「どうなの」「大丈夫」 leaves the world, intent, or subject open.

Do NOT use it when the reading is already unambiguous. Invoke the owning skill directly. Never use
this skill to produce an answer, procedure, comparison, or review.

## Contract

| | |
| --- | --- |
| **Owns** | 問いの意味論的解釈（世界 / 意図 / 対象の 3 軸）、人間への確認、所有スキルへの引き渡し |
| **Never** | 自分で答える / 手順を組む / 比較する / レビューする / 軸が割れているのに黙って片方を選ぶ |
| **Starts when** | 問いの読み方が一意でないとき、どのスキルが担当か分からないとき |
| **Stops when** | 引き渡し先が決まったとき、または該当するスキルが無いと分かったとき |

## Why This Exists

Another skill can reject a question that plainly arrived at the wrong door. That does not solve a
question that is valid under multiple readings: different owners could each produce a confident,
competent, differently scoped answer, and the answer itself would not reveal that another reading
was possible.

A bare 「今」 is the common case. It may refer to practice outside the repository, the repository as
it stands, or the current change window. The ambiguity exists in the asker's intent, not in a file.
The asker therefore resolves it. Asking is the correct source-of-truth lookup, not a fallback for a
weak heuristic.

## The Three Axes

Resolve the question along these independent axes. Most questions settle all three; questions that
reach this skill usually split on one and occasionally on two.

| Axis | Readings | Signal that it is split |
| --- | --- | --- |
| **World** | 外の世界 / このリポジトリ全体 / この窓の差分 | 裸の「今」「最近」「普通」、または基準を示さない比較 |
| **Intent** | 症状 / 目標 / 知識 / 未決の選択 | 行動や失敗を示す動詞がない「どう」「大丈夫」「どうなってる」 |
| **Subject** | 実装 / テスト / コメント / ドキュメント / 語彙 / 運用 | 複数の対象を包む「品質」「これ」「この機能」 |

State each resolved axis as a reading of the asker's own question before naming any skill. The asker
can correct 「差分ではなくリポジトリ全体の話」 without knowing the tool catalog; asking them to
choose between skill names wrongly assumes that knowledge.

## Step 1 — Resolve What the Question Already Says

Restate the question in one line and give the reading of every axis its wording or immediate context
actually settles. Do not inspect implementation or documentation to disambiguate intent. That would
perform a destination's work and cannot reveal what the asker meant.

Two contextual signals may legitimately settle an axis, and must be disclosed when used:

- **Current diff state.** Unmerged branch commits make the current-window reading plausible; a clean
  tree makes it unlikely. Inspect Git state only, and state what was found.
- **The immediately preceding conversation.** A question directly following a review usually refers
  to that change. Label this as an inference from conversational context so the user can correct it.

## Step 2 — Confirm Only the Split Axes

If all axes resolve, skip confirmation and proceed to Step 3 after stating the reading in one line.
Do not add friction to an unambiguous question.

For each genuinely split axis, ask in the conversational response with numbered choices and wait for
the user's reply. Ask no more than two short questions in one turn. Phrase every choice as a reading
of the user's question, never as a skill name, and always include 「どれでもない」. For example:

```text
「今」はどの範囲を指していますか？

1. 外の世界と比べて — 一般的なやり方として今も妥当か
2. このリポジトリ全体 — コードベース全体が筋の通った状態か
3. この窓の変更 — いま書いた差分がどうか
4. どれでもない — 別の意味だった
```

A router that offers only its own candidate readings will collect one even when none is correct.
The 「どれでもない」 choice prevents that false resolution.

## Step 3 — Discover the Destination at Runtime

Never hardcode a routing table. Enumerate the installed Codex skills and read their canonical
frontmatter:

```bash
find .codex/skills -mindepth 2 -maxdepth 2 -name SKILL.md -print
sed -n '1,/^---$/p' .codex/skills/<candidate>/SKILL.md
```

Use each `description` as the primary ownership signal. Its final “Do NOT use…” clause is especially
reliable because the owner wrote that boundary. When the decision remains close, read the
candidate's `## Contract` table if present. Confirm the destination exists before routing.

The Codex skill tree may temporarily lag its Claude counterpart during synchronization. Route only
against the currently installed `.codex/skills/` canonical files; do not assume a Claude-side skill
is available here. If no installed Codex skill owns the resolved question, report the catalog gap and
stop. A missing owner is a finding, not permission for this router to answer.

Pass the original question, the resolved three-axis reading, and any destination arguments implied by
that reading. Derive those arguments from the destination's current `SKILL.md`; do not preserve
remembered flags from another environment or an earlier revision.

## Step 4 — Hand Off

State the resolved reading, destination, and ownership reason in one Japanese line, then invoke that
installed skill. Do not ask for a second confirmation after the user has confirmed the reading.

```text
「この窓の変更について、テストを評価したい」という読みで受け取りました。この問いを所有する /<skill> に引き渡します。
```

This one-line handoff is the only output this skill owns. Do not preview an answer, summarize what the
destination may find, or invoke multiple destinations to cover unresolved readings.

## Arguments

| Argument | Effect |
| --- | --- |
| `[question]` | Route this question using the three-axis workflow |
| `--to=<skill>` | The caller already knows the destination; verify it is installed, skip axis resolution, and hand off |
| `--explain` | Show the axis resolution, destination, and ownership reason, then stop without invoking it |

Use `--explain` when the user asks where a question would go or wants to inspect a routing decision.

## Not a Chain

This skill routes one resolved reading to one independently invocable owner. It is not a pipeline that
collects several answers. The human owns any genuinely split reading; after that decision, the router
carries it out exactly once. A clearly scoped question never needs to pass through this skill.

## Do / Do NOT

- ✅ Resolve every axis the wording and immediate context settle.
- ✅ Ask only about axes that genuinely remain split.
- ✅ Phrase numbered choices as readings of the question, not as skill names.
- ✅ Always include 「どれでもない」 in confirmation choices.
- ✅ Discover installed owners from `.codex/skills/*/SKILL.md` frontmatter at runtime.
- ✅ Pass the resolved reading and current destination arguments with the original question.
- ✅ Disclose when Git diff state or preceding conversation settled an axis.
- ✅ Report an unowned question as a catalog gap and stop.
- ✅ Produce all user-visible text in Japanese.
- ❌ Answer the question, construct a procedure, compare options, or review anything.
- ❌ Choose a reading silently when an axis is split.
- ❌ Hardcode a routing table or route to a skill that is not installed.
- ❌ Read the codebase to infer the asker's meaning.
- ❌ Ask a question when the reading is already unique.
- ❌ Invoke multiple destinations for one unresolved question.

## Checklist

- [ ] Restated the question in one line with a reading for each axis.
- [ ] Checked only legitimate context signals; disclosed any inference from diff or conversation state.
- [ ] Asked only about genuinely split axes, using at most two numbered questions.
- [ ] Wrote choices as readings and included 「どれでもない」.
- [ ] Read installed Codex skill frontmatter at runtime and confirmed the destination exists.
- [ ] Derived and passed destination arguments from its current canonical file.
- [ ] Handed off with a one-line reason, or reported that no installed skill owns the question.
- [ ] Answered, compared, reviewed, and implemented nothing in this router.
