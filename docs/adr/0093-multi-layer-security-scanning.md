---
status: accepted
date: 2026-07-26
deciders: [maintainers]
tags: [ci, security]
---

# ADR-0093: Multi-layer security scanning, splitting reporting from gating, on hardened runners

## Status

accepted

## Context

A Go web service has several distinct attack surfaces that require different scanning
approaches:

- **Known CVEs in dependencies**: Traditional dependency scanners report every CVE in
  every transitively imported module, including ones in code paths that the application
  never executes. This produces a high false-positive rate that erodes trust in the
  scanner.
- **Logic-level security bugs**: SQL injection, path traversal, insecure deserialization,
  and similar patterns cannot be detected by dependency version checks; they require
  semantic analysis of the source code.
- **Leaked secrets**: API keys, passwords, or private keys accidentally committed to the
  repository need a different detector, and a working-tree scan misses a secret that was
  committed and later deleted. Pattern-shaped detection also cannot tell whether a matched
  string is a live credential or a placeholder.
- **Dependency vulnerability breadth**: A second scan covering the full dependency tree,
  across every ecosystem in the repository rather than Go alone, provides a broader safety
  net for prioritisation and advisory tracking.
- **The CI definitions themselves**: Workflows hold the most privileged credentials in the
  repository, and an untrusted expression interpolated into a `run:` block or an
  over-scoped token is a direct compromise path. Nothing in the source-level scanners looks
  at them.
- **The runner's own behaviour**: A compromised action or a transitive tool download
  exfiltrates at execution time, which no static scan observes.

Further surfaces were added as the pipeline matured:

- **The workflows and their runners themselves**: a CI definition is code with credentials,
  and a compromised action or a template-injected `run:` block is an attack on the build
  rather than on the product.
- **The shape of the API contract**: an endpoint that declares no bound on its input, or no
  authentication, is a defect in the spec that no code scanner reads.
- **Inputs nobody wrote a test for**: a parser accepts whatever arrives, and the cases that
  break it are by definition the ones nobody imagined.

No single tool covers these surfaces adequately, so a layered approach using purpose-fit
tools is required.

A second question sits on top of tool choice: **when may a scanner fail the build?** The
naive answer — always — is only tenable while the scanners are quiet. The moment one
reports a vulnerability inherited from the existing dependency tree, every unrelated PR
turns red until the dependency update lands. That finding is not something those PRs
introduced and not something they can fix. Suppressing unfixed advisories globally
(`ignore-unfixed`) trades one failure for another: it also silences them at the point where
the decision actually matters. The two questions are genuinely different — *"does this
change make things worse?"* versus *"is what we are about to promote acceptable?"* — and
need to be answered in different places.

## Decision

Operate purpose-fit scanning layers, and separate reporting from gating.

Layers are split by *surface × gate policy*, not by tool. The same tool appears in more than
one workflow when the questions differ — Trivy scans dependencies, Dockerfiles and licences
in three workflows with three thresholds — and overlapping detections are resolved by giving
each surface a single owner rather than gating twice on one finding.

**1. Reporting and gating are distinct mechanisms.**

An ordinary PR gets *reporting*: findings go to GitHub code scanning as SARIF and to an
upsert PR comment, and the check does not fail on a vulnerability inherited from the
existing dependency tree. *Gating* happens on a PR into `develop` / `staging` /
`production`, where the dependency state under review is the state about to be promoted.

| Gate | Fails on |
| --- | --- |
| `trivy-release-gate.yaml` | any Trivy finding, including one with no released fix |
| `osv-release-gate.yaml` | any OSV finding rated HIGH or CRITICAL, plus an unrated finding that has a fixed version |

Neither release gate carries a `paths` filter: a promotion PR often changes no manifest,
and a check has to run to be able to block. Severity for the OSV gate comes from the
advisory's own rating and falls back to the CVSS score osv-scanner aggregates per group;
advisories from the Go vulnerability database publish neither, so they gate only when a
fixed version exists.

`dependency-review.yaml` is the exception that proves the rule — it evaluates only what a
PR *adds*, so it can block on an ordinary PR without punishing anyone for inherited state.

**2. Each surface gets a purpose-fit tool, triggered where its result can change.**

| Surface | Tool | Reports | Gates |
| --- | --- | --- | --- |
| Reachable Go CVEs | `govulncheck` (reachability-filtered, trace depth > 1) | PR / protected push / weekly | — |
| Go dependency breadth | Trivy FS | PR / protected push / weekly | `trivy-release-gate` |
| Cross-ecosystem dependencies (Go + npm) | OSV-Scanner | PR / protected push / weekly | `osv-release-gate` |
| Newly introduced dependencies | Dependency Review | dependency-change PRs | same workflow |
| Logic-level bugs | CodeQL | PR / protected push / weekly | — |
| Secrets, by pattern | gitleaks | all PRs; weekly over full history | same workflow |
| Secrets, verified live | TruffleHog | all PRs (diff); weekly over full history | same workflow |
| Workflow definitions | zizmor | Actions-file PRs / protected push / weekly | same workflow |
| Container image | Trivy image + SBOM | deploy-branch PRs / weekly | same workflow |
| Repository posture | OpenSSF Scorecard | default branch / weekly | — |
| Dependency versions younger than the cooldown | Go cooldown + tool cooldown ([ADR-0095](0095-malicious-package-detection-via-cooldown.md); pnpm enforces its own at resolution time) | `go.mod` / `mise.toml` / `python/*.in` changes / weekly | same workflow (direct Go requirements and tool declarations) |
| Dockerfile misconfiguration | Trivy config | Dockerfile-change PRs / protected push | same workflow (HIGH+) |
| Dependency licences | Trivy licence | same trigger as Trivy FS / weekly | — (no policy yet) |
| Newly introduced advisories, GHAS-independent | OSV diff | dependency-change PRs | same workflow (no threshold) |
| First-party source patterns | Opengrep (taint-tracking) | Go / dependency / spec PRs / weekly | same workflow (ERROR) |
| Lockfile `resolved` URLs | lockfile-lint | lockfile-change PRs | same workflow |
| API contract shape | Spectral (OWASP API) | spec-change PRs / protected push | same workflow |
| Inputs nobody tested | Go native fuzzing | weekly | same workflow |
| Dependency capabilities | capslock | `go.mod`-change PRs | — (report-only) |

A PR surfaces the risk the change introduces; a push to a protected branch keeps a
code-scanning baseline for branch protection to judge; a weekly schedule exists only where
the result can change while the code stands still — a newly disclosed CVE, a new CodeQL
query, an action that became archived. Weekly runs are staggered in 15-minute steps so a single
cron minute does not queue every scanner at once.

The CLI-based scanners (`govulncheck`, zizmor, OSV-Scanner, TruffleHog, gitleaks, Trivy)
declare their version in `mise.toml`, so the supply-chain surface grows by binaries pinned
in one place rather than by additional third-party actions. Only the tools that genuinely
cannot run as a CLI — Dependency Review (GitHub's dependency-graph API), Scorecard (OIDC
publishing), Harden Runner (runner-level monitoring) — are consumed as actions, and those
go through the SHA pinning and quarantine of [ADR-0094](0094-sha-pinned-actions.md).

**3. Every job is hardened at the runner level.**

Each job in `.github/workflows/**` starts with `step-security/harden-runner` in
`egress-policy: block` with its own `allowed-endpoints`, refusing every outbound call the
job has no business making and recording file-integrity events alongside. One job —
`trufflehog` — stays on `audit`, because it verifies a candidate credential against an
open-ended set of issuers and a blocked endpoint there would silently downgrade a real leak
to an unverified result. The lists are generated from the SSOT in `.github/egress.toml` by
capability class rather than written per job. Jobs that need no git credentials after
checkout set `persist-credentials: false`.

## Consequences

### Positive Consequences

- Govulncheck's reachability filter reduces noise: only vulnerabilities in code the
  application actually calls are reported as actionable.
- CodeQL covers logic-level bugs that version-based scanners miss entirely.
- The weekly schedules catch newly published CVEs and newly archived actions against the
  current codebase even without code changes.
- An inherited vulnerability no longer blocks unrelated work, while still being impossible
  to promote silently — the block moves to the point where someone can act on it.
- The release gates deliberately do *not* use `ignore-unfixed`. A vulnerability with no
  released fix is precisely the kind that warrants a human decision before promotion.
- Secret detection covers both axes on which it can be wrong: pattern-shaped detection over
  the full history (gitleaks) and proof that a credential is live (TruffleHog).
- The workflow definitions are themselves audited, closing the gap where the most
  privileged part of CI was the least inspected.
- Separate tools scanning separate surfaces mean a failure in one layer is independently
  actionable.

### Negative Consequences

- The workflow count is materially more CI surface to maintain than a handful. The shared
  `.github/actions/osv-scan` composite action keeps the OSV parser and severity policy in
  one place, but the breadth itself is a cost, and adding a scanner is never just "add a
  tool" — it requires deciding which surface owns the finding.
- Blocking depends on the gates being registered as required status checks — a
  `required_status_checks` rule listing each gate's check context in
  `.github/settings/branch-protection.json`, which takes effect only once that ruleset is
  applied to the repository with `make apply-branch-protection`. Until that rule is present
  and applied, the gates report without blocking. Because a required check on a path- or
  branch-filtered workflow hangs a PR that never triggers it, each gate carries a
  `*-guard.yaml` companion that reports the same check context as a success on the
  complementary path/branch set, so a PR that skips the body still reports the context — an
  extra workflow per gate whose filter has to be kept in sync with the body it guards.
- Detections overlap across tools, and the overlap has to be resolved by hand each time
  (Opengrep and Trivy both flag a root-user Dockerfile; Opengrep and gosec both flag
  `math/rand`). Left unresolved, the same finding gates twice and gets suppressed twice.
- Govulncheck's reachability analysis requires Go module awareness; if the module graph
  changes structure, filtering assumptions may need revisiting.
- Secret scans run unconditionally on every PR (no path filter), adding fixed overhead.
- `harden-runner` adds a third-party step to every job and reports to an external service.
  Under `block` an endpoint the allowlist omits fails the job, so a new outbound dependency
  surfaces as a red build rather than as a warning.
- TruffleHog's `--results=verified` deliberately drops `unknown` results, so a credential
  whose verification call failed is not reported.
- Scheduled runs may produce findings not associated with any open PR, requiring
  maintainers to triage proactively.

## Alternatives Considered

### Single dependency scanner only (e.g. Dependabot)

Dependabot updates dependencies reactively but does not perform reachability analysis or
SAST. It would miss logic-level bugs entirely.

### govulncheck without reachability filtering

Running govulncheck without the `trace > 1` post-filter produces a large volume of
advisory-level findings for code paths never exercised. PR authors would habitually
dismiss all govulncheck output, destroying the signal.

### Gate on every PR

Simplest, and correct while the scanners are quiet. Rejected because the first real
inherited finding turns every unrelated PR red — observed directly when these scanners were
introduced. The predictable outcome is that the team disables the check or learns to merge
past it, which costs more than the gate was worth.

### Suppress specific advisories with an ignore list

`.osv-scanner.toml` / `.trivyignore` entries would keep ordinary PRs green. Rejected
because an allowlist keyed by vulnerability ID goes stale silently: the entry outlives the
reason, and nothing fails when upstream ships a fix. The report/gate split needs no list —
the moment a fix exists, the gate picks it up on its own.

### Consolidate all scanning into a single Trivy step

Trivy can scan for both CVEs and secrets, but its semantic analysis is not equivalent to
CodeQL, its reachability awareness is weaker than govulncheck, and it cannot audit the
workflow definitions that invoke it. Consolidation would sacrifice depth for simplicity.

### `egress-policy: block` from the start

Stronger than `audit`, and the eventual goal. Rejected for now because a block policy needs
a settled allowlist of endpoints; deriving one from guesswork breaks CI on the first
unlisted host. Audit first, then narrow.

**Since adopted.** The allowlist was derived and every job moved to `block` (`trufflehog`
excepted, for the reason given above). What the rejection anticipated did happen — the first
runs broke on unlisted hosts, over several rounds — and the resolution was not more accurate
per-job lists but a coarser unit: the endpoints follow from a job's capability class, which
is declared in `.github/egress.toml` and generated into every inline block. The rejection is
kept here because its reasoning still holds for anyone considering the same move without an
allowlist to move to.

## Notes

- Workflow inventory and the full trigger matrix:
  [`.github/workflows/README.md`](../../.github/workflows/README.md).
- zizmor's audit exceptions live in `.github/zizmor.yml`; `ignore` there is file-scoped so
  a new workflow hitting the same audit still fails.
- zizmor is the only scanner here that also runs pre-commit. The hook and CI call different
  `make` targets — the hook's drops the online audits — but both read the `high` threshold from
  the one variable in `.makefiles/security/zizmor.mk`, so neither can be loosened alone
  ([ADR-0089](0089-local-hooks-mirror-ci.md)). It earns that
  because it audits files the developer is editing right then, needs no build and no
  service, and finishes in well under a second; the hook drops the online audits so it
  needs no token. The rest of this layer scans dependencies and images, where the finding
  set changes with the outside world rather than with the edit, and a local run would be
  neither faster nor more current than CI.
- Action pinning and the supply-chain quarantine the added actions go through:
  [ADR-0094](0094-sha-pinned-actions.md).
- Release-image integrity, a separate concern from these scans:
  [ADR-0105](0105-release-image-supply-chain.md).
- The `gosec` linter in `.golangci.yaml` provides an additional in-process security
  check during static analysis (see [ADR-0088](0088-layered-golangci-config.md)).
