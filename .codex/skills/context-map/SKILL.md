---
name: context-map
description: >-
  Draft and land this repository's Context Map at `docs/design/context-map.md`, describing every boundary-crossing contact point with Evans's relationship vocabulary (Customer-Supplier, Conformist, Open Host Service, Published Language, Anticorruption Layer, and Separate Ways). A human confirms every relationship label — never infer one from code. Use when the map is missing, an external dependency has no mapped edge, someone asks how this system relates to neighbouring systems, or the DDD ledger marks `context-map` as uninterpreted; Japanese triggers include 「コンテキストマップを作って」「外部連携の関係を整理して」「境界の接触点を洗い出して」. Do NOT use for map-vs-code drift (`context-map-audit`), Evans-pattern auditing (`ddd-audit`), feature specifications (`new-spec`), or mechanics of a single integration.
---

# Context Map

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory (not loaded as a skill; for human reference only).

Create `docs/design/context-map.md`, the English-canonical relationship map for every place this
system exchanges a model with a context it does not own. Maintain
`docs/design/context-map.ja.md` as its Japanese translation.

## Non-negotiable rule

**Never choose a relationship label.** Enumerate contact points, state the structural evidence,
offer only labels compatible with that evidence, and obtain an explicit human decision per edge.

This is an accuracy rule, not a courtesy. Customer-Supplier and Conformist can look identical from
inside this codebase: both consume an upstream model. Their distinction—whether the upstream will
accept this system's requirements—is an organisational fact absent from source. Other relationships
also depend on commitments that code cannot prove. A guessed label makes an unresearched map look
settled; an unlabelled edge invites the decision that is still needed.

Structural facts are readable and must be documented: whether an adapter translates, whether a
contract is published, which vocabulary survives, and how a counterpart's change affects this
system. Relational facts must be asked. `未確定` is a correct, first-class result when the human
cannot yet decide.

## Workflow

### 1. Confirm scope

Ask explicitly in Japanese whether to map all contact points or only unmapped points (`--update`).

- **All contact points** is the default.
- With **`--update`**, read the existing English map first. Carry every existing edge and its label
  forward unchanged. Do not silently re-label a prior human decision; investigate and confirm only
  newly discovered contact points.

### 2. Enumerate the node set from the repository

Read these at runtime; they are the source of truth, not a list embedded in this skill:

```sh
ls internal/usecase/boundary/
ls internal/infrastructure/
```

Also read `internal/usecase/boundary/README.md` and `internal/infrastructure/README.md`. A contact
point exchanges a model with a system this repository does not own. A clock or transaction manager
port is not one merely because it is a port.

Include inbound contacts: the HTTP surface this system publishes and known consumers of emitted
models. Do not imply that no consumer exists merely because code does not identify one; record that
as an open question when necessary.

Present the complete enumeration in Japanese before gathering candidates. Correct the node set with
the user before continuing: an incorrect node set invalidates every relationship conclusion.

### 3. Gather verifiable evidence for every edge

For each contact point, inspect the relevant port, adapter, handler, schema, and design document.
Use `file:line` citations and record only what they demonstrate:

- Whether a translating adapter confines external concepts and translates into this system's
  vocabulary. This is structural evidence compatible with an Anticorruption Layer.
- Which vocabulary reaches the boundary. External vocabulary reaching inside is evidence that the
  system conforms in effect, irrespective of the intended relationship.
- Whether a committed, consumable contract artifact exists. That is evidence compatible with a
  Published Language; a surface designed for multiple consumers can also support Open Host Service.
- The direction of model flow, and what breaks here if the counterpart changes shape.

Do not mistake an implementation mechanism for a relationship. Link to its dedicated design or
package documentation instead of reproducing it in the map.

### 4. Offer candidates and expose the unresolved fact

For each edge, report in Japanese:

```text
<接触点> — 候補: <label A> / <label B>
  根拠: <file:line と、そこから確認できる構造的事実>
  未確定の点: <人だけが答えられる質問>
```

Offer every label the evidence is compatible with. If structure supports only one label, present it
as the sole candidate but still require confirmation; it is evidence, not a decision. Phrase the
gap as an answerable organisational question, for example whether the upstream accepts this
system's requirements.

### 5. Confirm each edge individually

Use Codex's explicit user-question interaction once per edge. Write the prompt and choices in
Japanese. Offer the evidence-compatible labels and always include:

```text
保留（未確定として地図に残す）
```

Do not batch edges, infer an answer, or proceed past a required edge confirmation. For a deferred
edge, record `未確定`, its evidence, and the open question. Do not use `未確定` as a reason to skip
the edge.

### 6. Write and synchronize the map

Write only these two documentation files:

- `docs/design/context-map.md` — English canonical
- `docs/design/context-map.ja.md` — Japanese translation

Use this structure:

1. **Node set** — this system and each external counterpart, with one line identifying it.
2. **Edges table** — counterpart, direction, relationship label, `file:line` evidence, and what
   changes if the other side moves. An undecided edge includes `未確定` and its open question.
3. **Mermaid diagram** — nodes and labelled edges only; do not add styling.
4. **Out of scope** — deliberately excluded contacts and why.

Keep the map about relationships and link to mechanical integration documentation where useful.
After finalizing the English document, use `canonicalize-doc` to synchronize its Japanese pair.
Preserve headings, paths, code blocks, and evidence citations in the translation.

`docs/design/**/*.md` is already within the DDD ledger corpus. Do not edit the ledger.

### 7. Close

Respond in Japanese with the landed edges, the edges left `未確定` and their open questions, and a
recommendation to rerun `/ddd-audit` for `context-map`. Do not commit or push.

## Boundaries and checklist

- Read only the relevant boundary, infrastructure, controller, OpenAPI, design, ADR, and package
  documentation needed to establish an edge.
- Write only `docs/design/context-map.md` and `docs/design/context-map.ja.md` while this skill
  executes. Never edit the DDD ledger, ADRs, source code, generated files, or `AGENTS.md`.
- Do not hardcode the contact-point list; resolve it from boundary and infrastructure at runtime.
- Do not turn structural evidence into a relationship decision.
- Run `make md-lint` after writing the document pair.
