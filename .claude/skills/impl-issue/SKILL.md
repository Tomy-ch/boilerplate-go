---
name: impl-issue
description: >-
  Drive a GitHub issue from environment setup to a merged PR as a semi-automatic pipeline whose stopping points are enumerated rather than judged. Use whenever the user hands over an issue URL or number to be worked end-to-end ("この issue やって", "wt 上で解決しよう", "着手して PR まで"), or asks to resume such a run. It owns three things — progress orchestration, reconciling the approved plan against what was actually built, and mechanically detecting the moments needing a human call — and no implementation judgment: the work is delegated to `commit` / `submit-pr` and to the three peer review skills `impl-review` / `test-review` / `comment-sweep`, and design decisions are surfaced, never taken. It sets up an isolated worktree and DB slot, then builds the written plan in three stages — framing, research and draft, and a review whose necessity is derived from one invariant: the plan is seen by a model that is not the implementer's — and holds it for the user's approval before coding, then watches five mechanical trip-wires so drift becomes visible instead of silent. The plan's approval covers the whole run, and the skill carries a closed list of the five places it may stop — everywhere else it continues and records the call for the PR. Runtime verification (`make serve` + curl + LGTM traces) runs after the PR is opened and gates the merge; green CI is not a substitute. Four modes are confirmed once, in two back-to-back calls that are still one stop: review mode, issue mode, flow mode, and a plan mode that prices the planning phase so a one-line fix and a cross-layer feature do not buy the same budget. Do NOT use for a change with no issue behind it (`commit` + `submit-pr`), for reviewing an existing diff (`impl-review` / `test-review` / `comment-sweep`), or for authoring skills (`manage-skill`).
argument-hint: '<issue-url-or-number> [--review-mode=all|harmful|issues] [--issue-mode=search|file] [--flow=record-on-tripwire|halt-on-tripwire] [--plan=full|draft-review|single]'
---

# Impl Issue

Semi-automatic issue → PR pipeline. The machine handles progression, bookkeeping, and detection; the
human keeps every judgment call. A long autonomous run stops being a black box because each departure
from the approved plan surfaces when it happens rather than at the end.

The commands live here so a run is reproducible from this file alone. Detail that drives itself stays
behind pointers: `.makefiles/README.md` (target registry), `docs/maintenance/db-worktree-pool.md`
(slot pool), `docs/development-flow.md` (per-change-type flows).

A Japanese reference translation lives at `SKILL.ja.md` in this directory (for human reference only;
not loaded as a skill).

## When to Use

- The user hands over an issue URL / number and wants it taken to a merged PR.
- The user asks to resume a run that stopped at a decision point.

Do NOT use it for a change with no issue behind it (`commit` + `submit-pr` directly), for reviewing an
existing diff (`impl-review` / `test-review` / `comment-sweep`), or for authoring skills
(`manage-skill`).

## Contract

| | |
| --- | --- |
| **Owns** | issue → merged PR のパイプライン進行、承認済み計画と実物の突き合わせ、人間判断が要る瞬間の機械的検出 |
| **Never** | 未決の設計を独自に補完する / 実装判断そのもの（委譲先が持つ） |
| **Starts when** | 採択済みの issue が提示されたとき |
| **Stops when** | 下表の 5 箇所だけ。それ以外では停止せず、判断は PR コメントへ記録する |

## Stopping — the complete list

Where this pipeline stops is a specification, not a judgment. It stops here and nowhere else:

| # | Where | What is decided |
| --- | --- | --- |
| 1 | Step 0 | The four modes, in two back-to-back calls, before anything else |
| 2 | Step 3 | Approval of the written plan |
| 3 | Step 4 | A trip-wire whose row says halt |
| 4 | Step 7 | Which of the three peer review skills to run, each with its estimated return |
| 5 | Step 8 | Runtime verification failed; and the merge itself |

Three moments look like stopping points and are not. Each is where an unlisted stop otherwise creeps
in:

- **A phase boundary.** The Step 3 approval covers Steps 4–9, because the plan enumerates the whole
  run and that is what was approved. A phase ending is not an event.
- **A subagent's completion notification.** Reviews and audits fan out; a report arriving is where
  work resumes, not where it pauses.
- **A mode settled in Step 0.** That is spent authority. Re-confirming a fix which review mode already
  authorized asks the user to approve the same thing twice.

Asked one at a time, a stop always looks cheap while its cost is diffuse, so "ask" wins every
individual judgment. That is why the list above is closed rather than advisory.

## What this skill does NOT do

It holds no implementation judgment. It never decides which design to adopt, whether a reviewer is
right, or whether a finding deserves an issue. It routes those to the user and records the answer.

| Work | Owner |
| --- | --- |
| Commit splitting and execution | `commit` |
| Push + PR create/update | `submit-pr` |
| Review of the change itself | `impl-review` |
| Review of the tests | `test-review` |
| Review of the comment stock | `comment-sweep` |
| The implementation itself | you, following the approved plan |

The three review skills are peers: none invokes another, and each is asked for separately (Step 7).

## AI Modification Scope

`AGENTS.md` confines AI edits to `internal/` / `pkg/` / `database/` / `openapi/` and treats everything
else — `.github/workflows/`, `docker/`, `scripts/`, `docs/`, `.makefiles/`, root dotfiles — as out of
scope. **Invoking this skill is the explicit user instruction that relaxes that**, because this skill
is issue-generic: the issue decides the surface, and an issue about CI, tooling, container images, or
documentation cannot be resolved inside the four default directories.

The relaxation is bounded, and the bound is the plan:

- The Step 3 plan's **Files to touch** section is the permitted surface. A sensitive path outside the
  four default directories must appear there **before** it is edited, named explicitly rather than
  implied by a glob.
- Say so when presenting the plan, so the user approves the sensitive paths knowingly rather than
  discovering them in the diff.
- Reaching a sensitive path the plan does not list halts under either flow mode (trip-wire 1′,
  Step 4). Ask; do not widen the surface and report it afterwards.

Hard-protected even during this skill (never touch, regardless of what the issue asks):

- `AGENTS.md` / `CLAUDE.md`
- Generated files: `**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, and generated content
  under `docs/` (`docs/openapi/**`, `docs/coverage/**`, `docs/db-schema/**`, `docs/godoc/**`,
  `docs/portal/docs.json`, `docs/portal/guides/**`). Regenerating through a `make` target is fine;
  hand-editing is not.
- Anything under `permissions.deny` in `.claude/settings.json`
- Existing files under `database/migrations/**` (new migration files only)

## Step 0 — Confirm the four modes (two consecutive `AskUserQuestion` calls)

Ask before anything else, in two back-to-back calls: the three run-policy modes, then the plan mode.
Two calls rather than one because they answer different questions — how the run behaves, and what the
planning phase costs — and because a single call caps at four questions, which would leave no room to
ever add a fifth mode. **This is still one stopping point.** The user answers both without the run
doing anything in between; a second dialog is not a second stop.

Defaults are marked; the user's choice always wins.

### First call — run policy

**Review mode** — what happens to a review finding.

| Mode | Confirmed finding | Everything else |
| --- | --- | --- |
| `all` | Apply, even if the change is large | — |
| `harmful` *(default)* | Apply only what is clearly harmful within the change's scope | Route to issue mode |
| `issues` | Apply nothing | Route to issue mode |

**Issue mode** — how an unrelated finding becomes tracked.

| Mode | Behavior |
| --- | --- |
| `search` *(default)* | Search existing issues first; on a duplicate, comment there instead of filing |
| `file` | File without searching |

`issues` × `file` produces the most new issues of any combination. Before executing it, show the count
and confirm — a review easily yields a dozen findings, and a dozen new issues is itself noise.

**Flow mode** — what a trip-wire does. The names carry their trigger because this mode governs
trip-wires only; it is not a posture for the run, and it reaches none of the other four stopping
points.

| Mode | Behavior |
| --- | --- |
| `record-on-tripwire` *(default)* | Record the call and continue; surface every recorded call in one PR comment at the end |
| `halt-on-tripwire` | Stop at that trip-wire and ask |

**Neither mode reaches the trip-wires marked halt in Step 4.** Those are architecture, domain and
policy decisions, which `AGENTS.md` keeps behind a human gate unconditionally. The mode decides only
what happens at the remaining rows: `record-on-tripwire` continues and records them,
`halt-on-tripwire` asks about them too — worth picking when the user is present and wants scope
growth surfaced as it happens rather than at the end.

### Second call — plan mode

**Plan mode** — how much the planning phase spends. Step 3 has three stages; this decides which of
them run. **Every mode satisfies Step 3's invariant** (the plan is seen by a model that is not the
implementer's) — that is not what is being traded away here. What varies is how many passes the plan
gets, and how many of those land on a frontier tier.

| Mode | Stages | Cost |
| --- | --- | --- |
| `full` *(default)* | 3a + 3b + 3c | Three passes — the drafter's, plus two more on a top tier |
| `draft-review` | 3b + 3c | Two passes; the framing stage is skipped |
| `single` | 3b only, drafted by a model that is not the implementer's | One pass |

**Say what the mode costs when you ask, not just what it does.** This is the one mode whose price is
paid every run regardless of the issue's size, so a default that is never priced is a default that
gets paid by accident. A one-line documentation fix and a cross-layer feature do not deserve the same
planning budget, and the user is the only one who knows which this is before Step 3 has read anything.

Recommend `full` when the issue spans layers, changes a contract, or names a design decision;
`draft-review` when the shape is already settled and only the details need working out; `single` when
the change is small enough that the plan is a formality — and say which one you are recommending and
why, rather than presenting three unpriced options.

## Step 1 — Kickoff

```bash
printf '\033]0;%s\007' "<issue-number>-<slug>"   # label the window so parallel runs stay distinguishable
gh issue view <n> --json number,title,body,labels,state,comments
```

**Compare the issue against the actual base before writing anything.** An issue body is a snapshot of
the repo as it was when someone wrote it; line numbers, "X does not exist yet", and "Y has no consumer"
go stale. Verify each factual claim against the base you are about to branch from.

Then post a kickoff comment recording branch name, base commit, isolation method, and — most
importantly — **every discrepancy found above**. This marks the issue as taken and gets the
corrections to the user while they are still cheap.

```bash
gh issue comment <n> --body-file <file>
```

## Step 2 — Secure the environment

Do this before any code is touched, so nothing lands in a shared checkout. Which half applies depends
on whether the worktree exists yet — setting one up and resuming into one are different operations,
and running the first against an already-live worktree destroys work.

### Initial setup — no worktree yet

```bash
# 1. Resolve the active release line off origin's live state.
BASE=$(make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }

# 2. Branch from current origin, not a stale local ref.
git fetch origin "$BASE"
git worktree add -b feature/<n>-<slug> ../go-boilerplate.worktrees/<n>-<slug> "origin/$BASE"

# 3. Lease a DB slot: own databases (wt<N>_local / wt<N>_test), API port 8080+N, mock-auth 2010+N.
cd ../go-boilerplate.worktrees/<n>-<slug> && make slot-acquire

# 4. A fresh worktree has no vendor/ and air builds with --mod=vendor, so serve would fail without this.
go mod vendor
```

`make base-branch` reads `origin`'s live state. Use nothing else: the local `refs/remotes/origin/HEAD`
is fixed at clone time and `git fetch` never updates it, the GitHub default branch stays on an earlier
release line, and the harness's own "Main branch" line reports that same stale symref. All three answer
without warning, so a branch cut from a generation-old base looks correct until the files everyone
expects turn out to be missing.

If `slot-acquire` reports failure, run `make slot-status` before retrying — the lease often succeeded
even when the command errored.

**Never release the slot on your own, and do not offer to during cleanup.** A slot is cheap to hold
(the lease is reclaimed automatically once stale) and expensive to lose mid-task; only the user knows
when the work is really over.

If the user's instruction named a release version other than the resolved one, ask before branching —
a deliberate backport target is the one case the resolver cannot know about.

### Resuming into an existing worktree

Setup already happened. Observe it and report what you found; do not re-run any of it.

The session-start environment line (`[agent-env] checkout=… branch=… vendor=… db-slot=…`) already
carries the three facts that matter — which checkout this is, whether `vendor/` exists, and which DB
slot is held. When it is present, state those back before continuing rather than re-deriving them.

Do not depend on it being there. A hook can be absent, disabled, or attached to a harness that never
ran it, and the fallback has to be inspection rather than repair. Every command below only reads:

```bash
git rev-parse --show-toplevel
test -d vendor && echo 'vendor: present' || echo 'vendor: absent'
cat .gobp-db-slot 2>/dev/null || echo 'slot: none'
```

A missing or malformed slot is a fact to report, not a fault to fix on the spot. Lease one with
`make slot-acquire` immediately before DB-backed work actually begins — the first `make test`,
`make serve`, or `psql` — and not before. `go mod vendor` is the same: run it when `vendor/` is absent
and a build is imminent, not as a resume ritual.

**Never acquire a slot or reinitialize a database because a session resumed.** `slot-acquire` and the
`db-*-reinit` targets rebuild the slot's databases from scratch, so a reflexive resume-time acquire
destroys the state belonging to the very run it is resuming.

## Step 3 — Plan, then wait

**The invariant: the plan is seen by a model that is not the implementer's.** The three stages below
are one default way of satisfying it, not the rule — read the rule off the session's own model rather
than off a model name written here.

No later gate re-opens the plan: `impl-review` / `test-review` / `comment-sweep` all take the finished
change as their subject, so whether the plan solves the issue at all is checked here or nowhere.
Drafting it well and drafting it unbiased are different jobs, so they run as separate stages. **The
three add no stopping point** — the approval at the end is the same single wait.

| Stage | Runs on | Produces | Runs in |
| --- | --- | --- | --- |
| 3a Framing | a tier at or near the top, on a family that is not the implementer's | the questions the plan must answer — nothing else | `full` |
| 3b Research and draft | the strongest tier available for research and drafting, as a subagent | the plan file | every mode |
| 3c Plan review | the same model as 3a; in `single`, the invariant is carried by 3b instead | findings, appended to the plan file as their own section | `full`, `draft-review` |

**Resolve each stage's model at runtime; do not read one off this file.** Model ids here are
`<family><generation>`, and the session states which model is running it, so both facts this section
needs — which family is the implementer's, and which tier is above which — are available when Step 3
executes. A name written into a skill is a snapshot of a roster that changes; the properties above do
not. Under `single`, 3b's model must be a family that is not the implementer's, because it is then
the only pass the plan gets.

### 3a — Framing

Hand it the issue body, the Step 1 issue-vs-base discrepancies, and the paths you have already read.

**It returns open questions, not answers.** It has read almost nothing of the repository at this
point, so anything it asserts is a generality — and a generality handed to a stronger drafter anchors
rather than widens. Ask it what must be decided and where a plan of this shape usually misses
something. Refuse a draft plan, a recommendation, or a
direction if it returns one.

### 3b — Research and draft

Run it as a subagent, so the research happens in a window that carries none of the orchestrator's
accumulated framing. Give it the issue, your Step 1 corrections, the paths you have already read, and
3a's questions. Tell it to verify your summary rather than trust it.

It owes an answer to **every** 3a question — either how it decided, or that the question does not
apply here.

The plan is a written artifact, not a chat message, because Step 5 compares against it mechanically.
Write it under the repo's gitignored `tmp/` (it may be a symlink to a directory outside the repo if
the operator prefers). It must contain:

| Section | Why it is required |
| --- | --- |
| Files to touch | Step 5 diffs this against `git diff --name-only` |
| Per-step deliverables | Lets a partially-finished run be resumed or handed over |
| Chosen options **and rejected ones, with reasons** | Trip-wire 2 fires when a rejected option is later adopted |
| Gate table | Fixes at plan time whether runtime verification is required, so it cannot be quietly dropped |

### 3c — Plan review

Whether it is required is derived, never assumed:

- **3b's model is the implementer's** — the ordinary case, where the strongest tier for drafting is
  also the one running this session. The plan has been seen by no other model yet, so **3c is
  required.**
- **3b's model already differs from the implementer's** — the session runs on a family that is not
  the strongest drafter. The invariant is satisfied the moment 3b finishes, and 3c is optional: run
  it when the change is large enough to be worth a second pass, skip it when it is not.

Append its findings to the plan file as their own section and **present them beside the plan, not
folded into it** — a reviewer that silently rewrote the plan would hide the disagreement at the moment
the user is being asked to approve it. The user arbitrates each finding at the approval below.

### Then wait

Present the plan and **wait for approval. Do not implement before it.**

**That approval covers Steps 4–9.** The plan enumerates the whole run, so no phase inside it needs
approving again; the run continues to the next stopping point on its own.

## Step 4 — Implement, watching five trip-wires

The plan is approved and implementation begins — the boundary between deciding and building, which
only this skill knows:

```sh
.agents/closed-loop/marks.sh planApprovedAt 2>/dev/null || true
.agents/closed-loop/marks.sh implStartedAt 2>/dev/null || true
```

Follow the approved plan. These triggers are deliberately mechanical — relying on you to *notice* that
a decision was significant is exactly how drift goes unreported.

| # | Trip-wire | Default | Why |
| --- | --- | --- | --- |
| 1 | Touching a file the plan does not list, **inside** the four default directories | Record | Scope grew, but within the surface `AGENTS.md` already permits |
| 1′ | The same, **outside** them (`docker/`, `scripts/`, `.github/`, `docs/`, `.makefiles/`, root dotfiles) | **Halt** | The plan is the permitted surface; widening it is the user's call |
| 2 | Choosing an option the plan rejected, or a third one | **Halt** | The rejection had a reason; overriding it silently discards that reasoning |
| 3 | A lint/CI failure rooted in an architecture rule (`interfacebloat`, `gocognit`, `depguard`, architest, …) | **Halt** | These are not formatting — satisfying them changes the design |
| 4 | Rejecting a reviewer's finding, or applying a different fix than proposed | **Halt** | A finding can be correct while its proposed fix is harmful; that judgment is not yours alone |
| 5 | Skipping a gate | Record | Step 6 already requires stating it in the PR |

**Halt rows halt under either flow mode** — they are the human gate `AGENTS.md` places on architecture,
domain and policy decisions. When one fires, present the situation with your recommendation.
`halt-on-tripwire` extends that treatment to the Record rows; `record-on-tripwire` logs them and
continues, and Step 9 surfaces every recorded call in one PR comment.

### When code generation is blocked

The generation make targets wrap `docker compose run … make <target>-ci`, and the `-ci` halves run on
the host with mise-installed tools. When the container runtime is unavailable, call them directly:

```bash
make merge-dml-ci work-dir="."     # DML concatenation
make sqlc-generate-ci              # sqlc
make gen-bundle-oapi-ci            # OpenAPI bundle
make gen-api-docs-ci               # docs/openapi/index.html
cd <pkg> && mockgen -source=<f>.go -destination=mock/mock_<f>.go.gen.go -package=mock_<pkg>
```

Only `make dump-schema` truly needs the container, and only when a migration was added.

Two traps. `merge-dml-ci` runs `go run ./cmd/`, so a Repository method added before its query exists
deadlocks the build — stub the implementation for the duration of generation, then restore it (`cp`
the file first). And the embedded-spec generator's `//go:generate` line points at a container path, so
invoke it with the real one:

```bash
cd internal/controller/httpstack/oapi/validator \
  && oapi-codegen --package=gen --generate=spec -o ./gen/validate.gen.go <repo>/openapi/openapi.gen.yaml
```

Changing an OpenAPI description alone still moves three artifacts: the bundle, `docs/openapi/index.html`,
and that embedded spec. Miss one and CI's generate checks fail.

## Step 5 — Reconcile the plan against reality

Run this before the gates. Compare:

- `git diff --name-only` against the plan's file list — report additions and untouched entries.
- Options actually taken against the plan's chosen/rejected lists.
- Gate table entries against what you actually ran.

Present the deltas. A long run drifts for good reasons; the problem is drift the user never saw. If
nothing drifted, say so in one line and move on.

## Step 6 — Local gates

`make fix`, then `make lint` / `make test`. When many worktrees are active these may be left to CI,
but **say in the PR that they were not run locally**. Silence reads as "verified".

Runtime verification is deliberately *not* here. It belongs after the PR exists (Step 8), so CI runs
in parallel with it instead of after it.

## Step 7 — Review

A completed change has three review subjects, each owned by one skill: `impl-review` (the change),
`test-review` (the tests), `comment-sweep` (the comment stock of the touched files). They are peers —
none invokes another — so this step must not silently pick one.

Follow the Review Phase Protocol in `AGENTS.md`: **estimate each skill's return from the context this
run already holds** — which layers the change touched, whether tests or comments moved at all, what an
earlier pass already covered — then ask the user per skill, stating that estimate and its reason, and
run what they approve. "Shall I run all three?" is not a question; it hands the cost back unpriced.

This step is where the estimate is cheapest to make: the plan, the diff, and the Step 5 reconciliation
are already in hand.

Handle findings per the review mode from Step 0. Auto-application is confined to what is
machine-checkable — formatting, lint fixes, comment-quality findings, regenerated artifacts. **A fix
that changes the design is always a decision point**, even under review mode `all`: `all` authorizes a
large rewrite, not an unreviewed one.

Then present every recorded trip-wire and deferred judgment together, in one place. Batching beats
trickling: the user sees the shape of the whole run at once.

## Step 8 — PR, then runtime verification, then merge

When this step merges the pull request, stamp it — a merge performed here is observed by nobody
else until the loop goes back to `gh` for it:

```sh
.agents/closed-loop/marks.sh mergedAt 2>/dev/null || true
```

Open the PR first via `submit-pr`, so CI starts while you verify locally.

### Runtime verification — the merge gate

Exercise the real HTTP path against the running system. No mode relaxes this.

```bash
make serve                                    # API on 8080+N, mock-auth on 2010+N

TOKEN=$(curl -fsS -X POST http://localhost:201N/default/token \
  -d 'grant_type=password' \
  -d 'client_id=go-boilerplate-client' \
  -d 'password=unused' \
  --data-urlencode 'username=<seeded-subject>' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')

curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $TOKEN" http://localhost:808N/v1/...
```

`docs/design/auth.md` is canonical for this — the mock provider's standard token endpoint,
minting a token without a browser. Where this snippet and that document disagree, the
document decides.

The token subject must be the identity `subject` string the seed registered, not an internal UUID —
the seeded UUID rows belong to a different issuer than the one the slot's port produces, so a UUID
yields a confusing 401. Resolve real subjects from the identity table when unsure:

```bash
docker exec gobp-shared-database-1 psql -U postgres -d wt<N>_local -c \
  "select subject from <identity table> where issuer = 'http://localhost:201N/default';"
```

Check the happy path, the error paths the change introduces, and — for a protected operation — that
omitting the token gives 401. Then **read the traces in the LGTM stack and confirm the request took
the path you expect** (controller → usecase → infrastructure, with the SQL you intended). A response
code alone does not prove the request reached the layer you changed; a wrong-but-plausible route
produces the right status for the wrong reason.

**Green CI is not a substitute.** Review lenses and CI checks are static analysis or tests that stop
at the database layer, so a documented status code the middleware never lets the request reach passes
all of them. One real HTTP request settles it.

When runtime verification cannot run at all, there are two honest options and no third:

1. Do not merge yet.
2. Add an integration test driving the same HTTP path, and merge on that.

Say plainly which one you took.

### Merge

Wait for CI without burning the session on a foreground sleep loop — run a background command that
exits once the checks settle, so one notification arrives:

```bash
until [ "$(gh pr checks <n> --json bucket --jq '[.[]|select(.bucket=="pending")]|length')" = "0" ]; do sleep 30; done
gh pr checks <n>
```

Then `gh pr merge <n> --merge`.

## Step 9 — Close out

Close the issue **manually** — auto-closing keywords do not fire when the PR targets a release branch
rather than the default branch:

```bash
gh issue comment <n> --body-file <handover> && gh issue close <n>
```

The handover comment covers what was decided, what surprised you relative to the plan, and what was
deliberately left undone.

Route findings that fall outside the change to the tracker per the issue mode. Under `search`, prefer
a follow-up comment on an existing issue over a new one — the issue count is itself a cost, and a
duplicate buries the original.

**Verify a finding against the running system before filing it.** A finding derived purely from
reading code can be wrong in a way static review cannot catch — most often because a layer outside
the one being read (middleware, DI wiring, the database) already handles the case. Step 8's runtime
stage is usually enough to check.

Finally, record in a PR comment any call not already visible in a commit message or the PR
description. Every trip-wire recorded rather than halted on lands here.

## Delegating without double-asking

Sub-skills ask their own questions. Since this skill already settled them with the user, pass the
answers as a payload so the sub-skill skips its own gate.

| Sub-skill | Pass through | Suppresses |
| --- | --- | --- |
| `commit` | The grouping you already presented | Its grouping-approval question |
| `submit-pr` | That a review already ran; the push decision | Its Phase 0 review prompt and push confirmation |
| `impl-review` | Scope, reviewer model | Its Step 0 |
| `test-review` | Scope, reviewer model | Its scope question |
| `comment-sweep` | Scope **and apply mode** | Its scope and apply-mode questions |

**Every row is required, because a missing one reinstates a gate this skill already settled.** A
sub-skill whose default is to confirm per item — `comment-sweep` is the one to watch — will do exactly
that when its apply mode does not arrive, and the omission is invisible until the questions start.

Asking the user the same thing twice trains them to approve without reading, which defeats the
decision points this skill exists to create.

## Do / Do NOT

- ✅ Secure the worktree and slot before touching code.
- ✅ On resume, inspect the worktree and report what you found — path, `vendor/`, slot.
- ✅ Verify the issue's claims against the actual base, and put the discrepancies in the kickoff comment.
- ✅ Get the plan approved before implementing, and keep it as a file so Step 5 can diff against it.
- ✅ Put the plan through a model that is not the implementer's, and present that review's findings
      beside the plan rather than folded into it.
- ✅ Treat the five trip-wires as mechanical triggers, not as things to notice.
- ✅ Stop only at the five listed places; record every other call for the PR comment.
- ✅ Pass every sub-skill its settled answers, apply mode included.
- ✅ Say explicitly which gates ran and which did not.
- ✅ Read the traces, not just the status code.
- ✅ Verify a finding at runtime before filing an issue for it.
- ❌ Merge a change to implementation code that has never been exercised over HTTP.
- ❌ Present green CI as runtime verification.
- ❌ Auto-apply a fix that changes the design, in any mode.
- ❌ Ask for approval at a phase boundary, or treat a subagent's completion as one.
- ❌ Let the framing stage return a plan, a recommendation, or a direction instead of questions.
- ❌ Pick which review skills run, or run one on the assumption another chains it.
- ❌ File an issue without checking for an existing one, unless issue mode says to.
- ❌ Release the DB slot, or ask about releasing it, unprompted.
- ❌ Acquire a slot or reinitialize a database merely because a session resumed.
- ❌ Poll CI in a foreground sleep loop.

## Checklist

- [ ] Four modes confirmed in Step 0's two calls, with plan mode's cost stated when it was asked.
- [ ] Kickoff comment posted, including issue-vs-base discrepancies.
- [ ] Environment secured on the right half of Step 2: a new worktree created from a freshly fetched
      base with a slot leased and `go mod vendor` run — or an existing one observed and reported,
      with no slot acquired and no database reinitialized just because the session resumed.
- [ ] Plan built through the stages plan mode selected, every 3a question answered when 3a ran, all
      four sections present, seen by a model that is not the implementer's, and approved before
      implementation.
- [ ] Trip-wires handled per their row's default and the flow mode; nothing silently absorbed.
- [ ] No stop outside the five listed places.
- [ ] Plan reconciled against the actual diff.
- [ ] Local gates run, or their delegation to CI stated in the PR.
- [ ] The three review skills each estimated and put to the user; the approved ones run with their
      answers passed through. Auto-application confined to machine-checkable fixes.
- [ ] Decision points presented together.
- [ ] PR opened, then runtime verification (curl + traces) completed — or its absence stated together
      with which of the two options was taken — before merging.
- [ ] Issue closed manually with a handover comment; unrelated findings routed per issue mode, each
      verified before filing; remaining judgment calls recorded in a PR comment.
