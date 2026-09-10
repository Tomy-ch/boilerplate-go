---
name: context-map
description: >-
  Draft and land this repository's Context Map at `docs/design/context-map.md` — the one document that characterises every boundary-crossing contact point with Evans's relationship vocabulary (Customer-Supplier / Conformist / Open Host Service / Published Language / Anticorruption Layer / Separate Ways). It proposes candidate labels with evidence and asks the user to choose; it NEVER assigns one itself. Use it when the Context Map is missing or stale, when a new external dependency lands and the map needs an edge, when someone asks how this system relates to the systems around it, or when the DDD ledger reports `context-map` as uninterpreted — 「コンテキストマップを作って」「外部連携の関係を整理して」「境界の接触点を洗い出して」. Do NOT use it to detect drift between an existing map and reality (`context-map-audit`), to audit DDD patterns against Evans (`ddd-audit`), to write a feature spec (`new-spec`), or to document how a single integration works mechanically (that belongs in `docs/design/*.md` or a package README).
argument-hint: '[--update]'
allowed-tools: Bash(ls:*), Bash(find:*), Read, Write, Edit, Glob, Grep, AskUserQuestion
---

# Context Map

Produces `docs/design/context-map.md`: one page characterising every place this system touches
something outside itself, using Evans's relationship vocabulary for the edges.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## The one thing this skill must not do

**It never chooses a label.** It finds the contact points, states what each one shows, offers the
labels the evidence is compatible with, and stops.

The reason is not caution. Several of Evans's relationships are distinguished by facts that do not
exist in the codebase. Customer-Supplier and Conformist look identical from inside — both are
"we consume their model" — and differ only in whether the upstream team will take our requirements.
Partnership needs a mutual commitment nobody wrote down. Guessing produces a map that reads as
researched and is not, which is worse than no map: **an unlabelled edge invites a decision, and a
wrongly labelled edge closes it.**

Structural facts (does a translating adapter exist, is a contract published as an artifact) can be
read and should be stated as evidence. Relational facts must be asked.

## When to Use

- The Context Map does not exist yet, or an external dependency landed and the map has no edge for it.
- Someone asks how this system relates to the systems around it, or which side owns a shared concept.
- The DDD ledger reports `context-map` as `uninterpreted`.

Do NOT use for:

- Drift between an existing map and reality — `context-map-audit`.
- DDD patterns against Evans's originals — `ddd-audit`.
- Feature specs — `new-spec`; mechanics of one integration — the relevant `docs/design/*.md` or
  package README.

## Step 0. Confirm scope

`AskUserQuestion`, one question:

```text
質問: どこまでを地図に載せますか？
選択肢:
  - 全接触点（既定。境界ポートと外向きの面をすべて）
  - 新規接触点のみ（--update。既存の地図に無い辺だけを足す）
```

With `--update`, read the existing map first and carry its edges forward unchanged; only new contact
points reach Step 3. Never silently re-label an edge a human already decided.

## Step 1. Enumerate contact points (deterministic — do not delegate)

A contact point is a place where this system exchanges a model with something it does not own.
Resolve them from the repository, never from a list written into this file:

```sh
ls internal/usecase/boundary/          # the ports: what the inside asks for
ls internal/infrastructure/            # the adapters: what answers, and to whom
```

Read `internal/usecase/boundary/README.md` and `internal/infrastructure/README.md` at runtime as the
source of truth for what each one is. A port with no external counterpart (a clock, a transaction
manager) is not a contact point — the test is whether a model crosses out of this system.

Include the inbound direction too: the HTTP surface this system publishes, and anything that
consumes what it emits. A map with only outbound edges says this system is nobody's upstream, which
is a claim, not an omission.

Report the enumeration before going further. Getting the node set wrong invalidates every label.

## Step 2. Gather evidence per contact point

For each edge, collect what the repository can actually show:

- **Is there a translating adapter?** A port stated in this system's vocabulary with the conversion
  confined to the adapter is the structural signature of an Anticorruption Layer.
- **Whose vocabulary survives at the boundary?** If external names reach the inside, the edge is
  conformist in effect whatever anyone intended.
- **Is there a published contract artifact?** A committed, consumable schema is the signature of a
  Published Language; an endpoint set designed for many consumers suggests Open Host Service.
- **Which direction does the model flow, and who breaks whom?** State what happens here when the
  other side changes shape.

Cite `file:line`. Evidence the reader cannot check is not evidence.

## Step 3. Propose candidates — never choose

For each edge, present the labels the evidence is compatible with, and say plainly what the evidence
cannot settle. Phrase the gap as the question a human can answer:

```text
<接触点> — 候補: <A> / <B>
  根拠: <file:line と、そこから言えること>
  判別できないこと: <組織的事実。例「上流にこちらの要件を通せるか」>
```

If the evidence is compatible with exactly one label, say so — but still confirm it. A single
compatible label is a strong reading of structure, not a decision about a relationship.

## Step 4. Confirm each edge

`AskUserQuestion` per edge, options being the candidate labels plus 「保留（未確定として地図に残す）」.

**Keep 保留 available and mean it.** An edge recorded as undecided, with its evidence and its open
question, is a correct map entry. It tells the next reader exactly what to find out. Forcing a label
to avoid a blank is how a map stops being trustworthy.

Batch at most 4 edges per call.

## Step 5. Write the map

Write `docs/design/context-map.md` (English canonical), then chain `canonicalize-doc` for the
Japanese pair. Structure:

- **The node set** — this system, and each external counterpart, with one line on what it is.
- **The edges table** — counterpart / direction / relationship label / evidence `file:line` /
  what would change if the other side moved. Undecided edges carry `未確定` and their open question.
- **A mermaid diagram** — nodes and labelled edges, no styling beyond the labels.
- **What this map does not cover** — contact points deliberately excluded, and why.

Keep the prose about *relationships*. How an integration works mechanically belongs to its own
design doc; link, do not restate.

`docs/design/**/*.md` is already inside the DDD ledger's corpus, so the map enters the audit's field
of view the moment it lands. Do not edit the ledger here — say that `/ddd-audit` should re-run for
`context-map`, and stop.

## Step 6. Closing

Report in Japanese: the edges landed, the edges left `未確定` with their open questions, and the
recommendation to re-run `/ddd-audit`. No commit, no push.

## AI Modification Scope

- Read: `internal/usecase/boundary/**`, `internal/infrastructure/**`, `docs/design/**`,
  `docs/adr/**`, per-package `README.md`
- Write: `docs/design/context-map.md` and its `.ja.md` pair only
- Never touch: the DDD ledger, ADRs, source code, generated files, `AGENTS.md`

## Constraints

- ❌ ラベルをスキルが決める（構造から読めるのは候補まで。関係は人が決める）
- ❌ 「未確定」を潰すために推測でラベルを埋める
- ❌ 接触点の一覧を本文へハードコード（boundary / infrastructure を実行時に読む）
- ❌ 連携の仕組みを地図へ書き写す（関係だけを書き、機構は既存の design doc へリンク）
- ❌ 台帳の書き換え、commit / push
- ✅ 辺ごとに `file:line` の根拠と、構造からは判別できないことの明示
- ✅ 出力・地図本文の日本語ミラーは日本語（英語正本 + 隣の `.ja.md`）

## Checklist

- [ ] スコープを `AskUserQuestion` で確認（`--update` は既存の辺を持ち越す）
- [ ] 接触点を boundary / infrastructure から解決し、一覧を先に提示
- [ ] 辺ごとに構造的根拠を `file:line` で収集し、判別できないことを言語化
- [ ] 候補提示 → 辺ごとに人が確定（保留を選べる状態を保つ）
- [ ] `docs/design/context-map.md` を作成し `canonicalize-doc` で日本語ペアを同期
- [ ] `/ddd-audit` の再実行を促して終わる（台帳は触らない）
