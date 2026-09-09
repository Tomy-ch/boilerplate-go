# Post-Repository Clone Task List

For details of Make commands, refer to [Make Target List](../../.makefiles/README.md).

## Phase 1: Install mise and activate it in your shell

This project requires [mise](https://mise.jdx.dev) as the tool / runtime version manager. Install it via the [official installation guide](https://mise.jdx.dev/getting-started.html), then **activate it in your shell init** — this is mandatory, not optional. The repository's Make targets resolve `golangci-lint`, `lefthook`, etc. through mise's shims, and the shims are only on `PATH` after activation:

```sh
# zsh
echo 'eval "$(mise activate zsh)"' >> ~/.zshrc

# bash
echo 'eval "$(mise activate bash)"' >> ~/.bashrc

# then reload (or open a new terminal)
exec $SHELL
```

Verify activation with:

```sh
mise --version
which mise
```

## Phase 2: Install the Go runtime and project tools

All tool versions (golangci-lint / sqlc / oapi-codegen / mockgen / dlv / lefthook / ...) are pinned in [`mise.toml`](../../mise.toml) as the single source of truth. The Dockerfiles, the local installer (`.makefiles/go/installer.mk`), and the CI workflows all install only what they need via `mise install <tool>` against the same `mise.toml`.

```sh
make go-update       # installs the pinned Go runtime
make install-tools   # installs gopls / gotests / impl / dlv / lefthook / golangci-lint / zizmor
make activate-tools  # runs `lefthook install` to wire git hooks
```

## Phase 3: Install the agent configuration (standard path)

The AI-assist layer ships as configuration: project-scoped official plugins, this repository's own skills under [`.claude/`](../../.claude/README.md) / [`.codex/`](../../.codex/README.md), and one external skill carried as **standard equipment** (`graphify`, a queryable knowledge graph of the repository, whose commands and usage discipline are in [AGENTS.md](../../AGENTS.md)). Two idempotent bootstraps install the parts a clone does not carry:

```sh
bash .claude/scripts/bootstrap-plugins.sh          # official plugins (project scope)
bash .claude/scripts/bootstrap-external-skills.sh  # external skills (user scope: Claude Code + Codex)
```

`graphify` is pinned separately from the rest of the toolchain: the version is declared in [`python/graphify.in`](../../python/graphify.in) and locked with hashes in `python/graphify.txt`, **not in `mise.toml`**. The bootstrap installs it from that lockfile into a machine-local venv under `${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify`, then uses that binary to write the skill into each assistant's config directory. The same lockfile installs it into the `python_tool_runner` image, which is what `make graphify-*` runs. Which of its commands stay local and which reach an LLM API is stated in [AGENTS.md](../../AGENTS.md); what pays off on this repository specifically, and what does not, is measured in [`.claude/README.md`](../../.claude/README.md).

**This phase is the standard path, not an extra.** AI-assisted development is how this template expects a change to be made, and the skills under `.claude/` / `.codex/` are the executable form of the flows in [docs/development-flow.md](../development-flow.md). Rationale: [ADR-0007 (agents-md-operational-contract)](../adr/0007-agents-md-operational-contract.md).

**Declining the layer is still the adopting architect's call**, and nothing above is load-bearing for building, testing, or shipping: the layering rules live in [docs/rules.md](../rules.md), not in the assistant configuration, and the application never depends on an AI service. What you give up is the developer experience — the flows remain documented, but the tooling that drives them is not maintained in a manual form, so a project that declines the layer is on a compatibility path rather than a second supported one.

A project that does not want the layer should remove it deliberately rather than leave it half-configured:

- skip both bootstraps; no other phase of this setup depends on them, and
- drop what you do not want to carry: `.claude/`, `.codex/`, `python/graphify.in` and `python/graphify.txt`, the graphify layer in `docker/tools/Dockerfile`, `.graphifyignore`, and the `graphify-out/` entries in `.gitignore`, `.markdownlint-cli2.yaml`, and `scripts/mermaid-lint/index.ts`. That list is not exhaustive — `grep -ril graphify` over the checkout is, and it also reaches `.makefiles/graphify/`, `scripts/graphify-*`, `.github/workflows/graphify-*.yaml`, and `.agents/graphify/`.

Removing it later costs the same as removing it now, so adopting the standard configuration first and deciding afterwards is a safe order.

## Phase 4: Local Startup Verification

Start the application locally and confirm it works without issues.

```sh
make serve
make tools
make db-init
```

<!-- boilerplate-only:begin -->
## Phase 5: Execute Localization Script

<!-- lang-choice:begin -->
### Choose the documentation language

This repository ships every document twice: an English canonical and a Japanese translation beside
it (`<name>.ja.md`), and the same for every skill (`SKILL.md` / `SKILL.ja.md`). That is 422 pairs.
A project that reads only one of the two languages maintains the other for nobody.

Run this **first**, before the replacements below and before every later removal. The removal tools
carry paired declarations for the canonical and the translation, and each prunes its own pair the
moment the fold resolves it — so a folded tree is what they expect. The reverse does not hold:
Phase 12 deletes a workflow this tool declares a string in, so a fold attempted after it aborts.

The working tree must be clean, which it is here and stops being so once the replacements below run.

```bash
# Preview: writes nothing and commits nothing. Works on a dirty tree.
DRY_RUN=1 make setup-remove-doc-language LANG_CHOICE=en

# Fold. The working tree must be clean — the script commits the whole fold at once.
make setup-remove-doc-language LANG_CHOICE=en
```

| `LANG_CHOICE` | What happens |
| --- | --- |
| `en` | Deletes every `*.ja.md` and removes the references the surviving English documents make to them |
| `ja` | Deletes every English canonical and **renames** each `<name>.ja.md` to `<name>.md`, so the filename contract (`SKILL.md`, `README.md`) still holds. A skill's YAML frontmatter is transplanted from the canonical onto the translation, which is what keeps the skills loadable |
| `both` | Keeps the pairs. The markers that mark the fold sites are resolved and the tool retires itself, so this is a decision, not a deferral — revert the commit to reopen it |

Picking `en` or `ja` leaves no `*.ja.md`, so the checks that require a translation pair
are removed too: `doc-ref-lint`'s pair check, `skill-lint`'s `SKILL.ja.md` requirement, the portal's
Japanese entries and the viewer's language switch. The `canonicalize-doc` skill goes with them —
it exists to create and sync the pairs, and it has no subject once there is one language.

`ja` is the more expensive choice and worth a moment's thought. The English canonical exists because
agents read it: Japanese costs roughly 1.5–2.0× the tokens and carries a higher misreading risk
([ADR-0011](../adr/0011-docs-as-canonical-source.md)). Choosing `ja` accepts that for every agent
run in the project's life. Choose it when the team reads Japanese and the documents are for people
first.

The script refuses to run rather than guess. Prose that explains the translation convention itself
cannot be folded mechanically, so anything not covered by a `doc-pair` marker or declared in
`language-manifest.ts` stops the run with its file and line — and nothing is written when it does.

**Decide it here, not later.** Everything past this point — the rest of the setup and then ordinary
development — pays for both languages every day: each documentation edit is written twice, each
agent run reads past the pairs it does not need, and the two sides drift the moment one is updated
alone. That cost recurs; the decision does not — it is one command.
<!-- lang-choice:end -->

### Replace the module name and the project metadata

Run the following commands to execute the script that replaces the Go module name in bulk.

Replace ORG, REPO, and CODE_OWNERS as appropriate. Only change derived settings if necessary.

```sh
export ORG=<your-org/git-user-name>
export REPO=<your-repo>

# CODEOWNERS owner — a user (@name) or a team (@org/team). An organization itself
# cannot own a path, so a repository owned by one must name a team.
export CODE_OWNERS=<@your-org/tech-leads>

export MODULE=${REPO}
export APP_NAME=${REPO}
export OPENAPI_TITLE=${REPO}
export COPILOT_TITLE=${REPO}
export COPYRIGHT_HOLDER=${ORG}
export COPYRIGHT_YEAR=$(date +%Y)

make setup-replace-module OLD_MODULE=go-boilerplate NEW_MODULE=$MODULE
make setup-replace-repository-reference REPOSITORY=$ORG/$REPO
make setup-replace-app-metadata APP_NAME=$APP_NAME OPENAPI_TITLE="$OPENAPI_TITLE" COPILOT_TITLE="$COPILOT_TITLE"
make setup-replace-license-copyright COPYRIGHT_HOLDER="$COPYRIGHT_HOLDER" COPYRIGHT_YEAR=$COPYRIGHT_YEAR
make setup-replace-codeowners OWNERS="$CODE_OWNERS"
make gen-api
make gen-sqlc
make tidy-lib

# Verify the replacements landed, then remove the localization tooling.
# Run this last: the scripts above are one-shot, and keeping them lets you re-run
# any of them until this passes.
make setup-verify
```

`setup-verify` checks that every file `replace-module` claims to cover is free of the
boilerplate name, and that LICENSE, CODEOWNERS, README and the OpenAPI spec carry the values you
passed. Only once it passes does it delete `scripts/setup/replace-*` and itself — re-applying
them to an already-localized repository is wrong, since `replace-codeowners` rewrites *every*
rule's owner to a single value. `scripts/setup/lib` goes with whichever of the two one-shot
tools (this one and the sample remover) runs last.

## Phase 6: Localization Verification

Confirm that basic functionality works correctly, including tests, static analysis, code generation, and health checks.

```sh
make go-test
make go-lint
make gen
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## Phase 7: Manual Rewrites

1. Rewrite the contents of README.md and README.ja.md according to your project; replace or remove
   the repository-specific branch-rule exception in the maintainer-policy section.
2. If your project keeps its documentation in a single language, you may collapse the pair — for
   example by replacing README.md with the contents of README.ja.md.
    - [gen-docs-json.ts](../../scripts/portal/gen-docs-json.ts) and the
      [manifest.yaml](../../docs/portal/manifest.yaml) it generates from both reference README.md,
      so a full replacement has to rewrite those scripts as well.
    - The portal UI carries an En / Jp switch of its own, so it needs the same treatment.
3. Rewrite the contents of [openapi.yaml](../../openapi/openapi.yaml) according to your project.
    - Rewrite the entire Info section according to your project.
        - title
        - termsOfService
        - contact
        - version
        - description
        - license

<!-- boilerplate-only:end -->
## Phase 8: Rewrite env Files

Rewrite the files in the [env/](../../env/) directory according to your project.

For the meaning of configuration values, refer to [env/README.md](../../env/README.md).

### Review the clamped config values

A few subsystems **clamp** out-of-range values to safe defaults instead of failing startup — a resilience choice so a misconfigured process still runs. A clamp only triggers when you explicitly set `0` / a negative / an invalid value (the shipped env vars carry sane `envDefault`s), but because the correction is applied silently at runtime, review these when you tune the corresponding `WORKER_*` / `OUTBOX_*` / `SECURE_COOKIE_*` values:

- **Worker engine** (`WORKER_*`) — `Settings.normalize()`; see [internal/controller/worker/README.md](../../internal/controller/worker/README.md) (Config clamping).
- **Outbox relay** (`OUTBOX_*`) — `provideRelaySettings`; see [internal/controller/outbox/README.md](../../internal/controller/outbox/README.md) (Settings → Clamping).
- **Secure cookie** (`SECURE_COOKIE_SAME_SITE`) — `normalizeSameSite` clamps any non-`Lax`/`Strict`/`None` value to "do not override"; see [internal/controller/httpstack/cookie/README.md](../../internal/controller/httpstack/cookie/README.md).

## Phase 9: Repository Initialization

After completing the above steps, initialize the repository after the first push.

`setup-repo` enables workflows before it applies the ruleset. Before applying required status
checks to an existing repository, confirm that each declared context has reported successfully on
a pull request; otherwise GitHub can block merges while waiting for a context it has never seen.

### Register the auto-PR app before you require any status check

Three workflows open a pull request of their own — [`auto-generate-docs.yaml`](../../.github/workflows/auto-generate-docs.yaml),
[`graphify-sync.yaml`](../../.github/workflows/graphify-sync.yaml) and
[`graphify-extract.yaml`](../../.github/workflows/graphify-extract.yaml). GitHub does not start a
workflow run for an event a workflow raised with its own `GITHUB_TOKEN`, which is what stops a
workflow from triggering itself. A pull request opened with that token therefore reports no check at
all, and a required context that never reports is the case the warning above describes: the merge
waits with nothing to wait for, and re-running fixes nothing. An app token is a separate identity, so
the events it raises are ordinary ones.

Skip this whole step if you require no status checks. Otherwise do it **before** `make setup-repo`
applies the ruleset, so the first auto-PR is mergeable rather than the first one to get stuck.

#### Create the app (by hand)

This cannot be automated: the REST API has no endpoint that creates an app, and the private key is
shown once, at generation.

Create it at <https://github.com/settings/apps/new>.

| Field | Value |
| --- | --- |
| GitHub App name | `<this repository's name>-auto-pr-app` (**unique across all of GitHub**) |
| Description | `Opens the generated-artifact pull requests for <this repository's name>` |
| Homepage URL | `https://github.com/<owner>/<this repository's name>` |
| Webhook | **clear the Active checkbox** |
| Repository permissions → Contents | **Read and write** |
| Repository permissions → Pull requests | **Read and write** |
| Every other permission | leave at No access |
| Where can this GitHub App be installed? | **Only on this account** |

Contents write pushes the `auto/*` branch and Pull requests write opens the pull request; that is the
whole job, and the workflows narrow the installation token to exactly those two, so a wider app
grants nothing extra. A common repository name may already be taken — prefix the owner name if so.
The name can be changed later.

Three things follow, in the order the **General** page presents them after creation:

1. **Note the App ID** — a number at the top of the General page
2. **General → Private keys → Generate a private key** — downloads a `.pem`
3. **Install App → Only select repositories** — this repository, and nothing else

#### Register the secrets

```bash
gh secret set AUTO_PR_APP_ID --body '<the App ID from step 1>'
gh secret set AUTO_PR_APP_PRIVATE_KEY < ~/Downloads/<the downloaded>.pem
```

**Delete the `.pem` once it is registered.** What the browser downloaded is a live private key sitting
in a directory nothing protects.

```bash
gh secret list   # AUTO_PR_APP_ID / AUTO_PR_APP_PRIVATE_KEY are listed
```

Nothing breaks while the app is missing. Each workflow warns and falls back to `GITHUB_TOKEN`, so the
pull request still opens — it just carries no checks, which means it cannot be merged while any check
is required. A pull request already opened that way cannot be rescued by registering the app
afterwards: an event is raised for a head commit, and none was. Close it and let the workflow open a
fresh one.

### When starting from a GitHub template

```sh
git add -A
git commit -m "Initial commit: setup boilerplate for $REPO"
git push origin main
make setup-repo
make branch-minor
```

### When starting from Git Clone

```sh
git remote set-url origin ${ORG}/${REPO}
git add -A
git commit -m "Initial commit: setup boilerplate for $REPO"
git push -u origin main
make setup-repo
make branch-minor
```

## Phase 10: Create Deployment Configuration

This boilerplate adopts a configuration that does not depend on a specific cloud provider or deployment method, allowing flexible deployment to various cloud or on-premise environments.

Therefore, deployment settings do not include a specific deployment target. Add necessary settings according to your project's deployment destination.

Deployment CI/CD: Complete [.github/workflows/deploy-app.yaml](../../.github/workflows/deploy-app.yaml).

`Note: Please modify this section according to your environment` indicates sections that need to be modified according to your environment.

## Phase 11: Implement Authentication & Authorization

This boilerplate ships **development-only stubs** for both authentication (authn) and authorization (authz), and they are wired **only** for the `local` / `ci` / `test` environments. For `development` / `staging` / `production` the DI providers are **fail-closed**: they refuse to wire the stub and return an error, so the application **deliberately fails to start** until you implement and wire real components.

This is an intentional forcing function — it guarantees a signature-skipping authenticator or an allow-all authorizer can never ship to a real environment. **Implementing both for `development` / `staging` / `production` is a required project-start task.**

> [!IMPORTANT]
> The `Authorizer` is provided inside `InfrastructureModule`, so **every process that builds a usecase** — the HTTP server **and** the background job / worker processes — requires a configured `Authorizer`. Until the authorization step below is done, running any of them with `APP_ENV=development` / `staging` / `production` exits at Fx construction with `no authorizer configured for environment` (authn behaves the same with `no authenticator configured for environment`). Seeing this before you implement the real components is expected, not a bug.

### Authentication (authn)

This boilerplate includes sample code using JWT as an example implementation of authentication. Implement authentication according to your project requirements.

Create authentication functionality by implementing the usecase [Authenticator](../../internal/usecase/boundary/auth/authenticator.go) interface.

- Reference: [internal/infrastructure/auth/README.md](../../internal/infrastructure/auth/README.md)
- Stub example (local, signature-less): [internal/infrastructure/auth/local/auth_local.go](../../internal/infrastructure/auth/local/auth_local.go)
- Add your `stg` / `prd` implementations (JWT / OAuth2 / OIDC / Cognito / Auth0 など) under `internal/infrastructure/auth/{stg,prd}/`.
- Wire them per environment by editing the [authentication DI module](../../internal/di/module/core/auth.go) (`provideAuthenticator`): replace the `default` fail-closed branch with `case config.EnvDevelopment / EnvStaging / EnvProduction` returning your real `Authenticator`.

### Authorization (authz)

This boilerplate ships an **allow-all** authorizer as a development stub. Implement a real policy decision point (PDP) for your project.

Create authorization functionality by implementing the usecase [Authorizer](../../internal/usecase/boundary/authz/authorizer.go) interface.

- Reference: [internal/infrastructure/authz/README.md](../../internal/infrastructure/authz/README.md)
- Stub example (allow-all): [internal/infrastructure/authz/allowall/authz_allowall.go](../../internal/infrastructure/authz/allowall/authz_allowall.go)
- Add your `stg` / `prd` implementations (RBAC from claims / ownership check / external policy engine such as OPA / Cedar) under `internal/infrastructure/authz/{stg,prd}/`.
- Wire them per environment by editing the [authorization DI module](../../internal/di/module/authz.go) (`provideAuthorizer`): replace the `default` fail-closed branch with `case config.EnvDevelopment / EnvStaging / EnvProduction` returning your real `Authorizer`.

The `Authorize(ctx, *auth.Authn, Action, *Resource)` signature already carries the full `Authn` (subject / scopes / claims) and the target `Resource` (with optional `OwnerID`), so both RBAC and ownership (object-level) models are expressible without changing call sites.

<!-- boilerplate-only:begin -->
## Phase 12: Remove what only holds while this is a boilerplate

Two kinds of statement in this repository stop being true the moment you create a repository from
it: the passages
where it describes *itself* as a boilerplate, and the conventions it follows *because* it is one —
the in-place ADR amendment regime, the consolidation pass, the `setup-review` device. Both are
template scaffolding, not your project's documentation.

```sh
DRY_RUN=1 make setup-remove-boilerplate-identity
make setup-remove-boilerplate-identity
```

It scans the repository for `boilerplate-only` markers and resolves each one, deletes
[boilerplate-only-conventions.md](boilerplate-only-conventions.md) and its Japanese mirror along
with `docs/plan/**` (the upstream's requirements for a release line it has not built yet), drops
its own make target from the registry, and then removes itself.

`docs/project/roadmap.md` is **not** deleted: its opening is written so the pass swaps in a
project-appropriate replacement, leaving you a place to record your own direction. It scans rather than working from
a list of files, because a list is something a marker can be written outside of — and a marker
nobody strips is a premise that survives into your project with nothing to announce it.

What it does **not** touch: the repository / module name (already replaced in Phase 5), and the
parts of this guide you keep reading — the clamped-config review and the exclusion ADRs below,
which several package READMEs link to.

What survives each removal is the general form of the rule, stated in the document that owns it:
[docs/adr/README.md](../adr/README.md), [docs/rules.md](../rules.md), the layer READMEs. Where a
statement needed a project-appropriate replacement rather than plain deletion, the replacement is
already parked beside it and is swapped in by the same pass.

> Never strip `boilerplate-only` and `sample-api` markers in one run. They fire at different
> moments — this phase versus the sample removal in Phase 15 — and a project may reasonably do one
> without the other.

<!-- boilerplate-only:end -->
## Phase 13: Review the template's deliberate exclusions (ADRs)

Beyond authentication / authorization (Phase 11) and deployment (Phase 10), this template makes other **deliberate non-choices** — for example: no in-application rate limiter, no generic cache abstraction, scheduled-job concurrency left to the scheduler, and push / streaming brokers kept out of the worker port.

Each such non-choice is recorded as an **exclusion ADR** under [docs/adr/](../../docs/adr/), tagged `setup-review`. List them with:

```sh
grep -rl "setup-review" docs/adr/
```

For your project, review each and decide:

- **Keep** — the exclusion fits your project; leave the ADR as is.
- **Change** — you need the opposite. Setup is where your project establishes its **own baseline** from the template, so **edit the ADR directly** (rewrite its Decision / Consequences and update `deciders` / `date`) to record your project's choice, then implement accordingly.

The immutable, supersede-by-new-ADR model (do not edit; add a superseding ADR) applies to decisions you revisit **later**, during ongoing development — not to this one-time re-baselining at setup.

## Phase 14: Decide the dependency-license policy

The dependency-license scan (`make trivy-license`, and the `trivy-license` job in [.github/workflows/trivy-fs.yaml](../../.github/workflows/trivy-fs.yaml)) is **report-only, permanently**. It enumerates every dependency's license into the job summary and a PR comment, and never fails the build.

That is a deliberate non-choice, not an unfinished gate. Which licenses are acceptable is a legal question owned by the organization adopting this template: copyleft that is disqualifying for a distributed binary can be entirely acceptable for a service whose binary never leaves your infrastructure, and the answer varies by company, product, and distribution model. Picking a threshold here would bake one company's legal posture into every project built on it, so the template ships the inventory and leaves the judgement to the adopter.

If your organization has (or needs) a prohibited-license policy, gate it yourself:

1. Decide the acceptable set in terms of Trivy's own classification (`notice` / `unencumbered` / `permissive` / `reciprocal` / `restricted` / `forbidden` / `unknown`), and decide whether shipped artifacts and build-only tooling get the same bar. They may not: the classifications outside `notice` / `unencumbered` in this repository come from `docker/tools/`, which is build-only and never shipped.
2. Treat Trivy's classification as a starting point, not an authority. `BlueOak-1.0.0` lands in `unknown` even though it is OSI-approved and permissive, so decide such cases explicitly instead of letting the bucket decide for you.
3. Add the threshold to `trivy-license-ci` in [.makefiles/security/trivy.mk](../../.makefiles/security/trivy.mk) and a failing step to the `trivy-license` job, recording per-package exceptions in [.trivyignore.yaml](../../.trivyignore.yaml).
4. Update the trigger matrix in [.github/workflows/README.md](../../.github/workflows/README.md) and the license row of [ADR-0093 (multi-layer-security-scanning)](../adr/0093-multi-layer-security-scanning.md), which both currently state that no policy exists.

## Phase 15: Remove Sample APIs

This boilerplate includes sample APIs. Remove them according to your project requirements.

If you use AI-driven development, keeping sample APIs helps AI understand code structure and implementation patterns. You may also refactor sample APIs to align them with your project requirements.

### Removal Procedure

Use the automated command. It deletes the sample API (`user` / `product` / `order`) declared in `scripts/setup/remove-sample-api/sample-manifest.ts` — named rather than linked, because the removal takes that file with it and a link would break the moment the procedure succeeds — strips the `sample-api` marker blocks from the shared files (4 DI modules + `openapi.yaml`), and then regenerates / formats / lints.

> **The DB container must be running** before you run this — the final `gen-query` step dumps the **live** schema with `pg_dump`, so a stopped DB fails with `connection refused`.

```bash
# 0. Start the DB container (gen-query dumps the live schema)
docker compose up -d database

# Preview what will be removed (no changes are made)
DRY_RUN=1 make setup-remove-sample-api

# Remove the sample (deletes files + strips markers, then gen-api → gen-query → fix → lint)
make setup-remove-sample-api

# Rebuild the DB from the now sample-free migrations and re-dump the schema,
# so the dropped `users` table does not linger in models.gen.go
make db-init-local db-init-test
make gen-query
```

Notes:

- The core tables `idempotency_keys` (migration `000001`) and `outbox` (migration `000002`) are **kept**; every other migration goes with the sample it belongs to, `prefecture` included.
- `gen-query` regenerates Go models from a `pg_dump` of the **live** DB. If you skip the DB rebuild above, the still-present `users` table is re-dumped and a stale `Users` type is regenerated into `models.gen.go` — the rebuild + re-`gen-query` is what actually drops it.
- Shared generated files (`*.gen.go`, `openapi.gen.yaml`, etc.) are not deleted directly — they are refreshed by the regeneration step. The published docs (`docs/openapi/**`, `docs/godoc/**`, `docs/coverage/**`, `docs/portal/**`) and the code graph (`graphify-out/**`) are not refreshed by it, and still describe the sample until you regenerate them (`make gen-all-docs`).
- The sample spans several full-stack domains, each declared as its own block in `sample-manifest.ts`. When you extend one, append its new paths to the matching block, and wrap any sample lines interleaved in a shared file with `// sample-api:begin` … `// sample-api:end` (or a trailing `// sample-api:line`; `%%` inside a mermaid fence, `<!-- -->` in Markdown prose). They are then covered by the same command automatically.

### Rules keep their examples only until you remove the sample

Several rules in `docs/rules.md`, `docs/adr/**`, and the layer `README`s are stated in general terms
and then illustrated with a concrete example taken from the sample. **The rule survives removal; the
example does not.** What is left is a correct statement that no longer shows a reader what it looks
like in their own system.

Each of those places carries an HTML comment immediately above the removed line, stating **why an
example is needed there, what the example has to demonstrate, and how to write a replacement**. Find
them and work through the list:

```bash
grep -rn "撤去後にこの箇所へ自分の例を置くための指針" docs/ internal/ pkg/
```

This is not cosmetic. An abstract rule with no example is the form a rule takes right before people
stop applying it: every reader has to decide alone what it covers, and they decide differently. The
comments exist so that decision is made once, by you, with the original intent still in front of you.

The business vocabulary has its own home. The term table in
[`docs/spec/glossary.md`](../spec/glossary.md) is emptied by the same removal, and the rules for
filling it back in stay on the page.

<details>
<summary>Manual procedure (reference — no longer required)</summary>

1. Remove sample API definitions from [openapi.yaml](../../openapi/openapi.yaml)
    - Remove Path definitions under `サンプルAPI用のパス` and delete the referenced YAML files.
    - Remove Parameter definitions under `サンプルAPI用のパラメーター定義` and delete the referenced YAML files.
    - Remove Schema definitions under `サンプルAPI用の型定義` and recursively delete the referenced YAML files.
2. Remove sample API Controller and Usecase
    1. Run `make gen-api` to regenerate code and remove sample API Controller code.
    2. Delete Usecase files referenced by the sample API and their test files.
        - Also delete mock files.
    3. If there are files causing errors in [internal/integration](../../internal/integration/), delete those files as well.
    4. Delete handler files and test files affected by the absence of generated sample API code.
    5. If reference errors occur in the Infra layer (QueryService or CommandService interface errors), remove files used by the sample API and their test code from those interfaces.
3. Remove sample API Infra code
    1. Run `make db-test-migrate-down` and `make db-local-migrate-down` to reset the DB to a clean state.
    2. Delete execution SQL in `dml`.
        - Delete directories under [database/dml/repository](../../database/dml/repository).
        - Delete directories under [database/dml/query_service](../../database/dml/query_service).
        - Delete directories under [database/dml/command_service](../../database/dml/command_service).
    3. Run `make gen-query` to regenerate SQLC code and remove sample SQLC code.
    4. Remove sample Infra code and its test code that now cause errors.
4. Remove sample API domain code
    - Delete code used by the sample API and its test code under [internal/domain/](../../internal/domain/). Since directories under this path contain only sample domain code, you may delete entire directories.

</details>

## Phase 16: Decide your own ADR regime

The upstream's own ADR conventions were removed along with everything else that rested on this
repository being a boilerplate. What is left is a decision only you can take, because it is about
how your project records its own history rather than how this one shipped.

What you inherit is [docs/adr/README.md](../adr/README.md) as written: an ADR is an immutable
record, and a decision that changes is replaced by a new `accepted` ADR while the old one is marked
`superseded`. That is the ADR form as [MADR](https://adr.github.io/madr/) defines it, and what
[ADR-0000 (record-architecture-decisions)](../adr/0000-record-architecture-decisions.md) decided.

If you want in-place amendment instead — a legitimate choice for a design document that is shipped
rather than lived — record that as a decision of your own, in your own ADR. Do not infer it from
the fact that the upstream did it.

## Phase 17: Decide whether to keep the DAST setup

The DAST setup is already done. [`.github/workflows/zap-api-scan.yaml`](../../.github/workflows/zap-api-scan.yaml)
boots this application inside a GitHub-hosted runner and drives an authenticated
[OWASP ZAP](https://www.zaproxy.org/) API scan at it, with the endpoint list taken from the OpenAPI
definition. It runs weekly and on demand, writes to code scanning, and never fails a build on a
finding. If you want dynamic scanning, you can use it as it stands — nothing else needs wiring.

**The configuration values describe the API that shipped with this repository, not yours.** The
alert thresholds in [`.github/zap/rules.tsv`](../../.github/zap/rules.tsv), the identity the scan
authenticates as, and the surface the scan reaches were all derived from what that API answers under
the `dast` environment profile — which, unlike `ci`, verifies a real JWT signed by the mock auth
server instead of accepting a stub bearer token. None of it is a claim about your API: your endpoints differ, and so
does what counts as an accepted finding. Read the workflow header before the first scheduled run and
re-derive both files against the API you actually have — an inherited `IGNORE` is a finding nobody
will ever see again.

**Two values name what the scan reaches, and Phase 15 resets both.** Before ZAP starts, the
workflow probes one protected operation (`PROBE_PATH`) as one identity (`SCAN_SUBJECT`) to prove the
credential is accepted — because a scan whose credential is rejected collects 401 everywhere and
still reports a completed run. Both values point at the API this repository shipped with, so
removing it takes them with it: `PROBE_PATH` becomes `/health` and `SCAN_SUBJECT` is emptied.

`/health` answers without a credential. It keeps the workflow runnable when no protected operation
exists yet, and it proves nothing — **the run stays green while the scan sees only the
unauthenticated surface.** The step says so on every run as a warning rather than passing in
silence, but a warning nobody reads is the same as no warning. **Point `PROBE_PATH` at a protected
operation of yours and `SCAN_SUBJECT` at an identity your seed data actually has, as soon as you
have either.** Until you do, DAST is running but not testing what it exists to test.

If you do not want it, delete [`.github/workflows/zap-api-scan.yaml`](../../.github/workflows/zap-api-scan.yaml)
and [`.github/zap`](../../.github/zap), then drop the scanner action's entry from
[`.github/actions-pin.toml`](../../.github/actions-pin.toml) and the `zap-api-scan.yaml:dast` job
section from [`.github/egress.toml`](../../.github/egress.toml) — `make pin-actions-check` and
`make egress-check` both fail on an entry no workflow claims, so neither one can be forgotten
silently. There is no enable / disable switch and there will not be one: keeping it means keeping
it, and a scanner left configured-but-off is one nobody reads and nobody maintains.

The `dast` environment profile is not part of that deletion. It names an execution context rather
than a scanner, so `env/.env.dast` and the `EnvDast` branches stay useful to anything else that
needs the real authenticator against a mocked provider.

## Phase 18: Decide whether to keep the credential-bearing scanners

Two scanners here need something this repository cannot supply on its own. One wants a token for
a vendor's service — [`sonarqube.yaml`](../../.github/workflows/sonarqube.yaml) —
and [`code-ql.yaml`](../../.github/workflows/code-ql.yaml) needs GitHub Advanced Security, which is
free for a public repository and billed for a private one. Both are free here because this
repository is public. Yours may be neither public nor willing to pay.

Nothing is broken while you decide. Each checks for what it needs before it scans and skips
itself when it is absent, leaving the run green — a missing credential is a setup gap, not a scan
result. The same path covers pull requests from forks, which never receive repository secrets at all.

### To keep them

Register the secret on the repository, and create the matching project on the vendor's side:

| Secret | Vendor-side setup |
| --- | --- |
| `SONAR_TOKEN` | Create the organization and project on [SonarQube Cloud](https://sonarcloud.io), then turn **Automatic Analysis off** — leaving it on makes the server analyse the repository itself and collide with the CI analysis |

`sonar-project.properties` identifies a project on SonarQube Cloud, so it is boilerplate identity
rather than configuration. Phase 5's `make setup-replace-repository-reference` rewrites it along with
the README and OpenAPI references; if you skipped that phase, rewrite `sonar.projectKey` and
`sonar.organization` by hand before the first scan, or the analysis is sent to the template's project.

### To remove them

```bash
# Preview: writes nothing and commits nothing.
DRY_RUN=1 make setup-remove-licensed-scanners

# Remove. The working tree must be clean — the script commits as it goes.
make setup-remove-licensed-scanners
```

The script deletes both and **commits each product separately**, so a licence you already hold
is one `git revert` away:

```bash
git revert <the "CI: SonarQube Cloud のワークフローを撤去する" commit>
```

Two things to know about that undo. The README rows come off in a single final commit rather than
with each product, because the two occupy adjacent rows of the same tables and a per-product doc
edit would make every revert but the last one conflict — so a restored scanner works but is no longer
documented. And a lockfile entry shared between scanners is deleted by whichever commit removed its
last user, so a revert can leave `make pin-actions-check` red on an unregistered reference;
`make pin-actions-resolve` puts it back and the check names the entry.

Removing the workflows does not remove the secret. Delete `SONAR_TOKEN` from the repository
settings yourself — a token nothing uses is still a token someone can steal.

There is no enable / disable switch, for the reason Phase 17 gives: a scanner left configured-but-off
is one nobody reads and nobody maintains. `bearer.yaml` is deliberately outside this set — its
Elastic License 2.0 costs nothing to run in CI and constrains only redistribution as a service, which
is a different question with its own answer in
[the workflows README](../../.github/workflows/README.md#bearers-licence-and-removal).
