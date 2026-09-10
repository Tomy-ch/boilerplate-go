---
name: context-map-audit
description: >-
  Detect drift between `docs/design/context-map.md` and the system's real contact points — an unmapped contact, a recorded edge whose counterpart disappeared, and a recorded label whose cited structural evidence no longer holds. Use after adding or removing an external dependency, before relying on the map, after changes under `internal/usecase/boundary/**`, `internal/infrastructure/**`, or `docs/design/**`, or for a periodic sweep; Japanese triggers include 「コンテキストマップは最新か」「地図に載っていない外部連携がないか」「接触点の棚卸し」. Read-only: a divergence may mean the map is stale or that code violates a decided relationship, and only a human can distinguish them. Do NOT use to create or extend the map (`context-map`), audit DDD patterns against Evans (`ddd-audit`), or check README-to-code drift (`back-prop`).
---

# Context Map Audit

Compare `docs/design/context-map.md` with the contact points the repository currently has.

A Japanese reference translation is available at `SKILL.ja.md`; do not load it as a skill.

## Read-only rule

A divergence has two valid readings: the map may be stale, or the code may have drifted from a
relationship that someone decided. Never edit the map: doing so silently assumes the former and can
hide the latter. Report the observation and its evidence; leave the decision to the maintainer.

## Workflow

### 1. Load the map

Read `docs/design/context-map.md`. If it is absent, report that it is absent and point to
`/context-map`. Do not treat absence as a finding and do not invent a baseline.

Extract every edge's counterpart, direction, label, and cited evidence. Keep `未確定` edges: they
are real map entries and may drift like any other edge.

### 2. Re-enumerate contact points deterministically

Resolve the current set from the repository at runtime, rather than from a list in this skill:

```sh
ls internal/usecase/boundary/
ls internal/infrastructure/
```

Read `internal/usecase/boundary/README.md` and `internal/infrastructure/README.md`. Apply the same
test as `/context-map`: identify a model crossing this system's boundary, not a port merely because
it is a port. Include inbound contacts.

### 3. Compare edge by edge

Keep these divergences separate because they call for different follow-up:

- **地図に無い接触点**: a repository contact point absent from the map.
- **相手が消えた辺**: a map edge whose counterpart is no longer present.
- **根拠が変わった辺**: an edge exists in both places, but current structure no longer supports the
  map's label. Inspect the evidence cited by the map for each edge, including whether a translating
  adapter remains, whether external vocabulary now crosses inward, and whether a published contract
  artifact was added or withdrawn.

Give particular attention to `根拠が変わった辺`: the other two are visually apparent, while this
one can look correct until somebody relies on its relationship label.

### 4. Report in Japanese

Use `file:line` evidence and this structure. State observations only: never use
「修正してください」, 「対応必須」, or 「違反」. Where an observation can lead to work, name the
responsible skill, such as `/context-map --update` for a missing edge, and stop there.

```text
context-map-audit 結果（辺 <N> 件 / 接触点 <M> 件）

[地図に無い接触点] <n> 件
  <接触点> — <どこで見つかったか file:line>

[相手が消えた辺] <n> 件
  <辺> — <地図の記載> / <現状>

[根拠が変わった辺] <n> 件
  <辺> — 記録ラベル: <L>
    地図の根拠: <引用されている file:line>
    現状: <その根拠がどう変わったか>

[未確定のまま残っている辺] <n> 件
  <辺> — <地図に書かれた未解決の問い>

総計: 追加候補 <n>, 消滅 <n>, 根拠変化 <n>, 未確定 <n>
```

List all `未確定` edges on every run. An open question cannot be answered if it stops being visible.

## Boundaries

- Read only the map and the relevant boundary, infrastructure, package, and design documentation.
- Write nothing: never change the map, source code, the DDD ledger, or `AGENTS.md`.
- Never hardcode contact points, infer a missing baseline, or omit `未確定` edges.
- Do not commit or push.

## Checklist

- [ ] Read the map and extract all edges, including `未確定`.
- [ ] Re-enumerate inbound and outbound contact points from boundary and infrastructure directories.
- [ ] Separate the three divergence types and inspect cited evidence for each possible label change.
- [ ] Report in Japanese with neutral wording and `file:line` evidence.
- [ ] Include every unresolved edge and the totals line.
- [ ] Make no writes, commit, or push.
