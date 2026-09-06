# scripts

`scripts/` contains **utility scripts** for code generation, documentation, versioning, and initial project setup.

## Directory Structure

One directory per tool, named after what it does; `lib/` holds what more than one of them needs.
Node configuration and the cross-package gates sit at the top level. What each tool is for is in
*Script Categories* below — that is the part a name cannot carry.

## Script Categories

### Documentation Generation

|Script|Description|Invoked By|
|---|---|---|
|`portal/gen-portal-docs.ts`|Copy source docs to portal `guides/` based on `manifest.yaml`|`make gen-docs`|
|`portal/gen-docs-json.ts`|Generate `docs.json` navigation for the portal app|`make gen-docs`|

### Linting

|Script|Description|Invoked By|
|---|---|---|
|`marker-baseline/`|Pin the number of removal-marker (`boilerplate-only` / `sample-api`) lines per file in `baseline.json`, and fail when the count moves. A marker that fires and a marker shown as an example look identical, so the removers carry a `MARKER_LITERAL_FILES` declaration for the latter; forgetting it either aborts the removal (loud) or silently deletes the illustrated passage (not loud — an emptied code fence is valid Markdown). An added marker line is the only signal either way, so it is made a decision: update the baseline, or declare the file. Regenerate with `tsx scripts/marker-baseline --write`.|`make test` (vitest) <!-- boilerplate-only:line -->|
|`premise-lint/`|Mechanises the *No premise the document will outlive* rule from [docs/rules.md](../docs/rules.md). Reads every Markdown file that survives template instantiation — `docs/adr/**`, `docs/design/**`, `docs/rules.md`, the layer READMEs, … — with the marked regions removed, and fails on a self-reference that stops being true once a repository is created from the template. A premise belongs in `README*` / `docs/get-started/**`, which the setup rewrites or deletes, or inside a `boilerplate-only` / `sample-api` marker. Other senses of the same words are declared with a reason in `allowances.ts`.|`make md-premise-lint` <!-- boilerplate-only:line -->|
|`mermaid-lint/`|Extract every ` ```mermaid ` fence from the repo's Markdown (same exclusions as `markdownlint-cli2`) and validate each with the real `mermaid.parse` (DOM provided by `linkedom`). Reports every broken diagram, then exits non-zero if there was any. Fills the gap that `markdownlint` only checks Markdown shape, never the diagram grammar.|`make md-lint` / `make md-mermaid-lint`|
|`skill-lint/`|Check the skill / agent definitions under `.claude/**` semantically: frontmatter (`name` matches the directory / file name, `name` + `description` present), translation pairs (`SKILL.ja.md` exists, carries no frontmatter, opens with a sync note, and its heading-level sequence matches `SKILL.md`), reference existence (every `` `make <target>` `` resolves against `Makefile` / `.makefiles/**`, every repo-root-relative path in inline code exists), and cross-skill section references (`` `repo-ops` §19 `` resolves against the numbered sections `repo-ops/SKILL.md` actually declares — this one reads `.codex/**` too). Also checks that each skill / agent exists in `.codex/**` too. Fills the gap that a skill definition is an agent instruction sheet whose prose nothing else checks against reality, and that a skill landing on only one of the two AI environments goes unnoticed. See [Skill Lint](#skill-lint) for scope and the ignore directive.|`make md-lint` / `make md-skill-lint`|
|`openapi-client-check/`|Run the frontend generator (`orval`) over the bundled spec (`openapi/openapi.gen.yaml`) into `tmp/openapi-client/` and fail unless the SSE contract types (`DeliveryEvent` / `ControlEvent` / `StreamCursor`) appear in the output. Fills the gap that `redocly lint` and Spectral judge the spec as a document, never whether a consumer's generator can turn a component into a type — the only place a server-side contract break would otherwise surface is the client's generated validation ([openapi/boundary-ownership.md](../openapi/boundary-ownership.md)). An empty or declaration-less output is reported as degenerate (exit 2), not as "all types present".|`make openapi-client-check` / CI `oapi-lint.yaml`|
|`doc-ref-lint/`|Check ADR filename/H1/reference consistency and the existence of English/Japanese documentation pairs. Every ADR reference carries its filename slug with the number, so a renumbering cannot silently retarget it. `docs/spec/**` is intentionally excluded from translation existence checks until its Japanese specification set is introduced.|`make md-lint` / `make md-doc-ref-lint`|
|`actions-shellcheck/`|Parse every `action.yaml` / `action.yml` under `.github/actions/**`, extract `runs.steps[].run` from the composite ones and check each script with `shellcheck` over stdin, remapping every finding back to its line in the `action.yaml`. Fills the gap that `actionlint` walks only `.github/workflows` and cannot be pointed at an action manifest (handed one directly, it parses it as a workflow and fails), so the shell inside a composite action was checked by nothing. The dialect comes from the step's `shell:` — passed to shellcheck as a shebang, which also settles the target shell without a `-s` flag; `pwsh` / `python` / `cmd` and an expression-valued `shell:` are counted as skipped instead. `${{ }}` expressions are masked to a placeholder that preserves the line count, the same approach `actionlint` takes for workflow `run:`. Per file, the number of extracted steps must equal the number a plain decode of the same YAML counts, and a mismatch exits non-zero — the two routes break independently, so a broken extractor cannot pass as a clean run; a `run:` written as a folded scalar (`>`) is rejected outright, because folding drops the line breaks a finding's position is mapped back through. Masking is also the reason this script says nothing about whether an expression was quoted — that question survives the mask only for a checker that reads the interpolation site itself, which is `make actions-zizmor`'s job.|`make actions-lint` / `make actions-shellcheck`|
|`pr-comment-secret-lint/`|Split every workflow in `.github/workflows/` into jobs and fail when a job using `./.github/actions/upsert-pr-comment` references a secret other than `GITHUB_TOKEN`, workflow-wide `env:` included. Enforces a rule `actionlint` cannot express — see [`.github/workflows/README.md`](../.github/workflows/README.md) for why the rule exists. Reach: direct `secrets` references inside a `${{ }}` expression, whether `secrets.NAME`, `secrets['NAME']`, or the whole context (`toJSON(secrets)`); a secret read in one job and handed on through `needs.<job>.outputs` is beyond static reach and passes.|`make actions-lint` / `make actions-comment-secret-lint`|
|`pr-comment-fence-lint/`|Fail when a workflow's `run:` block emits a fixed-length Markdown fence around a PR comment body, when the duplicated `fence_for` helpers stop agreeing with each other, and when a workflow that passes a body through interpolates a value into an inline code span. Enforces rules `actionlint` cannot express — see [`.github/workflows/README.md`](../.github/workflows/README.md) for why a fence must be sized from the text it wraps. Reach: literal fences in an `echo`, textual equality between the helper implementations, and a span written literally around a shell expansion — one built through a variable or assembled by `jq` is invisible here, and whether a given body is attacker-controlled is not decidable at all; both are left to the rule. The span check is file-scoped and keeps an exclusion map for a workflow whose body is not yet on a safe path: an entry names the issue tracking it, is printed on every run so a skipped file cannot pass for a checked one, and goes away when that issue is fixed.|`make actions-lint` / `make actions-comment-fence-lint`|
|`actions-cutoff-lint/`|Fail when a job carries no `timeout-minutes`, and when a step calling `./.github/actions/upsert-pr-comment` has an `if:` a cancelled job cannot reach or a `title:` with no cut-off heading. Enforces rules `actionlint` cannot express — see [`.github/workflows/README.md`](../.github/workflows/README.md) for what a cut-off has to leave behind and why the three are one check. Reach: `always()` / `cancelled()` in the condition, `failure()` deliberately not counting since it is false for a cancelled job; the literal `CUT OFF` in the title expression; jobs calling a reusable workflow are skipped because the key is invalid there. Structure is read by column rather than by a YAML parser, which holds because a block scalar's body is always more indented than its key — `actionlint` runs first in the same target and guarantees the input parses at all. A condition that negates its own reachability (`!always()`) is writable and not statically caught — the rule is what holds.|`make actions-lint` / `make actions-cutoff-lint`|

#### Skill Lint

`skill-lint/` only asserts what can be derived mechanically from the Makefile target list, the
filesystem, and heading extraction — it never judges wording. Reference checks read **inline code
spans outside fenced blocks** (a fence is an example or a sample output, so it guarantees nothing).

A path reference is checked only when it is unambiguously a path: its first segment is an existing
repo-root entry, and it either ends with `/` or has a dotted basename. That deliberately leaves Go
import paths (`database/sql`), package-qualified symbols (`pkg/ptr.Copy`), ellipses
(`internal/controller/handler/...`), and context-relative filenames (`SKILL.md`) unchecked — none of
them resolve to a unique path. `<placeholder>` / `*` / `**` / `{a,b}` are resolved as patterns, and a
path is also tried relative to the referencing file so a skill can point at its own bundled
`scripts/`.

For a reference that is intentionally absent — a hypothetical example, an optional location, a file
that lives in the counterpart AI tool's repository — put the ignore directive anywhere on that line:

```markdown
- `internal/controller/handler/debug/README.md` → `docs/portal/guides/controller-handler-debug.md` (if it were added) <!-- skill-lint-ignore -->
```

A **cross-skill section reference** — `` `repo-ops` §19 ``, `` `repo-ops` section 19 ``, or
`` `repo-ops` の §19 `` — is resolved against the numbered `## N.` headings that skill's `SKILL.md`
declares. This is the one check that reads `.codex/**` as well as `.claude/**`, and it builds a
separate index per tree: a Codex document's reference points at the Codex copy, so judging it by the
Claude copy's numbering would report a real section as missing while the two are mid-sync.

Unlike a make target or a path, a renumbered section leaves the reference syntactically intact — it
just points somewhere else, which surfaces as an agent executing the wrong step. Two cases are
deliberately **not** judged: a name that is not an installed skill (it may be an unrelated word in a
code span), and a skill that declares no numbered sections at all (it may use a different heading
convention). Requiring the skill name to sit in a code span is what keeps `RFC 7235 section 3` out.

Across the two AI environments the script checks **existence only**: every `.claude/skills/<name>/`
has a `.codex/skills/<name>/` and vice versa, every `.claude/agents/<name>.md` has a
`.codex/agents/<name>.toml` and vice versa, and every Codex skill carries the `SKILL.md` +
`agents/openai.yaml` pair its README documents. Codex-side `SKILL.ja.md` is optional, so it is
checked as a translation pair only when present.

Body correspondence is deliberately **not** checked. `sync-ai` performs a semantic port, not a
verbatim copy, so `CLAUDE.md` ↔ `AGENTS.md` rewording, adaptation of Claude-only mechanisms, and
condensed rewrites leave permanent intentional differences — for a substantial minority of the
shared skills the two heading sets do not overlap at all. Existence parity has few enough
exceptions to declare, and still catches the failure that matters: a skill merged into one
environment only.

A skill that intentionally lives in one environment goes in the `PLATFORM_ONLY_SKILLS` map in
`skill-lint/checks.ts` **with a reason**. An entry with an empty reason fails, and so does one whose skill
has since appeared in both environments (or in neither) — so the exception list cannot outlive the
exception. **This is the canonical description of the mechanism; other documents link here rather
than restating it.**

Agent roles have no such escape hatch: their parity is unconditional. No agent has ever been
deliberately one-sided, so an exception map for them would carry no entries — add the mechanism
when a real case appears, not before.

A skill this repository does not write is excluded from every check above through the
`EXTERNAL_SKILLS` map in `skill-lint/checks.ts`, also with a reason. Those skills are installed from
a pinned package by [`.claude/scripts/bootstrap-external-skills.sh`](../.claude/README.md) and are
gitignored, so their frontmatter, their translation pair, and the paths they reference are all
decided upstream — most of those paths are artifacts that exist only after a run. Checking them
would produce findings nobody here can act on, and a gate that reports unfixable findings stops
being read. Absence from a checkout is normal for these entries, which is why they carry no
staleness check of their own. A path reference *to* one of those landing directories resolves as
present rather than missing — every document describing the arrangement has to name the directory,
and none of them could pass a check that requires it to exist. Only the declared names resolve this
way, so a misspelling still fails.

### Versioning

|Script|Description|Invoked By|
|---|---|---|
|`semver/`|Bump semantic version (patch/minor/major)|Release workflow|
|`stamp-openapi-version/`|Derive `X.Y.Z` from a `release/vX.Y.Z` branch name and write it into `openapi.yaml` `info.version` (first `version:` line only; idempotent; no-op for non-release refs). Contract version only — no SHA / build metadata (commit-level traceability is the runtime `/version`'s job). Runs through `tsx`.|`auto-generate-docs.yaml`|
|`sync-versions/`|Go-based sync utility. Parses `mise.toml` `[tools]` (table-scoped, no external deps) and propagates `go` / `node` / `python` versions to `go.mod` (`go` directive) + `docker/*/Dockerfile` `FROM golang:` / `FROM node:` / `FROM python:` lines. Pre-validates all rules (version present, file exists, expected match count) and writes per file atomically, so failures never leave a partial state.|`make sync-versions`|
|`release/`|Create a release tag (`tag`) or the next release branch (`branch`), deriving the next version from the newest semantic-version `git tag` with `-bump patch\|minor\|major`. The steps live here rather than in a Make recipe because both include operations that cannot be taken back — pushing a tag, creating a GitHub Release, moving the default branch — so exercising the branches for real would mean actually releasing. The sequencing and the abort conditions are pure functions pinned by tests.|`make tag-patch` / `tag-minor` / `tag-major` / `branch-patch` / `branch-minor` / `branch-major` / `hotfix-patch`|
|`base-branch/`|Print the branch name of the latest release line — the branch a feature branch is cut from. The source is `origin`'s live state (`git ls-remote --heads origin 'refs/heads/release/*'`); no local ref is read, because `refs/remotes/origin/HEAD` is fixed at clone time and `git fetch` never updates it, and the GitHub default branch can still point at an earlier release line. Both go stale without warning, which is how a feature branch ends up cut from a generation-old base. "Latest" is the numeric comparison of `major` / `minor` / `patch`, the same basis `release/` uses to choose the next version, so the tool that creates these branches and the tool that resolves them agree: the commit date reorders under a hotfix or a base merge into an older line, and string order puts `v1.10.0` before `v1.9.0`. A remote with no `release/vX.Y.Z` branch is an error rather than an empty answer — a caller cannot tell an empty base from an unresolved one. Scope is `release/*` only, matching the rule this resolves ("cut a feature branch from the latest `release/*`"); a `hotfix/*` branch, which `make hotfix-patch` also makes the GitHub default, is not a candidate.|`make base-branch`|

All other tool versions are managed by [`mise.toml`](../mise.toml) as the single source of truth, except the PyPI tools, which are declared in [`python/`](../python/README.md) and installed from a hash-pinned lockfile. Each environment (host / docker / CI) installs only what it needs via `mise install <tool>` (or `uv pip install --require-hashes`) — no sync script required for those.

### Makefile Support

|Script|Description|Invoked By|
|---|---|---|
|`make-help/`|Parse `.makefiles/*.mk` and display target descriptions|`make help`|
|`load-band/`|Resolve the host load band (`full` / `low` / `ci-first`) and the per-window CPU share from the number of `git worktree`s, emitting them as `KEY=VALUE` for a recipe to `eval` (`env`) or as a human-readable summary (`status`). Resolution happens inside the recipe rather than at make's parse time, so targets that run nothing heavy pay nothing for it. The shell it replaced counted windows with `git worktree list \| grep -c . \|\| echo 1`, which emits `0` *and* `1` when git cannot answer — the comparisons then failed with `integer expression expected` and the band silently degraded to `full`.|`make load-status` / the `gate-*` targets|

### Code Generation

|Script|Description|Invoked By|
|---|---|---|
|`genctxkey/`|Generate Echo context key helpers (Go code generator). Driven by the `//go:generate` directives in `internal/controller/ctxhelper/generate.go`, run via `go generate ./...`.|`make gen-go-code`|

See [genctxkey/README.md](genctxkey/README.md) for details.

### Knowledge Graph (`graphify-*`)

|Script|Description|Invoked By|
|---|---|---|
|`graphify-export/`|Convert the `graphify-out/graph.json` node-link graph into the deterministic `nodes.json` / `edges.json` / `metadata.json` triple: nodes and edges sorted by a stable key, a schema version stamped on each document, and every attribute the upstream format carries preserved rather than dropped. Consumers read the triple instead of the upstream shape, so an upstream format change lands in one place.|`make graphify-export`|
|`graphify-pending/`|Report how much semantic extraction is waiting. The pending set comes from `graphify-out/manifest.json`, where a document is behind when its stored `semantic_hash` no longer matches the file's current MD5 — not when the manifest's two hashes disagree, which they never do until the deterministic half has run and is the mistake that makes the count read zero forever. Size is measured in changed lines rather than files, since a typo fix and a rewritten ADR cost the same to count but not to re-extract; the baseline is the last commit touching `graphify-out/cache/semantic/`, which is written only by semantic extraction and is tracked, so the measure accumulates across pushes without inventing any new state. Exclusions are read from `.graphifyignore` so generated output does not inflate the number through a second list that could drift. Reports only — the extraction itself needs a model, and nothing in CI can start one.|`make graphify-pending` / `graphify-sync.yaml`|
|`graphify-check/`|Verify that the tracked graphify artifacts stay portable and that the tracked cache's provenance is known. Three checks: nothing outside the `.gitignore` whitelist is tracked; no tracked artifact carries an absolute path from the machine that produced it; and the tracked semantic cache sits in the namespace [`.agents/graphify/spec-pin.toml`](../.agents/graphify/spec-pin.toml) pins. The first two are both needed — one alone misses an upstream regression that starts leaking host paths into an already-whitelisted file, the other alone misses machine-local files that happen to contain no path. The AST cache is what makes them real: graphify builds node IDs from the paths it was handed, and its relativizing pass runs after the cache is written, so those entries keep the producing machine's user name; a public repository cannot take such a commit back. The third check exists because graphify namespaces the semantic cache by a fingerprint of the extraction prompt, and that prompt lives in an assistant's skill directory rather than here — without the pin, a cache baked by a different prompt variant is indistinguishable from a correct one. `-spec <path>` fingerprints a prompt file against the pin before an extraction, reimplementing `graphify.cache.prompt_fingerprint` (CRLF normalize, strip trailing whitespace per line, trim, SHA-256, first 12 hex) with cases pinned to values measured from graphify itself. `-base <ref>` adds a fourth check: the diff against `<ref>` must not touch the output directory. That one is about merge behaviour rather than portability — the graph is a single 450k-line blob rewritten whenever any source file moves, so concurrent branches always conflict on it, and regenerating from the source of truth (the usual remedy for a generated artifact) does not apply because the semantic half needs a model. Updates therefore stay on the release line, and when to enforce that is the caller's decision, not this tool's. The one case the tool decides itself is a base that carries no output yet: there is nothing to conflict with, and failing there would leave no route by which the graph could ever be placed for the first time. Nothing tracked is a pass reported in its own words, not as a clean run.|`make graphify-check [SPEC=<path>]` / `pre-commit` hook / `graphify-check.yaml`|

### CI / Supply Chain

|Script|Description|Invoked By|
|---|---|---|
|`pin-actions/`|Pin every external GitHub Actions `uses:` in `.github/workflows/**` and `.github/actions/**` to an immutable commit SHA. `resolve` walks the references and resolves each tag/branch to a SHA via `git ls-remote`, writing the lockfile `.github/actions-pin.toml` (SSOT) — with a supply-chain quarantine that refuses commits younger than `PIN_ACTIONS_MIN_AGE_DAYS` (default 14, keeping the existing pin instead). `apply` rewrites each `uses:` to `@<sha> # <tag>` from the lockfile. `check` runs the same comparison without writing and exits non-zero on any unpinned/stale/unregistered reference (for CI / hooks). Idempotent: an already-pinned line re-resolves off its trailing `# <tag>` comment.|`make pin-actions-resolve` / `pin-actions-apply` / `pin-actions-check`|
|`pin-images/`|Pin every `FROM` base image in `docker/*/Dockerfile` to an immutable digest. `resolve` collects each `image:tag` and resolves its current digest via `docker buildx imagetools inspect`, writing the lockfile `docker/images-pin.toml` (SSOT) — with a supply-chain cooldown that refuses digests whose image-config `created` is younger than `PIN_IMAGES_MIN_AGE_DAYS` (default 14). A mutable tag has no queryable history, so the step-back target is the tool's own prior lock entry; with none (bootstrap) the image is left tag-only. `apply` normalizes each `FROM` to `image:tag@sha256:...` from the lockfile, leaving a tag-only line where the lockfile carries no entry (which is how a quarantined image stays unpinned). `check` runs the same comparison without writing and exits non-zero on drift (for CI / hooks). The tag stays inline as the version SSOT.|`make pin-images-resolve` / `pin-images-apply` / `pin-images-check`|
|`egress/`|Generate every job's inline harden-runner `allowed-endpoints` from `.github/egress.toml` (SSOT), where a job declares the capability classes it belongs to (`base` / `mise` / `image` / `db`) plus its own `extra`, and the class definitions hold the hosts. `apply` rewrites the folded block of every `allowed-endpoints:` in `.github/workflows/*.yaml`; `check` runs the same comparison without writing and exits non-zero on drift (for CI / hooks). Fails closed rather than silently: a job whose block is missing from the SSOT, an SSOT entry no workflow claims, an `egress-policy` that disagrees with the SSOT, and a non-host line inside a block are all errors. The step must stay inline (harden-runner runs before checkout, so a composite action cannot hold it), so the SSOT is what removes the duplication instead — see [`.github/workflows/README.md`](../.github/workflows/README.md) § Runner Hardening.|`make egress-apply` / `egress-check`|
|`go-cooldown/`|Check `go.mod` against the supply-chain cooldown window using the publish time the Go module proxy reports (`<module>/@v/<version>.info`), so no extra dependency is needed. `gate` compares against a base ref and fails on a **direct** requirement the change adds or upgrades that was published inside the window; an indirect one is reported instead, since MVS can hold it above a direct dependency's lower bound where the pull request cannot lower it. `audit` inventories every requirement and never fails on the window itself, because existing dependencies are grandfathered. Both fail on a bypass entry in `.github/go-cooldown-bypass.toml` that has expired, reaches beyond three months, or matches nothing in `go.mod`, and an invalid entry loses its effect so a lapsed bypass cannot keep letting its module through. Unlike pnpm's `minimumReleaseAge`, Go enforces no window at resolution time — this check is the guard, not a detector for one.|`make go-cooldown-gate BASE=<ref>` / `make go-cooldown-audit`|
|`tool-cooldown/`|Check the tool versions this repository declares — `mise.toml` for everything mise resolves, plus `python/*.in` for the PyPI tools that install from a hash-pinned lockfile ([ADR-0084 (mise-ssot-drift-gate)](../docs/adr/0084-mise-ssot-drift-gate.md)) — against the supply-chain cooldown window. The window comes from the backend, not the tool: 14 days for one resolved through a GitHub release (aqua / ubi / github), matching `pin-actions` and `pin-images` because a tag can be moved onto another commit; 7 days for one resolved through a package registry (go / npm / PyPI), matching `go-cooldown` because a published version there is immutable. The lockfiles themselves (transitive dependencies) are out of scope, for the same reason `go-cooldown` gates only direct requirements. Publish times come from the GitHub Releases API, the Go module proxy, the npm registry and PyPI respectively — a `go:` backend names a package path, so the module path is found by walking the prefix back until the proxy answers. A short name's backend is resolved by asking `mise registry` rather than by a table kept here, which would drift the next time mise changes. **Language runtimes (`core:` backend) are excluded as an accepted risk** — a compromised go / node / python distribution is a failure of the language's trust model rather than of one supply-chain link, and no cooldown protects against it. `gate` compares against a base ref and fails; `audit` inventories everything and never fails on the window. Both also fail when a `python/*.in` declaration and its `python/*.txt` lockfile name different versions, because the version cleared here would then not be the version installed — `make py-lock` regenerates it. Both fail on a bypass entry in `.github/tool-cooldown-bypass.toml` that has expired, reaches beyond three months, or matches nothing, and an invalid entry loses its effect.|`make tool-cooldown-gate BASE=<ref>` / `make tool-cooldown-audit`|
|`pnpm-cooldown/`|Check the `minimumReleaseAgeExclude` entries in every `pnpm-workspace.yaml` against the deadlines in [`.github/pnpm-cooldown-bypass.toml`](../.github/pnpm-cooldown-bypass.toml). Unlike `go-cooldown` and `tool-cooldown` this is not the window's guard: pnpm's resolver enforces the window itself and re-verifies the whole lockfile on every install, `--frozen-lockfile` included. What had no guard is the exemption. The deadline lives in its own TOML rather than in the declaration, for the same reason the siblings do — an `expires` is meaningless to pnpm, so it has no claim on the file pnpm reads, and keeping it out also keeps the check off the comment-parsing path where a form pnpm honours but a reader misses would let an undated exemption through. The check fails on an exclusion with no entry, an entry that has expired, one reaching beyond three months, one matching no exclusion, and an exclusion naming a version the sibling `pnpm-lock.yaml` no longer resolves. The declaration is read as a `yaml.Node` rather than by scanning lines, because YAML spells the same sequence several ways — indented, flush with the key, flow-style, quoted — and every form pnpm honours must be read or an undated exemption passes as "no exemptions". Lockfile keys are normalized before comparison (`'@scope/name@1.0.0'` is quoted, `name@1.0.0(peer@2.0.0)` carries a peer suffix), since a raw match reports every scoped package as unresolved. A deadline arrives without either file changing, so this runs on a schedule as well as on the pull request.|`make pnpm-cooldown-check`|
|`migration-lint/`|Check the sequence numbers under `database/migrations` for duplicates (`-check duplicate`) and gaps (`-check gap`), reading the number before the first `_` of `<seq>_<name>.<kind>.sql` and selecting up / down with `-kind`. Called from the lefthook pre-commit gate. The decision lives in Go rather than in the shell recipe because this check fails towards *inspecting nothing*, which a test can pin and a shell pipeline cannot.|`make check-migration-up-version` / `check-migration-down-version` / `check-migration-up-gap` / `check-migration-down-gap`|
|`cover-gate/`|Compare the total coverage `go tool cover -func` reports against the `-threshold` value and exit non-zero below it. Extracting the `total:` line and judging it are separate pure functions, so both are pinned by tests — the `awk` pipeline this replaced coerced any non-numeric percentage to `0` through `t+0`, which reported a malformed profile as a coverage failure rather than as the tooling failure it is.|`make cover-gate`|

### Local Environment Verification

|Script|Description|Invoked By|
|---|---|---|
|`realtime-smoke/`|Check that DynamoDB Local and GoAWS accept, over the AWS SDK Go v2, the calls Realtime Delivery makes in production — DynamoDB conditional put / `ConsistentRead` query / pagination / TTL, and SNS topic → N SQS queues with `RawMessageDelivery` plus the queue policy — and print one verdict per call: 互換 (accepted and the postcondition holds), 非互換 (accepted but the postcondition fails, or refused), 未対応 (the emulator says it is not implemented), 検証不能 (transport failure, or a halted prerequisite). Only the 非互換 / 未対応 rows are the scope of a local compatibility implementation, and the summary lists just those. Resources carry a per-run random name and are deleted on exit (`-keep` retains them for inspection). A 検証不能 row makes the exit code non-zero — a run that looked at nothing must not read as clean — and `-strict` extends that to 非互換 / 未対応 for CI use. The wire-protocol probe (`ListQueues` over AWS JSON 1.0) runs first because every later call depends on it, and a failed prerequisite leaves its dependants in the table as 検証不能 rather than dropping them.|`make realtime-smoke`|

### AI Feedback Loop (`closed-loop/`)

|Script|Description|Invoked By|
|---|---|---|
|`closed-loop/`|Read the phase marks a window left under `tmp/closed-loop/marks/` and report each window's phase intervals, repeated stamps, and the states that must be looked at before the numbers are used (no opening mark, never closed, marks out of order). Given `--transcripts` / `--codex` it also folds the session records for a period into deterministic counts — prompts, tool calls, failures, interruptions, skill invocations — and lists the skills that went uncalled, with the warning that a count alone is not grounds to retire one. What a window is, and why the order comes from a declaration rather than from the timestamps, is [ADR-0010 (development-window-as-feedback-unit)](../docs/adr/0010-development-window-as-feedback-unit.md).|`make closed-loop-report`|
|`closed-loop/send/`|Send the windows that have closed but never reached their feedback issue. The observation block becomes the issue body, and the reading of it is produced here too — a local model call over the selected turns, so the transcript never leaves the machine. When that call cannot run the issue is labelled `feedback/needs-summary` and the selected turns are posted as a comment, which is the only way a runner can see them; that is the one route on which verbatim turns are published, and [`docs/design/security.md`](../docs/design/security.md) records it. Creating the issue and posting the comment are separate failures — an issue with no comment is retried for its comment alone, because folding them together would let a half-sent window read as done. Runs on the host: the transcripts it reads live under the user's home, which the tool-runner container cannot see.|`make closed-loop-send` / SessionStart hook|
|`closed-loop/weekly/`|Collect the feedback issues created in a period, cluster them by the classification labels they carry, and score each cluster on frequency / impact / human intervention / recurrence so a retrospective has an order to work through, printing each issue's `Suggested Improvement` under its cluster so the agenda does not require opening every issue. Recurrence weighs heaviest: a problem that reappears across windows is a foundation problem rather than one person's slip. Also names the windows whose merge wait exceeded their implementation time, since working faster does not shorten those, and — taking a closed feedback issue as the signal that an improvement landed — reports whether the same classification has come back since. It reports the recurrence and never the verdict: keep, simplify or revert is decided in the retrospective.|`make closed-loop-weekly FROM=<date> TO=<date>` / weekly workflow|

### Initial Setup (`setup/` / `repo-setup/`)

Scripts for configuring the boilerplate when creating a new project from this template.

|Script|Description|
|---|---|
|`replace-module/`|Replace Go module name across all `.go`, `go.mod`, etc. <!-- setup-localize:line -->|
|`replace-app-metadata/`|Replace app name/description in env files and OpenAPI spec <!-- setup-localize:line -->|
|`replace-license-copyright/`|Replace LICENSE copyright holder and year <!-- setup-localize:line -->|
|`replace-repository-reference/`|Replace GitHub repository references in READMEs and OpenAPI <!-- setup-localize:line -->|
|`replace-codeowners/`|Replace the owner of every rule in `.github/CODEOWNERS`. Comment lines keep their example owner, and a rule whose owner field is unrecognizable is reported instead of rewritten. <!-- setup-localize:line -->|
|`remove-sample-api/`|Remove the sample API (`user`/`prefecture`/`product`/`order`): deletes paths declared in `sample-manifest.ts` and strips `sample-api` marker blocks from the shared DI modules and `openapi.yaml`. Run via `make setup-remove-sample-api` to also regenerate/format/lint. <!-- sample-api:line -->|
|`repo-setup/`|The git / gh half of initialising this boilerplate as your own repository: `preflight` refuses to proceed when a `v0.0.0` tag is present, `bootstrap` recreates the tags, prepares `develop` / `staging` / `production` and moves the default branch, and `prune-release-notes` deletes every release note but `v0.0.0.md`. Labels, rulesets and workflow enablement stay in `setup-repository.mk`, which owns the overall chain. Here too the steps are Go because deleting tags in bulk and moving the default branch cannot be rehearsed without breaking a real repository.|
|`remove-doc-language/`|Fold the documentation / skill translation pairs into a single language (`--lang en\|ja\|both`). `en` deletes every `*.ja.md`; `ja` renames each translation onto the canonical name and transplants the canonical's frontmatter, so `SKILL.md` stays loadable; `both` keeps the pairs and only resolves the markers, because a declaration left in a tree that has chosen reads as a choice still open. Prose that explains the pairing convention is not folded mechanically — it is carried by `doc-pair` markers, or declared in `language-manifest.ts`, and an undeclared line stops the run before anything is written. <!-- lang-choice:line -->|

All setup scripts support `--dry-run` for preview.
<!-- sample-api:begin -->

The deletion targets are declared in [`sample-manifest.ts`](setup/remove-sample-api/sample-manifest.ts) and the marker-stripping rules in [`sample-api.ts`](setup/remove-sample-api/sample-api.ts). The sample spans several full-stack domains, so expanding it only requires appending paths to the matching domain block and wrapping interleaved lines with the `sample-api:begin … sample-api:end` markers (or `sample-api:line`).

#### Where a marker may sit

A marker only fires inside a comment, and which comment forms are available is decided by where the
line lives, not by the file's extension. Getting this wrong fails in two directions: the marker is
not recognised and the line survives the removal, or it is recognised but its own presence breaks
the document while the sample is still there. Both are silent until a lint or a post-removal build
catches them, and only `sample-removal-check` sees the second half of that.

Below, `<marker>` stands for the marker name and suffix (`sample-api` + `:begin` / `:end` / `:line` /
`:replace-*`) — spelled out here with its comment prefix, this page would become a marker itself.

|Context|Form that works|Why the others do not|
|---|---|---|
|Markdown prose|`<marker>` in an HTML comment, `:begin` and `:end` on their own lines|Nothing else is a comment here|
|Markdown table row|The `:line` form **inside the last cell**, before the closing `\|`|A comment on its own line ends the table. Match the table's own spacing: a tight table (`\|a\|b\|`) takes the comment with no surrounding space, a loose one (`\| a \| b \|`) with it — otherwise MD060|
|Markdown list item|Marker lines indented to the item's continuation indent|A comment at column 0 splits the list (MD032 / MD007)|
|Fenced code|The fence language's own comment (`//` for Go)|An HTML comment inside a fence renders as text|
|mermaid fence|`%%`, and **block form only**|mermaid takes comments on their own line, so there is no trailing form. Mark the edges and the `class` line that name the node too, not just the node|
|YAML block scalar|Nothing works inside it — wrap the whole key from outside with `replace`|YAML has `#` comments, but inside a block scalar (`>-`, `\|`) a `#` line is part of the string, so the marker becomes the value's own text and the scalar keeps it after the removal|
|Go declaration|`:begin` goes **above the doc comment**|A doc comment left outside survives alone at end of file, and `gofmt` does not remove it|
|Go import|The `:line` form on the import line|An import used only inside a marked region fails `typecheck` once the region is gone|

Two rules for the `replace` escrow (the lines beginning `=`):

- The resulting indentation is the escrow line's own leading whitespace **plus** whatever the content
  carries. Write the indent once, not twice.
- Never put an HTML comment inside an HTML-comment escrow. The first `-->` closes the outer comment
  and the remainder leaks into the rendered page. Keep such a comment outside the `replace` block —
  which is where a guidance comment belongs anyway, since it has to survive the removal.

A whole construct that cannot hold a marker — a `sql` or `text` fence, a table the change restructures
— is wrapped from the outside with `replace`, substituting the entire construct rather than a line
inside it.

Adding or removing a marker line moves the count `scripts/marker-baseline/baseline.json` pins, so
regenerate it (`tsx scripts/marker-baseline --write`) in the same change. A file that carries the
marker form as *data* rather than as an instruction goes in `MARKER_LITERAL_FILES` instead — but only
a file with no real markers of its own, since that declaration removes it from the scan entirely.

`sample-api` needs that reverse declaration in a way `boilerplate-only` does not: tutorials, fixtures
and the convention write-ups carry the `sample-api` marker form in their own prose, so a naive scan
would harvest text that was never an instruction. Declaring the file is therefore the exception that
keeps the scan honest, and forgetting one corrupts that file — `sample-removal-check.yaml` catches it
as a failing `make test` / `md-markdownlint-ci` / `go build`.
<!-- sample-api:end -->

## Test Strategy

These tools are not a layer, so no layer README governs them; per section 11 of
[`docs/testing-conventions.md`](../docs/testing-conventions.md) their viewpoints live here. The
cross-cutting structure rules (`t.Parallel()`, subtest groups, assertions) still come from that
document — only the viewpoints below are local. They hold for the Go tools and the TypeScript ones
alike: the runner differs (`make test-scripts` vs. `make scripts-test`), the viewpoints do not.

- **Test the decision, not the shell around it.** Each tool splits into an entry that only reads
  files, prints, and sets an exit code, and the decision modules beside it. In Go that entry is
  `main` plus a `run` that takes its impure dependencies as arguments — the working directory, an
  HTTP client, the current time, a coverage total, a command runner — so the dispatch itself is
  reachable from a test. In TypeScript it is `index.ts`, which holds no branch at all: what it may
  not contain, and why, is declared in [`lib/untested-modules.ts`](lib/untested-modules.ts). A
  module named there is claimed to hold no decision, so the claim has to stay true — when one of
  them turns out to own a rule (an ordering, a dry-run equivalence, a safety guard), it leaves the
  declaration rather than keeping the exemption.
- **Pin the degenerate input, not just the violation.** Most of these tools are gates, and a gate
  fails towards *inspecting nothing and reporting a clean run*. A malformed glob pattern, an
  unreadable target file, a lockfile line that does not parse, and an empty scan therefore each get
  a case asserting an error — never a silent fall-back to zero findings. This is the viewpoint that
  keeps "found no violations" distinguishable from "looked at nothing".
- **Assert on the sentinel, not on the message.** Every failure mode is a package-level sentinel and
  tests reach it with `require.ErrorIs`. A substring assertion is added on top when the message
  carries something a caller acts on — which file drifted, which key failed — but never as the only
  check, or a reworded message quietly becomes a passing test for the wrong error.
- **Give every window both of its sides.** Where a tool compares against a threshold — a cooldown
  window in days, a coverage percentage — the case pair is the boundary value itself and the value
  one step below it. A single comfortably-inside case cannot tell `>=` from `>`.
- **Stub the external world at its own boundary.** `docker` and `npm` are replaced by a shell script
  placed first on `PATH` so the argument list a tool composes is itself under test; the GitHub API
  and the module registries are replaced by an `httptest` server. `t.Setenv` makes the `PATH` cases
  incompatible with `t.Parallel()`, which is declared per case rather than worked around.
  `actions-shellcheck` is the exception: it drives the real `shellcheck` and skips when absent, and
  `REQUIRE_SHELLCHECK` exists so that skip cannot pass for a run.
- **An irreversible step is verified as a plan, never performed.** `release` and `repo-setup` push
  tags, cut GitHub Releases and move the default branch, so their steps go through a `runner` seam
  and the tests assert the composed command sequence and the abort conditions. Exercising these for
  real would mean actually releasing.
- **Prove that a failed run wrote nothing.** A tool that rewrites files decides everything before it
  writes, so an abort partway through must leave the working tree untouched. The assertion is on the
  file contents after the error, not merely on the error.
- **Treat the reported text as the contract where it is the only output.** A drift list, a
  `::warning::` annotation, a quarantine note — a human or a CI annotation reads exactly that, so
  those cases capture the standard logger and assert on what was emitted.
- **Fixtures live under `t.TempDir()`, never in the real tree.** A test that reads the repository's
  own workflows or lockfiles passes or fails with today's contents and has stopped being about the
  tool.

## Notes

- The Go tools' unit tests run through `make test-scripts` / `make test-scripts-cached` alone —
  `make test` excludes `scripts/`. How they are wired is in
  [`.makefiles/README.md`](../.makefiles/README.md)
- `actions-shellcheck`'s tests shell out to the real `shellcheck` and skip themselves when it is
  absent. `REQUIRE_SHELLCHECK` turns those skips into failures, because a skip is invisible in the
  default output and would leave a run green while checking less than it reports
- The TypeScript scripts run through `tsx` and are type-checked with `tsc`; their dependencies are
  declared here (`package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml`) and installed into
  `scripts/node_modules` by the `node_tool_runner` image build and by the CI jobs that need them
- Every entry point is invoked as `pnpm --dir scripts <script>` rather than through
  `node_modules/.bin`, because `pnpm-workspace.yaml`'s `verifyDepsBeforeRun` only inspects the
  installed tree when a run goes through pnpm. The `tsx` entry returns to the repository root
  first: the scripts resolve their relative paths from there
- Their decision logic lives beside each entry (`scripts/<tool>/*.ts`), in `scripts/lib/**` when it is
  shared, and in `scripts/portal/*.ts`, separated from the CLI entry points on purpose: five of these scripts are gates, and a gate that stops inspecting reports
  a clean run rather than an error. `make scripts-test` / `make scripts-typecheck` are what keep a
  silent no-op from passing for a pass, and CI runs them in `scripts-check.yaml`
- Setup scripts run after the sample API (and parts of this tooling) has been removed, so they must
  not rely on the rest of the tree still being there. `remove-sample-api.ts` deletes its own manifest
  and marker logic, and `verify-sample-removal.ts` deletes itself, its decision module and that
  module's test once the verification passes
- Setup scripts are one-time use — run when creating a new project from the boilerplate
- AI agents should not modify this directory unless explicitly instructed
