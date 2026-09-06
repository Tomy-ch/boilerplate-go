# Tools Container

This Dockerfile provides **code generation and bundling tool containers** for the project. It uses multi-stage builds to offer Go, Node.js, and Python tool environments.

## Role

`docker/tools/Dockerfile` packages every code-generation / linting / security / documentation tool the build needs into language-isolated runner images, one stage per language. Developers and CI invoke these containers from `make` targets (`make gen-api`, `make gen-query`, `make sql-lint`, etc.) so nobody has to install Go, Node, or Python toolchains locally. This keeps tool versions reproducible across machines and locks generated output to a known toolchain set.

## Build Targets

|Target|Base Image|Covers|
|---|---|---|
|`go_tools`|`golang:1.27.1-alpine`|Go code generation, linting, security scanning, documentation ([tools](#go_tools))|
|`node_tools`|`node:24.19.0-alpine`|OpenAPI bundling, Markdown / commit linting, portal build and script tests ([tools](#node_tools))|
|`python_tools`|`python:3.14.7-slim`|SQL linting, knowledge-graph build ([tools](#python_tools))|

## go_tools

Code generation, linting, security, and documentation tools for Go:

|Tool|Purpose|
|---|---|
|`oapi-codegen`|Generate Go server/types from OpenAPI spec|
|`mockgen`|Generate mocks from Go interfaces|
|`sqlc`|Generate type-safe Go code from SQL|
|`migrate`|Database migration CLI|
|`trivy`|Vulnerability and misconfiguration scanner|
|`actionlint`|GitHub Actions workflow linter|
|`shellcheck`|Shell script linter|
|`hadolint`|Dockerfile linter|
|`gitleaks`|Secret scanner for committed credentials|
|`godoc`|Serve/generate Go package documentation|
|`godoc-static`|Generate static HTML from godoc output|

## node_tools

Tools for OpenAPI document processing and portal generation:

|Tool|Purpose|
|---|---|
|`redocly-cli`|Bundle OpenAPI YAML (`$ref` resolution) and generate HTML docs|
|`markdownlint-cli2`|Markdown linter for docs (`make md-lint`)|
|`@commitlint/cli`|Commit-message linter (`make commitlint`, wired to the `commit-msg` hook)|
|`js-yaml`|YAML processing for portal doc generation scripts|
|`pnpm`|Resolve the two Node packages in the repository, each with its own lockfile and its own `node_modules`: `scripts/` (installed into `/app/scripts/node_modules`) and `docs-viewer/`, which builds the portal frontend into `docs/portal/` (`make gen-portal-build`, `make portal-test`).|
|`tsx`|Run the repository's TypeScript helper scripts (`scripts/**/*.ts`) without a build step|
|`typescript`|Type check those scripts (`make scripts-typecheck`)|
|`vitest`|Unit tests for the scripts' decision logic (`make scripts-test`)|
|`mermaid`|Lets `scripts/mermaid-lint/index.ts` validate ` ```mermaid ` fences with the real parser (`make md-lint`)|
|`linkedom`|Headless DOM that lets `mermaid.parse` run in Node for the Markdown mermaid syntax lint (`scripts/mermaid-lint/index.ts`)|

## python_tools

|Tool|Purpose|
|---|---|
|`sqlfluff`|SQL linter for migrations, DML, and seed files|
|`graphify`|Knowledge-graph build. Only the deterministic half runs here — `graphify update` re-extracts changed code with tree-sitter and needs no model. Semantic extraction over docs is driven by an assistant and never runs in this image|

Unlike the other two stages, the tools themselves do not come from `mise.toml`: mise installs
only `uv`, which then installs each of them from its own lockfile
([`python/sqlfluff.txt`](../../python/sqlfluff.txt) / [`python/graphify.txt`](../../python/graphify.txt))
with `--require-hashes`, so the whole transitive tree is version- and hash-pinned
(see [ADR-0084 (mise-ssot-drift-gate)](../../docs/adr/0084-mise-ssot-drift-gate.md)).

graphify is here for reproducibility rather than speed. Its semantic extraction is driven by a
prompt that ships with the tool, and the cache namespace is a fingerprint of that prompt
([`.agents/graphify/spec-pin.toml`](../../.agents/graphify/spec-pin.toml)); a host-installed
graphify makes which prompt produced a committed artifact a property of the machine that ran it.

## docker-compose Services

```yaml
go_tool_runner:    # target: go_tools,    profile: generate
node_tool_runner:  # target: node_tools,  profile: generate
python_tool_runner: # target: python_tools, profile: generate
```

All tool runners mount the project root to `/app` and run as root.

## Execution

```bash
make gen        # Run all code generation
make gen-api    # OpenAPI bundle + Go code generation
make gen-query  # sqlc code generation
```

## When Adding a New Tool

1. Install the tool in the appropriate Dockerfile stage
2. Add a new service in `docker-compose.yaml` with `profiles: [generate]`
3. Add a Makefile target if needed

## Notes

- Working directory is `/app` for all targets
- Tools are installed in a builder stage and copied to the runtime stage to minimize image size (`go_tools`)
- Tool versions are pinned in `mise.toml` (the version SSOT); update them there so local and CI images stay in sync. PyPI tools are the exception — see [python_tools](#python_tools) above
- The Node dependencies this image installs are declared by `scripts/` and `docs-viewer/` (each with its own `package.json` / `pnpm-lock.yaml` / `pnpm-workspace.yaml`); the build copies each manifest set into the directory it belongs to and installs it there. Neither lives in this directory, so a dependency change is reviewed next to the code that uses it
