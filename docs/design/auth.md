# Authentication Subsystem Design Reference

This document consolidates the authentication subsystem's **role theory, state transitions (normal + error), implementation locations, what an integrator must implement, and glossary** into a single reference, derived from a close reading of the implementation. The subsystem has two halves that meet at one contract — a **JWKS key set** and an **access-token claim shape**:

- **Resource-Server side** — the Go API *verifies* an incoming access token. It never mints one.
- **Provider side** — a mock OIDC provider run from an upstream image (`ghcr.io/navikt/mock-oauth2-server`) *issues* tokens for local development and implements the OIDC login flow.

For the verification core see the [jwt README](../../internal/infrastructure/auth/jwt/README.md); for how the provider is wired and configured see the [docker README](../../docker/README.md). Its HTTP surface is plain OIDC Core — this repository documents no copy of it, because a spec of somebody else's implementation is a spec that rots. This page is the cross-component narrative that ties the two sides together.

---

## 1. Role theory (what, and what for)

Authentication here means: **the Resource Server (Go API) trusts an access token only after verifying its signature and standard claims against a public key it fetches from a trusted JWKS endpoint.** The RS mints nothing. In production that endpoint is a real IdP (Cognito / Auth0 / Keycloak / Entra ID); in local development it is the mock provider. The issuer and the verifier are **deliberately different implementations** (a JVM provider using Nimbus vs Go) so a bug in one JOSE library cannot cancel out a bug in the other.

Responsibility split (who owns what):

| Component | Side | Responsibility | Does NOT hold |
| --- | --- | --- | --- |
| **auth middleware** (`httpstack/oapi` + `oapi/auth`) | RS / controller | enforce `security:` on protected routes: extract `Bearer`, call Authenticator → IdentityResolver under the **request's** context, put `Authn` in context, `401` on a rejected credential and the classified status when verification could not be carried out | verification logic, business logic |
| **Authenticator** (`infrastructure/auth/jwt`) | RS / infrastructure | verify signature (RS256 allowlist) + claims (`iss`/`aud`/`exp`/`nbf`/`sub`) + `typ=at+jwt`; resolve the key by `kid` | key issuance, identity, HTTP policy |
| **JWKS resolver** (`jwt/jwks.go`) | RS / infrastructure | fetch the JWK Set via the resilient `httpclient` substrate, cache `kid → RSA key` (TTL), refresh on unknown `kid` under a cooldown, suppress re-fetch of confirmed-absent `kid`s (negative cache), tolerate key rotation | claim verification |
| **IdentityResolver** (`usecase/boundary/auth`) | RS / usecase boundary | map `(issuer, subject)` → internal `userID`; `401` on unknown / deleted | token verification |
| **IdentityRegistrar** (`usecase/boundary/auth`) | RS / usecase boundary | create the `(issuer, subject)` → `userID` link the resolver later reads; join the caller's transaction; `409` when the pair is already linked | deciding who may register |
| **mock provider** | provider (dev) | issue access / id tokens, serve JWKS + discovery, run Authorization Code Flow + PKCE | production use; anything the RS's own tests must be able to drive deterministically |
| **`AUTH_*` config** | config | issuer / audience / JWKS URL / algorithms / clock-skew / cache-TTL | logic |

### Registration: the one route that skips identity resolution

Resolution is enforced for every authenticated route, which creates a bootstrap problem: a subject with
no internal user yet is rejected with `401`, so it can never reach the endpoint that would create one.
`user_identities.user_id` carries a non-deferrable foreign key to `users(id)`, so the link cannot be
provisioned ahead of the user either.

The exception is declared in the spec rather than inferred in code. A route that creates its own
internal user declares the `BearerAuthRegistration` security scheme instead of `BearerAuth`; the
middleware verifies the token identically and skips only the resolution step, leaving `Authn` with an
unresolved `UserID`. Every other route keeps `BearerAuth` and stays fail-closed.

Two consequences worth stating:

- **The handler cannot use `RequireUserID`** — there is no internal user yet. It reads `Issuer()` and
  `Subject()` from `Authn` and hands them to the usecase, which mints the internal ID itself.
- **Re-registration is refused by the database, not by a pre-check.** The usecase writes the user and
  the identity link in one transaction, so a second attempt by the same subject violates the
  `(issuer, subject)` unique constraint and rolls the whole registration back. A read-then-write guard
  would be both redundant and racy.

Declaring `BearerAuthRegistration` on a route that does *not* create an internal user would hand an
unresolved — and therefore unauthorized — subject to a handler expecting one, so the scheme belongs
only on registration.

Design principles (invariants):

- **Fail-closed.** No error ever grants access. Every verification *failure* — a verdict reached about the credential — normalizes to `apperror.ErrUnauthenticated` (`401`), with the underlying cause preserved in the error chain for logs/traces. An *inability to verify* is a different fact and keeps its own classification: when the signing key cannot be fetched or the request's context ends, the error stays `apperror.ErrUnavailable` (`503`) or `apperror.ErrCanceled` (`499`), because it says nothing about the token and a `401` would tell the client to fix a credential nobody examined. Both are denials; only the reason reported differs.
- **Standard core only.** RS256 allowlist (`alg=none` and `HS256` always rejected — key-confusion defense); `iss` / `aud` / `exp` / `nbf` / `sub`; `typ=at+jwt` (RFC 9068) to reject ID-Token misuse. IdP dialects (Cognito `token_use`, Azure `scp`) are **extension points**, not built in.
- **Split-horizon.** The `issuer` (the token's `iss`, host/browser-resolvable) is separated from the **JWKS fetch URL** (container-internal). `ENDPOINT_JWKS` is set to the internal URL so `iss` stays host-resolvable while key fetching uses the container hostname.
- **Provider is dev-only.** It is reachable only through the `development` / `auth` compose profiles and is never part of a deployed environment.
- **Contract, not implementation.** What the RS depends on is the JWKS shape and the access-token claim shape (`typ=at+jwt` / `iss` / `aud` / `sub` / `exp`), which `docker/mock-auth-server/config.json` pins. Anything satisfying that contract — including a real IdP — drops in with **config changes only**, no Go change.
- **A second authenticator exists for streams.** A long-lived SSE connection cannot carry an
`Authorization` header through `EventSource`, so Realtime Delivery authenticates it with a
short-lived, single-purpose **stream ticket** carried as a query parameter, registered as its own
security scheme beside the JWT one ([ADR-0074 (query-ticket-stream-authentication)](../adr/0074-query-ticket-stream-authentication.md),
[realtime-delivery.md §2.4](realtime-delivery.md#24-ticket-lifecycle)). It is not a second way to
present a JWT: the feature authorizes the subject for a destination *first*, using the ordinary token
path above, and only then asks for a ticket. Authorization is therefore still decided here, and the
ticket only carries that decision to a transport that cannot hold a header. Revocation is explicit —
invalidating the tickets and notifying every instance — because a credential that outlives the
authorization that produced it is exactly what the deactivation rules in §2 exist to prevent.

---

## 2. User validity & deactivation (who invalidates, and how)

Authentication (the IdP asserts *who* the caller is) and **validity in this system** (whether that user is still an active member *here*) are **separate axes**. A request is honored only when **both** hold — a logical AND that the RS evaluates on **every** request:

> **effective access = ( token verifies — IdP side ) AND ( user is valid here: not soft-deleted + roles permit — RS side )**

This is why a structurally valid JWT is not sufficient. The provider's deleted-user fixtures (Charlie / Frank) carry perfectly valid, correctly-signed tokens yet are rejected — **"valid JWT ≠ usable in this system."**

Who owns which invalidation:

| Invalidation | Owner | Effect | Where (this repo) |
| --- | --- | --- | --- |
| **Account deactivation** (can no longer authenticate) | IdP | stops issuing *new* tokens; already-issued JWTs stay valid until `exp` | external IdP — the mock has **none** (it is a token stub with no account lifecycle) |
| **Membership invalidation** (soft-delete: withdrawn / banned) | this RS | rejected per-request regardless of token | the `IdentityResolver` implementation → `401` |
| **Authorization** (roles) | this RS | allow / deny per action | the role store + the `Authorizer` implementation |

The parts most often misread:

- **No runtime cross-query.** With JWT + JWKS the RS evaluates the AND **locally**: the IdP-side term is the JWT itself (signature checked against the *cached* JWKS — no per-request call to the IdP), and the validity term is this service's own `deleted_at` / roles. The RS never asks the IdP — nor any other service — for "deletion state" at request time. (A model that *does* consult the IdP per request is token **introspection** for opaque tokens — a different design this stack does not use.)
- **Per-service, not global.** Each Resource Server owns its own `deleted_at`. A user withdrawn from service A can still be valid in service B; there is no shared "deletion-state" service that RSs poll, and the IdP does not aggregate per-app membership.
- **Stateless caveat → why the RS must check locally.** Because a JWT is self-contained, an IdP account deactivation does **not** invalidate tokens already issued; they remain acceptable until `exp`. The per-request `deleted_at` check is what provides *immediate* revocation, independent of token lifetime.

Two provisioning paths *set* invalidity — distinct from the enforcement above, which only *reads* it:

- **App-initiated** (withdrawal / ban): this service soft-deletes its own user record. This is the primary runtime path; the withdrawal endpoint that drives it is a separate PBI.
- **IdP-initiated** (deprovisioning): when a real IdP disables a user and that must be reflected here, add a **thin ingress adapter** (a SCIM / webhook receiver, or an event consumer) that calls this service's deactivate path. The mock emits no such events and the propagation protocol is IdP-specific, so **this seam is intentionally left unbuilt** — the enforcement side (the `IdentityResolver` implementation honoring the soft-delete) is already ready to consume whatever sets it. The IdP *triggers*; the RS *enforces*.

---

## 3. State transitions

### 3.1 RS token verification — normal path

```mermaid
sequenceDiagram
    participant C as Client
    participant MW as auth middleware (oapi)
    participant A as Authenticator (jwt)
    participant J as JWKS resolver
    participant R as IdentityResolver
    C->>MW: request + Authorization: Bearer <jwt>
    MW->>A: Authenticate(ctx, Bearer token)
    A->>J: resolve key by kid
    alt kid in cache (fresh)
        J-->>A: RSA public key
    else miss / expired
        J->>J: fetch JWKS via httpclient (cooldown-throttled)
        J-->>A: RSA public key
    end
    Note over A: verify sig (RS256) + iss/aud/exp/nbf/sub + typ=at+jwt
    A-->>MW: Authn(subject, issuer, scopes, claims)
    MW->>R: Resolve(issuer, subject) → userID
    R-->>MW: Authn + internal userID
    Note over MW: put Authn in request context
    MW-->>C: → handler (business logic)
```

### 3.2 RS token verification — error paths (every verdict reached normalizes to `401`)

```mermaid
flowchart TD
    S["incoming request"] --> H{"Bearer present?"}
    H -- no --> E1["401 token not provided"]
    H -- yes --> K{"kid resolvable?"}
    K -- "JWKS unreachable / context ended" --> E7["503 or 499 — no verdict reached"]
    K -- "unknown kid (after cooldown-throttled refetch)" --> E2["401 invalid token"]
    K -- yes --> V{"sig + iss/aud/exp/nbf valid?"}
    V -- no --> E3["401 invalid token"]
    V -- yes --> T{"typ = at+jwt?"}
    T -- "no (ID Token misuse)" --> E4["401 invalid token"]
    T -- yes --> SUB{"sub present?"}
    SUB -- no --> E5["401 subject missing"]
    SUB -- yes --> ID{"identity (issuer, sub) known?"}
    ID -- "no / deleted" --> E6["401 identity not found"]
    ID -- yes --> OK["authenticated → handler"]
```

Every one of these `401` branches is driven deterministically in `internal/integration/jwt_auth_test.go`, which mints its own tokens from keys it generates in-process — expired, not-yet-valid, wrong issuer, wrong audience, missing subject, unknown `kid`, retired key, an unsupported algorithm, an ID Token in an access token's place. That coverage deliberately does not run through the mock provider: a provider whose job is to issue *valid* tokens is a poor instrument for proving invalid ones are refused, and depending on one to do so ties the RS's error paths to a third party's feature set.

### 3.3 Provider Authorization Code Flow + PKCE — normal path

Endpoint paths are the ones the issuer's discovery document advertises; `default` below is the
`issuerId` from `docker/mock-auth-server/config.json`, which is also the JWKS `kid`.

```mermaid
sequenceDiagram
    participant C as Client (RP)
    participant M as mock provider
    C->>M: GET /default/authorize (client_id, redirect_uri, code_challenge=S256, state, nonce)
    Note over M: login form → the username entered becomes sub
    M-->>C: 302 redirect_uri?code&state
    C->>M: POST /default/token (code, code_verifier, redirect_uri, client_id)
    Note over M: consume code once + verify PKCE S256
    M-->>C: 200 access_token (typ=at+jwt) + id_token (nonce)
    C->>M: GET /default/endsession (id_token_hint, post_logout_redirect_uri, state)
    M-->>C: 302 post_logout_redirect_uri?state
```

A token can also be minted without a browser, which is what scripted checks and the DAST scan use —
`POST /default/token` with `grant_type=password` and a `username`, which the provider copies onto `sub`
just as the login form does. Nothing about that path is privileged: it is the standard token endpoint,
and the token it returns has the same shape as the one the login flow produces. It is the replacement
for the bespoke test hatch the previous provider carried.

The `sub` is whatever `username` the login form (or the password grant) was given, so it has to be one
<!-- sample-api:replace-begin -->
the seed registered in `user_identities` — otherwise verification succeeds and identity resolution then
refuses it. **Roles are not carried in the token.** They live in this service's database (`user_roles`)
and are served by `GET /v1/users/me/roles`; an IdP that happens to know them is a property of one
<!-- sample-api:replace-with -->
<!-- = the seed registered as an identity — otherwise verification succeeds and identity resolution then -->
<!-- = refuses it. **Roles are not carried in the token.** They live in this service's database -->
<!-- = and are served by its own API; an IdP that happens to know them is a property of one -->
<!-- sample-api:replace-end -->
deployment, not of the contract, so a client deciding what to show an administrator reads the API.

### 3.3.1 Why `aud` is multi-valued

The provider applies one claim set to *both* tokens it issues, so `aud` cannot be set per token type.
That forces a choice, because the two tokens want different audiences: an access token's `aud` names the
resource server, an ID Token's names the client (OIDC Core 3.1.3.7). Leaving `aud` unset gives the ID
Token the right value and strips the access token of one entirely, which the RS then refuses.

So `docker/mock-auth-server/config.json` sets **both** — `aud: ["go-boilerplate-api",
"go-boilerplate-client"]` plus `azp: "go-boilerplate-client"`. Each side validates successfully against
the half it cares about: the RS requires its own audience to be *present* in `aud`, and a client
requires `aud` to *contain* its `client_id` (§3) and, when `aud` is multi-valued, checks `azp` (§4).
`AUTH_AUDIENCE` therefore stays the resource-server audience, which is the value a real IdP will be
configured with.

> **Three consequences of the provider being an upstream image, all accepted deliberately.**
>
> 1. **The ID Token is not distinguishable from the access token here.** One claim set and one `typ`
>    header cover both, so the `typ=at+jwt` the RS requires (RFC 9068) is stamped on the ID Token too,
>    and the `aud` above is shared. Locally, presenting the ID Token to the API therefore succeeds where
>    a real IdP's ID Token — carrying only `aud=<client_id>` — would be refused. The RS's rejection of
>    that misuse is real and is pinned by `internal/integration/jwt_auth_test.go`; what is lost is only
>    the ability to *demonstrate* it against this provider.
> 2. **The provider's own `/userinfo` is unusable.** It refuses `at+jwt` as a JOSE type. A client that
>    needs profile claims locally should read the ID Token rather than call UserInfo — which is what
>    OIDC client libraries do by default anyway.
>
> 3. **`redirect_uri` is not registered.** The provider accepts any value, where a real IdP matches it
>    against a registered list and is the main defence against an open redirect in the code flow. The
>    permissiveness is what lets several worktrees — each on its own port — drive the flow without
>    re-registering a client per slot. Treat client registration as something the integrator configures
>    at the real IdP; nothing in the RS depends on it, since the RS never sees `redirect_uri`.
>
> None of these is worth surrendering the RS's `at+jwt` requirement for: that requirement is production
> behavior, and weakening it locally would mean the environment closest to a developer is the one
> exercising the least of the verification path.

### 3.4 Key rotation (JWKS phases)

Rotation is defined by the separation between the **published set** (keys served in the JWKS) from the **single signing key**. The classic three phases are:

```text
Phase 1  JWKS: [key-a]         Signing: key-a   (initial)
Phase 2  JWKS: [key-a, key-b]  Signing: key-b   (add-key + promote key-b)
Phase 3  JWKS: [key-b]         Signing: key-b   (retire key-a)
```

On the RS side the JWKS resolver survives this without re-fetching on every request:

- **Known `kid` → cache hit**, no fetch (a rotation does not add per-request cost).
- **Unknown `kid` → one refetch** (cooldown-throttled, concurrent fetches collapse to one), so a `key-b` token issued mid-rotation is picked up.
- **Negative cache**: a `kid` confirmed absent *by an actual fetch* in the current cache generation is remembered, so repeated bogus/`kid` probes do not each trigger a refetch. It is discarded when a successful fetch changes the published set, and never applies to a stale cache or to a `kid` that was only throttled (not fetched) — so a `kid` added by a rotation is still resolved on the next fetch (bounded by the cache TTL), never permanently rejected.
- **Retired key → `401`**: once the cache generation refreshes and `key-a` is gone from the published set, tokens signed by it fail the "`kid` resolvable?" branch.

The state-transition end-to-end is covered deterministically in `internal/integration/jwks_rotation_test.go`, which drives the phases through the real HTTP boundary against the golden JWKS bytes and PEMs under `internal/integration/testdata/`.

> **The mock provider cannot replay a rotation.** It derives the JWKS `kid` from the `issuerId` and publishes exactly one key per issuer, so no reachable configuration puts two `kid`s in one JWKS or moves the signing key between them. The phases above are therefore driven only by the test's own fixtures, which the test owns outright rather than sharing with the provider. This is a deliberate trade: rotation is a property of the RS's resolver, and the resolver is what the test exercises.

---

## 4. Implementation locations

| Aspect | Location |
| --- | --- |
| Auth enforcement (middleware, priority 6) | `internal/controller/httpstack/oapi/oapi.go`, `oapi/auth/auth.go` |
| Authenticator boundary interface | `internal/usecase/boundary/auth/{authenticator,credential,auth,resolver}.go` |
| JWT verification core | `internal/infrastructure/auth/jwt/auth_jwt.go` |
| JWKS resolution (`kid` lookup, TTL cache, unknown-`kid` refresh cooldown, negative cache, key rotation) | `internal/infrastructure/auth/jwt/jwks.go` |
| Dev-only stub (`Bearer debug:<subject>`, `ci` / `test` env) | `internal/infrastructure/auth/local/auth_local.go` |
| Identity resolution (`sub` → internal `userID`) | the project's `IdentityResolver` under `internal/infrastructure/auth/`; the substrate default is the passthrough in `internal/infrastructure/auth/identity/` |
| DI wiring (env-driven authenticator selection, JWKS downstream profile) | `internal/di/module/core/auth.go`, `internal/di/module/auth.go` |
| Real-JWT execution context for scanning (`dast` env: JWKS-backed authenticator against the mock provider over http) | `env/.env.dast`, `.github/workflows/zap-api-scan.yaml` |
| Config (`AUTH_*`) | `internal/config/envspec.go`, `internal/config/model.go` |
| Ops-path / metrics auth exception | `internal/controller/httpstack/oapi/skipper/`, ADR [0020](../adr/0020-metrics-endpoint-auth-exception.md) |
| Development OIDC provider | `docker-compose.yaml` (`mock_auth_server`) + `docker/mock-auth-server/config.json`; the image digest is pinned in `docker/images-pin.toml` |

---

## 5. What an integrator implements

1. **Point the RS at an IdP via config.** `AUTH_ISSUER` (must equal the token's `iss`), `AUTH_AUDIENCE`, and `ENDPOINT_JWKS` (the IdP's `jwks_uri` — optional; leave it empty to derive `jwks_uri` from the issuer via OIDC discovery, see the note below). Locally, `env/.env` sets `ENDPOINT_JWKS` explicitly at the container-internal host per split-horizon. Optional knobs: `AUTH_ALLOWED_ALGORITHMS` (default `RS256`), `AUTH_CLOCK_SKEW` (`60s`), `AUTH_JWKS_CACHE_TTL` (`1h`).
2. **Swap the mock for a real IdP** by changing only those env values — the JWKS + claim contract is what the RS depends on, so no Go change is required. Keep `iss` host-resolvable and `ENDPOINT_JWKS` reachable from the API container.
3. **Add IdP dialects** when your IdP deviates from the standard core — Cognito `token_use` / `aud`→`client_id`, Azure `scp` / `roles`, EC keys, opaque tokens — at the extension points listed in the [jwt README](../../internal/infrastructure/auth/jwt/README.md).
4. **Identity resolution** maps `(issuer, subject)` to an internal user; provide an `IdentityResolver` implementation for your user store. Without one the DI wires the passthrough default, which leaves the internal UserID unresolved — so no unknown or deactivated subject is rejected here.

> **JWKS URL resolution (static vs discovery).** By default the RS resolves the JWKS URL **statically** from `ENDPOINT_JWKS` — this is what `env/.env` sets (split-horizon). Alternatively, leaving `ENDPOINT_JWKS` empty makes the RS derive `jwks_uri` from the issuer's `/.well-known/openid-configuration` via **OIDC discovery** (issuer strict-match + same-origin + https), cached on its own `AUTH_JWKS_DISCOVERY_TTL` (default `24h`). Independently, `AUTH_JWKS_UNKNOWN_KID_COOLDOWN` (default `60s`) is the minimum interval between unknown-`kid` JWKS refetches. The mock provider serves the discovery document for both modes, at the issuer URL it was reached through.

---

## 6. Glossary

| Term | Meaning |
| --- | --- |
| **Access token** | The JWT the RS verifies to authenticate a request. Carries `typ=at+jwt` (RFC 9068). |
| **ID Token** | An OIDC token about the end-user (`token_use=id`, `aud=client_id`, `typ=JWT`). **Must not** be used as an access token — the RS rejects it via the `typ` check. |
| **JWKS** | JSON Web Key Set — the public keys the RS fetches to verify signatures (RFC 7517). |
| **`kid`** | Key ID in the JWT header; selects which JWKS key verifies the signature. |
| **OIDC discovery** | The `/.well-known/openid-configuration` document that advertises `issuer` / `jwks_uri` / endpoints. Served by the provider; consumed by the RS only in discovery mode (empty `ENDPOINT_JWKS`). |
| **`issuer` / `iss`** | The token issuer identifier; the RS requires an exact match. |
| **`audience` / `aud`** | The intended recipient; the RS requires its configured audience. |
| **PKCE (S256)** | Proof Key for Code Exchange (RFC 7636): `code_challenge = base64url(sha256(code_verifier))`; the token endpoint re-derives and compares. `plain` is not accepted. |
| **Authorization code** | A short-lived, single-use credential from the authorization endpoint, exchanged once at the token endpoint. |
| **Split-horizon** | Separating the browser/host-facing `issuer` URL from the container-internal JWKS fetch URL, so `iss` stays host-resolvable while key fetching uses the internal hostname. |
| **Authn** | The verified-but-optionally-unresolved result (`subject`, `issuer`, `scopes`, `claims`, and — after identity resolution — internal `userID`). |
| **Identity resolution** | Mapping `(issuer, subject)` to an internal application `userID`; a concern separate from token verification. |
| **Fail-closed** | Any verification error results in denial, never a default-allow — `401` when the credential was rejected, `503` / `499` when no verdict could be reached. |
| **Algorithm allowlist** | The set of accepted signature algorithms (default `RS256`); `alg=none` / `HS256` are always rejected to prevent key-confusion. |
