# OpenAPI Paths

`openapi/paths/` stores **API endpoint route definitions** organized by resource and version.

## Directory Structure

One file per path, named after its last segment. A path with children gets a directory of the
same name beside its own file, so the directory layout mirrors the URL structure.

## Endpoint Categories

### Operational Endpoints

Infrastructure endpoints for monitoring and orchestration. These are not part of the business API and are excluded from OpenAPI validation and authentication via the `Skipper`.

|Path|File|Description|
|---|---|---|
|`/health`|`health.yaml`|Health check|
|`/healthz`|`healthz.yaml`|Kubernetes liveness probe|
|`/ready`|`ready.yaml`|Readiness probe|
|`/version`|`version.yaml`|Build version / revision / date|
|`/metrics`|`metrics.yaml`|Prometheus metrics (Basic auth protected)|

### Versioned API (`v1/`)

Business API endpoints following a **URL versioning strategy**.

<!-- sample-api:replace-begin -->
```text
/v1/users             → users.yaml
/v1/users/{userId}    → users/userId.yaml
/v1/users/me          → users/me.yaml
/v1/users/search      → users/search.yaml
```
<!-- sample-api:replace-with -->
<!-- = ```text -->
<!-- = /v1/<resources>             → <resources>.yaml -->
<!-- = /v1/<resources>/{<id>}      → <resources>/<id>.yaml -->
<!-- = /v1/<resources>/me          → <resources>/me.yaml -->
<!-- = /v1/<resources>/search      → <resources>/search.yaml -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

> `v1/` contents are **sample implementations**. Replace with your own resources when building a service.

One path under `v1/` is not a sample: `/v1/streams/{destination}` (`v1/streams/destination.yaml`) is the generic SSE endpoint of the Realtime Delivery mechanism. It survives sample removal, is authenticated by the `StreamTicket` query scheme instead of Bearer, and is the only operation generated without `strict-server` — its handler lives in `internal/controller/stream/`, beside `handler/`, not under it (see that package README and `docs/design/realtime-delivery.md` §3.1).

#### Versioning Strategy

This project recommends **URL path versioning** (`/v1/`, `/v2/`, etc.):

- Clear and explicit in URLs and documentation
- Allows parallel operation of multiple API versions
- Breaking changes are introduced in a new version prefix
- Non-breaking additions can be made within the current version

When introducing breaking changes, create a new `v2/` directory alongside `v1/`.

### Internal Types

`internal/types/error_response.yaml` defines the error response type used by `oapi-codegen` for type generation. This is not a public API endpoint.

## Naming and Structure Rules

<!-- sample-api:replace-begin -->
|Element|Convention|Example|
|---|---|---|
|File name|Match the URL segment (lowercase); path-parameter files match the param's `camelCase` name|`search.yaml`, `userId.yaml`|
|Directory layout|Mirror the URL path (one file per path item)|`/v1/users/search` → `v1/users/search.yaml`|
|Leaf vs. parent|Leaf = flat `<segment>.yaml`; endpoint-with-children = `<segment>.yaml` beside `<segment>/`|`users.yaml` + `users/`|
|Path parameter|File name matches the param's `camelCase` name (no braces)|`{userId}` → `userId.yaml`|
|operationId|`{HTTPMethod}{Resource}` (PascalCase, verb-first)|`GetUsers`, `PostUsers`|
|tags|Path-based grouping|`v1/users`, `health`|
<!-- sample-api:replace-with -->
<!-- = |Element|Convention|Example| -->
<!-- = |---|---|---| -->
<!-- = |File name|Match the URL segment (lowercase); path-parameter files match the param's `camelCase` name|`search.yaml`, `<id>.yaml`| -->
<!-- = |Directory layout|Mirror the URL path (one file per path item)|`/v1/<resources>/search` → `v1/<resources>/search.yaml`| -->
<!-- = |Leaf vs. parent|Leaf = flat `<segment>.yaml`; endpoint-with-children = `<segment>.yaml` beside `<segment>/`|`<resources>.yaml` + `<resources>/`| -->
<!-- = |Path parameter|File name matches the param's `camelCase` name (no braces)|`{<id>}` → `<id>.yaml`| -->
<!-- = |operationId|`{HTTPMethod}{Resource}` (PascalCase, verb-first)|`Get<Resources>`, `Post<Resources>`| -->
<!-- = |tags|Path-based grouping|`v1/<resources>`, `health`| -->
<!-- sample-api:replace-end -->

## Path-to-Handler Mapping

Path definitions correspond to handler implementations:

<!-- sample-api:replace-begin -->
```text
paths/v1/users.yaml             → handler/v1/users/
paths/v1/users/userId.yaml      → handler/v1/users/detail/
paths/v1/users/me.yaml          → handler/v1/users/detail/
paths/v1/users/search.yaml      → handler/v1/users/search/
```
<!-- sample-api:replace-with -->
<!-- = ```text -->
<!-- = paths/v1/<resources>.yaml             → handler/v1/<resources>/ -->
<!-- = paths/v1/<resources>/<id>.yaml        → handler/v1/<resources>/detail/ -->
<!-- = paths/v1/<resources>/me.yaml          → handler/v1/<resources>/detail/ -->
<!-- = paths/v1/<resources>/search.yaml      → handler/v1/<resources>/search/ -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

## Rules

- Use `$ref` to reference schemas, parameters, and responses — do not define inline
- Split path files by responsibility (list vs. detail vs. search)
- Do not use `#/` fragment pointers in this directory
- Align naming with `components/` for easy cross-referencing
- Each path file should define a single route
