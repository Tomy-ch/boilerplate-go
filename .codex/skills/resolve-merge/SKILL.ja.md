> この文書は `SKILL.md` を正本として保守される日本語訳です。単独では編集しないでください。

# マージ解決

各パスを既存の機械的 resolver に振り分け、意味判断だけを人へ返してマージを完了させます。

## 使用する場面

- 依頼されたマージが衝突を報告した直後。
- 衝突なしのマージでも、連結物や派生物の再構築が必要なとき。
- 手動解決後、意図的に編集していない生成物でゲートが落ちたとき。

実装の意味判断、rebase や squash、重複レジストリ定義の選択、依頼されていないブランチ同期には使いません。

## 契約

| | |
| --- | --- |
| **担当する** | 衝突パスを分類し、再生成・resolver 再実行・集合和・派生の再伝播という既存の機械的手順を適用する |
| **決してしない** | 実装の意味を決める、生成物の片側を採る、rebase・squash・force-push、重複レジストリキーの勝者を決める |
| **開始条件** | 依頼されたマージの直後。Git が衝突を報告したかは問わない |
| **停止条件** | 機械的でない項目が 1 つでも残った時点。マーカーを保ち、コミットせず、コミットを提案もしない |

## このスキルが必要な理由

生成物には「正しい片側」がありません。`*.gen.go`、`*.sql.go`、`*_mock.go`、
`openapi.gen.yaml`、`database/gen/*.gen.sql`、生成文書、pin lockfile、バージョン派生値には、
それぞれ正本か resolver があります。片側を選ぶとマーカーは消えても正本から再現できず、後続の無関係な変更で
`gen-*-artifacts-check` が失敗し得ます。

名前が「衝突」ではなく **「マージ」** なのは意図的です。`database/gen/*.gen.sql` は DML 入力の連結物です。
両ブランチが別々の DML を追加すると、テキスト衝突なしでマージできても連結物は古いままです。すべての
コミット済み派生物に同じ危険があるため、衝突パスが 0 件でも再生成を省略しません。

## 引数

| 引数 | 効果 |
| --- | --- |
| `--base=<ref>` | ベース解決を行わず、この ref を使う。hotfix が関係するときは必須 |
| `--class=<csv>` | 指定クラスだけを処理し、それ以外は報告して触らない |
| `--dry-run` | 変更せず、調査・分類・予定操作の報告だけを行う |

## Step 1 — ポリシーを読み、ベースを確定してマージする

作業前に `AGENTS.md` と有効な Codex 運用保護を読みます。変更権限についてこの手順と衝突する場合は、寛容な
ほうを選ばず、両方の規則を報告して停止します。

優先順は、明示された `--base=<ref>`、現在の PR の `baseRefName`（`gh pr view`）、PR がない場合だけ
`make -s base-branch` です。`refs/remotes/origin/HEAD` や `gh repo view --json defaultBranchRef` からは
取りません。新しい release ラインができても既存 PR のベースを置き換えません。hotfix が関係し `--base` が
なければ、人に付与して再実行するよう依頼して停止し、ブランチ名から推測しません。

マージ中でなければ、カスタムコミットメッセージを与えずに取得・マージします。

```bash
git fetch origin "$BASE"
git merge "origin/${BASE}"
```

rebase は使わず、Git が生成する merge subject と body を保持します。

## Step 2 — 解決前にすべてのパスを分類する

未解決パスを次で確認します。

```bash
git diff --name-only --diff-filter=U
```

| クラス | パス | 機械的解決 |
| --- | --- | --- |
| Go / API 生成物 | `**/*.gen.go`、`**/*.sql.go`、`**/*_mock.go`、`openapi/openapi.gen.yaml` | 衝突した生成物を除き、所有する `make gen-api` または `make gen-query` で再生成 |
| DML 連結物 | `database/gen/*.gen.sql` | 割り当て済み worktree で `make merge-dml-ci work-dir=.`、続いて `make sqlc-generate-ci`。衝突なしでも実行 |
| 生成文書 | `docs/openapi/**`、`docs/godoc/**`、`docs/db-schema/**`、`docs/coverage/**`、`docs/portal/guides/**`、`docs/portal/docs.json` | 正本から再生成。release-push が同期するため feature PR には含めない |
| Pin lockfile | `.github/actions-pin.toml`、`docker/images-pin.toml` | `make pin-actions-resolve` と `make pin-actions-apply`、または `make pin-images-resolve` と `make pin-images-apply` を再実行。手で行を統合しない |
| バージョン派生 | `go.mod` の `go` directive、Dockerfile の `FROM`、文書中のバージョン | 先に `mise.toml` を解決し、`make sync-versions` |
| Vendored dependency | `vendor/**` | `go mod vendor` で再構築 |
| 追記専用レジストリ | `docs/adr/README.md`、`docs/spec/glossary.md`、`.agents/ddd-audit/pattern-ledger.yaml` の表 | 異なる項目の和集合。同一キーは人へ返す |
| Migration | `database/migrations/**` | 既存 migration は編集しない。番号衝突では新しい側だけを改番 |
| 対訳ペア | `**/*.ja.md` | 英語正本を先に解決し、`canonicalize-doc` で再同期。翻訳を直接解決しない |
| 実体化環境 | `env/.env` | コミットせず `repo-ops` section 7 に従う |
| 実装 | どの行にも一致しないもの | 非機械的。マーカーを保って人へ返す |

既定分類は必ず実装です。誤って人へ返すコストは 1 メッセージですが、意味を発明した解決は誤動作を静かに
着地させ得ます。

`canonicalize-doc`、`repo-ops`、`commit`、`submit-pr` などの兄弟 Codex スキル参照は、現在の
`.codex/skills/<name>/SKILL.md` を意味します。利用前に読みます。他のエージェント環境より古い可能性があるため、
現行リポジトリ方針との不一致は報告し、上位のリポジトリ方針に従います。

## Step 3 — 依存順に機械的クラスを適用する

派生物より先に正本を解決します。`mise.toml` → `sync-versions`、migration → DML 連結、DML → sqlc 生成、
OpenAPI 正本 → API 生成の順です。

生成物では生成物レベルの衝突を除き、解決済みの正本から全体を作り直します。生成行を統合したり片側を
選んだりしません。pin lockfile は resolver を再実行します。手で統合した tag-to-SHA cache は、上流に
存在しなかった対応を正しいものとして表せてしまいます。

`--class` では除外クラスに触れず、結果に列挙します。除外された未解決クラスは Step 6 の残件です。

## Step 4 — 衝突なしでも派生物を再構築する

Step 2 が 0 件でも、毎回該当する再構築を実行し、実行コマンドを明記します。Codex に割り当てられた
worktree では CI 用 SQL コマンドを使います。

```bash
make merge-dml-ci work-dir=.
make sqlc-generate-ci
make gen-api
make sync-versions
```

`--dry-run` では変更コマンドを実行せず、実行予定として報告します。生成文書は正本の変更または該当クラスが
ある場合だけ再構築し、リポジトリ方針に従い feature PR から除外します。

## Step 5 — 比例した検証を行う

関連する決定的チェックを実行し、正確なコマンドと結果を報告します。

```bash
make pin-actions-check
make pin-images-check
make check-migration-up-version
make check-migration-down-version
make check-migration-up-gap
make check-migration-down-gap
make md-doc-ref-lint
make md-skill-lint
```

有効な Codex の CI-first 保護が適用される間、ユーザーがローカル検証を明示しない限り、`make go-lint`、
`make go-test`、`make sql-lint` や同等物はローカルで実行しません。CI へ延期したゲートとして記録し、承認済みの
commit / push 後に得られる CI check を根拠にします。未実行ゲートを成功と表現しません。

機械的に clean と宣言する前に、未解決エントリと conflict marker を再確認します。根拠（Git 状態と
コマンド出力）と推論を分け、resolver が完了しなかったクラスの成功を発明しません。

## Step 6 — ちょうど 2 通りのどちらかで終える

分岐条件は、機械的でない残件または除外された未解決項目が残るかどうかだけです。

### 残件あり — その場で停止

マーカーを保ち、コミットせず、コミットも提案せず、人が解決した後に「仕上げ」として戻りません。人の解決を
含む結果は人のコミットです。日本語で次のように報告します。

```text
## 機械的に統合した項目

<クラス、パス、コマンド>

## 人に返す項目

<path> — <実装の意味的衝突 / 重複キー / ADR 採番規則の不一致 / ベース未確定>

## 検証

<実行・失敗・延期したゲート>

機械的に解けるところまで統合しました。残りはここからお願いします。
```

人が所有する項目は、実装の意味、両側で追加された同一レジストリキー、文書規約と実態が衝突する ADR 改番、
単一 ref に確定できないベースです。マーカー入り merge commit は、解決済みに見える壊れたツリーです。

### 残件なし — コミットや push の前に質問

先に `## 統合結果` と `## 検証` の日本語見出しで完了クラスとゲートを報告し、次の番号付き選択肢を
対話本文に提示して待ちます。

```text
質問: マージ元との統合が完了しました。コミットしてプッシュしますか？

1. コミットしてプッシュ
2. コミットのみ（プッシュしない）
3. 何もしない（作業ツリーのまま）
```

マージ成功を承認とみなしません。commit 作成前に `.codex/skills/commit/SKILL.md` を読み、手動で stage / commit
せずその workflow を使います。PR の作成・更新前に `.codex/skills/submit-pr/SKILL.md` を読みます。兄弟スキルは
未同期の可能性があるため、選択された終端を現行挙動が表現できるか呼び出し前に確認します。`commit` が開いている
PR への自動 push を所有する場合、その状態で選択肢 2 は実現できません。workflow を迂回せず、方針不一致を
報告して停止します。

既存 PR では、有効な workflow が push を別ゲートにできる箇所で、リポジトリ指定の確認文をそのまま使います。

> 変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？

amend、rebase、squash、force-push、現行スキルの確認契約の手動迂回は行いません。

## すること / しないこと

- PR の `baseRefName` を使い、hotfix では `--base` を必須にします。
- 変更前に全パスを分類し、未一致パスを実装とします。
- 生成物を正本から再構築し、pin resolver を再実行します。
- 衝突 0 件でも連結物と派生物を再構築します。
- 実行したゲートと CI へ延期したゲートをすべて明示します。
- Step 6 の 2 状態の一方だけで停止し、日本語で報告します。
- 生成物、lockfile、翻訳を手で編集したり片側採用したりしません。
- 実装の意味や重複レジストリ定義を決めません。
- 既存 migration を変更せず、古い側を改番しません。
- マーカーが残る状態でコミットせず、提案もしません。
- マージ成功だけを理由に push しません。
- rebase、squash、force-push、依頼ブランチ自身のベース以外の merge を行いません。

## チェックリスト

- [ ] 方針を読み、ベースは `--base` または PR の `baseRefName` から取得し、PR がないときだけ `make base-branch` を使った。
- [ ] カスタムメッセージなしの merge を使い、rebase しなかった。
- [ ] 解決前に全パスを分類し、未一致パスを実装とした。
- [ ] 生成物は正本から再構築し、pin データは resolver から得た。
- [ ] 追記専用レジストリを和集合にし、重複キーは判断せず返した。
- [ ] 日本語訳の同期より先に英語正本を解決した。
- [ ] 衝突 0 件でも DML、sqlc、API、バージョン派生を検討した。
- [ ] 実行コマンド、失敗、CI 延期ゲートを正確に報告した。
- [ ] Step 6 の 1 状態だけで終了した。
- [ ] 残件ではマーカーを保ち、commit も提案もしなかった。
- [ ] 機械的に clean なツリーでは番号付き commit / push 選択を待ち、現行 Codex workflow を使った。
