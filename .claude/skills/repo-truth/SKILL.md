---
name: repo-truth
description: >-
  Answer "how does this repository actually work right now" from its own primary sources, separating what the code and the governing documents state from what you inferred, and naming the gap instead of filling it — including reporting a conflict or an absence as the answer rather than resolving it silently. Use whenever someone asks where something is implemented, what the rule or convention is here, why a design is the way it is, what a term means in this codebase, or whether a procedure exists at all — 「このリポジトリではどうなってる？」「どこで認可してる？」「この規約の正本はどれ？」「そもそも決まってる？」. Reach for it over a plain grep: a document here is named for the concern it owns, so the file that governs a question routinely contains none of its words. Read-only: it never edits, never runs a mutating command, and never repairs the drift it finds. Do NOT use it for a general programming or library question with no repository-specific answer, for a known operational symptom with a documented fix (`repo-ops`), for an undecided design question that needs options compared (`research`), for reviewing a diff (`impl-review` / `test-review` / `comment-sweep`), or for filing what it found (`new-issue`).
argument-hint: '[question] [--depth=quick|full] [--kind=fact|rule|rationale|procedure|vocabulary|history]'
---

# Repo Truth

Answer what is true of this repository *right now*, from its own primary sources, with the reasoning
visible.

A Japanese reference translation lives at `SKILL.ja.md` in this directory (for human reference only;
not loaded as a skill).

## When to Use

- Someone asks where something is implemented, or how a path actually behaves.
- Someone asks what the rule, convention, or authoritative document is here.
- Someone asks why a design is the way it is.
- Someone asks whether a procedure, rule, or term exists at all.

Do NOT use it for a general programming question with no repository-specific answer, for a known
operational symptom with a documented fix (`repo-ops`), for an undecided design question that needs
options compared (`research`), for reviewing a diff (`impl-review` / `test-review` /
`comment-sweep`), or for filing what it found (`new-issue`).

## Contract

| | |
| --- | --- |
| **Owns** | repo 内の現状の事実回答、事実と推論の分離、未定義 / 確認できず の判定 |
| **Never** | repo 外の一般論を主根拠にした将来設計 / 未決の採択 / 変更 / 見つけた drift の修復 |
| **Starts when** | 現状・規約・根拠・語義・存在有無を問われたとき |
| **Stops when** | 二つの権威が矛盾したとき（提起して停止）、索引を通読しないまま断定を求められたとき |

## Why this exists

Two failure modes produce almost every wrong answer about this repository, and neither is fixed by
reading harder.

**The first is answering from memory.** A layer rule, a make target's behavior, a status code — each
feels recallable and each is decided in a file that changed since. An answer stated in a confident
register is acted on; being approximately right is worse than saying you have not checked.

**The second is reading the wrong copy.** The search space here is actively misleading: of roughly
1,000 tracked `*.md`, over 40% are `*.ja.md` translations that `AGENTS.md` forbids reading, and
`docs/portal/**` / `docs/godoc/**` / `docs/db-schema/**` / `docs/openapi/**` / `docs/coverage/**` are
generated copies that lag their sources. They are *tracked*, so `.gitignore`-aware search does not
exclude them, and they rank as well as the original. Locating the governing file is therefore a
separate step from reading it — and it is the step that decides whether the answer is right.

That is why this skill's output separates Evidence from Interpretation. The point is not politeness
about uncertainty; it is that a reader can only overturn a claim whose basis is visible.

## Arguments, and the door check

This skill asks nothing before starting. A modal confirming scope would cost more than most answers
are worth. What varies is expressed as arguments instead, so a chained caller states it rather than
letting this skill infer it:

| Argument | Effect |
| --- | --- |
| `--depth=quick` *(default)* | Skim the owning indexes. **未定義 is unavailable** — an absence can only be reported as 確認できず |
| `--depth=full` | Read the owning indexes in full, which is what earns the right to report 未定義 |
| `--kind=<kind>` | Skip the Step 1 classification; the caller already knows which corpus governs |

`--depth` is the real dial, and it is deliberately the cost of an authoritative absence: reading 110
ADR entries and a README chain is expensive, and nobody should pay it for a question that only needs
a pointer. Escalate to `full` on your own when the answer turns out to hinge on something not
existing — and say that you did.

**Then check you are the right door.** Three skills sit next to each other here and the distinguishing
signal is intent, not vocabulary — the same nouns appear in all three:

| The user is describing | Door |
| --- | --- |
| something that broke, or a gate that failed | `repo-ops` |
| an operation they want to perform | `how-to` |
| a choice nobody has made yet | `research` |
| how something works, what the rule is, whether it exists | here |
| something that could be read as more than one of the above | `question` — the router, which asks |

If this is the wrong door, say so in one line and name the right one. Answering anyway is worse than
mis-triggering, because the answer will be shaped like a knowledge answer for someone who needed a
procedure.

The row that matters most is the last one. A bare 「今」 or 「どうなの」 can mean the world outside, this
repository as it stands, or the diff in this window, and no amount of code reading settles which —
the ambiguity is in the asker. Send it to `question` rather than picking a reading, because picking
one produces a confident answer to a question nobody asked.

Graphify's own description claims any codebase question should be treated as a graph query first, and
`graphify-out/` exists in this checkout, so that claim always applies. It is upstream's wording for an
installed dependency, not this repository's routing: the graph is a **tool this skill uses** (Step 2),
and a repository question still comes here, because a graph result carries no evidence separation, no
frontier, and no 未定義 / 確認できず distinction.

## Step 1 — Classify what is being asked

The classification decides what counts as an answer, and — more importantly — what an absence
*means*. Do this before searching.

| Kind | What answers it | What an absence means |
| --- | --- | --- |
| Fact — how does it behave | the implementation on the request path | the path does not exist; say so rather than describing a plausible one |
| Rule — what should be done here | the governing document | **the rule may be undefined** — a finding in its own right, but only once Step 2 has exhausted the owning index |
| Rationale — why is it this way | `docs/adr/`, `docs/design/` | the decision was made implicitly and never recorded |
| Procedure — how is it done here | `.makefiles/**`, `.lefthook.yaml`, `.github/workflows/`, `docs/maintenance/` | **no canonical procedure may exist** — same bar; never invent a command to close the gap |
| Vocabulary — what does this term mean | `docs/spec/glossary.md`, `docs/spec/{domain,usecase}/` | the term is not business vocabulary, or it has leaked without registration |
| History — when and why did it change | `git log`, merged PRs | — |

A question often carries an unstated assumption about which kind it is. "How do I run only the
integration tests?" is a Procedure question; "why do integration tests need a DB slot?" is a
Rationale question, and they resolve in different corpora.

## Step 2 — Establish the search frontier, then locate the source

This is the step the skill exists for, and the one that decides whether the answer is trustworthy.

**You cannot conclude anything from a file you never opened, and this corpus is too large to have
opened it by accident.** 518 canonical Markdown files (after excluding 495 `*.ja.md` mirrors and 144
generated copies), 110 ADRs, 150 package READMEs, 52 `.mk` files — plus the code. Whatever you read,
most of it stayed unread, so *absence of a hit is not absence of an answer*.

Grep does not rescue this, and `AGENTS.md` says why in as many words: **a document is named for the
concern it owns**, so searching an index for your feature's words is not enough. The file that
governs your question is routinely one whose name does not contain any word in it.

### Search index-first, by concern

Read the indexes and pick entries by *what concern they own*, not by keyword match:

| Index | Covers |
| --- | --- |
| `docs/adr/README.md` | why a decision was made — 110 records |
| `docs/design/README.md` | how a subsystem or cross-cutting concern works |
| `.makefiles/README.md` | every make target, grouped by area |
| the README chain from the path in question up to its layer root | responsibilities, prohibitions, design intent — 150 of them, but only the chain matters |
| `docs/spec/glossary.md`, `docs/spec/{domain,usecase}/` | business vocabulary and per-feature behavior |

`.claude/skills/repo-ops/SKILL.md` section 0, *Finding the authoritative source*, is the canonical
answer-target → source table and carries the noise-free search invocation plus the list of files that
look authoritative but are not. Read it at runtime; do not copy it here, it is maintained there.

Keyword search comes **last**, as a net for what the indexes missed — never as the primary method.

### Record the frontier before concluding

Keep track of what was actually covered: which indexes were read in full, which README chain was
walked, which globs were searched — **and which sweeps you decided not to run, with the reason**.
A frontier listing only what was covered reads identically whether the rest was ruled out or
forgotten, and only one of those is a finding about the repository. This is not bookkeeping — it is what makes an absence falsifiable,
and it draws a line this skill must not blur:

| Verdict | Requires |
| --- | --- |
| **未定義** — no rule / procedure exists | the owning indexes read **in full**, and the README chain walked |
| **確認できず** — could not establish it | anything less |

Downgrading to 確認できず is always available and costs nothing. Reporting 未定義 off a partial sweep
is worse than having no answer, because it reads as a settled fact and the next person builds on it.

Two things `repo-ops` section 0 establishes that this skill must not soften:

- **Never read a `*.ja.md`.** Hitting one is still useful as a *locator* — it proves the topic is
  documented — but read the English original beside it.
- **Precedence when sources disagree** is `AGENTS.md` → `docs/rules.md` → `docs/architecture.md` →
  user instruction, and for design intent and implementation policy it is **README > Code > SKILL**.

### Use the graph where structure beats text

Keyword search fails on exactly the questions this section opened with — a governing document named
for its concern, a caller that shares no vocabulary with the callee. Graphify indexes **structure**,
so it reaches what text search cannot. Use it; do not merely guard against it.

Two commands pay off here (measured on this repository in `.claude/README.md`, not inherited from
upstream's claims — read that section before reaching for anything else):

```bash
# Impact radius / callers — the answer to any "only X does this" claim.
# Takes a node id, so go through the wrapper: it resolves the symbol name and lists candidates.
node .claude/scripts/graph-affected.ts <symbol> --depth 2

# Structural lookup. The default budget (~2000 tokens) truncates on a repo this size and says so —
# raise it, or read the truncation warning, because the answer may be in the cut part.
GRAPHIFY="${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify"
"$GRAPHIFY" query "<question>" --budget 8000
```

`affected` is the paying command: reverse traversal returns each call site with a relation label and
`file:line`, which is precisely the exhaustive-search evidence a scope claim needs. Ignore
`god-nodes` — it ranks by edge count, and this repo's 1:1 test-mapping rule puts test scaffolding
above production code, so it answers a question nobody asked.

Then the guardrails, which do not shrink any of the above:

- **A node, an edge, or a generated summary is never the evidence.** It is how you reached the file;
  open that file and cite it. A graph answer that was not confirmed in source stays Interpretation.
- **State freshness whenever you used it.** Compare the `Built from commit:` line in
  `graphify-out/GRAPH_REPORT.md` against `git rev-parse HEAD`, and say which commit the graph is at.
  For a question about uncommitted work the graph is blind — rebuild
  (`make graphify-update`, which runs the pinned build) or use `grep`, which for a
  small diff is the cheaper of the two.
- **Its absence is normal.** It is gitignored and installed per machine, so fall back to index
  reading, search, `git log`, and direct reading — and record the frontier accordingly, because a
  structural sweep you could not run is part of what was left uncovered.

## Step 3 — Read the primary sources, and mark the seam

Open what you cite. A path you did not read is not evidence.

As you go, keep two piles apart, because they get merged the moment they are written into one
paragraph:

- **Evidence** — a sentence the source actually contains, or behavior the code actually expresses.
- **Interpretation** — anything you concluded by combining sources, by absence, or by analogy with a
  sibling. Inference is legitimate and often the whole value of the answer. Presenting it in the same
  register as evidence is not.

The most common leak is a claim of *scope*: "only X does this" is a claim about absence, and absence
is established by an exhaustive search or not at all. Reverse-traverse the graph
(`node .claude/scripts/graph-affected.ts <symbol> --depth 2`) rather than guessing at call sites; if
neither that nor an exhaustive search was run, narrow the claim to what was actually covered.

## Step 4 — Check currency and conflict

- **Currency.** A governing file may have moved this week. Check history for the paths you are
  citing (`git log --oneline -10 -- <paths>`) when the answer depends on it being current.
- **Conflict.** When two sources that both claim authority disagree, report both with their
  freshness — do not silently pick the one that answers the question. `AGENTS.md`'s *Conflicting
  Authority* section governs this, and it is deliberate: noticing a disagreement is the job,
  resolving one is not.
- **Drift is a finding, not a task.** When code and its README disagree, the README is the governing
  side and the code is the drift; say so and stop. Repairing it belongs to `back-prop`, and
  "correcting" a governing document to match the code is not this skill's call at all.

## Step 5 — Answer in this contract

Always this shape, in Japanese. Keep the Answer short enough to be read first.

```markdown
## 回答
<結論を 1〜3 文で>

## 根拠
- <主張> — `<path>` の `<symbol / target / 節>`
- <主張> — `<path>` の `<symbol / target / 節>`

## 推論
- <根拠から導いたこと。断定と区別できる書き方で>

## 矛盾 / 欠落
- <食い違う出典と、それぞれの鮮度> / <未定義 または 確認できず> / <古い可能性のある記述>
- 探索範囲: <通読した索引 / 辿った README 連鎖 / 検索した glob / 回さなかった掃引とその理由>
  ← 欠落を報告するときは必須

## 確度
High | Medium | Low — <そう判断した理由>

## 次にできること
<追加確認 / research / new-issue / ADR 化 / back-prop など。実行はしない>
```

Cite **symbols and paths, never line numbers** — a line number is stale by the next refactor, and a
reader who follows one to the wrong place trusts what they find there.

Confidence is judged on the sources, not on how sure you feel:

| Level | When |
| --- | --- |
| High | Multiple current primary sources agree, and they govern the question asked |
| Medium | A single primary source, an implicit one, or one whose currency is unverified |
| Low | Mostly inference, sources conflict, or the deciding source could not be reached |

When only part of the question could be answered, return the verified part and name the rest as a
gap. A precise partial answer beats a complete-looking one.

## Gaps are an answer

"There is no canonical procedure for this" is a finding this skill owns, and it exists because the
alternative is worse: an invented-but-plausible command reads exactly like a documented one, and the
next person runs it.

State it as a result, and publish the frontier with it, so the absence is falsifiable:

> 正規手順は**未定義**。`.makefiles/README.md`（全 target）と `docs/maintenance/` の索引を通読し、
> `.lefthook.yaml` / `.github/workflows/` を確認したが該当なし。近いのは `<target>`（ただし〜の点で
> 目的が異なる）。

If the indexes were not read in full, the verdict is **確認できず**, and it says which index was left
unread. The two are not interchangeable: one is a finding about the repository, the other is a
finding about how far this run got.

`repo-ops` deliberately does not carry either verdict — it is a lookup table of known symptoms, and
teaching it to conclude "undefined" would turn it into a general search skill. When a symptom is not
in its index, the question comes here.

## Standalone by design

This skill is invoked in its own right and chains into nothing. It reports what `Next action` would
be — `research` for an undecided design question, `new-issue` to file what it found, `back-prop` for
drift, `repo-ops` for a known symptom — and the user decides whether to run it.

That is the same reason the three review skills are peers under the Review Phase Protocol in
`AGENTS.md`: a skill that runs the next one for you removes that decision from the user, and a drift
in this skill's judgment would then silently redirect every flow that passed through it.

## Do / Do NOT

- ✅ Say so and redirect when this is the wrong door, instead of answering anyway.
- ✅ Search index-first by concern; keep keyword search as the last net, never the first move.
- ✅ Record the frontier — indexes read in full, README chain walked, globs searched.
- ✅ Say 確認できず whenever the owning indexes were not exhausted; reserve 未定義 for when they were.
- ✅ Open every source you cite; cite symbols and paths.
- ✅ Keep Evidence and Interpretation in separate sections.
- ✅ Report a conflict with both sources and their freshness.
- ✅ Report an absence as the answer, with what was searched.
- ✅ Reach for the graph where structure beats text — `affected` for callers and scope claims — then
  confirm what it pointed at in source, and state its freshness.
- ✅ Record a sweep you deliberately did **not** run, with its reason. A frontier that lists only what
  was covered cannot be told apart from one where the rest was forgotten.
- ✅ Answer in Japanese.
- ❌ Answer from memory about anything the repository decides.
- ❌ Read or cite a `*.ja.md`, or cite generated output (`docs/portal/**`, `docs/godoc/**`,
  `docs/db-schema/**`, `docs/openapi/**`, `docs/coverage/**`) as authority.
- ❌ Treat a skill body as authority over a README, or a code fact as authority over a governing
  document.
- ❌ Resolve a conflict between two authorities on your own.
- ❌ Report 未定義 from a keyword search, or from indexes that were not read in full.
- ❌ Invent a command, a rule, or a rationale to close a gap.
- ❌ Edit anything, run a mutating command, or repair the drift you found.
- ❌ Claim a scope ("only X does this") from a partial search.
- ❌ Write line numbers into the answer.

## Checklist

- [ ] Door check done — a symptom goes to `repo-ops`, an operation to `how-to`.
- [ ] `--depth` resolved; 未定義 claimed only under `full`, and any escalation to `full` stated.
- [ ] Question classified; what an absence would mean is settled before searching.
- [ ] Indexes read by concern before any keyword search; `repo-ops` section 0 read at runtime.
- [ ] Frontier recorded — which indexes in full, which README chain, which globs.
- [ ] No `*.ja.md` read; generated trees excluded from search.
- [ ] Every cited source actually opened; symbols and paths, no line numbers.
- [ ] Evidence and Interpretation separated; scope claims backed by exhaustive search.
- [ ] Currency checked where the answer depends on it; conflicts reported with both sources.
- [ ] Graph accounted for either way — used where structure beats text (`affected` for any scope
      claim) with its output confirmed in source and its freshness stated, or deliberately not run
      with that stated in the frontier alongside the reason.
- [ ] Contract emitted in full, in Japanese, with Confidence and its reason.
- [ ] 未定義 used only on exhausted indexes; otherwise 確認できず, naming what was left unread.
- [ ] Gaps stated as results with the frontier attached; nothing invented to fill one.
- [ ] Nothing edited, nothing run that mutates, no chained skill invoked.
