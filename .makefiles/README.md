# Make Command List

## Role

`.makefiles/` is the central registry for every `make` target used by the project. Each `.mk` file groups related targets by area (application / database / sql / go / openapi / docs / github / tools). The top-level `makefile` simply `include`s them, so adding a new target means dropping it into the right group file — no top-level edits required.

Make targets are mainly organized into the following units.

- `.makefiles/app` : Application startup / Long-running process (worker / outbox-relay) / Job execution / Embedded env materialization
- `.makefiles/database` : DB initialization / Migration / Seed / DML / Schema
- `.makefiles/sql` : SQL Lint / Fix
- `.makefiles/markdown` : Markdown Lint / Fix
- `.makefiles/security` : Trivy dependency vulnerability scan
- `.makefiles/docker` : Compose project / host port definitions / Dockerfile lint (hadolint) / image digest pinning
- `.makefiles/openapi` : OpenAPI bundle / API documentation generation
- `.makefiles/go` : Go code generation / Format / Lint / Test / Tool management
- `.makefiles/python` : PyPI tool lockfile generation
- `.makefiles/graphify` : Knowledge-graph export / tracked-artifact portability check
- `.makefiles/docs` : Portal / Tool information documentation generation
- `.makefiles/gen` : Batch execution of various generation processes
- `.makefiles/github` : GitHub initialization / Release / Labels / Rule configuration

## Conventions

- Target names use dash-separated lower case (`make new-migrate-<name>`, `make gen-api`).
- Targets are split into two flavors:
  - **Normal targets**: invoked by developers locally; run inside Docker containers for reproducibility. A few resolve their tool on the host instead — `lint` / `fix` (`golangci-lint`), `actions-zizmor` (`zizmor`), `go-cooldown-gate` / `go-cooldown-audit`, `tool-cooldown-gate` / `tool-cooldown-audit`, `pnpm-cooldown-check` — because the tool-runners are Alpine and upstream publishes no musl build. That is the documented last resort in [Toolchain Execution Rules](../docs/rules.md#toolchain-execution-rules), not an exception to the convention: `make install-tools` provisions those tools, and the `mise.toml` pin carries the reproducibility the image otherwise would.
  - **`-ci` targets**: low-level commands intended to run on bare metal (CI runners, or developers who already have the tool installed).
- Every target should be `.PHONY` and self-documenting via a trailing `##` comment so `make help` can pick it up.

## Notes

- Adding a new target to an existing group file needs no top-level edit. Adding a new `.mk` file, however, requires appending its `include` line to the top-level `makefile` — files are included individually, not by wildcard.
- Prefer `make new-migrate-<name>` (and similar helpers) over manual file creation; the helpers enforce naming conventions and number sequences.
- For one-off operational commands (`make setup-repo`, etc.) keep them under `.makefiles/github/operation/` so they stay separate from developer-facing targets.

## `.makefiles/app` group

This is a group of targets related to application development environment startup and Job execution.

Compose services are split into two layers (see `.makefiles/docker` group below): the shared **infra**
layer (`database` / `observability` / `garage` / `elasticmq` / `dynamodb_local` / `goaws`) lives once in the fixed `gobp-shared` project, and the
per-checkout **app** layer (`api_server` / `mock_auth_server`) runs in this checkout's `APP_PROJECT`.

### Application startup related

| Command | Description | Main Use |
| --- | --- | --- |
| `make serve` | Brings up the shared infra (`infra-up`), then starts this checkout's app services through `app-up` and refreshes the DB slot heartbeat. | Start normal local development |
| `make serve-build` | Rebuilds the app images (cache enabled), brings up the shared infra, then starts the app services through `app-up`. | Reflect Dockerfile or dependency changes |
| `make serve-build-clean` | Cleanly rebuilds the app images with `--no-cache --pull`, brings up the shared infra, then starts the app services through `app-up`. | Pick up base image updates (e.g., Go version upgrade) |
| `make app-up` | Starts this checkout's app services and waits for the `api_server` healthcheck (`up --wait`), so the caller's success message means the API can answer rather than that a container exists. On failure prints the last `APP_LOG_TAIL` (default `50`) lines of the `api_server` log and exits non-zero. | Internal — called by the three `serve` targets. Useful on its own to restart just the app after a startup failure, skipping the infra and provisioning steps |
| `make serve-stop` | Stops this checkout's app project only. | Stop the API without touching the shared infra or other checkouts |
| `make infra-up` | Starts the shared infra services (`--wait`) plus the one-shot `garage_init` in the `gobp-shared` project. | Bring up the shared infra alone (called idempotently by `serve` / `job` / `worker`). In a worktree it also passes `INFRA_NO_RECREATE`, keeping a running container another checkout may be using — a definition change then takes `infra-down` followed by `infra-up` |
| `make infra-down` | Stops the shared infra project (named volumes are kept). | Shut the infra down — **affects every checkout / worktree** |
| `make tools` | Starts development support tools with the `tools` profile in the shared infra project. | When using development tools (SQL editor `:2000` / docs viewer `:2001`). Also passes `INFRA_NO_RECREATE` — the profile covers `database` / `garage` too |
| `make all` | Starts everything: `tools` followed by `serve-build`. | Bring up the whole local stack at once |
| `make tool-runners-build` | Builds the on-demand tool runner images (go/node/python, cache enabled, no startup). | When updating tool runner Dockerfile or dependencies |
| `make tool-runners-build-clean` | Cleanly builds the tool runner images with `--no-cache --pull` (no startup). | Pick up base image updates for tool runners |

#### `make job NAME=<job_name> ARGS="<arguments>"`

Executes an application Job.
Brings up the shared infra, then runs `cmd/main.go job` in a one-off `api_server` container
(`run --rm`) of this checkout's app project.

- `NAME`: Job name to execute
- `ARGS`: Additional arguments passed to the Job (optional)

Example:

```sh
make job NAME=sample-job
make job NAME=batch-import ARGS="--target=local --dry-run"
```

### Long-running process (worker / outbox-relay) related

Both are long-running daemons that reside until `SIGTERM` / `Ctrl-C`. They run in a one-off
`api_server` container of this checkout's app project after the shared infra is brought up
(same mechanism as `make job`, via `go run ./cmd/`).

#### `make worker NAME=<worker_name> ARGS="<arguments>"`

Starts a pull-ack worker. `NAME` is the worker name (required); `ARGS` is optional.

> No worker is registered by default (`WorkerModule()` is an empty seam), so
> this fails with `unknown worker` until you wire a real worker. It is kept as the
> entry point for local verification once a worker is added.

```sh
make worker NAME=sampleworker
```

#### `make outbox-relay ARGS="<arguments>"`

Starts the outbox relay (periodically polls the outbox table and publishes pending
messages). `ARGS` is required — the relay serves exactly one delivery channel and has no
default one — and also reaches the `replay` subcommand.

```sh
make outbox-relay ARGS="--channel=http"
make outbox-relay ARGS="replay --message-id=<id>"
```

### Embedded env materialization related

The server binary embeds `env/.env`. CI and the Docker build materialize the
per-environment file into `env/.env` before building, so these targets centralize
that step (and its undo for drift checks).

| Command | Description | Main Use |
| --- | --- | --- |
| `make materialize-env` | Copies `env/.env.$(APP_ENV)` over `env/.env` (defaults to `APP_ENV=ci`). | Materialize the embed target in CI / build before `go build` / `go run` |
| `make restore-env` | Restores `env/.env` to its git-tracked content via `git restore`. | Undo materialization before a generated-artifact drift / commit check |

### Realtime Delivery smoke related

| Command | Description | Main Use |
| --- | --- | --- |
| `make realtime-init` | Brings up the shared infra, then creates the Realtime Delivery tables (EventLog / StreamTicket / InstanceLease) in DynamoDB Local and the fan-out topic on GoAWS from inside the app container (`go run ./cmd/ realtime-init`). Idempotent — re-running converges on the same state. | Provisioning without starting the app. `make serve` runs the same one-shot itself (`realtime-provision`), so the ordinary path needs no separate call |
| `make realtime-reset` | Brings up `dynamodb_local`, then runs `scripts/realtime-reset` from the host to delete this checkout's three tables and wait until each is gone. Creation stays with `realtime-provision`. Requires a database owner, and refuses a host-less `-endpoint` or an AWS host — the dummy signing credential is the second control, so the deny list is not the only guard. | Run by every path that rebuilds a local database — `slot-acquire`, `db-local-reinit`, `db-init-local` — so the sequence allocator and its EventLog are reset together; see [db-worktree-pool.md](../docs/maintenance/db-worktree-pool.md). Also useful by hand when a local stream is wedged behind a stale sequence |
| `make realtime-smoke` | Brings up the shared infra, then runs `scripts/realtime-smoke` against DynamoDB Local and GoAWS with the AWS SDK Go v2 and prints one verdict per call (互換 / 非互換 / 未対応 / 検証不能). Resources are created under a per-run random name and deleted afterwards. `ARGS` passes flags through (`-format markdown` / `-subscribers N` / `-keep` / `-strict`). | Confirm the emulators still accept the calls Realtime Delivery makes — e.g. after bumping either image |

## `.makefiles/database` group

This is a group of targets that handle all DB operations.
Provides migration, seed insertion, DML merge, schema generation, DB initialization, etc.

### DB initialization related

| Command | Description | Notes |
| --- | --- | --- |
| `make db-init` | Executes initialization for both the owned local and test databases. | Calls `db-init-local` and `db-init-test` sequentially. |
| `make db-init-local` | Initializes the owned local database. | Executes `db-local-migrate-down` → `db-local-migrate-up` → `db-local-seed`. |
| `make db-init-test` | Initializes the owned test database. | Executes `db-test-migrate-down` → `db-test-migrate-up` → `db-test-seed`. |
| `make require-db-owner` | Verifies that this checkout owns a database. | Prerequisite of every target that resolves a database name. Fails in a linked worktree that holds no DB slot, instead of falling back to the main checkout's `local` / `test` — see `docs/maintenance/db-worktree-pool.md`. The decision lives in `internal/cli/dbslot`: a missing `git` executable and a directory that is no repository both pass through, while a repository whose layout `git` will not report fails. |

### DB migration related

| Command | Description | Notes |
| --- | --- | --- |
| `make new-migrate-<name>` | Generates a new migration file. | Creates numbered `.up.sql` / `.down.sql` under `database/migrations`. |
| `make check-migration-up-version` | Checks duplicate versions in `up` migrations. | Runs `scripts/migration-lint`; the numbering rules live there with tests, not in a shell snippet. |
| `make check-migration-down-version` | Checks duplicate versions in `down` migrations. | Runs `scripts/migration-lint`. |
| `make check-migration-up-gap` | Checks sequence gaps in `up` migrations. | Runs `scripts/migration-lint`. Passes when there are no migrations at all, so an empty migration set does not trip the gate. |
| `make check-migration-down-gap` | Checks sequence gaps in `down` migrations. | Runs `scripts/migration-lint`. |
| `make db-migrate-up DB=<database>` | Applies all migrations to the specified DB up to the latest. | Example: `make db-migrate-up DB=local` |
| `make db-migrate-up-<steps> DB=<database>` | Applies the given number of migrations relative to the current position. | Example: `make db-migrate-up-2 DB=local` |
| `make db-migrate-down DB=<database>` | Downgrades all migrations to the initial state. | None |
| `make db-migrate-down-<steps> DB=<database>` | Rolls back the given number of migrations. | None |
| `make db-local-migrate-up` | Applies all migrations to the owned local database. | Alias for `db-migrate-up` with `DB=$(DB_LOCAL)` (`local`, or `wt<N>_local` while a slot is held). |
| `make db-local-migrate-up-<steps>` | Applies the given number of migrations on LocalDB. | None |
| `make db-local-migrate-down` | Downgrades the owned local database to initial state. | Alias for `db-migrate-down` with `DB=$(DB_LOCAL)`. |
| `make db-local-migrate-down-<steps>` | Rolls back the given number of migrations on LocalDB. | None |
| `make db-test-migrate-up` | Applies all migrations to the owned test database. | Alias for `db-migrate-up` with `DB=$(DB_TEST)` (`test`, or `wt<N>_test` while a slot is held). |
| `make db-test-migrate-up-<steps>` | Applies the given number of migrations on TestDB. | None |
| `make db-test-migrate-down` | Downgrades the owned test database to initial state. | Alias for `db-migrate-down` with `DB=$(DB_TEST)`. |
| `make db-test-migrate-down-<steps>` | Rolls back the given number of migrations on TestDB. | None |
| `make db-migrate-ci-up DB=<database>` | Executes `cmd/main.go migrate-up` directly without Docker. | CI target |
| `make db-migrate-ci-up-<steps> DB=<database>` | Executes `migrate-up` for the given number of steps without Docker. | CI target |
| `make db-migrate-ci-down DB=<database>` | Executes `cmd/main.go migrate-down` directly without Docker. | CI target |
| `make db-migrate-ci-down-<steps> DB=<database>` | Executes `migrate-down` for the given number of steps without Docker. | CI target |

Example:

```sh
make new-migrate-create_users_table
make db-migrate-up DB=local
make db-migrate-up-10 DB=local
```

### DB seed related

| Command | Description | Notes |
| --- | --- | --- |
| `make db-seed DB=<database>` | Inserts seed data into the specified DB. | Executes `cmd/main.go db-seed` inside a Docker container. |
| `make db-seed-ci DB=<database>` | Executes seed insertion directly without Docker. | CI target |
| `make db-local-seed` | Inserts seed data into the owned local database. | Alias for `db-seed` with `DB=$(DB_LOCAL)`. |
| `make db-test-seed` | Inserts seed data into the owned test database. | Alias for `db-seed` with `DB=$(DB_TEST)`. |

### DB generation / helper related

| Command | Description | Notes |
| --- | --- | --- |
| `make gen-db-schema` | Generates DB schema documentation. | Used to update ER diagrams and schema outputs. |
| `make gen-db-schema-ci` | Executes SchemaSpy container directly to generate schema docs. | CI target |
| `make dump-schema` | Executes schema dump. | Used as preprocessing for SQLC generation and DML merge. Rebuilds the owner's throwaway database (`gen_schema`, or `gen_schema_wt<N>` while a slot is held) from this branch's migrations and dumps that. |
| `make dump-schema-ci` | Executes `cmd/main.go dump-schema` directly without Docker. | CI target |
| `make fix-collation` | Fixes database collation. | None |
| `make fix-collation-ci` | Executes collation fix directly without Docker. | CI target |

### DML merge related

| Command | Description | Notes |
| --- | --- | --- |
| `make merge-dml` | Executes all DML merge processes. | Executes `merge-dml-repo` → `merge-dml-qs` → `merge-dml-sysq` → `merge-dml-cs`. |
| `make merge-dml-repo` | Merges DML for Repository. | None |
| `make merge-dml-qs` | Merges DML for Query Service. | None |
| `make merge-dml-cs` | Merges DML for Command Service. | None |
| `make merge-dml-sysq` | Merges DML for System Query. | None |
| `make merge-dml-core type="<type>" work-dir="<dir>"` | Executes DML merge for specified type. | Calls `make merge-dml-ci-core` via Docker container. |
| `make merge-dml-ci` | Executes all DML merge processes directly. | CI target |
| `make merge-dml-ci-repo` | Merges DML for Repository. | CI target |
| `make merge-dml-ci-qs` | Merges DML for Query Service. | CI target |
| `make merge-dml-ci-cs` | Merges DML for Command Service. | CI target |
| `make merge-dml-ci-sysq` | Merges DML for System Query. | CI target |
| `make merge-dml-ci-core type="<type>" work-dir="<dir>"` | Executes `cmd/main.go merge-dml` directly. | CI target |

Example:

```sh
make merge-dml-core type="repository" work-dir="/app"
```

## `.makefiles/sql` group

This group handles static analysis and auto-fix for SQL files.
Targets include Migration / DML / Seed SQL.

### SQL Lint related

| Command | Description | Notes |
| --- | --- | --- |
| `make sql-lint` | Executes SQL lint in batch. | Executes `sql-lint-migrations` → `sql-lint-dml` → `sql-lint-seed`. |
| `make sql-lint-migrations` | Lints migration SQL. | None |
| `make sql-lint-dml` | Lints DML SQL. | None |
| `make sql-lint-seed` | Lints seed SQL. | None |
| `make sql-lint-migrations-ci` | Executes `sqlfluff lint` on `database/migrations/`. | CI target |
| `make sql-lint-dml-ci` | Executes `sqlfluff lint` on `database/dml/`. | CI target |
| `make sql-lint-seed-ci` | Executes `sqlfluff lint` on `database/seed/`. | CI target |

### SQL Fix related

| Command | Description | Notes |
| --- | --- | --- |
| `make sql-fix` | Executes SQL auto-fix in batch. | Executes `sql-fix-migrations` → `sql-fix-dml` → `sql-fix-seed`. |
| `make sql-fix-migrations` | Auto-fixes migration SQL. | None |
| `make sql-fix-dml` | Auto-fixes DML SQL. | None |
| `make sql-fix-seed` | Auto-fixes seed SQL. | None |
| `make sql-fix-migrations-ci` | Executes `sqlfluff fix` on `database/migrations/`. | CI target |
| `make sql-fix-dml-ci` | Executes `sqlfluff fix` on `database/dml/`. | CI target |
| `make sql-fix-seed-ci` | Executes `sqlfluff fix` on `database/seed/`. | CI target |

## `.makefiles/markdown` group

This group handles linting and auto-fixing of Markdown files.

| Command | Description | Notes |
| --- | --- | --- |
| `make md-lint` | Lints Markdown (markdownlint + mermaid syntax + skill-definition lint). | Invokes `make md-lint-ci` inside the `node_tool_runner` container. |
| `make md-fix` | Auto-fixes Markdown files. | Invokes `make md-fix-ci` inside the `node_tool_runner` container. |
| `make md-mermaid-lint` | Validates only the ` ```mermaid ` fences. | Invokes `make md-mermaid-lint-ci` inside the `node_tool_runner` container. |
| `make md-skill-lint` | Validates only the skill / agent definitions under `.claude/**` and their `.codex/**` counterparts. | Invokes `make md-skill-lint-ci` inside the `node_tool_runner` container. |
| `make md-premise-lint` | Checks only that no document which outlives template instantiation rests on a premise that lapses with it. | Invokes `make md-premise-lint-ci` inside the `node_tool_runner` container.  <!-- boilerplate-only:line --> |
| `make md-lint-ci` | Runs `markdownlint-cli2`, then the mermaid syntax lint, then the skill-definition lint. | CI target. Excludes `vendor/`, `node_modules/`, `.git/`. |
| `make md-mermaid-lint-ci` | Validates ` ```mermaid ` fences with `scripts/mermaid-lint/index.ts` (real `mermaid.parse`). | CI target. markdownlint never checks diagram grammar. |
| `make md-skill-lint-ci` | Checks `.claude/**` definitions with `scripts/skill-lint/index.ts` (frontmatter / translation-pair structure / reference existence) and their `.codex/**` correspondence (skill / agent existence parity, Codex skill structure). | CI target. markdownlint never checks whether the prose matches reality, and nothing else notices a skill that landed on only one of the two environments. |
| `make md-premise-lint-ci` | Mechanises the *No premise the document will outlive* rule from [docs/rules.md](../docs/rules.md) with `scripts/premise-lint/index.ts`. Fails when a document that survives template instantiation carries a self-reference that stops being true there; the phrases it looks for are declared in `scripts/premise-lint/rules.ts`. | CI target. A premise may be stated in `README*` / `docs/get-started/**`, which the setup rewrites or deletes, or inside a `boilerplate-only` / `sample-api` marker. Other senses of the same words are declared with a reason in `scripts/premise-lint/allowances.ts`.  <!-- boilerplate-only:line --> |
| `make md-fix-ci` | Fixes `**/*.md` directly with `markdownlint-cli2 --fix`. | CI target. Excludes `vendor/`, `node_modules/`, `.git/`. |

## `.makefiles/security` group

This group runs local security scans (Trivy dependency / secret scan, gitleaks secret scan, zizmor Actions audit), mainly to reproduce a CI security finding on the developer's machine. Image scanning is CI-only (`image-scan.yaml`).

| Command | Description | Notes |
| --- | --- | --- |
| `make trivy-fs` | Scans library dependencies with Trivy fs. | Invokes `make trivy-fs-ci` inside the `go_tool_runner` container. |
| `make trivy-fs-ci` | Runs `trivy fs` directly. | CI target. Skips `vendor/` to match CI. |
| `make trivy-fs-release-ci` | Runs `trivy fs` including unfixed vulnerabilities. | CI target for the promotion gate; differs from `trivy-fs-ci` only by dropping `--ignore-unfixed`. |
| `make trivy-config` | Scans the Dockerfiles for misconfiguration. | Invokes `make trivy-config-ci` inside the `go_tool_runner` container. |
| `make trivy-config-ci` | Runs `trivy config` directly. | CI target. Gates at `CRITICAL,HIGH`; accepted exceptions live in `.trivyignore.yaml`. |
| `make trivy-license` | Lists dependency licences. | Invokes `make trivy-license-ci` inside the `go_tool_runner` container. |
| `make trivy-license-ci` | Runs `trivy fs --scanners license` directly. | CI target. Report-only; no severity threshold until a prohibited-licence policy exists. |
| `make trivy-secret` | Scans the working tree for secrets with Trivy. | Invokes `make trivy-secret-ci` inside the `go_tool_runner` container. |
| `make trivy-secret-ci` | Runs `trivy fs --scanners secret` directly. | CI target. The vulnerability targets pass `--scanners vuln` explicitly, so the secret scanner is a check of its own rather than a second view of theirs. No severity threshold; accepted detections are pinned per path in `.trivyignore.yaml`. Deliberately overlaps gitleaks: Trivy's curated rules false-positive less, gitleaks' regex / entropy net catches more. |
| `make trivy-image-ci` | Scans a built image for vulnerabilities. | CI target. Pass the image with `TRIVY_IMAGE=`. |
| `make trivy-image-gate-ci` | Fails on fixable `CRITICAL` / `HIGH` in a built image. | CI target. Pass the image with `TRIVY_IMAGE=`. |
| `make trivy-sbom-ci` | Matches a pre-generated SBOM against the vulnerability database. | CI target. Pass the file with `TRIVY_SBOM_FILE=`. |
| `make secret-scan` | Scans the working tree for secrets with gitleaks. | Invokes `make secret-scan-ci` inside the `go_tool_runner` container. |
| `make secret-scan-ci` | Runs `gitleaks dir . --redact` directly. | CI target. Generated files are allowlisted in `.gitleaks.toml`. |
| `make secret-scan-history-ci` | Runs `gitleaks git . --redact` directly. | CI target, used by the weekly run. `dir` only sees the working tree, so it misses a secret that was committed and later deleted; `git` walks the whole history. |
| `make go-cooldown-gate BASE=<ref>` | Fails when the `go.mod` diff against `BASE` adds or upgrades a **direct** module published inside the cooldown window. | Runs on the host. `BASE` has no default on purpose: a stale one would silently narrow the diff until the gate inspects nothing. Go has no resolver-side cooldown, so this check is the guard rather than a detector for one. |
| `make go-cooldown-audit` | Reports every module in `go.mod` published inside the window, and fails on a bypass entry that has expired, reaches beyond three months, or matches nothing. | Runs on the host. The window itself never fails here — existing dependencies are grandfathered — but a lapsed bypass does, since its deadline arrives without `go.mod` changing. |
| `make tool-cooldown-gate BASE=<ref>` | Fails when the declaration diff against `BASE` — `mise.toml` and `python/*.in` — pins a tool version published inside its backend's window (14 days for a GitHub release, 7 for a package registry). Also fails when a `python/*.in` declaration and its `python/*.txt` lockfile name different versions. | Runs on the host; needs `mise` on PATH to resolve a short name's backend, and a `GITHUB_TOKEN` because the unauthenticated API cannot carry one run. Language runtimes are excluded as an accepted risk. |
| `make tool-cooldown-audit` | Reports every declared tool published inside its window, and fails on a bypass entry that has expired, reaches beyond three months, or matches nothing. | Runs on the host. Same grandfathering and same lapsed-bypass failure as the Go counterpart. |
| `make pnpm-cooldown-check` | Fails on a `minimumReleaseAgeExclude` entry with no deadline in `.github/pnpm-cooldown-bypass.toml`, a deadline that has expired or reaches beyond three months, a bypass entry matching no exclusion, or an exclusion naming a version the sibling `pnpm-lock.yaml` no longer resolves. | Runs on the host. There is no `gate` counterpart because pnpm's resolver already enforces the window on every install; what this checks is the exemption, whose deadline arrives without either file changing. |
| `make actions-zizmor` | Audits the workflow / composite-action definitions with zizmor and fails on a `high` finding. | Runs on the host. `--offline`, so the pre-commit hook needs no network and no `GH_TOKEN`; the online audits are left to CI. Exceptions live in `.github/zizmor.yml`. |
| `make actions-zizmor-sarif-ci` | Writes every zizmor finding to stdout as SARIF. | CI target. Not filtered by severity, so code scanning keeps the full picture; call it with `make -s`. |
| `make actions-zizmor-gate-ci` | Fails on a `high` zizmor finding. | CI target. Same gate as `actions-zizmor` but with the online audits, which need `GH_TOKEN`. |

## `.makefiles/docker` group

This group holds the compose project / host port definitions shared by every target, lints
Dockerfiles with hadolint via the `go_tool_runner` container, and pins the `FROM` base images to
an immutable digest (supply-chain hardening).

### Compose project definitions (`compose.mk`)

`compose.mk` declares no target — it defines the variables the app / database groups build on, so it
is `include`d at the top of the top-level `makefile` (the "depended-on files" section). Defaults are
overridden by `.gobp-db-slot` when a DB slot is held (see `internal/cli/dbslot/README.md`).

| Variable | Default | Description |
| --- | --- | --- |
| `INFRA_PROJECT` | `gobp-shared` | Fixed compose project holding the single shared infra instance. |
| `APP_PROJECT` | `gobp-app-$(notdir $(CURDIR))` | Per-checkout compose project for the app layer. Becomes `SERVE_PROJECT` (`gobp-wt-N`) when a DB slot is held. |
| `INFRA_SERVICES` | `database observability garage elasticmq dynamodb_local goaws` | Services that can only run on fixed ports, hence shared. |
| `APP_SERVICES` | `api_server mock_auth_server` | Services started per checkout. |
| `COMPOSE_INFRA` | `docker compose -p $(INFRA_PROJECT)` | Compose invocation for the infra layer. |
| `INFRA_NO_RECREATE` | `--no-recreate` in a worktree, empty otherwise | Keeps a shared-infra container another checkout is using instead of re-creating it. Empty in a single checkout, where compose re-converges on a definition change as usual. Set it explicitly for a topology the worktree test misses, such as several independent clones. Resolved inside the recipe by `db-slot env`, not at make's parse time. |
| `COMPOSE_APP` | `docker compose -p $(APP_PROJECT) -f docker-compose.yaml -f docker-compose.attach.yaml --profile development` | Compose invocation for the app layer. `docker-compose.attach.yaml` points the app services at the shared infra via `host.docker.internal`. |
| `APP_LOG_TAIL` | `50` | Lines of the `api_server` log `app-up` prints when the API fails to come up. Raise it when the failure is older than the tail. |
| `API_HOST_PORT` / `MOCK_AUTH_HOST_PORT` | `8080` / `2010` | Published host ports of the API / mock auth server. |
| `DLV_HOST_PORT` / `PPROF_HOST_PORT` | `2345` / `6060` | Published host ports of the dlv debug / pprof endpoints. |
| `COMPOSE_PROJECT_NAME` | `$(INFRA_PROJECT)` | Default project for compose calls that don't pass `-p`, so DB tooling shares the infra network. |

### Dockerfile lint / image pin related

| Command | Description | Notes |
| --- | --- | --- |
| `make docker-lint` | Lints `docker/*/Dockerfile` with hadolint. | Invokes `make docker-lint-ci` inside the `go_tool_runner` container. |
| `make docker-lint-ci` | Runs `hadolint docker/*/Dockerfile` directly. | CI target. Ignored rules are in `.hadolint.yaml`. |
| `make compose-lint` | Checks the compose service declarations against the rules in [`scripts/compose-lint`](../scripts/README.md) — currently that every `APP_SERVICES` member declares a `healthcheck`. | Runs on the host (`go run`), like `migration-lint`, because the decision is our own tool rather than an installed linter. Also runs in CI (`compose-lint.yaml`). |
| `make pin-images-resolve` | Resolves every registry reference — `FROM`, `docker-compose*.yaml` `image:`, and a workflow's `uses: docker://` and `services.*.image` — to its current digest and updates the `docker/images-pin.toml` lockfile. | Quarantines digests younger than `PIN_IMAGES_MIN_AGE_DAYS` (default 14; 0 disables). Needs registry access (`docker`). |
| `make pin-images-apply` | Pins those same four reference forms to `image:tag@sha256:...` from the lockfile (quarantined images stay tag-only). | None |
| `make pin-images-check` | Verifies those same four reference forms are pinned per the lockfile (no write). An `image:` built from a `${{ }}` expression is not a fixable reference and is skipped. | CI / pre-commit gate. |

## `.makefiles/openapi` group

| Command | Description | Notes |
| --- | --- | --- |
| `make gen-bundle-oapi` | Bundles split OpenAPI definitions into a single file. | Generates `openapi/openapi.gen.yaml` from `openapi/openapi.yaml`. |
| `make gen-api-docs` | Generates API documentation from OpenAPI definition. | None |
| `make lint-oapi` | Validates the OpenAPI definition with `redocly lint`. | Invokes `make lint-oapi-ci` inside the `node_tool_runner` container. |
| `make gen-bundle-oapi-ci` | Generates `openapi/openapi.gen.yaml` via `redocly bundle`. | CI target |
| `make gen-api-docs-ci` | Generates `docs/openapi/index.html` via `redocly build-docs`. | CI target |
| `make lint-oapi-ci` | Runs `redocly lint openapi/openapi.yaml` directly. | CI target |
| `make stamp-openapi-version` | Rewrites `info.version` from a release branch name. | Invokes `make stamp-openapi-version-ci` inside the `node_tool_runner` container. Takes `REF=release/vX.Y.Z`, falling back to `GITHUB_REF_NAME`; any other ref is a no-op. |
| `make stamp-openapi-version-ci` | Runs `scripts/stamp-openapi-version/index.ts` directly. | CI target |
| `make lint-oapi-security-ci` | Runs Spectral with the OWASP API Security ruleset. | CI target. Runs outside `node_tool_runner` so a spec-only check does not build the tool image; run `pnpm install --dir scripts --frozen-lockfile` first. |
| `make openapi-client-check` | Confirms the frontend generator (orval) can generate the SSE contract types (`DeliveryEvent` / `ControlEvent` / `StreamCursor`) from the bundled spec. | Invokes `make openapi-client-check-ci` inside the `node_tool_runner` container. Output goes to `tmp/openapi-client/` and is never committed. |
| `make openapi-client-check-ci` | Runs `tsx scripts/openapi-client-check` directly. | CI target. Same preparation as `lint-oapi-security-ci`. |

## `.makefiles/load` group

Host CPU is finite, but the number of checkouts working against it is not. When several worktrees each
run a gate sized for the whole host, the machine saturates and the gates start failing for reasons that
have nothing to do with the change under test — an untouched test times out, `golangci-lint` takes
17 minutes, `docker` stops answering. The cost is not the wall time; it is that a gate failure stops
being evidence about the code.

`.makefiles/load.mk` sizes the heavy gates from the number of open windows (`git worktree list`), so the
throttling happens without anyone remembering to ask for it. Three bands:

| Band | Trigger (default) | Behaviour |
| --- | --- | --- |
| `full` | fewer than 3 worktrees | Unchanged from before — tools use their own defaults and the whole host |
| `low` | 3 or more | Heavy gates get `CPU / windows` of parallelism, run at `nice -n 10`, and run one at a time |
| `ci-first` | 5 or more | Heavy gates are not run locally at all; the push carries them to CI |

`ci-first` keeps every gate that is cheap **and** cannot be recovered after a push — `commitlint`,
`secret-scan`, the pin lockfile checks, migration numbering. What it drops is only what CI re-runs
identically, so nothing goes unverified; it moves where the verification happens.

| Command | Description | Notes |
| --- | --- | --- |
| `make load-status` | Prints the resolved band, window count, CPU share and the flags each tool will receive. | Start here when a gate is behaving unexpectedly |
| `make gate-go` | The `pre-commit` Go gate (`lint` + `test-cached`), bundled so the band decides parallel / serial / deferred. | Called by lefthook, not usually by hand |
| `make gate-go-push` | The `pre-push` Go gate (`test` + `test-scripts`), same bundling. | Called by lefthook |
| `make gate-heavy-skip` | Predicate for lefthook's `skip:` — exit 0 means "CI will do this". | Exit status is the whole interface |

Override the band explicitly with `GOBP_LOAD=full|low|ci-first` (e.g. `make lint GOBP_LOAD=low` to run
one heavy gate by hand while the rest stays deferred). The thresholds are `GOBP_LOW_THRESHOLD` and
`GOBP_CI_FIRST_THRESHOLD`. Their defaults, and the band resolution itself, live in
`scripts/load-band` and are evaluated when a gate recipe runs rather than when make parses.

Why the gates are bundled rather than listed individually in `.lefthook.yaml`: lefthook runs a hook's
commands in parallel, so a per-gate entry multiplies load by the number of gates *on top of* the number
of windows. Bundling puts the parallel-vs-serial decision in one place that already knows the band.

Only the gates that run on **every** commit and push are throttled. One-shot heavy work (image builds,
code generation, Trivy scans) is left alone — it is not what saturates a host, because nobody runs it
in a loop.

## `.makefiles/go` group

### Go generation related

| Command | Description | Notes |
| --- | --- | --- |
| `make gen-go-code` | Executes Go code generation. | Runs `go generate ./...` inside a Docker container. |
| `make gen-go-code-ci` | Executes `go generate ./...` directly without Docker. | CI target |
| `make gen-sqlc` | Executes SQLC code generation in batch. | Executes `remove-generated-sqlc` → `sqlc-generate`. |
| `make remove-generated-sqlc` | Deletes existing SQLC generated code. | None |
| `make sqlc-generate` | Executes SQLC code generation. | None |
| `make remove-generated-sqlc-ci` | Deletes `*.gen.sql.go` under `$(SQLC_OUT)`. | CI target |
| `make sqlc-generate-ci` | Executes `sqlc generate -f sqlc.yaml` directly. | CI target |

### Go format / lint / dependency update related

| Command | Description | Notes |
| --- | --- | --- |
| `make fmt` | Formats Go code. | Executes `go fmt ./...`. |
| `make lint` | Executes static analysis via GolangCI-Lint. | None |
| `make fix` | Executes auto-fix via GolangCI-Lint. | None |
| `make tidy-lib` | Cleans Go module dependencies and updates `vendor`. | Executes `go mod tidy` and `go mod vendor`. |
| `make vendor-sync` | Regenerates `vendor` when it has drifted from `go.mod`. | Runs `go mod vendor` only when Go's own vendor consistency check fails, so it is a no-op in the normal case. Called by the `post-merge` / `post-checkout` hooks: `vendor` is gitignored, so the checkout that breaks is the one merely receiving someone else's `go.mod` change. |

### Go test related

| Command | Description | Notes |
| --- | --- | --- |
| `make test` | Executes tests for CI. | Runs `go test` on packages excluding `gen` / `cmd` / `mock` / `apperror` / `scripts` (the `internal/cli` core is now included). |
| `make test-cached` | Executes tests locally with the test cache enabled. | For pre-commit local runs. Same excluded packages as `test`, but omits `-count=1` so cached results are reused. |
| `make gen-test-repo` | Executes tests and generates HTML coverage report. | Output is `docs/coverage/index.html`. |
| `make test-cover-ci` | Executes tests with coverage. | CI target, outputs `coverage.out`. |
| `make cover-gate` | Fails if total coverage is below the threshold. | CI gate. `COVERAGE_THRESHOLD` (default 90). Requires `coverage.out` (run `test-cover-ci` first). |
| `make test-scripts` | Executes the `scripts/` tool tests for CI. | `scripts/` is excluded from the coverage targets above, so its tests need their own entry point. Not part of `cover-gate`. `actions-shellcheck`'s tests need `shellcheck` on the host (installed by `install-tools`) and skip themselves without it; CI sets `REQUIRE_SHELLCHECK` so those skips fail instead. |
| `make test-scripts-cached` | Executes the `scripts/` tool tests locally with the test cache enabled. | For pre-commit local runs. Same packages as `test-scripts`, without `-race -count=1`. |

### Go tool installation related

| Command | Description | Notes |
| --- | --- | --- |
| `make go-update` | Installs the Go runtime pinned in `mise.toml` via mise. See `docs/maintenance/go-upgrade.md`. | mise required |
| `make install-tools` | Installs the host development tools via mise (versions from `mise.toml`). | Installs `gopls`, `gotests`, `impl`, `dlv`, `lefthook`, `golangci-lint`, `zizmor`, `shellcheck`. `golangci-lint` and `zizmor` are the tools the pre-commit hook runs on the host because no musl build exists for the Alpine tool-runners; `shellcheck` is there because the hook's `test-scripts` runs `actions-shellcheck`'s tests on the host and they shell out to the real binary. |
| `make activate-tools` | Executes `lefthook install` to set up Git hooks. | None |
| `make sync-versions` | Propagates the `mise.toml` go / node / python versions into `go.mod` and the Dockerfile `FROM` lines. | Referenced by the `docs/maintenance/go-upgrade.md` procedure. Runs `scripts/sync-versions`. |

## `.makefiles/node` group

The repository's helper scripts are TypeScript, run through `tsx` from `scripts/node_modules/.bin`.
Their decision logic lives in `scripts/lib/**` so it can be tested without a repository to scan —
several of these scripts are gates, and a broken gate reports a clean run rather than an error.

| Command | Description | Notes |
| --- | --- | --- |
| `make scripts-test` | Runs the unit tests for `scripts/**/*.ts` with coverage and no cache. | Invokes `make scripts-test-ci` inside the `node_tool_runner` container. Fails when coverage drops below the thresholds in `scripts/vitest.config.mts`. |
| `make scripts-test-cached` | Runs the same tests with the cache enabled and no coverage. | Invokes `make scripts-test-cached-ci` inside the `node_tool_runner` container. The `pre-push` variant. |
| `make scripts-typecheck` | Type-checks `scripts/**/*.ts`. | Invokes `make scripts-typecheck-ci` inside the `node_tool_runner` container. |
| `make scripts-test-ci` | Runs `pnpm --dir scripts run test` (`vitest run --coverage --no-cache`). | CI target |
| `make scripts-test-cached-ci` | Runs `pnpm --dir scripts run test:cached` (`vitest run`). | CI target |
| `make scripts-typecheck-ci` | Runs `pnpm --dir scripts run typecheck` (`tsc --noEmit`). | CI target |

## `.makefiles/python` group

The CLI tools this repository installs from PyPI are declared in `python/*.in` and locked, with a
sha256 hash per package, in `python/*.txt` ([ADR-0084 (mise-ssot-drift-gate)](../docs/adr/0084-mise-ssot-drift-gate.md)).
These targets regenerate the lockfiles; nothing installs from a `.in` file directly.

| Command | Description | Notes |
| --- | --- | --- |
| `make py-lock` | Recompiles every `python/*.txt` from its `python/*.in`. | Invokes `make py-lock-ci` inside the `python_tool_runner` container. Run it after changing a pin, and commit both files. |
| `make py-lock-ci` | Runs `uv pip compile --generate-hashes --universal` per declaration, resolving against the Python version `mise.toml` declares. | CI target |

## `.makefiles/graphify` group

[graphify](https://github.com/graphify/graphify) turns the repository into a knowledge graph under
`graphify-out/`. That directory mixes two kinds of file: artifacts that mean the same thing in any
checkout, and state that only means something on the machine that produced it — the extraction cache
builds node IDs from the paths it was handed, so an absolute run bakes the host's user name into
them. `.gitignore` therefore whitelists the artifacts rather than listing what to ignore, and these
targets keep the two apart.

The semantic cache is the one part of `cache/` that is shared: its keys are content hashes, so it
saves re-running LLM extraction on a full rebuild. What it is *not* keyed by is the extraction prompt
that produced its contents. That prompt ships with graphify, so pinning the build is what makes it a
property of `python/graphify.in` rather than of whoever happened to run it — the `python_tool_runner`
image for the deterministic half, and `UV_CONSTRAINT` pointed at the lockfile for the extraction
workflow, whose skill would otherwise resolve its own interpreter from the newest release on PyPI.
[`.agents/graphify/spec-pin.toml`](../.agents/graphify/spec-pin.toml) records the resulting
fingerprint so CI can check a committed cache without installing graphify at all, and
`graphify-check` refuses one baked by anything else.

The build splits along what needs a model, and the two halves are produced in different places.
`graphify update` re-extracts changed code with tree-sitter, is deterministic, and runs unattended on
the release line (`graphify-sync.yaml`). Semantic extraction over docs needs a model and is driven by
an assistant, so it is manual: dispatch the `Graphify Extract` workflow, which is the canonical route
and runs on the repository's own token.

Running it locally instead is the escape hatch, not the default — the extraction is `/graphify
--update` in your own assistant session, so it spends *your* plan quota rather than the project's,
and there is deliberately no make target for it because make cannot invoke an assistant command. A
local run refreshes your working copy only: `graphify-check BASE=<ref>` rejects a feature branch that
carries the output, so the shared graph still comes from the release line. The manifest keeps an `ast_hash` and a `semantic_hash`
per file precisely so the deterministic half can run as often as it likes without stamping the
semantic side — pending doc work accumulates rather than being masked. `make graphify-pending`
reads that accumulation and reports it in changed lines rather than file count, because a typo fix
and a rewritten ADR are not the same amount of work to re-extract.

| Command | Description | Notes |
| --- | --- | --- |
| `make graphify-update` | Re-extracts the code files whose content changed into `graph.json`. Deterministic and needs no model. | Invokes `make graphify-update-ci` inside the `python_tool_runner` container. The graph is updated on the release line by `graphify-sync.yaml`; locally this keeps your own working copy current. |
| `make graphify-export` | Converts `graphify-out/graph.json` into the deterministic `nodes.json` / `edges.json` / `metadata.json` triple. | Invokes `make graphify-export-ci` inside the `go_tool_runner` container. |
| `make graphify-check` | Verifies that nothing outside the whitelist is tracked, that no tracked artifact carries an absolute path from the machine that produced it, and that the tracked semantic cache belongs to the extraction prompt `.agents/graphify/spec-pin.toml` pins. | Runs on the host: the subject is the git index, not a toolchain. Called from the `pre-commit` hook and the `Graphify Check` workflow. |
| `make graphify-check SPEC=<path>` | Also fingerprints the given `extraction-spec.md` and fails when it differs from the pin. | Run before an extraction. The prompt lives in the assistant's skill directory, which `bootstrap-external-skills.sh` writes into the checkout, so the extraction workflow verifies it there as well. |
| `make graphify-check BASE=<ref>` | Also fails when the diff against `<ref>` touches the output directory. | Used by the pull-request gate: the graph is a single blob that conflicts between concurrent branches, so its updates stay on the release line. |
| `make graphify-pending` | Reports how much semantic extraction is waiting: which documents changed since the last extraction, and by how many lines. | Reports only, never fails. The threshold is `GRAPHIFY_PENDING_THRESHOLD` (default 3000 changed lines, about 3% of the ~92,500-line document corpus) and exists to inform the decision to run `/graphify --update`, which needs a model no workflow here can start. The count and the per-file breakdown are printed on every run regardless, so a small but consequential rewrite is still visible below the threshold. |
| `make graphify-update-ci` | Runs `graphify update .` directly. | CI target. The scan root is passed explicitly because graphify would otherwise recover it from `graphify-out/.graphify_root`, which holds a host path and is not tracked. |
| `make graphify-export-ci` | Runs the export directly with `go run ./scripts/graphify-export`. | CI target |

## `.makefiles/docs` group

| Command | Description | Notes |
| --- | --- | --- |
| `make gen-portal-docs` | Generates Portal documentation. | None |
| `make gen-docs-json` | Generates Portal documentation link JSON. | None |
| `make gen-portal-build` | Builds the Portal frontend (`docs-viewer/`) into `docs/portal/` via Vite. | None |
| `make portal-test` | Runs the `docs-viewer/` test suite. | None |
| `make portal-typecheck` | Type-checks `docs-viewer/`. | None |
| `make gen-portal-docs-ci` | Generates Portal documentation directly via Node.js script. | CI target |
| `make gen-docs-json-ci` | Generates Portal JSON directly via Node.js script. | CI target |
| `make gen-portal-build-ci` | Runs pnpm directly to build the Portal frontend. | CI target |
| `make portal-test-ci` | Runs pnpm directly to test the Portal frontend. | CI target |
| `make portal-typecheck-ci` | Runs pnpm directly to type-check the Portal frontend. | CI target |
| `make gen-godoc` | Generates static godoc HTML into `docs/godoc/`. | None |
| `make gen-godoc-ci` | Runs godoc-static directly to generate static HTML. | CI target |

## `.makefiles/gen` group

| Command | Description | Notes |
| --- | --- | --- |
| `make gen` | Executes all code and documentation generation in batch. | Executes `gen-api` → `gen-query` → `gen-docs`. |
| `make gen-api` | Executes API-related generation in batch. | Executes `gen-bundle-oapi` →  `gen-api-docs` → `gen-go-code`. |
| `make gen-docs` | Executes documentation-related generation in batch. | Executes `gen-api-docs`, `gen-portal-docs`, `gen-docs-json`. |
| `make gen-all-docs` | Executes all documentation generation processes. | Executes `gen-docs`, `gen-db-schema`, `gen-test-repo`. |
| `make gen-query` | Executes SQLC code generation in batch. | Executes `dump-schema` → `merge-dml` → `gen-sqlc` → `fmt`. |
| `make gen-query-repo` | Executes SQLC code generation for Repository. | Executes `dump-schema` → `merge-dml-repo` → `gen-sqlc`. |
| `make gen-query-qs` | Executes SQLC code generation for Query Service. | Executes `dump-schema` → `merge-dml-qs` → `gen-sqlc`. |
| `make gen-query-sysq` | Executes SQLC code generation for System Query. | Executes `dump-schema` → `merge-dml-sysq` → `gen-sqlc`. |

## `.makefiles/github` group

### GitHub Actions lint / pin related

| Command | Description | Notes |
| --- | --- | --- |
| `make actions-lint` | Lints workflow definitions with actionlint, shellchecks the `run:` scripts of every composite action, then runs the three node checks: no job posting a PR comment receives a secret, no comment body is wrapped in a fixed-length fence, and every job defines what happens when it is cut off. | The one lint group whose stages span two tool-runners: it invokes `make actions-actionlint-ci` and `make actions-shellcheck-ci` in `go_tool_runner` and `make actions-node-lint-ci` in `node_tool_runner`, rather than one `-ci` target in one container, because actionlint / the shellcheck runner are Go tools and the rest node scripts. The three node checks are bundled behind one target so they cost one container start, the same shape `md-lint` uses. |
| `make actions-comment-secret-lint` | Runs the PR-comment secret check alone. | Invokes `make actions-comment-secret-lint-ci` inside the `node_tool_runner` container. |
| `make actions-comment-fence-lint` | Runs the PR-comment fence check alone. | Invokes `make actions-comment-fence-lint-ci` inside the `node_tool_runner` container. |
| `make actions-cutoff-lint` | Runs the job cut-off check alone. | Invokes `make actions-cutoff-lint-ci` inside the `node_tool_runner` container. |
| `make actions-shellcheck` | Extracts `runs.steps[].run` from every composite action under `.github/actions/**` and checks each `bash` / `sh` script with `shellcheck`; a step on any other shell is reported as skipped (`scripts/actions-shellcheck`). | Invokes `make actions-shellcheck-ci` inside the `go_tool_runner` container. Covers what `actionlint` cannot see: it only walks `.github/workflows`, and an `action.yaml` handed to it directly is parsed as a workflow. A `run:` written as a folded scalar (`>`) is rejected — write it as a literal (`\|`) — because folding drops the line breaks a finding's position is mapped back through. |
| `make actions-lint-ci` | Runs actionlint, the composite-action shellcheck, then the bundled node checks directly. | CI target. actionlint runs first on purpose: the node checks read workflow structure by column and rely on the file parsing as YAML at all. |
| `make actions-node-lint-ci` | Runs the three node checks (secret / fence / cut-off) directly. | CI target. |
| `make actions-actionlint-ci` | Runs `actionlint` directly. | CI target. |
| `make actions-shellcheck-ci` | Runs `scripts/actions-shellcheck` directly. | CI target. |
| `make actions-comment-secret-lint-ci` | Fails when a job using `upsert-pr-comment` is passed a secret other than `GITHUB_TOKEN` (`scripts/pr-comment-secret-lint/index.ts`). | CI target. Why the rule exists: [`.github/workflows/README.md`](../.github/workflows/README.md). |
| `make actions-comment-fence-lint-ci` | Fails when a `run:` block emits a fixed-length Markdown fence around a PR comment body, or the duplicated `fence_for` helpers diverge (`scripts/pr-comment-fence-lint/index.ts`). | CI target. Why the rule exists: [`.github/workflows/README.md`](../.github/workflows/README.md). |
| `make actions-cutoff-lint-ci` | Fails when a job carries no `timeout-minutes`, or a step calling `upsert-pr-comment` has an `if:` a cancelled job cannot reach (`scripts/actions-cutoff-lint/index.ts`). | CI target. Why the rule exists: [`.github/workflows/README.md`](../.github/workflows/README.md). |
| `make pin-actions-resolve` | Resolves each `uses:` tag to its commit SHA and updates the `.github/actions-pin.toml` lockfile. | Quarantines refs younger than `PIN_ACTIONS_MIN_AGE_DAYS` (default 14; 0 disables). |
| `make pin-actions-apply` | Pins `uses:` to `@<sha> # <tag>` from the lockfile. | None |
| `make pin-actions-check` | Verifies `uses:` are pinned per the lockfile (no write). | CI / pre-commit gate. |
| `make egress-apply` | Writes `.github/egress.toml` into every job's inline `allowed-endpoints` block. | The classes and how to add a host: [`.github/workflows/README.md`](../.github/workflows/README.md) § Runner Hardening. |
| `make egress-check` | Verifies every inline `allowed-endpoints` block matches the SSOT (no write). | CI / pre-commit gate. |

### Commit message lint related

| Command | Description | Notes |
| --- | --- | --- |
| `make commitlint COMMIT_MSG_FILE=<file>` | Lints a commit message with commitlint. | Invokes `make commitlint-ci` inside the `node_tool_runner` container. Wired to the `commit-msg` hook. The message file is copied under `tmp/` and handed over as a relative path, because in a `git worktree` the path git gives the hook lies outside the container's `.:/app` mount. `COMMIT_MSG_FILE` defaults to `git rev-parse --git-path COMMIT_EDITMSG`. |
| `make commitlint-ci COMMIT_MSG_FILE=<file>` | Runs `commitlint --edit <file>` directly. | CI target. |
| `make commitlint-range-ci COMMITLINT_FROM=<ref> COMMITLINT_TO=<ref>` | Lints every commit message in the range. | CI target, and the only route that reaches a message the `commit-msg` hook was bypassed for. Exits 2 when either ref is missing or the range is empty, so a broken ref cannot pass as a clean run. Has no `node_tool_runner` wrapper: the container mounts `.:/app` only, which leaves a worktree's gitdir outside it, and history cannot be copied in the way a message file can. |

### GitHub configuration related

| Command | Description | Notes |
| --- | --- | --- |
| `make gh-login` | Logs in to GitHub using `gh` command. | Uses browser-based authentication. |
| `make delete-all-labels` | Deletes all existing labels in the GitHub repository. | None |
| `make create-default-labels` | Creates default labels based on `.github/settings/labels.json`. | None |
| `make apply-branch-protection` | Applies branch rules based on `.github/settings/branch-protection.json`. | One-directional apply. Nothing re-applies the JSON or compares it against the live ruleset afterwards, so the file states intent rather than the enforced state — see `.github/settings/README.md`. |
| `make enable-workflows` | Enables every workflow left in `disabled_fork` state. | Idempotent. A newly created repository starts with all workflows disabled. |

### GitHub repository initialization related

#### `make setup-repo`

Executes repository initialization in batch.
Performs the following in order.

- `gh` login
- Create and push initial tag `v0.0.0`
- Create branches `develop` / `staging` / `production`
- Set GitHub default branch
- Apply branch rule set
- Initialize labels

The `git` / `gh` parts run through `scripts/repo-setup` (`preflight` / `bootstrap` /
`prune-release-notes`); the label, rule set, and workflow steps stay as their own `make` targets, so
this target is the chain of the two.

This is the initial setup command when launching a new repository.

#### Setup helper commands

| Command | Description | Notes |
| --- | --- | --- |
| `make setup-replace-module OLD_MODULE=<old> NEW_MODULE=<new>` | Replaces Go module name in batch. | Updates `go.mod` and import paths using `node_tool_runner`.  <!-- setup-localize:line --> |
| `make setup-replace-app-metadata APP_NAME=<name> OPENAPI_TITLE=<title> COPILOT_TITLE=<title>` | Replaces application name and OpenAPI title in batch. | Reflected in README and OpenAPI definitions.  <!-- setup-localize:line --> |
| `make setup-replace-repository-reference REPOSITORY=<org/repo>` | Replaces repository references (GitHub URLs, etc.) in batch. | Updates links in README and documentation.  <!-- setup-localize:line --> |
| `make setup-replace-license-copyright COPYRIGHT_HOLDER=<name> [COPYRIGHT_YEAR=<year>]` | Updates LICENSE copyright notation. | Year is optional.  <!-- setup-localize:line --> |
| `make setup-replace-codeowners OWNERS='<owners>'` | Replaces the owner of every rule in `.github/CODEOWNERS` in batch. | Takes `@user` / `@org/team` / an email, space-separated for several. Comment lines are left untouched, so the header keeps its example.  <!-- setup-localize:line --> |
| `make setup-verify` | Verifies the localization landed, then removes the localization tooling. | Runs `scripts/setup/verify-setup` in `node_tool_runner`; expects the Phase 5 values in the environment.  <!-- setup-localize:line --> |
| `make setup-remove-boilerplate-identity` | Removes what only holds while this repository is a boilerplate. | Scans the repository for `boilerplate-only` markers and resolves each via `node_tool_runner`, deletes the boilerplate-only conventions doc, then removes itself. Preview with `DRY_RUN=1`. <!-- boilerplate-only:line --> |
| `make setup-remove-sample-api` | Removes the sample API (`user`/`product`/`order`) in batch. | Deletes via `node_tool_runner`, then runs `db-local-reinit` / `db-test-reinit` → `gen-api` → `gen-query` → `tidy-lib` → `fix` → `lint`. The DB rebuild keeps dropped tables out of the generated models, and `tidy-lib` drops the direct dependencies the sample API was the only user of. **Requires the DB container (`database`) running** (`gen-query` dumps the live schema). Preview without changing anything with `DRY_RUN=1` (any non-empty value counts as preview, `0` included, so omit the variable entirely for a real run). <!-- sample-api:line --> |
| `make setup-remove-doc-language` | Resolves the documentation / skill translation pairs against `LANG_CHOICE` — `en` / `ja` fold into that language, `both` keeps the pairs and resolves the markers. | Runs on the host, not through the tool runner: it commits the whole fold at once and needs the host's git. Run it **before** every other removal — each removal tool carries paired declarations and prunes its own pair when the fold resolves it, while Phase 12 deletes a workflow this tool declares a string in, so a fold attempted afterwards aborts. Preview with `DRY_RUN=1` (works on a dirty tree; the real run requires a clean one). <!-- lang-choice:line --> |

### Base branch resolution related

| Command | Description | Notes |
| --- | --- | --- |
| `make base-branch` | Prints the branch name of the latest release line (`release/vX.Y.Z`) on one line. | Reads `origin`'s live state with `git ls-remote` (`scripts/base-branch`), so a stale local `refs/remotes/origin/HEAD` — which `git fetch` never updates — and a GitHub default branch still pointing at an earlier release line both leave the answer unchanged. "Latest" means the numeric version comparison, not the commit date; the reasoning is in the package comment. Output carries no decoration so a caller can take it with `$(make base-branch)`; where a pull request already exists, its `baseRefName` is the authority and this is the fallback. |

### Release branch related

| Command | Description | Notes |
| --- | --- | --- |
| `make hotfix-patch` | Creates a hotfix branch from `production` and sets it as the default branch. | Advances patch by one based on the latest tag. Runs `scripts/release branch`, which aborts when the branch already exists on `origin` or the worktree is dirty. |
| `make branch-patch` | Creates a patch release branch from `production` and sets it as the default branch. | Advances patch version based on the latest tag. Runs `scripts/release branch`. |
| `make branch-minor` | Creates a minor release branch from `production` and sets it as the default branch. | Advances minor version based on the latest tag. Runs `scripts/release branch`. |
| `make branch-major` | Creates a major release branch from `production` and sets it as the default branch. | Advances major version based on the latest tag. Runs `scripts/release branch`. |

### Release tag related

| Command | Description | Notes |
| --- | --- | --- |
| `make tag-patch` | Creates a tag with incremented patch version and creates a GitHub Release. | Uses `.github/release/<version>.md` for release notes. Runs `scripts/release tag`, which syncs `production` to `origin` **before** looking for that file — the tag is cut from `production` HEAD, so the note has to exist there. |
| `make tag-minor` | Creates a tag with incremented minor version and creates a GitHub Release. | Based on the latest tag. Runs `scripts/release tag`. |
| `make tag-major` | Creates a tag with incremented major version and creates a GitHub Release. | Based on the latest tag. Runs `scripts/release tag`. |
