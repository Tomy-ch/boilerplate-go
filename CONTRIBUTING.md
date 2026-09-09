# Contributing

This page covers **how a change travels** — where to branch from, how to commit, what has to be
green, and what a reviewer will ask. It states no architectural rules of its own: those live in
[docs/rules.md](docs/rules.md) and the per-package `README.md`, and a change that disagrees with
them is a design discussion, not a review comment.

If this is your first change here, read [docs/architecture.md](docs/architecture.md) and
[docs/development-flow.md](docs/development-flow.md) first. The flow document is the one that says
where to start for each kind of change — an API change begins in the OpenAPI document, a schema
change in a migration, and neither begins in Go.

## Before you start

Set the repository up as described in
[docs/get-started/setup-repository.md](docs/get-started/setup-repository.md), and confirm the git
hooks are wired (`make activate-tools`). Hooks that were never installed are the most common reason
a pull request fails on something that would have been caught in a second locally.

When something in that setup does not behave, check
[docs/get-started/troubleshooting.md](docs/get-started/troubleshooting.md) before digging — several
of the failures are the environment working as designed.

**Install the agent configuration too** (Phase 3 of that page). AI-assisted development is this
repository's standard path: the flows in `docs/development-flow.md` have an executable form as skills
under `.claude/` / `.codex/`, and the conventions a reviewer will ask about are the ones those skills
already apply. Working without them is allowed but is a compatibility path — you perform by hand what
a skill would otherwise drive, and no manual equivalent is maintained alongside it. What is *not*
optional is the outcome: every gate below is the same for a change made either way, and a
deterministic check outranks whatever an assistant reported. Rationale:
[ADR-0007 (agents-md-operational-contract)](docs/adr/0007-agents-md-operational-contract.md).

## Branching

Feature branches are cut from the **latest `release/*` branch**, never from a protected branch and
never from a stale base. `make base-branch` resolves which one that is from the remote's live state;
a base picked by memory is how a branch ends up missing files everyone else already has.

Name the branch after what it does, and include the issue number when there is one:

```txt
feature/1234-add-authentication-check
feature/add-authentication-check
```

Direct commits to `production` / `staging` / `develop` / `release/*` are rejected by branch
protection. When your base moves ahead while you work, **merge** it into your branch rather than
rebasing — the full rule, including how to resolve conflicts in generated artifacts, is in
[docs/rules.md](docs/rules.md).

## Commits

Each commit message starts with a type prefix, and `commitlint` enforces the set on
`commit-msg`:

```txt
Feat | Fix | Refactor | Perf | Docs | Test | Build | CI | Chore | Style | Revert
```

Only the prefix and a non-empty subject are enforced mechanically. Everything else is convention:
one scope per commit, so that a revert is a decision about one thing, and a subject that says what
changed rather than which files moved. The commit messages in this repository are written in
Japanese; the enforced part is language-independent, so a project created from this template sets its own.

## Before pushing

The gates that run on `pre-commit` and `pre-push` are the same ones CI runs, so the fastest review
is one where you ran them first:

```sh
make go-fix     # format + lint auto-fix
make go-lint    # the authoritative golangci-lint gate
make go-test    # tests with coverage (run `make db-init` first)
make gen     # only when you changed OpenAPI, SQL, or anything else generated
```

How much of this actually runs on your machine is sized from the number of open worktrees — past a
threshold the heavy gates are deferred to CI and re-run there identically. `make load-status` says
which band you are in.

A change is done when it is **tested**, not when it compiles. The coverage bar, the layer-by-layer
expectation, and the runtime verification that unit tests cannot replace are defined in
[docs/rules.md](docs/rules.md) (*Testing & Definition of Done*) and
[docs/testing-conventions.md](docs/testing-conventions.md).

## Pull requests

Fill the three sections the template asks for — what changed, the change itself, and how to verify
it. The third is the one reviewers rely on: a reviewer who cannot reproduce your verification is
reviewing the diff alone.

- **Required checks** must pass; the promotion gates into deploy branches are stricter than the
  per-pull-request ones, and both are listed in
  [.github/workflows/README.md](.github/workflows/README.md).
- **CODEOWNERS review** is required, and is what makes "this decision belongs to a role" real rather
  than advisory.
- **Generated files are never hand-edited.** CI regenerates them and fails on a diff, so an edited
  artifact does not survive review.

## When a change needs a decision record

A change that alters *why* the system is shaped the way it is — a boundary moves, a technology is
replaced, a constraint is dropped — belongs in an ADR under [docs/adr/](docs/adr/README.md) as well
as in code. The threshold is recurrence: a decision that will be re-litigated, or that a later
reader will otherwise have to reverse-engineer from the diff.

Documentation follows the code in the same change. Canonical documents are English, each paired
with a Japanese translation (`*.ja.md`); the rules that keep the pair and the documentation portal
consistent are in [docs/maintenance/docs-structure.md](docs/maintenance/docs-structure.md).

## Security

Do not report a vulnerability through an issue or a pull request. Use the private route in
[.github/SECURITY.md](.github/SECURITY.md).
