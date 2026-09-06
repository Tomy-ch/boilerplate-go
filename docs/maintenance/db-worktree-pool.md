# Shared Infra and the DB Slot Pool (parallel worktree development)

The mechanism that lets several git worktrees (and the main checkout) use a **single shared infra**
in parallel without colliding. Compose services are split into two layers:

- **infra layer** — the services that can only run on fixed ports (`database` 5432 / `observability`
  3000, 4317, 4318, 3200 / `garage` 3900, 3902). They live in the fixed compose project
  `gobp-shared`, with **one instance for all checkouts**.
- **app layer** — `api_server` / `mock_auth_server`, which every checkout needs for itself. They are
  split into a per-checkout compose project and started in parallel on shifted host ports.

Worktree separation of the DB is "a different database inside the same instance"
(`wt<N>_local` / `wt<N>_test`), not "a different container on a different port". That removes the
need to allocate a host port per DB, so neither "another worktree holds 5432, so `make test` cannot
run" nor "the main checkout's serve collides with a worktree's DB" can happen. For o11y, sharing is
an advantage: the traces / metrics / logs of every checkout land in a single Grafana.

## The invariant: database : worktree = 1 : 0..1

**No database is ever reached from two places.** A worktree is free to take a slot or not, but a
worktree that takes none owns *no database* — it does not fall back to the default `local` / `test`.

| Who | Owns |
| --- | --- |
| main checkout | `local` / `test` / `gen_schema` |
| worktree holding slot N | `wt<N>_local` / `wt<N>_test` / `gen_schema_wt<N>` |
| worktree holding no slot | nothing — targets that touch a database fail |

Falling back to the defaults would put the main checkout's databases under two owners, and it would
do so *silently*: the failure surfaces later as a test that passes against another branch's
migrations, or as a generated artifact rebuilt from a schema someone else was mid-migration on.
`make require-db-owner` (`.makefiles/database/pool.mk`) is therefore a prerequisite of every target
that resolves a database name — `db-migrate-*` / `db-seed` / `db-drop-tables` / `db-ensure` /
`dump-schema`, plus `make test` / `test-cached` / `gen-test-repo` (a host-run `go test` reads
`DB_NAME_TEST`) and `make serve` / `serve-build` / `serve-build-clean` (the app container reads
`DB_NAME_LOCAL`). The check lives in `internal/cli/dbslot`. A linked worktree is identified by the
`git-dir` ≠ `git-common-dir` split, so the main checkout and CI pass through untouched. The cases
where `git` cannot answer at all are not treated alike: no `git` executable (the tool-runner
containers) and no repository both pass through, since neither can be a worktree, while a directory
that *is* a repository whose layout `git` will not report fails instead — a worktree cannot be ruled
out there, and falling back silently is precisely what this guard exists to prevent.

The consequence to know about: in a worktree, `make test` fails until you run `make slot-acquire`.
That is the point — before this guard it quietly ran against the shared `test` database.

## How it works

- **infra layer** = the fixed compose project `gobp-shared` (`GOBP_DB_SHARED_PROJECT`). A worktree
  gets a different default compose project name per directory, so the shared side is pinned to an
  explicit fixed name. `make infra-up` starts it; `make serve` / `make job` / `make worker` also call
  it idempotently first. Idempotent here means non-destructive too: an already-running container is
  left alone rather than re-created (see Caveats).
- **app layer** = `APP_PROJECT`: `gobp-app-<directory name>` when no slot is held, `gobp-wt-N` when
  one is. `docker-compose.attach.yaml` is overlaid so the shared infra is reached through
  `host.docker.internal` on its host-published ports (`DB_HOST` / `ENDPOINT_OTLP` /
  `ENDPOINT_OBJECT_STORAGE` are overridden as runtime env; `loader.go` gives runtime env priority
  over `env/.env`).
- **slot N** = the database-name pair `wt<N>_local` / `wt<N>_test` inside the shared DB (MAX 12 by
  default = wt1–wt12), plus the throwaway `gen_schema_wt<N>` that schema generation rebuilds.
  Acquiring a slot is an **opt-in for parallel work**: the main checkout needs none, and a worktree
  that wants no database need not take one. What a worktree cannot do is use a database it does not
  own (see the invariant above).
- **implementation** = the host-run Go CLI `cmd/db-slot` (core in `internal/cli/dbslot`), which owns
  lease decisions, database creation, and compose startup in a testable form. The make targets call
  `go run ./cmd/ db-slot <sub>`.
- **lease** = the lock directory `${GOBP_DB_POOL_DIR:-~/.cache/gobp-db-pool}/slot-N.lock` on the
  host. A new acquisition relies on the atomicity of `os.Mkdir`, stale reclaim seizes ownership
  atomically via `rename`, and the whole acquire scan is serialized with `flock`, so two worktrees
  never double-lease the same slot. `meta` (owner / heartbeat / branch) is `0600`, and a pool dir
  pointing at a symlink is rejected as a pre-read attack countermeasure.
- **runtime environment guard** = db-slot refuses to run when `APP_ENV` is a deploy environment
  (dev/stg/prd), since it creates and destroys databases and is therefore dev/test only. The test
  configuration in `internal/config` likewise ignores the DB_NAME override in deploy environments.
- **slot record** = acquire writes `.gobp-db-slot` (gitignored) at the worktree root. `make`
  `-include`s it, overriding the defaults from `.makefiles/docker/compose.mk` and propagating to
  every target:
  - `DB_NAME_LOCAL` / `DB_NAME_TEST` = `wt<N>_local` / `wt<N>_test` (default `local` / `test`; a
    host-run `go test` connects through the shared DB on localhost:5432 to its own worktree database
    under this name — read by the test configuration in `internal/config`)
  - `API_HOST_PORT` = `8080+N` / `MOCK_AUTH_HOST_PORT` = `2010+N` / `DLV_HOST_PORT` = `2345+N` /
    `PPROF_HOST_PORT` = `6060+N` (every app-layer host port is relative to the slot number)
  - `SERVE_PROJECT` = `gobp-wt-N` (the app layer's compose project = `APP_PROJECT`)
  - `COMPOSE_PROJECT_NAME` = `gobp-shared` (moves the default project to the infra layer so DB
    tooling — migrate / seed / psql / gen — runs on the shared infra's network; `compose.mk` sets the
    same default even when no slot is held)
- **persisted data that follows the shifted ports**: a host port is not only something to connect to —
  it can also be *stored* in the database. The JWT issuer is one such value: the mock auth server
  publishes on `2010+N`, so the `iss` of the tokens it issues shifts with the slot, and the
  row that the resolver matches on `(issuer, subject)` has to shift with it — with a
  pinned literal, every authenticated endpoint answers 401 in a worktree that holds a slot. The seed
  file therefore stores `${AUTH_ISSUER}` instead of the URL and `make db-seed` passes this slot's value
  in (see `database/seed/README.md`), so `db-reinit` / `db-seed` / `slot-acquire` all leave an identity
  that matches the environment. Data of this kind that you add later has to follow the slot the same
  way, rather than pinning the default port. Like the database name, the value reaches a host-run
  `go test` only through `make` (`make test` / `test-cached` export it) — run DB-backed tests through
  those targets, since a bare `go test` gets neither `DB_NAME_TEST` nor the slot's issuer.
- **extension bootstrap**: after `CREATE DATABASE` (guarded by an existence check) for
  `wt<N>_local` / `wt<N>_test`, acquire sets the `pg_trgm` extension on each database (the same thing
  the init script applies to `local` / `test`; a dynamically created worktree database needs it set
  explicitly). Timezone is *not* set per database: the `database` service's `TZ` is written into
  `postgresql.conf` at `initdb` time, so it is the cluster default and a database created later
  inherits it. The consequence is that a shared volume initialised before `TZ` was set keeps its old
  cluster default, and a slot leased in it shows that timezone in `psql` — the application is
  unaffected, because it sets the timezone per connection in the DSN. To pick the new default up,
  recreate the volume (`docker compose -p gobp-shared down -v` → `make db-init`) once every slot has
  been freed; the volume is shared by every worktree. See `env/README.md` (Changing the Timezone).
- **schema safety**: after acquiring, acquire rebuilds `wt<N>_local` / `wt<N>_test` to the current
  branch's schema via drop → migrate → seed. Inheriting a slot another branch used is therefore safe.
- **schema-generation isolation**: `dump-schema` (behind `make gen-query`) dumps neither the shared
  `local` nor your own working database. It drops a throwaway database (`SCHEMA_GEN_DB` —
  `gen_schema_wt<N>` when a slot is held, `gen_schema` for the main checkout), migrates it up from
  *this* branch's migrations, and dumps that. A deterministic dump needs an unconditional
  drop → migrate immediately before it, which cannot be aimed at a working database — it would wipe
  the seed on every `gen-query` and pull the tables out from under a running `make serve`. Because
  the throwaway database is per-owner like every other one, two checkouts running `make gen-query`
  at the same time no longer rebuild the same database underneath each other. This is a local-only
  guard: CI migrates a fresh postgres service and calls `dump-schema-ci` directly, so it never takes
  this path.
- **release**: `slot-free` stops the containers of this slot's app project (`gobp-wt-N`) before
  deleting the lease and `.gobp-db-slot`. The databases are kept warm for the next tenant.
- **safe stale reclaim**: a lease whose heartbeat has exceeded the TTL (1800 seconds by default,
  `GOBP_DB_POOL_TTL`) can be re-acquired by another worktree, so a crashed worktree does not hold a
  slot forever. Because the heartbeat is only sent on `make serve`, a slot whose app is left running
  also goes stale once the TTL passes. Before rebuilding the databases, therefore, two checks confirm
  the slot is genuinely unused; if it is in use, the slot is skipped rather than destroyed:
  1. the app project (`gobp-wt-N`) has no running container
  2. `pg_stat_activity` shows no connection to that slot's databases

  Connection pools drain when idle, so check 2 alone would miss a worktree that is serving.
  Conversely check 1 alone cannot catch a host-run `go test`, so both are used.

## Usage

A checkout that takes no slot (the main checkout, typically) reaches the shared infra with a plain
`make serve`.

```sh
make serve           # start the shared infra and the app as gobp-app-<dir> → curl localhost:8080
make serve-stop      # stop only this checkout's app (the shared infra stays up)
make infra-up        # start just the shared infra
make infra-down      # stop the shared infra (affects every checkout)
```

Acquire a slot when working in a worktree in parallel.

```sh
make slot-acquire    # lease a free slot and create/rebuild this worktree's databases
make test            # connects from the host through localhost:5432 to wt<N>_test
make serve           # start the app as gobp-wt-N → curl localhost:$API_HOST_PORT (DB is the shared wt<N>_local)
make slot-status     # show slot occupancy (database names / API ports)
make slot-free       # release only the slot (databases stay warm, the worktree stays)
```

Use `slot-release` when the work is done and the worktree itself is being retired. It stops the app
and removes locally built images, releases the slot, and removes the worktree, in that order.

```sh
make slot-release    # stop app + remove images → release the slot → remove the worktree
```

The order cannot be rearranged. Once `slot-free` deletes `.gobp-db-slot`, `SERVE_PROJECT` is lost and
`APP_PROJECT` falls back to `gobp-app-<dir>`, so releasing first would point the app shutdown at a
different project. `git worktree remove` deletes the cwd along with everything else, so it must come
last. Git refuses the removal when uncommitted or untracked files remain, which is why `--force` is
not passed. Run by mistake in the main checkout, it exits with an error without doing anything.

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `GOBP_DB_POOL_DIR` | `~/.cache/gobp-db-pool` | Where the lease registry lives (a symlink is rejected) |
| `GOBP_DB_SHARED_PROJECT` | `gobp-shared` | Fixed compose project name of the shared infra |
| `GOBP_API_POOL_BASE` | `8080` | Base for `API_HOST_PORT` (slot N = base + N) |
| `GOBP_MOCK_AUTH_POOL_BASE` | `2010` | Base for `MOCK_AUTH_HOST_PORT` |
| `GOBP_DLV_POOL_BASE` | `2345` | Base for `DLV_HOST_PORT` |
| `GOBP_PPROF_POOL_BASE` | `6060` | Base for `PPROF_HOST_PORT` |
| `GOBP_DB_POOL_MAX` | `12` | Number of slots (= the cap on concurrent parallel work) |
| `GOBP_DB_POOL_TTL` | `1800` | Heartbeat grace period for the stale decision (seconds) |

## Caveats

- **Blast radius of a shared instance**: every checkout shares one Postgres / o11y / object storage.
  Databases are isolated, so DDL cannot cross a database boundary, and `db-init` / `db-local-reinit` /
  `db-test-reinit` now resolve their target from the slot you hold rather than hardcoding
  `DB=local` / `DB=test` — so they rebuild your own databases. Passing `DB=` explicitly still overrides
  that, and a name belonging to another owner is not checked, so `make db-reinit DB=<name>` is the one
  way left to destroy someone else's. `make infra-down` likewise stops every checkout.
- **Parallel tests contend over establishing connections, not over capacity**: when tests run at the
  same time, packages unrelated to your change fail with `failed to ping DB`, while `too many clients`
  never appears. The instance has connections to spare; what saturates is how many are being
  *established* at the same instant, and the ping budget expires while they queue. It does not take two
  worktrees — lefthook's `pre-commit` / `pre-push` are `parallel: true`, so `make lint` and `make test`
  overlap inside a single checkout. The test path is already tuned against this: see `DBCONN_MIN_CONNS`
  and `DB_PING_TIMEOUT` in `env/README.md` for what `ci` sets and why, mirrored by the test
  configuration in `internal/config` for the paths that do not load an env file. To diagnose a
  recurrence: `pgrep -fl "go test"` then
  `lsof -a -p <pid> -d cwd` identifies which checkout is running tests, sampling `pg_stat_activity`
  shows whether the peak is a momentary spike rather than a plateau, and a re-run with `go test -p 1`
  that comes back green means the failure was load rather than the change under test.
- **Re-creation of the infra layer**: compose decides whether a container is current by hashing its
  resolved service definition, and that hash includes bind-mount sources and build contexts as
  *absolute* paths. Every worktree resolves them under its own directory, so the hash differs
  between checkouts of the very same commit — re-creation is the norm, not a branch-divergence edge
  case. `database` and `garage` are affected (they bind-mount `docker/database/sql` and
  `docker/garage/garage.toml`); `observability` is not, because it mounts nothing. An `up` against
  `gobp-shared` from a worktree therefore passes `--no-recreate` (`INFRA_NO_RECREATE` in
  `.makefiles/docker/compose.mk`), which keeps a container another checkout is using rather than
  replacing it. The cost is that a *legitimate* definition change — a new image digest pin, an
  edited `garage.toml` — no longer takes effect on its own: run `make infra-down && make infra-up`
  at a point where every checkout can afford the interruption. The same applies to the `tools`
  profile, so `docs_server` keeps serving the `docs/` of whichever checkout first created it.
  A single checkout has no one to contend with, so the flag stays empty there and compose
  re-converges on a definition change as usual.
- **Object storage is shared**: the `garage` bucket is common to every checkout (unlike a database it
  has no schema, so it does not break across branches). Point a branch at a different
  `OBJECT_STORAGE_BUCKET` to isolate it. The access key is shared the same way — `garage_init` imports
  it under one fixed key name from the running checkout's `env/.env`, so a branch that edits
  `OBJECT_STORAGE_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` changes what every other checkout authenticates
  with. Isolate by bucket, not by credential.
- **The queue is shared and cannot be isolated by configuration alone**: `elasticmq` serves one set of
  queues to every checkout, and unlike the object-storage bucket, pointing `OUTBOX_QUEUE_URL` at a
  different name is not enough — ElasticMQ creates only the queues declared in
  `docker/elasticmq/elasticmq.conf` and expands no environment variables, so a name nothing declared
  is simply absent. Two checkouts running `make outbox-relay` at once therefore publish into the same
  queue, and whichever consumer reads first takes the message. Nothing consumes it today
  unless a worker is registered, in which case two checkouts running the relay do contend. To isolate a
  branch, declare an extra queue in `elasticmq.conf` and point `OUTBOX_QUEUE_URL` at it — the conf is
  read at start-up, so the change needs `make infra-down && make infra-up`, which interrupts every
  checkout. Per-slot queues are not pre-declared because the pool size is configurable
  (`GOBP_DB_POOL_MAX`), and a static list in the conf would silently stop covering the pool as soon as
  that value changed.
- **The Realtime Delivery emulators are shared instances that isolate by name.** Neither one needs a
  conf change or an `infra-down` to separate two checkouts, which is what distinguishes them from
  `elasticmq` above: `dynamodb_local` runs `-sharedDb`, so a single table namespace serves everyone,
  and `goaws` creates topics and queues at runtime (`docker/goaws/goaws.yaml` declares none). In both
  cases a branch isolates itself by choosing different names, and the `Resolver` in
  `internal/cli/dbslot` chooses them: it suffixes `REALTIME_TOPIC`, `REALTIME_QUEUE_PREFIX` and
  `REALTIME_TABLE_SUFFIX` with the slot and emits all three from `db-slot env`, so each worktree's
  serve subscribes to its own topic and reads its own tables (`realtime_event_log_local_wt<N>` and its
  two siblings, created by `make realtime-init` from the same value). `docker-compose.attach.yaml`
  reads those values instead of deriving them a second time, and keeps only the no-slot default, the
  way it does for the host ports. A checkout holding no slot therefore keeps the unsuffixed names, so
  the main checkout is unaffected. The table suffix is joined with `_` rather than the `-` the topic
  and queue use, because `REALTIME_TABLE_SUFFIX` admits only lowercase letters, digits and underscores.

  Both halves matter, and they fail differently when absent. Unsuffixed **tables** let one worktree
  read a stream another wrote. An unsuffixed **topic** fails more narrowly but more surprisingly: a
  wakeup for an unknown stream is a no-op, whereas a revocation is matched on `(subject, destination)`
  and every worktree's database is seeded from the same fixtures, so revoking a ticket in one window
  would close another window's connection for the same subject. `make realtime-smoke` isolates itself
  the same way, under a per-run random name it deletes on exit.
- **A slot's two stores are reset together.** Realtime Delivery allocates stream sequences in
  PostgreSQL and keeps the delivered events in the DynamoDB EventLog, so rebuilding one without the
  other leaves the allocator issuing numbers the log already holds. `Append` refuses that position —
  correctly, since a different event ID at the same sequence is a real conflict — and the relay's
  head-of-line blocking then stops that stream. Every path that rebuilds a **local** database
  therefore ends in `make realtime-reset`, which drops that checkout's three tables — `slot-acquire`,
  `db-local-reinit` and `db-init-local` alike, since the hazard follows the database rather than the
  slot. `realtime-provision` creates them again on the next `make serve`; the test database has no
  paired store, so its rebuild does not carry the reset. Only streams with a fixed identifier can
  collide at all (a per-subject stream gets a fresh UUID from the reseed), which is what makes the
  failure rare enough to be confusing rather than obvious.
- `sql_editor` / `docs_server` / `er_diagram_generator` / `mock_auth_server` sit in the `2000` range
  because none of them has a de-facto port of its own. The rule, and why that range is safe, are in
  [`local-environment.md`](local-environment.md).
- The wiring spans `docker/`, `internal/cli/dbslot`, and `.makefiles/`, so update this document
  whenever it changes.
