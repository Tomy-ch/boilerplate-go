# OpenAPI Guide (`openapi/`)

This directory contains the **OpenAPI definitions** used in this project.

- Modular structure using Redocly
- Go code generation using oapi-codegen
- Boundary design aligned with Onion Architecture

## Directory Structure

The split is fixed by how the spec is assembled, not by convenience:

- `openapi.yaml` — the entry point; every other file is reached from here by `$ref`
- `openapi.gen.yaml` — the bundled result, generated and read by code generation (do not edit)
- `paths/` — endpoint definitions, laid out to mirror the URL structure
- `components/` — the reusable halves of a definition: `schemas/`, `parameters/`, `requests/`, `responses/`

Reference documents sit beside them as ordinary Markdown; each states its own subject.

## File Responsibilities

|File|Role|
|---|---|
|`openapi.yaml`|Entry point — references split files via `$ref`|
|`openapi.gen.yaml`|Bundled single file by Redocly — **input for oapi-codegen** (do not edit)|

## Code Generation

```bash
make gen-api    # Bundle OpenAPI + generate Go code
```

Generated components:

- Handler interfaces (`gen/server.gen.go`)
- Request/Response types (`gen/type.gen.go`)
- Validation spec (`gen/validate.gen.go`)

## Position in Architecture

```mermaid
flowchart TB
    OpenAPI["OpenAPI (contract)"] --> Controller["Controller (oapi-codegen)"] --> Usecase --> Domain
```

OpenAPI defines the **contract at the Controller boundary**. It specifies input/output formats and HTTP semantics, while Controller converts between OpenAPI types and application DTOs.

## Design Principles

### 1. Modular Structure (Redocly)

- Endpoints → `paths/`
- Data structures → `components/schemas/`
- Parameters → `components/parameters/`

### 2. `$ref` Uses Relative Paths

```yaml
# Recommended
$ref: '../components/schemas/UserResponse.yaml'

# Forbidden
$ref: '#/components/schemas/UserResponse'
```

Reason: compatibility with Redocly bundling.

### 3. One File = One Responsibility

- schema → one structure per file
- parameter → one definition per file
- path → one endpoint per file

### 4. Separation from Implementation

|Layer|Knows OpenAPI?|
|---|---|
|Controller|Yes — converts OpenAPI types ↔ DTO|
|Usecase|No — receives/returns DTO only|
|Domain|No — uses Entities and Value Objects|

## API Design Policy

### REST Design (Google API Design Guide)

- Resources are plural: `/users`
- CRUD via HTTP methods: `GET`, `POST`, `PATCH`, `DELETE`
- Non-CRUD actions: `POST /users/{id}:deactivate`

### Naming / Casing

A deliberate, per-location convention (enforced by `redocly lint`):

|Location|Casing|Example|
|---|---|---|
|Request / response **body fields**|`camelCase`|`firstName`, `postalCode`, `nextCursor`, `hasNext`, `requestId`|
|**Query / path parameters**|`camelCase`|`perPage`, `userId`|
|`operationId`|`PascalCase`, verb-first|`GetUsers`, `PostUsers`|

HTTP headers are out of scope for this table — they follow the conventional `Train-Case` (e.g. `Idempotency-Key`).

Body fields and parameters use the same `camelCase` casing on purpose: aligning parameters with body fields keeps the wire contract consistent with JS / TS frontends and generated SDKs. Keep each location internally consistent.

### Partial Update (PATCH) — Three-State Fields

A PATCH request body must distinguish three states per field: **not sent** (keep the current value), **sent as `null`** (clear the value), and **sent with a value** (replace it). The default oapi-codegen mapping generates `*T` for an optional nullable field, which collapses "not sent" and "null" into the same `nil` — a clear request becomes indistinguishable from an omit.

For a field that supports explicit-null clearing, override the generated type with the `x-go-type` extension and [`oapi-codegen/nullable`](https://github.com/oapi-codegen/nullable), whose `Nullable[T]` preserves all three states through standard `encoding/json` decoding:

```yaml
description:
  type: string
  nullable: true
  description: 説明。null を指定すると値をクリアします。
  x-go-type: nullable.Nullable[string]
  x-go-type-import:
    path: github.com/oapi-codegen/nullable
  x-go-type-skip-optional-pointer: true   # *Nullable[T] にしない（3 状態は型自身が表現する）
```

Rules:

- Apply this only to PATCH request fields where "clear" is a meaningful operation. Plain optional fields (where absent and null need no distinction) stay as the default `*T`.
- `x-go-type-import` always points at the `nullable` package — even when `T` needs another import (e.g. `time.Time`); oapi-codegen resolves `time` on its own, and declaring it here duplicates the import in the generated file.
- Per "Do not pass OpenAPI generated types to Usecase": convert `nullable.Nullable[T]` to the framework-agnostic three-state value (`pkg/patch.Field[T]`) at the controller boundary. Inner layers never see the generated type; the domain receives only resolved concrete values.

#### Collections as a three-state field

`T` may be a slice. A collection that is replaced as a whole — rather than merged element by element —
takes the same three states, with `null` meaning "remove every element":

```yaml
items:
  type: array
  nullable: true
  description: 送ると集合ごと置き換えます（差分更新ではありません）。null を指定すると全て取り除きます。
  items:
    $ref: './ItemInput.yaml'
  x-go-type: nullable.Nullable[[]ItemInput]
  x-go-type-import:
    path: github.com/oapi-codegen/nullable
  x-go-type-skip-optional-pointer: true
```

The element type is referenced by its generated name, so the `$ref` target must land in the same
generated package as the field that uses it.

Two things this shape is easy to get wrong:

- **State the replacement semantics in `description`.** `[]` and `null` both end up removing
  everything, so a caller cannot infer from the schema alone whether sending a shorter array deletes
  the missing elements or leaves them. Say it.
- **Do not act on an unspecified collection.** Resolving three states with `Resolve` alone cannot
  separate "not sent" from "sent as null" — both yield no value. Branch on
  `pkg/patch.Field.IsSpecified()` before touching the stored collection; otherwise every unrelated
  PATCH rewrites rows the request never mentioned.

### Versioning

URL path versioning: `/v1/<resources>`

Breaking changes → create `/v2/` alongside `/v1/`

### Security

- JWT (BearerAuth) for authenticated endpoints
- An opaque stream ticket in the query (`StreamTicket`, `apiKey` / `in: query`) for the SSE endpoint only — the browser `EventSource` cannot set headers (ADR-0074 (query-ticket-stream-authentication)). The ticket is short-lived, bound to one destination, stored hashed, and stripped from logs by `httpstack/redaction`; do not reuse the scheme for anything a Bearer token can protect
- Resource ownership validated via `sub` claim in Usecase/Middleware
- UUID as public identifiers — see `secure-uuid.md` for security evaluation
- IDOR protection required
- For OpenAPI-routed endpoints the `security:` declaration **is** the enforcement source of truth: the `oapi` middleware's `AuthenticationFunc` only fires for operations that declare it. **Exception:** `/metrics` is an ops path skipped from the OpenAPI validation pipeline, so its declared `BasicAuth` is documentation-only — the actual auth is a separate Echo `BasicAuth` middleware on that route.

## Prohibited Practices

- Do not include business logic in OpenAPI definitions
- Do not write schemas inline in path definitions
- Do not duplicate structures across files
- Do not expose DB column structures in API schemas
- Do not pass OpenAPI generated types to Usecase — convert to DTO

## Guides

- [parameter-guide.md](parameter-guide.md) — Parameter definition quick reference
- [secure-uuid.md](secure-uuid.md) — UUID exposure security evaluation
- [boundary-ownership.md](boundary-ownership.md) — Ownership of `min` / `max` / length constraints: an OpenAPI constraint is the **wire contract**, not the domain's business rule (the two may legitimately differ)

## Subdirectory Documentation

- [paths/README.md](paths/README.md) — Endpoint definitions and versioning
- [components/schemas/README.md](components/schemas/README.md) — Schema design policy
- [components/parameters/README.md](components/parameters/README.md) — Parameter conventions
- [components/requests/README.md](components/requests/README.md) — Request body semantics (content / required)
- [components/responses/README.md](components/responses/README.md) — Response semantics (status / description)
