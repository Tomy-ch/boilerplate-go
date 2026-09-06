# pkg

`pkg/` is the directory for **shared utility packages** used across the entire application.

## Policy

`pkg/` only accepts new packages when they meet the following criteria:

- The functionality is **referenced from multiple locations**
- The package **wraps an external library** so that application code does not depend on it directly

Helpers used by only one feature should be placed within that feature's package.

Packages that perform external I/O (e.g. `exec`, `fs`) follow a common
shape: define an **interface** for the capability, provide a concrete implementation
(`OS{}` etc.) that wires the real dependency, and add a
`//go:generate mockgen` directive so callers can inject a mock in tests.

### `pkg/` vs application-wide cross-cutting concerns

"Referenced from multiple locations" alone is **not** sufficient. `pkg/` is for
**context-independent, generic utilities** — code that could be lifted into any
project unchanged and carries no knowledge of this application's domain or system
decisions (e.g. `xerrors`, `uuid`, `ptr`, `stringkit`).

Concerns that are cross-cutting but **specific to this application/system** — the
application-wide error taxonomy (`internal/apperror`), logging (`internal/logging`),
observability (`internal/observability`), configuration (`internal/config`) — do
**not** belong in `pkg/` even though they are used across layers. They encode this
system's choices (error semantics, frameworks such as zap / otel) and therefore live
under `internal/` as cross-cutting concerns. The domain layer may depend on
`internal/apperror` as the one permitted exception among these.

### Constraints

- Must not contain business logic
- Must not depend on `internal/` packages
- Must not depend on infrastructure or framework-specific packages
- Must not depend on other `pkg/` packages. Two exceptions are permitted, both enforced by depguard `independent_pkg` in `.golangci-full.yaml`: `pkg/xerrors` may be imported by any package, and a `testkit` sub-package may import its own parent (the rule's file pattern excludes `**/pkg/**/testkit/**.go`)
- Each package must have a single responsibility

### Doc comments must stay context-independent too

The constraints above govern the doc comments, not just the code. A `pkg/` package is meant to survive
being copied into another project, so its doc comments must not bake in **this** application's context:
no specific environment-variable names, no naming of the current call sites, and no examples that
mirror this repository's layer structure. Write `envutil.Override("SOME_KEY", "value")`, not a real
`DB_NAME`; say a retry loop is shared by "any caller that classifies retryability", not by "the tx
retry and the external HTTP retry". Naming the current consumers is also plain noise under
[`docs/rules.md`](../docs/rules.md) § Comment Rules ("where it is called from").

Conversely, the contract itself must be **complete**, because a generic utility is read without the
surrounding application to fall back on: mutation of pointer arguments, `nil` semantics, silent
clamping of out-of-range inputs, and units all belong in the doc comment.

## Package List

|Package|Summary|Wraps|
|---|---|---|
|`backoff`|Exponential backoff duration (pure, clock/randomness-free)|None|
|`datetime`|Date/time parsing|Standard library `time`|
|`decimal`|Exact-decimal type (money / rate)|`github.com/shopspring/decimal`|
|`envutil`|Environment variable override (test helper)|Standard library `os`|
|`exec`|External command execution (interface + mock)|Standard library `os/exec`|
|`fnmeta`|Function / package name extraction|None|
|`fs`|Filesystem operations (interface + mock)|Standard library `os`|
|`httpheader`|Classification of HTTP header names (credential-carrying or not)|None|
|`patch`|Three-state values for partial-update (PATCH) input|None|
|`ptr`|Pointer operations|None|
|`retry`|Bounded-retry behavior layer (backoff + full jitter, deadline-aware)|None|
|`safecast`|Type conversion with overflow detection|None|
|`stringkit`|String length validation|None|
|`uuid`|UUID type|`github.com/google/uuid`|
|`xerrors`|Errors with stack traces|`github.com/cockroachdb/errors`|

## Package Details

### backoff

Computes exponential backoff wait durations as a pure function of the attempt count, free of clock or randomness (the jitter step lives in `retry`).

|Symbol|Description|
|---|---|
|`Exponential` (struct)|`Initial` / `Max` / `Multiplier` configuration|
|`Duration(attempt)`|Return the base wait duration for the given attempt|

### datetime

A parsing utility supporting multiple date/time formats.

Key functions

|Function|Description|
|---|---|
|`ParseRFC3339`|Parse RFC3339 format|
|`ParseRFC3339UTC`|Parse RFC3339 format (UTC)|
|`ParseRFC3339Nano`|Parse RFC3339Nano format|
|`ParseISO8601`|Parse ISO8601 format|
|`ParseDateTime`|Parse standard datetime format|
|`ParseDateOnly`|Parse date-only format|
|`ParseCustomLayout`|Parse with an arbitrary layout|

All functions have `ToLocation` variants (e.g. `ParseRFC3339ToLocation`) for parsing with a specified timezone.

### decimal

An exact-decimal type wrapping `github.com/shopspring/decimal`, hiding the vendor behind a seam (the `pkg/uuid` precedent). Carries no money semantics — currency / non-negativity / minor-unit choice belong to the caller; this package is pure decimal arithmetic, rounding, scaling, and the DB / wire boundary. Wire representation is a JSON string, because a JSON number is decoded as an IEEE754 double and loses precision.

|Symbol|Description|
|---|---|
|`Parse` / `FromInt`|Construct from a decimal string / `int64`|
|`Add` / `Sub` / `Mul` / `Neg` / `DivRound`|Exact decimal arithmetic|
|`RoundHalfAwayFromZero` / `Truncate`|Rounding at a given number of places|
|`ToScaledInt64(n)`|Round to `n` places, scale by `10^n`, and return the minor-unit `int64` (or `ErrOverflow`)|
|`Cmp` / `Equal` / `Sign` / `IsZero` / `IsNegative`|Comparison and inspection|
|`MarshalJSON` / `UnmarshalJSON`|JSON string wire representation (accepts JSON number on decode)|
|`Scan` / `Value`|`NUMERIC` database boundary (`sql.Scanner` / `driver.Valuer`)|

Test helpers live in the separate package `pkg/decimal/testkit` (`MustParse`), so `testing` is never
linked into a production binary.

### envutil

Temporarily overrides an environment variable and returns a restore function (mainly for tests / config loading).

|Function|Description|
|---|---|
|`Override(key, value)`|Set an env var and return a `func()` that restores the previous state|

### exec

Abstracts external command execution behind an interface so callers can inject a mock in tests. Production wires the `OS{}` implementation.

|Symbol|Description|
|---|---|
|`Runner` (interface)|`Output(ctx, dir, env, name, args)` — run a command and return stdout|
|`OS` (struct)|`os/exec`-based implementation of `Runner`|

### fnmeta

Decomposes full function names obtained from `runtime` to extract package and function names.

|Function|Description|
|---|---|
|`ExtractFunctionName`|Extract method name from full function name|
|`ExtractPackageName`|Extract package name from full function name|

### fs

Abstracts filesystem operations behind an interface so callers can inject a mock in tests. Production wires the `OS{}` implementation.

|Symbol|Description|
|---|---|
|`FS` (interface)|`ReadFile` / `WriteFile` / `Glob`|
|`OS` (struct)|`os`-based implementation of `FS`|

### patch

Three-state values for partial-update (PATCH) input. Distinguishes "not sent (keep current)", "sent as `null` (clear)", and "sent with a value (replace)" — a distinction a plain `*T` collapses into `nil`. The zero value of `Field[T]` is unspecified, so a struct of `Field` values defaults to "change nothing".

|Symbol|Description|
|---|---|
|`Field[T]` (struct)|One field's specification state in a partial update|
|`Unspecified[T]` / `Null[T]` / `Value[T]`|Constructors for the three states|
|`Field[T].Resolve`|Apply the specification to a current value|

### ptr

Pointer manipulation utilities using generics.

|Function|Description|
|---|---|
|`To[T]`|Create a pointer from a value|
|`Copy[T]`|Copy a pointer (nil-safe)|
|`Map[T,U]`|Apply a function to the pointed-to value, preserving nil|
|`Deref[T]`|Dereference a pointer, returning a fallback when nil|

### retry

A bounded-retry behavior layer that consumes a failure classification (`classify → bounded attempts → backoff + full jitter → deadline-aware`). Keeps `backoff` pure by confining the randomness (full jitter) here.

|Symbol|Description|
|---|---|
|`Do`|Run a function with bounded retries while a classifier marks the error retryable|
|`Full`|Full jitter — uniform random duration in `[0, d]`|
|`Policy`|`MaxAttempts` + `Backoff` (`func(attempt int) time.Duration`)|
|`Sleeper` (interface)|`Sleep(ctx, d)` wait abstraction (satisfied structurally by the caller's own sleeper type)|

### safecast

Provides safe type conversion with overflow detection.

|Function|Description|
|---|---|
|`UintToInt`|Safe conversion from `uint` to `int`|
|`IntToInt32`|Safe conversion from `int` to `int32`|
|`IntToInt16`|Safe conversion from `int` to `int16`|
|`IntPtrToInt32Ptr`|Safe conversion from `*int` to `*int32` (`nil` means nothing to convert and returns `nil`)|

Returns `ErrOverflow` when an overflow occurs.

### stringkit

A set of validation functions based on string length (rune count).

|Function|Description|
|---|---|
|`RuneCount`|Return UTF-8 rune count|
|`InRange`|Check if length is within closed interval|
|`MaxOrLess`|Check if length <= max|
|`MinOrMore`|Check if length >= min|
|`StrictInRange`|Check if length is within open interval|
|`LessThanMax`|Check if length < max|
|`GreaterThanMin`|Check if length > min|
|`ValidateInRange`|Check closed-interval length and also return the error message|

Each function has a corresponding `ErrorMsg` function for generating validation error messages.

### uuid

A UUID type wrapping `github.com/google/uuid`.

Test helpers live in the separate package `pkg/uuid/testkit` (`NewTestFromSalt`), so `testing` is never
linked into a production binary.

Generates UUIDv7 and supports database integration (`sql.Scanner` / `driver.Valuer`).

|Function / Method|Description|
|---|---|
|`New`|Generate UUIDv7|
|`Parse`|Parse UUID from string|
|`NewTestFromSalt`|Generate deterministic UUID for testing|
|`String`|Return string representation|
|`IsNil`|Check if zero value|
|`Equal`|Compare UUIDs|
|`EqualPtr`|Compare against a `*UUID` (nil-safe)|
|`Bytes`|Return the raw `[16]byte`|
|`ToPtr`|Return a pointer to the value|
|`ToPrimitive` / `FromPrimitive`|Convert to / from `github.com/google/uuid` (e.g. sqlc integration)|
|`MarshalJSON` / `UnmarshalJSON`|JSON string wire representation (rejects a non-string value; `null` is a no-op)|
|`Scan` / `Value`|DB integration interface implementation|

### xerrors

Wraps `github.com/cockroachdb/errors` to provide error operations with stack traces.

|Function|Description|
|---|---|
|`New`|Create a new error|
|`Wrap`|Wrap an existing error|
|`Is`|Check error identity|
|`As`|Type-assert an error|
|`Join`|Combine multiple errors|
|`StackTrace`|Get stack trace string|

## What a sub-package `README.md` carries

Every `pkg/<name>/` has its own `README.md`, and it is the file a reader opens to decide whether the
package does what they need. It carries these sections, in this order; omit one only when the package
genuinely has nothing to say under it.

| Section | What goes in it |
| --- | --- |
| `Role` | One paragraph: what this package is for, and where the boundary of its responsibility sits. |
| `Public API` | Every exported symbol, with its signature and a one-line contract. This is the section drift is checked against, so an exported symbol missing here reads as undocumented. |
| `Wraps` | The external package being wrapped, and what this package adds or hides. Omit when the package wraps nothing. |
| `Notes` | Pitfalls a caller cannot see from the signature — allocation behavior, precision limits, goroutine safety, what happens on a zero value. |

Anything beyond those four is the package's own call. What is **not** optional is `Public API`:
`drift-detector-pkg` compares it against the package's exported symbols, so a package that names the
section something else, or drops it, cannot be checked.

## Checklist for Adding a New Package

- [ ] Referenced from multiple locations, or wraps an external package
- [ ] Does not contain business logic
- [ ] Does not depend on `internal/`
- [ ] Single responsibility per package
- [ ] Tests are written
- [ ] Documentation is written
- [ ] Package summary is added to this README
- [ ] The sub-package `README.md` carries `Role` / `Public API` (+ `Wraps` / `Notes` when they apply)
