# OpenAPI Parameters

`openapi/components/parameters/` stores **reusable OpenAPI parameter definitions** organized by concern.

## Directory Structure

One directory per parameter group. A group shared across endpoints is named after the concern
(`pagination/`, `search/`); one owned by a single resource is named after that resource.

## Pagination strategies

This project offers **two pagination strategies**, and each deliberately keeps its own idiomatic parameter names — they are **not** unified on purpose:

- **Offset** (`page` / `perPage`) — REST-style page navigation. Use for finite, page-addressable lists.
- **Cursor / keyset** (`first` / `after`) — Relay-style forward traversal. Use for large or infinite feeds where offset is too expensive or unstable.

Renaming the cursor params to `perPage` etc. would be non-idiomatic (cursor traversal has no "pages") and an API-breaking change, so the split is intentional.

## Filter conventions

These apply to every search endpoint, not only the ones that exist today.

### Multiple values for one condition repeat the parameter name

A condition that can hold several values is declared as an array and sent by repeating the name —
`categoryCodes=1&categoryCodes=2` — which is OpenAPI's default serialization (`style: form`,
`explode: true`). A parameter that is not declared as an array cannot express "either of these" at all:
a repeated name is rejected by request validation.

```yaml
explode: true
schema:
  type: array
  uniqueItems: true
  maxItems: 32
  items: { type: integer, format: int32, minimum: 1, maximum: 32767 }
```

`uniqueItems` and `maxItems` are wire limits: they bound the URL length and the size of the resulting `IN`
<!-- sample-api:replace-begin -->
list. Existing examples: `product/CategoryCodesParam.yaml`, `product/StatusCodesParam.yaml`, and
`purchase/PurchaseGroupByParam.yaml` (a string `enum` array whose order is significant — repetition
preserves it).
<!-- sample-api:replace-with -->
<!-- = list. A string `enum` array whose order is significant keeps that order across repetitions. -->
<!-- sample-api:replace-end -->

### Free-text input is never an array

A parameter that carries text the user typed is a single `string` with a `maxLength`, never an array —
<!-- sample-api:replace-begin -->
`search/KeywordParam.yaml` is the shape to copy. Only a reference to a row (`code`) or a fixed `enum` may
<!-- sample-api:replace-with -->
<!-- = Only a reference to a row (`code`) or a fixed `enum` may -->
<!-- sample-api:replace-end -->
be multi-valued.

This is what keeps the rule above cheap. Percent-encoded UTF-8 costs up to 9 characters per character, so
one free-text parameter dominates the URL budget by an order of magnitude over any array-serialization
choice; and a delimiter-separated array could not carry a value containing the delimiter, because the
separator is split after percent-decoding and therefore cannot be escaped. Restricting arrays to codes and
enums removes both problems at once.

A search that outgrows this — many facets, or values too large for a URL — belongs in a `POST` body, not in
a longer query string. See [ADR-0019 (search-query-parameter-shape)](../../../docs/adr/0019-search-query-parameter-shape.md).

### Filtering by master data takes `code`, not the row's UUID

The identifier a client sends for a master row is its `code` — a static alias fixed by the migration that
inserted the row. Which UUID that row carries is decided by the migration, so application code must not
hold it ([ADR-0031](../../../docs/adr/0031-master-data-via-migration.md)); the same reasoning applies at the
API boundary. A client can keep `code` as a constant, whereas a UUID has to be resolved by calling the
master endpoint first, and at 36 characters each it makes a multi-value filter expensive to express.

A code matching no master row yields **zero results**, not a 404 — it is a filter, not a lookup.

### `code` is an int16 range

`type: integer` / `format: int32` / `minimum: 1` / `maximum: 32767`. OpenAPI has no int16, so the range is
carried by the bounds and narrowed in Go through `pkg/safecast`. This matches the `SMALLINT` column, the
sqlc-generated `int16`, and the domain's own `minCode` / `maxCode`.

## Usage

Reference parameters in endpoint definitions using `$ref`:

```yaml
parameters:
  - $ref: '../components/parameters/pagination/PageParam.yaml'
  - $ref: '../components/parameters/pagination/PerPageParam.yaml'
  - $ref: '../components/parameters/search/KeywordParam.yaml'
```

## Naming Convention

|Element|Convention|Example|
|---|---|---|
<!-- sample-api:replace-begin -->
|Directory|lowercase by concern|`pagination/`, `search/`, `stream/`, `user/`|
|File name|PascalCase + `Param`|`PageParam.yaml`, `UserIdParam.yaml`|
<!-- sample-api:replace-with -->
<!-- = |Directory|lowercase by concern|`pagination/`, `<resource>/`| -->
<!-- = |File name|PascalCase + `Param`|`PageParam.yaml`, `<Resource>IdParam.yaml`| -->
<!-- sample-api:replace-end -->

## Rules

- `in: path` parameters must always set `required: true`
- Include `description` and `example` on all parameters for documentation quality
- Use `minimum`, `maximum`, `default` where appropriate for query parameters
- One parameter per file for maximum reusability
