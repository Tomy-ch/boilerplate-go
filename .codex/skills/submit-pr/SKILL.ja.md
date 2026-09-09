> **このファイルは `SKILL.md` の日本語訳です。**
> 直接編集しないでください。内容の変更が必要な場合は canonical な `SKILL.md`（英語版）を更新し、その後この日本語訳を同期してください。
> Claude Code のスキルとしては `SKILL.md` のみが読み込まれます。このファイルはスキル本体ではなく、レビューや学習用の翻訳ドキュメントです。

# Submit PR

このスキルは、現在のブランチを `origin` に push し、対応する GitHub プルリクエストが存在することを保証する。次の 2 ケースを自動で判別して処理する。

- **Create**: 現在のブランチに対する PR が存在しない場合 → `-u` 付き push（upstream 未設定時）と新規 PR 作成を行う。
- **Update**: 既存の open PR が存在する場合 → ユーザーに確認してから push する（PR 上の diff は自動で更新される）。

PR の本文は `.github/pull_request_template.md` をひな型にして埋める。自動 push は行わず、既存 PR のタイトル / 本文を勝手に上書きせず、force push もしない。

## 前提

- `gh` CLI がインストールされ、認証済みであること（`gh auth status` が成功する）。
- 現在のブランチが保護ブランチ（`production` / `develop` / `staging` / `release/*`）ではないこと。
- working tree がクリーンであること。未コミット変更がある場合はスキルを中断し、先に `/commit` を実行するよう促す。

## Step 0. 事前チェック

並列で実行:

```sh
git rev-parse --abbrev-ref HEAD                          # 現在のブランチ
git status --porcelain                                   # working tree の状態
git rev-parse --verify '@{u}' 2>/dev/null                # upstream の有無
git log '@{u}'..HEAD --oneline 2>/dev/null               # 未 push コミット（upstream がある場合）
gh auth status
```

以下のいずれかに該当する場合は中断する。

- ブランチが `^(production|develop|staging|release/.+)$` にマッチする → feature ブランチに切り替えるようユーザーに伝える。
- `git status --porcelain` が非空 → 先に `/commit`（または stash）を実行するようユーザーに伝える。
- `gh auth status` が失敗 → `gh auth login` の実行をユーザーに依頼する。

Step 2 に進むときの状態は以下の 4 パターン。

| upstream | 未 push コミット | 意味 |
| --- | --- | --- |
| なし | n/a | 初回 push |
| あり | > 0 | 追加 push |
| あり | 0 | push 不要だが PR が未作成の可能性 |
| あり | 0 + PR open | 何もすることが無い（Step 2 で処理） |

### Step 1. push 前ローカルレビュー ゲート（確認）

事前チェックの bail-out を通過した直後、**何も compose せず push もする前に**、push 前の `/impl-review` を実行するか確認する。 これがローカルレビューの唯一の決定点。 ローカルレビューは実装者と別モデルでローカル差分を検査し、モックテストでは出ない不具合（認証 / IDOR・DI / SQL・共有スキーマ波及）を拾う。変更がローカルを離れる前に行うべきもの。 自動実行はしない。

`ask the user explicitly`:

- 質問: 「push 前に `/impl-review`（実装者とは別モデルの独立・敵対レビュー）を実行しますか？」
- 選択肢:
  - 「`/impl-review` を実行する（submit-pr はキャンセル）」 — 下記のキャンセル＆案内参照。
  - 「実行済み / 不要（このまま進める）」 — Step 2 へ進む。
  - 「キャンセル」 — 中止。

**レビューを選んだら submit-pr をキャンセルし、レビューへ案内する — `/impl-review` を inline chain せず、この run を再開しようともしない。** 次を表示する:

> submit-pr をキャンセルします。`/impl-review` を実行し、指摘を修正してから `/commit` で確定し、改めて `/submit-pr` を実行してください。（clean tree でないと push できないため、レビュー修正の commit を先に済ませる必要があります。次回はこの Step 1 で「実行済み」を選べばそのまま進みます。）

再開ではなくクリーンにキャンセルする理由: ローカルレビューはしばしば修正を生み、その修正は submit-pr が動く前に commit しておく必要がある（Step 0 の clean-tree 前提、Step 6 の push）。 どのみち working tree は変わるので「再開」するものは無く、修正を commit すれば次の `/submit-pr` はフレッシュで安価な run として素通りする。 inline chain せず案内に留めることで、submit-pr が抱えるべきでない review + fix + commit ループを持ち込まない。

**変更種別による深さ** — diff が触る範囲に応じて推奨をスケールする（このスケールは Step 9 の PR 後レビューにも効く）:

- **振る舞いに影響するコード**（`internal/**`・`pkg/**` の `.go`・SQL・OpenAPI）→ 既定でレビューを推奨。
- **ドキュメント / ツール主体の変更**（`docs/**`・`*.md`・`.codex/**`・`AGENTS.md`・CI 設定 — 本番挙動の変更なし）→ ROI が低い旨を伝え、素早く見送れるようにする（それでも尋ねる）。

diff の主たる性質（変更パス / commit prefix）で既定の推奨を判断するが、ユーザーの選択が常に優先。

## Step 2. 既存 PR とベースブランチの検出

```sh
gh pr view --json number,state,baseRefName,headRefName,url,title,body 2>/dev/null
make -s base-branch
```

結果に応じて分岐する。

- **PR が存在し state が `OPEN`** → "update" 経路。ベースブランチは固定（結果の `baseRefName`）。PR の base は既にその PR がマージ先にしているブランチであり、何かが再解決してよいものではない。
- **PR が存在するが state が `MERGED` / `CLOSED`** → `ask the user explicitly` でユーザーに確認:
  - 質問: 「このブランチには `<state>` 状態の PR #N があります。新規 PR を作成しますか？」
  - 選択肢: 「新規 PR を作成する」 / 「キャンセル」
- **PR が存在しない** → "create" 経路。ベースは `make base-branch` が解決するブランチ、すなわち `origin` の実状態から読んだ最新のリリースライン。`gh repo view --json defaultBranchRef` は使わない。GitHub のデフォルトブランチは現行のリリースラインより遅れており、そこへ向けて PR を作ると 1 世代前のベースを対象にしてしまう。

`make base-branch` が失敗した場合は、ベースを推測せず報告して止める。間違ったブランチへ PR を作ってしまうと、PR を編集して取り消せる種類の失敗ではない。

"create" 経路では、解決したベースを `ask the user explicitly` で確認する。backport や意図的な hotfix 対象は、解決器が知りようのないケースだからである:

- 質問: 「ベースブランチをこれで作成しますか？」
- 選択肢: 「`<resolved-base>` を使う」 / 「別のブランチを指定する」

早期終了の特殊ケース:

- "update" 経路で未 push コミットが 0 件 → push する必要が無い旨を伝えて終了。既存 PR の URL を表示する。
- "create" 経路で未 push コミットが 0 件だがリモートブランチは存在 → Step 3 へ進む（リモートに既にある内容で PR を作成する）。

## Step 3. コンテキスト収集とテンプレート読み込み

タイトル・本文を組み立てるための入力を収集する。`<base>` は Step 2 で確定したベースブランチ。

```sh
git log <base>..HEAD --pretty=format:'%h %s'                # コミットタイトル
git log <base>..HEAD --pretty=format:'%h%n%s%n%b%n---'      # コミットタイトル + 本文
git diff <base>...HEAD --shortstat                          # diff サマリ
git diff <base>...HEAD --name-only                          # 変更ファイル
```

`.github/pull_request_template.md` を読み、`#` / `##` ヘッダでセクションを識別する。現行テンプレートは以下のセクションを持つ。

- `# 概要`
- `## 変更内容`
- `## 動作確認方法`

HTML コメントのプレースホルダーは取り除く。テンプレートが存在しない場合は、同じ 3 セクション構成をインラインのフォールバックとして使う。

## Step 4. タイトルと本文の生成

### タイトル

- 最も大きい変更から導出する。単一コミットの PR ならそのコミットタイトルを使う（冗長な場合のみ先頭の `<Prefix>:` を外す）。複数コミットなら全体の意図を日本語で要約する。
- 70 文字以内。
- ブランチ名に issue 番号が埋め込まれている場合（`feature/1234-...`、`bugfix/5678-...`）、`#1234` を自然な形でタイトルに含める。
- "update" 経路では、ユーザーから明示的な指示がない限り既存タイトルを変更しない。

### 本文

テンプレートの各セクションを日本語で埋める。

- **概要**: PR の意図を 1〜3 文で要約。主にコミットメッセージから抽出する。
- **変更内容**: 領域別の箇条書き（API / DB / 内部ロジック / テスト / ドキュメント など）。変更ファイルとコミットタイトルを参照する。生のファイル一覧の貼り付けは避け、意味のある粒度でまとめる。
- **動作確認方法**: 具体的な確認手順。実際の変更内容に合わせて適応する（API 変更なら `make serve` + curl、マイグレーションなら `make db-local-migrate-up`、ロジックなら `make go-test` など）。

ブランチ名から issue 番号が拾えれば、本文末尾に `closes #N` を追加する（自然なら 概要 に折り込んでも可）。

## Step 5. ユーザー確認

push 前のローカルレビュー可否は **Step 1（Phase 0）** で既に確認済み — ここでは再度尋ねない。

確定したタイトル、ベースブランチ、push コマンド、本文全文を表示する。

### Create 経路

`ask the user explicitly`:

- 質問: 「以下の内容で PR を作成しますか？」
- 選択肢:
  - 「この内容で作成する」
  - 「draft で作成する」
  - 「title / body を修正したい」
  - 「キャンセル」

「修正したい」が選ばれた場合、自由記述のフィードバックを収集し、該当セクションを再生成して再確認する。

### Update 経路

未 push コミット一覧と diff サマリを表示してから、`AGENTS.md` で指定された文面で確認する。

- 質問: 「変更はローカルにコミット済みです。これらの変更をプルリクエストにプッシュしますか？」
- 選択肢: 「push する」 / 「キャンセル」

## Step 6. push

```sh
# 初回 push (upstream 未設定)
git push -u origin <branch>

# 2 回目以降
git push
```

`origin/release/*` から切ったブランチ（`commit` のマージ済み PR 復旧フロー）は upstream が**保護**ベースを指すため、素の `git push` は保護ブランチを対象にしてしまう。初回は必ず明示 refspec `git push -u origin <branch>` で upstream をフィーチャーブランチへ張り直す。それ以降のみ素の `git push` が安全。

ユーザーから明示指示がない限り `--force` / `--force-with-lease` は使わない。

push が失敗（non-fast-forward、権限エラー、ネットワークエラー等）した場合は、エラー内容をそのままユーザーに伝えて停止する。自動復旧は試みない。

### pre-push フックは自分で規模を決める — 先回りしない

`pre-push` は重い Go ゲートを `make gate-go-push` 経由で走らせ、`.makefiles/load.mk` が開いている worktree の数から「全開・絞る・CI へ委譲」のどれかを決める（`repo-ops` §19、現在の帯は `make load-status`）。その判断はフックに任せる。

push 前に「念のため」`make go-lint` / `make go-test` を手で回さないこと。窓が複数開いている状態では、CI が同一に再実行する内容を再発見するために飽和したホストを数分間占有するだけであり、その飽和自体が無関係なゲートを落とす。`ci-first` の帯では **push そのものが検証ステップである**。自分の変更と無関係な理由でフックが落ちた場合の `--no-verify` の扱いは `repo-ops` §11 が持つ。

帯がゲートを CI へ委譲した場合は Step 8 のレポートにその旨を書き、チェックが揃うまで PR は未検証として扱う。実行していないローカル検証を通ったかのように書かないこと。

## Step 7. PR の作成 / 更新

PR が存在した時点で打刻する。GitHub も作成時刻を持つが、両者は別の問いに答えており、そちらへの
結合は 3 分の 2 程度しか成立しない。

```sh
.agents/closed-loop/marks.sh prOpenedAt 2>/dev/null || true
```

### PR を作成する

```sh
gh pr create \
  --base "<base-branch>" \
  --title "<title>" \
  --body "$(cat <<'EOF'
<body>
EOF
)" [--draft]
```

### PR を更新する

Step 6 の push で既に PR の diff は更新されている。デフォルトでは PR のタイトル・本文には触れない。

ユーザーから明示的に更新指示があった場合のみ、以下を実行する。

```sh
gh pr edit <number> [--title "<new-title>"] [--body "$(cat <<'EOF'
<new-body>
EOF
)"]
```

## Step 8. 結果報告

PR の URL と簡単な要約を日本語で表示する。`make -s load-status` を実行する。解決された帯が `ci-first` の場合、push フックが重いゲートを CI へ委譲しているため、`gh pr checks --watch` で CI 結果を待ち、最終ステータスをこの報告へ含める。チェックが失敗したら失敗を報告し、PR を検証済みとして扱わない。`full` / `low` では CI を待たない。この手順は全 CI 専用チェックを待つためではなく、委譲した検証を追従するためのものである。

Create 経路:

```text
PR を作成しました: <url>
ベース: <base-branch>
タイトル: <title>
コミット数: N
CI 委譲: <なし / ci-first。gh pr checks の最終結果>
```

Update 経路:

```text
PR を更新しました: <url>
追加コミット数: N
CI 委譲: <なし / ci-first。gh pr checks の最終結果>
```

## Step 9. PR 後レビュー（確認）

PR の URL を報告したら、**必ず PR ベースのレビュー実行可否をユーザーに確認する**（スキップしない／自動実行しない）。 ここで扱うのは PR が存在して初めて可能なレビュー（push 前 `/impl-review` は Step 1 で提示済み）。 `ask the user explicitly` を使う:

- 質問: 「PR を作成/更新しました。コードレビューを実行しますか？」
- 選択肢（該当するものを提示）:
  - 「`/code-review <PR#>` を実行」 — PR ベースのレビュー（`--comment` でインラインコメント可）
  - 「ultrareview を案内」 — クラウド多エージェントレビュー。**ユーザー起動・課金**のためスキルからは起動できず、コマンドの案内のみ
  - 「`/impl-review` を実行」 — Step 1 の push 前ゲートを見送り、今ローカルの別モデル敵対レビュー（認証 / IDOR・DI / SQL・共有スキーマ波及などモックで出ない不具合）を回したい場合のみ提示
  - 「レビューしない」

既定の推奨は変更内容に応じてスケールする。 Step 1 の **変更種別による深さ** ガイダンスに従う（挙動に影響するコード → 既定で推奨 / ドキュメント・ツール主体 → ROI 低を添える）。 最終判断は常にユーザーが優先。

## 制約

- ❌ 保護ブランチ（`production` / `develop` / `staging` / `release/*`）への push
- ❌ `git push --force` / `--force-with-lease`（ユーザーから明示指示があった場合のみ可）
- ❌ 既存 PR のタイトル・本文の自動更新（明示指示があった場合のみ可）
- ❌ working tree に未コミット変更があるまま push する
- ❌ ユーザー確認なしで PR を作成する
- ❌ 既存 PR ブランチへの push を、`AGENTS.md` 指定の文面で再確認せずに実行する
- ✅ `.github/pull_request_template.md` を本文のひな型として使う
- ✅ タイトル・本文は日本語
- ✅ `gh pr create` / `gh pr edit` の body は HEREDOC で渡す
- ✅ ブランチ名から issue 番号を検出してタイトル / 本文に反映する

## チェックリスト

完了報告の前に以下を確認する。

- [ ] 現在のブランチが保護ブランチではない
- [ ] (必須) Phase 0（Step 1）で push 前 `/impl-review` の実行可否を確認した（レビューを選んだら submit-pr はキャンセルし、review→fix→commit→再実行へ案内）
- [ ] push 前の working tree がクリーンだった
- [ ] `gh auth status` が成功した
- [ ] PR テンプレートを読み、本文に反映した
- [ ] タイトル・本文が日本語である
- [ ] タイトルが 70 文字以内である
- [ ] push 前にユーザー確認を取得した（update 経路では `AGENTS.md` 規定文面で必須）
- [ ] PR URL をユーザーに伝えた
- [ ] (必須) PR 作成/更新後に PR ベースのレビュー実行可否を確認した（深さは変更種別でスケール）
- [ ] `--force` 系を使っていない
