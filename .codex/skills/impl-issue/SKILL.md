---
name: impl-issue
description: >-
  Drive a GitHub issue from environment setup to a merged PR as a semi-automatic pipeline whose stopping points are enumerated rather than judged. Use whenever the user hands over an issue URL or number to be worked end-to-end (「この issue やって」「wt 上で解決しよう」「着手して PR まで」), or asks to resume such a run. It sets up an isolated worktree and DB slot, builds a written plan and holds it for the user's approval before coding, then runs to a merged PR, stopping only where its closed list says and recording every other call for the PR. It owns orchestration and no implementation judgment: the work is delegated to `commit` / `submit-pr` and to the peer review skills `impl-review` / `test-review` / `comment-sweep`, and design decisions are surfaced, never taken. Do NOT use it for a change with no issue behind it (`commit` + `submit-pr`), for reviewing an existing diff (`impl-review` / `test-review` / `comment-sweep`), or for authoring skills (`manage-skill`).
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
| **Owns** | Progressing the issue → merged PR pipeline, reconciling the approved plan with the implementation, and mechanically detecting when human judgment is required |
| **Never** | Fill in unresolved design decisions independently / make implementation judgments (delegatees own them) |
| **Starts when** | An accepted issue is presented |
| **Stops when** | Only at the five places listed below; everywhere else it continues and records the call for the PR comment |

## Stopping — the complete list

Where this pipeline stops is a specification, not a judgment. It stops here and nowhere else:

| # | Where | What is decided |
| --- | --- | --- |
| 1 | Step 0 | The four modes, in two back-to-back calls, before anything else |
| 2 | Step 3 | Approval of the written plan |
| 3 | Step 4 | A trip-wire whose row says halt |
| 4 | Step 7 | Which of the three peer review skills to run, each with its estimated return |
| 5 | Step 8 | Runtime verification failed; and the merge itself |

**Stopping means ending the turn, not merely asking a question.** If the list is read only as a ban
on asking the user, it can be obeyed in form while being broken in effect: a phase completes, a
progress report is the natural response, and the turn ends there. No approval was requested, so the
Do / Do NOT prohibition never fires, but the run is over and its remaining work has been returned to
the user — exactly the cost the closed list exists to prevent. Apply the five rows to turn endings:
unless one of them applies or the run is finished, the reply does not end.

Progress reports are not the problem; they keep a long run legible. Making one the final act of the
turn is the problem. Write the report, then continue working in that same turn. The phrases
`続けます` and `次は〜します` signal this bug rather than a plan: they show that the next step is
already known and therefore not blocked. Perform that step instead of announcing it.

Three moments look like stopping points and are not. Each is where an unlisted stop otherwise creeps
in:

- **A phase boundary.** The Step 3 approval covers Steps 4–9, because the plan enumerates the whole
  run and that is what was approved. A phase ending is not an event. Neither is a seam: write the
  run record, recommend compacting, and continue. At the PR seam, ask first through Codex's explicit
  user-input interaction, but that question is about context rather than the work; under standing
  full delegation while the user is away, announce the recommendation and continue instead.
- **A delegated agent's completion notification.** Reviews and audits fan out; a report arriving is
  where work resumes, not where it pauses.
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
documentation cannot be resolved inside the four default directories. This is a documented,
non-loophole exception per the "Skills must not be a loophole" clause in `AGENTS.md`.

The relaxation is bounded, and the bound is the plan:

- The Step 3 plan's **Files to touch** section is the permitted surface. A sensitive path outside the
  four default directories must appear there **before** it is edited, named explicitly rather than
  implied by a glob.
- Say so when presenting the plan. The user approves the sensitive paths knowingly, not by discovering
  them in the diff — a plan that quietly widens the scope is the failure this clause exists to prevent.
- Reaching a sensitive path the plan does not list halts under either flow mode (trip-wire 1′,
  Step 4). Ask; do not widen the surface and report it afterwards.

Hard-protected even during this skill (never touch, regardless of what the issue asks):

- `AGENTS.md` / `CLAUDE.md`
- Generated files: `**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, and generated content
  under `docs/` (`docs/openapi/**`, `docs/coverage/**`, `docs/db-schema/**`, `docs/godoc/**`,
  `docs/portal/docs.json`, `docs/portal/guides/**`). Regenerating through a `make` target is fine;
  hand-editing is not.
- Anything under `permissions.deny` in the agent's own permission configuration
- Existing files under `database/migrations/**` (new migration files only)

## Step 0 — Confirm the four modes (two back-to-back `ask the user explicitly` interactions)

This is one stopping point made of two back-to-back calls. Ask the first call, then the second, before
anything else; do no work between them. The first call settles how the run behaves, and the second
settles what the planning phase costs. They answer different questions, and one
`ask the user explicitly` interaction caps at four questions, so combining them would leave no room
to add a fifth mode. Defaults are marked; the user's choice always wins.

### First call — run policy

Ask one `ask the user explicitly` interaction containing the following three questions. Review mode,
issue mode, and flow mode keep the semantics and defaults below.

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
| `record-on-tripwire` *(default)* | Append the call to the run record and continue; surface every recorded call in one PR comment at the end by reading that file |
| `halt-on-tripwire` | Stop at that trip-wire and ask |

**Neither mode reaches the trip-wires marked halt in Step 4.** Those are architecture, domain and
policy decisions, which `AGENTS.md` keeps behind a human gate unconditionally. The mode decides only
what happens at the remaining rows: `record-on-tripwire` appends them to the run record and continues,
`halt-on-tripwire` asks about them too — worth picking when the user is present and wants scope
growth surfaced as it happens rather than at the end.

### Second call — plan mode

Ask one `ask the user explicitly` interaction containing the plan-mode question immediately after
the first call. **State what each mode costs when asking, not only what it does, and recommend one
instead of presenting three unpriced options.** This cost is paid on every run regardless of issue
size; the user is the only one who knows before Step 3 whether the change is a one-line documentation
fix or a cross-layer feature.

**Plan mode** — how much the planning phase spends. It selects which Step 3 stages run. Every mode
satisfies Step 3's invariant that a model other than the implementer's sees the plan; the trade is
the number of passes and how many use a tier at or near the top.

| Mode | Stages | Cost |
| --- | --- | --- |
| `full` *(default)* | 3a + 3b + 3c | Three passes — the drafter's, plus two more on a tier at or near the top |
| `draft-review` | 3b + 3c | Two passes; the framing stage is skipped |
| `single` | 3b only, drafted by a model that is not the implementer's | One pass |

Recommend `full` when the issue spans layers, changes a contract, or names a design decision;
recommend `draft-review` when the shape is settled and only details need working out; recommend
`single` when the change is small enough that the plan is a formality.

## The run record

Append every call this pipeline is told to record to a file as it happens, beside the plan file under
the gitignored `tmp/`, named for the issue. Do not carry it only in the conversation. A long run
outlives its context, and anything held there can disappear without Step 9 being able to detect the
loss. The file also makes the run resumable: a later session can recover which modes were settled and
what has fired since, which inspecting the worktree alone cannot reveal.

Write one line per event, carrying both what happened and what was decided:

| Written at | Entry |
| --- | --- |
| Step 0 | The four settled modes |
| Step 3 | The plan file's path, and whether 3a / 3c ran |
| Step 4 | Every trip-wire that fired — its number, what triggered it, and the call taken |
| Step 6 | Every gate that did not run, and why |
| Step 7 | Every finding rejected, or fixed differently than proposed, with the reason |
| Step 8 | What runtime verification covered, and what it did not |

**Write each entry when the event happens, not in a batch at the end.** A batch is exactly what a
compaction eats. **Step 9 builds its comment by reading this file, never by recalling the run.**

## Seams

Two points carry almost nothing forward because everything downstream needs is already on disk:

| Seam | Everything downstream needs | Where it already lives |
| --- | --- | --- |
| After Step 5 (implementation reconciled) | The approved plan, the diff, the calls taken so far | The plan file, `git diff`, the run record |
| After the PR is opened (Step 8, before runtime verification and while CI runs) | The PR, the branch, the calls taken so far | GitHub, `git`, the run record |

At either seam, **write the run record first, then recommend compacting, then keep going.** After
Step 5, announce the recommendation without asking and continue: blocking there buys nothing the PR
seam does not buy better. After the PR opens, use Codex's `ask the user explicitly` interaction to
ask whether to compact before runtime verification. That question pauses progress until answered,
but it decides nothing about the work; continue either way once answered. Under standing full
delegation while the user is away, announce the recommendation and continue without asking at both
seams, because an unanswered question would stall the run it was told to finish autonomously. **A
seam is not a stopping point and does not belong on the list of five.**

These seams sit here because the implementation phase cannot be delegated to a sub-agent and is
therefore where an orchestrator's window actually fills. Planning stages and reviews already run in
delegated agents, whose windows never reach the orchestrator's.

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

Do this before any code is touched, so nothing lands in a shared checkout.

When resuming an existing worktree, read the run record before anything else. Inspecting the worktree
recovers where the work is, not what has been decided about it: the settled modes, fired trip-wires,
and skipped gates live in that file and nowhere else. Skipping it silently restarts with a blank
decision history and loses every one of those calls from Step 9's comment. Then inspect the worktree
without changing state: confirm its path, whether `vendor/` exists, and whether `.gobp-db-slot`
exists. A missing slot means that DB work
must run `make slot-acquire` immediately before it begins; do **not** acquire it merely to resume the
conversation. Never run slot acquisition or DB reinitialization as an unconditional resume action.

### [Codex-side difference]

Codex has no verified session-start hook contract in this repository. This resume check therefore
lives in Step 2 rather than in `.codex/hooks.json`. Keep it here when synchronizing the skill: a
Claude-side session hook may provide the same observation, but must not cause Codex to acquire a DB
slot or reinitialize a database on resume.

```bash
# 1. Resolve the active release line off origin's live state.
BASE=$(make -s base-branch)
test -n "$BASE" || { echo "ベースブランチを解決できませんでした"; exit 1; }

# 2. Branch from current origin, not a stale local ref.
git fetch origin "$BASE"
mkdir -p .codex/worktrees
git worktree add -b feature/<n>-<slug> .codex/worktrees/<n>-<slug> "origin/$BASE"

# 3. Lease a DB slot: own databases (wt<N>_local / wt<N>_test), API port 8080+N, mock-auth 2010+N.
cd .codex/worktrees/<n>-<slug> && make slot-acquire

# 4. A fresh worktree has no vendor/ and air builds with --mod=vendor, so serve would fail without this.
go mod vendor
```

`.codex/worktrees/` is a linked-worktree container inside the trusted workspace, so ordinary work
does not cross the sandbox write boundary. It is ignored because the parent checkout otherwise sees
each linked worktree as untracked. Never run `git clean -fdx` in the parent checkout: it can delete
these worktrees. The repository's local-safety rule forbids `git clean`; do not request an exception
to clean this directory.

`make base-branch` reads `origin`'s live state. Use nothing else: the local `refs/remotes/origin/HEAD`
is fixed at clone time and `git fetch` never updates it, the GitHub default branch can stay on an
earlier release line, and an agent- or environment-supplied “main branch” hint can report that stale
local symref. All three answer without warning, so a branch cut from a generation-old base can appear
valid while expected files are missing.

If `slot-acquire` reports failure, run `make slot-status` before retrying — the lease often succeeded
even when the command errored.

**Never release the slot on your own, and do not offer to during cleanup.** A slot is cheap to hold
(the lease is reclaimed automatically once stale) and expensive to lose mid-task; only the user knows
when the work is really over.

If the user's instruction named a release version **other than the resolved one**, ask before branching
— a deliberate backport target is the one case the resolver cannot know about.

## Step 3 — Plan, then wait

**The invariant: the plan is seen by a model that is not the implementer's.** The three stages below
are one default way of satisfying it, not the rule — read the rule off the session's own model rather
than off a model name written here.

No later gate re-opens the plan: `impl-review` / `test-review` / `comment-sweep` all take the finished
change as their subject, so whether the plan solves the issue at all is checked here or nowhere.
Drafting it well and drafting it unbiased are different jobs, so keep them as separate stages when
the selected plan mode includes them. **The three stages add no stopping point.** The approval at the
end is the same single wait.

**Resolve each stage's model at runtime; do not read one off this file.** Model ids are
`<family><generation>`, and the session states which model is running it, so both facts this section
needs — which family is the implementer's, and which tier is above which — are available when Step 3
executes. A name written into a skill is a snapshot of a roster that changes; these properties do not.

| Stage | Runs on | Produces | Runs in |
| --- | --- | --- | --- |
| 3a — Framing | A tier at or near the top, on a family that is not the implementer's | The questions the plan must answer — nothing else | `full` |
| 3b — Research and draft | The strongest tier available for research and drafting, as a sub-agent | The plan file | Every mode |
| 3c — Plan review | The same model as 3a; in `single`, the invariant is carried by 3b instead | Findings appended to the plan file as their own section | `full`, `draft-review` |

### 3a — Framing

In `full`, use Codex's agent delegation mechanism to give a tier at or near the top on a family that
is not the implementer's the issue body,
the Step 1 issue-vs-base discrepancies, and the paths already read. Require **open questions only**:
the questions the plan must answer. Refuse a draft plan, a recommendation, or a direction if the
stage returns one. At this point it has read almost nothing of the repository, so anything it asserts
is a generality — and a generality handed to a stronger drafter anchors rather than widens.

### 3b — Research and draft

In every mode, delegate this stage to a sub-agent on whichever model is strongest for research and drafting, so the
research happens in a window carrying none of the orchestrator's accumulated framing. Give it the
issue, the Step 1 corrections, the paths already read, and, when 3a ran, 3a's questions. Tell it to
verify your summary rather than trust it. When 3a ran, require an answer to **every** 3a question:
either how it decided or that the question does not apply here. Under `single`, 3b's model must be on
a family that is not the implementer's, because it is the only pass the plan gets.

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

Derive whether this stage is required by the invariant; never assume it from a fixed model name. If
3b's model is the implementer's, which is the ordinary case when the strongest drafting tier is also
the one running this session, no other model has seen the plan yet, so 3c is required. If 3b's model
already differs because the session runs on a family that is not the strongest drafter, the invariant
is satisfied when 3b finishes and 3c is optional as far as the invariant is concerned. Plan mode still
governs execution: run 3c whenever `full` or `draft-review` selected it, and omit it under `single`.
Run 3c on the same model as 3a. Append its findings to the plan file as their own section and present
them beside the plan, not folded into it. A reviewer that silently rewrites the plan hides the
disagreement at the moment the user is asked to approve it; the user arbitrates each finding at
approval.

If the current Codex surface cannot dispatch an agent on a different model, state explicitly which
of the selected stages could not run on a distinct model. In `full`, run 3a yourself while enforcing
its questions-only contract. Delegate 3b when possible or draft it yourself, then, in `full` or
`draft-review`, make a separate self-critique pass as 3c and append those findings as above. State
that this fallback does not satisfy the invariant. `single` is not a way around the invariant: if no
distinct family is reachable at all, say so explicitly instead of silently turning it into a
same-model draft. Do not silently drop the invariant or make the self-critique stand in for all of
Step 3.

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
`halt-on-tripwire` extends that treatment to the Record rows; `record-on-tripwire` appends them to the
run record and continues. Step 9 surfaces every recorded call in one PR comment built by reading that
file rather than by recalling the run.

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

Two traps. `merge-dml-ci` runs `go run ./cmd/`, so adding a Repository method before its query exists
deadlocks the build — stub the implementation for the duration of generation, then restore it (`cp`
the file first). The embedded-spec generator's `//go:generate` line points at a container path, so
invoke it with the real path:

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

`make go-fix`, then `make go-lint` / `make go-test`. When many worktrees are active these may be left to CI,
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
at the database layer, so a documented status code that the middleware never lets the request reach
passes all of them. One real HTTP request settles it.

When runtime verification cannot run at all, there are two honest options and no third:

1. Do not merge yet.
2. Add an integration test driving the same HTTP path, and merge on that.

Say plainly which one you took.

### Merge

Wait for CI without repeatedly foreground-polling in separate sleep calls. Run this as one
long-running command and use Codex's background/yield-and-notify mechanism when the current surface
provides it, so one completion notification arrives. If that mechanism is unavailable, retain the
single loop below and state that it blocks; do not replace it with a foreground sleep-poll sequence:

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

Finally, read the run record and work down it to put every entry into one PR comment, together with
any further call not already visible in a commit message or the PR description. Every trip-wire
recorded rather than halted on lands here, and the file is the only place they all survive.

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
- ✅ Verify the issue's claims against the actual base, and put the discrepancies in the kickoff comment.
- ✅ Get the plan approved before implementing, and keep it as a file so Step 5 can diff against it.
- ✅ Put the plan through a model that is not the implementer's, and present that review's findings beside the plan rather than folded into it.
- ✅ Treat the five trip-wires as mechanical triggers, not as things to notice.
- ✅ Stop only at the five listed places — where stopping means ending the turn, not only asking —
  and append every other call to the run record as it happens;
  at a seam, write the record and recommend compacting: announce at the Step 5 seam, ask through
  Codex's explicit user-input interaction at the PR seam, and announce at both under standing full
  delegation while the user is away.
- ✅ Pass every sub-skill its settled answers, apply mode included.
- ✅ Say explicitly which gates ran and which did not.
- ✅ Read the traces, not just the status code.
- ✅ Verify a finding at runtime before filing an issue for it.
- ❌ Merge a change to implementation code that has never been exercised over HTTP.
- ❌ Present green CI as runtime verification.
- ❌ Auto-apply a fix that changes the design, in any mode.
- ❌ Ask for approval at a phase boundary, or treat a delegated agent's completion as one.
- ❌ End a turn on a progress report; write the report and continue in the same turn.
- ❌ Carry the run's decisions in context alone, or build Step 9's comment by recalling them.
- ❌ Treat a seam as a stopping point, or wait at one.
- ❌ Pick which review skills run, or run one on the assumption another chains it.
- ❌ Let the framing stage return a plan, a recommendation, or a direction instead of questions.
- ❌ File an issue without checking for an existing one, unless issue mode says to.
- ❌ Release the DB slot, or ask about releasing it, unprompted.
- ❌ Poll CI in a foreground sleep loop.

## Checklist

- [ ] Four modes confirmed across Step 0's two back-to-back `ask the user explicitly` interactions, with plan-mode cost stated when asked.
- [ ] Kickoff comment posted, including issue-vs-base discrepancies.
- [ ] Worktree created from a freshly fetched base; DB slot leased only when DB work begins; `go mod vendor` run when needed.
- [ ] Plan built through the stages plan mode selected, every 3a question answered when 3a ran, all four sections present, seen by a model that is not the implementer's, and approved before implementation.
- [ ] Trip-wires handled per their row's default and the flow mode; nothing silently absorbed, and
      every call appended to the run record when it happened.
- [ ] No stop outside the five listed places, and no turn ended at a phase boundary or on a progress
      report.
- [ ] Both seams taken: run record written and compaction recommended; the Step 5 seam announced and
      continued, and the PR seam asked through Codex's explicit user-input interaction unless the run
      was under standing full delegation while the user was away, in which case it announced and
      continued.
- [ ] Plan reconciled against the actual diff.
- [ ] Local gates run, or their delegation to CI stated in the PR.
- [ ] The three review skills each estimated and put to the user; the approved ones run with their
      answers passed through. Auto-application confined to machine-checkable fixes.
- [ ] Decision points presented together.
- [ ] PR opened, then runtime verification (curl + traces) completed — or its absence stated together
      with which of the two options was taken — before merging.
- [ ] Issue closed manually with a handover comment; unrelated findings routed per issue mode, each
      verified before filing; remaining judgment calls recorded in a PR comment.
