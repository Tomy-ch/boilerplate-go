---
name: comment-sweep
description: >-
  Sweep the EXISTING STOCK of source-code comments in a chosen scope and decide whether each comment's content is in the right place — the jurisdiction question no other reviewer asks. Beyond shortening it can relocate a design rationale into `docs/adr/` / `docs/design/**` / `docs/spec/**` / a package README, and collapse a Why written at several declarations to one authoritative site plus pointers. Use it whenever comments feel bloated, verbose, over-explained, or essay-like even though each line is individually true; whenever the same reason appears at several declarations and no one place is authoritative; whenever a doc comment has grown into a design argument, threat-model analysis, or rejected-alternative discussion; for a periodic hygiene sweep of a package / layer / whole repo; before a large PR or a template cut; and when someone asks 「コメントが長すぎる」「コメントを整理して」「この Why はコードに置くべきか」「コメントを ADR に移したい」. Modes: 確認して適用 (default), 自動適用 (`--apply`), 報告のみ (`--report-only`). Sole owner of the comment subject — no review skill carries a comment lens — and a peer of `/impl-review` and `/test-review` under the Review Phase Protocol in `AGENTS.md`, asked for separately. Do NOT use it to judge README / docs prose quality (`doc-reviewer`), to fix README-to-code structural drift (`back-prop` / `sync-readme`), or to delete `// Name は、〜です。` field comments — that convention is deliberately preserved.
---

# Comment Sweep

Judge accumulated comments on one question the existing reviewers cannot ask: **does this content
belong here?**

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## When to Use

- Comments in a package read as bloated / essay-like even though nothing in them is wrong.
- A doc comment has grown into a design argument (rejected alternatives, threat model, architecture policy).
- Periodic hygiene sweep of a package, a layer, or the repo.
- Before a template cut, where accumulated commentary becomes downstream reading burden.

Do NOT use for:

- README / `docs/**` prose quality — `doc-reviewer`.
- README↔code structural drift — `back-prop` / `sync-readme`.

## Why this skill exists (read this before judging anything)

The repo already forbids How-narration, 経緯, and restatement. Comments kept growing anyway. Two
structural reasons, and understanding them is what makes this skill work:

1. **The argument to keep always beats the argument to cut.** "This Why is non-obvious and
   verifiable" is a concrete claim with explicit permission in `docs/rules.md`. "Volume is a cost"
   is an abstract one. Concrete beats abstract every time, so every judgment call resolves toward
   keeping, and the stock only grows. You will not win by re-arguing volume — do not try.
2. **Review is diff-scoped.** A review of a change never re-examines what is already there. There is a
   path in and no path out.

So this skill does not re-litigate whether a Why is good. It asks a **different, equally concrete
question** — one that can actually beat "but it's non-obvious":

> **If someone reversed this decision, which document would they be obliged to update?**

Write it there, link to it from the code, and keep in the comment only the **operative residue**: the
one or two sentences a person editing *this* declaration must not violate. When the honest answer is
"no document — the constraint exists only at this call site", the code **is** the jurisdiction and
the comment stays in full. This is a relocation skill, not a deletion skill.

### The second question: what one comment at a time cannot see

Jurisdiction is asked of a single comment, and that leaves a blind spot with the same shape as the one
above. When the same Why is written at three call sites, **each copy passes the jurisdiction test
independently** — each is non-obvious, each sits at the site whose premise it states, each is
individually defensible. Judged one at a time they are three 維持. The redundancy is only visible when
the surrounding comments are read as one body, so a per-comment pass cannot find it no matter how
carefully it is run.

So every audit asks a second question of its **whole assigned package's** comment stock as a unit:

> **Is this content already carried at another declaration in this package, and if so, which single
> site owns it?**

Three shapes answer to it, and none of them is reachable per comment:

- **Scattered duplication** — one Why restated at several declarations. One site owns the concept; the
  rest shrink to a pointer.
- **Fragmentation** — a constraint split across declarations so that no single place states it, and a
  reader has to assemble it. The fix is to make one site whole, not to add a fourth fragment.
- **Aggregate over-explanation** — every comment is individually correct, yet the package's total
  commentary costs more to read than the code it explains.

This is the same trap as the diff-scope one, one level down: the argument to keep each copy wins every
time it is asked in isolation, so nothing ever consolidates. Ask it of the set instead.

### The third question: what one package at a time cannot see

The trap recurs at the next level out, and it is the reason the second question is asked of a package
rather than a file. A Why repeated across *packages* passes the second question in every auditor
independently, because each auditor sees only its own scope. Nobody is looking at the relation.

This is not hypothetical. A sweep of this repository found the same sentence, verbatim, at six
declarations in three packages; each auditor could only report its own two or four copies as separate
findings, and the run that produced the sentence had written it six times without ever being asked
once whether it belonged in the code at all.

**Only the integrator sees every package, so the third question is the integrator's** (Step 2.5). It
is asked mechanically rather than by judgment, because the auditors report 維持 as a count and their
content is therefore not comparable across reports:

> **Does the same comment line appear at declarations the auditors will judge separately?**

A cluster found this way is not automatically a 集約. Resolve it by jurisdiction first, and the answer
is usually different from the within-package case:

- **The repeated content's jurisdiction is a document** — then it is **移設 at every site** (or 短縮,
  when the document already says it). No declaration owns a concept that spans packages, so there is
  no site to consolidate into. What detecting the cluster buys is that N independent "keep" judgments
  become one visible decision.
- **One declaration genuinely owns the concept and the others can name it** — then it is a 集約 whose
  members span files. The pointer must name the owning declaration, because a reader in another
  package cannot find it by proximity.

Limit worth stating: a mechanical scan finds repeated *lines*, so it catches verbatim repetition and
misses paraphrase. Verbatim is the dominant shape — the same sentence gets copied, not re-derived —
and a scan that never claims to find paraphrase is more useful than a judgment call nobody performs.

## Authoritative sources — read at runtime, hardcode nothing

| Question | Source of truth |
| --- | --- |
| What may a comment contain; the jurisdiction clause | **Comment Rules** in `docs/rules.md` |
| Where does relocated content belong (decision / exclusion / rule / inventory) | the *What belongs here* table in `docs/adr/README.md` |
| What a subsystem design reference is for | `docs/design/README.md` |
| Package-level scope | the nearest ancestor `README.md` |

Read these at the start of every run and apply them verbatim. They may have changed; a remembered
version is not good enough. If anything in this file disagrees with `docs/rules.md`, `docs/rules.md`
wins.

### Relocating is not dumping — each destination has an entry bar

A relocation only helps if the prose lands where that *kind* of knowledge is owned. A document that
accepts everything answers nothing, and `docs/adr/` is the one most at risk of becoming the default
bucket, because from inside a comment almost anything reads as "design rationale". Two misroutes to
refuse outright:

- **A library's or an API's specific behavior** (this driver returns X on Y, this SDK reads that env
  var) is not a choice among alternatives — it is a property of the thing being called, and it
  changes when the dependency is upgraded. Its home is the comment at the call site. Verdict: 維持.
- **Business / domain knowledge** (what a rule means, why a status transitions this way) belongs to
  `docs/spec/**`, where the behavior is specified and kept current. Never route it to an ADR.

`docs/adr/` takes only a **choice among alternatives with lasting consequences** or a deliberate
exclusion. When a candidate fits no destination, that is evidence the code was the right place all
along — return 維持 rather than forcing a home. Proposing a bad destination is worse than proposing
nothing, because a wrong move is much harder to undo than a comment left alone.

## Step 0 — Confirm scope and apply mode

One `AskUserQuestion` call carrying **two** questions. Skip whichever question a flag or a caller
already answers it; skip the call entirely when both are fixed.

- 「comment-sweep の対象スコープを選んでください」
  - 「指定パス配下（パッケージ / ディレクトリを続けて指定）」
  - 「ベースブランチとの diff で変更されたファイル」
  - 「レイヤ全体（`internal/domain` / `usecase` / `controller` / `infrastructure` / `pkg` から選択）」
  - 「キャンセル」
- 「検出結果をどう適用しますか？」
  - 「1 件ずつ確認して書き換える」 ← 既定
  - 「そのまま書き換える（1 件ずつの確認をしない。文書書き込みを伴う移設は対象外）」
  - 「報告のみ（書き込まない）」

Stock scope is the point of this skill. Diff scope exists so a large refactor can be swept without
naming every package by hand — but even then the subject is the **whole comment stock of the files the
change touched**, never the changed lines alone. A file judged in pieces cannot answer the second
question, and the duplication this skill exists to find lives between the pieces.

### Apply modes

| Mode | Selected by | What Step 4 does |
| --- | --- | --- |
| 確認して適用 | the default option, or `mode: confirm` | per-item approval, then write — every verdict is reachable |
| 自動適用 | `--apply`, or the second option | writes 短縮 / 削除 / high-confidence 集約 with no per-item question; a 移設 needing a document write is reported, not applied |
| 報告のみ | `--report-only`, or `mode: report` | Step 3 renders the findings and the run ends; nothing is written |

### Flags

- `--apply` — 自動適用. Fixes the mode, so the mode question is not asked.
- `--report-only` — 報告のみ. Detect and report; never write.
- Both at once is a contradiction, not a precedence puzzle: say so and fall back to the mode
  question rather than silently picking one.

## Step 1 — Resolve targets

Exclude generated files and tests; they are not this skill's business:

```sh
find <scope> -name '*.go' \
  ! -name '*.gen.go' ! -name '*.sql.go' ! -name '*_mock.go' ! -name '*_test.go'
```

Non-Go sources (shell, Dockerfile, Makefile, SQL, YAML, `.mjs`) are in scope for the same content
standard — `docs/rules.md` says so explicitly, and they are higher-risk because `revive` does not
see them. Include them when they fall under the chosen scope.

Rank by comment volume so the sweep starts where the payoff is, and tell the user the ranking:

```sh
for f in <files>; do
  printf '%s %s\n' "$(grep -c '^[[:space:]]*//' "$f")" "$f"
done | sort -rn | head -30
```

Group the resolved files by package directory — that is the fan-out unit, because jurisdiction is a
per-package judgment (a package's own README is one of the candidate destinations).

## Step 2 — Fan out read-only auditors in parallel

For each package group, spawn one auditor via the **Agent tool** (`subagent_type: general-purpose`),
all in a **single message with multiple tool calls** so they run concurrently. Give each:

- the package directory and its resolved file list
- the instruction to read `references/audit-prompt.md` in this skill's directory and follow it verbatim
- the repo-root-relative paths of the authoritative sources above

`references/audit-prompt.md` is the auditors' single source of instructions — do not paraphrase it
into the spawn prompt, or the two will drift and the auditors will disagree with each other.

Each auditor runs **both passes** described in *Why this skill exists* — the per-comment jurisdiction
question and the per-package stock question — over the same files it has already read. The second pass
costs reading no extra material; what it adds is a question, and the findings it produces (verdict
**集約**) name a *set* of comments rather than one. Hand each auditor any cross-package cluster from
Step 2.5 that touches its files, so it judges those comments knowing they are repeated elsewhere.

Auditors are **strictly read-only**. They surface verdicts with evidence and a proposed landing
form; they never call `AskUserQuestion` and never write. Approval and every write happen in this
integrator, single-threaded, so parallel auditors cannot contend.

If subagents cannot be spawned in the current environment, follow `references/audit-prompt.md`
inline per package instead; the rest of the flow is unchanged.

## Step 2.5 — Scan for repetition across packages (integrator, mechanical)

Run this in the same message as the fan-out, before the auditors report. It is the third question from
*Why this skill exists*, and it belongs here because no auditor can see another auditor's scope.

Collect the comment lines of every resolved file, normalise away leading markers and indentation, drop
lines shorter than a clause, and report any text that appears at declarations in **more than one
file**:

```sh
for f in <resolved files>; do
  grep -hE '^[[:space:]]*(//|#|--)' "$f" \
    | sed -E 's@^[[:space:]]*(//|#|--)[[:space:]]?@@' \
    | awk -v f="$f" 'length($0) > 30 { print f "\t" $0 }'
done | sort -t$'\t' -k2 \
  | awk -F'\t' '{ n[$2]++; src[$2] = src[$2] "\n    " $1 }
                 END { for (k in n) if (n[k] > 1) print "[" n[k] "] " k src[k] }'
```

The `grep` is load-bearing: without it the pipeline clusters code and blank lines too, and every run
reports one enormous meaningless cluster. That is not a hypothetical — it is what the first draft of
this step did.

The exact pipeline matters less than the property: it is **deterministic and cheap**, so it runs on
every sweep rather than when someone suspects duplication. Tune the length floor to the scope — too
low and boilerplate field comments dominate, too high and a one-line Why slips through.

Each cluster is then resolved by the rule in *The third question*: jurisdiction first (usually 移設 /
短縮 at every site), 集約 across files only when one declaration genuinely owns the concept. Clusters
whose members all sit in one package belong to that package's auditor; carry the rest yourself.

**A cluster is a finding even when every member is individually correct.** That is the whole point —
each copy already passed jurisdiction on its own, which is why nobody had noticed.

## Step 3 — Aggregate (read-only checkpoint)

Show the full surface before any decision, so the user sees the shape of the sweep rather than being
walked through unbounded one-by-one questions:

```text
comment-sweep 検出結果（scope: <X>, 対象 <n> ファイル / <m> パッケージ）

[<package>]  維持 <a> / 短縮 <b> / 削除 <c> / 移設 <d> / 集約 <e>
  ...（各 finding: 場所・対象コメント・判定・根拠・着地形）

移設先の内訳: docs/adr/ <p> 件 / docs/design/ <q> 件 / パッケージ README <r> 件
集約: <e> 件（対象コメント計 <t> 箇所 / 内訳 重複 <u> ・分散 <v> ・総量過多 <w>）
パッケージ横断の重複: <x> クラスタ（対象コメント計 <y> 箇所 / <z> パッケージにまたがる）
総 finding: <sum>（うち要判断 <k>）。<確認して適用のときだけ「これから 1 件ずつ確認します。」を続ける>
```

Count a 集約 finding **once**, not once per member comment — it is one decision. Report the member
count alongside it so the size of the edit is visible before anyone approves it.

`維持` findings are reported as a count only — they need no decision, and listing them in full buries
the ones that do. If nothing needs action, say so plainly and stop.

**In 報告のみ mode the run ends here**, and the aggregate above is not enough on its own. Render every
non-`維持` finding in full — the evidence, the comment before and after, and for a 移設 the exact prose
proposed for the destination — because no approval loop follows to reveal them one at a time. Close by
saying how to act on the report: re-run with `--apply` for the 短縮 / 削除 / 集約, or in 確認して適用 for
those plus the 移設. For a 集約, render every member comment, not just the site that keeps the content —
a reader cannot judge a consolidation from the winner alone.

## Step 4 — Apply (integrator-side)

Not reached in 報告のみ mode. Between the other two the write itself is identical; what differs is who
approves it, and how much of the verdict set is in play.

### 自動適用 — no per-item question

Apply **短縮**, **削除**, and **集約** as the auditor landed them, in one pass, and report what was
applied. Three exclusions come off that set first:

- **A finding whose comment contradicts the code** (`誤り/陳腐化`) is reported, never applied. Which
  side is wrong — the comment or the code — is not a comment-cleanup call, and deleting the comment
  can erase the only surviving evidence of a bug.
- **`追記なし` 移設 is applied only after the integrator opens the destination and confirms the content
  is actually there.** With a human in the loop that claim is checked at approval time; unattended,
  an auditor that misread a section would strip the rationale from the code and point the residue at
  a document that never says it. When the check fails, report the finding instead of applying it.

- **A 集約 is applied only at `確度: high`, and only when its members share one file.** 短縮 risks the
  wrong wording at one site; a consolidation additionally picks *which declaration owns the concept*,
  and it has already shrunk the other sites by the time a wrong pick becomes visible. That is markedly
  harder to undo, so anything the auditor itself rated `medium` or `low` is reported for 確認して適用
  instead of applied. A 集約 whose members span files is withheld for the same reason one level up:
  the pointer has to name a declaration a reader cannot reach by proximity, and whether that
  declaration is really the owner is exactly the judgment a no-question mode cannot make.

A `追記なし` 移設 that survives the check is applied: the destination already states the content, so the
finding is really a 短縮 to the residue plus a link and touches no document. A same-file 集約 likewise
writes no document — it only moves content between comments — which is why it belongs to this mode
at all.

**Do not apply a 移設 that would write to a destination document.** Report those with their count and
proposed landing form, and say that 確認して適用 is where they land. The reason is not caution in
general, it is the ADR question below: whether a rationale becomes a new record or a rewrite of an
existing one is a repository-policy call under the immutability rule in `docs/adr/README.md`, and a
mode whose contract is "no questions" has no way to ask it. Keeping that one question alive would
break the contract; answering it silently would settle a policy question by generator.

Every guard in this file still holds — an exported Go declaration's doc comment is rewritten rather
than deleted, functional directives are untouched, and the out-of-scope list is out of scope.
自動適用 removes the question, not the rules.

### 確認して適用 — per-item approval

For each non-`維持` finding, in descending impact order:

1. Present the finding with its evidence and the concrete landing form — show the comment **before**
   and **after** as a diff, and for a 移設 also show the exact prose that will be added to the
   destination document. A verdict the user cannot see the result of is not reviewable.
2. `AskUserQuestion` with the options the auditor surfaced (typically 移設 / 短縮 / 削除 / 維持 / 判断を保留).
   A **集約 is one question covering the whole set**, never one question per member. Splitting it
   produces incoherent outcomes the user never chose — approve the deletions but not the surviving
   site and the Why is gone; approve the survivor but not the deletions and nothing consolidated.
   Offer 集約 / 維持（現状のまま） / 別の site を本体にする / 判断を保留, and show every member.
3. On approval, write in this order — **destination document first, code second**. Reversing it
   creates a window where the rationale exists nowhere, and if the run is interrupted there, the
   reasoning is simply gone. When the auditor found the destination **already states the content**
   (`追記なし`), there is no document write: the finding lands as a 短縮 to the residue plus a link,
   and only the code changes.
4. If the destination is an **ADR**, do not decide the ADR's shape yourself. `docs/adr/README.md`
   declares accepted ADRs immutable, so a new rationale can land either as a new record or as a
   rewrite of an existing one, and which is right is a repository-policy call, not a code-cleanup
   call. Ask: 「既存 ADR-NNNN を書き換える」 / 「新規 ADR を起こす」 / 「ADR ではなく docs/design か README へ」 / 「今回は移設しない」.
   Whichever is chosen, the English canonical file and its `.ja.md` translation — plus the log table in
   `docs/adr/README.md` and `docs/adr/README.ja.md` — are updated in the same change.
5. If the destination is a package README, and the addition materially changes what that README
   claims, mention that `back-prop` is the right follow-up to check the README against code reality.

In this mode, never batch-apply without per-item confirmation. The judgments here are close calls by
construction: an obvious one is caught by `revive` or by reading the diff, and what is left over is
the stock nobody re-reads.

## Step 5 — Verify

Run this only when something was written. 報告のみ has nothing to verify; 自動適用 needs it most,
because no human read the edits one at a time.

- `make go-fix` then `make go-lint` over the touched packages — `revive exported` will catch a doc comment
  deleted where the convention requires one.
- `make md-lint` when a Markdown destination was written.
- Re-read each edited comment once: does the residue still stand on its own for someone who does not
  follow the link? A residue that only makes sense after reading the ADR has been cut too far, and
  that failure is invisible to every linter.
- After a 集約, read every file it touched top to bottom rather than each edited site in isolation —
  the finding was about a body of comments, so the check has to be too. Two failures show up only this
  way: the surviving site does not actually carry what the shrunk ones gave up, and a pointer names a
  declaration a reader cannot find from where they are standing. When the members spanned packages,
  read the pointer from the *other* package's side: a reference that is obvious next to the owning
  declaration is often unnavigable from three directories away.

## Explicitly out of scope

- **`// Name は、〜です。` field comments** — `// Limit は、取得件数の上限です。`, `// StatusCode は、HTTP ステータスコードです。`.
  These are name restatements and carry little information, but the repo deliberately keeps one line
  per field for visual uniformity, and that call has been made. The bloat this skill exists to fix
  lives in long-form Why, not here. Do not flag them, do not propose deleting them, and do not
  reopen the question as a side note — it has already been decided.
- **Package overviews** — a `// Package …` comment, wherever it lives. Usage and How belong there and
  `docs/rules.md` exempts them. Note that this repository has **no `doc.go`**: all 300-plus overviews
  sit at the top of an ordinary source file, so an exclusion phrased as a filename excludes nothing.
  A 集約 does not get to reopen this from the other side: an overview is not a landing site for
  consolidated content either. When the fragments really do add up to a package-level statement, that
  is a 移設 to the package README, which is already a supported destination and is where a reader
  looking for package-level prose goes.
- **Generated files** (`**/*.gen.go`, `*.sql.go`, `*_mock.go`) and `*_test.go`.
- **Functional / directive comments** — `//go:generate`, `//nolint`, `//go:build`, `//go:embed`,
  `// Code generated ... DO NOT EDIT`, shebangs, tool directives. These are not prose.
- **Docs prose quality** — `doc-reviewer` owns it. This skill only *adds* to a destination document;
  it does not audit what is already there.

## Standalone by design

This skill is invoked in its own right, never from inside another review skill. `/impl-review` audits the change and `/test-review` the tests; the three are peers under the Review Phase Protocol in `AGENTS.md`, each asked for separately, and none of them delegates to another. A review skill that offers to run the next one makes the three subjects stop being independently answerable and lets one skill's drift silently drop the others from every flow that went through it.

That independence is also what keeps the sweep stock-level. Nothing hands it a diff, nothing filters which comments its auditors may read, and nothing removes a comment from a package before the stock pass sees it — so the duplication that lives *between* comments stays visible, whether it sits in one file, one package, or across three. A sweep that received only changed regions would be a second diff review wearing the word "stock".

The comment subject therefore has exactly one owner. When a diff's newly added comments need judging, they are judged here, as part of the file they now live in.

## Relationship to the existing reviewers

| | Unit judged | Verdicts | Owns |
| --- | --- | --- | --- |
| `doc-reviewer` | `README*` / `docs/**` | content findings | quality of docs prose |
| **`comment-sweep`** (this skill) | **one comment, a package's whole stock, and repetition across packages** | **維持 / 短縮 / 削除 / 移設 / 集約** | **jurisdiction — where content belongs, and which single site owns it** |

No review skill carries a comment lens any more, so this is where the whole subject is answered —
both the comments a change just added and the ones the file was already carrying, judged together as
the body they now form.

All user-visible output — findings, questions, proposed prose, summaries — is written in **Japanese**
per `CLAUDE.md`.
