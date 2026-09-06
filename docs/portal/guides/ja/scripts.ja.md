# scripts

`scripts/` には、コード生成・ドキュメント・バージョニング・プロジェクト初期設定のための**ユーティリティスクリプト**が格納されています。

## ディレクトリ構成

ツールごとに 1 つのディレクトリを置き、その働きを名前にする。複数のツールが必要とするものは `lib/` に置く。
Node の設定とパッケージ横断のゲートは直下に置く。各ツールが何のためにあるかは下の *スクリプトカテゴリ* にある
— 名前が運べないのはそこだからである。

## スクリプトカテゴリ

### ドキュメント生成

|スクリプト|説明|実行元|
|---|---|---|
|`portal/gen-portal-docs.ts`|`manifest.yaml` に基づきソースドキュメントをポータルの `guides/` にコピー|`make gen-docs`|
|`portal/gen-docs-json.ts`|ポータルアプリ用のナビゲーション `docs.json` を生成|`make gen-docs`|

### Lint

|スクリプト|説明|実行元|
|---|---|---|
|`marker-baseline/`|撤去マーカー（`boilerplate-only` / `sample-api`）の行数をファイルごとに `baseline.json` へ固定し、動いたら落とす。発火する本物のマーカーと、規約を説明する例示とは同じ形をしているため、除去側は後者を `MARKER_LITERAL_FILES` で宣言する。宣言し忘れると、除去が中断する（声が出る）か、例示した区域が黙って消える（空フェンスは valid な Markdown なので誰も鳴らない）。どちらの経路でも唯一の手がかりは「マーカー行が増えたこと」なので、そこを判断の場にする——ベースラインを更新するか、ファイルを宣言するか。再生成は `tsx scripts/marker-baseline --write`。|`make test`（vitest） <!-- boilerplate-only:line -->|
|`openapi-client-check/`|bundle 済み spec（`openapi/openapi.gen.yaml`）を frontend generator（`orval`）に渡して `tmp/openapi-client/` へ生成し、SSE の契約型（`DeliveryEvent` / `ControlEvent` / `StreamCursor`）が生成物に現れなければ失敗する。`redocly lint` と Spectral は spec を文書として判定するだけで、消費側の generator が component を型にできるかは見ない — サーバ側の契約破りが表面化するのは client の生成済み検証だけになる（[openapi/boundary-ownership.md](../openapi/boundary-ownership.ja.md)）という穴を埋める。生成物が空か宣言を含まない場合は「全型あり」ではなく退化入力（exit 2）として報告する。|`make openapi-client-check` / CI `oapi-lint.yaml`|
|`doc-ref-lint/`|ADR のファイル名 / H1 / 参照の整合と、英日ドキュメント対の存在を検査する。ADR 参照は番号と併せてファイル名の slug を持つため、再採番が黙って別の ADR を指すことはない。`docs/spec/**` は日本語版の spec 一式が入るまで対訳存在チェックから意図的に除外している。|`make md-lint` / `make md-doc-ref-lint`|
|`premise-lint/`|[docs/rules.md](../docs/rules.md) の *No premise the document will outlive* を機械化したもの。テンプレート作成後も残る Markdown（`docs/adr/**` / `docs/design/**` / `docs/rules.md` / 各層 README …）をマーカー除去後の姿で読み、テンプレートから作成した瞬間に真でなくなる自己参照があれば落とす。前提を書いてよいのは、セットアップが書き換え・削除する `README*` / `docs/get-started/**` と、`boilerplate-only` / `sample-api` マーカーで囲った領域だけ。同じ語の別語義は `allowances.ts` へ理由付きで宣言する。|`make md-premise-lint` <!-- boilerplate-only:line -->|
|`mermaid-lint/`|リポジトリ内 Markdown の ` ```mermaid ` フェンスを全抽出し（除外範囲は `markdownlint-cli2` と同一）、実 `mermaid.parse` で構文検証する（DOM は `linkedom` で供給）。壊れた図が 1 つでもあれば非 0 で終了。`markdownlint` は Markdown の体裁しか見ず図の文法を見ない、その穴を塞ぐ。|`make md-lint` / `make md-mermaid-lint`|
|`skill-lint/`|`.claude/**` のスキル / エージェント定義を意味的に検査する: frontmatter（`name` がディレクトリ / ファイル名と一致、`name` + `description` の存在）、対訳ペア（`SKILL.ja.md` の存在・frontmatter 不在・冒頭の翻訳注記・見出しレベル列が `SKILL.md` と一致）、参照の実在性（本文の `` `make <target>` `` が `Makefile` / `.makefiles/**` に実在、インラインコード中のリポジトリルート相対パスが実在）、スキル横断の節番号参照（`` `repo-ops` §19 `` が `repo-ops/SKILL.md` の実際に宣言する番号付き節に解決する。この検査だけは `.codex/**` も読む）。あわせて各 skill / agent が `.codex/**` にも存在することを検査する。スキル定義はエージェントの指示書でありながら、記述と実態の一致を誰も検査しておらず、片側の AI 環境にだけ入った skill にも誰も気づかない — その穴を塞ぐ。検査範囲と ignore ディレクティブは [Skill Lint](#skill-lint) を参照。|`make md-lint` / `make md-skill-lint`|
|`actions-shellcheck/`|`.github/actions/**` の `action.yaml` / `action.yml` を解析し、composite action の `runs.steps[].run` を抽出して各スクリプトを標準入力経由で `shellcheck` に掛け、指摘を `action.yaml` 上の行番号へ写し戻す。`actionlint` は `.github/workflows` しか走査せず、action マニフェストを直接渡してもワークフローとして解釈して失敗するため、composite action 内のシェルはどのゲートにも掛かっていなかった。その死角を埋める。方言はステップの `shell:` から決め、shebang として渡すことで `-s` を使わずに対象シェルを確定させる。`pwsh` / `python` / `cmd` や式で指定された `shell:` は検査せず skip として数える。`${{ }}` 式は行数を保つプレースホルダへ置換する（ワークフローの `run:` に対して `actionlint` が採る方式と同じ）。抽出したステップ数は、同じ YAML をそのままデコードして数えた件数とファイル単位で一致していなければならず、食い違えば非 0 で終了する。2 つの経路は独立に壊れるため、抽出が壊れた状態が「緑」として通ることはない。`run:` をブロック折り畳み（`>`）で書いた場合は拒否する（折り畳みは指摘の位置を写し戻す基準である改行を落とすため）。式がクオートされていたかどうかを本スクリプトが何も言わないのもこのプレースホルダ置換のためで、その問いが残るのは展開位置そのものを読む検査に限られる。担当は `make actions-zizmor`。|`make actions-lint` / `make actions-shellcheck`|
|`pr-comment-secret-lint/`|`.github/workflows/` の各ワークフローをジョブ単位に切り出し、`./.github/actions/upsert-pr-comment` を使うジョブが `GITHUB_TOKEN` 以外の secret を参照していれば失敗する（ワークフロー全体の `env:` も対象）。`actionlint` では表現できない規約を機械化したもので、規約の理由は [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) を参照。検出範囲は `${{ }}` 式に現れる secrets の直接参照（`secrets.NAME` / `secrets['NAME']` / `toJSON(secrets)` のようなコンテキスト全体）。別ジョブで読んで `needs.<job>.outputs` 経由で渡す間接参照は静的には追えず、検査を通る。|`make actions-lint` / `make actions-comment-secret-lint`|
|`pr-comment-fence-lint/`|ワークフローの `run:` ブロックが PR コメント本文を固定長の Markdown フェンスで囲んでいる場合、複製されている `fence_for` の実装が互いに一致しなくなった場合、本文を素通しさせるワークフローが inline code span の内側へ値を補間している場合に失敗する。`actionlint` では表現できない規約を機械化したもので、フェンスを囲む本文から算出すべき理由は [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) を参照。検出範囲は `echo` 中のリテラルなフェンス、実装同士の文字列一致、そしてシェル展開をリテラルな span で囲んだ形。変数経由や `jq` の連結で組んだ span はここからは見えず、ある本文が攻撃者制御かどうかはそもそも判定できない。いずれも規約に委ねる。span 検査はファイル単位で、まだ安全な形に乗っていない本文のための除外マップを持つ。エントリは追跡 issue を明記し、検査を素通りしたファイルが検査済みと区別できなくならないよう毎回出力され、その issue が解決したら消える。|`make actions-lint` / `make actions-comment-fence-lint`|
|`actions-cutoff-lint/`|ジョブに `timeout-minutes` が無い場合と、`./.github/actions/upsert-pr-comment` を呼ぶステップの `if:` がキャンセルされたジョブから到達できない場合・`title:` に打ち切り時の見出しが無い場合に失敗する。`actionlint` では表現できない規約を機械化したもので、打ち切りが何を残すべきか・なぜ 3 つで 1 本かは [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) を参照。検出範囲は条件中の `always()` / `cancelled()`（`failure()` はキャンセル時 false なので意図的に数えない）と、title 式中のリテラル `CUT OFF`。reusable workflow を呼ぶジョブは同キーが invalid なため除外する。構造は YAML パーサではなく桁で読む。ブロックスカラーの中身が必ず親より深い桁に来ることが前提で、入力がそもそもパースできることは同じターゲット内で先に走る `actionlint` が担保する。到達性を自ら打ち消す条件（`!always()`）は書けてしまい静的には捕まらないので、そこは規約が支える。|`make actions-lint` / `make actions-cutoff-lint`|

#### Skill Lint

`skill-lint/` は、Makefile のターゲット一覧・ファイルシステム・見出し抽出から機械的に導出できることだけを主張する（文面の良し悪しは判断しない）。参照検査が読むのは**コードフェンス外のインラインコードスパン**に限る（フェンス内は例示・出力サンプルであり実在性を保証しない）。

パス参照は、パスであることが一意に決まるときだけ検査する: 先頭セグメントがリポジトリルート直下の実在エントリであり、かつ末尾が `/` か basename にドットを含むもの。これにより Go の import パス（`database/sql`）、パッケージ修飾シンボル（`pkg/ptr.Copy`）、省略記法（`internal/controller/handler/...`）、文脈相対のファイル名（`SKILL.md`）は意図的に対象外となる — いずれも解決先が一意に決まらない。`<placeholder>` / `*` / `**` / `{a,b}` はパターンとして解決し、パスは参照元ファイルからの相対でも解決を試みる（スキルが同梱する `scripts/` を指せるようにするため）。

仮定の例示・任意配置・対向 AI ツール側リポジトリのファイルなど、意図的に不在な参照には、その行のどこかに ignore ディレクティブを置く:

```markdown
- `internal/controller/handler/debug/README.md` → `docs/portal/guides/controller-handler-debug.md` (if it were added) <!-- skill-lint-ignore -->
```

**スキル横断の節番号参照** —— `` `repo-ops` §19 ``、`` `repo-ops` section 19 ``、`` `repo-ops` の §19 ``
—— は、そのスキルの `SKILL.md` が宣言する番号付き見出し `## N.` に照らして解決する。この検査だけは
`.claude/**` に加えて `.codex/**` も読み、索引はツリーごとに別で作る。Codex 側の文書の参照は Codex 側の
写しを指すので、Claude 側の採番で判定すると、同期途中に実在する節を「無い」と報告してしまう。

make ターゲットやパスと違い、採番し直された節は参照を構文として壊さない —— 別の場所を指すだけであり、
それは「エージェントが誤った手順を実行する」形で表に出る。意図的に**判定しない**ケースが 2 つある:
インストール済みスキルでない名前（code span 中の無関係な語かもしれない）と、番号付き節を 1 つも宣言して
いないスキル（別の見出し規約かもしれない）。スキル名が code span に入っていることを要求しているのが、
`RFC 7235 section 3` を巻き込まない仕掛けである。

2 つの AI 環境をまたぐ検査は**存在のみ**に限る: `.claude/skills/<name>/` には `.codex/skills/<name>/` があり（逆も同様）、`.claude/agents/<name>.md` には `.codex/agents/<name>.toml` があり（逆も同様）、Codex の各スキルは README が定める `SKILL.md` + `agents/openai.yaml` を持つこと。Codex 側の `SKILL.ja.md` は任意なので、存在するときだけ対訳ペアとして検査する。

本文の追随は意図的に検査しない。`sync-ai` は逐語コピーではなく意味ポートであり、`CLAUDE.md` ↔ `AGENTS.md` の言い換え・Claude 固有機構の適応・凝縮スタイルへの書き下ろしといった意図的な差分が恒久的に残る（共通スキルのうち少なからぬ数は、見出し集合がまったく重ならない）。存在の対応であれば例外は宣言可能な件数に収まり、それでいて肝心の事故 — 片側の環境にだけマージされた skill — は捕まえられる。

意図的に片側の環境だけへ置く skill は、`skill-lint/checks.ts` の `PLATFORM_ONLY_SKILLS` へ**理由付きで**登録する。理由が空の登録は落ち、両環境に揃った（あるいはどちらにも無くなった）skill の登録も落ちるので、例外リストが例外より長生きすることはない。**この仕組みの正典はここで、他のドキュメントは再掲せずここへリンクする。**

エージェント役割にはこの逃げ道が無く、対応は無条件に要求される。意図的に片側だけへ置いたエージェントはこれまで無く、例外の表を用意してもエントリが 0 件になる — 例外機構は実際の事例が出てから足す。

本リポジトリが書いていない skill は、`skill-lint/checks.ts` の `EXTERNAL_SKILLS` へやはり理由付きで登録して、上の全検査から外す。それらは [`.claude/scripts/bootstrap-external-skills.sh`](../.claude/README.ja.md) が pin 済みのパッケージから展開するもので gitignore されており、frontmatter も対訳も参照するパスも upstream が決める — 参照先の多くは実行後にしか存在しない成果物である。検査してもここでは誰も対応できない指摘が出るだけで、対応できない指摘を出すゲートは読まれなくなる。チェックアウトに存在しないことがこれらの登録では正常なので、腐り検査は持たせない。着地先ディレクトリ*への*パス参照は、存在しないではなく存在するものとして解決する — この仕組みを説明する文書はいずれもそのディレクトリ名を書くことになり、実在を要求する検査はどれも通せないため。解決するのは宣言済みの名前だけなので、綴り違いは従来どおり落ちる。

### バージョニング

|スクリプト|説明|実行元|
|---|---|---|
|`semver/`|セマンティックバージョンのバンプ（patch / minor / major）|リリースワークフロー|
|`stamp-openapi-version/`|`release/vX.Y.Z` のブランチ名から `X.Y.Z` を導出し `openapi.yaml` の `info.version` に書き込む（先頭の `version:` 行のみ・冪等・非 release ref は no-op）。契約版のみで SHA / build metadata は付けない（commit 単位の追跡は runtime の `/version` の責務）。`tsx` 経由で実行する。|`auto-generate-docs.yaml`|
|`sync-versions/`|Go 実装の sync ツール。`mise.toml` の `[tools]` table を行ベース parser で解析し（外部依存ゼロ）、`go` / `node` / `python` を `go.mod` の `go` directive と `docker/*/Dockerfile` の `FROM golang:` / `FROM node:` / `FROM python:` 行へ反映する。version 存在・ファイル存在・期待マッチ数の事前 validate を全 rule で通してからファイル単位 atomic に書き出すため、partial state にならない。|`make sync-versions`|
|`release/`|リリースタグ（`tag`）と次のリリースブランチ（`branch`）を作る。次バージョンは `git tag` の最新セマンティックバージョンから `-bump patch\|minor\|major` で決める。手順が make のレシピではなくここに在るのは、どちらも取り消しの効かない操作（タグの push / GitHub Release の作成 / デフォルトブランチの切り替え）を含み、分岐を実地で確かめようとすると本当にリリースするしかないためである。手順の組み立てと中止条件は純粋関数へ寄せてテストで固定してある。|`make tag-patch` / `tag-minor` / `tag-major` / `branch-patch` / `branch-minor` / `branch-major` / `hotfix-patch`|
|`base-branch/`|フィーチャーブランチの分岐元となる最新のリリースラインのブランチ名を出力する。出所は `origin` の実状態（`git ls-remote --heads origin 'refs/heads/release/*'`）で、ローカルの参照は一切読まない。`refs/remotes/origin/HEAD` は clone 時に決まったきり `git fetch` では更新されず、GitHub のデフォルトブランチも前のリリースラインを指したままのことがある。どちらも警告なく古い答えを返すので、1 世代前のベースからフィーチャーブランチを切る事故になる。「最新」は `major` / `minor` / `patch` の数値比較で、ブランチを作る `release/` が次版を決める基準と同じにしてある（作る側と解決する側の基準が揃う）。コミット日時は古いラインへの hotfix やベース merge で前後し、文字列順は `v1.10.0` を `v1.9.0` より前に並べる。`release/vX.Y.Z` が 1 本も無いリモートは空の答えではなくエラーにする — 空のベースと解決できなかったことを呼び出し側が区別できないため。対象は `release/*` のみで、これが解決する規則（「最新の `release/*` からフィーチャーブランチを切る」）に合わせてある。`make hotfix-patch` が GitHub のデフォルトに設定する `hotfix/*` は候補に入れない。|`make base-branch`|

その他のツールのバージョンは [`mise.toml`](../mise.toml) を SSOT として管理しています（PyPI のツールだけは例外で、[`python/`](../python/README.ja.md) で宣言しハッシュ固定の lockfile から入れます）。各環境（host / docker / CI）は必要なものだけ `mise install <tool>`（または `uv pip install --require-hashes`）で個別に取得するため、sync スクリプトは不要です。

### Makefile サポート

|スクリプト|説明|実行元|
|---|---|---|
|`make-help/`|`.makefiles/*.mk` を解析してターゲット説明を表示|`make help`|
|`load-band/`|`git worktree` の数からホストの負荷帯（`full` / `low` / `ci-first`）と 1 窓あたりの CPU share を解決し、レシピが `eval` できる `KEY=VALUE`（`env`）または人間向けの要約（`status`）として出力する。解決は make のパース時ではなくレシピ内で行うため、重い処理を伴わないターゲットはこのコストを払わない。置き換え前のシェルは窓数を `git worktree list \| grep -c . \|\| echo 1` で数えており、git が答えられないときに `0` と `1` の両方を出力していた。結果、比較が `integer expression expected` で失敗し、帯は黙って `full` へ縮退していた。|`make load-status` / `gate-*` 系ターゲット|

### コード生成

|スクリプト|説明|実行元|
|---|---|---|
|`genctxkey/`|Echo コンテキストキーヘルパーの生成（Go コードジェネレータ）。`internal/controller/ctxhelper/generate.go` の `//go:generate` ディレクティブから `go generate ./...` 経由で実行される。|`make gen-go-code`|

詳細は [genctxkey/README.ja.md](genctxkey/README.ja.md) を参照。

### CI / サプライチェーン

|スクリプト|説明|実行元|
|---|---|---|
|`pin-actions/`|`.github/workflows/**` と `.github/actions/**` の外部 GitHub Actions `uses:` を不変の commit SHA へ固定する。`resolve` は参照を走査し各 tag/branch を `git ls-remote` で SHA へ解決して lockfile `.github/actions-pin.toml`（SSOT）へ書き出す。`PIN_ACTIONS_MIN_AGE_DAYS`（既定 14）日未満の新しすぎるコミットは採用せず既存ピンを維持する supply-chain quarantine 付き。`apply` は lockfile を元に各 `uses:` を `@<sha> # <tag>` へ書き換える。`check` は書き換えずに同じ判定を行い、未固定/古い/未登録があれば非 0 で終了する（CI / hook 用）。既に固定済みの行はコメント末尾の `# <tag>` を版として再解決するため冪等。|`make pin-actions-resolve` / `pin-actions-apply` / `pin-actions-check`|
|`pin-images/`|`docker/*/Dockerfile` の全 `FROM` base image を不変の digest へ固定する。`resolve` は各 `image:tag` を集め `docker buildx imagetools inspect` で現在 digest へ解決して lockfile `docker/images-pin.toml`（SSOT）へ書き出す。image-config の `created` が `PIN_IMAGES_MIN_AGE_DAYS`（既定 14）日未満の digest は採用しない supply-chain cooldown 付き。mutable tag は履歴を問えないため step-back 先はツール自身の前回 lock で、初回（無い場合）は tag のまま残す。`apply` は lockfile を元に各 `FROM` を `image:tag@sha256:...` へ正規化し、quarantine 中の image は digest を剥がして tag のみへ戻す。`check` は書き換えずに同じ判定を行い、drift があれば非 0 で終了する（CI / hook 用）。tag は版の SSOT として `FROM` 行に残す。|`make pin-images-resolve` / `pin-images-apply` / `pin-images-check`|
|`egress/`|各ジョブのインライン harden-runner `allowed-endpoints` を `.github/egress.toml`（SSOT）から生成する。ジョブは自分が属する能力クラス（`base` / `mise` / `image` / `db`）と自分固有の `extra` を宣言し、ホストの列挙はクラス定義が持つ。`apply` は `.github/workflows/*.yaml` の各 `allowed-endpoints:` の折り畳みブロックを書き換え、`check` は書き換えずに同じ判定を行い drift があれば非 0 で終了する（CI / hook 用）。黙って通さず fail-close する: SSOT に無いジョブのブロック、どの workflow も名乗らない SSOT エントリ、SSOT と食い違う `egress-policy`、ブロック内のホスト以外の行は、いずれもエラーになる。このステップは checkout より前に走る必要があり composite action へ括り出せないため、重複の除去はこの SSOT が担う — [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) の「ランナーのハードニング」節を参照。|`make egress-apply` / `egress-check`|
|`go-cooldown/`|Go module proxy が返す公開時刻（`<module>/@v/<version>.info`）で `go.mod` を供給網 cooldown 窓に照らす。GOPROXY プロトコルの一部なので追加依存は不要。`gate` は base ref と比較し、その変更が追加 / 更新した **direct** の require が窓内なら失敗する。indirect は MVS が direct の要求下限より上に固定することがあり PR 側で下げられないため報告に留める。`audit` は全 require を棚卸しし、窓そのものでは失敗しない（既存依存は grandfather）。`.github/go-cooldown-bypass.toml` のエントリが期限切れ・3 ヶ月超・`go.mod` に不在のいずれかなら双方で失敗し、無効なエントリは効力も失うので失効したバイパスがモジュールを通し続けることはない。pnpm の `minimumReleaseAge` と違い Go は解決時に窓を強制しないため、この検査は検知器ではなく防御そのものである。|`make go-cooldown-gate BASE=<ref>` / `make go-cooldown-audit`|
|`tool-cooldown/`|このリポジトリが宣言するツール版を供給網 cooldown 窓に照らす。対象は mise が解決するもの全部（`mise.toml`）に加え、hash 固定の lockfile から入る PyPI ツール（`python/*.in`、[ADR-0084 (mise-ssot-drift-gate)](../docs/adr/0084-mise-ssot-drift-gate.md)）。窓はツールではなく backend の性質で決まる。GitHub リリース経由（aqua / ubi / github）は 14 日で、tag が別 commit へ付け替えられ得るぶん `pin-actions` / `pin-images` と揃える。パッケージレジストリ経由（go / npm / PyPI）は公開が immutable なので 7 日で、`go-cooldown` と揃う。lockfile 側（推移依存）は `go-cooldown` が direct のみを見るのと同じ理由で対象外。公開時刻はそれぞれ GitHub Releases API・Go module proxy・npm registry・PyPI から取る。`go:` backend はパッケージパスを指すため、proxy が答えるまで接頭辞を遡ってモジュールパスを見つける。短縮名の backend は対応表を持たず `mise registry` に解決させる（表を持つと mise の更新で静かにずれる）。**言語ランタイム（`core:` backend）は受容したリスクとして対象外** — go / node / python の配布自体が汚染される事態は供給網の 1 リンクではなく言語の信頼モデルの崩壊であり、冷却期間で守れるものが無い。`gate` は base ref と比較して失敗し、`audit` は全件を棚卸しして窓では失敗しない。双方とも `python/*.in` の宣言と `python/*.txt` の lockfile が違う版を指していれば失敗する（cooldown を通した版が実際に入る版と別になるため。再生成は `make py-lock`）。また `.github/tool-cooldown-bypass.toml` のエントリが期限切れ・3 ヶ月超・対象不在なら失敗し、無効なエントリは効力を失う。|`make tool-cooldown-gate BASE=<ref>` / `make tool-cooldown-audit`|
|`pnpm-cooldown/`|各 `pnpm-workspace.yaml` の `minimumReleaseAgeExclude` を、[`.github/pnpm-cooldown-bypass.toml`](../.github/pnpm-cooldown-bypass.toml) が持つ期限と突き合わせて検査する。`go-cooldown` / `tool-cooldown` と違い、これは窓の番人ではない。窓そのものは pnpm の解決器が強制しており、しかも install のたびに lockfile 全体を照らし直す（`--frozen-lockfile` の再生も含む）。番人が居なかったのは免除のほう。期限を宣言ではなく専用 TOML へ置くのは兄弟と同じ理由で、`expires` は pnpm にとって意味を持たないため pnpm が読むファイルを占める筋合いが無く、外へ出すことで検査がコメント解釈の経路から離れる——そこは pnpm が honor するのに読み手が取りこぼす書き方があり、期限の無い免除を通してしまう。期限の無い例外、期限切れ、上限 3 ヶ月超過、どの例外にも対応しないエントリ、対になる `pnpm-lock.yaml` がもう解決していない版、のいずれでも失敗する。宣言は行走査ではなく `yaml.Node` として読む。YAML は同じシーケンスを何通りにも書ける（インデントあり・キーと同じ桁・フロー形式・引用符つき）ため、pnpm が honor する形を 1 つでも取りこぼすと、期限の無い免除が「免除ゼロ」として通る。lockfile のキーは比較前に正規化する（`'@scope/name@1.0.0'` は quote され、`name@1.0.0(peer@2.0.0)` は peer サフィックスを持つ）。素の完全一致では scoped パッケージが常に未解決に見えるため。期限はどちらのファイルが変わらなくても訪れるので、pull request だけでなくスケジュールでも走る。|`make pnpm-cooldown-check`|
|`migration-lint/`|`database/migrations` の連番について、重複（`-check duplicate`）と欠番（`-check gap`）を検査する。読むのは `<連番>_<名前>.<kind>.sql` の最初の `_` より前で、up / down は `-kind` で切り替える。lefthook の pre-commit ゲートから呼ばれる。判定がシェルのレシピではなく Go に在るのは、この検査の壊れ方が「何も検査しなくなる」方向に出るためで、そこはテストで固定できるがシェルのパイプラインでは固定できない。|`make check-migration-up-version` / `check-migration-down-version` / `check-migration-up-gap` / `check-migration-down-gap`|
|`cover-gate/`|`go tool cover -func` が報告する総カバレッジを `-threshold` の値と比較し、下回れば非 0 で終了する。`total:` 行の抽出と判定を別々の純粋関数に分けてあるため双方をテストで固定できる。置き換え前の `awk` パイプラインは数値でないパーセント表記を `t+0` で `0` に丸めていたため、壊れたプロファイルを「ツールの失敗」ではなく「カバレッジ不足」として報告していた。|`make cover-gate`|

### ローカル環境の検証

|スクリプト|説明|実行元|
|---|---|---|
|`realtime-smoke/`|Realtime Delivery が production で行う呼び出し — DynamoDB の conditional put / `ConsistentRead` query / pagination / TTL、SNS topic → N 個の SQS queue への `RawMessageDelivery` 配送と queue policy — を AWS SDK Go v2 で DynamoDB Local と GoAWS に投げ、呼び出しごとに 互換（受理され事後条件も成立）/ 非互換（受理されたが事後条件が不成立、または拒否）/ 未対応（エミュレータが未実装を明示）/ 検証不能（transport 失敗、または先行検査の失敗）を表にする。local compatibility implementation の範囲になるのは 非互換 / 未対応 の行だけで、末尾の要約はそれだけを列挙する。resource は実行ごとの乱数名で作り終了時に削除する（`-keep` で残せる）。検証不能が 1 件でもあれば非 0 で終了し（何も見ていない実行がクリーンに読めてはならない）、`-strict` は CI 向けにそれを 非互換 / 未対応 へ広げる。wire protocol の probe（AWS JSON 1.0 での `ListQueues`）を最初に置くのは後続の全呼び出しがそれに依存するためで、先行検査の失敗は依存する行を落とさず 検証不能 として表に残す。|`make realtime-smoke`|

### AI フィードバックループ（`closed-loop/`）

|スクリプト|説明|実行元|
|---|---|---|
|`closed-loop/`|`tmp/closed-loop/marks/` に窓が残した打刻を読み、窓ごとのフェーズ区間・重複した打刻・数字を使う前に見るべき状態（開始の打刻が無い / 閉じられていない / 打刻が順序どおりでない）を報告する。`--transcripts` / `--codex` を渡せば、期間内のセッション記録を決定論的な数——発話数・ツール呼び出し・失敗・中断・スキル呼出——へ畳み、一度も呼ばれなかったスキルも挙げる（回数だけでは撤去の根拠にならない、という但し書き付きで）。窓とは何か、なぜ順序をタイムスタンプではなく宣言から取るのかは [ADR-0010 (development-window-as-feedback-unit)](../docs/adr/0010-development-window-as-feedback-unit.md)。|`make closed-loop-report`|
|`closed-loop/send/`|閉じたのに Feedback Issue へ到達していない窓を送る。観測ブロックが Issue 本文になり、その読解もここで作る——選んだターンに対する手元のモデル呼び出しなので、トランスクリプトはマシンから出ない。その呼び出しが走らなかったときだけ `feedback/needs-summary` ラベルを付け、選んだターンをコメントとして投稿する。ランナーがそれを見る唯一の手段だからである。逐語が公開されるのはこの経路だけで、[`docs/design/security.ja.md`](../docs/design/security.ja.md) に記録してある。Issue の作成とコメントの投稿は別の失敗として扱う——コメントの無い Issue はコメントだけを再試行する。束ねると半分だけ送れた窓が「送出済み」に見えるためである。ホストで動く: 読むトランスクリプトが利用者のホーム配下にあり、tool-runner コンテナからは見えない。|`make closed-loop-send` / SessionStart hook|
|`closed-loop/weekly/`|期間内に作られた Feedback Issue を集め、付いている分類ラベルでまとめ、頻度 / 影響 / 人の介入 / 再発でクラスタごとにスコアを付けて、レトロが着手順を持てるようにする。各 Issue の `Suggested Improvement` をクラスタの下に並べるので、議題を作るのに Issue を 1 件ずつ開かなくてよい。再発を最も重く見るのは、窓をまたいで再び現れる問題が個人の失敗ではなく基盤の問題だからである。マージ待ちが実装時間を超えた窓も名指しする。速く作っても短くならない待ちだからである。加えて、Feedback Issue のクローズを改善が着地した合図として、その後に同じ分類が戻ってきたかを報告する。報告するのは再発であって判定ではない。保持 / 簡素化 / 撤回はレトロで決める。|`make closed-loop-weekly FROM=<date> TO=<date>` / 週次ワークフロー|

### 初期設定（`setup/` / `repo-setup/`）

ボイラープレートから新規プロジェクトを作成する際の設定スクリプトです。

|スクリプト|説明|
|---|---|
|`replace-module/`|Go モジュール名を全 `.go`、`go.mod` 等で置換 <!-- setup-localize:line -->|
|`replace-app-metadata/`|env ファイルと OpenAPI 仕様のアプリ名・説明を置換 <!-- setup-localize:line -->|
|`replace-license-copyright/`|LICENSE の著作権者名と年を置換 <!-- setup-localize:line -->|
|`replace-repository-reference/`|README と OpenAPI の GitHub リポジトリ参照を置換 <!-- setup-localize:line -->|
|`replace-codeowners/`|`.github/CODEOWNERS` の全ルールの所有者を置換。コメント行は記載例を保つため対象外で、所有者欄を判定できないルール行は書き換えずに報告する。 <!-- setup-localize:line -->|
|`remove-sample-api/`|サンプルAPI(`user`/`prefecture`/`product`/`order`)を削除。`sample-manifest.ts` に宣言したパスを削除し、共有 DI モジュールと `openapi.yaml` の `sample-api` マーカーブロックを除去する。再生成・整形・Lint まで行うには `make setup-remove-sample-api` 経由で実行する。 <!-- sample-api:line -->|
|`repo-setup/`|boilerplate を自分のリポジトリとして初期化する際の git / gh 側の手順。`preflight` は `v0.0.0` タグがあれば中止し、`bootstrap` はタグを作り直して develop / staging / production を用意しデフォルトブランチを移し、`prune-release-notes` は `v0.0.0.md` 以外のリリースノートを削除する。ラベル・ルールセット・ワークフローの有効化は全体の連鎖を持つ `setup-repository.mk` に残る。ここも Go なのは、タグの一括削除やデフォルトブランチの移動が、実物のリポジトリを壊さずには試せないためである。|
|`remove-doc-language/`|ドキュメント / スキルの対訳ペアを 1 言語へ畳む（`--lang en\|ja\|both`）。`en` は `*.ja.md` を全削除し、`ja` は対訳を正本の名前へ改名して正本のフロントマターを移植する（`SKILL.md` が読み込み可能なまま保たれる）。`both` は対訳を残してマーカーだけを解決する —— 選び終えたツリーに宣言が残ると、まだ選べると読み違えるためである。対訳規約を説明している散文は機械的に畳まず、`doc-pair` マーカーか `language-manifest.ts` の宣言が持つ。宣言の無い行に当たると 1 バイトも書かずに中止する。 <!-- lang-choice:line -->|

すべての setup スクリプトはプレビュー用の `--dry-run` をサポートしています。
<!-- sample-api:begin -->

削除対象は [`sample-manifest.ts`](setup/remove-sample-api/sample-manifest.ts) に、マーカー除去の規則は [`sample-api.ts`](setup/remove-sample-api/sample-api.ts) に宣言されています。サンプルは複数のフルスタックドメインからなり、拡張時は該当ドメインブロックにパスを追記し、混在行を `sample-api:begin … sample-api:end`（または `sample-api:line`）で囲むだけで対象に含まれます。

#### マーカーを置ける場所

マーカーはコメントの中でしか効かず、どのコメント形が使えるかは拡張子ではなく**その行が置かれている
文脈**で決まります。取り違えると失敗は 2 方向に出ます。認識されずに撤去後まで行が残るか、認識される
代わりにマーカー自身がサンプル在中の文書を壊すかです。どちらも lint か撤去後のビルドまで黙っており、
後者を見るのは `sample-removal-check` だけです。

以下の `<marker>` はマーカー名と接尾辞（`sample-api` ＋ `:begin` / `:end` / `:line` / `:replace-*`）を
指します。コメント接頭辞まで書くと、この頁自身がマーカーになるためです。

|文脈|効く形|他が使えない理由|
|---|---|---|
|Markdown 散文|HTML コメント内の `<marker>`。`:begin` と `:end` は独立行|ここではこれ以外がコメントにならない|
|Markdown の表の行|`:line` 形を**最後のセルの中**（閉じ `\|` の手前）へ|独立行のコメントは表を切る。空白はその表の記法に合わせる。詰めた表（`\|a\|b\|`）は前後に空白を置かず、緩い表（`\| a \| b \|`）は置く。違えると MD060|
|Markdown の箇条書き|マーカー行をその項目の継続インデントに揃える|列 0 のコメントは箇条書きを分断する（MD032 / MD007）|
|コードフェンス|フェンスの言語自身のコメント（Go なら `//`）|フェンス内の HTML コメントは本文として描画される|
|mermaid フェンス|`%%`、かつ**ブロック形のみ**|mermaid のコメントは独立行なので行末形が無い。ノードだけでなく、それを名指す辺と `class` 行にも付ける|
|YAML のブロックスカラー|中には置けない。キーごと外側から `replace` で囲む|YAML には `#` コメントがあるが、ブロックスカラー（`>-` / `\|`）の中では `#` 行が文字列の一部になる。マーカーが値の本文そのものになり、撤去後もスカラーに残る|
|Go の宣言|`:begin` を **doc コメントより前**へ|外に残った doc コメントはファイル末尾へ孤児として残り、`gofmt` でも消えない|
|Go の import|import 行に `:line` 形|マーカー領域の中でしか使われない import は、領域が消えた時点で `typecheck` に落ちる|

`replace` の退避行（`=` で始まる行）については 2 点:

- 結果のインデントは、退避行自身の先頭空白と本文が持つ空白の**合計**になります。インデントは二重に
  書かないこと。
- HTML コメントの退避行の中に HTML コメントを入れないこと。最初の `-->` が外側を閉じ、残りが本文として
  描画されます。そうしたコメントは `replace` ブロックの外へ置きます——撤去後も残る必要がある指針
  コメントは、そもそもそこが置き場です。

マーカーを持てない構造そのもの（`sql` / `text` フェンス、変更が組み替える表など）は、中の 1 行ではなく
構造ごと外側から `replace` で囲んで差し替えます。

マーカー行の増減は `scripts/marker-baseline/baseline.json` が固定している件数を動かすので、同じ変更の中で
再生成（`tsx scripts/marker-baseline --write`）してください。マーカーの形を指示ではなく**データ**として
持つファイルは、代わりに `MARKER_LITERAL_FILES` へ宣言します——ただしその宣言は走査から丸ごと外すので、
本物のマーカーを 1 つも持たないファイルに限ります。
<!-- sample-api:end -->

## テスト戦略

このツール群は層ではないため、それを統べる層 README は存在しません。
[`docs/testing-conventions.md`](../docs/testing-conventions.md) の 11 節に従い、観点はここが持ちます。
横断的な構造規則（`t.Parallel()`・サブテストのグループ・アサーション）は引き続きその文書が持ち、
ここが持つのは以下の観点だけです。観点は Go のツールにも TypeScript のツールにも等しく効きます。
違うのは走らせ方（`make test-scripts` か `make scripts-test` か）であって観点ではありません。

- **判定を検査し、その外側の入口は検査しない。** どのツールも、ファイルを読み・出力し・終了コードを
  返すだけの入口と、その隣に並ぶ判定モジュールへ分かれる。Go では入口が `main` と `run` で、`run` は
  不純な依存——作業ディレクトリ・HTTP クライアント・現在時刻・カバレッジ総量・コマンド実行器——を
  引数で受け、振り分けそのものをテストから触れるようにする。TypeScript では入口は `index.ts` で、
  分岐を一切持たない。何を持ってはいけないかとその理由は
  [`lib/untested-modules.ts`](lib/untested-modules.ts) が宣言する。そこに載ったモジュールは
  「判定を持たない」と主張していることになるので、その主張は真であり続けなければならない。
  規則（並び順・ドライランの同値性・安全策）を持っていたと分かったモジュールは、免除を保つのではなく
  宣言から外す。
- **違反だけでなく退化した入力も固定する。** これらの多くはゲートであり、ゲートは*何も検査せずに
  「違反なし」を報告する*向きに倒れる。したがって、解釈できない glob パターン・読めない対象ファイル・
  代入として読めない lockfile 行・空の走査には、それぞれエラーを主張するケースを置く。0 件へ黙って
  縮退させることはしない。「違反が無かった」と「何も見ていない」を区別し続けるための観点である。
- **メッセージではなく sentinel を主張する。** 失敗の種類はすべて package レベルの sentinel であり、
  テストは `require.ErrorIs` でそれを特定する。呼び出し側が行動に使う情報——どのファイルがずれたか、
  どのキーで失敗したか——を運ぶ場合に限り部分一致を重ねるが、それを唯一の検査にはしない。
  そうすると文言を書き換えただけで、別のエラーに対する合格テストへ静かに化ける。
- **窓は必ず両側を与える。** しきい値と比較するツール——日数の cooldown 窓、カバレッジの下限——では、
  境界値ちょうどと、そこから 1 段外れた値を対で置く。余裕を持って内側にあるケース 1 本では
  `>=` と `>` を区別できない。
- **外部は自分の境界でスタブする。** `docker` と `npm` は `PATH` の先頭へ置いたシェルスクリプトで
  差し替え、ツールが組み立てる引数列そのものを検査対象にする。GitHub API と各レジストリは
  `httptest` サーバへ差し替える。`t.Setenv` を使う `PATH` 系のケースは `t.Parallel()` と両立しないため、
  回避せずケース単位で宣言する。`actions-shellcheck` だけは例外で、実物の `shellcheck` を呼び、
  無ければ skip する。その skip を実行と取り違えないために `REQUIRE_SHELLCHECK` がある。
- **不可逆な手順は計画として検証し、実行しない。** `release` と `repo-setup` はタグを push し、
  GitHub Release を作り、デフォルトブランチを移す。そのため各手順は `runner` の継ぎ目を通し、
  テストは組み立てたコマンド列と中止条件を主張する。実際に走らせて確かめることは、
  本当にリリースすることを意味する。
- **失敗した実行が何も書いていないことを示す。** ファイルを書き換えるツールは、書き込む前にすべてを
  決める。途中で中止したときに作業ツリーが元のままであることが要件なので、アサーションはエラーだけで
  なく中止後のファイル内容に対して置く。
- **出力文言が唯一の成果物である場合は、それを契約として扱う。** drift の一覧・`::warning::` の
  アノテーション・検疫のノートは、人間や CI のアノテーションがまさにそれを読む。だからこれらのケースは
  標準ロガーを捕まえ、何が出力されたかを主張する。
- **フィクスチャは `t.TempDir()` 配下に置き、実ツリーには置かない。** リポジトリ自身のワークフローや
  lockfile を読むテストは、その日の内容次第で合否が変わり、ツールについてのテストではなくなる。

## 注意点

- Go ツールのユニットテストを実行するのは `make test-scripts` / `make test-scripts-cached` だけ。
  `make test` は `scripts/` を除外する。配線の詳細は
  [`.makefiles/README.md`](../.makefiles/README.md) を参照
- `actions-shellcheck` のテストは実物の `shellcheck` を呼び、無い環境では自分で skip する。
  `REQUIRE_SHELLCHECK` を立てるとその skip を失敗に変える。skip は既定の出力に現れないため、
  そのままでは「緑だが報告より少なくしか検査していない」状態が残る
- TypeScript のスクリプトは `tsx` で実行し `tsc` で型検査する。依存はここが宣言し
  （`package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml`）、`node_tool_runner` のイメージ
  ビルドと、必要とする CI ジョブが `scripts/node_modules` へ展開する
- 入口はいずれも `node_modules/.bin` ではなく `pnpm --dir scripts <script>` で起動する。
  `pnpm-workspace.yaml` の `verifyDepsBeforeRun` は pnpm を通った実行しか検査しないため。
  `tsx` の入口は先にリポジトリルートへ戻る。スクリプトはそこを基点に相対パスを解決する
- 判定ロジックは CLI の入口から切り離して `scripts/lib/**` と `scripts/portal/*.ts` に置く。
  これらのうち 5 本はゲートであり、検査をやめたときエラーではなく「違反なし」を報告する向きに
  倒れる。`make scripts-test` / `make scripts-typecheck` が、その沈黙を合格と取り違えないための
  歯止めで、CI では `scripts-check.yaml` が回す
- setup スクリプトはサンプル API（およびこのツール群の一部）を撤去した後に走るため、残りのツリーが
  在ることを前提にできない。`remove-sample-api.ts` は自身の manifest とマーカー除去ロジックごと消え、
  `verify-sample-removal.ts` は検証を通した後に自身と判定モジュール・そのテストを消す
- setup スクリプトは一度だけ使用 — ボイラープレートから新規プロジェクト作成時に実行
- AI エージェントは明示的な指示がない限りこのディレクトリを変更しないこと
