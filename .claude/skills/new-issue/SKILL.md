---
name: new-issue
description: >-
  Turn a question about a possible change into a GitHub issue whose premises have been verified against the actual implementation — or into the finding that it should not be an issue at all. Use whenever the user wonders aloud whether something is feasible, reports behavior they think is wrong, proposes a refactor, or asks 「これ issue にしといて」「これって直せる？」「こういう機能入れられる？」. The value is not drafting prose: it is tracing the request path end to end before asserting anything, refusing to guess when information is missing, and writing each factual claim so it is individually falsifiable later. Filing is gated behind "should this be an issue at all" — an existing issue that only needs a comment, or a fix small enough to just make, is not a new issue. Do NOT use it to work an issue that already exists (`impl-issue`), to review a diff (`impl-review`), or to write specs or ADRs (`new-spec`, `docs/adr/`).
argument-hint: '[question or description] [--verify=runtime|static] [--output=file|draft]'
---

# New Issue

Convert a question about a possible change into an issue that can be trusted later — or into the
conclusion that no issue is warranted.

The drafting is the easy part. What this skill exists for is the step before it: **checking that what
you are about to assert is true of the code as it stands right now**, and refusing to fill gaps with
plausible guesses. An issue is read months later by someone who will act on it; a confident sentence
that was never verified costs more than no issue at all.

A Japanese reference translation lives at `SKILL.ja.md` in this directory (for human reference only;
not loaded as a skill).

## When to Use

- The user wonders whether something is feasible, or proposes a change in passing.
- The user reports behavior they believe is wrong.
- The user asks for an issue outright.

Do NOT use it to work an issue that already exists (`impl-issue`), to review a diff (`impl-review` /
`test-review`), or to write specs / ADRs (`new-spec`, `docs/adr/`).

## Why this exists

An issue's factual claims are usually wrong for one reason: nobody looked outside the layer they were
thinking about. A status code, an event's consumer, the cost of an option on the hot path — each is
statically checkable, and each is decided somewhere the author never opened. That is the failure this
skill is shaped around. Reading "the relevant layer" is not the same as tracing the path.

## Step 0 — Confirm two things (one `AskUserQuestion`)

**Verification depth** — how far a behavioral claim must be proven before filing.

| Mode | Behavior |
| --- | --- |
| `runtime` *(default when the draft asserts runtime behavior)* | Confirm the behavior against the running system before filing |
| `static` | Code reading only; every behavioral claim is then marked as unverified in the body |

**Output** — whether this run ends in a filed issue or a handed-over draft.

| Mode | Behavior |
| --- | --- |
| `file` *(default)* | Present the body, then file after approval |
| `draft` | Produce the body and the analysis; file nothing |

Searching existing issues is not a mode. It always happens — it is cheap, and skipping it is how a
duplicate gets filed.

## Step 1 — Capture the question

Restate what the user is actually asking, in one or two sentences, and confirm it. A question asked in
passing ("これ直せる？") usually carries an unstated assumption about *where* the problem is; naming
that assumption early is what lets Step 2 disprove it.

Record what triggered this: a symptom the user hit, a review finding, a code reading. The origin
determines how much of it is already evidence and how much is conjecture.

## Step 2 — Trace the path end to end

This is the step the skill exists for. Do not read only the layer the question points at.

For a behavioral question, follow the request from entry to storage and back:

```txt
OpenAPI → middleware (auth / identity resolution / validation) → handler → usecase → domain → infrastructure → SQL
```

Middleware is the most commonly skipped segment and the most commonly decisive one, because it can
reject or transform a request before the layer under discussion ever runs. For a structural question,
trace the dependency direction instead, and check the layer rules in `docs/rules.md` and the relevant
`README.md` at runtime rather than from memory.

Then establish, for each thing you intend to assert:

- **Is it current?** The file may have changed in a branch that merged this week. Check recent history
  for the paths involved (`git log --oneline -15 -- <paths>`) and recently merged PRs touching them.
- **Is the radius complete?** If the claim is "only X does this", reverse-traverse the graph
  (`node .claude/scripts/graph-affected.ts <symbol> --depth 2`) rather than guessing at call sites —
  it returns each one with a relation label and `file:line`, which is the evidence a claim of absence
  needs. Check the graph's freshness first (`AGENTS.md`), and fall back to an exhaustive search of
  callers and siblings when it is behind or the symbol is new. A claim of scope is a claim about
  absence, and absence is what searches are for.
- **Does a cost comparison rest on anything?** "Adds a read on the hot path" is checkable — count the
  queries in both designs. An unmeasured cost is not a trade-off, it is a guess.

Read `docs/architecture.md` and `docs/development-flow.md` to locate the relevant code rather than
guessing at paths.

## Step 3 — Five blockers

A draft does not proceed to Step 4 while any of these is true. They are deliberately mechanical: an
author who is asked to *notice* that they are unsure will usually not notice.

| # | Blocker | Resolution |
| --- | --- | --- |
| 1 | The draft asserts runtime behavior that was never executed | Run it (Step 5), or mark the claim unverified and say so in the body |
| 2 | Cited implementation was not checked for currency | Check history for those paths |
| 3 | An option comparison has no measured basis | Measure it, or drop the comparison and present the options without a cost claim |
| 4 | An impact-radius claim rests on a partial search | Reverse-traverse the graph, search exhaustively, or narrow the claim to what was covered |
| 5 | Existing issues were not searched | Search |

**When information is missing, ask — do not estimate.** This is the instruction the user gave when
asking for this skill, and it is worth stating plainly: a plausible guess written in an issue's
confident register becomes fact for everyone who reads it afterward. Missing information includes
which behavior is actually desired, which of several possible causes the user has in mind, and
whether a constraint the code implies is intentional. Ask about those; do not resolve them by
inference.

## Step 4 — Draft the body

Use the shape this repository's issues already use, plus a premises section:

```markdown
## 概要
## 前提            ← each factual claim, and where it was verified
## 背景
## やることリスト
## 論点            ← options A / B / C, each with cost and consequence, plus a recommendation and its basis
## やらないことリスト
## 完成の定義
## 関連
```

**The 前提 section is the part that is new, and it is the point.** Write each premise as a separate,
individually falsifiable statement with the evidence behind it:

```markdown
## 前提

- `POST /v1/<resource>` reaches the usecase for a deactivated identity — **verified**: the resolver
  only rejects on `deleted_at`, and the identity is resolved before the handler runs
  (`internal/infrastructure/auth/<impl>/resolver.go`, read at `abc1234`)
- `<aggregate>.<event>.v1` has no consumer — **verified**: no handler matches the event type
  (`rg 'Type<Event>' internal/controller/worker/`, at `abc1234`)
```

`impl-issue` reads this section when the issue is picked up and reports every premise that no longer
holds. Prose that buries its assumptions cannot be checked that way, which is exactly how the three
wrong claims above survived to implementation.

Two writing rules that keep an issue from rotting:

- **Cite symbols and paths, never line numbers.** Line numbers are stale by the next refactor, and a
  reader who follows one to the wrong place trusts what they find there.
- **Record the recommendation together with its basis.** A recommendation that turns out to be wrong
  is fine and normal; one whose reasoning is invisible cannot be overturned by evidence.

For a link to another repository's issue or PR, use `redirect.github.com` — a plain `github.com` link
posts a public cross-reference on that thread. Whether to send that signal deliberately is a human
decision, without exception: ask every time, and never make the call yourself. See `CLAUDE.md`.

Write the body in Japanese. Present it and wait for approval.

## Step 5 — Runtime confirmation (when the draft asserts behavior)

Under `--verify=runtime`, confirm the behavioral claims against the running system before filing.
This is the same stage `impl-issue` runs before merging, for the same reason: a status code produced
by an unexpected path looks identical to the right one.

```bash
make slot-acquire && make serve            # API on 8080+N, mock-auth on 2010+N

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

The token subject must be the identity `subject` string the seed registered, not an internal
UUID — the seeded UUID rows belong to a different issuer than the slot's port produces, so a UUID
yields a confusing 401.

Read the traces too, not just the status: they show which layers actually ran, which is precisely
what the wrong claims above got wrong.

Do not release the DB slot afterward unless the user asks.

Under `--verify=static`, skip this — and mark every behavioral claim in 前提 as unverified. An
unverified claim presented in the same register as a verified one is worse than an admitted gap.

## Step 6 — Decide whether this should be an issue at all

Run this gate before filing. An AI that can write issues quickly will produce more of them than a
human would, and issue count is itself a cost — a duplicate buries the original, and a backlog nobody
can read is a backlog nobody uses.

| Situation | Action instead of filing |
| --- | --- |
| An existing issue covers it | Comment there with the new finding |
| The fix is small enough to just make | Offer to make it now |
| Step 2 disproved the premise | Report that; file nothing |
| It is a decision, not a task | Propose an ADR (`docs/adr/`) |
| It is behavior of a feature, not a defect or change | Propose a spec update (`docs/spec/**`) |

Search before concluding it is new:

```bash
gh issue list --state open --limit 100 --search "<keywords> in:title"
gh issue list --state all --limit 50 --search "<keywords>"
```

Include closed issues. A previously rejected proposal is important context, and re-filing it without
acknowledging the rejection wastes the reader's time.

State which branch of the table applied. Silence reads as "it was obviously an issue".

## Step 7 — File

```bash
gh issue create --title "<title>" --label <label> --body-file <file>
```

Report the URL, and say which premises were verified at runtime and which only statically.

If the user selected `--output=draft`, stop before this and hand over the body.

## Handoff to `impl-issue`

Upstream of this skill sits `research`, which compares undecided options and stops at
`決めるべきこと`. Its recommendation is not a decision — a human's approval is what turns it into one,
and this skill is where that approved outcome becomes trackable. When an issue arrives carrying a
recommendation nobody has approved yet, that is the gap to name, not to close: file the decision as
the 論点, not as a settled plan.

An issue produced here is meant to be picked up by `impl-issue`, whose first step compares the issue
against the base and reports every discrepancy in the kickoff comment. The 前提 section is what makes
that comparison possible. When drafting, write for that reader: state assumptions where they can be
checked, not where they read most smoothly.

## Do / Do NOT

- ✅ Trace the whole request path, including middleware, before asserting anything.
- ✅ Ask when information is missing; never fill the gap by inference.
- ✅ Write each premise as a separate falsifiable claim with its evidence.
- ✅ Search existing issues, closed ones included, before concluding it is new.
- ✅ Say explicitly which claims are unverified.
- ✅ Cite symbols and paths; record the recommendation's basis alongside it.
- ❌ Assert runtime behavior that was never executed, without labelling it as unverified.
- ❌ Present an unmeasured cost as a trade-off.
- ❌ Claim an impact radius from a partial search.
- ❌ File when an existing issue only needs a comment, or when the fix is smaller than the issue.
- ❌ Link another repository's issue with a plain `github.com` URL, or decide on your own that a
  cross-reference is warranted.
- ❌ Write line numbers into the body.
- ❌ Release the DB slot unprompted.

## Checklist

- [ ] Verification depth and output mode confirmed in one `AskUserQuestion`.
- [ ] The question restated and confirmed; its origin recorded.
- [ ] Request path traced end to end, middleware included; cited code checked for currency; impact
      radius searched exhaustively.
- [ ] All five blockers cleared, or the corresponding claim narrowed / marked unverified.
- [ ] Missing information asked about rather than estimated.
- [ ] Body drafted in Japanese in the repo's shape, with a 前提 section carrying evidence per claim,
      symbols not line numbers, and the recommendation's basis recorded.
- [ ] Runtime confirmation run, or every behavioral claim marked unverified.
- [ ] Existing issues searched including closed; the should-this-be-an-issue gate applied and its
      outcome stated.
- [ ] Filed and URL reported, or handed over as a draft.
