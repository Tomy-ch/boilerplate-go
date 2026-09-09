# go-boilerplate Copilot Instructions

This document defines how GitHub Copilot must operate in this repository.
Project rules and architectural constraints are defined in AGENTS.md.
Copilot must follow them strictly.

## Repository Overview (Reference Only)

Refer to AGENTS.md for:

- Architecture rules
- Layer responsibilities
- Modification scope
- Strict constraints

Do not duplicate architectural rules here.

## AI Modification Scope

Copilot may modify only:

- `internal/`
- `pkg/`
- `database/dml/**`
- `database/migrations/**` (new files only)
- `openapi/**`

Do NOT modify:

- `cmd/`
- `docker/`
- `scripts/`
- `docs/`
- `vendor/`
- `makefile`
- `.github/workflows/`
- `.github/actions/`

Unless explicitly instructed.

## Required Workflow Before Implementation

### 1. API Changes

- Modify OpenAPI source (`openapi/**/*.yaml`)
- Run: `make gen-api`
- Update handler implementation
- Update usecase

### 2. Database Changes

- Create new migration file (never modify existing ones)
- Update SQL in `database/dml/**`
- Run: `make gen-query`
- Update repository implementation

### 3. Business Logic Changes

- Modify usecase
- Adjust domain if required
- Do NOT expose domain entities outside usecase

## Generated Code Rules

Never edit:

- `*.gen.go`
- `*.sql.go`
- `openapi.gen.yaml`

Always regenerate instead.

## Testing Workflow

Before writing tests:

    make gen-api
    make gen-query

After implementation:

    make go-fix
    make go-lint
    make go-test

New or modified packages must exceed 90% coverage.

## Git Rules

Feature branches must be created from the latest active release branch.

Do not branch from develop, staging, or production.

Protected branches:

- `production`
- `staging`
- `develop`
- `release/*`
- `hotfix/*`

The following rules are enforced by repository branch protection:

- Direct commits are strictly prohibited.
- Force pushes and rebases are prohibited.
- Branch deletion is prohibited.
- Pull requests are mandatory.
- At least **one approval** is required before merge.
- Any new push dismisses previous approvals.
- The latest push must be approved.
- All review threads must be resolved before merging.

Copilot must:

1. Always create a feature branch.
2. Never attempt to merge without approval.
3. Never attempt to bypass branch protection.
4. After amending a PR branch, STOP and wait for human review.
5. Never use rebase, force-push, or history rewriting unless explicitly instructed.

Branch naming convention:

- `feature/<issue>-short-description`
- `bugfix/<issue>-short-description`

## Language Rule

All outputs, including pull request reviews, comments, and suggestions, MUST be written in Japanese.

Do NOT output English unless explicitly requested by the user.
