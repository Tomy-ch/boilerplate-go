---
name: tools-upgrade
description: >-
  Audit this repository's pinned tool versions against upstream latest, across both declaration sites — `mise.toml` `[tools]` and the `python/*.in` PyPI tools installed from the hash-pinned `python/*.txt` — with a configurable supply-chain quarantine. Releases newer than `min_age_days` are reported as informational only and never applied automatically, and when an advisory drove the run each quarantined release is handed to `/supply-chain-triage` for a scored evidence verdict. The window and the per-tool update set are confirmed with the user before anything is applied. Use this skill on a routine cadence (monthly / quarterly) or after a security advisory. Do NOT use it to patch a specific advisory-flagged dependency (`/dep-vuln-upgrade`), to upgrade the Go language version (`/go-upgrade`), or to pin GitHub Actions or Docker images (`/actions-pin`, `/images-pin`).
---

# Tool Version Upgrade

This skill audits every pinned tool version against upstream latest, with a **supply-chain quarantine gate**: releases newer than `min_age_days` are surfaced as informational only and are never applied automatically. The gate exists because malicious uploads to npm / PyPI / Go module proxies are typically detected and revoked within hours to days; waiting reduces exposure.

The audit surface is **both** declaration sites, because a tool that is not read is a tool that never gets upgraded:

- `mise.toml` `[tools]` — everything mise resolves.
- `python/*.in` — the PyPI tools, whose resolved trees are hash-pinned in `python/*.txt` ([ADR-0084 (mise-ssot-drift-gate)](../../../docs/adr/0084-mise-ssot-drift-gate.md)). Bumping one of these is a two-file change: the pin, then `make py-lock`.

A Japanese reference translation is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

## When to Use

Use this skill when:

- Routine periodic (monthly / quarterly) check of pinned tool versions
- Before a release, to confirm there are no known CVEs patched since the current pins
- After a security advisory, to see whether the relevant tool can be updated

Do NOT use this skill for:

- Upgrading Go itself — use `/go-upgrade` (different downstream sync via `make sync-versions`)
- Updating Go module dependencies (`go.mod` `require` block) — use `make tidy-lib` directly
- One-off ad-hoc version bumps — just edit the declaration (`mise.toml` + `make sync-versions`, or a `python/*.in` pin + `make py-lock`)

## First Step: Confirm `min_age_days`

This skill **MUST call `AskUserQuestion` immediately after invocation** to confirm the quarantine threshold.

Procedure:

1. If a value is present in the skill arguments (e.g., `/tools-upgrade 14`), include it as a candidate in the question (e.g., "Candidate: `14`").
2. Always invoke `AskUserQuestion`:
    - Question: "Specify the minimum age (in days) for a release to be eligible for auto-apply. Recommended: `7`."
    - Default candidate: `7`
3. Validate the answer is a non-negative integer. Use it as `<MIN_AGE_DAYS>` throughout the rest of the procedure.

Do NOT fetch any upstream API or read the declarations until `<MIN_AGE_DAYS>` is confirmed.

## AI Modification Scope

Per the "Exception: Skill Execution" clause in `CLAUDE.md`, the following paths are permitted to be modified while this skill is running:

- `mise.toml` (the `[tools]` table — write only entries the user explicitly approved)
- `python/*.in` (the version pin — only entries the user explicitly approved) and `python/*.txt` — the latter only as the output of `make py-lock`, never hand-edited
- `go.mod`, `docker/**/Dockerfile`, `docker/**/README.md`, `docker/**/README.ja.md` — only as the downstream output of `make sync-versions` (the script handles these atomically)
- `docker/**/Dockerfile` `FROM` `@sha256:...` digests + `docker/images-pin.toml` — only via `make pin-images-apply` / `pin-images-resolve`, when a `go` / `node` / `python` runtime bump changed a base-image tag

The following remain protected even during skill execution:

- `AGENTS.md` / `CLAUDE.md`
- Generated files (`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, generated content under `docs/`)
- Any file unrelated to the version bump

## Execution Steps

### 1. Parse the Declarations

Read `mise.toml` and enumerate every key under `[tools]`, then read every `python/*.in` and enumerate
its `==` pins. For each entry, determine the backend:

| Key format | Declared in | Backend | Latest-version source |
| --- | --- | --- | --- |
| `aqua:owner/repo` | `mise.toml` | aqua (GitHub Releases) | `gh api repos/owner/repo/releases/latest` |
| `go:path/to/module` | `mise.toml` | go install | GitHub Releases (if hosted there) or `go list -m -versions path/to/module` |
| `npm:package` | `mise.toml` | npm | `https://registry.npmjs.org/{package}` |
| `pipx:package` | `mise.toml` | pipx (PyPI) | `https://pypi.org/pypi/{package}/json` |
| `package[extras]==X.Y.Z` | `python/<tool>.in` | PyPI (installed by `uv` from `python/<tool>.txt`) | `https://pypi.org/pypi/{package}/json` (strip the extras) |
| Short name (e.g., `golangci-lint`) | `mise.toml` | mise registry default | Resolve via `mise registry`, then query the resolved backend |
| `go` (runtime) | `mise.toml` | language download manifest | `https://go.dev/dl/?mode=json` |
| `node` (runtime) | `mise.toml` | language download manifest | `https://nodejs.org/dist/index.json` |
| `python` (runtime) | `mise.toml` | language download manifest | `https://www.python.org/api/v2/downloads/release/` |

For each tool, fetch:

- **Latest stable version** (skip pre-release tags: `-rc`, `-beta`, `-alpha`, `-pre`, `-dev`, etc.)
- **Release date** (ISO 8601 timestamp)

Prefer the `gh` CLI (it handles `GITHUB_TOKEN` automatically and raises the rate limit). For non-GitHub endpoints, use `curl -fsSL`.

### 2. Classify

For each tool:

| Class | Condition |
| --- | --- |
| **up-to-date** | `pinned == latest` (after normalizing the optional leading `v` prefix) |
| **eligible** | `pinned != latest` AND `now - release_date >= MIN_AGE_DAYS` |
| **pending** | `pinned != latest` AND `now - release_date < MIN_AGE_DAYS` |
| **resolution_failed** | Backend lookup failed (network error, 404, parsing failure) |

Sanity rule: refuse to "upgrade" to a strictly lower version per semver — if the parsed latest is `<` the pinned version, classify as `resolution_failed` with reason "potential downgrade".

### 3. Display Summary

Print a Japanese-language summary grouped by class. Example:

```text
ツールバージョン監査結果（min_age_days = 7）

✅ 更新候補（公開から 7 日以上経過 / supply-chain quarantine 通過）:
  - golangci-lint: 2.12.2 → 2.13.0 （公開 2026-05-18, 17 日前）
  - sqlc: 1.31.1 → 1.32.0 （公開 2026-04-29, 36 日前）

⚠️ supply-chain quarantine（公開から 7 日未満、通知のみ）:
  - air: 1.65.3 → 1.66.0 （公開 2026-06-02, 2 日前）
      ※ 直接証拠での評価が必要なら /supply-chain-triage を実行できます（既定では未実行）

✓ 既に最新:
  - oapi-codegen 2.7.0
  - lefthook 2.1.8
  ... (省略可)

❌ 取得失敗:
  - sqlfluff（python/sqlfluff.in）: PyPI への接続失敗
```

Show the declaration file for a `python/*.in` entry, as above. Which file holds the pin decides what
applying the bump involves, and the user is about to approve that in step 5.

### 4. Triage Pending Releases the Quarantine Caught

A `pending` classification means the quarantine is holding a release back purely on age. That is a proxy for four questions — did the publisher change, does the artifact match its source, what actually changed, did new dependencies appear — and the proxy can be discharged by answering them directly (`docs/design/security.md` → "Dependencies"). Chain **`/supply-chain-triage`** per pending tool to do that when the answer would change what the user does:

- Always, when the pending release is the reason the skill was invoked (a security advisory naming that tool).
- Otherwise only on request. A routine monthly audit that reports three pending tools does not need three triages — nobody is deciding anything yet, and next month they will simply be eligible. Say the triage is available rather than spending the run on it.

Pass the backend, tool key, candidate version, the **version currently pinned in `mise.toml`** (the diff's other end), `<MIN_AGE_DAYS>`, and the release date. Triage is report-only: it reads the release artifact without executing it, returns a 0–12 score with a band and citations, and never edits `mise.toml` or applies anything. A pending tool stays pending — the score is what the user weighs, and adoption remains a separate, explicit decision (step 5).

### 5. Confirm Per-tool Update Set

If **eligible** is empty and nothing pending was triaged into a decision, skip to step 7 with no writes.

Otherwise invoke `AskUserQuestion` with `multiSelect: true`. Each option corresponds to one eligible tool, with the version diff and release date as the description. Default state: all selected.

The user may deselect individual entries (e.g., if a specific bump is known-broken).

A **pending** tool may appear in this question only when it was triaged in step 4 and the user is explicitly deciding whether to adopt it early — in that case it is listed separately, **deselected by default**, with its band in the description. Never fold a pending tool into the default-selected eligible set: the quarantine's whole value is that age-based eligibility is the default and early adoption is a deliberate act.

### 6. Update the Declarations

For each approved tool declared in `mise.toml`:

- Locate the exact line in `mise.toml`
- Replace the version literal only — preserve the original key (`aqua:owner/repo` / `go:path/to/module` / short name) and the original `v`-prefix convention if any
- Do not reorder keys, do not touch unrelated keys, do not touch the `[settings]` table
- **Never record why a pin sits below latest, and delete such a note when you meet one.** A note naming the upstream latest and its age restates a policy [ADR-0095](../../../docs/adr/0095-malicious-package-detection-via-cooldown.md) already states and `tool-cooldown` already enforces, and both of its facts rot: the age is wrong tomorrow, the named latest at the next upstream release. `make tool-cooldown-outdated` answers the same question every time without going stale, and against this repository's own window rather than upstream latest alone. What survives is a constraint that does not depend on a version — why a backend was chosen over another, a floor and the thing that requires it — and those stay.

After computing all approved changes, write `mise.toml` **once** (atomic single-pass write). Read the file → apply all replacements in memory → write.

For each approved tool declared in `python/*.in`:

- Replace the version after `==` only — preserve the package name and its extras (`graphifyy[sql]`)
- Delete any note above the pin that says why the tool is held below latest, by the rule stated for `mise.toml` above.
- Then regenerate the lockfiles:

  ```sh
  make py-lock
  ```

  The `.in` and its `.txt` are one change. `make tool-cooldown-gate` fails on a pin whose lockfile still names the old version, precisely so that a forgotten regeneration cannot leave the quarantine clearing a version that is never installed. Commit both files together; never hand-edit a `.txt`.

  `py-lock` regenerates **every** `python/*.txt`, so an untouched tool can still show a diff when one of its transitive dependencies published a new release. That diff is real and is part of the change — review it, do not discard it. It is also not covered by this run's quarantine decision, which was made per direct pin: say so in the final report.

### 7. Run `make sync-versions` if Necessary

If any of `go` / `node` / `python` was updated, run `make sync-versions`. This propagates the new runtime version to `go.mod` and the hardcoded `FROM golang:` / `FROM node:` / `FROM python:` references in the Dockerfile and `docker/**/README.md` files.

If only non-runtime tools were updated, skip `make sync-versions`.

### 8. Re-pin Base Image Digests if a Runtime Changed

If step 7 ran `make sync-versions` (i.e. a `go` / `node` / `python` bump changed a `FROM` **tag**), the previously-pinned `@sha256:...` digest now points at the OLD image — a tag/digest mismatch (Docker honors the digest). Re-pin from the registry (this is the `images-pin` skill's job, chained here):

```sh
make pin-images-resolve   # run `docker login` first if Docker Hub returns 429
make pin-images-apply
make pin-images-check
```

`sync-versions` rewrites only the version portion of the tag; the trailing `@sha256:...` is left untouched and now names the *old* image. Docker honors the digest, so the tree sits in a tag/digest mismatch that must not be committed.

Resolving that lands on `images-pin`'s **rule 3**: the new tag has no prior lockfile entry and its image was just published, so there is no aged digest to step back to. `pin-images-resolve` **fails closed** (`❌ 退行先の無い出来立て image は採用できません`) rather than adopting the fresh digest or stripping the pin to tag-only, `apply` never runs, and `pin-images-check` rejects the stale digest as `未登録`.

The consequence worth stating plainly: **a runtime bump and its digest pin are coupled.** Unless the new image has already aged past `PIN_IMAGES_MIN_AGE_DAYS`, the run cannot be finished cleanly, and the choice belongs to the user — bootstrap deliberately with `days=0` (which `/images-pin` step 3 gates behind a `/supply-chain-triage` evidence check), or hold the runtime bump itself until the image ages. Do not force `resolve` through, and do not leave the mismatched digest in the tree.

Skip this step entirely when step 7 was skipped (no runtime change → no tag change → digests still valid).

### 9. Verify

```sh
make go-lint
make go-test
```

If a `python/*.in` pin changed, also run:

```sh
make tool-cooldown-audit
```

It is the check that the pin and its lockfile agree, and it re-measures the window against the version now declared — the same gate the pull request will run.

Report the result table to the user (OK / FAIL per command). Do NOT automatically roll back on failure — the user decides whether to amend, revert, or proceed.

### 10. Final Report

Summarize:

- Number of tools updated, and for PyPI tools whether the lockfile was regenerated
- Any transitive-dependency movement `make py-lock` pulled in, stated as outside this run's per-pin quarantine decision
- Number quarantined (pending, not applied), and when each clears the window
- For any pending tool that was triaged: its band and the axis that drove it (including any axis that came back unanswerable), plus whether the user adopted it early or left it pending
- Verification result
- Any failures to surface

Do NOT commit, stage, or push. The user reviews the resulting working tree and runs `/commit` (or similar) manually.

## Notes

- **Supply-chain quarantine rationale**: typical "dependency confusion" / "malicious release" attacks (e.g., npm `ua-parser-js` 2021, PyPI `ctx` 2022) were detected and yanked within 24–72 hours. A 7-day quarantine catches the vast majority while still being responsive for routine bumps.
- **Pre-release exclusion**: this skill always selects the latest **stable** release. Pre-release tags are visible in upstream but never chosen as `latest`.
- **Calendar versioning**: for tools using calendar versioning (e.g., `2024.12.30`), comparison is lexicographic with semver fallback. The "potential downgrade" guard remains active.
- **Rate limits**: GitHub API anonymous limit is 60 req/h per IP. The skill SHOULD use `gh api` which authenticates via `GITHUB_TOKEN` (1000 req/h authenticated).
- **Idempotency**: multiple invocations are safe. A second run after a successful apply will show those tools as up-to-date.
- The skill never auto-pushes. The user reviews the working tree, runs `make sync-versions` if needed, then commits and pushes manually.

## Checklist

Confirm the following before reporting completion:

- [ ] `<MIN_AGE_DAYS>` confirmed via `AskUserQuestion`
- [ ] Both declaration sites enumerated: `mise.toml` `[tools]` and every `python/*.in`
- [ ] Every entry's backend was resolved (or surfaced as resolution_failed with a reason)
- [ ] Each tool was classified (up-to-date / eligible / pending / resolution_failed)
- [ ] Classification table presented to the user
- [ ] Pending releases triaged via `/supply-chain-triage` when an advisory drove the run (baseline = the version pinned in `mise.toml`); otherwise offered, not spent
- [ ] If eligible set non-empty: user confirmed per-tool update set via `AskUserQuestion`; any early-adopted pending tool listed separately and deselected by default with its band
- [ ] `mise.toml` rewritten atomically with only approved changes, preserving key formats and `v`-prefix convention
- [ ] Approved `python/*.in` pins rewritten (package name and extras preserved), `make py-lock` run, and both files left in the tree; no `.txt` hand-edited
- [ ] No note recording why a pin sits below latest was written, and any met along the way was deleted; version-independent constraints left in place
- [ ] `make tool-cooldown-audit` run if a `python/*.in` pin changed
- [ ] `make sync-versions` run if go / node / python was updated
- [ ] If a runtime was bumped: base image digests re-pinned (`make pin-images-resolve` + `pin-images-apply` + `pin-images-check`). A rule 3 fail-closed on the new tag is the expected outcome for a just-published image — surfaced with the coupling (bootstrap via `days=0` after triage, or hold the bump), never forced through, and never left as a tag/digest mismatch
- [ ] `make go-lint` + `make go-test` run after writes
- [ ] Final result table reported to the user
- [ ] After updating `SKILL.md`, also update `SKILL.ja.md` to keep the Japanese translation in sync
- [ ] No commit / stage / push performed
