# Security Posture

This document records **how security controls in this repository are reasoned about** — what
they assume, what each one is actually for, and where each one fires. It does not enumerate
the tools; that inventory lives in [`.github/workflows/README.md`](../../.github/workflows/README.md)
and drifts with the code. What is recorded here is the part that should *not* drift.

It covers three surfaces, in the order a risk reaches them: what CI **executes** (build
inputs), what the application **links** (dependencies), and what the running service **does
with a request** (application runtime). A fourth — the developer's own machine — sits outside
all three on purpose, and *The developer endpoint* below records where that boundary falls
rather than leaving it to be rediscovered. The mechanics of identity verification are a separate
concern with their own reference — see [auth.md](auth.md); what appears here is only where
authentication sits in the enforcement model.

## Threat model

Every control below is shaped by one assumption, and it is worth stating plainly because the
shape stops making sense without it:

> **The people with commit access to this repository are an internal team whose members are known
> to each other.**

That assumption buys something specific: **attribution is a usable deterrent**. A control does
not have to be technically unbypassable to work, as long as a bypass is visible and traceable
to a person. The organisation supplies the consequence; the repository supplies the evidence.

It also means a whole class of attack is deliberately **out of scope**. Someone with commit
access who intends harm can delete a workflow, edit a lockfile, and change the very check that
would have caught them — all in the same pull request. No amount of CI defends against that,
and pretending otherwise produces theatre: checks that look protective, cost maintenance, and
stop nothing. Where a control cannot prevent, this repository says so and settles for
detecting.

A repository that operates under different assumptions — public contributions, untrusted
committers, regulatory obligation — should revisit this document first, then the controls. Several of them
would need to become fail-closed.

## Three kinds of control

Conflating these is the most common way a security setup becomes both annoying and useless.
Each mechanism here is exactly one of the three, on purpose.

| Kind | What it does | Failure mode when misapplied |
| --- | --- | --- |
| **Enforcement** | Refuses the action. Fail-closed. | Blocks legitimate emergency work; gets disabled |
| **Detection** | Lets the action through, records it, makes it attributable | Mistaken for protection; nobody reads the record |
| **Deterrence** | Organisational consequence, informed by detection | Assumed to exist without evidence to act on |

Where each mechanism sits:

| Mechanism | Kind | Note |
| --- | --- | --- |
| `pin-actions-check` / `pin-images-check` | Enforcement | Fail-closed: an unpinned or unregistered reference is an error |
| `egress-check` | Enforcement | Fail-closed: an inline `allowed-endpoints` that has drifted from `.github/egress.toml` is an error |
| Release gates (`trivy-release-gate` / `osv-release-gate`) | Enforcement | Only on PRs into a deploy branch |
| `dependency-review` | Enforcement | Evaluates only what the PR *adds* |
| Secret scans (gitleaks / TruffleHog) | Enforcement | A committed secret is never an acceptable trade |
| `zizmor` | Enforcement | High severity only; exceptions are file-scoped in `.github/zizmor.yml`. Also gates pre-commit, offline audits only |
| Reporting scanners (`trivy-fs` / `osv-scanner` / `govulncheck` / CodeQL) | Detection | Findings reach code scanning and the PR, but do not block |
| `harden-runner` (`egress-policy: block`) | Enforcement | Refuses any egress outside the job's `allowed-endpoints`; `trufflehog` alone stays on `audit` |
| `CODEOWNERS` | Enforcement | The review requirement behind "this decision belongs to a role" |
| OpenSSF Scorecard | Detection | Posture measurement, no verdict |

## Where a control fires

A control that runs everywhere is a control nobody reads. Each trigger answers a different
question, and a mechanism belongs where its question is actually being asked:

| Trigger | Question it answers |
| --- | --- |
| Pull request | *Does this change make things worse?* |
| Push to a protected branch | Keeps a code-scanning baseline for branch protection to judge |
| PR into `develop` / `staging` / `production` | *Is what we are about to promote acceptable?* |
| Weekly schedule | *Did the world change while the code stood still?* — new CVEs, new queries, newly archived actions |

The consequence worth internalising: **a scheduled run only exists for a tool whose result can
change without the code changing.** Everything else is noise on a timer.

### Why reporting and gating are split

An ordinary PR reports; a promotion PR gates. The reasoning is in
[ADR-0093 (multi-layer-security-scanning)](../adr/0093-multi-layer-security-scanning.md), and reduces to this: a vulnerability
inherited from the existing dependency tree is not something the current PR introduced, and
not something its author can fix. Blocking there turns every unrelated change red until an
upstream fix lands elsewhere. The predictable outcome is that the check gets disabled or
routinely merged past — which costs more than the gate was worth.

Suppressing the finding instead (`ignore-unfixed`, an ID allowlist) trades one failure for
another: it also silences the finding at promotion time, which is the one moment it mattered.
The split keeps the signal in both places and puts the *verdict* where someone can act on it.

## Build inputs

What CI *executes* is a different risk from what the application *links*. Actions and base
images run with the job's credentials before any of our code does, so they are pinned harder.

**A version reference is not an identity.** A tag can be re-pointed and a mutable image tag can
be rebuilt, so the version stays in the source as human-readable intent while an immutable
digest lives in a lockfile that is the single source of truth —
`.github/actions-pin.toml` ([ADR-0094 (sha-pinned-actions)](../adr/0094-sha-pinned-actions.md)) and
`docker/images-pin.toml`. Both checks are fail-closed, and unpinned versus unregistered are
distinct errors so neither degrades into the other.

**The quarantine buys time; it does not verify a date.** An action's age is taken as the *newer*
of its release publication date and its resolved commit date. Neither alone is trustworthy: a
release object is bound to the tag *name*, so its publication date survives the tag being
re-pointed — the exact threat the quarantine exists for — while a commit date can be set to any
past instant by the committer. Taking the newer of the two means the quarantine holds as long as
either says the target is new, but it is a delay against automated takeover, not a defence
against a forged date. Detecting the re-point itself is the lockfile's job: the resolved digest
changes, the diff is small, and a human reads it.

**A `docker://` step reference belongs to the image lockfile, not the action one.** A workflow may
run a container directly (`uses: docker://<image>[:<tag>|@<digest>]`), which is a registry
reference rather than a GitHub repository. `pin-actions` resolves a ref to a commit SHA through
`git ls-remote`, an operation that has no meaning for a registry, so it treats `docker://` as out
of scope the same way it treats a local `./` reference — forcing it through would either fabricate
a repository name or duplicate the digest resolution `docker/images-pin.toml` already owns. So
`pin-images` scans `.github/workflows/**` and `.github/actions/**` for those steps: a registry
reference is pinned by the mechanism that resolves digests, wherever it is written. Both tools
read some of the same files and never the same lines.

**Out of scope is not the same as unwatched.** Excluding `docker://` from `pin-actions` answers
only who owns it; on its own it drops the reference out of every net, and a mutable tag nothing
reports is worse than one nothing pins — the pinning exists precisely to stop a tag from being
re-pointed under a name that already passed review. A local `./` reference may be dropped
silently because it resolves inside the repository and no third party can re-point it; a registry
reference may not. Hence the scan above, and hence a `docker://` reference must carry a tag:
omitting one means `:latest`, so a reference the strict pattern cannot read fails the check
instead of passing unpinned.

## Dependencies

### Three principles that hold for every ecosystem

**A cooldown window is proportional to upstream's detection latency, not to blast radius.** A
compromised publish is usually caught and pulled within hours to days; the window exists to sit
out that interval. Where upstream detects and corrects quickly the window is shorter (npm: 7
days), and where remediation is slower it is longer (Actions and images: 14 days,
`PIN_ACTIONS_MIN_AGE_DAYS` / `PIN_IMAGES_MIN_AGE_DAYS`). The number is a policy input derived
from the ecosystem, not a constant to copy across them.

**The window is a proxy, and a proxy can be discharged by direct evidence.** Waiting N days is a
cheap stand-in for four questions: did the publisher change, does the artifact match its source,
what actually changed, and did new dependencies appear. When a fix is urgent those can be
answered directly — transparency-log lookup, artifact-versus-tag comparison, reading the diff,
checking the manifest for new requirements. Answering them beats counting days. Skipping *both*
does not.

**A prerelease is outside what the window measures.** The window buys time for upstream to notice
a bad publish and pull it. An alpha or beta is not a publish anyone intends to stand behind: it is
offered on the understanding that it may change shape or vanish, so surviving N days says nothing
about it. That makes "it cleared the cooldown" the wrong reason to take one. Prereleases therefore
stay out of the resolved tree unless a specific need names one, even where a dependency's own range
permits them — a range like `^1.7.1 || ^2.0.0-alpha.3` says the author will accept either, not that
the alpha is preferred, and a resolver picking the highest match will take the alpha every time.
Pinning back to the stable line is an `overrides` entry in the ecosystem's own config, carrying the
reason.

### Go modules

The Go ecosystem gives integrity almost for free, and offers nothing at all for freshness. Both
halves matter.

**Integrity is strong and on by default.** Every module version's hash is recorded in a public
transparency log (`sum.golang.org`) and checked against `go.sum` on download; a published
version is immutable in the proxy and cannot be withdrawn or silently swapped. Nothing in this
repository weakens that — there is no `GOFLAGS`, `GONOSUMDB`, `GOPRIVATE`, or `GOINSECURE`
override anywhere. `vendor/` is untracked, so build inputs are re-fetched and re-verified rather
than trusted from the tree.

**Freshness has no enforcement whatsoever.** There is no `min-release-age` equivalent: `go get`
will take a version published minutes ago without complaint. This inverts the npm situation in a
way worth internalising:

| | pnpm | Go modules |
| --- | --- | --- |
| Cooldown enforced by the toolchain | Yes, on every install | **No** |
| Can be bypassed | Only by a recorded exclusion | Nothing to bypass |
| Bypass is visible | Yes, in the reviewed diff | **Nothing to detect** |
| Remaining control | Review + toolchain refusal | **Review only** |

Because review is the only control here, `go.mod` and `go.sum` are in
[`CODEOWNERS`](../../.github/CODEOWNERS). That entry is load-bearing rather than symmetrical
bookkeeping: under pnpm a slip cannot reach an install unrecorded, for Go it is not caught at all.

**Reachability filtering cuts both ways.** `govulncheck` reports only vulnerabilities the
application actually calls, which is what makes it trustworthy enough to act on. The cost is
coverage: an advisory the Go vulnerability database has not ingested yet produces no finding at
all, so a clean `govulncheck` says nothing about a GHSA published this week. Breadth is covered
separately by Trivy FS and OSV-Scanner, which match on version rather than call graph.

### pnpm

Every Node package resolves with pnpm — `scripts/` and `docs-viewer/` —
each carrying its own `pnpm-workspace.yaml` and `pnpm-lock.yaml`. The window is 7 days, derived
the same way as the other ecosystems; pnpm states it in minutes, so `minimumReleaseAge: 10080`.

The window is enforced on **every install**, not only while resolving: pnpm re-verifies the
entire lockfile against the active policies each time — including `--frozen-lockfile`, the
replay path CI and image builds use. An in-window entry cannot reach the lockfile and then
become invisible.

Verified behaviour, not inference:

| Action | Result |
| --- | --- |
| Resolving an exact in-window version, `minimumReleaseAgeStrict: true` | Rejected (`ERR_PNPM_NO_MATURE_MATCHING_VERSION`), naming the version, its publish time, and the cutoff |
| Resolving a range whose newest match is in-window | Silently resolves to the newest *aged* match, exit 0 |
| Resolving an exact in-window version, `minimumReleaseAgeStrict: false` | Succeeds — and pnpm writes the `minimumReleaseAgeExclude` entry into `pnpm-workspace.yaml` itself |
| `--frozen-lockfile` replaying an in-window entry that no exclusion covers | Rejected (`ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`) |
| Any `pnpm run` after a policy setting changed | Rejected (`ERR_PNPM_VERIFY_DEPS_BEFORE_RUN`) until `pnpm install` re-records the settings |

Three consequences follow. The first two are why no after-the-fact audit exists for the override
*reaching* the tree: there is nothing to detect, because nothing gets through unrecorded. The third
is where that argument stops — an exemption that entered under review can still outlive its date,
and being reviewed once says nothing about being revisited.

**An override cannot be silent.** Taking an in-window version requires a
`minimumReleaseAgeExclude` entry, and without one every later install fails — CI included, since
its install is the frozen replay. The entry lives in `pnpm-workspace.yaml`, which is tracked and in
[`CODEOWNERS`](../../.github/CODEOWNERS), so the override is reviewed by construction rather than
reported by a bot afterwards.

**`minimumReleaseAgeStrict: true` is what keeps the override a human's.** With it off, pnpm writes
the exclusion itself and carries on — the resolver, not a person, decides to take a fresh publish.
Strict mode turns that into a stop, on the same reasoning that puts install-shaped commands in
`ask` rather than letting them run: the point of an override is that someone chose it.

**An exclusion is dated debt whose removal date is load-bearing in both directions.** Deleting the
entry before the window opens breaks every install; once the version ages past the cutoff the entry
is redundant and the lockfile passes without it. Each entry therefore records what it exempts, why,
where that package runs, and the date it becomes deletable — and that date is machine-readable
rather than prose, because a date only a human reads is a date nobody re-reads.
It lives in [`.github/pnpm-cooldown-bypass.toml`](../../.github/pnpm-cooldown-bypass.toml), the same
shape as the `go-cooldown` / `tool-cooldown` bypass files, and
[`scripts/pnpm-cooldown`](../../scripts/pnpm-cooldown) (`make pnpm-cooldown-check`) fails on an
exclusion with no entry, an entry that has expired or reaches beyond three months, an entry matching
no exclusion, and an exclusion naming a version the lockfile no longer resolves. The deadline is kept
out of `pnpm-workspace.yaml` deliberately: it means nothing to pnpm, so it has no claim on the file
pnpm reads, and a check that had to read it there would be parsing comments — where a form pnpm
honours but the parser misses lets an undated exemption pass as no exemption at all. It runs on a
schedule as well as on the pull request, because the deadline arrives without either file changing.
This is the one check pnpm needed of its own — the resolver guards the window, nothing guarded the
exemption, and two entries sat 23 and 27 days past their stated removal date before the gate existed.

An exclusion names `<package>@<version>`, never a bare package name: a name-only exemption would
excuse every future publish of that package, which is exactly what the window exists to catch. The
same rule governs `trustPolicyExclude`, whose subject is provenance regression rather than freshness.

One operational caveat: the lockfile verification result is cached (`Lockfile passes supply-chain
policies (verified 1m ago)`), so editing a policy does not always force a re-check on the very next
install.

### PyPI

Python tools are the one exception to `mise.toml` holding every tool version, and the reason is a
supply-chain one: pinning a PyPI tool's version pins almost nothing, because its dependency tree is
resolved at install time, so the same pin installs a different tree on different days. Each tool
therefore declares its version in `python/<tool>.in` and carries the resolved tree — every
transitive package, with sha256 hashes — in `python/<tool>.txt`
([ADR-0084 (mise-ssot-drift-gate)](../adr/0084-mise-ssot-drift-gate.md); `python/README.md` has the mechanics). Installs
are `uv pip install --require-hashes -r <tool>.txt`, which refuses any requirement lacking a version
or a hash, so integrity verification is part of installing rather than a step that can be skipped.

**The window is 7 days**, derived the way npm's is — PyPI detects and yanks a malicious publish
quickly, so the interval to sit out is short. A bump therefore takes the newest release already aged
past the window, not the newest release; when a pin deliberately trails it says so in a comment, and
`python/graphify.in` is the PyPI instance of that convention.

Freshness has no resolver-side enforcement here — `uv pip compile` will resolve a version published
minutes ago without complaint. What enforces it is a repository gate, `scripts/tool-cooldown`, and
unlike npm's audit-only counterpart it **fails the build**.

Verified behaviour, not inference:

| Action | Result |
| --- | --- |
| `gate` (a pull request's diff) adds or raises a declaration inside the window | Non-zero exit, naming the version, its age, and the bypass file |
| `audit` (full inventory) finds an in-window declaration | Reported as a warning, **exit 0** — it takes stock, it does not block |
| A `.in` declares a version its `.txt` does not pin | Non-zero exit in **both** `gate` and `audit` |
| Installing | `--require-hashes` refuses any requirement without a version and a hash |

The third row is what keeps the first two honest. The gate reads the declaration while the installer
reads the lockfile, so without that cross-check raising a `.in` and forgetting `make py-lock` would
clear the cooldown for a version that never gets installed, and say nothing about the one that does.
It is a structural fault rather than a cooldown finding, which is why it fails the inventory command
too.

The `gate` / `audit` split is the same one drawn under *Why reporting and gating are split*: `gate`
judges what a change introduces and can therefore block, while `audit` inventories what is already
there, where findings arrive as versions age rather than as anyone's edit.

**A bypass is dated debt.** `.github/tool-cooldown-bypass.toml` takes
`"<key>@<version>" = { expires, issue, reason }`, all three required. An entry that has expired, that
reaches more than three months out, or that no longer matches any declaration fails `gate` *and*
`audit`. The expiry arrives without anyone editing a file, which is why the check also runs on a
schedule instead of only on pull requests.

Two properties make freshness sharper here than for a library:

- **A tool runs with the developer's privileges**, not inside the service. A compromised release
  executes on a workstation that has repo write access and whatever credentials the shell holds.
- **Cadence can be high enough that the newest version is never aged.** `graphifyy` published 198
  releases in its first 117 days, so its pin trails by design; the comment saying so is the steady
  state rather than a one-off note.

Discharging the window early follows the general rule above — answer the four questions with
`/supply-chain-triage` instead of counting days. A tool that ships an **agent skill** adds one
question a library does not have: **what it sends off the machine, and on which commands.** Which
of `graphifyy`'s commands stay local and which reach an API is recorded in
[`.claude/README.md`](../../.claude/README.md), which also documents the local subset as the
default path. Its installer additionally writes user-scope agent config (`~/.claude/skills/`,
`~/.codex/skills/`, `~/.claude/CLAUDE.md`) — outside the repo, and therefore outside every gate
described here.

The same tool also ships subcommands that write **project** scope: `graphify claude install`
rewrites `CLAUDE.md`, `graphify codex install` rewrites `AGENTS.md`, and `graphify hook install`
installs git hooks and a merge driver. A skill an agent can invoke therefore has a documented path
to edit the very files that define what that agent may do, which is why those forms sit in
`settings.json` rather than in prose alone.

They sit in **`ask`, not `deny`**, and as **patterns, not names**. `ask` because the goal is that no
install happens without a human choosing it — not that installs are forbidden; a `deny` also blocks
the legitimate case, and the user issues "use this tool" and "install this tool" as separate
instructions (`AGENTS.md`, *Installing Things*). Patterns because the risky thing is the shape
`<something> install`, not the platform names that happened to exist when the list was written:
`graphify --help` grew to 20 `<platform> install` subcommands, and an enumeration written against an
earlier release silently stops covering the new ones. A gate that decays without telling anyone is
worse than a narrower gate that holds.

**A pin bump is therefore a review of the CLI surface, not just of the version number.** The window
and the triage above answer "is this release trustworthy"; they say nothing about "does this release
expose new commands that write to the repo". Read the new `--help` when raising the pin, and check
that the patterns still cover what it can write. This is the failure mode that produced the
enumeration gap in the first place — 198 releases in 117 days, each one able to add a subcommand.

## Application runtime

The controls above protect what enters the repository. These protect what the running service
does with a request. Three patterns repeat, and they are the part worth carrying forward rather
than rediscovering.

**Deny by default, at every boundary.** The outbound dial guard
(`internal/observability/http_client_transport.go`, [ADR-0025 (egress-ssrf-guard)](../adr/0025-egress-ssrf-guard.md))
refuses link-local, multicast, unspecified, and bogon destinations unconditionally, and refuses
loopback / private / CGNAT unless the caller opted in through the context — with the unset case
resolving to the safe `false`. Error-detail exposure
(`internal/controller/httpstack/errorhandler/detail_exposure.go`) is the same shape: route
mismatch, unresolved operation, empty `operationId`, and not-opted-in all evaluate to *deny*.
Neither control has a state where forgetting something opens it.

**The spec is the authority for the request boundary.** Request validation and authentication are
enforced at runtime from the OpenAPI document ([ADR-0016 (spec-driven-request-validation)](../adr/0016-spec-driven-request-validation.md)),
not from hand-written checks in handlers. The direct consequence is that **reviewing the spec
diff is reviewing the security posture** — an operation that omits its `security` requirement is
unprotected no matter how the handler is written, and no amount of Go review will surface it.
Business-validity rules are deliberately *not* here: the domain layer is their sole authority
([ADR-0017 (validation-value-authority)](../adr/0017-validation-value-authority.md)), so the two never drift into each other.

**A credential in a URL is a logging problem, not only a transport one.** Realtime Delivery
authenticates its SSE stream with a ticket carried as a **query parameter**, because `EventSource`
cannot send an `Authorization` header ([ADR-0074 (query-ticket-stream-authentication)](../adr/0074-query-ticket-stream-authentication.md)).
That choice moves a credential into the one part of a request that infrastructure copies by default.
The in-process answer is to excise it from logs and span attributes, but that reaches only what this
process writes: the load balancer, the proxy and the CDN each log the query string on their own, so
the deployment must exclude or redact it on the stream path ([realtime-delivery.md §4.2](realtime-delivery.md#42-on-the-deployment-side)).
The mitigations that make this acceptable are that the ticket is short-lived, bound to a single
destination, single-purpose — it authenticates a stream and authorizes nothing — and explicitly
revocable. Note also that the stream's security scheme is a second authenticator in the OpenAPI
document, so the rule above holds for it too: an operation that omits its `security` requirement is
unprotected, and reviewing the spec diff is how that is caught.

**Escape hatches are named, narrow, and greppable.** `ContextWithAllowPrivateNetwork`, the
`details` property that opts an error schema into detail exposure, and `/metrics` as a declared
auth exception ([ADR-0020 (metrics-endpoint-auth-exception)](../adr/0020-metrics-endpoint-auth-exception.md)) are each a specific
seam you can search for and enumerate. None of them is a general-purpose flag, because a general
flag becomes the thing everyone sets.

Two absences are deliberate rather than pending: there is no in-application rate limiter
([ADR-0108 (no-in-app-rate-limiter)](../adr/0108-no-in-app-rate-limiter.md)), and responses are not validated against the
spec ([ADR-0016 (spec-driven-request-validation)](../adr/0016-spec-driven-request-validation.md)). SQL injection is handled
structurally instead of by review — queries are generated by sqlc and therefore parameterised
([ADR-0027 (sqlc-type-safe-sql)](../adr/0027-sqlc-type-safe-sql.md)) — and `gosec` runs in the authoritative
golangci gate ([ADR-0088 (layered-golangci-config)](../adr/0088-layered-golangci-config.md)).

A detail worth copying rather than rediscovering: Go's `netip.Addr.IsPrivate` covers RFC1918 and
ULA but **not** CGNAT (`100.64.0.0/10`), so the guard carries its own prefix check. Reimplementing
the dial guard from the standard library alone inherits that hole.

## Secrets

Two detectors, because a secret scanner can be wrong along two independent axes: it can fail to
recognise the shape, or it can flag a string that was never a live credential. gitleaks covers
pattern-shaped detection over the full history; TruffleHog reports only credentials it verified
against the issuing service.

One rule overrides convenience everywhere: **a detected secret's value never reaches a job log,
a PR comment, or an artifact.** Summaries carry detector name, path, line, and commit — never
the match itself. A leak report that leaks is worse than no report, because it publishes the
credential to a wider audience than the commit did.

A third path can leave the repository without passing either detector, and it is worth stating
because it looks like neither of the two above. The AI feedback loop can post excerpts of session
transcripts to public feedback issues, and a transcript is not a commit — both detectors scan what
is committed, so neither of them sees this path at all. What a person types into a session is also
precisely where a dead API key or a connection string gets pasted in order to ask about it:
measured over this repository's own history, 2 of 4,093 raw human turns carried a live credential.

**The path is deliberately kept closed on the normal route.** The loop reads its own transcripts
locally and publishes only the resulting prose, so the raw turns never leave the machine that
produced them. They are published in one case: the local reading could not run, and the excerpts
are the only way a CI runner can see what happened. That trade is why the reading was moved off CI
in the first place — a runner cannot read a file under the developer's home, so putting the reading
there forces the publication.

**Prose is not trusted merely because it is prose.** The reading is done by a model that could, if
left unconstrained, reach files nobody selected for it — so the local call runs outside the
repository with its file, shell and network tools denied, and what it returns is filtered again
before anything is posted. A section matching a secret pattern is dropped whole. Filtering only the
input would leave the output as an unwatched exit, which is the shape of the two paths above: a
control that inspects one direction and calls the surface covered.

On that fallback the loop filters its own candidates, and that filter is the only gate. Shape-based
detection cannot be complete, so the rule is to **drop anything suspicious**: a dropped excerpt
costs one of several candidates for the same window, while a published one cannot be recalled. The
patterns are in [`scripts/closed-loop/candidates.ts`](../../scripts/closed-loop/candidates.ts).

## The developer endpoint

Every control above sits on a path *into* the repository: what CI executes, what the application
links, what a request reaches. None of them observes a machine a developer already has. That is a
separate surface, deliberately out of scope — but it is easy to mistake for covered, because the
controls that leave it open are the ones that look most complete.

**A cooldown window protects a resolved tree, not a machine.** The window keeps a compromised publish
out of the lockfiles here — npm refuses it while resolving, pnpm re-checks the whole lockfile on
every install. Neither reaches an install a developer ran somewhere else: a scratch project, a global
CLI, a package pulled once to try it. That machine is exposed, and no lockfile here says so. The same
holds, more sharply, for everything that never reaches a lockfile
at all: editor and browser extensions, agent skills, and MCP server configs are user-scope by
construction (`~/.claude.json`, `~/.gemini/settings.json`, `~/.agents/.skill-lock.json`) — the shape
already noted above for a tool's user-scope installer, generalised to everything installed that way.

**The question is about state, not behaviour.** When an advisory names a package and a version, what
matters is which machines match *right now*. An SBOM answers what shipped and endpoint telemetry
answers what ran; neither answers what is sitting on disk. Answering it takes an inventory collected
from the endpoint plus a catalog of what to match against — and without the catalog, an inventory is
a census rather than an exposure check.

**It is not a repository control, and could not be made into one.** What is protected is the
developer's machine, not this repository, so the decision to inventory one belongs to whoever owns
it. Wiring a collector into `mise.toml` and `install-tools` would put it on every developer's host,
on machines whose owner never chose to be inventoried, and the Toolchain Execution Rules could not
carry it either: the
tool-runner containers cannot see the host state that is the entire subject. The repository's part
is to record the gap; closing it is the operating organisation's call.

**If it is closed, `perplexityai/bumblebee` is the candidate examined** — Apache-2.0, a single static
Go binary with no non-stdlib dependencies. It reads lockfiles, package-manager install metadata,
extension manifests and MCP JSON configs, and executes no package manager, so a scan cannot become
the incident it was meant to find. It has no configuration file: profile, roots, exposure catalog,
cadence (`launchd` / `cron`) and output are supplied per run and live outside the repository — so
taking it is a decision about a run, made when an advisory lands, rather than a decision about this
repository's toolchain.

## Honest limits

Worth stating so nobody builds on a stronger assumption than the controls support:

- `harden-runner` runs in `block` mode, and an allowlist is only as good as its accuracy. The
  lists are inferred from what each job does rather than measured, and they are generated from
  the SSOT in `.github/egress.toml` by capability class, so a class is wider than the narrowest
  job in it. One job — `trufflehog` — stays on `audit` deliberately: it verifies a candidate
  credential against an open-ended set of issuers, so a missing endpoint there would turn a real
  leak into an unverified result instead of a red build.
- Release gates only block if registered as **required status checks**. Without that branch
  protection rule they report and nothing more, and the reporting/gating split degrades to
  reporting everywhere.
- `CODEOWNERS` only enforces under the **"Require review from Code Owners"** rule; otherwise it
  merely auto-requests a reviewer.
- Detection reaches a PR comment and a run annotation. Both are passive, and the annotation
  disappears with the run's retention. Routing findings to a channel the owning role actually
  watches is tracked separately.

## Related

- [ADR-0093 (multi-layer-security-scanning)](../adr/0093-multi-layer-security-scanning.md) — layered scanning, reporting/gating split, runner hardening
- [ADR-0094 (sha-pinned-actions)](../adr/0094-sha-pinned-actions.md) — SHA-pinned Actions and the supply-chain quarantine
- [ADR-0105 (release-image-supply-chain)](../adr/0105-release-image-supply-chain.md) — release-image integrity (signing, provenance, SBOM)
- [ADR-0088 (layered-golangci-config)](../adr/0088-layered-golangci-config.md) — `gosec` as an in-process check during static analysis
- [`.github/workflows/README.md`](../../.github/workflows/README.md) — the workflow inventory and full trigger matrix
