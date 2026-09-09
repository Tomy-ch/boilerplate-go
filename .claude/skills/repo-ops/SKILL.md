---
name: repo-ops
description: >-
  Operational runbook for this repository's recurring, easy-to-trip-on gotchas around the two-layer docker compose setup, the shared Postgres + worktree slot ring, generated artifacts, the dockerized tool runners, and the local/CI gates (lefthook hooks + GitHub Actions). Read-only knowledge skill — it names the exact command; it does not silently mutate state. Use it for a symptom: 「schema.gen.sql が diff る / gen-*-artifacts-check が落ちる」「docker compose ps に何も出ない / 5432 が既に使われている」「git が docs/portal や mock を permission denied」「gen-query が DB に繋がらず落ちる / 2 つの checkout がスキーマ生成で競合する」「slot 取得後にテストが別 DB を見る」「新しい worktree で make serve しても API が上がらない / air が inconsistent vendoring で落ちる」「触っていないのに hook や CI が落ちる（secret-scan / sample-removal-check / pin-images-check / migration 番号 / sync-versions）」「env/.env が勝手に汚れる」「ローカルの golangci-lint が CI と食い違う」「commitlint / orval が not found」「per-env イメージのビルド」「どのドキュメントが正本か分からない / grep が対訳・生成物に埋もれる」. Do NOT use it for a goal with no symptom behind it (`how-to`) or to explain how something works (`repo-truth`).
---

# Repo Ops Runbook

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

Concrete recovery steps for the operational gotchas that recur in this repo. This is a lookup
table, not a workflow: find the symptom, run the fix. When a step is destructive (drops DB data,
`chown`s a tree, stops containers other checkouts rely on) say so to the user first per `CLAUDE.md`.

Three facts explain almost everything below:

1. **Codegen runs as root inside Docker tool-runner containers** (`go_tool_runner` /
   `node_tool_runner` / `python_tool_runner`) over the `.:/app` bind mount → generated files come
   back root-owned, and the tools live in the image, not on your host.
2. **Infra is a single shared compose project.** `database` / `observability` / `garage` run once
   for *every* checkout under the fixed project `gobp-shared`; only `api_server` /
   `mock_auth_server` are per-checkout. Worktrees are separated by *database name*
   (`wt<N>_local` / `wt<N>_test`), not by port. Canonical design:
   `docs/maintenance/db-worktree-pool.md`, topology: `docs/maintenance/local-environment.md`.
3. **`make go-lint` / `fix` / `test` run on the host** via mise — the exception to the "everything is
   dockerized" rule, and the source of host-vs-CI mismatches.

## Contract

| | |
| --- | --- |
| **Owns** | 既知の運用症状 → 是正手順の索引（§1-21）と、どの文書が正本かの所在（§0） |
| **Never** | 索引に無い手順を発明する / 「未定義」を宣告する / 設計判断 / 実行承認の代行 |
| **Starts when** | 症状・失敗したゲート・想定外の挙動が提示されたとき |
| **Stops when** | 症状が索引に無い（`how-to` / `repo-truth` へ）、資料が矛盾する、要求が許可範囲を超える |

## Symptom index

| Symptom | Where the fix lives |
| --- | --- |
| `docker compose ps` empty / "port 5432 already allocated" / service not found | §1 |
| `schema.gen.sql` or sqlc output drifts, `gen-db-artifacts-check` fails | §2 |
| `make gen-query` fails to reach the DB, or two checkouts clobber each other | §3 |
| `git add` / `restore` → permission denied on a generated dir | §4 |
| Tests / migrations hit the wrong database after `make slot-acquire` | §5 |
| Integration tests fail right after switching branches | §5 |
| pre-push `secret-scan` flags a secret you did not add | §6 |
| `sample-removal-check` fails in CI | §99 <!-- sample-api:line --> |
| `env/.env` is dirty and you did not edit it | §7 |
| Local golangci-lint disagrees with CI, or `golangci-lint: not found` | §8 |
| `commitlint: not found`, `orval: not found`, stale tool version | §9 → `tools-upgrade` (version bumps) |
| `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` in a containerized gate, or one gate fails only inside Docker | §9 |
| A containerized gate reports missing dependencies for a package you never touched, or `make` fails where a bare `docker compose run` passes | §9 |
| A hook fails for something outside your change | §10 |
| A gate fails / crawls for reasons unrelated to the change while several worktrees are open | §19 |
| Want to know why `make go-lint` skipped, throttled, or deferred itself to CI | §19 |
| `pin-images-check` / `pin-actions-check` errors (未固定 / 未登録) | §11 → `images-pin` / `actions-pin` |
| `tool-cooldown` / `go-cooldown` refuses a version you just declared | §20 → `supply-chain-triage` |
| A runtime bump breaks a gate through a tool that cannot be upgraded yet | §20 → `tools-upgrade` |
| "Migration version gap / duplicate" from pre-commit | §12 |
| S3 calls return 503 locally | §13 |
| Building an image for a specific environment | §14 |
| `sync-versions` drift in CI | §15 → `go-upgrade` (Go version bumps) |
| `go: inconsistent vendoring` from air or the image build in a fresh worktree (`vendor/` absent) | §18 |
| Cannot tell which document decides an answer / `grep` drowns in mirrors and generated copies | §0 → `repo-truth` |
| Conflicts after taking the base in, or markers inside a generated artifact / pin lockfile | §21 → `resolve-merge` |
| `gen-*-artifacts-check` fails after a merge, in files the branch never touched | §21 → `resolve-merge` |

**Where a skill owns the fix, the index names it, and the section stops at the boundary.** A symptom
whose resolution is a procedure — not a command — belongs to whoever owns that procedure: pin
lockfiles to `images-pin` / `actions-pin`, a refused version to `supply-chain-triage`, a version bump
to `tools-upgrade` / `go-upgrade`, a merge to `resolve-merge`, "which document decides this" to
`repo-truth`. The section keeps the diagnosis — what the symptom means, why the gate fired, what is
destructive about it — and hands the steps over rather than restating them, because a restated
procedure is the copy, and the copy is what rots when the owner changes.

Routing from the index rather than from inside the section matters: a reader who has to finish a
section before learning it defers to a skill has already spent the reading the deferral was meant to
save.

<!-- sample-api:begin -->
**A section that `make setup-remove-sample-api` deletes is numbered out of sequence (§99).** Inside
the running sequence its removal would leave a permanent gap in the runbook this repository ships;
out of sequence, deleting it changes nothing else. Number a removable section that way, and give
new sections the next sequential number.
<!-- sample-api:end -->

**A symptom that is not in this index does not belong here.** This is a lookup table of known
gotchas, not a search: it answers "the fix for this is §N", never "no fix exists". Establishing that
a procedure is genuinely undefined takes an exhaustive sweep of the owning indexes, and two other
skills own that verdict: `how-to` for "what is the sanctioned way to do X" (goal-driven, reporting
`UNDEFINED` with the frontier it searched), `repo-truth` for "what is the rule / how does this work"
(distinguishing 未定義 from 確認できず the same way). Teaching this table to conclude "undefined" would
turn it into a general search skill and make its silence indistinguishable from an answer.

## 0. Finding the authoritative source

Most of the Markdown in this tree is either a Japanese mirror you must not read or generated output
that lags the code, so a naive repo-wide search buries the one file that actually decides the answer.
Of roughly 1,000 tracked `*.md`, **over 40% are `*.ja.md` translations** and **72 are generated
`docs/portal/guides/**` copies of READMEs**; `docs/godoc/**` adds ~1,800 files and
`docs/db-schema/**` ~400.

### Where the truth lives

| To answer | Read | Looks authoritative but is not |
| --- | --- | --- |
| What a make target runs / which target does X | `.makefiles/**/*.mk`, index `.makefiles/README.md`, `make help` | `docs/portal/guides/make.md` — generated, lags the `.mk` |
| Layer boundaries, generated-file / DTO / tx / comment rules | `docs/rules.md` | — |
| System structure, layer responsibilities | `docs/architecture.md` | — |
| How to carry out a change (API / DB / business logic) | `docs/development-flow.md` | — |
| Test conventions | `docs/testing-conventions.md` + the layer README's *Test Strategy* | — |
| Why something is designed this way | `docs/adr/` (100+ records; log at `docs/adr/README.md`) | — |
| How a subsystem works (rest / worker / job / outbox / idempotency / o11y / auth) | `docs/design/README.md` and its siblings | — |
| Local topology, compose layering, hot reload | `docs/maintenance/local-environment.md` | — |
| Worktree slot ring, shared DB | `docs/maintenance/db-worktree-pool.md` | — |
| Per-package detail and design intent | the nearest `internal/**/README.md` / `pkg/**/README.md` (100+ of them) | a skill body — skills follow READMEs, not the reverse |
| An environment variable | `internal/config/envspec.go` + `model.go`, table in `env/README.md` | a value in `env/.env*` alone |
| What a CI gate or hook actually checks | `.github/workflows/*.yaml`, `.lefthook.yaml` | — |
| Tool / runtime versions | `mise.toml` (everything else is derived — §15) | `go.mod`, Dockerfiles, READMEs — all derived |
| Generated artifacts, protected paths, scope | `AGENTS.md` | — |

### Searching without the noise

```bash
rg "<pattern>" \
  -g '!**/*.ja.md' -g '!docs/portal/**' \
  -g '!docs/godoc/**' -g '!docs/db-schema/**' -g '!docs/openapi/**' -g '!docs/coverage/**'
```

Typical effect: hit counts drop by half or more (e.g. `gen-query`, `NormalizeError`). `rg` honours
`.gitignore`, so `vendor/` and `node_modules/` are already excluded — but the generated `docs/` trees
are *tracked*, so they need these explicit globs. Hitting a `*.ja.md` is still useful as a
**locator** (it proves the topic is documented); read the English original beside it, per
`AGENTS.md`'s rule never to read `*.ja.md`.

### When sources disagree

Follow `AGENTS.md`'s instruction priority: `AGENTS.md` → `docs/rules.md` → `docs/architecture.md` →
user instruction. For design intent and implementation policy the order is **README > Code > SKILL**
(the rule `back-prop` enforces): a skill that contradicts a README is the thing that is out of date.
If code and its README disagree, that is drift worth surfacing — `back-prop` detects it.

## 1. Compose project resolution — a bare `docker compose` targets the wrong project

Compose derives its project name from the directory, so a bare `docker compose <cmd>` in this repo
(or in a worktree) addresses an *empty* project — not the shared infra. `.makefiles/docker/compose.mk`
exports `COMPOSE_PROJECT_NAME=gobp-shared`, so this only works when the command comes from `make`.

```bash
docker compose ps                  # ← empty: project = <directory name>
docker compose -p gobp-shared ps   # ← the actual shared infra
```

Consequences of getting it wrong: `docker compose up -d database` starts a **second** Postgres that
collides on host port 5432; `docker compose exec database psql …` reports the service is not running;
`docker compose run --rm go_tool_runner make db-migrate-ci-up` cannot resolve the `database` host
because the one-off container joined a different network.

Fix: drive infra through `make`, or set the project explicitly.

```bash
make infra-up                                    # start database / observability / garage (+ garage_init)
make serve                                       # infra-up + this checkout's app layer
make serve-stop                                  # stop only this checkout's app
COMPOSE_PROJECT_NAME=gobp-shared docker compose exec -T database psql -U postgres -l
```

`make infra-down` stops the infra for **every** checkout and worktree — confirm before running it.

## 2. Generated DB artifacts drift → `gen-db-artifacts-check` fails

`database/gen/schema.gen.sql` and the sqlc output (`internal/infrastructure/rdb/sqlc/gen/*.gen.sql.go`,
`database/gen/*.gen.sql`) are produced by `make gen-query`. Any change under `database/migrations/**`,
`database/dml/**`, or `docker/database/**` changes them, so committing the source without the
regenerated output fails the CI check (regenerate → `git diff`).

```bash
make infra-up
make gen-query                     # dump-schema → merge-dml → sqlc generate → fmt
git add database/gen internal/infrastructure/rdb/sqlc/gen
```

Rule of thumb: **a schema- or DML-affecting change and its regenerated artifacts belong in the same
change.** If CI reports drift while the PR touches no SQL source, it is generator-side drift (sqlc
version, stale artifacts on the base) — regenerate locally, or refresh the base and merge.

## 3. `make gen-query` — needs infra up, and `gen_schema` is shared

`dump-schema` no longer dumps your `local` database. It bootstraps a dedicated **`gen_schema`**
database (`SCHEMA_GEN_DB` in `.makefiles/database/gen.mk`), drops its tables, migrates it up from
*this branch's* migrations, and dumps that. So stale tables in your working database can no longer
leak into generated code — but two consequences remain:

- It still talks to the shared Postgres via `docker compose exec database`, so the infra must be up
  (§1); otherwise it dies with a connection error.
- **`gen_schema` is a single database name inside the one shared instance.** Two checkouts running
  `make gen-query` at the same time rebuild the same database and corrupt each other's output. Run
  schema generation in one checkout at a time. (This constraint currently lives only in the
  `gen.mk` comment — it is not in `docs/`.)

## 4. `git` refuses a generated directory — it became root-owned

Tool runners run as root over the bind mount, so files they *create* are root-owned on the host:
`docs/portal/{guides,docs.json}` from `make gen-portal-docs`, freshly created mock directories from
`make gen-go-code` (e.g. `pkg/fs/mock`), `docs/godoc/`, and any newly added generated file. Host
`git add` / `git restore` then fail with `permission denied`.

Fix: hand ownership back through a container (preferred over host `sudo`):

```bash
docker compose run --rm --user root node_tool_runner chown -R $(id -u):$(id -g) /app/docs/portal
docker compose run --rm --user root go_tool_runner   chown -R $(id -u):$(id -g) /app/pkg/fs/mock /app/pkg/exec/mock
```

Trap: `git restore docs/portal` to clean up root-owned files also **reverts hand edits** under
`docs/portal` — re-apply them after the chown, in a separate commit.

## 5. Wrong database after acquiring a slot, or after switching branches

`make slot-acquire` writes `.gobp-db-slot` and make propagates `DB_NAME_LOCAL` / `DB_NAME_TEST`
(`wt<N>_local` / `wt<N>_test`) from it. Two traps follow:

- **`db-init` / `db-local-reinit` / `db-test-reinit` hardcode `DB=local` / `DB=test`.** After
  acquiring a slot they rebuild the *shared* databases, not yours — destructive to whoever is using
  them. Target your own explicitly:

  ```bash
  set -a; . ./.gobp-db-slot; set +a
  make db-reinit DB="$DB_NAME_LOCAL"      # drop tables → migrate-up → seed
  make db-reinit DB="$DB_NAME_TEST"
  ```

- **A bare `go test ./...` does not see `DB_NAME_TEST`** — make exports it, your shell does not — so
  it silently connects to the shared `test` database, which another branch's migrations may own.
  Run `make go-test` (or export the variable yourself).

Integration tests (`internal/integration`, `internal/infrastructure/rdb/**`) do not migrate anything;
they expect a migrated + seeded database. After switching to a branch with different migrations,
rebuild first — `make slot-acquire` does it for slot databases, `make db-test-reinit` for the shared
`test`. Prefer `db-reinit` over `db-init`: `db-init` runs `migrate-down` first and cannot recover a
dirty schema whose `.down.sql` no longer exists.

## 6. pre-push `secret-scan` flags a secret you did not add

`.gitleaksignore` entries are fingerprints of the form `<path>:<rule>:<line>` — they **embed the line
number** (e.g. `docker-compose.yaml:generic-api-key:134`, `env/.env:generic-api-key:64`). Editing
those files shifts the line, the old fingerprint stops matching, and the intentionally-allowed
sample credential is reported as a fresh finding.

```bash
make secret-scan          # reproduce; the output carries the new fingerprint (values are redacted)
```

Then update the matching line in `.gitleaksignore` to the new number, keeping the explanatory comment.
Only do this for entries already documented there as intentional (the JWKS rotation test's signing keys, Garage dev
credentials) — a genuinely new finding is a real secret and must be removed from the tree instead.

## 7. `env/.env` is dirty and you did not edit it

`env/.env` is committed, and `make materialize-env` overwrites it with `env/.env.$(APP_ENV)` (default
`ci`) so the value set can be embedded at build time. CI and image builds do this, and an interrupted
local run leaves the copy in place.

```bash
make restore-env          # git restore env/.env
```

Never commit a materialized `env/.env`. Note that editing this file also shifts the gitleaks
fingerprint in §6.

## 8. Host-side lint / format / test, and the two golangci configs

`make go-lint` / `make go-fix` resolve golangci-lint through mise on the **host** (`mise which golangci-lint`),
and `make go-test` runs `go test` on the host too. If the binary is missing, `mise install` — do not
reach for a container.

The repo ships two configs, and a bare `golangci-lint run` picks the wrong one:

```bash
golangci-lint run                                  # → .golangci.yaml (lightweight; some linters off)
make go-lint                                          # → .golangci.yaml, what CI runs
golangci-lint run --config .golangci.yaml     # equivalent, when you need extra flags
```

Always reproduce CI failures with the full config.

## 9. `commitlint: not found` / `orval: not found` / `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` / a stale tool version

The tool-runner images are **build artifacts of `docker/tools/Dockerfile`, `mise.toml`, and the
`package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` triple of every package the node runner
installs in-tree — `scripts/` and `docs-viewer/`**. Tools
are resolved inside the runners, never on the host (the same reproducibility rule as codegen — see
`docs/rules.md`). After changing any of those files — or on a fresh clone whose images predate them —
the runner is missing the tool, ships an old version, or refuses to run anything at all.

The cross-package span is the part that surprises: a change confined to `docs-viewer/` still
stales the image that `scripts/`-based gates run in, because both are installed into the one node
runner.

That last symptom is the one that reads as unrelated to the edit. `scripts/pnpm-workspace.yaml` sets
`verifyDepsBeforeRun: error`, so once its settings no longer match what the image's
`scripts/node_modules` was installed under, every `pnpm run` inside the runner fails with
`[ERR_PNPM_VERIFY_DEPS_BEFORE_RUN] The value of the <setting> setting has changed`. It takes down
gates with no visible connection to the change — `make md-lint`, `make actions-lint`, `make
oapi-lint` — while the host-side `-ci` targets stay green, because the host tree was re-installed
when the file changed. Adding a `minimumReleaseAgeExclude` or `overrides` entry is enough to trigger
it. Green on the host is therefore **not** evidence that the containerized gate passes: rebuild
first, then re-run whichever gate you intend to report.

A stale image also shows up as **a containerized gate reporting missing dependencies for a package
you never touched**. The 1:1 test gate (`scripts/one-to-one.gate.test.ts`) type-checks both packages,
so an image built before `docs-viewer/` gained its current manifests answers with a column of TS2307
`Cannot find module` against `docs-viewer/src/**`. It reads as a dependency the repository forgot to
declare, which is the wrong tree to search.

**The tell is that `make` fails while the same command through a bare `docker compose run` passes.**
`.makefiles/docker/compose.mk` exports `COMPOSE_PROJECT_NAME` as the shared infra project (§1), so
`make` always reaches the `gobp-shared` image; a bare invocation takes the project from the directory
name and therefore hits a different — usually newer — one. Read that asymmetry as an image-age
difference, never as evidence about the code.

**Across worktrees this is a shared resource, and it holds one branch's settings at a time.** The
runner images belong to the single `gobp-shared` compose project (§1), so a rebuild in one worktree
re-points every other worktree's containerized gates at *that* branch's `scripts/pnpm-workspace.yaml`.
Two windows whose branches disagree — one adding a `minimumReleaseAgeExclude` entry, one not — will
take turns failing, and the direction flips with whoever rebuilt last. The failure is the same
`ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` in both directions, so read it as "the image matches someone
else's branch", not as a defect in yours. Rebuild before the gates you are about to report, and
expect the other window to need the same.

```bash
make tool-runners-build           # rebuild go / node / python runners (cached)
make tool-runners-build-clean     # --no-cache --pull, when the cached layer is the problem
```

**While the cause of a containerized failure is still unidentified, rebuild clean before concluding
anything about it.** "Unrelated to my change" and "a pre-existing failure on the base branch" are
conclusions, and a stale image produces evidence for both. So spend the rebuild first: run
`make tool-runners-build-clean`, restart whatever is already up (`make serve` and the tool stack, not
only the runner that failed), then re-run the gate. It costs minutes and it is the cheapest way to
eliminate the one explanation that mimics every other one. Reaching for `--no-verify` while the cause
is unknown skips that check rather than passing it.

The node runner also carries `/app/scripts/node_modules` as an anonymous volume (so the bind mount
does not shadow it); helper scripts resolve their binaries from there, which
is why a stale image breaks them.

The commit-msg hook runs `make commitlint COMMIT_MSG_FILE={1}` through `node_tool_runner`, so this is
the usual cause of a failing commit. `commitlint.config.js` deliberately disables `type-case` (the
repo prefixes are Cap-first `Feat`/`Fix`/… while CI messages are upper-case, so no single case can be
enforced) and pins `type-enum` to the project prefixes; `Merge` / `Revert` are ignored by default.

This section covers a tool that is *missing or stale in the image*. Deliberately **raising** a pinned
version is a different job with its own supply-chain window, and `tools-upgrade` owns it — do not
bump a pin in `mise.toml` to make this symptom go away.

## 10. Hook map — what runs when, and what to do when it fails for reasons outside your change

`.lefthook.yaml`, by trigger:

| Hook | Glob → command (abridged) |
| --- | --- |
| pre-commit | `*.go` → `make gate-go` (bundles `lint` + `go-test-cached`); `scripts/**/*.go` → `make go-test-scripts-cached`; `*.sql` → `make sql-lint`; `*.md` → `make md-lint`; `.github/workflows/**` → `make actions-lint`, `make pin-actions-check`; `openapi/**` → `make oapi-lint`; `docker/**/Dockerfile`, `docker-compose*.yaml` → `make docker-lint`, `make pin-images-check`; `database/migrations/*.sql` → migration version + gap checks |
| commit-msg | `make commitlint COMMIT_MSG_FILE={1}` |
| pre-push | `make secret-scan`; `*.go` → `make gate-go-push` (bundles `test` + `go-test-scripts`); `*.go` / `openapi/**` → regenerate and `git diff --exit-code` on `*.gen.go` / mocks / `openapi.gen.yaml`; `go.mod` / `go.sum` → `go mod tidy` + diff |

The Go gates are **bundled** into `gate-go` / `gate-go-push` rather than listed one per command,
because lefthook runs a hook's commands in parallel and a per-gate entry multiplies host load by the
number of gates on top of the number of open windows. How hard they run is decided by §19.

The pre-push `gen-go-check` regenerates in Docker and fails on any diff — the fix is to commit the
regenerated output (§2, §4), not to re-run it. When a hook is red for a reason unrelated to your
change (a pre-existing failure on the base branch, an environment problem), push with `--no-verify`
and fix the cause separately rather than reshaping your change around it. Rule out a stale
tool-runner image (§9) before calling it pre-existing — the failure text alone does not tell the two
apart.

## 11. `pin-images-check` / `pin-actions-check` — fail-closed lockfiles

Both checks are fail-closed: every `FROM` in `docker/*/Dockerfile`, every `image:` in
`docker-compose*.yaml`, and every `uses:` in `.github/workflows/**` must be pinned *and* registered in
its lockfile (`docker/images-pin.toml`, `.github/actions-pin.toml`). Adding a new compose service or
action therefore blocks the commit with 未登録 (not in lockfile) or 未固定 (tag-only).

Do not hand-edit the pinned digest or SHA. Use the dedicated skills — `images-pin` for images,
`actions-pin` for actions — which own the supply-chain cooldown (a freshly published digest is
refused, not adopted) and the `resolve` → `apply` → `check` sequence.

## 12. "Migration version gap / duplicate" from pre-commit

`make check-migration-{up,down}-{version,gap}` require the `database/migrations/**` sequence numbers
to be unique and contiguous for both directions. The usual trigger is merging an advanced base branch
that already added your number. Renumber *your* new files (both `.up.sql` and `.down.sql`); never edit
an existing committed migration, per `AGENTS.md`. Scaffold new ones with
`make new-migrate-<name>` so numbering comes from the tool.

## 13. Local S3 calls return 503

The `garage` bucket, layout, and access key are provisioned by a **one-shot `garage_init`** container.
`make infra-up` starts it and waits for it to finish, because compose's `depends_on` cannot express it
across projects; an app started before provisioning completes gets 503 from the S3 endpoint.

```bash
make infra-up                     # idempotent; waits for garage_init to exit
```

The bucket is shared by every checkout (unlike databases, it has no schema to break across branches).
To isolate a branch, point it at a different `OBJECT_STORAGE_BUCKET`. Go tests do not touch garage —
they use in-process gofakes3.

## 14. Per-environment Docker images

The runtime image bakes a single `env/.env.<env>` chosen at build time via `--build-arg APP_ENV=<env>`
(the deploy workflow injects it). There is no single image switched by a runtime ENV.

```bash
go mod vendor   # the builder stage is vendor-mode + GOPROXY=off; skip this and it fails (§18)
docker build --build-arg APP_ENV=stg --target runtime -t <img> -f docker/server/Dockerfile .
# verify only .env.stg was baked in
```

There is no separate migration image: `env/` and `database/migrations` are embedded in the binary, so
migrations run from the same `runtime` image via a command override (`./server migrate-up`).

## 15. `sync-versions` drift

`mise.toml` is the source of truth for tool versions; `go.mod`, the Dockerfiles, and the `docker/**`
READMEs are derived. Editing a derived file directly, or bumping `mise.toml` without propagating,
fails the CI check.

```bash
make sync-versions                # propagate from mise.toml, then commit the result
```

For a Go version bump use the `go-upgrade` skill, which owns the full procedure.

## 16. Generated mocks — never hand-write, regenerate via Docker

Each source declares
`//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE`
(the `$GOFILE`-based destination is uniform across the repo, so the same line copies into any
interface file), and `make gen-go-code` runs them in `go_tool_runner` with the pinned mockgen
(`mise.toml`, currently `0.6.0`). After changing an interface, regenerate rather than editing
`*_mock.go`; new mock directories may come back root-owned (§4).

```bash
make gen-go-code
```

## 17. The `.makefiles` DRY_RUN convention

Setup / teardown targets gate dry-run on `$(if $(DRY_RUN),--dry-run,)` plus `[ -n "$(DRY_RUN)" ]`,
which treat **any non-empty value as truthy** — `DRY_RUN=0 make <target>` is still a dry-run. To
actually run, omit the variable entirely; to preview, `DRY_RUN=1 make <target>`. `setup-repo` rejects
`DRY_RUN` outright because it cannot be previewed.

## 18. `go: inconsistent vendoring` in a fresh worktree — `make serve` reports success but the API never answers

`vendor/` is untracked (`.gitignore`) — deliberately, for the supply-chain reason recorded in
`docs/design/security.md` — so a fresh worktree or clone starts without it. Exactly two build paths
force vendor mode: air's hot-reload build
(`.air.toml`, `go build --mod=vendor`) and the runtime image build (`docker/server/Dockerfile`,
`go build -mod=vendor` under `GOPROXY=off`, so it cannot fall back to fetching). Everything else —
`make go-test`, `make go-lint`, host `go run` — resolves from the module cache and stays green, which is
why nothing warns you until the app itself builds:

```txt
go: inconsistent vendoring in /app:
        github.com/…: is explicitly required in go.mod, but not marked as explicit in vendor/modules.txt
```

Do not read that wall of `go.mod`-vs-`vendor/modules.txt` lines as a merge or dependency-update
accident; the usual cause is simply that `vendor/` does not exist yet. Note that `make serve` still
prints its success line — it runs `docker compose up -d` without `--wait`, so it returns before air
compiles. The failure surfaces as an API that never answers, with the error in the `api_server`
logs, and your editor's LSP reports the same thing across unrelated files.

```bash
go mod vendor       # the whole fix
make serve
```

`make tidy-lib` owns this step, but it runs `go mod tidy` first and can therefore rewrite `go.mod`;
prefer the bare `go mod vendor` when you only need to populate a missing `vendor/`. Nothing guards
this state: no hook or CI check inspects `vendor/` (`tidy-check.yaml` says so explicitly), and the
workflows that build the image regenerate it first, so they never fail on it. Re-run the command
yourself after a base merge that changes dependencies. The same trap hits a manual image build (§14),
which drives the vendor-mode `Dockerfile` directly.

## 19. A gate failed for a reason unrelated to the change — check how many windows are open

When several worktrees each run a gate sized for the whole host, the host saturates and gates start
failing in ways that look like defects in the change: a test you did not touch times out, `make go-lint`
takes 17 minutes, `docker` stops answering (see also the shared-DB and CPU-saturation traps this
manifests as). The loss is not the wall time — it is that **a gate failure stops being evidence about
the code**.

`.makefiles/load.mk` sizes the heavy gates from `git worktree list` at make parse time, so nobody has
to remember to throttle. Ask it what it decided before diagnosing anything else:

```bash
make load-status     # band, window count, CPU share, and the flags each tool will receive
```

| Band | Trigger (default) | What changes |
| --- | --- | --- |
| `full` | fewer than 3 worktrees | Nothing — tool defaults, whole host |
| `low` | 3 or more | `CPU / windows` parallelism, `nice -n 10`, heavy gates run one at a time |
| `ci-first` | 5 or more | Heavy gates do not run locally; the push carries them to CI |

`ci-first` keeps every gate that is cheap **and** unrecoverable after a push — `commitlint`,
`secret-scan`, pin lockfile checks, migration numbering. It drops only what CI re-runs identically,
so nothing goes unverified; the verification moves.

Override per invocation when you need one heavy gate by hand while the rest stays deferred:

```bash
make go-lint GOBP_LOAD=low       # run this one throttled
make go-test GOBP_LOAD=full      # ignore the band entirely (single-window machines)
```

Thresholds are `GOBP_LOW_THRESHOLD` / `GOBP_CI_FIRST_THRESHOLD`. Only gates that run on **every**
commit and push are throttled — one-shot heavy work (image builds, codegen, Trivy) is left alone,
because nobody runs it in a loop.

**Do not reach for `make go-lint` locally to reproduce a CI lint failure when windows are many.** Read the
CI log and apply the single formatter or linter it named (§8 has the config choice); a full local run
costs minutes of saturated host to rediscover what CI already printed.

For a hook already red for an outside reason, §10 covers the `--no-verify` carve-out.

## 20. A cooldown gate refused a version you just declared

`make tool-cooldown-gate` (declarations in `mise.toml` and `python/*.in`) and `make go-cooldown-gate`
(direct modules in `go.mod`) fail closed when a version you added is younger than its window:

```txt
❌ uv@0.12.2（aqua:astral-sh/uv）は公開 13 日で cooldown 14 日を満たしていません
```

These are a different mechanism from §11's pin checks. `pin-actions` / `pin-images` build the
step-back into `resolve` and simply keep the old pin; these two gates have no such fallback, so the
declaration is yours to correct.

**The window is not one number.** It is chosen per backend, from how fast that ecosystem detects and
revokes a bad publish — not from blast radius. `scripts/tool-cooldown/main.go` holds both constants
(`releaseWindowDays` for `aqua:` / `ubi:` / `github:`, `registryWindowDays` for `go:` / `npm:` /
`pypi:` / `pipx:`), `scripts/go-cooldown/main.go` holds `defaultWindowDays`, and the reasoning is
[ADR-0094](../../../docs/adr/0095-malicious-package-detection-via-cooldown.md) plus the Dependencies
section of `docs/design/security.md`. Read the constants rather than trusting a number quoted
anywhere — including here. Language runtimes are outside every window by the same ADR.

**The fix is normally a step back, not a bypass.** Take the newest version that is already older
than the window; `tools-upgrade` and `dep-vuln-upgrade` do this selection for you, and the repo
records the reason in a comment beside the declaration. Reach for `supply-chain-triage` when
stepping back is not an option — the fix exists only in the fresh release, or nothing in the target
major has aged — and let its verdict, not the day count alone, decide.

**Bypass is the exception, and it is designed to nag.** `.github/tool-cooldown-bypass.toml` and
`.github/go-cooldown-bypass.toml` take one entry per key, with all three of `expires` / `issue` /
`reason` required and `expires` capped three months out:

```toml
"uv@0.12.2" = { expires = 2026-08-20, issue = 1272, reason = "…" }
```

`validateBypasses` turns an expired entry, an over-cap `expires`, and an entry naming something no
longer declared into violations of the same gate. That is the point: the expiry arrives whether or
not anyone touches the declaration, and the scheduled run collects it, so a bypass cannot sit
unnoticed the way a plain allowlist would. Set `expires` to the day the window actually opens, not
to a comfortable margin — the entry should start failing as soon as it stops being needed.

**Two traps worth knowing before you diagnose.**

A runtime bump can force a *tool* bump that is itself inside its window. The runtime is exempt from
the gate, so nothing in the declarations shows the coupling: raising `python` past what the pinned
`uv` can download makes `uv venv --python <version>` fail on the host leg of `sql-lint`, and the
only fix is a newer `uv` — which the gate then blocks. Expect the chain to surface in CI rather
than locally, because the containerized path uses the image's interpreter and never exercises it.

Raising a `python/*.in` pin without regenerating its `.txt` fails too, from `verifyLocks` rather
than from the window. Run `make py-lock` and commit both files; the pin and its lockfile are one
change.

## 21. Conflict markers after taking the base in

Two things about a merge here are not obvious, and both are why `resolve-merge` owns the steps.

**Most markers land in files nobody should be editing.** `*.gen.go`, `*.sql.go`, `*_mock.go`,
`openapi.gen.yaml`, `database/gen/*.gen.sql`, the generated `docs/` trees, and the pin lockfiles all
have a generator or a resolver, so none of them has a correct side. Choosing one produces a file with
no markers that reviews clean and no longer reproduces from its source — and `gen-*-artifacts-check`
then fails on somebody else's pull request, in code they never touched.

**A clean merge is not a finished merge.** `database/gen/*.gen.sql` is a concatenation of the DML
files, so two branches can each add one, conflict on nothing, and leave the concatenation stale. The
same holds for every derived artifact.

Run `resolve-merge` either way — it classifies each path, rebuilds the generated classes from source,
re-resolves the lockfiles through their resolvers, and hands back only what needs a human. Take the
base from the pull request's `baseRefName`, and merge rather than rebase (`CLAUDE.md`); during a
hotfix the base is a human decision, so pass it explicitly.

<!-- sample-api:begin -->
## 99. `sample-removal-check` fails in CI

`scripts/setup/remove-sample-api/sample-manifest.ts` declares every path that `make setup-remove-sample-api` <!-- skill-lint-ignore -->
deletes when a template user strips the sample APIs. Adding, moving, or renaming files under a sample
domain (user / product / purchase / …) without registering them leaves dangling references after
removal, which the CI job catches by actually performing the removal and then building, linting, and
testing. Nothing local fails — this is CI-only unless you run it yourself.

When you add sample-domain files (handler, usecase, domain, repository, DML, migration, seed, spec,
integration test, sample-only generated output), add their paths to the matching domain entry. Lines
mixed into shared files are handled with `sample-api` marker comments instead of paths. Preview the
effect without deleting anything:

```bash
DRY_RUN=1 make setup-remove-sample-api
```

<!-- sample-api:end -->
## Constraints

- ✅ Read-only knowledge: surface the exact command; run it only when the user asked you to perform
  the operation.
- ✅ Warn before anything destructive: DB rebuilds (§5), `chown -R` (§4), `make infra-down` and other
  shared-infra operations that affect every checkout (§1).
- ✅ Prefer `make` over raw `docker compose`; when raw compose is unavoidable, set
  `COMPOSE_PROJECT_NAME=gobp-shared` / `-p gobp-shared` explicitly (§1).
- ✅ Prefer the dockerized `--user root … chown` recovery over host `sudo`.
- ❌ Do not hand-edit generated artifacts (`*.gen.go`, `*.sql.go`, `*_mock.go`, `openapi.gen.yaml`,
  `schema.gen.sql`, `docs/portal/guides/**`, pinned digests/SHAs) — regenerate or use the owning skill.
- ❌ Do not commit a schema- or DML-affecting change without its regenerated artifacts (§2), and do
  not commit a materialized `env/.env` (§7).
