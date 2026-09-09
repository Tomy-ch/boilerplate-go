---
status: accepted
date: 2026-07-04
deciders: [maintainers]
tags: [contract, openapi, tooling]
---

# ADR-0013: Author the spec in modular Redocly files, bundle, then generate

## Status

accepted

## Context

[ADR-0012](0012-openapi-first.md) establishes that OpenAPI is the single source of truth
for the wire contract. As the spec grows it becomes impractical to maintain a single flat
YAML file: path, schema, parameter, and response objects are all mixed in one place,
making code review, reuse, and navigation difficult. A bundled single-file output is still
required downstream for oapi-codegen (code generation) and Redoc (documentation
rendering).

## Decision

Author the spec as a **modular Redocly project** split across dedicated directories, with
`openapi/openapi.yaml` as the entry point and `$ref` pointers to files under `paths/`,
`components/schemas/`, `components/parameters/`, `components/requests/`, and
`components/responses/`. One file equals one responsibility (one schema per file, one
endpoint per file, one parameter per file).

The build pipeline is:

1. **Lint** — `redocly lint openapi/openapi.yaml` validates the modular sources against
   `redocly.yaml` (naming conventions, required metadata, no unused components).
2. **Bundle** — `redocly bundle openapi/openapi.yaml -o openapi/openapi.gen.yaml` resolves
   all `$ref` pointers into a single flat file.
3. **Generate** — oapi-codegen reads `openapi/openapi.gen.yaml` to produce Go code (see
   [ADR-0014](0014-oapi-codegen-strict-server.md)).
4. **Docs** — `redocly build-docs openapi/openapi.yaml --output docs/openapi/index.html`
   generates the static API documentation.

All `$ref` values must use relative paths (e.g. `../components/schemas/UserResponse.yaml`);
inline component references (`#/components/...`) are forbidden because Redocly's bundler
requires relative-file refs to resolve correctly.

## Consequences

### Positive Consequences

- Split files are independently reviewable and reusable via `$ref`.
- Naming conventions (camelCase body fields, camelCase parameters, PascalCase
  `operationId`) are enforced at lint time before code generation runs.
- `make gen-api` is the single command that runs bundle, docs, and codegen in sequence; `redocly lint` runs separately via `make oapi-lint` (also a CI gate).
- Documentation is generated from the same source as the code.
- Because handler code is generated from the bundled spec, the YAML definition always precedes the implementation; drift between definition and implementation cannot flow undetected into production.

### Negative Consequences

- Contributors must learn the Redocly split-file convention and always use relative `$ref`
  paths — a non-standard authoring style compared to a flat OpenAPI YAML.
- The `openapi.gen.yaml` bundled file is generated output and must never be edited by hand;
  this can cause confusion when the two files diverge during a partial edit. (The CI
  `gen-oapi-artifacts-check` workflow detects divergence and fails the PR if the committed
  file is stale, making manual-edit confusion discoverable before merge.)

## Alternatives Considered

### Single flat OpenAPI file

Avoids the Redocly toolchain dependency. Rejected because a single file becomes
unmaintainable at scale and prohibits per-object reuse via `$ref`.

### Inline `$ref` using JSON pointer fragments

`$ref: '#/components/schemas/UserResponse'` keeps everything in one file and is the
standard OpenAPI practice. Rejected because the Redocly bundler requires separate files to
resolve relative `$ref` pointers correctly across the modular structure.

## Notes

- Build targets defined in [`.makefiles/openapi/gen.mk`](../../.makefiles/openapi/gen.mk).
- Lint rules and naming-convention enforcement defined in [`redocly.yaml`](../../redocly.yaml).
- Modular directory structure described in [`openapi/README.md`](../../openapi/README.md).
- `openapi/openapi.gen.yaml` is generated output — do not edit by hand (see the generated-file rules in [`docs/rules.md`](../rules.md)).
- Parent decision: [ADR-0012](0012-openapi-first.md) (OpenAPI-first).
