# CLI core

`internal/cli` holds the **pure, testable core logic** for the application's CLI commands.

It does NOT depend on Cobra or infrastructure wiring. The Cobra command definitions and the
composition root that wires real dependencies (config / DB / DI / OS signals / golang-migrate)
live in `cmd/` (package `main`). This split keeps the core unit-testable and the wiring thin.

## Command List

|Command|Core package|Cobra shell|Description|
|---|---|---|---|
|`serve`|`server/`|`cmd/serve.go`|Start HTTP server and metrics server|
|`migrate-up`|`migrate/`|`cmd/migrate.go`|Upgrade DDL (`--steps` / `--database` options)|
|`migrate-down`|`migrate/`|`cmd/migrate.go`|Downgrade DDL (`--steps` / `--database` options)|
|`db-seed`|`seed/`|`cmd/seed.go`|Insert initial data into database|
|`job`|`job/`|`cmd/job.go`|Execute a registered job (`job <job-name> [args...]`)|
|`fix-collation`|`fixcollation/`|`cmd/fix_collation.go`|Fix PostgreSQL collation version mismatch|
|`dump-schema`|`dumpschema/`|`cmd/dump_schema.go`|Dump and format DB schema|
|`merge-dml`|`mergedml/`|`cmd/merge_dml.go`|Merge DML directory SQL files by type|
|`worker`|`worker/`|`cmd/worker.go`|Run a registered worker (`worker <worker-name> [args...]`)|
|`outbox-relay`|`outbox/`|`cmd/outbox_relay.go`|Run the outbox relay; `replay` subcommand returns dead rows to pending|
|`realtime-init`|`realtimeinit/`|`cmd/realtime_init.go`|Create the Realtime Delivery tables (EventLog / StreamTicket / InstanceLease) in the DynamoDB-compatible store and the fan-out topic on the SNS-compatible broker; idempotent one-shot, never run at application start|
|`db-slot`|`dbslot/`|`cmd/db-slot.go`|Lease a database slot from the shared worktree pool (`acquire` / `release` / `heartbeat` / `status` / `env` / `require-owner`)|

## Structure

One directory per subcommand, named after it, paired with the `cmd/<name>.go` that wires it.
The pairing is the point: `cmd/` stays a Cobra shell with a composition root and is excluded from
coverage, while the testable core lives here and is covered like any other package.

`registerCommands` in `cmd/commands.go` registers all subcommands to the Cobra root command.

## Design Policy

- Each command is one core package under `internal/cli/` + one thin shell file under `cmd/`.
- The core must NOT import Cobra, `internal/di`, OS signals, or infrastructure (except
  `infrastructure/rdb/driver` types). It operates on injected interfaces / function seams.
  `internal/config` is permitted — the enforced boundary is the `independent_cli` depguard rule in
  `.golangci-full.yaml`, which denies the layers above but not `config`.
- The CLI layer does not contain feature business logic (that belongs in usecase / domain).
- Adding a new command: add `cmd/<command>.go` (Cobra def + real-dependency wiring), add the core
  logic under `internal/cli/<command>/`, and register it in `registerCommands`.

## Testing Policy

The core is a **humble object**: all decision logic lives here and is unit-tested; the wiring is
pushed out to `cmd/`. The package boundary equals the test boundary.

- **No silently-wrong logic in the shell.** Any decision (error handling, branching, formatting,
  deletion conditions, timeout dispatch) lives in `internal/cli/*` and is **unit-tested for branch
  coverage**. `internal/cli/*` is included in the coverage gate and is expected to meet 90%+.
- **OS / filesystem / external-process / DB / logger dependencies are injected** (interfaces or
  function seams). Production wires the real implementations in `cmd/`; unit tests pass fakes, so
  **the decision logic's tests never touch the real filesystem, run external binaries
  (`pg_dump` / `psql`), or open a DB**. The **real implementation adapter behind a seam** is the one
  exception: its contract is that the external substrate behaves as expected, which a fake cannot
  establish, so it may be tested against the real thing. A package doing this declares the split in
  its own README — see [`dbslot/`](dbslot/README.md).
- **The thin `cmd/` shells are excluded** from the coverage gate (`gen|cmd|mock|apperror|scripts`).
  Their runtime correctness is covered by CI boot checks: `app-di-startup-check` (serve → `/ready`),
  `job-boot-check` (job dispatch), `worker-boot-check` (worker dispatch),
  `outbox-relay-boot-check` (relay start and drain), `migration-check`
  (up/down round-trip), `gen-*-artifacts-check` (codegen dogfooding) — all against a real Postgres
  service. DB access behaviour is covered by
  repository tests against a real Postgres (`internal/infrastructure/rdb/testkit`).

### When adding a command

- Keep the `cmd/` shell thin: parse flags → build dependencies → delegate to a core function.
- Bind flags to **local variables**, not package globals, so commands are parallel-test-safe.
- Inject OS / FS / exec / DB / logger through interfaces or params; provide the real implementations
  in `cmd/`.
- Unit-test the core for branch coverage; leave the thin `cmd/` shell to the CI gates.

## Notes

- Backup is recommended before running migration or seed operations.
- Server startup settings are managed via environment variables (see `internal/config`).
