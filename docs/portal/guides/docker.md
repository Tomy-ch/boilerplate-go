# docker

`docker/` is the directory for **Docker-related configuration files** required for development, building, and operations.

## Directory Structure

One directory per image or service, named after it; each holds that unit's Dockerfile and any
configuration it needs at build or run time.

## Compose Layering (infra / app)

Compose services are split into two layers, so the main checkout and any number of worktrees can run side by side.

|Layer|compose project|Services|
|---|---|---|
|**infra**|`gobp-shared` (fixed name) — **one instance for every checkout**|`database` / `observability` / `garage` (+ `garage_init`) / `elasticmq` / `dynamodb_local` (+ `dynamodb_init`) / `goaws`. The auxiliary services and tool runners default to the same project via `COMPOSE_PROJECT_NAME`|
|**app**|`APP_PROJECT` — one per checkout (`gobp-app-<directory name>`, or `gobp-wt-N` while a DB slot is held)|`api_server` / `mock_auth_server`|

The infra layer holds every service that can only run on a fixed host port, which is why it exists exactly once on the host.

|File|Role|
|---|---|
|`docker-compose.yaml`|Definitions of all services|
|`docker-compose.attach.yaml`|app layer override, **always** overlaid (`docker compose -f docker-compose.yaml -f docker-compose.attach.yaml`)|

`docker-compose.attach.yaml` publishes the `api_server` host ports as `${API_HOST_PORT:-8080}` / `${DLV_HOST_PORT:-2345}` / `${PPROF_HOST_PORT:-6060}`, narrows `depends_on` to `mock_auth_server` alone (the infra layer is already up), and points the app at the shared infra by overriding `DB_HOST=host.docker.internal` / `DB_NAME=${DB_NAME_LOCAL:-local}` / `ENDPOINT_OTLP=http://host.docker.internal:4318` / `ENDPOINT_OBJECT_STORAGE=http://host.docker.internal:3900` / `AUTH_ISSUER=http://localhost:${MOCK_AUTH_HOST_PORT:-2010}/default`. The provider needs no matching override: it derives the issuer from the `Host` it was reached through.

|Purpose|Reference|
|---|---|
|Layer variables (`INFRA_PROJECT` / `APP_PROJECT` / `COMPOSE_INFRA` / `COMPOSE_APP`)|[`.makefiles/docker/compose.mk`](../.makefiles/docker/compose.mk)|
|Local development topology as a whole|[`docs/maintenance/local-environment.md`](../docs/maintenance/local-environment.md)|
|Slot assignment of host ports / DB names|[`docs/maintenance/db-worktree-pool.md`](../docs/maintenance/db-worktree-pool.md)|

## Service Overview

The following table maps services defined in docker-compose.yaml to their corresponding Dockerfile / configuration.

### Development Environment (profile: `development`)

|Service|Layer|Dockerfile / Image|Port|Description|
|---|---|---|---|---|
|`api_server`|app|`docker/server/Dockerfile` (target: `tooling`)|`${API_HOST_PORT:-8080}`, `${DLV_HOST_PORT:-2345}`, `${PPROF_HOST_PORT:-6060}`|Development API server (hot reload via air)|
|`mock_auth_server`|app|`ghcr.io/navikt/mock-oauth2-server`|`${MOCK_AUTH_HOST_PORT:-2010}`|Mock OIDC provider (development only)|
|`database`|infra|`postgres:18.4-trixie`|5432|PostgreSQL database|
|`observability`|infra|`grafana/otel-lgtm`|3000, 4317, 4318, 3200|Local observability stack (OTLP endpoint / Grafana) for o11y verification|
|`garage`|infra|`dxflrs/garage`|3900, 3902|S3-compatible object storage (S3 API / Web API)|
|`garage_init`|infra|`docker/garage/Dockerfile`|-|One-shot provisioning of the garage layout / bucket / access key / website access (idempotent)|
|`elasticmq`|infra|`softwaremill/elasticmq-native`|9324|SQS-compatible message broker (local development; tests use an in-process fake)|
|`dynamodb_init`|infra|`amazon/dynamodb-local`|-|One-shot ownership fix of the `dynamodb_data` volume so `dynamodb_local` can stay non-root (idempotent; ordered by `depends_on`)|
|`dynamodb_local`|infra|`amazon/dynamodb-local`|8000|DynamoDB-compatible store for Realtime Delivery (EventLog / StreamTicket / InstanceLease) in local development (`-sharedDb`, persisted in the `dynamodb_data` volume)|
|`goaws`|infra|`admiralpiett/goaws` (config: `docker/goaws/goaws.yaml`)|4100|SNS / SQS emulator dedicated to Realtime Delivery's instance fan-out; the worker-side outbox queue stays on `elasticmq`|

The app layer host ports are the ones a DB slot shifts (`8080+N` / `2010+N` / `2345+N` / `6060+N`); the container-internal ports never move.

### Auxiliary Services (profile: `tools`, infra layer)

|Service|Dockerfile / Image|Port|Description|
|---|---|---|---|
|`docs_server`|`docker/document/Dockerfile`|2001|Documentation portal (nginx)|
|`sql_editor`|`sosedoff/pgweb`|2000|Web SQL editor|

### Tool Runners (profile: `generate`, infra layer)

|Service|Dockerfile / Image|Description|
|---|---|---|
|`go_tool_runner`|`docker/tools/Dockerfile` (target: `go_tools`)|Go code generation / linting / security / documentation tools ([list](tools/README.md#go_tools))|
|`node_tool_runner`|`docker/tools/Dockerfile` (target: `node_tools`)|OpenAPI bundling, Markdown / commit linting, portal build ([list](tools/README.md#node_tools))|
|`python_tool_runner`|`docker/tools/Dockerfile` (target: `python_tools`)|SQL linting ([list](tools/README.md#python_tools))|
|`er_diagram_generator`|`schemaspy/schemaspy`|ER diagram generation (SchemaSpy)|

## server

Application server images. One multi-stage Dockerfile produces three targets: `builder` (Go binary), `runtime` (production, non-root; migrations run from this same image via command override, so there is no dedicated migration image), and `tooling` (local hot-reload development). Base images and the tools each target carries: [`server/README.md`](server/README.md).

## tools

Tool containers for code generation and linting, split into three stages by language: `go_tools` / `node_tools` / `python_tools`. Which tools each stage carries and what each is for: [`tools/README.md`](tools/README.md).

## document

Container for the documentation portal.

- Base image: `nginx:1.31-alpine`
- Volume mounts the entire `docs/` directory
- Portal app is served at `/portal/`
- Accessing `http://localhost:2001/` redirects to `/portal/`

## garage

S3-compatible object storage for local development (tests use in-process gofakes3 instead).

- The server itself runs the official `dxflrs/garage` image. `garage_init` is the one that needs a build: the official image is `scratch`-based and has no shell, so the provisioning script cannot run in it — the Dockerfile copies the `garage` binary onto `alpine:3.24` for that
- `garage.toml`: single-node configuration (`replication_factor = 1`, S3 API `3900` / Web API `3902`), mounted read-only
- `init.sh`: idempotent provisioning run by the `garage_init` service (layout assignment → bucket creation → fixed access key import → bucket permission → website permission), mounted the same way so that editing it needs no image rebuild. It reads `OBJECT_STORAGE_*` straight from `env/.env` (passed via `env_file`) and fails immediately if one is unset, so the bucket and key can never drift from what the Go side connects with

### Public delivery (anonymous read)

The Web API (`3902`, Garage's `[s3_web]`) serves bucket objects **without credentials**, so a browser can load
<!-- sample-api:replace-begin -->
product images straight from the object storage, the way a CDN fronts S3 in production. Writing still goes through
`POST /v1/products/images` (BearerAuth + admin); only reading is open.
<!-- sample-api:replace-with -->
<!-- = objects straight from the object storage, the way a CDN fronts S3 in production. Writing still goes through -->
<!-- = an authenticated upload endpoint; only reading is open. -->
<!-- sample-api:replace-end -->

- Delivery origin: `http://gobp-local.web.garage.localhost:3902` — an object is `<origin>/<object key>`, e.g.
  `http://gobp-local.web.garage.localhost:3902/<prefix>/{uuid}.png`. This is the value the frontend puts in its
  media-origin setting; the API never returns a full URL, only the object key
- **Virtual-host addressing only.** Garage's web endpoint resolves the bucket from the `Host` header
  (`<bucket>.<root_domain>` or `<bucket>`), so path style (`localhost:3902/<bucket>/<key>`) does not work.
  macOS and the major browsers resolve `*.localhost` to `127.0.0.1` on their own, so no `/etc/hosts` entry is
  needed. From inside a glibc Linux container, which does not, either send the header explicitly
  (`curl -H 'Host: gobp-local' http://<host>:3902/<prefix>/...`) or add `127.0.0.1 gobp-local.web.garage.localhost`
  to `/etc/hosts`
- Listing stays closed: the web endpoint never lists, and an anonymous `ListObjects` against the S3 API is
  unsigned and rejected. Object keys carry a UUIDv7, whose ~74 random bits put enumeration out of reach —
  but note that the remaining bits are a millisecond timestamp, so a key is opaque, not secret. Treat
  anything in this bucket as world-readable to whoever holds the key
- **Website permission is per bucket, not per object** — every object in `gobp-local` becomes anonymously
  readable. The bucket holds nothing but publicly readable assets; storing non-public objects would require a second bucket

## elasticmq

SQS-compatible broker for local development (tests use an in-process fake instead). Only `elasticmq.conf` lives here; the queues and their DLQ redrive policy are read from it at startup, so there is no one-shot provisioning container. ElasticMQ does not expand environment variables, so the queue names are literal in that file — [`env/README.md`](../env/README.md) records which `*_QUEUE_URL` they pair with.

## goaws

SNS / SQS emulator for local development, used only by Realtime Delivery's instance fan-out ([`docs/design/realtime-delivery.md`](../docs/design/realtime-delivery.md)); the worker-side outbox queue stays on `elasticmq`. Only `goaws.yaml` lives here, and it fixes the port, region and account id — `000000000000`, the same account the ElasticMQ queue URLs carry, so every local ARN names one account. Topics and queues are not pre-declared: the application creates them at runtime, and that runtime creation is itself what `make realtime-smoke` verifies. `make realtime-smoke` is the authority on what the pinned GoAWS accepts — re-run it after bumping the image. At the pinned version the one call it refuses is the `Policy` queue attribute (`InvalidParameterValue` on both `SetQueueAttributes` and `CreateQueue`), so the local adapter has to skip the queue policy that production sets.

## mock-auth-server

Development OIDC provider. Nothing is built here — the service runs the upstream `ghcr.io/navikt/mock-oauth2-server` image, digest-pinned in [`images-pin.toml`](images-pin.toml), so this directory holds only the configuration handed to it.

- `config.json` is mounted read-only at `/etc/mock-oauth2-server/config.json` and passed via `JSON_CONFIG_PATH`. It declares the whole token contract: the `issuerId` (which becomes both the issuer's path segment and the JWKS `kid`), the `at+jwt` type header the resource server requires (RFC 9068), the `aud` / `azp` pair that lets one claim set satisfy both the resource server and an OIDC client (see [`docs/design/auth.md`](../docs/design/auth.md) § 3.3.1), and `${subject}`, which resolves to the login form's — or the password grant's — `username` and becomes the `sub` claim
- The internal port is always `4000` (`SERVER_PORT`); the process runs as a non-root UID from the upstream image
- The issuer is derived from the `Host` of the request that minted the token, so nothing has to declare it here — a token taken through the published host port carries an `iss` matching `AUTH_ISSUER`. `docker-compose.attach.yaml` only has to keep the API's `AUTH_ISSUER` on the slot's port
- Signing keys are generated at startup rather than checked in, so tokens are not reproducible across restarts. Nothing depends on them being reproducible: the resource server resolves keys from the JWKS at runtime, and the fixed keys the JWKS rotation test needs are its own (`internal/integration/testdata/`)

## database

### sql

Stores DB initialization SQL files.

- Executed via `docker-entrypoint-initdb.d` on PostgreSQL container startup
- Executed in order of filename numeric prefix (e.g., `001-...`, `002-...`)
- DDL (table definitions) should not be placed here; use `database/migrations/` instead

### schemaspy

Connection configuration for the ER diagram generator (SchemaSpy).

|File|Purpose|
|---|---|
|`schemaspy.properties`|Local environment (host: `database`)|
|`schemaspy-ci.properties`|CI environment (host: `localhost`)|

### sqlfluff

Configuration files for the SQL linter (sqlfluff). Different rules are applied per target.

|File|Target|Notes|
|---|---|---|
|`.dml.sqlfluff`|`database/dml/` (sqlc queries)|`@param` placeholder support, some rules excluded|
|`.migrations.sqlfluff`|`database/migrations/`|Max line length 150|
|`.seed.sqlfluff`|Seed data|Max line length 500|

Common rules

- Dialect: `postgres`
- Keywords: uppercase
- Identifiers: lowercase
- Functions / literals / types: uppercase
- `processes = 1` in all three. Parallel execution trips a CPython `resource_tracker` bug that surfaces as a
  `leaked semaphore` warning or an outright crash ([python/cpython#131788](https://github.com/python/cpython/issues/131788),
  [#142206](https://github.com/python/cpython/issues/142206)); revisit once those are fixed in the pinned interpreter
