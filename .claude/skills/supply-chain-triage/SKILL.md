---
name: supply-chain-triage
description: >-
  Gather direct supply-chain evidence about ONE artifact version that a cooldown / quarantine window has caught, and score how likely it is to be a compromised publish (0-12 over four evidence axes) so a human can decide adopt-now vs wait from evidence rather than from a day count alone. Report-only: it never edits a lockfile, pin or bypass file, never lowers a window, and never applies an upgrade. Use this whenever a version is held / deferred / blocked / quarantined or classified `too-new` / `pending` by `/dep-vuln-upgrade`, `/tools-upgrade`, `/actions-pin`, or `/images-pin`; whenever `make tool-cooldown-gate` blocks a `mise.toml` / `python/*.in` declaration; whenever the user asks to "analyze the implementation" of a release, "is this version safe / malicious", or "why is this quarantined and can we take it anyway"; and before any deliberate window override (`days=0`, `--min-release-age=0`, a pnpm `minimumReleaseAgeExclude` entry, a `.github/tool-cooldown-bypass.toml` entry, an `overrides` pin onto a fresh version). Do NOT use it to perform the upgrade itself (that is the four skills above), to triage a CVE in an already-adopted dependency (`/dep-vuln-upgrade`), or as a malware scanner for first-party code.
argument-hint: '[<ecosystem>:<name>@<candidate-version>] [baseline=<version>] [days=<N>]'
---

# Supply-chain Triage

This skill answers one question about one artifact version: **is there direct evidence that this
release is a compromised publish?** It produces a scored, cited verdict and changes nothing.

It exists because of a property recorded in `docs/design/security.md`:

> The window is a proxy, and a proxy can be discharged by direct evidence. Waiting N days is a
> cheap stand-in for four questions: did the publisher change, does the artifact match its source,
> what actually changed, and did new dependencies appear. […] Answering them beats counting days.
> Skipping *both* does not.

`docs/design/security.md` (section "Dependencies → Three principles that hold for every ecosystem")
is the **source of truth** for that reasoning — read it at runtime rather than trusting this
paragraph, because the policy lives there and this skill is only its instrument. The four axes
below are those four questions, made operational. In a repository created from this template where that document is absent, the
quoted principle above is the fallback.

The last sentence of the quote is the design constraint that shapes everything here: an axis you
could not answer is **not** a passing grade. A verdict is a claim about evidence obtained, so this
skill reports "unanswerable" explicitly and refuses to launder silence into a low score.

A Japanese reference translation is available at `SKILL.ja.md` in the same directory (not loaded as
a skill; for human reference only). The per-ecosystem procedures under `references/` are English
only — they are command recipes, not prose for human readers.

## When to Use

- A cooldown window caught a candidate and someone must decide whether to wait: `/dep-vuln-upgrade`
  marked a fix `too-new` or `blocked`, `/tools-upgrade` classified a release `pending`,
  `/actions-pin` had to step back or hold, `/images-pin` hit rule 2 / rule 3.
  That audit is detection-only by design — it says an override happened, not whether it was safe.
  This skill supplies the missing half.
- `make tool-cooldown-gate` blocks a `mise.toml` or `python/*.in` declaration. That gate *does* fail
  the build, so the question is not whether an override slipped through but whether waiting is
  protective — which is what this skill answers, and what the bypass entry's `reason` needs.
- Before any deliberate window override (`days=0`, `--min-release-age=0`, a pnpm
  `minimumReleaseAgeExclude` entry, a `.github/tool-cooldown-bypass.toml` entry, pinning `overrides`
  onto a fresh version). Overriding without evidence is the one combination the design doc rules out.
- The user asks, in any phrasing, whether a specific release is safe to take.

Do NOT use this skill for:

- Performing the upgrade — that is `/dep-vuln-upgrade` / `/tools-upgrade` / `/actions-pin` /
  `/images-pin`. This skill only reports.
- Triaging a CVE in a dependency already in the tree — that is `/dep-vuln-upgrade`.
- Scanning first-party code for defects — that is `/impl-review` and the `gosec` / CodeQL gates.
- **A routine `images-pin` rule 2 hold.** For a mutable image tag two of the four axes are usually
  unanswerable (there is no source diff for a rebuild), so triage will normally confirm the hold
  rather than discharge it. Run it there only when the decision actually needs it — a rule 3
  bootstrap, or a `days=0` override under time pressure. `references/docker-images.md` explains
  which axes survive.

## Hard limits

These are what make the skill safe to invoke on a possibly-malicious artifact.

- **Report-only.** Never edit `mise.toml`, `.github/actions-pin.toml`, `docker/images-pin.toml`,
  `.github/tool-cooldown-bypass.toml`, `package.json`, `pnpm-workspace.yaml`, `python/*.in`, a
  lockfile, `go.mod`, `.npmrc`, or a `FROM` / `uses:` / `image:` line. Never
  re-run a caller's `resolve` / `apply`. Never lower a window or pass `days=0` /
  `--min-release-age=0` yourself. A low score is evidence handed to a decision-maker, not a
  decision — the caller's `AskUserQuestion`, or the user, adopts or waits.
- **Never execute the artifact.** Download and read; do not install, build, or run it. Running an
  install script *is* the attack for npm, so use `npm pack` / the registry tarball, never
  `npm install`; if a resolve is unavoidable add `--ignore-scripts --dry-run`. The equivalent for
  PyPI is an sdist's build backend, which runs on install — download and unzip the wheel, never
  `pip install` / `uv pip install` the candidate. Never `docker run` a
  quarantined image, never execute a downloaded binary, never `go build` / `go generate` a candidate
  module you are about to accuse.
- **Work outside the repo tree.** Extract artifacts into the session scratchpad (or `tmp/`), never
  into the working tree, so nothing you fetched can be committed by accident.
- **Cite or drop.** Every axis score carries the command and the observation it rests on. An
  unsupported hunch is reported as unanswerable, not as a score.

## The four evidence axes

Score each axis `0`–`3`, or mark it `?` when the evidence is not obtainable for this ecosystem or
artifact. Higher is worse. The per-ecosystem recipes in `references/` say how to obtain each one.

| Axis | Question | What a `0` looks like |
| --- | --- | --- |
| **P** — Publisher | Did the publisher change? | Same maintainer / committer / owner as the baseline version, with a publishing cadence in line with history |
| **A** — Attestation | Does the artifact match its source? | Provenance or transparency-log evidence ties the artifact to a named source commit, and that commit is on the upstream default branch |
| **D** — Diff | What actually changed? | The diff against the baseline is read in full, is proportionate to the release notes, and contains none of the signatures below |
| **S** — Surface | Did new dependencies or capabilities appear? | No new dependency, no new `bin` / entrypoint / packaged path, no widened permissions |

Score each axis on what the evidence says, not on how it feels:

- `0` — answered, and the answer is continuity or benign change.
- `1` — answered and benign overall, but one detail is unexplained (a first-time-but-plausible
  co-maintainer, a vendored file bump with no note).
- `2` — answered, and something is off in a way the release notes do not account for.
- `3` — a compromise signature below is present, or the axis is answered adversely (the publisher
  changed to an unknown account; the artifact does not match the tag; a tag we already trust now
  resolves to a different commit).
- `?` — not obtainable. Say why in one clause. Never substitute `0`.

**The baseline is the version the caller would otherwise keep** — the currently-pinned SHA, the
aged lockfile digest, the installed version — not simply the previous release in the registry. The
question being answered is "what am I taking on by moving", so the diff has to span exactly the
move under consideration.

### Compromise signatures (the teeth of axis D)

These are what a malicious publish has actually looked like. Read for them specifically; a diff
skimmed for "does it look reasonable" catches nothing.

- **Install / lifecycle hooks** added or altered — npm `preinstall` / `install` / `postinstall`, an
  `action.yml` gaining a `run:` step, a `go:generate` directive. Code that executes before anyone <!-- skill-lint-ignore -->
  reviews it.
- **Credential and secret access** — `process.env`, `~/.npmrc`, `~/.aws`, `~/.docker/config.json`,
  `GITHUB_TOKEN`, `ACTIONS_RUNTIME_TOKEN`, SSH keys, keychain, the runner's memory.
- **New outbound network calls**, particularly to a raw IP, a URL shortener, a paste service, a
  webhook sink, or any domain unrelated to the project's own infrastructure.
- **Obfuscation** — packed or minified source in a package that otherwise ships readable code, long
  base64 / hex literals, `eval` / `new Function` / `Function(atob(...))`, string-array decoders,
  code hidden behind a build tag or an unusual platform check.
- **Shelling out** — `child_process`, `os/exec`, `curl | sh`, `wget`, in something with no reason to
  spawn a process.
- **A bundled artifact that outruns its source** — for a JS action or a package shipping `dist/`,
  a bundle diff with no corresponding `src/` change is the highest-signal finding available, because
  the bundle is what runs and the source is what gets reviewed.
- **Widened packaging or capability surface** — new `files` / `bin` / `directories` entries, a new
  entrypoint, added `permissions`, a new binary or blob.
- **Disproportion** — a "typo fix" release carrying a 400-line diff, or a package dormant for two
  years publishing twice in a day.

## Bands and the two overrides

Sum the answered axes:

| Sum | Band |
| --- | --- |
| 0–2 | **LOW** |
| 3–5 | **MEDIUM** |
| 6–8 | **HIGH** |
| 9–12 | **CRITICAL** |

Two rules override the sum, because averaging is the wrong operation for this data:

1. **Any axis at `3` floors the band at HIGH.** One confirmed red flag is not diluted by three clean
   axes — a compromised publish is usually clean everywhere except the payload.
2. **Unanswered axes cap the claim.** Two or more `?` → report **INSUFFICIENT-EVIDENCE** with no
   band: the proxy was not discharged, so the window simply stands. Exactly one `?` → compute the
   band but cap it at MEDIUM, because LOW asserts positive evidence on all four questions and you
   have three.

### Exposure is reported next to the score, never folded into it

The score measures *how likely this publish is malicious*. How much damage it would do is a
different quantity, and the design doc is explicit that window length tracks upstream detection
latency rather than blast radius — so mixing them would corrupt both numbers. Report exposure as a
separate line, in the terms that actually differ:

- **Executes in CI with the job's credentials before our code does** — a GitHub Action, a base
  image, an npm package with an install script. Highest; a compromise here reaches secrets directly.
- **Linked into the running service** — a Go module, a runtime npm dependency.
- **Build / dev-time only** — a generator or linter in the toolbox image. A **PyPI tool** sits above this line rather than on it: it runs with the developer's privileges on a workstation holding repo write access, so report it as such instead of as build-time only.

## Procedure

### 1. Establish the candidate

Fix these before gathering anything, from the arguments, the calling skill's context, or by asking:

| Field | Note |
| --- | --- |
| ecosystem | `npm` / `pnpm` / `pypi` / `go` / `github-actions` / `docker-image` — selects the reference file |
| name + candidate version | The exact thing under consideration (tag, version, digest) |
| baseline version | What the caller keeps if this is declined; the diff's other end |
| window `N` and how it was caught | Which disposition fired (`blocked` / `pending` / rule 2 …), and when the candidate ages out |
| urgency | Is a CVE forcing the move, or is this a routine bump? This does not change the score — it changes what the recommendation is worth to the reader |

If the baseline cannot be determined (a first-ever pin, `images-pin` rule 3), say so — axis D loses
its other end and is `?` unless the ecosystem offers another comparison.

### 2. Read the matching reference

Read only the one that applies. Each gives the commands per axis and states which axes that
ecosystem cannot answer:

- `references/npm.md` — npm **and pnpm** packages (same registry, so the same evidence commands; only the reporting differs), including transitive deps under `overrides`
- `references/pypi.md` — PyPI tools declared in `python/*.in` and locked in `python/*.txt` (no publisher identity in the API, so axis P is answered indirectly or not at all)
- `references/go-modules.md` — Go modules
- `references/github-actions.md` — `uses:` references (richest evidence: a real commit range)
- `references/docker-images.md` — Dockerfile `FROM` / compose `image:` digests (thinnest evidence)

Also read `docs/design/security.md`'s ecosystem subsection for that ecosystem — it records what the
toolchain already guarantees, which decides whether an axis is free (Go's transparency log) or
absent (Go freshness, image history).

### 3. Gather, score, band

Work axis by axis, recording the command and what it showed. Then score, apply both override rules,
and derive the band. Resist scoring before all four axes are attempted — a `3` on D found early
tempts you to skip P, and the publisher answer is what tells the maintainer whether to report the
account upstream.

### 4. Report in Japanese and stop

Print the report below, then hand back to the calling skill (which runs its own confirmation) or
end. Make the no-op explicit — the reader must not have to wonder whether a file moved.

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

Map the band to a recommendation, and keep the ecosystem's hard walls visible — a LOW score does
not make a blocked install possible:

| Band | Recommendation |
| --- | --- |
| LOW | 四つの問いは直接証拠で答えられており、窓の趣旨は満たされている。採用の可否は判断者に委ねる |
| MEDIUM | 既定は窓を待つこと。急ぐ理由があるなら、未解決の点を明示したうえで判断者が決める |
| HIGH / CRITICAL | 採用しない。上流への報告と、当該バージョンを避ける代替（一つ前の aged 版）を検討する |
| INSUFFICIENT-EVIDENCE | 証拠が取れていないため窓がそのまま効く。待つ |

Four walls to restate in the report whenever they apply:

- **pnpm under a `pnpm-workspace.yaml` `minimumReleaseAge`**: likewise uninstallable, and the block
  extends to `--frozen-lockfile` replay, so it reaches CI and every other checkout rather than only
  the machine doing the resolve. pnpm *does* have a per-version exemption
  (`minimumReleaseAgeExclude`), which makes the adopt-or-wait choice a real one — report the score
  as the input to it and leave the decision with the caller. Never suggest lowering
  `minimumReleaseAge` or turning off `minimumReleaseAgeStrict`.
- **PyPI under `scripts/tool-cooldown`**: the window is enforced by a repository gate rather than by
  the resolver, and that gate **fails the build** — so a blocked version is blocked by a check this
  repository owns, and a LOW score does not clear it. The escape hatch is a dated entry in
  `.github/tool-cooldown-bypass.toml` (`expires` / `issue` / `reason`, all required, three months
  maximum); a triage verdict is the evidence that belongs in `reason`, not permission to add the
  entry. Never propose editing the window constant in `scripts/tool-cooldown`.
- **`images-pin` rule 3**: there is no aged digest to fall back to, so a LOW score still leaves the
  choice between waiting and a deliberate `days=0` bootstrap.

## Notes

- **A clean report is a real result.** Most quarantined versions are ordinary releases that happened
  to be recent. Saying so with citations is the output that lets a team act on a CVE inside the
  window instead of guessing.
- **`?` is not failure.** The ecosystems differ in what they can prove, by construction — that
  asymmetry is documented in `docs/design/security.md`, not a gap in this skill. Reporting it
  honestly is what keeps the score meaningful.
- **The transparency log proves immutability, not benignity.** Go's `sum.golang.org` and an npm
  publish attestation tell you the artifact was not swapped after publication. A maliciously
  published version is faithfully, verifiably malicious. Axis A answers a narrower question than it
  first appears to.
- **Score the move, not the package's reputation.** A widely-used package with a compromised release
  is the normal case for this skill; download counts and stars are not evidence about this version.
- **Attribution matters as much as the verdict.** When a score lands HIGH or CRITICAL, the publisher
  and commit identified on axes P and A are what makes an upstream report actionable. Include them.
- The skill never commits, stages, or pushes, and never auto-pushes.

## Checklist

- [ ] `docs/design/security.md` "Three principles" read at runtime (or the fallback noted)
- [ ] Candidate fixed: ecosystem, name, candidate version, **baseline the caller would keep**, window `N`, disposition, urgency
- [ ] The one matching `references/<ecosystem>.md` read; its unanswerable axes honored
- [ ] All four axes attempted; each score carries a command + observation, or an explicit `?` with a reason
- [ ] Diff read against the baseline specifically, searched for the compromise signatures by name
- [ ] Overrides applied: any `3` floors at HIGH; ≥2 `?` → INSUFFICIENT-EVIDENCE; exactly one `?` caps at MEDIUM
- [ ] Exposure reported as a separate line, not folded into the score
- [ ] Artifact never executed (no `npm install`, no `docker run`, no downloaded binary, no build of the candidate); extracted outside the repo tree
- [ ] Japanese report printed with band, recommendation, and the explicit statement that nothing was changed
- [ ] pnpm `minimumReleaseAge` / PyPI `tool-cooldown` / `images-pin` rule 3 walls restated when they apply
- [ ] No file modified, no window lowered, no upgrade applied
- [ ] After updating `SKILL.md`, re-sync `SKILL.ja.md`
