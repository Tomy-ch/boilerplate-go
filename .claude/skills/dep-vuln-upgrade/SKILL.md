---
name: dep-vuln-upgrade
description: >-
  Patch specific vulnerable dependencies flagged by a security advisory (CVE / GHSA) across this repo's dependency resolvers — the pnpm-resolved packages (`scripts/`, `docs-viewer/`), the Go module graph, and the PyPI tools hash-locked in `python/*.txt` — targeting only the named packages rather than a blanket upgrade, and chaining `/supply-chain-triage` on every entry the supply-chain cooldown catches. Use it whenever the user pastes a vulnerability report — `npm audit`, `pnpm audit`, Trivy, Dependabot, `govulncheck`, or a hand-written list of "package current → fixed (CVE)" lines — and wants those exact packages bumped to a fixed version, including transitive deps that need an `overrides` pin and indirect Go modules. Safe patches are applied automatically; only major-version bumps and deliberate cooldown overrides are asked about. Do NOT use it for routine "bump every tool to latest" audits (`/tools-upgrade`, which owns `mise.toml` and `python/*.in`), for upgrading the Go language version (`/go-upgrade`), or for a general `go.mod` dependency refresh (`make tidy-lib`).
argument-hint: '[advisory list — one "package current → fixed (CVE/GHSA)" per line] [min_age_days]'
---

# Dependency Vulnerability Upgrade

This skill takes a **security advisory list** and patches only the named vulnerable dependencies to a fixed version. This repo resolves dependencies three ways, and an advisory can name a package in any of them:

- **pnpm** — dependencies recorded in `scripts/pnpm-lock.yaml` and `docs-viewer/pnpm-lock.yaml`, each package carrying its own `pnpm-workspace.yaml` that holds both its cooldown policy and its `overrides`.
- **Go** — modules in `go.mod` / `go.sum`, including **indirect** dependencies.
- **PyPI** — the CLI tools declared in `python/*.in` and resolved, with sha256 hashes, into `python/*.txt`. Locating one is in scope; **bumping it is not** — that belongs to `/tools-upgrade`, which owns both declaration sites. See *A PyPI advisory usually is not this skill's* below.

**A pnpm `overrides` entry is not a translated npm one.** The two ecosystems agree on direct
dependencies — bump the declared version, regenerate the lockfile — and diverge on transitive pins:
pnpm keeps `overrides` in `pnpm-workspace.yaml` rather than `package.json`, and its `parent>child`
selector constrains only that direct edge where npm's nested override applies to the whole subtree.
So a scoped npm override cannot be copied across unchanged. This repo's pnpm packages therefore
write the selector as a **resolution range** (`"fast-uri@<3.1.5": ">=3.1.5 <4"` — "if resolution
lands in this range, force it up") instead of naming a parent. Follow the existing entries in the
target `pnpm-workspace.yaml`; when a transitive pnpm pin is not obviously expressible that way, stop
and hand the entry to the maintainer rather than inventing a selector.

It is deliberately **targeted**: it changes only the packages named in the advisory, never a blanket "everything to latest". That keeps a security patch reviewable and decoupled from unrelated churn. For blanket upgrades use `/tools-upgrade` (mise tools) or `make tidy-lib` (Go modules) instead.

A Japanese reference translation is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- The user pastes a vulnerability report (`npm audit`, Trivy, Dependabot alert, `govulncheck`, or a hand-written list) and wants the flagged packages patched.
- A CVE / GHSA advisory names a package that lives in a `pnpm-lock.yaml` or in `go.mod`, and you want the minimal fixed-version bump plus verification.
- Transitive deps (pulled by a tool like `redocly` / `orval` / `spectral`) need forcing to a patched version via `overrides`.

Do NOT use this skill for:

- Routine "bump all pinned tools to latest" — that is `/tools-upgrade` for `mise.toml` `[tools]`.
- Upgrading the Go language version — that is `/go-upgrade` (different downstream sync).
- A general `go.mod` dependency refresh unrelated to a specific advisory — use `make tidy-lib`.
- npm packages that are actually mise-managed tools — those belong to `/tools-upgrade`.
- **Raising a PyPI tool's pin** — `/tools-upgrade` owns `python/*.in`, and raising one there means regenerating `python/*.txt` with `make py-lock`. Locate the package here, then hand it over.

## A PyPI advisory usually is not this skill's

A Python advisory reaches this repo through one of two shapes, and only the first is ever actionable here.

- **The advisory names a tool this repo declares** (`sqlfluff`, `graphifyy`). Locate it, report the fixed version and where it must be declared, and **hand the bump to `/tools-upgrade`** — it owns `python/*.in`, knows that a change there requires `make py-lock`, and is what `tool-cooldown` gates against. Do not edit `python/*.in` or `python/*.txt` here.
- **The advisory names a transitive package** that appears only in `python/<tool>.txt`. There is no per-package pin to raise: the lockfile is a resolution, so the fix arrives by raising the tool whose tree pulls it, or not at all until upstream releases. Report which tool's lockfile carries it, whether a newer tool version resolves past the advisory, and leave the decision with the user. Never hand-edit a `.txt` — it carries sha256 hashes that `--require-hashes` enforces at install, so an edited line does not install, it fails.

Either way the cooldown that governs the move is the one described under *the threshold plays two different roles* below, and the escape hatch is `.github/tool-cooldown-bypass.toml` rather than anything in this skill's write surface.

## First Step: Parse Advisories and Resolve the Caution Threshold

Parse the advisory list, then resolve the supply-chain caution threshold `<MIN_AGE_DAYS>` — **preferring whichever cooldown the repo already declares for that lockfile, so the skill and the toolchain agree**, and asking the user only when nothing authoritative is on disk. The threshold is per-lockfile, not global: an npm and a pnpm package can be governed by different files even when both currently say 7 days.

Procedure:

1. Parse the advisory list from the skill arguments or the most recent user message. Each entry yields: **package name**, **current version** (if given), **candidate fixed version(s)** (there may be several, one per major line), **CVE/GHSA id**, and **severity**. The list may be free-form — tolerate the common shapes (`- [HIGH] lodash 4.17.23 → 4.18.0 (CVE-...)`, `npm audit` blocks, Trivy rows). If an entry's ecosystem or location is ambiguous, resolve it in Step 1 (do not guess here).
2. Detect the repo's cooldown: read the `pnpm-workspace.yaml` beside each `pnpm-lock.yaml` for `minimumReleaseAge` (**stated in minutes** — `10080` is 7 days, so divide by 1440), plus `minimumReleaseAgeStrict` and the existing `minimumReleaseAgeExclude` list. Adopt that value as `<MIN_AGE_DAYS>` for that package. Read the exclusion list even when it looks irrelevant: an entry already covering the candidate version means the window has been opened deliberately and the disposition is not `blocked`.
3. If no cooldown governs a given change (e.g. a Go module, or a lockfile with neither setting), use `7` as the default `<MIN_AGE_DAYS>` for it, unless a value was passed in arguments. Only call `AskUserQuestion` to confirm the threshold when there is genuine ambiguity (the governing files disagree, or the user asked to override); a lone declared value or the `7` default does not need a question — proceed and state the value you used and where it came from.

The threshold plays two different roles depending on ecosystem:

- **pnpm under `minimumReleaseAge`**: a **hard block**, and a wide one — pnpm re-verifies the **whole lockfile** against the policy on every install, `--frozen-lockfile` included, so an in-window entry fails the replay path too (`ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`) and not merely fresh resolution (`ERR_PNPM_NO_MATURE_MATCHING_VERSION`). pnpm offers a first-class per-version escape hatch (`minimumReleaseAgeExclude`); it is a decision to surface at Step 5, never one to take here. The behavioural detail is recorded in [`docs/design/security.md`](../../../docs/design/security.md) → "Dependencies → pnpm" — read it rather than trusting this summary.
- **everywhere else (Go, a lockfile with no cooldown)**: a **caution flag, not a hard block** — because the point is to fix a known vulnerability, a too-new fixed version is surfaced and confirmed, not silently withheld.

Do NOT fetch registries or edit any file until `<MIN_AGE_DAYS>` is resolved.

## AI Modification Scope

`AGENTS.md` normally treats `docker/**` and repository-root files as out of scope for AI edits. **Invoking this skill is the explicit user instruction that relaxes that**, but only for the specific files a dependency patch must touch, and only for this run. This is a documented, non-loophole exception per the "Skills must not be a loophole" clause in `AGENTS.md` — the paths below are the entire surface, and the user is told which ones changed.

Permitted to modify while this skill runs:

- `**/package.json` — only to add/adjust a `dependencies` version for an approved package.
- `**/pnpm-lock.yaml` — the deterministic output of `pnpm install --lockfile-only` in that package directory for approved changes.
- `**/pnpm-workspace.yaml` — **only** the `overrides` and `minimumReleaseAgeExclude` keys, and the exclusion **only after the user has approved that specific version** at Step 5. Never touch `minimumReleaseAge`, `minimumReleaseAgeStrict`, `minimumReleaseAgeIgnoreMissingTime`, `trustPolicy*`, `allowBuilds`, `blockExoticSubdeps`, or `engineStrict` — those are the policy itself, not the patch.
- `go.mod` / `go.sum` — the output of `go get <module>@<version>` + `go mod tidy` for approved Go modules.
- `vendor/**` — **only** as the mechanical output of `go mod vendor`, and **only if the repo vendors** (a `vendor/modules.txt` exists). A `go.mod` bump leaves `vendor/modules.txt` out of sync; the build then fails with `inconsistent vendoring`, so re-vendoring is a required downstream step, not an edit of taste. Never hand-edit vendored files.
- Regenerated artifacts that these dependencies drive, **only** when a drift check (Step 7) shows they moved — e.g. `openapi/openapi.gen.yaml` via `make gen-bundle-oapi`. Regenerate with the repo's `make` target; never hand-edit a generated file.

Hard-protected even during this skill (never touch):

- `AGENTS.md` / `CLAUDE.md`
- `node_modules/**` (build product, not tracked source), and `vendor/**` **except** via `go mod vendor` as above
- Generated files edited by hand (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, generated content under `docs/`) — regeneration via `make` is fine; hand-editing is not.
- Any package NOT named in the advisory list. This skill is targeted; it must not opportunistically bump neighbors.

## Execution Steps

### 1. Locate Each Package

For every advisory entry, find where the package actually lives and classify it. Do not assume — verify against the tree.

```sh
# which lockfile(s) exist at all — this is what decides the ecosystem, not the package name
find . -name pnpm-lock.yaml -not -path '*/node_modules/*'

# pnpm: presence + installed version. The `importers:` block near the top lists the DIRECT deps
# with their specifier; a bare `<pkg>@<ver>:` under `snapshots:` / `packages:` is transitive.
grep -n "^  <pkg>@" <dir>/pnpm-lock.yaml            # resolved version(s)
grep -n "\"<pkg>\"" <dir>/package.json              # declared → direct

# PyPI: the declaration is the .in, the resolved tree (transitive deps included) is the .txt
grep -n '<pkg>' python/*.in                          # declared → this is a tool, /tools-upgrade owns it
grep -niE '^<pkg>==' python/*.txt                    # resolved → may be transitive only

# Go: is the module in go.mod, and is it direct or indirect?
grep -n '<module-path>' go.mod
```

Record per entry:

| Field | How |
| --- | --- |
| ecosystem | `pnpm`, `pypi`, or `go` |
| location | the `pnpm-lock.yaml` dir, `python/<tool>.in` + `.txt`, or `go.mod` |
| installed version | from the lockfile / `go.mod` |
| direct / transitive | declared in that `package.json`'s `dependencies`/`devDependencies` (pnpm) or without `// indirect` (go) → direct; else transitive/indirect |

A package can appear in **more than one** lockfile (this repo resolves `mermaid`, `zod`, and `js-yaml` in several). Record one entry per location — they move independently and each has its own cooldown.

If a package cannot be found in any lockfile or `go.mod`, report it as **not-present** (already removed, or in a lockfile this repo does not have) and skip it — do not invent a location.

### 2. Select the Fixed Version

For each present package, choose the target version:

- **Default: the minimal fixed version that stays on the currently-installed major line.** From a multi-candidate advisory (`brace-expansion 1.1.15 → 5.0.7 / 1.1.16 / 2.1.2`), pick the one matching the installed major (`1.1.15` → `1.1.16`). This minimizes breaking risk.
- **Major bump required** (the only fix is a higher major, or the installed line has no patched release — e.g. `@hono/node-server 1.19.14 → 2.0.5`): flag it explicitly as **potentially breaking**. It will need per-package confirmation (Step 5) and closer verification (Step 7).
- **Downgrade guard**: never select a version strictly lower than installed. If the only "fix" parses lower, mark it `needs-manual` and surface it rather than applying.
- **Gate the version that actually resolves, not just the advisory's number.** A `^`/`~` range in `package.json` floats to the newest matching patch, which may be far newer than the advisory's fixed version and thus `too-new`. Compute the date on the version the lockfile will land on (Step 6 re-checks this), and when a range would resolve to a too-new / unvetted version in a dir with **no cooldown** to hold it back, **pin the exact approved fixed version** instead of leaving the caret to float. Under a cooldown the range case is benign in both ecosystems — it silently lands on the newest *aged* match rather than the newest one — but that also means a range can quietly fail to reach the advisory's fixed version, so re-read what the lockfile actually pinned.

Fetch each chosen version's **publish date** to feed the caution gate (Step 3):

```sh
# npm
curl -fsSL https://registry.npmjs.org/<pkg> | jq -r '.time["<version>"]'
# Go (module publish time)
curl -fsSL "https://proxy.golang.org/<module>/@v/<version>.info" | jq -r '.Time'
```

### 3. Apply the Supply-chain Caution Gate

For each chosen version, compute `now - publish_date` and set a disposition:

| Flag | Condition | Effect |
| --- | --- | --- |
| **clear** | `>= MIN_AGE_DAYS`, or already named in `minimumReleaseAgeExclude` | eligible — apply by default (see Step 5) |
| **too-new** | `< MIN_AGE_DAYS`, in a dir with **no** cooldown, or Go | eligible but flagged; ⚠️ surfaced and NOT applied by default — the user must opt in |
| **blocked** | `< MIN_AGE_DAYS` under a pnpm `minimumReleaseAge` | **cannot be installed as-is** — the repo's own cooldown hard-rejects it. Do not apply on your own; mark **deferred**, report when it will clear (`publish_date + N days`), and for pnpm also present the exclusion option at Step 5 |

The caution exists because malicious uploads to npm / the Go proxy are typically detected and revoked within hours to days. A security fix is urgent, so `too-new` is a warning the user can override — but `blocked` is the repo's own policy enforced by the package manager itself, and the skill never disables that policy. Note the boundary is real: a version published even a few minutes inside an N-day window is `blocked` until the window rolls past its exact publish timestamp.

**The two ecosystems differ in what `blocked` leaves you.** npm has no per-version escape hatch, so a blocked npm entry really is "wait, or take an older aged fixed version". pnpm has one — `minimumReleaseAgeExclude` — which this repo's `pnpm-workspace.yaml` files describe as the path for an urgent security fix, and which `minimumReleaseAgeStrict: true` deliberately keeps out of the resolver's hands. **That difference changes the options you present, not who decides.** Adding an exclusion is a human's call every time; a precedent in the file is not an authorization to add the next one (`AGENTS.md`, *Conflicting Authority*).

### 4. Triage What the Gate Caught

For every entry dispositioned `too-new` or `blocked`, chain **`/supply-chain-triage`** (one invocation per entry) before Step 5 presents the decision.

The window is a proxy for four questions — did the publisher change, does the artifact match its source, what actually changed, did new dependencies appear — and those can be answered directly instead of waited out (`docs/design/security.md` → "Dependencies"). This step is where that happens: a `too-new` opt-in is precisely the decision that should rest on evidence rather than on the user's tolerance for a day count, and a `blocked` entry needs to know whether waiting is merely inconvenient or actually protective.

Pass the ecosystem, package, candidate version, the **baseline the lockfile currently holds** (the diff's other end), `<MIN_AGE_DAYS>`, the disposition, and the CVE forcing the move. Triage is report-only — it reads the tarball / module zip without executing it and returns a 0–12 score, a band, and citations. It changes nothing and never adopts anything; the decision stays in Step 5.

Skip it when there is nothing flagged (all `clear`), and skip it for an entry the user has already declined this run. Carry each entry's band into Step 5's summary and option descriptions so the choice is made with the evidence attached.

### 5. Display Summary; Auto-apply Clear, Confirm Only the Flagged

Print a Japanese summary grouped by disposition. Example:

```text
依存脆弱性パッチ監査（min_age_days = 7, pnpm-workspace.yaml 由来）

✅ 適用（同一 major の最小修正版 / caution 通過 → 確認なしで適用）:
  - lodash 4.17.23 → 4.18.0  [docker/tools, 推移的]  (CVE-2026-4800, CVE-2026-2950 / HIGH)
  - js-yaml 4.2.0 → 4.3.0     [docker/tools, 直接]    (CVE-2026-59869, CVE-2026-53550 / HIGH)

⚠️ major 跨ぎ（breaking の可能性 / 別途確認）:
  - vite 7.4.2 → 8.0.1  [docs-viewer, 直接]  (GHSA-frvp-7c67-39w9 / MEDIUM)

⚠️ too-new（公開が min_age 未満 / 別途 opt-in）:
  - fast-uri 3.1.3 → 3.1.4  [docker/tools]  (公開 3 日 / CVE-2026-16221 / HIGH)
      トリアージ: 1/12 LOW（発行者同一・provenance 一致・差分は URL parser のみ・新規依存なし）

⛔ deferred（repo の cooldown に阻まれ install 不可）:
  - brace-expansion 1.1.16  [docker/tools, spectral-core 内]  (公開が cooldown 内 / 2026-07-22 頃に解除)
      トリアージ: 2/12 LOW（ただし cooldown により install 自体が不可。解除には minimumReleaseAgeExclude の記載が要る）
  - mermaid 11.16.1  [docs-viewer + scripts, 直接]  (公開 3 日 / 2026-08-12 00:09 JST に解除)
      トリアージ: 0/12 LOW（pnpm は minimumReleaseAgeExclude での版指定例外が選択肢）

❓ 未検出 / 要手動:
  - （lockfile に見つからない等）
```

The confirmation policy is deliberately asymmetric — a clear patch is the whole point of running the skill, so do not make the user click through it:

- **clear AND not a major bump → apply without asking.** These are default patches; confirming each one is friction the user has already implicitly authorized by invoking the skill on the advisory.
- **major-bump (a higher major than installed) → always report separately and confirm**, even when the version itself is `clear`. A major bump can break the code that imports it, so the user decides knowingly. Verify it more closely in Step 7.
- **too-new (caution, no repo cooldown) → report and confirm (opt-in)**, default not applied.
- **blocked / deferred → never applied on your own.** Report it and when it will clear, and offer the `minimumReleaseAgeExclude` option so the user can decide between waiting and opening the window for that one version — offer it, never assume it.

Only call `AskUserQuestion` when there is a **major-bump**, a **too-new**, and/or a **blocked pnpm** entry to decide (a single `multiSelect: true` question listing just those, all deselected by default). Give each flagged option its Step 4 triage band in the description (`1/12 LOW` / `7/12 HIGH` / `INSUFFICIENT-EVIDENCE`) — that band is the reason to opt in or wait, so it belongs where the click happens rather than only in the summary above. For a pnpm exclusion option, state in the description what the entry buys and what it costs: the version installs now, and every checkout carries a policy exemption until someone deletes the line. If every eligible entry is `clear`-and-non-major, apply them straight away with no question. If nothing is eligible, skip to Step 8 with no writes.

### 6. Apply the Updates

Group approved packages by location and apply per group. Never edit a `pnpm-lock.yaml` by hand — let `pnpm` regenerate it.

Only `package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` are tracked here (`node_modules/` is git-ignored and rebuilt in the toolbox image), so update the **lockfile only** with `--lockfile-only` — it resolves from the registry and rewrites the lockfile without materializing the tree.

**pnpm — direct dependency**: bump the declared version in `<dir>/package.json`, then regenerate the lockfile only. This repo's pnpm packages pin **exact versions** rather than ranges, so keep that form.

```sh
# edit <dir>/package.json: "<pkg>": "<new-version>"
cd <dir>
pnpm install --lockfile-only
```

`--lockfile-only` resolves and rewrites `pnpm-lock.yaml` without materializing `node_modules/`. Read back the diff and confirm it moved only the intended package: a pnpm lockfile bump is usually a handful of lines, and a wide diff means something else re-resolved.

**pnpm — transitive dependency**: add an `overrides` entry to `<dir>/pnpm-workspace.yaml` (not `package.json`), written as a resolution range per the caution at the top of this file, then `pnpm install --lockfile-only`. The same-major-floor rule and the provisional-debt rule stated above apply unchanged.

**pnpm — a version the cooldown blocks, after the user approved the exclusion at Step 5**: add the entry to `minimumReleaseAgeExclude` in that package's `pnpm-workspace.yaml`, matching the form of the entries already there.

```yaml
minimumReleaseAgeExclude:
  - <pkg>@<version> # <解除日時> 以降に削除する。<対象 advisory> の修正版で、<どこで動くか>。
```

Four things make the entry reviewable, and all four are required:

- **`<pkg>@<version>`, never a bare package name** — a name-only exemption excuses every future publish of that package.
- **A removal date**, computed as `publish_date + MIN_AGE_DAYS` and written in JST. It is load-bearing in both directions: deleting the line before that moment breaks every install, and leaving it after that moment is a policy exemption nobody needs.
- **The advisory** the exemption is buying, so a reader can judge it without re-deriving the case.
- **Where the package runs** (browser bundle / tool-runner build step / service runtime), because that is the exposure the exemption accepts.

Add the same entry to **every** package whose lockfile takes the version — the exclusion is per `pnpm-workspace.yaml`, so covering one package leaves the other's install failing. Never edit `minimumReleaseAge` or `minimumReleaseAgeStrict` to make an install pass; that is the policy, and lowering it silently opens the window for every dependency at once.

**Go module** (direct or indirect):

```sh
go get <module>@<version>
go mod tidy
go mod vendor        # ONLY if the repo vendors (vendor/modules.txt exists)
```

Batch all approved Go modules into one `go get` (multiple `module@version` args) + a single `go mod tidy`, so `go.sum` settles once. If `vendor/modules.txt` exists, run `go mod vendor` afterwards — without it the build fails with `inconsistent vendoring` because `go.mod` and `vendor/modules.txt` disagree.

### 7. Verify

Run the checks that match what actually changed — a dependency patch rarely touches first-party source, so scope the verification to the ecosystems that moved rather than always running the full suite. Report each as OK / FAIL; do NOT auto-roll-back on failure — the user decides.

Go changes at minimum build and vuln-scan clean (`go build ./...` + `govulncheck ./...`); run `make go-lint` / `make go-test` when a Go change is broad enough to warrant the full suite. A **major bump of a pnpm package** must be verified more closely — run that package's own `typecheck` + tests (e.g. `pnpm typecheck` + `pnpm test` in `docs-viewer/`), since a major can change the API the code calls.

The advisory database is the source of truth for the **real fix floor**, and it can differ from the user's list. Two cases to surface rather than silently resolve:

- **A higher fix floor than the advisory named.** The package may carry a *second* advisory whose first-fixed version is higher than the one the user pasted (e.g. the list says "fixed in 2.0.5", but a separate moderate advisory still affects `2.0.0 - 2.0.9` and is fixed only in `2.0.10`). Do not silently jump to that higher version if it is `too-new` — surface the conflict and whether the vulnerable path is even reachable (e.g. a WebSocket-only DoS on a server that registers no WS handler is likely non-applicable), and let the user opt into the too-new full fix or accept the partial one.
- **A residual advisory on a deferred/skipped package** — expected; report it as still-open with the reason (deferred by cooldown / skipped by the user), not as a failure.

In each changed package dir:

```sh
cd <dir> && pnpm install --frozen-lockfile   # proves the lockfile still satisfies the policy
cd <dir> && pnpm audit                       # the patched CVEs should no longer appear
```

The frozen install is the real gate here. pnpm re-verifies the whole lockfile against the active policies on replay, so this is what proves the change is installable by CI and every other checkout — a `--lockfile-only` run alone does not. If it fails with `ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`, an in-window entry is uncovered: either the exclusion is missing from that package, or it was added to only one of the two.

`pnpm audit` reports limited detail; when you need the real fix floor and the per-advisory version ranges, query the advisory database directly (`gh api "/advisories?ecosystem=npm&affects=<pkg>"`) rather than inferring from the summary.

Go changes:

```sh
govulncheck ./...                # if available; the GHSA should clear
```

Generated-artifact drift — these deps drive code generators, so a bump can move generated output. If a changed lockfile belongs to a package that feeds a generator, run its generator and check for drift (the same check CI runs):

- `docker/tools/**` deps (the toolbox image: redocly / orval / sqlc-adjacent tooling) → regenerate the artifacts that image produces (`make gen-*-oapi`, `make gen-query`, etc.) only if the tool whose dep changed is one of them, and check for drift.

If regeneration produces changes, include them — a security bump that silently changes generated output must not leave the tree in a state where CI's drift check fails.

**Tool-runner image drift (pnpm changes only).** The runner images bake `scripts/node_modules`, so they are build artifacts of `scripts/package.json` + `scripts/pnpm-lock.yaml` + `scripts/pnpm-workspace.yaml`. After changing any of those, the containerized gates (`make md-lint`, `make actions-lint`, `make oapi-lint`, …) fail with `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` until the image is rebuilt:

```sh
make tool-runners-build
```

Host-side runs (`make md-lint-ci` and friends, which CI uses) are green before this and are **not** evidence the containerized gates are. Rebuild, then re-run whichever gate you intend to report as passing. `repo-ops` §10 covers the same failure from the other direction.

### 8. Final Report

Summarize in Japanese:

- Packages patched, grouped by ecosystem / location, with the version diff and CVE ids.
- Any `overrides` added, flagged as **provisional pins to reclaim** once the parent natively ships the fix (specifier used — floor vs the exceptional exact pin, and why).
- Any `too-new` or `major-bump` entries applied (and the confirmation), or deliberately skipped.
- Any `minimumReleaseAgeExclude` entry added: which packages got it, the version it exempts, and **the date it must be deleted** — stated as a follow-up the user owns, not as a closed item.
- For each entry the cooldown caught: its triage band and the axis that drove it, so the record shows the adopt-or-wait call rested on evidence. Name any axis that came back unanswerable.
- Any `not-present` / `needs-manual` entries the user must handle another way.
- Verification results (`make go-lint` / `make go-test` / `pnpm audit` / frozen install / `govulncheck` / drift checks).
- Regenerated artifacts, if any, and whether the tool-runner images were rebuilt.

Do NOT commit, stage, or push. The user reviews the working tree and runs `/commit` manually. If they ask you to commit, note that a security patch commonly spans `docker/**` + `go.mod` + regenerated artifacts, so group them into a clear `Build:` / `Fix:` commit describing the CVEs.

## Notes

- **Targeted, not blanket.** The defining property of this skill is that it touches only advisory-named packages. If mid-run you notice an unrelated outdated dep, mention it but do not bump it here.
- **Don't over-ask.** A `clear`, non-major patch is exactly what the user invoked the skill for — apply it without a confirmation click. Reserve `AskUserQuestion` for the genuinely consequential calls: **major bumps** (breaking risk), **too-new** opt-ins, and a **pnpm cooldown exclusion**. Report those separately and clearly.
- **Respect the repo's cooldown; never disable it.** pnpm's `minimumReleaseAge` is a deliberate supply-chain control. When one blocks a fix, the version is `blocked`/deferred — report when it clears (`publish_date + N days`), and do not lower the window, pass `--min-release-age=0`, add `--before`, flip `minimumReleaseAgeStrict` to `false`, or otherwise route around it. A LOW triage band does not change this: triage supplies evidence, not permission. Often the advisory's only in-cooldown fix is a fresh patch published the same day; wait it out, pick an older already-aged fixed version, or (pnpm only) add a version-scoped exclusion — each *only if the user approves it*.
- **A per-version exclusion is not a lowered window, and that distinction is the whole point.** `minimumReleaseAgeExclude` exempts one `pkg@version`; lowering `minimumReleaseAge` exempts every dependency at once, silently and indefinitely. Never offer the second as a way to achieve the first. Equally, never widen an exclusion to a bare package name.
- **Vendoring.** If `vendor/` is present, a `go.mod` change is not done until `go mod vendor` re-syncs `vendor/modules.txt`; otherwise the build breaks with `inconsistent vendoring`.
- **Transitive vs direct.** Bumping a direct dep is preferred when a compatible direct-dep version already carries the fix; scoped `overrides` is the tool for purely-transitive deps whose parent has not yet released. A scoped override (`"parent": {"pkg": ">=<fixed> <<next-major>"}`) fixes the vulnerable nested copy without downgrading an already-patched top-level copy. State which mechanism each package used in the report.
- **Overrides are provisional debt — write a floor, then reclaim them.** An `overrides` entry is a manual, sticky pin that the resolver neither expires nor reminds you about, so it rots into a silent cap on a transitive dep. Two rules keep it healthy: (1) write it as a **same-major floor** (`">=<fixed> <<next-major>"`), never an exact version, so it enforces the fix as a *minimum* without freezing the dep — reserve an exact pin for when a newer in-range version is known-broken and must be held; (2) treat every override as **temporary** — once the **parent** ships a release that natively pulls a fixed version, reclaim it: bump the parent, delete the now-redundant override, `pnpm install --lockfile-only`, and re-run `pnpm audit` to confirm the fix still holds without the pin. A stale exact override can even re-introduce a vulnerability once the pinned version is itself flagged.
- **Multi-CVE packages.** A package may appear in several advisory lines (e.g. `lodash` under two CVEs) — dedup to one bump that satisfies all, and cite every CVE it resolves.
- **Lockfile-only.** `node_modules/` is not tracked; `pnpm install --lockfile-only` updates the manifest + lockfile without a full install. A bump can still cause the resolver to re-arrange sibling copies in the lockfile — review the diff, but extra `4.17.x → 4.18.x`-style dedup churn on the same package is expected and benign.
- **The same package can live in several lockfiles.** Patch every location the advisory reaches, and keep them on one version unless there is a reason not to; the two pnpm packages deliberately share settings, so a fix applied to one and not the other is drift, not caution.
- **Idempotency.** Re-running after a successful apply shows the packages already at the fixed version (audit / `govulncheck` clean) and makes no writes.
- The skill never auto-pushes. The user reviews, then commits and pushes manually.

## Checklist

Confirm before reporting completion:

- [ ] Advisory list parsed into (package, current, fixed candidates, CVE/GHSA, ecosystem)
- [ ] `<MIN_AGE_DAYS>` resolved **per lockfile** from whichever file declares it (`pnpm-workspace.yaml minimumReleaseAge`, minutes ÷ 1440; `7` default elsewhere); existing `minimumReleaseAgeExclude` entries read; asked only on genuine ambiguity
- [ ] Each package located in **every** lockfile that holds it (`pnpm-lock.yaml` / `go.mod`) and classified direct vs transitive/indirect; not-present entries surfaced
- [ ] Fixed version chosen as minimal same-major; major bumps flagged as potentially breaking; downgrade guard applied
- [ ] Publish date fetched; disposition set (clear / too-new / blocked-by-cooldown)
- [ ] Every `too-new` / `blocked` entry triaged via `/supply-chain-triage` (baseline = the lockfile's current version); band carried into the summary and the `AskUserQuestion` option descriptions
- [ ] Japanese summary shown; **clear non-major applied without asking**; `AskUserQuestion` used only for major-bump / too-new / pnpm exclusion; blocked entries never applied unilaterally
- [ ] pnpm direct via an exact version in `package.json` + `pnpm install --lockfile-only`; transitive via a resolution-range `overrides` in `pnpm-workspace.yaml`; lockfile never hand-edited
- [ ] Any approved `minimumReleaseAgeExclude` entry written as `pkg@version` with removal date + advisory + where it runs, added to **every** affected package, and the window settings themselves left untouched
- [ ] Any override recorded as provisional in the report (reclaim — bump parent, drop override, re-audit — once the parent natively ships the fix)
- [ ] Go via a single batched `go get module@ver ...` + `go mod tidy`; `go mod vendor` if the repo vendors
- [ ] `pnpm install --frozen-lockfile` + `pnpm audit` for pnpm changes; `govulncheck` + build for Go changes; `make go-lint` / `make go-test` as scope warrants (major bumps verified more closely — typecheck / package tests)
- [ ] Generator drift checked for generator-feeding deps; regenerated artifacts included; tool-runner images rebuilt (`make tool-runners-build`) after a `scripts/` pnpm change before claiming a containerized gate passes
- [ ] Final Japanese report: applied set, major/too-new/exclusion decisions with removal dates, deferred/skipped items, verification results
- [ ] After updating `SKILL.md`, re-sync `SKILL.ja.md`
- [ ] No commit / stage / push performed
