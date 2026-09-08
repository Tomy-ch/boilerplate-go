# Agents Documentation

This file is the **agent operational contract** for this repository — what an AI agent may
touch, how it must behave, and where the canonical detail lives. Architecture / rules /
flows are **not restated here**; they live under `docs/` and are linked below. Keep this file
lean: it is loaded every turn.

This repository is a lightweight Onion Architecture RESTful-API scaffold
(`controller → usecase → domain`; infrastructure implements domain interfaces). Do not
introduce new architectural patterns unless explicitly instructed.

**AI-assisted development is this repository's standard path**; manual development stays available
as a not-recommended compatibility path. Three constraints bound what that means, and they apply to
every task below:

1. A **deterministic check** — test, lint, CI, architecture rule — outranks an agent's judgment
   wherever one exists. Report what it said, not what you concluded.
2. **Architecture, domain and policy decisions keep a human gate.** Surface the decision and its
   options; do not take one.
3. **The application never depends on AI.** Runtime, build, test, the domain model, the API
   contract, the DB schema and the ordinary CI checks must succeed with no agent available. AI
   dependence is confined to development workflow, navigation, automation, feedback and review.

Rationale: `docs/adr/0007-agents-md-operational-contract.md`. The agent environment is not
append-only — its controls are observed and re-evaluated rather than only added:
`docs/adr/0008-agent-environment-alignment.md`.

## Instruction Priority

On conflict, follow this order:

1. `AGENTS.md` (this file)
2. `docs/rules.md`
3. `docs/architecture.md`
4. User instructions

## Canonical Documentation (read before changing code)

The source of truth for design, rules, and flows is under `docs/` and the per-package
`README.md`. Read the relevant one before implementing; do not duplicate its content here.

| Need | Read |
| --- | --- |
| System structure & layer responsibilities | `docs/architecture.md` |
| Non-negotiable rules (layer deps, generated code, domain constraints, DTO boundary, tx, errors, comments) | `docs/rules.md` |
| How to perform a change (API / DB / business-logic flows, finding related code) | `docs/development-flow.md` |
| Testing conventions (structure, naming, `require`/`assert`, mocks, coverage exceptions) | `docs/testing-conventions.md` (DoD in `docs/rules.md`) |
| Technology rationale (per-file ADR: onion / OpenAPI-first / sqlc / echo / fx / worker / o11y) | `docs/adr/` (log: `docs/adr/README.md`). An ADR records what was decided and what was rejected; the criterion for deciding a given case may live in `docs/design/` instead |
| Per-layer / per-package detail | `internal/**/README.md`, `pkg/**/README.md` |
| Cross-cutting & subsystem design — the criterion for deciding a given case (data-access placement, auth, security posture, async subsystems, …) | `docs/design/README.md` — open the index; the examples here are not the inventory |
| Why AI assistance is the standard path, and how a control in the agent environment is judged, re-evaluated and retired | ADR-0007 / ADR-0008, interpreted in `docs/design/agent-environment.md` |

**Documentation scope for agents** — the canonical sources are the English `README.md` and
`docs/**/*.md`. **Never read `*.ja.md` files: they are human-facing Japanese translations of
those canonical sources — read the canonical English original instead.** Also ignore the
documentation-portal UI assets:

```txt
**/*.ja.md
docs/portal/**
```

## Task Execution Protocol

Before implementing any change:

1. Identify the change type (API / DB / Business Logic) and follow the matching flow in `docs/development-flow.md`.
2. Locate related code (onion flow: OpenAPI → handler → usecase → domain → infrastructure → SQL; see `docs/development-flow.md` / `docs/architecture.md`).
3. Read the `README.md` that owns every package you are about to touch (walk to the nearest ancestor when the package has none) — its declared responsibilities and prohibitions bound the change.
4. Open the `docs/design/README.md` and `docs/adr/README.md` indexes and read the entries that own the decisions your change touches. Searching either index for your feature's words is not enough — a document is named for the concern it owns.
5. Verify no existing implementation already covers it — search the same layer first; prefer editing an existing file over creating a new one.

For API changes: OpenAPI is defined first. For DB changes: the migration + SQL exist first.

<!-- boilerplate-only:begin -->
## Review Phase Protocol

A request to review work that has already been implemented — 「レビューして」 or any equivalent —
names **three** subjects this repository ships a skill for, not one:

| Skill | Subject |
| --- | --- |
| `/impl-review` | the change itself — architecture / DDD modeling / security / correctness / runtime gap |
| `/test-review` | the tests that pin the change down |
| `/comment-sweep` | the comment stock carried by the files the change touched |

Do not silently pick one. **Estimate each skill's return from the context you already hold** — which
layers the change touched, whether tests or comments moved at all, what an earlier skill in this
session already covered — then **ask the user per skill whether to run it, stating that estimate and
its reason**, and run what they approve.

The estimate is the work. 「三つとも回しますか」 is not a question; it hands the cost back to the user
unpriced. Say which pass you expect to pay off, which you expect to return nothing, and why.

The three are **peers, and none of them invokes another.** Each is asked for, decided, and run beside
the others — one subject to one skill, and that skill is the only place its subject is audited. So
`/impl-review` audits the change and nothing else, owning no test lens and no comment lens; the other
two are invoked in their own right, whether or not it runs.

Chaining them would look like convenience and cost more than it saves: the subjects stop being
independently answerable, and a drift in the entry skill's question silently drops the other two from
every flow that went through it. The coupling belongs in the *asking*, not in the skills.

**This holds inside a pipeline too.** A skill that drives an issue to a merged PR reaches a review
phase like any other flow, so it asks these three questions there rather than choosing for the user —
and never on the assumption that running one of them brings the others along.
<!-- boilerplate-only:end -->

## Layer Rules (hard constraints — enforced by `golangci-lint` depguard)

Boundaries are enforced in CI, not just documented. Full table + rationale: `docs/rules.md`.

- **Dependencies point inward.** `controller → usecase → domain`; `infrastructure` implements domain interfaces; never bypass a layer.
- **Domain** is pure: no framework / infrastructure / logging / DI; no env / I/O / DB clients; no dependence on time, randomness, or system state directly (abstract via domain interfaces implemented in outer layers). **No `context.Context` in domain logic** (Repository interface signatures may declare it for propagation only). The only permitted `internal/` dependencies are `internal/apperror` and the domain lexicon `internal/domain/lexicon` (cross-aggregate business-semantic value objects that cannot live in `pkg/`; admission is narrow, and failing `pkg/`'s entry bar is not an argument for admission here — see `docs/rules.md` / ADR-0039 (domain-lexicon)); a domain package must not import another aggregate. The one exception is `internal/domain/service/<name>/`, where a rule that is the natural responsibility of no entity and no value object lives — whether it spans aggregates or stays inside one: that path has its own depguard rule permitting aggregate imports while repeating every other domain deny, and admission is narrow (see `internal/domain/README.md`). Otherwise use `pkg/`.
- **`pkg/`** must not depend on infrastructure or framework-specific packages, must stay framework-agnostic, must not import `internal/**`, and holds no feature-specific business logic.
- **Usecase** depends only on domain interfaces (never infrastructure), owns transaction boundaries, and maps domain models to DTOs — never exposes domain entities to outer layers.
- **Controller** handlers stay lightweight: request/response only, no business logic, no infrastructure imports.

## Forbidden Shortcuts

AI agents MUST NOT:

- Call infrastructure directly from a handler
- Put business logic in a handler
- Skip the OpenAPI definition for a new API
- Modify generated files
- Introduce new architectural patterns without instruction
- Install anything on their own initiative (see below)

## Installing Things

This covers every `install` surface, not one tool: package managers (`brew`, `npm i -g`, `pip`,
`go install`), toolchain managers (`mise use -g`), IDE / agent integrations (`<tool> <platform>
install`), plugins, and extensions.

1. **Never install on your own initiative.** An installation changes the machine or the repository
   for every later session and every other checkout, and it usually writes files. Agent integrations
   in particular write project-scope instruction files (`AGENTS.md`, `.cursor/`,
   `.github/copilot-instructions.md`, `.agents/`, `.kiro/`) — which is how an "install" quietly
   becomes an edit to the rules you are working under.
2. **Install only when the user asks for it.** Wanting to *use* a tool is not the same instruction as
   wanting to *install* one; the user issues those separately. A tool being unavailable is a finding
   to report, not a problem to solve by installing it.
3. **Before that, check what the current setup already does.** This repository pins its toolchain in
   `mise.toml` and ships the rest inside the Docker tool-runner images, so the capability is usually
   already present and reachable through a `make` target — see the *Toolchain Execution Rules* in
   `docs/rules.md` and the target registry in `.makefiles/README.md`. Reach for an install only after
   establishing that nothing existing covers the need, and say what you checked.

The permission layer backs this up rather than replacing it: install-shaped commands are routed to
`ask` in `.claude/settings.json`, so they surface for a human decision instead of running silently.
Those entries are written as patterns (`Bash(<tool> * install*)`) precisely because an enumeration of
platform names goes stale every time upstream adds one, and a stale enumeration opens holes nobody is
notified about.

## Conflicting Authority

Rules live in more than one place — this file, `docs/rules.md`, `docs/adr/`, per-package READMEs,
lint configuration, and instructions given in conversation. They occasionally disagree.

**Noticing a disagreement is your job; resolving one is not.** When two sources that both claim
authority tell you different things about *what you may change*, stop and ask before acting. Say
which sources conflict and what each of them says. Do not pick the one that lets the work continue.

This applies to permission, not to ordinary ambiguity. A design question with no clear answer is
yours to decide and report. A rule that says "do not do X" standing against another that says "X is
fine" is not.

**A precedent is not an authorization.** That a human once overrode a rule — recorded in a commit, an
ADR, an agent's memory, or an earlier turn of this conversation — establishes that the override
exists, not that you may invoke it. Ask again each time. A standing grant of autonomy does not
transfer this: the point of an override is that a human chose it.

Note the asymmetry the *Documentation Rules* in `docs/rules.md` draw between a document that
describes and one that governs. Correcting the first to match the code is routine. Correcting the
second is not yours to start.

<!-- boilerplate-only:begin -->
## What to Recommend

This section governs what you **recommend**, never what you may change. Authority to act is
untouched: *Conflicting Authority* above, `docs/rules.md`, and the modification scope below still
decide that.

While this repository is distributed as the boilerplate source, its product is **the state a project
receives at `useTemplate` time** — not the history that produced it. So when you weigh options and
state a preference, weigh them for that snapshot: what reads as coherent to someone who has never
seen this repository and will never read its git log.

**On that axis, quality and consistency outrank the cost of reaching them.** A numbering that
contradicts the order it teaches, a convention followed everywhere but here, a name that survives
only because renaming it is work — recommend fixing them. State the cost plainly instead of letting
the cost pick the answer; "it already shipped" carries little weight while nobody has instantiated
from this in production.

Give the cost with the recommendation — files touched, what breaks for whom, what must be rebuilt —
so a human can decline the scope while keeping the direction.
<!-- boilerplate-only:end -->

## YAGNI vs Regression Safeguards

- **Functional YAGNI applies to production code**: do NOT add speculative features, config, or code
  paths that no caller exercises. Unreached "might-need-it-later" code is dead weight — untested,
  coverage-dragging, and prone to rot.
- **Deliberate regression safeguards are the encouraged exception**: a defensive branch/guard whose
  purpose is to catch a future mistake (e.g. mapping the right value per environment, refusing a
  dangerous operation) SHOULD be kept and **actively locked down with a test**. If it is unreachable
  through its current caller, extract the logic into a testable unit and cover every branch rather
  than deleting it. Never drop a meaningful safeguard just because it is currently unreached — make
  it testable and add the regression test.
- **Coverage % is a proxy, not the goal**: a test's worth is the contract it locks against
  regression, not the number it moves. A meaningful test can add 0 % (e.g. exercising an empty
  no-op default — an `{}` body has zero coverable statements, so its `0.0%` is a Go display artifact,
  NOT a gap). Do NOT chase whole-function `0.0%` that are empty/no-statement bodies, and do NOT
  delete a test that verifies a real contract just because it doesn't raise coverage. Conversely,
  code run only to hit lines with no meaningful assertion is coverage theater — that IS meaningless.
- **A test is meaningful only if** the contract it protects (correctness / invariant / boundary /
  safety) (1) can actually regress, (2) is not already locked elsewhere, and (3) is owned by that
  layer. The meaningless-test forms are the inverses: **wrong semantics** (tautology, asserting an
  incidental implementation detail, or coverage-only), **redundant duplication** (same path verified
  2×/3× across layers with no new viewpoint; re-testing a dependency / generated code), or **wrong
  layer** (verifying a concern the layer does not own).

## AI Modification Scope

AI agents may modify code only in these directories unless explicitly instructed otherwise:

- `internal/`
- `pkg/`
- `database/` (`database/dml/**`; `database/migrations/**` — new files only, never edit existing migrations)
- `openapi/`

Do NOT modify other top-level directories (e.g. `cmd/`, `docker/`, `scripts/`, `docs/`,
`vendor/`, `makefile`) unless the user explicitly requests it.

- **CLI command exception:** each CLI subcommand is a thin `cmd/<command>.go` shell (Cobra + real-dependency wiring) paired with its testable core under `internal/cli/<command>/`. Adding / modifying a command necessarily edits the matching `cmd/<command>.go` and that is in-scope; the restriction is about not arbitrarily restructuring `cmd/` entrypoint/build wiring.

**AI-tool configurations are out of scope** — do NOT create/modify/delete these unless the user explicitly requests it. Note that maintaining them is an ordinary, recurring part of the standard path rather than an exception: it is reached through the skill that owns it (`manage-skill` for a skill, `sync-ai` across environments), which is what supplies the explicit instruction. What is unchanged is the requirement — a direct user request or a running skill, never an agent's own initiative:

- Claude Code: `.claude/` (skills, `settings.json`, `settings.local.json`, …)
- OpenAI Codex CLI: `.codex/skills/`
- Cursor: `.cursor/` (incl. `.cursor/rules/*.mdc`), `.cursorrules`
- GitHub Copilot: `.github/copilot-instructions.md`, `.github/instructions/`, `.github/prompts/`
- Gemini CLI / Code Assist: `.gemini/`, `GEMINI.md`

### Exception: Skill Execution

Invoking a skill (Claude Code `/<skill-name>`, or an equivalent mechanism) counts as an
**explicit user instruction**. While the skill runs, the scope restrictions above are relaxed
for the paths the skill's defined procedure needs. Conditions:

- Relaxed only for the skill's duration and only to the scope the skill defines; the skill's `SKILL.md` still governs (honor any "confirm before touching X" step).
- **Hard-protected even during skill execution:**
  - `AGENTS.md`
  - Generated files: `**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`
  - Generated content under `docs/`: `docs/openapi/**`, `docs/coverage/**`, `docs/db-schema/**`, `docs/godoc/**`, `docs/portal/docs.json`, `docs/portal/guides/**`
  - Anything under `permissions.deny` in `.claude/settings.json`
- Canonical Markdown under `docs/` (`architecture.md`, `rules.md`, `decisions.md`, `development-flow.md`, `testing-conventions.md`, `maintenance/**`, `ja/**`, `portal/manifest.yaml`, …) is NOT generated and remains editable per the skill's scope.
- Skills must not be a loophole. If a procedure touches a sensitive area (`docker/`, `.github/workflows/`), the skill must document it so the user is aware.

## Do Not Edit Generated Files

- `**/*.gen.go`
- `**/*.sql.go`
- `*_mock.go`
- `**/openapi.gen.yaml`
- Generated content under `docs/`: `docs/openapi/**`, `docs/coverage/**`, `docs/db-schema/**`, `docs/godoc/**`, `docs/portal/docs.json`, `docs/portal/guides/**`

## Git Rules for AI Agents

1. **NEVER commit directly** to `production`, `develop`, `staging`, or any `release/*` branch. Always cut a feature branch from the branch `make base-branch` resolves: the latest `release/*` line, where **"latest" is the numeric comparison of `major` / `minor` / `patch`**, read from `origin`'s live state.
   - **Take the base from nowhere else.** The local `refs/remotes/origin/HEAD` is fixed at clone time and `git fetch` never updates it (only `git remote set-head` does, and that only copies the GitHub default); the GitHub default branch (`gh repo view --json defaultBranchRef`) lags behind the active release line; a harness-supplied "Main branch" value reads that same stale local symref. All three answer without warning, and a feature branch cut from a generation-old base stays invisible until the files everyone expects turn out to be missing.
   - **Where a pull request already exists, its `baseRefName` is the authority** and the resolver is the fallback. A PR's base is what it is already merging into; nothing may re-resolve it.
   - **During a hotfix, resolve nothing — ask.** `make hotfix-patch` cuts a `hotfix/vX.Y.Z` and makes it the GitHub default, so the branch under active development is then not the latest `release/*`, and `make base-branch` — which considers `release/*` only — will not name it. That scope is deliberate: a hotfix is an emergency, its base is a human decision taken on the spot, and an agent inferring one from branch names would be guessing at the moment guessing is most expensive.
2. Do NOT rebase, squash, or force-push unless the user explicitly requests it.
3. After amending an existing PR branch, do NOT auto-push — ask first: 「変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？」
4. **Syncing a feature branch with an advanced base — merge, never rebase (see rule 2).** When the base `release/*` has moved ahead and the feature branch must catch up, fast-forward the local base to its remote and **merge** it into the feature branch (`git merge origin/release/vX.Y.0`), rather than rebasing / force-pushing. The branch to merge is the one this feature branch was cut from — the pull request's base — **not** whatever `make base-branch` resolves today: if a newer release line has opened since, merging that would retarget the branch instead of catching it up. Resolve conflicts in generated artifacts (`**/*.gen.*`, `docs/openapi/**`, `openapi/openapi.gen.yaml`, …) by **regenerating from the source of truth** (`make gen-api` / `make gen-query`), not by hand-editing the generated output. Rebase only when the user explicitly requests it.

**Commit / PR execution:** split into scoped commits using the prefix convention
(Feat / Fix / Refactor / Perf / Docs / Test / Build / CI / Chore / Style / Revert), bypass
per-commit hooks during the split then run verification (lint / test / sql-lint / migration
checks) once at the end, add the `Co-Authored-By` footer, and never commit to protected
branches. If your agent provides a dedicated command/skill for this workflow, prefer it over
manual steps (keep the concrete names in your own agent config, not here).

**Branch naming:** include the issue number when provided (`feature/1234-description`);
otherwise a descriptive hyphenated name (`feature/add-authentication-check`).

**Linking to another repository's issue / PR — always go through `redirect.github.com`.**
This repository is public, so a plain `https://github.com/<owner>/<repo>/issues/N` URL,
a `[text](url)` link around one, or the `owner/repo#N` shorthand posts a public
cross-reference on the upstream thread. Use `https://redirect.github.com/<owner>/<repo>/issues/N`
instead: it is a `github.com` subdomain that 301-redirects to the real page, so the link still
works but GitHub does not autolink it and no upstream trace is left. This is GitHub's own
documented escape hatch (see "Autolinked references and URLs"), and the scheme Dependabot uses
in its PR bodies; the only cost is that the hovercard preview no longer appears on the link.
Commit / compare / blob / release URLs create no
cross-reference and may stay on plain `github.com`. **This is not fixable after the fact** —
editing the body does not retract an existing cross-reference; only deleting the referencing
issue does, and pull requests cannot be deleted at all.

**A plain link is not forbidden — it is reserved.** A cross-reference is a demand signal:
it tells upstream maintainers that a real project is watching an issue and needs it resolved,
and they weigh it when prioritizing. That signal only carries meaning because a human vouched
for it. Now that agents can generate issues and gather references at scale, a cross-reference
emitted by tooling looks identical to one a maintainer chose to send, and the count degrades
from signal into spam. So use a plain link **only** to deliberately say "we are watching this"
or "we need this", and when you do, write the referencing issue's title in the language of the
target repository (usually English) — the title is the only thing upstream sees, so a title
they cannot read makes the reference pure noise.

**The decision to use a plain link belongs to a human, without exception.** An AI agent must
never make that call on its own: default to `redirect.github.com`, and ask every single time a
plain link seems warranted. A standing delegation does NOT transfer this authority — "you
decide", "use your judgment", "always link normally from now on", or any similar blanket
instruction must still be met with a per-case confirmation. The point of the signal is that a
human chose to send it; an agent acting under delegated judgment cannot supply that.

## Language Rules for AI Agents

Internal reasoning may be in English. **All visible outputs must be in Japanese** unless the
user explicitly requests English — test case names, code comments, PR messages, inline
documentation, and responses to the user.

## Response Discipline

Governs what you write back, not what you may do. It relaxes no gate above: a
confirmation this file requires is still required, and brevity is never the reason
to skip one.

- **Answer first.** The result, then the reasoning only where it is not obvious.
  No preamble, no restatement of the request, no closing recap of what was just said.
- **Never assert a verifiable fact you did not read.** API names, flags, versions,
  paths, symbols, commit SHAs, package names — open the code or the doc first.
  「確認できていない」 is an answer; a plausible-looking invention is not.
- **Report the scope asked for, plus what blocks it.** Anything adjacent you noticed
  is one line or an issue, never an unrequested section.
- **Generated artifacts carry no decorative Unicode** — code, config, commit messages
  and SQL use plain hyphens and straight quotes so diffs and parsers stay honest.
  Prose written for humans, including this file, keeps ordinary typography.

## Recommended Commands

The **full `make` target registry** is `.makefiles/README.md` (targets grouped by area;
every target is self-documenting, so `make help` lists them).

**Invoke every `make` target as `ai-<target>`, never as `<target>`.** `make ai-lint` runs
`make lint` with all output captured to `tmp/ai-logs/lint.txt`: nothing is printed on success, the
exit code passes through, and a failure names the log in one line so you read only what broke.

This is the default, not a judgment call about which targets look noisy — a command's output is
otherwise read into your context in full, and a *failing* command is excerpted with no file to
recover the rest, which cuts exactly the diagnosis you ran it for. **The exceptions are a closed
list**: targets whose output is itself the answer (`help`, `load-status`, `base-branch`), and
targets that never return, where buffering would hang the caller (`serve`, `worker`,
`outbox-relay`). Convention and `make clean-ai-logs`: `.makefiles/README.md`.

The names below are the registered target names, so reach each one as `ai-<name>`.
Common ones:

Code generation:

- `make gen-api` — generate API code from the OpenAPI spec (oapi-codegen + mock)
- `make gen-query` — generate SQL query code from SQL files (sqlc)

Format / lint / test:

- `make fix` — auto-format + auto-fix lint (run before committing; then fix what remains)
- `make lint` — Go static analysis (golangci-lint)
- `make test` — run all tests with coverage
- `make md-lint` / `make md-fix` — Markdown lint / auto-fix
- `make sql-lint` / `make sql-fix` — SQL lint / auto-fix

Run / DB:

- `make serve` — start the local dev environment (for runtime / `curl` verification)
- `make db-init` — migrate + seed both the local and test DBs (prerequisite for DB-backed tests)
- `make new-migrate-<name>` — scaffold a new migration (`.up.sql` / `.down.sql`)
- `make job NAME=<job> ARGS="<args>"` — run an application job

**Graphify (standard equipment):** a queryable knowledge graph of this repository. The version is
declared in `python/graphify.in` and locked with hashes in `python/graphify.txt` — **not in
`mise.toml`** — and the skill is written into each assistant's config by
`bash .claude/scripts/bootstrap-external-skills.sh`. It indexes **structure**, which is what makes it
reach the two things text search cannot: a caller that shares no vocabulary with its callee, and a
document named for the concern it owns rather than for the words in your question.

```bash
GRAPHIFY="${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify"   # 固定版。bootstrap が作る venv
node .claude/scripts/graph-affected.ts <symbol> --depth 2   # 影響範囲・呼び出し元の逆引き
"$GRAPHIFY" query "<question>" --budget 8000
"$GRAPHIFY" path "<A>" "<B>"      # 2 ノード間の最短経路
"$GRAPHIFY" explain "<node>"      # ノードと隣接の平易な説明
make graphify-update              # グラフを現在のコードへ更新（コンテナ内、固定版）
```

- **`affected` is the paying command.** Reverse traversal returns each call site with a relation
  label and `file:line` — which is the evidence a claim of scope ("only X does this") requires, and
  the number a blast-radius estimate should be replaced by. It takes a node **id**, not a symbol
  name, so go through the wrapper; `path` / `explain` / `god-nodes` take ids straight from
  `graphify-out/graph.json`.
- **Raise `--budget`.** The default (~2000 tokens) truncates on a repo this size and says so; the
  answer may be in the cut part.
- **Ignore `god-nodes` here.** It ranks by edge count, and this repo's 1:1 test-mapping rule puts
  test scaffolding (`Any()`, `NewTestFromSalt()`, …) above production code.
- **State freshness whenever you used it.** Compare `Built from commit:` in
  `graphify-out/GRAPH_REPORT.md` against `git rev-parse HEAD`. For a question about *uncommitted*
  work the graph is blind — refresh with `make graphify-update` (deterministic, no API key) or use
  `grep`, which for a small diff is the cheaper of the two.
- **The graph is a way to reach a file, never the evidence itself.** Open what it points at and cite
  that. A graph result carries no separation of fact from inference.
- **AST-only commands are free and local.** `update` / `query` / `affected` / `path` / `explain` need
  no API key. Docs / PDF / image extraction, `extract`, `label`, community *naming*, `add`, and
  `--wiki` call an LLM API and send content off the machine — those stay opt-in.
- **Use the pinned binary, not `mise exec pipx:…`.** `python/graphify.txt` is installed twice from
  the same hashes — into `python_tool_runner` (what `make graphify-*` runs) and into the
  bootstrap's venv (the `$GRAPHIFY` path above). A bare `mise exec "pipx:graphifyy[sql]"` resolves
  whatever pipx hands back instead, which on this machine is *behind* the lock. Anything that
  **writes** the graph goes through `make graphify-update`: `docker/tools/Dockerfile` explains why —
  extraction results and the cache namespace depend on the prompt bundled with the tool, so the
  version has to be the fixed one. Read-only queries against an existing `graphify-out/graph.json`
  only need to be at the pinned version, not in the container.
- `make graphify-check` verifies the tracked artifacts, `make graphify-pending` reports how much
  semantic extraction is outstanding, and `.graphifyignore` declares what the graph excludes
  (changing it requires a full re-extraction; an incremental `update` is fail-closed).

Measured behaviour on this repository, and what does *not* pay off here:
`.claude/README.md`.

**Working in a `git worktree` (DB + serve isolation):** a single shared Postgres (fixed compose
project `gobp-shared`, host 5432) is shared by all worktrees; each leases a slot = its own
databases (`wt<N>_local` / `wt<N>_test`) inside that instance. Before DB-backed tasks or `make serve`
in a worktree, `make slot-acquire` to lease a slot (creates + rebuilds `wt<N>_local` / `wt<N>_test`,
propagates `DB_NAME_LOCAL` / `DB_NAME_TEST` / `API_HOST_PORT` `8080+N` / `MOCK_AUTH_HOST_PORT` `2010+N`),
then `make test` connects to `wt<N>_test` on localhost:5432, `make serve` isolates the app in
`gobp-wt-N` (curl `localhost:$API_HOST_PORT`) against the shared DB, and `make slot-free` when done —
do NOT start a duplicate DB stack or hijack another checkout's containers. To retire the worktree
entirely, `make slot-release` stops the app + removes its local images, frees the slot, and removes
the worktree, in that order. Without `slot-acquire`,
targets default to `local` / `test` on 5432 / 8080 / 2010 (single-stack, unchanged).
Details: `docs/maintenance/db-worktree-pool.md`.

**DB clean-up (worktree slot pool):** the pool shares one Postgres instance, so tables from another
branch's migrations can linger in a DB you reuse. As part of clean-up — at the start of DB-backed
work, and whenever the shared DB carries stale tables — rebuild your DB from THIS branch's migrations
so you always work against a clean, migration-faithful schema: `make slot-acquire` re-creates the
slot's `wt<N>` DBs this way, and `make db-local-reinit` / `db-test-reinit` drop every `public` table
then migrate-up + seed the shared `local` / `test`.

## Protected Documentation

`AGENTS.md` must be maintained by humans only. AI agents must NOT modify it unless explicitly
instructed by a human; changes must be intentional and reviewed carefully, as it defines
repository-wide development rules.
