# Agents Documentation

This repository is a lightweight Onion Architecture RESTful-API scaffold
(`controller → usecase → domain`; infrastructure implements domain interfaces). Do not
introduce new architectural patterns unless explicitly instructed. Architecture, rules and flows
are not restated here — the table below says which document owns each.

Three constraints apply to every task:

1. A **deterministic check** — test, lint, CI, architecture rule — outranks an agent's judgment
   wherever one exists. Report what it said, not what you concluded.
2. **Architecture, domain and policy decisions keep a human gate.** Surface the decision and its
   options; do not take one.
3. **The application never depends on AI.** Runtime, build, test, the domain model, the API
   contract, the DB schema and the ordinary CI checks must succeed with no agent available. AI
   dependence is confined to development workflow, navigation, automation, feedback and review.

Rationale: `docs/adr/0007-agents-md-operational-contract.md` and
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

Do not silently pick one, and do not ask 「三つとも回しますか」 — that hands the cost back unpriced.
**Estimate each skill's return from the context you already hold** — which layers the change touched,
whether tests or comments moved at all, what an earlier skill in this session already covered — then
**ask per skill whether to run it, saying which pass you expect to pay off, which you expect to return
nothing, and why**, and run what they approve.

The three are **peers, and none of them invokes another.** One subject to one skill, and that skill is
the only place its subject is audited: `/impl-review` owns no test lens and no comment lens, and the
other two are invoked in their own right whether or not it runs. The coupling belongs in the *asking*,
not in the skills. **This holds inside a pipeline too** — a skill that drives an issue to a merged PR
asks these three questions at its review phase rather than choosing for the user.
<!-- boilerplate-only:end -->

## Layer Rules

Import direction is a deterministic gate, not something to hold in your head: `depguard`'s
`maintain_a_sound_*` and `independent_pkg` rules fail the build on a crossed layer boundary, and
`internal/architest`'s `TestDomainAggregateImportIsolation` on a cross-aggregate import; `forbidigo`
rejects `time.Now` inside `internal/(domain|usecase)` and `time.Local` everywhere. The full table
and its rationale are in `docs/rules.md`; the domain lexicon and the `internal/domain/service/<name>/`
exception carry their admission criteria in `internal/domain/README.md`.

No gate catches the rest, so hold it yourself:

- **No `context.Context` in domain logic.** A Repository interface signature may declare one for
  propagation; nothing else in the domain takes one.
- **The domain depends on no ambient state beyond what those gates already block.** Time is gated and
  `uuid` is forced through `pkg/uuid`, but `math/rand` and every other environment read are yours to
  abstract behind a domain interface implemented in an outer layer.
- **Usecase owns the transaction boundary and maps domain models to DTOs** — a domain entity never
  reaches an outer layer.
- **Controller handlers are request / response only** — no business logic.
- **`pkg/` carries no feature-specific business logic** and stays framework-agnostic.

## Forbidden Shortcuts

This file does not enumerate them. What can be decided mechanically is caught by `depguard`, the
architecture tests in `internal/architest`, and the generated-file denials in `.claude/settings.json`;
the rest is stated by the `README.md` of the package you are touching — which is why step 3 of the
*Task Execution Protocol* is not optional. A rule this file does not repeat is still a rule.

## Installing Things

This covers every `install` surface, not one tool: package managers (`brew`, `pnpm add -g`, `pip`,
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
Why those entries are patterns rather than named platforms: `docs/design/security.md`.

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

This section governs what you **recommend**, never what you may change — *Conflicting Authority*
above, `docs/rules.md`, and the modification scope below still decide that.

This repository's product is **the state a project receives at `useTemplate` time**, not the history
that produced it, so weigh options for that snapshot: what reads as coherent to someone who has never
seen this repository and will never read its git log. **On that axis, quality and consistency outrank
the cost of reaching them** — a numbering that contradicts the order it teaches, a convention followed
everywhere but here, a name that survives only because renaming it is work: recommend fixing them.
"It already shipped" carries little weight while nobody has instantiated from this in production.

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
- **Coverage % is a proxy, not the goal**, and a test is meaningful only if the contract it protects
  can actually regress, is not already locked elsewhere, and is owned by that layer.
  `docs/testing-conventions.md` (sections 8 and 10) is the single source for test quality — both
  `scaffold-test` and `test-review` read it, so a criterion kept anywhere else goes unapplied.

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
   - **Take the base from nowhere else.** The local `refs/remotes/origin/HEAD`, the GitHub default branch (`gh repo view --json defaultBranchRef`), and a harness-supplied "Main branch" value are each stale or lagging, and all three answer without warning — a feature branch cut from a generation-old base stays invisible until the files everyone expects turn out to be missing.
   - **Where a pull request already exists, its `baseRefName` is the authority** and the resolver is the fallback. A PR's base is what it is already merging into; nothing may re-resolve it.
   - **During a hotfix, resolve nothing — ask.** `make hotfix-patch` cuts a `hotfix/vX.Y.Z` and makes it the GitHub default, so the branch under active development is then not the latest `release/*` and `make base-branch` — which considers `release/*` only — will not name it. A hotfix's base is a human decision taken on the spot.
2. Do NOT rebase, squash, or force-push unless the user explicitly requests it.
3. After amending an existing PR branch, do NOT auto-push — ask first: 「変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？」
4. **Syncing a feature branch with an advanced base — merge, never rebase (see rule 2).** When the base `release/*` has moved ahead and the feature branch must catch up, fast-forward the local base to its remote and **merge** it into the feature branch (`git merge origin/release/vX.Y.0`), rather than rebasing / force-pushing. The branch to merge is the one this feature branch was cut from — the pull request's base — **not** whatever `make base-branch` resolves today: if a newer release line has opened since, merging that would retarget the branch instead of catching it up. Resolve conflicts in generated artifacts (`**/*.gen.*`, `docs/openapi/**`, `openapi/openapi.gen.yaml`, …) by **regenerating from the source of truth** (`make gen-api` / `make gen-query`), not by hand-editing the generated output. Rebase only when the user explicitly requests it.

**Commit / PR execution:** split into scoped commits using the prefix convention
(Feat / Fix / Refactor / Perf / Docs / Test / Build / CI / Chore / Style / Revert — the enum
`commitlint.config.js` verifies) and add the `Co-Authored-By` footer. If your agent provides a
dedicated command/skill for this workflow, prefer it over manual steps; the hook handling and the
ordering of verification belong to that procedure, not here.

**Branch naming:** include the issue number when provided (`feature/1234-description`);
otherwise a descriptive hyphenated name (`feature/add-authentication-check`).

**Linking to another repository's issue / PR — always go through `redirect.github.com`.**
A plain `https://github.com/<owner>/<repo>/issues/N` URL, a `[text](url)` link around one, or the
`owner/repo#N` shorthand posts a public cross-reference on the upstream thread, and **this is not
fixable after the fact** — editing the body does not retract it, and pull requests cannot be deleted
at all. `https://redirect.github.com/<owner>/<repo>/issues/N` is a `github.com` subdomain that
301-redirects to the real page, so the link works but GitHub does not autolink it; this is GitHub's
own documented escape hatch (see "Autolinked references and URLs"). Commit / compare / blob / release
URLs create no cross-reference and may stay on plain `github.com`.

**A plain link is not forbidden — it is reserved**, for deliberately saying "we are watching this" or
"we need this"; when you use one, write the referencing issue's title in the language of the target
repository (usually English), since the title is the only thing upstream sees. **The decision belongs
to a human, without exception** — default to `redirect.github.com` and ask every single time, and a
standing delegation ("you decide", "always link normally from now on") does NOT transfer this
authority. Why: `docs/design/agent-environment.md`.

## Language Rules for AI Agents

Internal reasoning may be in English. **All visible outputs must be in Japanese** unless the
user explicitly requests English — test case names, code comments, PR messages, inline
documentation, and responses to the user.

## Recommended Commands

The **full `make` target registry** is `.makefiles/README.md` (targets grouped by area;
every target is self-documenting, so `make help` lists them). Common ones:

Code generation:

- `make gen-api` — generate API code from the OpenAPI spec (oapi-codegen + mock)
- `make gen-query` — generate SQL query code from SQL files (sqlc)
- `make gen` — all of the above plus the docs (`gen-api` → `gen-query` → `gen-docs`)
- `make tidy-lib` — `go mod tidy` + `go mod vendor` after any dependency change

Format / lint / test:

- `make go-fix` — auto-format + auto-fix lint (run before committing; then fix what remains)
- `make go-lint` — Go static analysis (golangci-lint)
- `make go-test` — run all tests with coverage
- `make go-lint-fast` + `make go-test-arch` — the pair to run while implementing: only what
  propagates to other files, no DB, seconds rather than minutes. Neither is a gate — `make go-lint`
  / `make go-test` and CI decide (ADR-0088)
- `make md-lint` / `make md-fix` — Markdown lint / auto-fix
- `make sql-lint` / `make sql-fix` — SQL lint / auto-fix

Run / DB:

- `make serve` — start the local dev environment (for runtime / `curl` verification)
- `make serve-build` — build the app image first, then start. Reach for it when `serve` exits 0 but
  the API never answers: a cached image built on an older toolchain fails silently
- `make serve-stop` — stop this checkout's app container (the shared infra stays up)
- `make db-init` — migrate + seed both the local and test DBs (prerequisite for DB-backed tests)
- `make new-migrate-<name>` — scaffold a new migration (`.up.sql` / `.down.sql`)
- `make job NAME=<job> ARGS="<args>"` — run an application job; `make worker` / `make outbox-relay`
  run the resident worker / outbox relay (Ctrl-C to stop)

Git:

- `make base-branch` — print the latest `release/*` line from `origin`'s live state (the only
  admissible base, per the Git rules above)

**Graphify (standard equipment):** a queryable knowledge graph of this repository, pinned in
`python/graphify.in` — **not in `mise.toml`**. It indexes **structure**, which is what makes it reach
the two things text search cannot: a caller that shares no vocabulary with its callee, and a document
named for the concern it owns rather than for the words in your question.

```bash
GRAPHIFY="${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify"   # 固定版。bootstrap が作る venv
node .claude/scripts/graph-affected.ts <symbol> --depth 2   # 影響範囲・呼び出し元の逆引き
"$GRAPHIFY" query "<question>" --budget 8000
"$GRAPHIFY" path "<A>" "<B>"      # 2 ノード間の最短経路
"$GRAPHIFY" explain "<node>"      # ノードと隣接の平易な説明
make graphify-update              # グラフを現在のコードへ更新（コンテナ内、固定版）
```

- **State freshness whenever you used it.** Compare `Built from commit:` in
  `graphify-out/GRAPH_REPORT.md` against `git rev-parse HEAD`, and say so in the answer. For a
  question about *uncommitted* work the graph is blind.
- **The graph is a way to reach a file, never the evidence itself.** Open what it points at and cite
  that. A graph result carries no separation of fact from inference.
- **Keep the LLM-calling commands opt-in.** The commands above are AST-only and local; docs / PDF /
  image extraction, `extract`, `label`, community *naming*, `add` and `--wiki` send content off the
  machine.

Which commands pay here and which do not, the pinned-binary discipline, and what the graph excludes:
`.claude/README.md`.

**Token proxy (`rtk`, standard equipment):** compresses shell output before it reaches your context.
The version is pinned in `mise.toml`; nothing in build / test / CI invokes it, so a checkout without it
behaves identically.

**Prefer it by default** — a command it cannot compress is passthrough, so there is nothing to weigh
per command. Three places using it is wrong:

- **Never read source through `rtk read`.** Its default level is lossless and therefore saves nothing;
  every other level drops lines, and a filtered file that still looks complete is worse to reason from
  than a long one.
- **Never report a deterministic check through it.** Constraint 1 above requires reporting what the
  test / lint / gate said, and `rtk` shows failures only, dropping counts and coverage. When the
  number itself is the subject, take the raw output with `rtk run <command>`.
- **`rtk diff <rev>` returns empty** — it takes file paths, not git revisions. Use `rtk git diff`.

The exclusion criterion, and the machine-local setup the pin does not reach: `.claude/README.md`.

**Working in a `git worktree` (DB + serve isolation):** all worktrees share one Postgres, and each
leases a slot for its own databases. Before DB-backed tasks or `make serve` in a worktree, run
`make slot-acquire` (`make slot-status` shows what the pool holds, and is the first thing to check
when acquire fails); `make slot-free` when done, or `make slot-release` to retire the worktree
entirely. `make serve` then isolates the app per worktree — curl `localhost:$API_HOST_PORT`. **Do NOT
start a duplicate DB stack or hijack another checkout's containers.** Without `slot-acquire`, targets
default to `local` / `test` on 5432 / 8080 / 2010 (single-stack, unchanged).
Details: `docs/maintenance/db-worktree-pool.md`.

**DB clean-up (worktree slot pool):** the pool shares one Postgres instance, so tables from another
branch's migrations can linger in a DB you reuse. At the start of DB-backed work, and whenever the
shared DB carries stale tables, rebuild from THIS branch's migrations: `make slot-acquire` for a
slot's DBs, `make db-local-reinit` / `db-test-reinit` for the shared `local` / `test`.

## Protected Documentation

`AGENTS.md` must be maintained by humans only. AI agents must NOT modify it unless explicitly
instructed by a human; changes must be intentional and reviewed carefully, as it defines
repository-wide development rules.
