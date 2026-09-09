---
name: actions-pin
description: >-
  Audit and update SHA-pinned GitHub Actions in `.github/workflows/**` and `.github/actions/**` using this repository's quarantine-aware lockfile workflow, stepping back to the previous aged version rather than adopting a freshly published one. Use for routine Actions pin refreshes, a GitHub Actions security advisory, or a requested major upgrade. Default to same-major updates; accept `major` for major upgrades and `days=N` (default 14) for the minimum release age. Sibling of `tools-upgrade` (`mise.toml`) and `images-pin` (Docker image digests).
---

# GitHub Actions Pin Update

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

Use the trailing tag comment in each external `uses:` reference as the intended version and `.github/actions-pin.toml` as the resolved SHA lockfile.

```yaml
uses: owner/repo@<40-hex-sha> # <tag>
```

`make pin-actions-resolve` resolves eligible tags into the lockfile, and `make pin-actions-apply` updates only the SHAs. Never update an Action release newer than the exclusion window unless the user explicitly passes `days=0`.

Quarantine age is the newer of the release `published_at` and the resolved commit date, so an old release object does not make a freshly re-pointed moving tag appear aged — a moving tag whose release date is old but whose head commit is recent is still quarantined. The quarantine buys time against automated takeover; reviewing lockfile diffs remains the way to detect a re-point itself. Reasoning: `docs/design/security.md`, Build inputs.

## Inputs

Parse invocation tokens in any order.

- No argument: update within existing major versions; minimum age is 14 days.
- `major` or `--major`: consider newer major versions too.
- `<integer>`, `days=N`, or `--days N`: set the minimum release age. Reject negative values. Warn explicitly for `0` because it disables quarantine.

Do not use this skill for `mise.toml`, Go, or Go module dependency upgrades. Do not change local composite actions (`uses: ./...`).

## Workflow

1. Read `AGENTS.md`, `.github/actions-pin.toml`, and all external `uses:` lines in `.github/workflows/` and `.github/actions/`. Identify each Action, all locations, current tag comment, and current major.
2. Ensure the repository can run the pin tooling. It uses `go run ./scripts/pin-actions` and requires a vendor tree consistent with `go.mod`. If that is the only blocker, run `go mod vendor`. Obtain an authenticated GitHub token if available (`gh auth token`) so release-date queries do not hit anonymous rate limits.
3. For each distinct Action, query non-prerelease releases and their publication dates. Also verify that a moving major tag exists before selecting it. Continue to rank candidates by `published_at`, but do not infer that an old release is aged: resolution gates it on the newer release or commit date.
4. Select a target for major `M`, where `M` is the current major unless `major` was requested:
   1. Prefer moving tag `vM` when its resolved head is older than the cutoff.
   2. Otherwise choose the newest exact `vM.x.y` release older than the cutoff.
   3. If no aged release exists in that major, hold the current pin and record why.
   Use the upstream tag spelling exactly; some projects change between `0.x.y` and `v0.x.y`.
5. For every proposed major upgrade, inspect the upstream release notes or `action.yml` and compare all repository `with:` inputs. Hold the Action if any input is incompatible; do not silently alter workflow behavior to accommodate it.
6. Present the concrete plan in Japanese before writing: changes (including moving versus exact pin and release age), held items and reasons, and unchanged items. Ask the user to confirm the proposed set. Do not write until confirmation.
7. For confirmed items only, edit the trailing `# <tag>` comment. Match the full `uses:` line when multiple Actions share a tag. Do not edit held or unchanged entries.
8. Resolve and apply:

   ```sh
   export GITHUB_TOKEN="$(gh auth token)"
   make pin-actions-resolve PIN_ACTIONS_MIN_AGE_DAYS=<days>
   make pin-actions-apply
   ```

   `apply` settles the verdict for every target before writing; if it aborts, the working tree is untouched. If the moving tag is still fresh under the newer-of-release-and-commit-date rule, retaining an existing within-major pin is expected. An old release whose moving-tag head is recent can therefore make `resolve` print `⚠️ ... 既存ピンを維持`: this is the rule 4.1 → 4.2 fallback firing late, not an error. If resolution reports a missing moving tag, use an eligible exact release instead.
9. Verify:

   ```sh
   make pin-actions-check
   make actions-lint
   ```

   `check` and `apply` also fail closed on repository-state problems; fix them locally before retrying:

   | Error | Meaning and fix |
   | --- | --- |
   | `lockfile に解釈できない行があります` (with a line number) | A lockfile line is not blank, a comment, or a `"key" = "<40-hex>"` assignment. Run `make pin-actions-resolve`, or delete the reported line. |
   | `lockfile にキーの重複があります` | One `owner/repo@tag` is assigned twice, often after a merge-conflict resolution. Run `make pin-actions-resolve`, or delete the duplicate. |
   | `lockfile に参照されていないエントリがあります` | A lockfile key matches no live `uses:`, usually after a workflow deletion. Run `make pin-actions-resolve`, or delete the orphan. |
   | `固定対象として解釈できない記法の uses: があります` | A `uses:` the pinner cannot rewrite: a flow mapping (`- {name: Checkout, uses: actions/checkout@v4}`), a quoted key (`"uses": ...`), a block scalar deferring the value to the next line (`uses: >-`), or a YAML alias (`uses: *anchor`). The message names the offending value. Rewrite the step in plain block notation with `- uses: owner/repo@sha # tag`; do not suppress the check. Text inside a block scalar is exempt, so a `run:` script printing the string `uses: owner/repo@ref` does not trip it. |

   Report each result. Do not automatically roll back on failure.

## Safety and Completion

- Modify only `.github/workflows/*.{yml,yaml}`, `.github/actions/**/action.{yml,yaml}`, and `.github/actions-pin.toml` while this skill runs.
- Do not change `with:` inputs, workflow step logic, `scripts/pin-actions`, generated files, or `AGENTS.md`.
- Do not stage, commit, or push. Report exact-version step-back pins so they can be revisited after the moving tag ages.
- Treat `make pin-actions-check` and `make actions-lint` as required completion checks.
