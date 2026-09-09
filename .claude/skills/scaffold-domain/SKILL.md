---
name: scaffold-domain
description: >-
  Implement the domain layer (entity / Repository interface / constants / errors / value objects / tests) for one feature, driven by `docs/spec/domain/<pkgpath>.md` (`<pkgpath>` = the package path under `internal/domain/`). The spec is the single source of truth — it does NOT invent fields, methods, or business logic beyond what the spec declares, and on failure leaves TODO comments plus a summary rather than rolling back. Use it as the first implementation step for a new aggregate once its spec exists. Standalone-callable; the first scaffold step when chained from `scaffold-endpoint`. Do NOT use it to implement any other layer (`scaffold-infra-db` / `scaffold-usecase` / `scaffold-controller`) or to write the spec itself (`new-spec-domain`).
---

# Scaffold Domain

Generate the domain layer for a feature based on `docs/spec/domain/<pkgpath>.md`. Produces the entity, Repository interface, value objects, constants, errors, getters, and tests in a single pass.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

- After the domain spec — `new-spec`-scaffolded or hand-written — is complete and verified.
- As the first step of `scaffold-endpoint` (auto-chained).
- Standalone when only the domain layer needs scaffolding.

Do NOT use this skill for:

- Modifying an existing domain package (the skill assumes a fresh aggregate directory).
- Adding only a single new method to an existing entity — edit by hand.
- Writing business logic not declared in the spec.

## What This Skill Reads / Writes

**Reads (always)**:

- `docs/spec/domain/<pkgpath>.md` — single source of truth for what gets generated.
- `internal/domain/README.md` — layer-wide convention (principles, naming, getter style, `ptr.Copy` usage, error wrapping, file separation). **Canonical**: README wins on any conflict with sibling code.
- Existing sibling aggregates under `internal/domain/<sibling>/` — secondary structural template for imports, file layout, formatting style.
- `internal/domain/<aggregate>/` — to verify the directory does not already exist (abort if it does).

**Writes (with confirmation)**:

- `internal/domain/<aggregate>/<aggregate>_domain.go` — entity struct + constructor
- `internal/domain/<aggregate>/<aggregate>_repository.go` — Repository interface with `//go:generate mockgen`
- `internal/domain/<aggregate>/constant.go` — derived constants (min/max boundaries)
- `internal/domain/<aggregate>/error.go` — derived errors (`ErrInvalid<Field>` per field)
- `internal/domain/<aggregate>/<value_object>.go` — one per VO declared in spec
- `internal/domain/<aggregate>/<aggregate>_domain_test.go` — invariant + behavior tests
- `internal/domain/<aggregate>/<value_object>_test.go` — VO boundary tests

**Triggers (via `make`)**:

- `make gen-api` — regenerates Repository mock under `internal/domain/<aggregate>/mock/`.
- `make go-fix` + `make go-test` — final verification.

**Never touches**:

- Other aggregates' directories.
- Anything outside `internal/domain/<aggregate>/`.
- The spec file itself.

## First Step: Resolve Spec Path

This skill **MUST call `AskUserQuestion` immediately after invocation** (unless invoked from `scaffold-endpoint` with the package path already in context):

- Question: 「対象 domain パッケージパス（`internal/domain/` 以下。例: `cart`, `product/category`）」
- Free-text. The skill then resolves the spec path as `docs/spec/domain/<pkgpath>.md` — the spec tree mirrors the package tree, so the target package and the spec path are the same answer.

Standalone alternative: accept an explicit `--spec=<path>` argument so the user can point at a spec outside the convention.

If the spec file is missing → abort with a message pointing to `/new-spec`.
If `internal/domain/<aggregate>/` already exists → abort to avoid clobbering hand-written code.

## Step 1. Read Spec + README Context

1. Read `docs/spec/domain/<pkgpath>.md` in full. Parse every YAML code block into in-memory inventory:
   - `entity`: package, struct, fields (each with name, type, required, min/max, etc.)
   - `cross_field_invariants`: list of constraint expressions
   - `behavior_methods`: list of (name, signature, description)
   - `value_objects`: list of (name, underlying_type, validation, factory, methods)
   - `repository_methods`: list of (name, signature, behavior)

2. Read `internal/domain/README.md` for layer convention (especially "Do / Don't", "Handling time and ID", "Invariants" sections).

3. Use `internal/domain/README.md` as the authoritative convention source — the README's `Implementation notes`, `Aggregate Design`, `Testing strategy`, and `Do / Don't` sections are canonical for naming, getter style, `ptr.Copy` usage, error wrapping pattern, and file separation. Existing aggregate code (`internal/domain/<sibling>/*.go`) is a **secondary** reference for imports / file layout / formatting — if it conflicts with the README, the README wins (this skill never silently follows code that drifts from the README).

If any YAML block fails to parse, abort and surface — point user at `/verify-spec`.

## Step 2. Test-Perspective Subagent

Before writing any code, invoke a subagent via the Agent tool to enumerate the layer's test viewpoints. This ensures the subsequent implementation is test-driven from a domain-appropriate perspective.

- `subagent_type: general-purpose`
- Prompt content (in Japanese): pass the parsed spec inventory + `internal/domain/README.md`'s `Test Strategy` section + a list of expected viewpoints for the domain layer:
  - invariant preservation (constructor + state-transition methods)
  - boundary values for each field (min length / max length / min / max)
  - value object boundary checks
  - immutability (defensive copy for pointer fields)
  - cross-field invariant verification (e.g., `updatedAt >= createdAt`)
  - error classification (`require.ErrorIs` for specific errors)
- Expected output: a structured list of test cases the skill must produce, grouped by entity / VO.

If the subagent returns no viewpoints, fall back to a minimal default set and warn the user.

## Step 3. Derive Auto-Generated Elements

Apply auto-derivation rules to fill in elements that are NOT in the spec but follow convention. These rules map spec → generated elements; the canonical *shape* of the resulting Go (error-wrapping pattern, constant naming, getter / `ptr.Copy` style) lives in `internal/domain/README.md` and wins on any drift:

- **Errors**: for every spec field with validation, generate `ErrInvalid<Field>` in `error.go`. For VOs, generate `ErrInvalid<VO>`. Wrap them under a single group root `errInvalid` (see README `error.go` example for the exact two-level wrapping).
- **Field identifiers + collect-all validation**: for every user-correctable input field (fields a client submits and can fix), generate a `Field<Name> = "<property>"` constant in `constant.go` matching the API request property name, and shape the input-field validation to collect **all** failures — append `xerrors.Wrap(ErrInvalid<Field>, msg)` and `Field<Name>` per failing field, then `return apperror.WithDetails(xerrors.Join(errs...), fields...)` — so the API can report every invalid field at once (`details`). Server-internal invariants (id, timestamps, password hash) keep first-error return: they are not user-correctable. The canonical shape lives in the README `Errors` section and the rationale in ADR-0048 (error-metadata-code-message-details); reason texts stay in the wrapped message (log-only), never in the identifiers.
- **Constants**: for every field with `min_length` / `max_length`, generate `min<Field>Length` / `max<Field>Length` constants. For `min` / `max` numeric fields, generate analogous constants.
- **Getters**: for every unexported entity field, generate `func (e *Entity) Field() T { return e.field }` on a single line. For pointer types, use `return ptr.Copy(e.field)`.
- **ID validation**: for `uuid.UUID` fields named `id` or `<x>ID`, add `if id.IsNil() { return nil, xerrors.Wrap(ErrInvalidID, "...") }` in the constructor.
- **Simple type checks**: for `string` fields with min/max length, generate `if ok, msg := stringkit.ValidateInRange(field, minXLength, maxXLength); !ok`. For nullable string fields, allow nil but apply the range when present (`if x != nil { if ok, msg := stringkit.ValidateInRange(*x, ...); !ok { ... } }`).

## Step 4. Plan and Confirm

Display a Japanese summary of files to be created and a brief content preview (first ~10 lines of `<aggregate>_domain.go`). Then ask:

- Question: 「以下の構成で domain 層を生成しますか？」
- Options: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. Write Files

Write in this order so cross-references inside the package are consistent:

1. `constant.go` (no deps)
2. `error.go` (no deps)
3. `<aggregate>_domain.go` (entity + constructor, depends on constants/errors)
4. `<aggregate>_repository.go` (Repository interface with `//go:generate mockgen`)
5. `<value_object>.go` per VO (depends on errors)
6. `<aggregate>_domain_test.go` (uses subagent's viewpoint list)
7. `<value_object>_test.go` per VO

Each file follows the existing aggregate's style (read in Step 1 #3) for imports, comments, formatting.

## Step 6. Regenerate Mocks

```sh
make gen-api
```

This processes the `//go:generate mockgen` directive in `<aggregate>_repository.go` and produces `internal/domain/<aggregate>/mock/mock_<aggregate>_repository.go.gen.go`. Verify the mock file exists after the command runs.

## Step 7. Verify

```sh
make go-fix    # absorb formatting
make go-test   # compile + tests + coverage
```

Check the coverage line for `internal/domain/<aggregate>`. Domain tests must reach 100% per the project's testing convention. If coverage dropped, identify untested branches (likely an invariant or VO factory path) and request a follow-up test addition.

If `make go-test` fails:

- Surface the failure to the user.
- Write a TODO comment in the failing file at the relevant location.
- Do NOT auto-rollback. Print FB summary and ask the user to fix forward.

## Step 8. Closing

Print a one-line summary:

```text
<Aggregate> domain 層を生成しました。<N> ファイル作成、make go-test OK、coverage 100%。
次は scaffold-infra-db で repository 実装、または scaffold-endpoint で残層を続行できます。
```

Do NOT commit. Do NOT trigger the next scaffold skill.

## AI Modification Scope

Per the "Exception: Skill Execution" clause in `CLAUDE.md` / `AGENTS.md`:

- Write scope: `internal/domain/<aggregate>/` only.
- The skill will refuse to start if `internal/domain/<aggregate>/` already exists (to avoid clobbering).

Remains protected:

- Other aggregates' directories.
- `docs/spec/` files (read only).
- Generated mock files are written via `make gen-api`, never hand-edited.

## Constraints

- ❌ Add comments that restate the code or explain *why* a choice was made — keep code comments minimal (behavior / contract only); rationale belongs in the commit message / README, not the code. One-line declaration godoc stays (even on unexported symbols). **Volume counts too**: what this skill scaffolds is idiomatic by construction, so a multi-line explanation of a constructor / Params struct / row-to-entity mapping / handler template is noise. State the contract in one line and stop; never restate a repo-wide rule `docs/rules.md` already carries. Suppression, not elimination — a genuinely non-obvious Why still stays.
- ❌ Invent fields, methods, errors, or constants not in the spec
- ❌ Hardcode the layer convention — always read `internal/domain/README.md` + an existing aggregate as template
- ❌ Skip the test-perspective subagent (Step 2)
- ❌ Skip the spec-confirmation `AskUserQuestion`
- ❌ Overwrite an existing aggregate directory
- ❌ Hand-edit the mock file
- ❌ Auto-rollback on failure (use TODO + FB instead)
- ✅ Japanese user-facing output and Japanese test case names (per CLAUDE.md output rule)
- ✅ Apply auto-derivation rules (Step 3) so the spec stays minimal
- ✅ Reach 100% coverage for the domain package (per project convention)
- ✅ Run `make gen-api` to regenerate the mock

## Checklist

Before reporting completion, confirm:

- [ ] Spec path resolved and file exists
- [ ] Aggregate directory does NOT already exist (skill aborts if it does)
- [ ] Spec read + all YAML blocks parsed successfully
- [ ] `internal/domain/README.md` (canonical) + existing sibling aggregate (secondary template) read
- [ ] Test-perspective subagent invoked; viewpoints captured before any write
- [ ] Auto-derived constants / errors / getters / ID checks computed from spec
- [ ] Plan displayed and confirmed via `AskUserQuestion`
- [ ] Files written in dependency order (constant → error → entity → repository → VO → tests)
- [ ] `make gen-api` executed; mock file present
- [ ] `make go-fix` + `make go-test` run; coverage 100% confirmed (or failure surfaced with TODO + FB)
- [ ] No commits / pushes
- [ ] Final summary in Japanese
