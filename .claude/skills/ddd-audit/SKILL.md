---
name: ddd-audit
description: >-
  Audit this repository's DDD interpretation against the original meaning of Eric Evans's patterns, and keep the pattern ledger at `.agents/ddd-audit/pattern-ledger.yaml` in sync with the ADR / README corpus. It emits three-valued verdicts (差異なし / 差異あり / 逸脱宣言あり) and never arbitrates — whether a divergence is deliberate or an oversight stays a human decision. Use it when asked whether the repo's DDD is faithful to Evans, whether a pattern (Aggregate, Value Object, Repository, Factory, Specification, Bounded Context, Anticorruption Layer, Ubiquitous Language, Domain Event ...) has been interpreted at all, when adding or rewriting an ADR or a domain README and you want to know which patterns it touches, when onboarding a reviewer who has not read Evans, or when the ledger has gone stale — 「DDD 原義と照らして」「Evans 的に正しいか」「この概念は解釈済みか」「台帳を更新して」. Do NOT use it to audit Go code against the repo's own rules (`arch-check`, and `type-design-reviewer` for domain type quality), to check README-to-code drift (`back-prop`), or to validate feature specs (`verify-spec`).
---

# DDD Audit

Integrator for auditing the repository's **DDD interpretation** against Evans's original pattern
language. Fans out read-only `ddd-origin-auditor` agents — **one per pattern** — verifies the
findings, then drives the ledger-write loop itself.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## Contract

| | |
| --- | --- |
| **Owns** | 層2（ADR / README）を層1（Evans 原義）に照らす判定と、パターン台帳の同期 |
| **Never** | 裁定（差異が意図か見落としかの決定）/ コードの監査 / 層3 の代替 |
| **Starts when** | 原義照合、パターン解釈の有無、台帳の陳腐化が問われたとき |
| **Stops when** | 差異が意図的な設計選択か否かの判断に達したとき（人間へ） |

## The three-layer model this skill serves

| Layer | What it is | Where it lives |
| --- | --- | --- |
| 1 | Evans's DDD pattern language, project-independent | `.agents/ddd-audit/pattern-ledger.yaml` |
| 2 | How this repository interprets each pattern | `docs/adr/` + per-layer `README.md` + `docs/rules.md` |
| 3 | What enforces the interpretation in code | depguard / golangci / custom analyzers |

This skill audits **layer 2 against layer 1**. Layer 3 is deterministic and already has CI gates —
keep it there. The reason this audit is an LLM and not a linter is that "has this pattern been
interpreted, under any wording?" is irreducibly a reading-comprehension question; a heuristic
linter answering it would be confidently wrong at scale, and putting it behind a CI gate would make
the repo's DDD claim rest on a coin flip.

## When to Use

- Asked whether the repo's DDD is faithful to Evans, or whether a specific pattern is interpreted.
- Adding or rewriting an ADR / domain README and you want to know which patterns it moves.
- Onboarding a reviewer who has not read Evans and needs the same coverage as one who has.
- The ledger looks stale (the corpus changed without it).

Do NOT use for:

- Go code vs the repo's own rules — `arch-check`; domain type quality — `type-design-reviewer`.
- README ↔ code ↔ skill drift — `back-prop`.
- Feature spec validation — `verify-spec`.

## Architecture: fan-out unit is the pattern, not the document

Detection is delegated to the read-only `ddd-origin-auditor` agent under `.claude/agents/`. The
integrator runs instances concurrently via the Agent tool, **one per ledger pattern**.

Fanning out per document is the obvious choice and it is wrong here. "Is Aggregate interpreted?"
cannot be answered by anyone holding one ADR — the interpretation may sit in a README section, under
a different name, three files away. Each auditor therefore owns one pattern and sweeps the whole
corpus for it. Documents get read many times; that is the cost of asking a question whose answer is
distributed.

The auditors are **strictly read-only**: they never write the ledger and never call
`AskUserQuestion`. All writes happen in this integrator, single-threaded, after approval.

## Step 0. Confirm scope

Call `AskUserQuestion` immediately after invocation, with two batched questions.

```text
質問 1: どの範囲を監査しますか？
選択肢:
  - 全パターン（台帳の全エントリ。初回 / 定期棚卸し向け）
  - 未解釈のパターンのみ（status が unexamined / examining / uninterpreted のもの）
  - 中核パターンのみ（scope: core。Evans 第1-3部）
  - 変更文書に関係するパターンのみ（quick。ADR / README を触った直後向け）

質問 2: 検出後に台帳を更新しますか？
選択肢:
  - 更新する（既定） — 1 件ずつ承認を取りながら integrator が台帳へ書き込む
  - 更新しない（read-only） — レポートのみ
```

When chained from `arch-check`, scope arrives pre-set to `quick` — skip this step and do not ask.

## Step 1. Load the ledger and resolve the corpus

Read `.agents/ddd-audit/pattern-ledger.yaml`. It gives you both the pattern list (the fan-out unit)
and the `corpus` globs (what layer 2 consists of). Never hardcode either — the ledger is the SSOT,
and a hardcoded copy here would drift the moment someone adds a README.

Select patterns per the Step 0 scope. For `quick`, resolve the changed corpus files first and keep
only the patterns whose current `interpreted_by` entries point at a changed file, plus every pattern
whose `status` is `unexamined` or `uninterpreted` (neither has pointers, so file overlap can never
select them — and those are the findings that matter most, since a pattern nobody has interpreted is
one nobody is watching):

```sh
BASE=$(gh pr view --json baseRefName -q '.baseRefName' 2>/dev/null || make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }
git diff --name-only "origin/${BASE}...HEAD"
```

An existing pull request's `baseRefName` stays the authority — the changed corpus you select from has
to be the one the PR shows. With no PR, `make base-branch` resolves the latest release line from
`origin`'s live state; `gh repo view --json defaultBranchRef` is not the fallback, because the GitHub
default branch keeps answering with an earlier release line. Stop on an unresolved base: an empty diff
selects no patterns, and `quick` would then exit cleanly having audited nothing.

If the selected pattern set is empty, say so and exit cleanly.

## Step 2. Check ledger staleness (deterministic — do not delegate this)

Before auditing, compare the diff against the ledger's own `corpus` globs. If any corpus file changed
and `.agents/ddd-audit/pattern-ledger.yaml` did not, the ledger is stale by construction:

```sh
CHANGED=$(git diff --name-only "origin/${BASE}...HEAD")
echo "$CHANGED" | grep -q 'pattern-ledger\.yaml' && echo "ledger touched" || echo "ledger NOT touched"
```

This is a pure set comparison, so answer it with the shell rather than an agent — spending a model
call on something `grep` decides is how audits become too slow to run. Report it as a banner on the
final report; it is information, not a gate.

## Step 3. Fan out auditors IN PARALLEL

Spawn one `ddd-origin-auditor` per selected pattern with the **Agent tool**, all in **a single
message with multiple tool calls** so they run concurrently. Pass each:

- `pattern` — the ledger entry `id` (exactly one)
- `mode` — `full` or `quick`
- `files` — for `quick`, the changed corpus files

Each auditor's final message **is** its finding: an Evans premise, one of three verdicts, evidence,
and a proposed ledger entry. Collect them keyed by pattern.

> If `ddd-origin-auditor` cannot be spawned in the current environment, follow
> `.claude/agents/ddd-origin-auditor.md` inline per pattern instead — sequentially, and say so in the
> report, since serial execution changes how long the user waits.

## Step 4. Verify every 差異あり finding

A `差異あり` verdict rests on the auditor's own recall of a book it cannot open, and a
misremembered premise produces a finding that reads exactly like a real one. So do not pass these
straight to the user: spawn one `review-verifier` per `差異あり` finding, in parallel, and have it
attack the premise rather than the conclusion.

Give the verifier the auditor's Evans premise, its verdict, and its evidence, and ask it to answer:

1. Is the stated Evans premise actually Evans's, or later community folklore attributed to him?
2. Does the cited evidence support the verdict, or was a section that does interpret the pattern
   missed because it uses different vocabulary?
3. Is there a deviation declaration elsewhere in the corpus that the auditor did not find?

Findings that come back `REFUTED` are dropped from the report with a one-line note. `CONFIRMED` and
`PLAUSIBLE` survive, labelled — the label is what lets the reader spend their scepticism where it
belongs.

`差異なし` and `逸脱宣言あり` verdicts skip verification: they assert that the corpus already handles
the pattern, and the evidence for that is a citation the reader can check in one click.

## Step 5. Aggregate report (Japanese, read-only checkpoint)

```text
ddd-audit 結果（スコープ: <X>, 対象 <N> パターン）

台帳鮮度: <corpus 変更あり / 台帳未更新 = 陳腐化の疑い | 同期済み>

[差異あり・解釈あり] <n> 件
  <pattern> — <CONFIRMED|PLAUSIBLE>
    Evans 原義: <前提（反証可能な形）>
    根拠: <file:line>
    現状: <別解釈 / 別名で実質解釈済み>

[差異あり・未解釈] <n> 件
  <pattern> — <CONFIRMED|PLAUSIBLE>
    Evans 原義: <前提（反証可能な形）>
    根拠: <参照した範囲と、そこに無かったこと>

[逸脱宣言あり(スコープ外)] <n> 件
  <pattern> — <宣言箇所 file:line と理由の要約>

[差異なし] <n> 件
  <pattern> — <解釈の所在 file:line>

[検証で棄却] <n> 件
  <pattern> — <棄却理由の 1 行>

総計: 差異あり <n>（解釈あり <k> / 未解釈 <m>、うち CONFIRMED <c>）, 逸脱宣言 <n>, 差異なし <n>
```

**`差異あり` は 2 つに割って出す。** 台帳の `status` がそれを決める——`interpreted` なら解釈あり、
`uninterpreted` なら未解釈。verdict だけでは「解釈はあるがずれている」と「そもそも解釈が無い」が
同じ見出しに並び、後者が前者の陰に隠れる。**誰かが取った立場と、誰も埋めていない空白は、読者が次に
取る手が違う**（前者は宣言するか閉じるか、後者はまず誰かが決めること）。台帳ヘッダの `status` ×
`verdict` 表がこの対応の正本であり、報告はその読み方をそのまま反映する。

Report findings as observations. Do not write 「修正してください」「対応必須」「違反」— this repo
advertises DDD alignment, not Evans-strict compliance, and whether a divergence is a deliberate
design choice or an oversight is the maintainer's call. Stating the difference and its evidence is
the entire deliverable; stating what to do about it exceeds what the audit can know.

## Step 6. Per-item ledger update (integrator-side, single-threaded)

Only if the user opted into updating at Step 0. For each finding whose proposed entry differs from
the ledger's current one:

1. `AskUserQuestion` with the options the finding supports — e.g. 「台帳を提案どおり更新」/
   「status のみ更新し逸脱理由は保留」/「不採用として rejected にする」/「今回は触らない」.
2. On approval, show the YAML diff (変更前 / 変更後), then write
   `.agents/ddd-audit/pattern-ledger.yaml` via `Edit`. Set `last_audited` to today's date.
3. Never write an ADR, a README, or source. If the user wants the corpus itself changed — adding a
   deviation declaration to an ADR, say — surface it as their task and stop there. That prose is a
   design statement in the maintainer's voice, and an audit tool authoring it would turn its own
   finding into the record of a decision nobody made.

Write scope is exactly one file: `.agents/ddd-audit/pattern-ledger.yaml`.

After writing, verify the ledger still parses and the entry count is unchanged.

## Step 7. Closing

- 検出は read-only auditor に委譲、検証は独立 verifier、書き込みは integrator 単一スレッド
- 台帳以外への書き込みなし（ADR / README / 実装コードは一切触らない）
- commit / push なし

## AI Modification Scope

- 読み込み: `.agents/ddd-audit/pattern-ledger.yaml`、`docs/adr/`、per-layer `README.md`、
  `docs/rules.md`、`docs/architecture.md`（auditor が実施）
- 書き込み: **integrator のみ**、per-item 承認後に `.agents/ddd-audit/pattern-ledger.yaml` へ
- 触らない: ADR、README、実装コード、生成物、`AGENTS.md`

## Constraints

- ❌ auditor を逐次起動（必ず1メッセージ内で複数 Agent 呼び出し＝並列）
- ❌ ドキュメント単位の fan-out（パターン単位でないと分散した解釈を追えない）
- ❌ `差異あり` を検証なしでユーザーに出す
- ❌ 裁定文言（「修正してください」「違反」「対応必須」）
- ❌ 台帳以外への書き込み、user 承認なしの台帳更新
- ❌ 台帳鮮度の判定を LLM にやらせる（shell の集合比較で足りる）
- ❌ パターン一覧 / corpus を skill 本文にハードコード（台帳が SSOT）
- ❌ `差異あり` を 1 つの見出しにまとめて出す（解釈ありと未解釈が混ざり、空白が逸脱の陰に隠れる）
- ✅ Japanese aggregated report、3 値判定、`file:line` 根拠
- ✅ Evans 原義の前提を finding ごとに明示（読者が反証できる形で）

## Checklist

- [ ] Scope + 台帳更新可否を `AskUserQuestion` で確認（`arch-check` chain 時はスキップ）
- [ ] 台帳から pattern 一覧と corpus を読み込み（ハードコードしない）
- [ ] 台帳鮮度を shell で判定
- [ ] 選択パターンの `ddd-origin-auditor` を **1メッセージ内で並列起動**
- [ ] `差異あり` を `review-verifier` で検証し REFUTED を落とす
- [ ] 集約 Japanese レポート出力（裁定文言なし、`差異あり` は status で 2 見出しへ分割）
- [ ] opt-in 時のみ per-item 承認 → 台帳へ書き込み → パース確認
- [ ] commit / push なし
