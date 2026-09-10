---
name: supply-chain-triage
description: >-
  Determine whether one quarantined supply-chain artifact version has direct evidence of a compromised publish, scoring four evidence axes with citations and changing nothing. Use when `dep-vuln-upgrade`, `tools-upgrade`, `actions-pin`, or `images-pin` holds, defers, quarantines, or labels a candidate `too-new`/`pending`; when `make tool-cooldown-gate` blocks `mise.toml` or `python/*.in`; before a deliberate cooldown override (`days=0`, `--min-release-age=0`, pnpm `minimumReleaseAgeExclude`, PyPI bypass entry, or fresh `overrides` pin); or when a user asks whether a release is safe to take. Do NOT use to perform an upgrade, triage a CVE already adopted in the tree, scan first-party code, or routinely revisit an `images-pin` rule 2 hold.
---

# Supply-chain Triage

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

Answer one question about one artifact version: **is there direct evidence that this release is a
compromised publish?** Produce a scored, cited Japanese verdict and change nothing.

At runtime, read `docs/design/security.md`, especially “Dependencies → Three principles that hold
for every ecosystem” and the matching ecosystem subsection. That document is the policy source of
truth. If it is absent in a repository created from this template, use this fallback: the cooldown is a proxy for whether the
publisher changed, the artifact matches its source, the diff is benign, and no dependency or
capability was added; direct answers beat counting days, but skipping both does not.

`references/` contains English-only command recipes, not human-facing prose. Read exactly the one
matching the ecosystem after fixing the candidate.

## Trigger boundary

Use this skill when a candidate is held, deferred, blocked, quarantined, `too-new`, or `pending` by
`dep-vuln-upgrade`, `tools-upgrade`, `actions-pin`, or `images-pin`; when either cooldown audit or
gate named in the description fires; before any listed deliberate override; or when a user asks in
any phrasing whether a release is safe to take.

Do not use it to apply the upgrade, triage a CVE in an already-adopted dependency, or scan
first-party code. Do not normally use it for `images-pin` rule 2: mutable image tags leave two axes
unanswerable, so triage normally confirms the hold. Use it there only for a rule 3 bootstrap or a
time-pressured `days=0` decision.

## Hard limits

- **Report only.** Never edit `mise.toml`, `.github/actions-pin.toml`, `docker/images-pin.toml`,
  `.github/tool-cooldown-bypass.toml`, `package.json`, `pnpm-workspace.yaml`, `python/*.in`, a
  lockfile, `go.mod`, `.npmrc`, or a `FROM` / `uses:` / `image:` line. Never rerun a caller's
  `resolve` or `apply`, lower a window, or pass `days=0` / `--min-release-age=0`. A low score is
  evidence for a decision-maker; the caller must ask the user explicitly before adopting or waiting.
- **Never execute the artifact.** Download and read only: use an npm registry tarball / `npm pack`,
  never `npm install`; unzip a PyPI wheel, never `pip install` / `uv pip install` or build an sdist;
  never `docker run`, execute a downloaded binary, `go build`, or `go generate` a candidate.
  Where a copied reference offers a less strict command, this limit prevails.
- **Work outside the repository.** Extract only into a scratch directory or `tmp/`, never the worktree.
- **Cite or drop.** Every score must name its command and observation. Mark unsupported evidence `?`.

## Evidence model

Attempt all four axes before scoring. Score each `0`–`3` (higher is worse), or `?` when the
evidence is unobtainable; **never substitute `?` with `0`**.

| Axis | Question | `0` means |
| --- | --- | --- |
| P — Publisher | Did the publisher change? | Same maintainer, committer, or owner and expected cadence |
| A — Attestation | Does the artifact match its source? | Provenance/transparency evidence binds it to a named source commit on the upstream default branch |
| D — Diff | What actually changed? | Baseline diff fully read, proportionate to notes, and free of the signatures below |
| S — Surface | Did dependencies or capabilities appear? | No new dependency, entrypoint, packaged path, or widened permission |

- `0`: answered with continuity or benign change.
- `1`: benign overall, but one plausible detail remains unexplained.
- `2`: answered, but an unexplained irregularity remains.
- `3`: a compromise signature is present or the axis answers adversely (for example, an unknown
  publisher or an artifact that does not match its trusted tag).
- `?`: unavailable; state why in one clause.

The baseline is what the caller would otherwise **keep** — current pinned SHA, aged lockfile digest,
or installed version — never merely the preceding registry release. If it cannot be identified (a
first pin or `images-pin` rule 3), D is `?` unless the ecosystem provides another comparison.

### Compromise signatures for D

Read explicitly for: install/lifecycle hooks; credential or secret access; new outbound network
calls; obfuscation; shelling out; a bundled artifact that outruns its source; widened packaging or
capability surface; and disproportion between the claimed change and the actual diff. A casual
"looks reasonable" skim is not evidence.

## Band and exposure

Sum answered axes: 0–2 **LOW**, 3–5 **MEDIUM**, 6–8 **HIGH**, 9–12 **CRITICAL**. Any `3` floors the
result at HIGH. With two or more `?`, report **INSUFFICIENT-EVIDENCE** and no band; with exactly one
`?`, calculate normally but cap the band at MEDIUM.

Report exposure separately; never fold it into the score:

- Executes in CI with job credentials before repository code: highest.
- Linked into the running service.
- Build/dev-time only. A PyPI tool is above this tier: it executes with developer workstation
  privileges and repository write access.

## Procedure

1. Establish the ecosystem (`npm`/`pnpm`, `pypi`, `go`, `github-actions`, or `docker-image`), exact
   name and candidate, baseline, window N and how it was caught, publish/clear dates, and urgency.
   If supplied, parse `<ecosystem>:<name>@<candidate-version>`, `baseline=<version>`, and
   `days=<N>` in any order; use calling context for omitted fields.
2. Read the one corresponding recipe: `npm.md` (also pnpm), `pypi.md`, `go-modules.md`,
   `github-actions.md`, or `docker-images.md`; then read the matching security-design subsection.
3. Gather P, A, D, and S with commands and observations. Only after attempting all four, assign
   scores, apply both overrides, and classify exposure.
4. Print the following Japanese report and stop. The calling skill handles any explicit user
   confirmation.

```text
供給網トリアージ: <ecosystem>:<name> <baseline> → <candidate>
  捕捉理由: <disposition>（窓 <N> 日 / 公開 <publish-date>, <age> 日 / 解除 <clear-date>）
  緊急度  : <CVE 等 / 定期更新>

スコア: <sum>/12 → <BAND>
  P 発行者     : <0-3|?>  <根拠を1行、コマンド名を添える>
  A 出所の一致 : <0-3|?>  <同>
  D 変更内容   : <0-3|?>  <同>
  S 依存/権限面: <0-3|?>  <同>
  <override が効いた場合はその旨: 「D=3 のため HIGH で下限」「? が2つのため INSUFFICIENT-EVIDENCE」>

暴露面: <CI 資格情報 / 実行時リンク / ビルド時のみ>（スコアとは別軸）

所見:
  - <検出した具体物、または「該当署名なし」>

推奨: <下記の対応表に従う>

本スキルは何も変更していません（報告のみ）。採用の判断は <呼び出し元 / 利用者> が行います。
```

| Band | Recommendation |
| --- | --- |
| LOW | 四つの問いは直接証拠で答えられており、窓の趣旨は満たされている。採用の可否は判断者に委ねる |
| MEDIUM | 既定は窓を待つこと。急ぐ理由があるなら、未解決の点を明示したうえで判断者が決める |
| HIGH / CRITICAL | 採用しない。上流への報告と、当該バージョンを避ける代替（一つ前の aged 版）を検討する |
| INSUFFICIENT-EVIDENCE | 証拠が取れていないため窓がそのまま効く。待つ |

Restate these walls whenever applicable. Never suggest lowering any window.

- pnpm `minimumReleaseAge`: the block also reaches `--frozen-lockfile` replay. A per-version
  `minimumReleaseAgeExclude` exists, but the score merely informs the caller's real choice.
- PyPI `scripts/tool-cooldown`: the repository gate fails the build. Its only escape hatch is a
  dated `.github/tool-cooldown-bypass.toml` entry with `expires`, `issue`, and `reason`; the verdict
  belongs in `reason`, not as permission to create the entry.
- `images-pin` rule 3: no aged digest exists, so even LOW leaves only waiting or a deliberate
  `days=0` bootstrap for the decision-maker.

## Notes

- A clean, cited report is a real result; `?` is not failure.
- A transparency log proves immutability, not benignity.
- Score this move, not the package's reputation.
- Attribution (publisher and commit) is as valuable as the verdict for an upstream report.
- Never stage, commit, push, or apply an upgrade.
