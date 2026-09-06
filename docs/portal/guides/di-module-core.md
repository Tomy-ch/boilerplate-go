# core module

`internal/di/module/core` provides **DI module groups for core components** commonly used in the HTTP stack.

Each module returns an `fx.Option` that registers the corresponding component in the DI container.

## Module List

|Function|File|Provided Component|
|---|---|---|
|`AuthnModule()`|`auth.go`|Authentication (Authenticator + Auth controller)|
|`BasicAuthModule()`|`basicauth.go`|Basic auth validator for metrics endpoint|
|`SecurityCookieModule()`|`security_cookie.go`|Cookie security attribute configuration|
|`SkipperModule()`|`skipper.go`|Skip OpenAPI validation for ops endpoints|
|`ValidatorModule()`|`validator.go`|OpenAPI schema validator|
|`RedactionModule()`|`redaction.go`|Log redactor for query credentials, derived from the OpenAPI security schemes|

## Design Policy

- Each module is isolated as one file = one module
- Internal implementation simply wraps constructors from `internal/controller/httpstack` etc. with `fx.Provide`
- Does not contain business logic

## Test Strategy

No module here needs real infrastructure to start, which affords a second tier of verification the parent
layer cannot afford. Each module therefore carries two sibling tests:

- **`Test<Module>_GraphIsValid`** — the layer baseline declared in [`../README.md`](../README.md).
  `fx.ValidateApp` resolves the module's output types **without** executing constructors or lifecycle
  hooks. Because nothing runs, this tier can demand the module's *full* output set against bare mocks:
  `AuthnModule` resolves its `IdentityResolver` and `AuthenticationFunc` here, which a booting test
  cannot reach without a usable tracer and database driver. Each one is asserted from **both sides** —
  the `異常系` case drops the module and requires the type to stop resolving, which is what proves the
  module is the provider rather than something else in the graph. Without that half, a module whose
  constructors take no arguments would have nothing left that could fail.
- **`Test<Module>`** — starts a minimal `fx.New` app and `fx.Populate`s the provided component, proving
  the constructors actually run and yield a usable value. Most modules assert only that the component is
  non-nil, because their constructor has one possible outcome; `AuthnModule`, whose provider selects an
  implementation per environment, asserts the selected value (`local.New()`).

The second tier is available **only because no module here needs real infrastructure to start**. That is
the criterion, not the directory: a module whose closure would require a live database or network
connection belongs in the parent, where `fx.ValidateApp` alone is the strategy.

Five of the six modules are thin wrappers over a constructor from `internal/controller/httpstack` and
friends. `AuthnModule` is the exception: `provideAuthenticator` branches per environment and takes an
`httpclient.Client`. Provider bodies carrying their own logic (`provideAuthenticator` /
`provideJWKSAuthenticator`) are unit-tested directly, per the DI layer baseline in
[`../../README.md`](../../README.md) — graph validation reaches neither, and the environment gate's
refusal cases are the point.

## Notes

- Adding or removing modules requires updating references from `applicationCoreOptions` in `internal/di/server.go`
