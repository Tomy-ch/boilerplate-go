> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Verify Spec

spec 検証の統合スキル。2 つの spec ツリー（`docs/spec/domain/**/*.md` と `docs/spec/usecase/**/*.md`）のうち対象パッケージパスの spec を持つものに応じて、per-spec **read-only validator サブエージェント**を並列 fan-out。

## 使うとき

- `scaffold-endpoint` 起動前の spec 不整合検知（`scaffold-endpoint` が自動 chain）
- spec 編集後の全 check 確認
- spec 作成中のクイックチェック

単一 spec だけ確認したい時はこの統合スキルを実行する — 対象パッケージパスの domain / usecase spec のうち存在するものを検出し、該当 validator のみ fan-out する。

以下の用途には使いません:

- 生成コードの検証 — `make go-test`
- 実装 ↔ spec drift — `arch-check`
- 不整合の修正 — read-only、レポートのみ

## アーキテクチャ: 並列 validator サブエージェント

検証は `.claude/agents/` 配下の **read-only ワーカーサブエージェント**（spec ファイルごとに1つ）へ委譲。integrator は Agent tool（`subagent_type`）でこれらを並列起動:

| validator サブエージェント | spec | チェック内容 |
| --- | --- | --- |
| `spec-validator-domain` | `docs/spec/domain/<pkgpath>.md` | format + パス ↔ `package:` + entity ↔ SQL soft + 内部整合性 |
| `spec-validator-usecase` | `docs/spec/usecase/<pkgpath>.md` | format + パス ↔ `package:` + 依存先 domain spec への cross-spec + 命名規約 + Workflow 整合性 |

lean A 構成では controller / infra の spec ツリーが存在しないため spec 検証は不要（controller / infra は実装時に OpenAPI + sqlc gen から導出され、verify は `arch-check`（controller / infra 監査）が implementation 側で実施）。

validator は per-spec 検証ワーカーで**厳密に read-only**（auto-fix なし・書込なし）。両 validator は独立に読む — `spec-validator-usecase` は自身の `## Dependencies` から解決した domain spec を自分で読む — ため**書込依存がなく並列実行可能**。

## 最初のステップ: 対象パッケージパス確認

spec の同一性は feature ディレクトリではなく**パッケージパス**で決まる: `docs/spec/<layer>/<rest>.md` ⇔ `internal/<layer>/<rest>`。

`AskUserQuestion` を起動直後に必ず呼ぶ（`scaffold-endpoint` から呼ばれて context にパッケージパスがある場合は除く）:

- 質問: 「検証対象のパッケージパスを選んでください」
- 選択肢: `docs/spec/domain/**/*.md` と `docs/spec/usecase/**/*.md` を列挙し、`docs/spec/<layer>/` 接頭辞と `.md` を落としたパッケージパスの和集合 + 規約外パス用のフリーテキスト

`glossary.md` は `docs/spec/` 直下にありどちらのツリーにも入らないので、この列挙から特別扱いなしに外れる。

どちらのツリーにも当該パスの spec が無い時は明確メッセージで中断。

## Step 1. 存在する spec ファイル検出

確認済みパッケージパスについて、以下の存在を確認:

- `docs/spec/domain/<pkgpath>.md`
- `docs/spec/usecase/<pkgpath>.md`

両方なし → メッセージ出して中断。片方のみ → 該当 validator のみ fan-out。**同じパスに domain spec が無い usecase spec は正常**であり findings ではない — 集約を持たない usecase には domain spec が無く、`spec-validator-usecase` は実際に参照する domain spec を自身の `## Dependencies` から解決する。

## Step 2. validator サブエージェントを並列 fan-out

存在する spec ファイルについて、該当 validator を **Agent tool** で起動。**1メッセージ内に複数 tool 呼び出し**を並べて並列実行。各 validator に渡す:

- `pkgpath` — 確認済みパッケージパス
- `specPath` — spec ファイルパス（`docs/spec/domain/<pkgpath>.md` または `docs/spec/usecase/<pkgpath>.md`）

各 validator の最終メッセージ**が** findings（日本語）で、末尾に機械可読な `SUMMARY violations=<v> suggestions=<s>` 行を持つ。spec ラベル付きで収集し SUMMARY をパース。

> `spec-validator-*` を起動できない環境では、各 `spec-validator-<layer>.md` の手順を本文がインラインで実行する（domain 先行、usecase が参照するため）。

## Step 3. 集約レポート（日本語）

```text
verify-spec 統合結果（package: <pkgpath>）

[domain] violations: N, suggestions: K
  - <spec-validator-domain の findings>

[usecase] violations: N, suggestions: K
  - <spec-validator-usecase の findings>

総計: violations <sum>, suggestions <sum>
```

全 clean:

```text
verify-spec 統合結果（package: <pkgpath>）
全 spec で違反は検出されませんでした（チェック済み: <spec list>）。
```

## Step 4. クロージング

- **単独実行**: レポート出力して exit。違反があっても exit 0（情報的）
- **`scaffold-endpoint` から chain**: 集約 `violations > 0` のとき親に下流 chain 中断を通知。「scaffold can not safely proceed」を明示（suggestion では中断しない）

## AI 修正スコープ

完全 read-only。integrator と全 validator サブエージェントは spec / source ファイルを一切触らない。integrator が行うのは `AskUserQuestion`（パッケージパス確認、単独時）と read-only validator の起動のみ。

## 制約事項

- ❌ validator を逐次起動（必ず1メッセージ内で複数 Agent 呼び出し＝並列）
- ❌ ルールをハードコード — validator が `.claude/scaffold-spec/<layer>-spec.md` + `verify-rules.md` を毎回読む
- ❌ 違反の自動修正 / ファイル変更
- ❌ 対象確認 `AskUserQuestion` をスキップ（`scaffold-endpoint` 供与時を除く）
- ❌ 対象 spec ファイルが無い validator を fan-out
- ✅ 日本語集約レポート
- ✅ 存在する spec のみ fan-out
- ✅ per-spec validator / skill が独立 standalone 動作可能であることを維持
- ✅ 全 per-spec チェックを 1 パスで実施（fail-fast しない）；`scaffold-endpoint` から chain かつ violations 時のみ下流中断

## チェックリスト

- [ ] 対象パッケージパスを `AskUserQuestion` で確認（または `scaffold-endpoint` から供与）
- [ ] 存在する spec ファイル検出（domain / usecase ツリー）
- [ ] 該当 `spec-validator-*` を **1メッセージ内で並列起動**（pkgpath / specPath を渡す）
- [ ] 各 validator の SUMMARY を集約
- [ ] 集約日本語レポート出力
- [ ] `scaffold-endpoint` から chain 時のみ violations>0 で下流中断
- [ ] ファイル変更なし
