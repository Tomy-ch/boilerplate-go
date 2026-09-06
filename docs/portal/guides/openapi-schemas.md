# OpenAPI Schemas

`openapi/components/schemas/` stores **reusable OpenAPI schema definitions** — data structures for requests, responses, and security schemes.

## Directory Contents

|File|Type|Description|
|---|---|---|
|`ErrorResponse.yaml`|Response|Unified error response schema (code / message / details / requestId)|
|`errors/`|Response objects|Reusable error responses — one per HTTP status covering every `apperror` kind (`<Reason><Code>.yaml`), wrapping `ErrorResponse`|
|`PaginationMetadataResponse.yaml`|Response|Offset pagination metadata (total / limit / offset)|
|`CursorPaginationMetadataResponse.yaml`|Response|Cursor (keyset) pagination metadata (nextCursor / hasNext)|
|`BasicAuth.yaml`|Security|HTTP Basic authentication scheme|
|`BearerAuth.yaml`|Security|HTTP Bearer (JWT) authentication scheme|
|`StreamCursor.yaml`|Value|Stream position as a decimal string (SSE `id`, `after`, `Last-Event-ID`)|
|`DeliveryEvent.yaml`|Event|Feature-neutral SSE business-event envelope|
|`ControlEvent.yaml`|Event|In-band SSE control instruction (`action` / `reason` / `retryAfterMs`)|
|`UserBaseInputRequest.yaml`|Request|User input fields (sample)|
|`UserResponse.yaml`|Response|User response fields (sample)|

> `User*` files are **sample implementations**. Use them as reference for naming and structure conventions when building your own schemas.

## Design Policy

### One File = One Schema

Each file defines a single top-level schema. Do not nest multiple schemas in one file.

```yaml
# Good: UserResponse.yaml
type: object
required:
  - name
  - email
properties:
  name:
    type: string
  email:
    type: string
    format: email
```

### Naming Convention

|Element|Convention|Example|
|---|---|---|
|File name|PascalCase|`ErrorResponse.yaml`, `UserBaseInputRequest.yaml`|
|Request schemas|`*Request.yaml`|`UserBaseInputRequest.yaml`|
|Response schemas|`*Response.yaml`|`UserResponse.yaml`, `ErrorResponse.yaml`|
|Security schemes|Descriptive name|`BasicAuth.yaml`, `BearerAuth.yaml` (the `StreamTicket` scheme is declared inline in `openapi.yaml` so a Spectral override can target it by JSON pointer)|

### Payloads are schemas, organized by role across three folders

Because of `redocly` bundling and `oapi-codegen` generation constraints, every request / response payload is modeled as a **schema** — never as the OpenAPI `requestBodies` / `responses` **component object** types. A path references these schemas directly under `content.<media>.schema.$ref`.

They are split across three folders **by role**, not by kind:

|Folder|Holds|Example|
|---|---|---|
|`schemas/`|Base & reusable schemas + security schemes|`UserResponse.yaml`, `ErrorResponse.yaml`, `PaginationMetadataResponse.yaml`|
|`requests/`|Endpoint **request-body** schemas (usually compose a base via `allOf`)|`UsersPostRequest.yaml` = `UserBaseInputRequest` + a `required` list|
|`responses/`|Endpoint **response-body** schemas (usually compose a base via `allOf`)|`UsersResponse.yaml` = `UserResponse[]` + pagination metadata|

Rule of thumb: a small reusable building block lives in `schemas/`; the per-endpoint shape that composes those blocks lives in `requests/` or `responses/`. See [`requests/README.md`](../requests/README.md) and [`responses/README.md`](../responses/README.md).

### $ref Usage

All `$ref` references use **relative YAML paths** (not `#/components/...` fragment format) for compatibility with `redocly bundle`.

```yaml
# From a path definition
schema:
  $ref: '../components/schemas/UserResponse.yaml'
```

### PATCH Support

For PATCH operations, create a dedicated wrapper schema using `allOf`:

```yaml
# UserPatchRequest.yaml
allOf:
  - $ref: './UserBaseInputRequest.yaml'
```

This separates the input structure from the operation semantics.

`additionalProperties: false` stays on `UserBaseInputRequest.yaml`, which declares the properties.
Putting it on the wrapper instead rejects **every** field: `additionalProperties` only sees properties
declared in the same schema object, and the wrapper declares none.

## Core Schemas

### ErrorResponse / ErrorResponseWithDetails

Two error envelopes. `ErrorResponse` is the base (no `details`) used by most error statuses;
`ErrorResponseWithDetails` adds `details` and is referenced **only** by responses that
intentionally expose it. Which operations reference `ErrorResponseWithDetails` is the
**per-endpoint opt-in switch** for detail exposure (enforced fail-closed at the edge — see
[ADR-0049 (error-details-opt-in-gate)](../../../docs/adr/0049-error-details-opt-in-gate.md)).

```yaml
# ErrorResponse.yaml (base)
type: object
required: [code, message, requestId]
properties:
  code:       # Machine-readable error code (e.g., BAD_REQUEST)
  message:    # Human-readable error message
  requestId:  # Request tracking ID

# ErrorResponseWithDetails.yaml (base + details)
#   ...same fields, plus:
#   details:  # Public-safe identifiers (e.g., invalid field names)
```

The Go builder (`response.HTTPErrorResponse`) embeds the `ErrorResponseWithDetails` superset —
see `internal/controller/error/response/`.

### errors/ — reusable error response objects (DRY)

Every operation returns the same `ErrorResponse` body for its error statuses. Instead of repeating the full block in each path, `schemas/errors/` holds **one reusable response object per HTTP status — covering every `apperror` kind** — and a path references the whole status entry:

```yaml
# in a path
responses:
  '401':
    $ref: '../../../components/schemas/errors/Unauthorized401.yaml'
```

```yaml
# schemas/errors/Unauthorized401.yaml
description: 認証が必要です。
content:
  application/json:
    schema:
      $ref: '../ErrorResponse.yaml'
```

These are technically OpenAPI **response objects** (they carry `description` + `content`, which a plain schema cannot), kept here next to `ErrorResponse` so all error definitions live together. `redocly bundle` hoists each into `#/components/responses/<FileName>`, which `oapi-codegen` turns into a `<FileName>JSONResponse` Go type — so **the file name must be a valid Go identifier (PascalReason + HTTP-code suffix, never a bare number)**. Keep a status **inline** only when its description is operation-specific (i.e. the wording is meaningful only for that one operation and cannot be shared).

**The full set (one per `apperror` kind).** Every fragment exists so it is ready to `$ref` the moment an endpoint needs it. A path declares **only the statuses that operation can actually produce** (derived from `internal/controller/error/response/http_error.go` + `internal/infrastructure/rdb/pgerror`):

|Fragment|Status|`apperror`|Reached by|
|---|---|---|---|
|`BadRequest400`|400|`ErrInvalidArgument`|OpenAPI request validation (param/body schema violation)|
|`Unauthorized401`|401|`ErrUnauthenticated`|auth middleware|
|`Forbidden403`|403|`ErrPermissionDenied`|auth middleware|
|`NotFound404`|404|`ErrNotFound`|missing resource|
|`Conflict409`|409|`ErrConflict`|`ErrAlreadyDeleted` (delete) or unique-violation `23505` (create/update, e.g. duplicate email)|
|`PayloadTooLarge413`|413|`ErrPayloadTooLarge`|usecase validation of an upload that exceeds the size limit|
|`UnsupportedMediaType415`|415|`ErrUnsupportedMediaType`|usecase validation of a disallowed `Content-Type`|
|`UnprocessableEntity422`|422|`ErrValidation`|domain validation the OpenAPI schema does not catch (e.g. email format)|
|`TooManyRequests429`|429|`ErrTooManyRequests`|rate limiting|
|`ClientClosedRequest499`|499|`ErrCanceled`|client disconnect mid-request|
|`InternalServerError500`|500|`ErrInternal`|unexpected server error|
|`NotImplemented501`|501|`ErrUnimplemented`|unimplemented operation|
|`ServiceUnavailable503`|503|`ErrUnavailable`|DB transient errors (`40001`/`40P01`/`57014`/connection) via `pgerror`|

A fragment not referenced by any operation is not included by `redocly bundle`, so `no-unused-components` does not flag it. When a code path starts producing that status, wire it up by adding a `'<code>': { $ref: ... }` entry to the operation's `responses`.

### PaginationMetadataResponse

**Offset** pagination metadata returned with list endpoints:

```yaml
type: object
required: [total, limit, offset]
properties:
  total:   # Total item count
  limit:   # Items per page
  offset:  # Current offset
```

### CursorPaginationMetadataResponse

**Cursor (keyset)** pagination metadata — the alternative strategy to offset:

```yaml
type: object
required: [nextCursor, hasNext]
properties:
  nextCursor:  # Opaque cursor for the next page; null on the last page
  hasNext:     # Whether a next page exists
```

Reuse pattern: the cursor pieces are **resource-agnostic and shared**. To add a cursor-paginated endpoint, you do **not** create new pagination components — reuse the existing ones:

- Query parameters: `parameters/pagination/CursorAfterParam.yaml` (`after`) + `parameters/pagination/CursorFirstParam.yaml` (`first`)
- Response: compose the item array with this metadata via `allOf` in a per-resource wrapper. Only that wrapper is feature-specific:

```yaml
# responses/users/UsersFeedResponse.yaml
allOf:
  - type: object
    required: [users]
    properties:
      users:
        type: array
        items:
          $ref: '../../schemas/UserResponse.yaml'
  - $ref: '../../schemas/CursorPaginationMetadataResponse.yaml'
```

## Rules

- Avoid defining schemas inline in path definitions — always extract to `schemas/`
- Do not make schemas serve both request and response purposes
- Include `description` and `example` on all properties
- Use `required` to explicitly list mandatory fields
- Declare `additionalProperties: false` on the request schema object that holds the properties — never on an `allOf` wrapper, which cannot see them (see [PATCH Support](#patch-support))
- Boundary values like `maxLength` are a **wire contract**, not the domain's business rule (different owner) — see [Input Boundary Value Ownership](../../boundary-ownership.md)

## Checklist

- [ ] One file = one schema
- [ ] File name matches schema purpose (PascalCase)
- [ ] `$ref` uses relative path format
- [ ] PATCH uses a dedicated wrapper schema
- [ ] `description` and `example` on all properties
