# tools

`internal/usecase/tools` contains **small reusable utilities for the Usecase layer**.

## Subdirectories

|Package|Description|Details|
|---|---|---|
|`paging/`|Pagination (page/perPage → limit/offset conversion) and the shared fetch-count policy, top-N reads included|[README](paging/README.md)|
|`search/`|Search keyword tokenization (split, dedup, limit)|[README](search/README.md)|
|`money/`|Money arithmetic (integer minor-unit, half-up rate application)|[README](money/README.md)|
|`timewindow/`|Half-open ordered-time interval `[After, Before)` and its emptiness rule|[README](timewindow/README.md)|

## Design Policy

- Utilities shared across multiple Usecases
- No business logic — only mechanical transformations
- No Infrastructure dependencies
- Before adding a package here, derive its shape from the existing ones (`paging` / `search` / `money` /
  `timewindow`) — they share unexported fields, a validating constructor, and a call site in the handler.
  See *New Type Derivation* in [docs/rules.md](../../../docs/rules.md).
