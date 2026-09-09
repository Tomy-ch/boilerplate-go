---
name: context-map-audit
description: >-
  Detect drift between `docs/design/context-map.md` and the contact points this system actually has — a contact point with no edge, an edge whose counterpart is gone, and an edge whose structural evidence no longer supports its recorded label. Use it after adding or removing an external dependency, before trusting the map in a design discussion, when a `docs/design/**` or boundary change lands, or on a periodic sweep — 「コンテキストマップは最新か」「地図に載っていない外部連携がないか」「接触点の棚卸し」. Read-only: it reports and never edits the map, since a divergence can equally mean the map is stale or the code drifted from a decision. Do NOT use it to create or extend the map (`context-map`), to audit DDD patterns against Evans (`ddd-audit`), or to check README-to-code drift (`back-prop`).
allowed-tools: Bash(ls:*), Read, Glob, Grep
---

# Context Map Audit

Compares `docs/design/context-map.md` against the contact points the repository actually has, and
reports where the two disagree.

A Japanese reference translation of this skill is available at `SKILL.ja.md` in the same directory
(not loaded as a skill; for human reference only).

## Why this is read-only

A divergence has two readings and this skill cannot tell them apart. Either the map went stale, or
the code drifted away from a relationship someone decided on. **Editing the map would silently pick
the first reading every time** — and the second is the one that matters, because it means a decision
is being violated rather than merely out of date.

Report the disagreement and its evidence. Whether the map or the code is the error is the
maintainer's call.

## When to Use

- An external dependency was added or removed.
- Before relying on the map in a design discussion.
- A change landed under `internal/usecase/boundary/**`, `internal/infrastructure/**`, or
  `docs/design/**`.
- Periodic sweep.

Do NOT use for:

- Creating or extending the map — `context-map`.
- DDD patterns against Evans — `ddd-audit`. README↔code drift — `back-prop`.

## Step 1. Load the map

Read `docs/design/context-map.md`. If it does not exist, say so and point at `/context-map` — an
absent map is not a finding this skill can report on, and inventing a baseline defeats the purpose.

Extract the edge set: counterpart, direction, label, cited evidence. Keep `未確定` edges — they are
real entries and can drift like any other.

## Step 2. Re-enumerate contact points (deterministic — do not delegate)

Resolve the current set the same way `/context-map` does, from the repository rather than from a
list in this file:

```sh
ls internal/usecase/boundary/
ls internal/infrastructure/
```

Read `internal/usecase/boundary/README.md` and `internal/infrastructure/README.md` at runtime. Apply
the same test: a model crossing out of this system, not merely a port. Include the inbound direction.

## Step 3. Compare

Three kinds of divergence, kept separate because they ask for different things:

- **地図に無い接触点** — exists in the repository, absent from the map. The map understates the
  surface, and nobody has decided how this edge relates.
- **相手が消えた辺** — recorded in the map, no longer present. The map overstates the surface.
- **根拠が変わった辺** — both present, but the structural evidence no longer supports the recorded
  label. Check each against what the map cites: a translating adapter that was removed, external
  vocabulary now reaching the inside, a published contract artifact withdrawn or newly added.

The third is the one worth the run. The first two are visible to anyone who looks; the third looks
fine until someone relies on the label.

## Step 4. Report (Japanese, read-only)

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

State findings as observations. Do not write 「修正してください」「対応必須」— which side is wrong is
not something this skill knows. Where a finding is actionable, name the skill that would act
(`/context-map --update` for a missing edge) and stop there.

Carry the `未確定` edges into the report every run. An open question that stops being visible stops
being answered.

## AI Modification Scope

- Read: `docs/design/context-map.md`, `internal/usecase/boundary/**`, `internal/infrastructure/**`,
  per-package `README.md`
- Write: nothing
- Never touch: the map, the DDD ledger, source code, `AGENTS.md`

## Constraints

- ❌ 地図の書き換え（どちらが誤りかを決めるのは人間）
- ❌ 接触点の一覧を本文へハードコード（実行時に解決する）
- ❌ 裁定文言（「修正してください」「対応必須」「違反」）
- ❌ 地図が無いときにベースラインを推測して比較する
- ❌ 未確定の辺を報告から落とす
- ✅ 3 種の乖離を分けて報告（要求されることが違うため）
- ✅ 根拠は `file:line`、日本語で報告

## Checklist

- [ ] 地図を読み、辺の集合（未確定を含む）を取り出す
- [ ] 接触点を boundary / infrastructure から再列挙（inbound も含む）
- [ ] 3 種の乖離に分けて比較し、根拠変化を各辺の引用に対して確認
- [ ] 日本語で報告（裁定文言なし、未確定の辺を毎回載せる）
- [ ] 書き込みなし、commit / push なし
