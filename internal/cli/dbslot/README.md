# db-slot

Leases a per-worktree slot on a single shared infra stack so multiple `git worktree`s can run DB-backed work in parallel without host-port conflicts. Each slot owns its own databases (`wt<N>_local` / `wt<N>_test`) inside the shared DB container (fixed infra compose project `gobp-shared`, host 5432) plus its own app-layer compose project (`gobp-wt-N`) whose host ports are slot-relative. Every checkout attaches to the same shared infra either way, so leasing a slot is **opt-in** — it is what makes parallel work collision-free. See `docs/maintenance/db-worktree-pool.md` for the full model.

Runs on the **host** (not in a tool-runner container): it manages host-filesystem leases and drives host `docker compose`, and connects to the shared DB via pgx on `localhost:5432`.

## Commands

```text
db-slot acquire        # lease a free slot, create/setup wt<N> DBs, write .gobp-db-slot
db-slot release        # stop the slot's app containers, drop the lease (DBs left warm)
db-slot heartbeat      # refresh the held slot's lease heartbeat
db-slot status         # print slot occupancy, then this checkout's resolved values
db-slot env            # print the resolved values as KEY=VALUE for `make` to eval
db-slot require-owner  # fail unless this checkout owns a database (exit status is the interface)
```

Prefer the make wrappers (`make slot-acquire` / `slot-free` / `slot-release` / `slot-status`) — `slot-acquire` also rebuilds the schema of the leased DBs, and `slot-release` tears the whole worktree down.

## Design

- **Lease** (`Registry`) — a lock directory per slot under `~/.cache/gobp-db-pool` (override `GOBP_DB_POOL_DIR`). `os.Mkdir` gives atomic fresh acquisition; stale reclaim atomically re-claims via `rename`, and a `flock` scan lock serialises the whole acquire loop so two worktrees can never double-lease the same slot. Symlinked pool dirs are refused (pre-attack guard); meta files are `0600`.
- **DB admin** (`DBAdmin` / `PgxAdmin`) — `CREATE DATABASE` + `pg_trgm` extension on each `wt<N>` DB, and a `pg_stat_activity` connection check used when deciding whether a stale slot is safe to reclaim. Timezone is not among them: the `database` container's `TZ` is the cluster default, which a database created later inherits.
- **In-use detection** — before reclaiming a stale slot, both its app project (`gobp-wt-N`) is checked for running containers and its databases for live connections. A connection pool empties while idle, so the connection check alone misses a worktree that is still serving; the container check alone misses host-run `go test`.
- **Compose** (`Compose` / `ExecCompose`) — brings up the shared DB in the fixed infra project (`--wait --no-recreate`) and, on release, tears down the slot's app project (`gobp-wt-N`) via `docker-compose.yaml` alone, with the `development` profile the app services live behind — without it `down` selects nothing and still exits 0, so the containers survive a release that reported success. The app-layer override is deliberately not stacked on that `down`: it changes only `api_server`'s ports, `depends_on` and environment, and `down` consults none of them. Leaving it stacked would also tie this cleanup path to a compose quirk. The override's `REALTIME_*` are required (`:?`) and only `make`'s `COMPOSE_APP` emits them, yet `down` still succeeds here because the project arrives as `COMPOSE_PROJECT_NAME` — in that form `down` works off the project label and never interpolates the files, while the same command with `-p` does interpolate and fails. Nothing states that asymmetry, so a later rewrite of `newComposeCmd` to the explicit `-p` form would break `slot-free` with no other change. Passing the file only where it is read removes that trap; injecting a placeholder value to survive interpolation instead was rejected, since `down` never reads what it would carry. `--no-recreate` keeps `acquire` from replacing a container another checkout is serving from; see [`docs/maintenance/db-worktree-pool.md`](../../../docs/maintenance/db-worktree-pool.md) for why it carries no condition here.
- **Slot file** — `acquire` writes `.gobp-db-slot`, a gitignored `KEY=VALUE` file that `make` `-include`s to override the defaults in `.makefiles/docker/compose.mk`: `SLOT`, `DB_NAME_LOCAL` / `DB_NAME_TEST`, the slot-relative host ports `API_HOST_PORT` / `MOCK_AUTH_HOST_PORT` / `DLV_HOST_PORT` / `PPROF_HOST_PORT`, `COMPOSE_PROJECT_NAME` (the shared infra project) and `SERVE_PROJECT` (`gobp-wt-N`, the app-layer project). The file is not proof of the lease: a worktree whose slot was reclaimed as stale keeps it, so both `Resolver` and `release` check the registry before acting on what it declares.
- **Resolved values** (`Resolver`) — everything that is a *function of the slot* is derived here rather than in `make` or a compose file: `DB_LOCAL` / `DB_TEST`, the app-layer compose project, the mock-auth issuer URL, `INFRA_NO_RECREATE`, and the Realtime Delivery names (`REALTIME_TABLE_SUFFIX` / `REALTIME_QUEUE_PREFIX` / `REALTIME_TOPIC`) that keep two worktrees off each other's streams. The Realtime names have two consumers — `docker-compose.attach.yaml`, which hands them to the app, and `make realtime-reset`, which drops the tables they name — which is exactly why the derivation may not be written twice. `DB_LOCAL` reaches the overlay the same way (as `DB_NAME`), so the app container names its database from the resolved value rather than from the raw `DB_NAME_LOCAL` the slot file carries. Each of them is derived from the slot number whose lease the registry confirms, never from the strings `.gobp-db-slot` happens to carry. The base names those three are suffixed onto come from the embedded `env/.env` alone (`LoadRealtimeBase`) and are never merged with the process environment: `db-slot env` emits those same three variables, so a resolver that read them back would suffix an already-suffixed name (`local_wt2` → `local_wt2_wt2`) the second time it ran in a shell that had exported its output. This is the one place the application's runtime-env-first rule (`config.Load`) deliberately does not apply, and an empty base is refused rather than passed on, because `realtimeName` will not suffix an empty base — the value emitted and the value shown would both name the main checkout's resource. The overlay refuses an empty value too (`:?`), but that error arrives from compose's interpolation stage and cannot name which key the embedded env is missing. One derivation feeds both `db-slot env` (what `make` eval's) and `db-slot status` (what a human reads), so the printed value can never disagree with the value actually used. `DB_LOCAL` / `DB_TEST` are additionally kept as `make` variables sourced from `.gobp-db-slot`, because target-specific assignments (`db-local-migrate-up: DB=$(DB_LOCAL)`) are evaluated at parse time, before any recipe could run.
- **Ownership guard** (`RequireOwner`) — the linked-worktree-without-a-lease check behind `make require-db-owner`, decided by the registry rather than by the presence of `.gobp-db-slot`. The distinction it rests on is three-valued, not two: **git absent** (tool-runner container) and **not a repository** pass through, while **a repository whose layout could not be read** fails. See [`docs/maintenance/db-worktree-pool.md`](../../../docs/maintenance/db-worktree-pool.md) for why.
- **Env guard** — refuses to run unless `APP_ENV` is empty or one of `local` / `ci` / `test` (`config.IsLocalClassEnv`); the pool creates and drops databases and must stay a dev/test-only tool. The check is an allowlist, so `dast` and any unrecognised value are refused too.

## Test Strategy

The parent layer's Testing Policy pushes every dependency behind a seam so the decision logic can be tested against doubles. That governs the decision logic here — but the seams' own implementations are adapters, and an adapter is only worth something if it drives the real thing. Each component is therefore tested at the tier its subject actually lives at:

- **`Pool`** (decision logic) — unit tests against the generated `MockDBAdmin` / `MockCompose`, reaching no Postgres and no docker. Every acquire / release / reclaim branch is pinned here, including the two-part in-use check whose whole point is that neither half suffices alone.
- **`Resolver`** (decision logic) — unit tests against a stub `GitProbe`, which is the only way to reach all four git contexts from one machine: a real host is always exactly one of them, and the dangerous case (a repository git cannot be read) does not occur on demand. A stub `LeaseProbe` does the same for the ownership branch, so held / reclaimed / unverifiable can each be reached without a registry on disk.
- **`Registry`** — real filesystem primitives under `t.TempDir()`. Faking the filesystem would prove nothing, because the subject *is* the atomicity of `os.Mkdir` and `os.Rename`; a double-lease is exactly what a fake would paper over.
- **`ExecCompose`** — a stub `docker` script prepended to `PATH` records the composed argument list and `COMPOSE_PROJECT_NAME`, pinning command construction and environment injection without running a real compose. `t.Setenv` on `PATH` makes these cases incompatible with `t.Parallel()`.
- **`PgxAdmin`** — the sole `DBAdmin` implementation, tested against the shared Postgres on `localhost:5432`. This is the only net proving its SQL actually executes; unreachable-host cases pin the error path without a server, and databases it creates are dropped in cleanup so runs stay repeatable.

The criterion is the subject, not the package: a component whose contract is a *decision* is tested against doubles, while a component whose contract is *the behaviour of an external substrate* is tested against that substrate. Tests here are consequently slower than pure unit-test packages and need the shared infra running.

## Environment variables

|Variable|Default|Description|
|---|---|---|
|`GOBP_DB_POOL_DIR`|`~/.cache/gobp-db-pool`|Lease registry location|
|`GOBP_DB_SHARED_PROJECT`|`gobp-shared`|Fixed compose project of the shared infra|
|`GOBP_DB_POOL_MAX`|`12`|Number of slots (max parallel worktrees)|
|`GOBP_DB_POOL_TTL`|`1800`|Heartbeat staleness grace (seconds)|
|`GOBP_API_POOL_BASE` / `GOBP_MOCK_AUTH_POOL_BASE`|`8080` / `2010`|Base host ports of the API / mock auth server (slot N = base + N)|
|`GOBP_DLV_POOL_BASE` / `GOBP_PPROF_POOL_BASE`|`2345` / `6060`|Base host ports of the dlv debug / pprof endpoints (slot N = base + N)|
|`GOBP_DB_POOL_PGHOST` / `GOBP_DB_POOL_PGPORT`|`localhost` / `5432`|Postgres the pool administers|
|`GOBP_DB_POOL_PGUSER` / `GOBP_DB_POOL_PGPASSWORD`|`postgres` / `postgres-password`|Credentials used for `CREATE DATABASE`|
|`GOBP_DB_POOL_PGMAINTDB`|`postgres`|Maintenance database connected to while creating / dropping|

## Notes

- A checkout without a slot still runs against the same shared infra — it just keeps the default `local` / `test` databases and the default host ports. Take a slot only when you need collision-free parallel work.
