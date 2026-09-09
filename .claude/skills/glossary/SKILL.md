---
name: glossary
description: >-
  Draft and maintain the business-vocabulary spec at `docs/spec/glossary.md` — the cross-cutting Ubiquitous Language page that per-feature specs cannot be, because one word meaning two things and two words meaning one only happen across features. It reports the findings a machine can settle — a term with no row, an orphan code symbol the specs never mention, a declared symbol that no longer resolves, one identifier defined in two features — and asks a human to decide each, never choosing the canonical name itself. Use it when adding a feature and its new terms need registering, when someone asks what a business word means or whether two words are the same thing, when the glossary looks stale against the code, or for a periodic vocabulary sweep — 「用語集を作って」「新出用語を登録」「この語の定義は」「orphan を出して」「語彙の棚卸し」. Do NOT use it to write a feature's own spec (`new-spec` / `new-spec-domain`), to validate spec format (`verify-spec`), to audit DDD patterns against Evans (`ddd-audit`), or to detect business vocabulary that leaked into READMEs and ADRs (that is `back-prop`'s glossary detector).
argument-hint: '[--feature <name>]'
allowed-tools: Bash(grep:*), Read, Write, Edit, Glob, Grep, AskUserQuestion
---

# Glossary

Maintains `docs/spec/glossary.md`, the single source of truth for this system's business vocabulary.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## What this skill decides and what it does not

It settles what a machine can settle: which identifiers exist, which are missing a row, which are
defined twice, which no longer resolve. **It never chooses the canonical name and never declares two
words synonymous.**

The split is not caution, it is where the evidence stops. Identifier collision is a string
comparison; whether the two definitions actually *differ* is a reading of prose. Two different words
naming one concept leaves no mechanical trace at all — it surfaces only when a person reads both rows
and recognises them. And which name wins is a decision about how the business will talk, which no
amount of code can supply.

Report the finding, propose wording, and hand the decision over.

## When to Use

- A feature landed and its new terms are not in the glossary.
- Someone asks what a business word means, or whether two words mean the same thing.
- The glossary looks stale against the code.
- Periodic vocabulary sweep.

Do NOT use for:

- A feature's own spec — `new-spec` / `new-spec-domain`; spec format — `verify-spec`.
- DDD patterns against Evans — `ddd-audit`.
- Business vocabulary that leaked into READMEs / ADRs — `back-prop`'s glossary detector.

## Step 0. Confirm scope

`AskUserQuestion`, one question:

```text
質問: どこまで見ますか？
選択肢:
  - 全体（既定。全 feature と domain 全体を突き合わせる）
  - 特定 feature のみ（--feature <name>。新出用語の登録向け）
```

## Step 1. Read the glossary as the baseline (deterministic — do not delegate)

Read `docs/spec/glossary.md`. It gives three things the run depends on, and none of them may be
hardcoded here:

- the existing rows (term, owner, code symbol, public name),
- the **Mechanism vocabulary** section — names already judged *not* to be business terms,
- the **Watch list** — homonyms and synonyms already known.

There are two more suppression sources, and skipping either produces a report nobody finishes
reading.

**Names the spec format declares auto-derived.** `.claude/scaffold-spec/domain-spec.md` names the
families a domain package generates from its fields — errors, bound constants, field identifiers.
They are written by convention, not chosen by anyone, and they arrive in the dozens. Read that file
for those families and subtract them, whole-name matching being useless against a family that shares
a prefix.

**The Watch list.** A term already sitting there has been seen and is awaiting a decision. Proposing
it again is not new information; it is the same question asked twice by something that did not
notice the first was open.

The Mechanism section is the suppression channel. **Subtract it from every orphan set before
reporting.** Match on the bare identifier and match it whole: the section lists names without a
package because the same structural name recurs across packages, and a prefix match would suppress
`DetailInput` on the strength of `Detail`. A sweep that re-proposes the same structural names every
run is a sweep nobody reads, which is the failure this section exists to prevent.

If the glossary does not exist, say so and stop. Creating the container is a design act with rules in
it; this skill fills a container, it does not invent one.

## Step 2. Extract the inventory

Read every one of these at runtime:

```sh
# terms declared by the specs
grep -rn '^package:\|^struct:' docs/spec/domain --include='*.md'

# exported types in the domain — the nouns
grep -rn '^type [A-Z][A-Za-z0-9]* ' internal/domain --include='*.go'

# exported behaviours in the domain — the verbs
grep -rn '^func \(([a-z]* \*\?[A-Z][A-Za-z0-9]*) \)\?[A-Z][A-Za-z0-9]*(' internal/domain --include='*.go'

# exported package-level values in the domain — the named states and roles
grep -rn '^[ \t]*[A-Z][A-Za-z0-9]* \(=\|[A-Z][A-Za-z0-9]* =\)' internal/domain --include='*.go'

# read-side concepts, for features that have no aggregate to introduce them
#   resolve the packages per feature (see below) — do not fix a path glob here
#   same three shapes as the domain sweep: types, behaviours, package-level values
grep -rn '^type [A-Z][A-Za-z0-9]* ' internal/usecase --include='*.go'
grep -rn '^func [A-Z][A-Za-z0-9]*(' internal/usecase --include='*.go'
grep -rn '^[ \t]*[A-Z][A-Za-z0-9]* \(=\|[A-Z][A-Za-z0-9]* =\)' internal/usecase --include='*.go'

# published names — the whole spec, not one directory of it
grep -rn '' openapi --include='*.yaml' -l
```

Excluding `_test.go` and `mock/`. Read `.claude/scaffold-spec/domain-spec.md` at runtime for the
YAML shape rather than assuming it — the section list is that file's to change.

**Do not narrow the type sweep to `struct` and `interface`.** A named slice or a named scalar is a
type the business may have a word for — a collection with behaviour on it, a code, a kind. Narrowing
there is the same mistake as sweeping types and skipping behaviours, one level down, and it silently
drops the one shape a homonym is most likely to take: the same name declared as a struct in one
package and a bare string in another.

**Take the package-level values as seriously as the types.** A named state or role is a `const`, not
a struct, and the code-symbol column already carries them in the `package.Value` shape. An inventory
that omits them cannot produce the very rows the page most depends on, and — worse — it reports them
as resolving while never having been able to propose them.

**The published-name source fills the last column and nothing else.** It does not produce findings —
a term the business uses need not cross the API boundary, so a blank there is normal and a mismatch
is not evidence of anything until someone reads both.

Search the whole of `openapi/`, not a chosen directory. A published name can be a schema, a property
inside a response body, a parameter, or **a path segment** — a verb-shaped operation name is a path
segment and appears in no schema at all. Each narrowing of the search reads as "this term
is not published" when the term is on the wire, and that is the one wrong answer this column can
give.

Resolve each feature's packages from what the spec itself declares:

```sh
grep -rn '^package:' docs/spec/domain docs/spec/usecase --include='*.md'
```

The spec tree carries the package path in its own layout — `docs/spec/<layer>/<rest>.md` is
`internal/<layer>/<rest>` — and the `package:` declaration says the same thing. Read the declaration
anyway: it is the half that can be checked, and a disagreement between the two is a finding
(`verify-spec` owns it).

**Then walk the directories anyway**, because the declaration is not complete, in three ways.

- A spec names the one package it is about. The `query/` sub-package and the boundary port that
  feature reads through are declared nowhere.
- A declaration can be stale. When it points at a directory that does not exist, that is a finding
  in its own right — and the real package still has to be found before the sweep can go on.
- A package can have no spec at all. Build the package set from `docs/spec/domain/**/*.md` +
  `docs/spec/usecase/**/*.md` alone and it is invisible, and **vocabulary nobody wrote a spec for is
  the vocabulary least likely to be right.**

Use the declaration to resolve what it covers and a directory listing to find what it omits;
neither alone reaches everything.

**Subtract the accessors from the behaviour set.** A method that hands a field back says nothing
its own name did not already say — `Name()`, `Email()`, `PublishedAt()`.

The fields are unexported, so no grep above yields them: **open the receiver's source and read the
struct.** Then judge by what the method does, not by string equality with a field name. That test
alone is too narrow to work — a getter may copy before returning, may delegate to an embedded value,
and may spell the field in an abbreviated form. The question is whether the body reaches one field
and returns it; if it does, it is an accessor whatever it is called.

What survives carries a verb or a judgement: `Cancel`, `IsCanceled`, `Update<Attribute>`.

Take the verbs as seriously as the nouns; they are the easier half to lose. **A vocabulary of nouns
can name what the business has and cannot say what happens to it** — and the rules live in what
happens. An inventory drawn from types alone reads as complete while every rule is missing from it.

Do not cut the constructors out mechanically. `New` is Go's word for construction, and when the
business calls the same act something else, that mismatch is a finding worth putting in front of a
person rather than a row to suppress. They dominate a first sweep; the Mechanism
vocabulary section is where the ones that are genuinely construction go, and Step 1 subtracts them
from then on — so on a container that has already been through this, expect the paragraph to apply
to nothing.

### The read side, and the bound on it

The rule elsewhere in this repo is that the domain layer introduces terms and the usecase layer only
uses them. **That rule assumes a domain layer exists.** Under the lightweight CQRS this repo runs,
several features are pure projections — no aggregate, no domain spec, nothing but a QueryService —
and for those, no aggregate is ever going to introduce the word. Sales, ranking, a postal-code
lookup: the business says all of them, and a vocabulary that omits them is not the vocabulary of the
business, whatever else it is.

So take the read side as a source, **bounded to the usecase packages that no domain spec introduces
the words for.** Decide that per usecase spec by resolving its Repository dependencies: read its
`## Dependencies`, take the domain package path each one names (`internal/domain/<X>`, from the entry
or its trailing comment), and look for `docs/spec/domain/<X>.md`. **Never derive it from the usecase
spec's own path** — `internal/usecase/user/search` is spec'd under `docs/spec/usecase/` at the path
mirroring its own package, and depends on `internal/domain/user`, so the paths do not coincide and a same-path lookup would
declare an aggregate-backed search a projection. Where an aggregate does exist, its read models
restate what the aggregate already introduced, and proposing them again produces a second row for one
concept — the exact failure this page exists to catch.

**This bound outranks whichever rule it meets** — the orphan rule and the new-term rule alike. A
read model can sit inside `internal/domain/**`, and be named in a spec's own section list, and so
qualify twice over by location and by declaration while being a restatement by nature. When that
happens it is mechanism vocabulary, not a candidate row.

**Sweep the read side in all three shapes, as on the write side.** The argument for taking verbs
and named values seriously does not weaken because a feature has no aggregate — a projection's
named states (`Ok`, `Degraded`, the period kinds) are exactly the words a dashboard is talked about
in. Reading only the types there would repeat, one layer over, the mistake this section was written
to correct.

The spec's path gives you its own package, and nothing more. A projection that reads the database
puts its types under a `query/` sub-package; one that calls an external system puts them behind a
boundary port, and the two live nowhere near each other — neither is named by the spec. **A glob
aimed at one of those shapes returns nothing for the other and looks exactly like a feature with no
vocabulary.** Walk the directories under the spec's package rather than fixing one path.

Strip the mechanism suffix before judging the name: `Result`, `ReadModel`, `View`, `Input`,
`Params`, `Cursor`, `Summary`, `Breakdown`, `Item`, `List`, `Count`, `DTO` describe the shape of a
query answer or of a DTO, not a thing the business has. Treat that list as a starting set, not a
closed one — strip whatever plays the same role and say which you stripped. What is left is the
candidate, and the suffix itself belongs in Mechanism vocabulary.

Drop the ports outright. A name that *is* `Usecase`, `Gateway`, `QueryService` or `Repository`
names a seam in the architecture; strip its suffix and nothing is left, which is the tell.

A term's owner is the feature the spec belongs to plus the aggregate it declares — read the feature
from the spec's package path, which is what the spec tree's layout now encodes. **A projection has no
aggregate, so its owner is the feature alone** — that is not a missing half, it is
what a term with no aggregate looks like, and forcing one in would attribute the word to a model
that does not define it. Two owners for one term is not a data problem to reconcile; it is the
finding.

## Step 3. Four findings, kept apart

They ask for different things, so do not merge them into one list.

- **新出用語** — declared by a spec, no row in the glossary. Read "declared" as the whole spec, not
  only its `package:` / `struct:` lines — a Value Object named in the spec's own section list is
  declared as surely as the aggregate is. Propose a row with the definition drafted
  from the spec's own prose, and let the human rewrite it. The definition is the part that must not
  read like generated text.
- **orphan** — exported in `internal/domain/**` — a type, or a behaviour that survived the accessor
  cut — or, for a feature with no aggregate, a read-side concept that survived the suffix strip —
  absent from every spec and from the glossary, and not suppressed. This is the one finding that
  catches what nobody wrote down: everything else starts from a document, so a term that was never
  documented is invisible to it. Subtract all three suppression sources here, not the Mechanism
  section alone. Each orphan resolves one of three ways — it becomes a row, it goes to Mechanism
  vocabulary, or it is a naming mistake in the code.
- **解決しない参照** — a declared code symbol that no longer resolves. Two sources, checked the same
  way: a spec's `package` / `struct`, and **the glossary's own code-symbol column**. That column
  takes four shapes and all four must resolve — `package.Type`, `package.Type.Method`,
  `package.Func`, and `package.Value` for a package-level constant or variable, which is how a
  named state or role is usually carried. A checker that knows only the first three reports a
  false defect against the rows that matter most. Deterministic — settle it with `grep`, never with a
  judgement call. The glossary side is the one that matters most and the one nobody was checking:
  the page declares that it governs the code, and **a governing claim never compared against the
  thing it governs is decoration.**
- **同音異義** — one identifier appearing in the code of two features. Read it at the code, not at
  the specs: a spec names its aggregate, so a type the specs never mention by that name still
  collides in the source, and the heaviest collisions in a repo tend to be exactly those. Report
  both definitions side by side and ask whether they are actually the same concept. Never answer
  that question here.

## Step 4. Decide per finding

**Group the variants of one word into one finding before asking.** A read model, its view, its
params and its result are four names for one concept, and asking four times about them is not
thoroughness — it is the same question spread thin enough that nobody answers the fourth. Name the
variants inside the finding so the grouping is visible and can be disputed.

`AskUserQuestion`, batched at most 4 findings per call. Options follow what the finding supports —
for an orphan: 「用語として登録」/「機構語として記録」/「コード側の命名が誤り」/「今回は保留」.

For a new term, present the drafted definition and make rewriting it the easy path. **A definition
nobody edited is a definition nobody agreed to.**

For an unresolved code symbol the options are ordered, and the order is the whole point:

1. 「コード側の改名が誤り。シンボルを表に合わせて戻す」
2. 「業務側で語の扱いが変わった。**先に表の行を改訂し**、そのあとコードを追随させる」
3. 「行が実体を失った（機能ごと消えた）ので行を削除する」
4. 「表とコードは一致しており、ずれているのは spec 側。`verify-spec` へ送る」
5. 「今回は保留」

Option 4 is not a formality. The unresolved reference can come from either side, and when it is the
spec that has drifted, **none of the first three apply** — this skill does not write feature specs,
so offering only row-or-code choices would push the run toward editing something correct.

**Never offer "rewrite the row to match the code" as an option of its own.** It is the cheapest fix
on the table at the exact moment the page is most vulnerable, and taking it turns the glossary into
an index of the code. An index cannot contradict what it indexes, and a vocabulary that cannot
contradict the code can never tell anyone the model is wrong. Option 2 reaches the same end state
through the order the page requires — the language moves first and the code follows.

## Step 5. Write

Write `docs/spec/glossary.md` only. The spec tree is Japanese single-file with English headings and
has **no `.ja.md` pair** — do not create one, and do not chain `canonicalize-doc`.

<!-- sample-api:begin -->
Sample-derived rows live between the `sample-api:begin` / `sample-api:end` markers so they leave with
the sample; a row for a term that survives sample removal goes outside them. Getting this wrong is
how the page starts describing terms that no longer exist.

<!-- sample-api:end -->
Do not touch feature specs, READMEs, ADRs, the DDD ledger, or source. A term that needs renaming in
code is a follow-up to surface, not an edit to make here.

## Step 6. Closing

Report in Japanese: rows added, orphans classified, unresolved references, homonyms left open. Name
the follow-ups that belong to other skills and stop there. No commit, no push.

## AI Modification Scope

- Read: `docs/spec/**`, `internal/domain/**`, `internal/usecase/**`, `openapi/**`,
  `.claude/scaffold-spec/domain-spec.md`
- Write: `docs/spec/glossary.md` only
- Never touch: feature specs, READMEs, ADRs, the DDD ledger, source code, generated files, `AGENTS.md`

## Constraints

- ❌ 正名をスキルが決める（どの名前が勝つかは業務の話し方の決定）
- ❌ 同義を機械が判定する（機械的痕跡が無い。提示までにとどめる）
- ❌ Mechanism vocabulary を差し引かずに orphan を報告する（毎回同じ一覧が出て読まれなくなる）
- ❌ 在庫を型だけから取る（動詞が落ち、業務規則が宿る側が丸ごと見えなくなる）
- ❌ 在庫を書き込み側だけから取る（集約を持たない feature の語を導入する場所が他に無い）
- ❌ 集約を持つ feature の read model を候補に上げる（1 概念に 2 行ができる）
- ❌ 投影の所有列に集約を埋める（定義していないモデルに語を帰属させることになる）
- ❌ 在庫から公開パッケージ値を落とす（`package.Value` 形の行を提案できないまま解決だけ報告することになる）
- ❌ メソッド名とフィールド名の文字列一致だけでアクセサを判定する（コピー・委譲・省略形を取り逃す）
- ❌ 解決しない参照の選択肢を「行 / コード」の二項に閉じる（spec がずれている場合が落ちる）
- ❌ 型の掃き出しを `struct` / `interface` に絞る（名前付きスライス・スカラが落ち、同音異義が最も取りやすい形を逃す）
- ❌ Watch list と自動派生の族を差し引かずに orphan を報告する（検討中の語と規約で生える名前が毎回積み上がる）
- ❌ 公開名を `openapi/` の一部ディレクトリだけで探す（パスセグメントの語が「未公開」と読める）
- ❌ 読み取り側を型だけ掃く（名前付き状態が落ち、書き込み側で立てた主張を 1 層上で破ることになる）
- ❌ 同音異義を spec 宣言だけで探す（spec が名指ししない型の衝突が丸ごと消える）
- ❌ 1 語の別形を別々の findings として問う（同じ問いが薄まり、4 つ目に誰も答えない）
- ❌ 用語表のコードシンボル列を検証せずに済ませる（照合されない統べる主張は装飾）
- ❌ 「表をコードに合わせて書き換える」を単独の選択肢として出す（索引への退化はこの瞬間に開く）
- ❌ 器が無いときに器を作る（規則を含む設計行為であり、このスキルの担当ではない）
- ❌ `docs/spec/glossary.md` 以外への書き込み、`.ja.md` ペアの作成、commit / push
- ✅ 4 種の findings を分けて報告（求められることが違うため）
- ✅ サンプル由来の行は `sample-api` マーカーの内側へ
- ✅ 出力は日本語

## Checklist

- [ ] スコープを `AskUserQuestion` で確認
- [ ] 器を読み、既存行・Mechanism vocabulary・Watch list を取り出す（無ければ停止）
- [ ] spec YAML / domain 公開型 / 公開振る舞い / 公開パッケージ値 / 読み取り側 / OpenAPI から実行時に抽出
- [ ] パッケージの所在は spec の `package:` 宣言で解決し、宣言に無いもの（`query/`・boundary port）はディレクトリ走査で補う
- [ ] orphan から Mechanism vocabulary・Watch list・自動派生の族を差し引く
- [ ] 読み取り側は Dependencies が domain spec に解決しない usecase に限定し、機構サフィックスを落とす
- [ ] レシーバの struct を読み、フィールドを 1 つ返すだけの振る舞い（アクセサ）を差し引く
- [ ] 用語表のコードシンボル列を grep で解決確認
- [ ] 4 種の findings を分けて報告する
- [ ] 1 語の別形を 1 件に束ねてから問う
- [ ] findings ごとに人が判断（定義文は書き換えられる形で提示）
- [ ] `docs/spec/glossary.md` のみ更新、サンプル行はマーカー内側
- [ ] 日本語で報告し、他スキルの担当は名指しして終わる
