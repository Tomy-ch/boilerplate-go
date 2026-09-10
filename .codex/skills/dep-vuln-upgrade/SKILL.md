---
name: dep-vuln-upgrade
description: >-
  Patch only advisory-named vulnerable dependencies across this repository's pnpm, Go, and PyPI resolution surfaces. Use for CVE, GHSA, pasted npm audit or pnpm audit reports, Dependabot, Trivy, or govulncheck findings that need minimal fixed-version updates, transitive overrides, per-lockfile release-age handling, and supply-chain triage for in-window candidates. Do not use for blanket tool updates, Go-version upgrades, general module refreshes, or PyPI pin changes; `tools-upgrade` owns Python declarations and lock regeneration.
---

# Targeted Dependency Vulnerability Upgrade

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

Accept an advisory list from the user message. It may contain one line per package
(`package current → fixed (CVE/GHSA)`) or pasted audit output. Parse package, installed version if
given, fixed candidates, advisory IDs, and severity; deduplicate a package with several
advisories. Change only advisory-named packages and mechanically required output.

The repository has three independent resolution surfaces:

- **pnpm:** `scripts/` and `docs-viewer/`; each has its own
  `pnpm-lock.yaml` and `pnpm-workspace.yaml`. The workspace file owns its cooldown,
  `overrides`, and release-age exclusions.
- **Go:** `go.mod` and `go.sum`, including indirect modules.
- **PyPI:** CLI tool declarations in `python/*.in`, resolved with sha256 hashes in `python/*.txt`.
  Locate these entries here, but never bump them here: `/tools-upgrade` owns the declaration and
  `make py-lock` regeneration.

Use `/tools-upgrade` for routine tool-version audits and PyPI declaration changes, `/go-upgrade`
for the Go language version, and `make tidy-lib` for a general Go dependency refresh.

## Allowed Write Surface

While this skill runs, modify only advisory-related `**/package.json`, package-manager generated
`**/pnpm-lock.yaml`, the `overrides` and explicitly approved `minimumReleaseAgeExclude` keys in
`**/pnpm-workspace.yaml`, `go.mod`, `go.sum`, and `vendor/**` only as the output of `go mod vendor`
when `vendor/modules.txt` exists. Regenerate a generated artifact only through its existing `make`
target when a dependency-driven drift check proves it changed.

Never hand-edit generated output, lockfiles, vendored files, `node_modules/**`, `python/*.in`, or
`python/*.txt`. Never change unrelated packages. Never alter `minimumReleaseAge`,
`minimumReleaseAgeStrict`, `minimumReleaseAgeIgnoreMissingTime`, `trustPolicy*`, `allowBuilds`,
`blockExoticSubdeps`, or `engineStrict`.

## 1. Locate and Classify

Find every location that resolves the named package; do not infer ecosystem from its name. Search
all pnpm lockfiles outside `node_modules`. In pnpm, `importers:` identifies direct dependencies and
`packages:` / `snapshots:` records resolved transitive packages. In Go, `go.mod` and its
`// indirect` marker establish directness. For Python, a match in `python/*.in` is a declared tool;
a match only in `python/*.txt` may be transitive.

```sh
find . -name pnpm-lock.yaml -not -path '*/node_modules/*'
grep -n "^  <pkg>@" <dir>/pnpm-lock.yaml
grep -n '"<pkg>"' <dir>/package.json
grep -n '<pkg>' python/*.in
grep -niE '^<pkg>==' python/*.txt
grep -n '<module-path>' go.mod
```

Record an entry per location: ecosystem, directory/file, installed version, direct/transitive or
indirect status, advisory IDs, and fixed candidates. A package may be present in several lockfiles.
Report an entry absent from all three surfaces as `not-present`; do not invent a location.

For a PyPI advisory:

- If `python/*.in` declares the tool, report its fixed version and declaration path, then hand the
  bump to `/tools-upgrade`. The governing policy is `tool-cooldown`; its escape hatch is
  `.github/tool-cooldown-bypass.toml`, neither writable by this skill.
- If only `python/<tool>.txt` resolves the package, report the carrying tool lockfile and whether a
  newer tool version resolves beyond the advisory. There is no per-package pin: wait for upstream
  or raise the parent tool through `/tools-upgrade` after the user decides.
- Never hand-edit `.txt`: `--require-hashes` makes an edited hash lockfile fail installation.

## 2. Select and Gate the Fixed Version

Choose the smallest fixed version on the installed major line. Never downgrade. Mark an only-higher-
major fix as `major-bump` and `needs-manual` when no non-downgrade candidate is available. Gate the
version that the lockfile actually resolves; pnpm direct dependencies in this repository use exact
versions, never ranges.

Resolve `<MIN_AGE_DAYS>` per pnpm lockfile from the adjacent `pnpm-workspace.yaml`: read
`minimumReleaseAge` in minutes (`10080 / 1440 = 7` days), `minimumReleaseAgeStrict`, and every
`minimumReleaseAgeExclude` entry. An existing exclusion for the exact candidate makes it `clear`.
For Go and any ungated location, use seven days unless the user supplied a value; ask only when an
authoritative on-disk policy leaves a genuine ambiguity.

pnpm re-verifies the whole lockfile on every install, including `--frozen-lockfile`. An in-window
version therefore fails fresh resolution with `ERR_PNPM_NO_MATURE_MATCHING_VERSION` and frozen
replay with `ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`. See `docs/design/security.md` → Dependencies.

Fetch each candidate's publish date and classify it:

| Disposition | Condition | Action |
| --- | --- | --- |
| `clear` | Aged enough, or already in the exact pnpm exclusion list | Eligible. |
| `too-new` | Inside the caution window without a pnpm hard policy | Eligible only by explicit opt-in. |
| `blocked` | Inside pnpm `minimumReleaseAge` | Defer; report `publish date + threshold`. |
| `major-bump` | Fixed version changes installed major | Requires explicit approval even when clear. |

Never lower or bypass a cooldown. A blocked pnpm package may be offered a version-specific
exclusion, but an existing precedent is not approval to add another one.

## 3. Triage Caught Candidates and Decide

For every `too-new` or `blocked` entry, invoke `/supply-chain-triage` once before presenting a
decision. Provide ecosystem, package, candidate, lockfile baseline version, per-lockfile threshold,
disposition, and advisory. Triage is read-only and supplies its 0–12 band and evidence; it never
authorizes adoption.

Present a Japanese summary grouped by disposition. When a `major-bump`, `too-new`, or blocked pnpm
entry exists, make one consolidated operator-decision prompt listing only those entries; require an
explicit selection for each and assume none is selected by default. Include its triage band (`1/12
LOW`, `7/12 HIGH`, or `INSUFFICIENT-EVIDENCE`). For a pnpm exclusion option, say that it installs the
version now and that every checkout carries the policy exemption until its line is removed.

- Apply `clear` non-major entries immediately without a question.
- Apply a major or too-new entry only when selected.
- Never apply a blocked item unilaterally. Report its clearing time and offer the
  exclusion-or-wait decision.
- If no eligible package exists, make no writes and proceed to the report.

## 4. Apply Approved Changes

Batch approved entries by package directory. Read the resulting diff; do not accept unrelated
re-resolution churn.

**pnpm direct:** set the exact version in `<dir>/package.json`, then run:

```sh
cd <dir>
pnpm install --lockfile-only
```

Re-read the resolved lockfile version and re-gate it. The command rewrites `pnpm-lock.yaml` without
materializing `node_modules`; investigate a wide diff.

**pnpm transitive:** add the override only to `<dir>/pnpm-workspace.yaml`, never `package.json`.
Use a same-major range floor (`">=<fixed> <<next-major>"`), not an exact pin, unless a documented
newer in-range version is known broken. Preserve sibling overrides, then run the lockfile-only
command. A pnpm selector is not a nested-override translation: follow the local resolution-range
form, and when the needed selector is not clearly expressible, report it for human decision. Treat
every override as provisional debt.

**Approved blocked pnpm version:** add `<pkg>@<version>` to `minimumReleaseAgeExclude` in every
affected package's `pnpm-workspace.yaml`, retaining all existing entries. Never use a bare package
name. Include its JST removal time (`publish date + age threshold`), advisory, and runtime exposure
(browser bundle, tool-runner build step, or service runtime). Do not change the release-age policy.

**Go:** batch approved modules in one `go get module@version ...`, then `go mod tidy`. Run
`go mod vendor` only when `vendor/modules.txt` exists.

## 5. Verify

Run checks matching changed ecosystems and report each result; do not auto-revert on failure.

- pnpm: in every changed package directory run `pnpm install --frozen-lockfile`, then `pnpm audit`.
  The frozen install proves the lockfile still satisfies policy. An age-violation means an affected
  workspace lacks its necessary exclusion.
- Go: run `go build ./...` and `govulncheck ./...` when available; add `make go-lint` and `make go-test`
  when the Go change is broad enough.
- For generator-feeding dependencies, run the existing generator target and check generated drift.
  For a changed `scripts/` pnpm manifest, lockfile, or workspace file, run `make tool-runners-build`
  before claiming a containerized gate passes.

`pnpm audit` may expose a higher fix floor than the advisory named. Surface it rather than silently
selecting a too-new version. A residual advisory on a deferred or skipped package is expected; report
it as still open with its reason.

## 6. Reclaim an Override

Once the parent natively ships the fix, bump that parent, delete the redundant override, run
`pnpm install --lockfile-only`, and re-run `pnpm audit`. Report the removed provisional debt.

## Final Report

Report in Japanese: patched packages by ecosystem and location; versions and advisory IDs;
provisional overrides and their reclaim condition; each major, too-new, deferred, or exclusion
decision; required JST removal times for exclusions; every triage band and unanswerable axis; PyPI
and not-present/manual follow-ups; generated drift; tool-runner image rebuild status; and each
verification result.

Do not stage, commit, push, broaden the patch beyond advisory-named dependencies, or start a
follow-up synchronization.

## Completion Checklist

- [ ] Located every advisory in all pnpm, Go, and PyPI surfaces; classified directness and reported absences.
- [ ] Read the pnpm cooldown policy and exclusions per lockfile; selected non-downgrade minimal same-major fixes.
- [ ] Triaged every `too-new` and `blocked` entry before a decision.
- [ ] Applied only clear non-major entries automatically; explicitly decided every major, too-new, and pnpm exclusion.
- [ ] Regenerated pnpm and Go output through their tools; never edited hash lockfiles or policy controls.
- [ ] Ran ecosystem-appropriate audit, build, frozen-install, and drift checks.
- [ ] Reported outcomes and user-owned removal/follow-up work in Japanese without staging, committing, or pushing.
