# リポジトリ複製後の作業リスト

Makeコマンド詳細は [Makeターゲット一覧](../../.makefiles/README.ja.md) を参照してください。

## Phase 1: mise のインストールとシェル activate

このプロジェクトは [mise](https://mise.jdx.dev) をツール / ランタイムバージョンマネージャとして必須利用します。[公式インストール手順](https://mise.jdx.dev/getting-started.html) で mise をインストールしたあと、**shell init に mise activate を仕込むことが必須です**（任意ではありません）。Make ターゲットは `golangci-lint` / `lefthook` 等を mise の shim 経由で解決しており、activate しない限り shim が `PATH` に載らないためです:

```sh
# zsh
echo 'eval "$(mise activate zsh)"' >> ~/.zshrc

# bash
echo 'eval "$(mise activate bash)"' >> ~/.bashrc

# shell を再起動 (or 新しいターミナルを開く)
exec $SHELL
```

確認:

```sh
mise --version
which mise
```

## Phase 2: Go ランタイムとプロジェクトツールのインストール

全ツール（golangci-lint / sqlc / oapi-codegen / mockgen / dlv / lefthook / ...）のバージョンは [`mise.toml`](../../mise.toml) を SSOT として管理しています。Dockerfile・ローカルインストーラ (`.makefiles/go/installer.mk`)・CI ワークフローはすべて同じ `mise.toml` を参照し、各環境で必要なものだけを `mise install <tool>` で個別取得します。

```sh
make go-update       # Go ランタイムを mise.toml 記載の pin でインストール
make install-tools   # gopls / gotests / impl / dlv / lefthook / golangci-lint / zizmor をインストール
make activate-tools  # `lefthook install` で git hooks を有効化
```

## Phase 3: エージェント設定のインストール（標準経路）

AI 支援レイヤは設定として同梱しています。project スコープの公式プラグイン、本リポジトリ自身のスキル（[`.claude/`](../../.claude/README.md) / [`.codex/`](../../.codex/README.md)）、および**標準装備**として同梱する外部スキル 1 つ（`graphify`。リポジトリを問い合わせ可能な知識グラフにするツール。コマンドと使い方の規律は [AGENTS.md](../../AGENTS.md) にあります）です。clone に付いてこない部分は、冪等な bootstrap 2 本で入れます。

```sh
bash .claude/scripts/bootstrap-plugins.sh          # 公式プラグイン（project スコープ）
bash .claude/scripts/bootstrap-external-skills.sh  # 外部スキル（user スコープ: Claude Code + Codex）
```

`graphify` はツールチェーンの他とは別に pin しています。版は [`python/graphify.in`](../../python/graphify.in) が宣言し `python/graphify.txt` が sha256 でロックします —— **`mise.toml` ではありません**。bootstrap はその lockfile から `${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify` 配下のマシンローカル venv へ入れ、そのバイナリを使って各アシスタントの設定ディレクトリへ skill を書きます。同じ lockfile が `python_tool_runner` イメージにも入り、`make graphify-*` はそちらを走らせます。どのコマンドがローカル完結で、どれが LLM API へ出るかは [AGENTS.md](../../AGENTS.md) に、このリポジトリで実際に元が取れるもの・取れないものは [`.claude/README.md`](../../.claude/README.md) に記載しています。

**この Phase は追加オプションではなく標準経路です。** 本テンプレートは AI 支援開発で変更が行われることを前提としており、`.claude/` / `.codex/` のスキルは [docs/development-flow.md](../development-flow.md) のフローの実行形態そのものです。根拠: [ADR-0007 (agents-md-operational-contract)](../adr/0007-agents-md-operational-contract.ja.md)。

**それでもレイヤを採らない判断は導入側のアーキテクトが行います。** 上記はビルド・テスト・リリースのいずれにも必須ではありません。レイヤ規約の正本は [docs/rules.md](../rules.md) であってアシスタント設定ではなく、アプリケーションが AI サービスへ依存することもありません。失うのは開発者体験です。フローは文書として残りますが、それを駆動するツールは手動の形では保守されないため、レイヤを採らないプロジェクトは第二の正規経路ではなく互換経路に乗ることになります。

採らないプロジェクトは、中途半端に設定を残さず意図的に外してください。

- bootstrap 2 本を実行しない（以降のどの Phase もこれらに依存しません）
- 保持しないものを削除する: `.claude/`、`.codex/`、`python/graphify.in` と `python/graphify.txt`、`docker/tools/Dockerfile` の graphify レイヤ、`.graphifyignore`、`.gitignore` / `.markdownlint-cli2.yaml` / `scripts/mermaid-lint/index.ts` の `graphify-out/` 記述。この一覧は網羅ではありません —— 網羅なのは checkout 全体への `grep -ril graphify` で、`.makefiles/graphify/`・`scripts/graphify-*`・`.github/workflows/graphify-*.yaml`・`.agents/graphify/` にも及びます

後から外すコストは今外すコストと同じなので、まず標準構成で入れて後から判断する順序でも安全です。

## Phase 4: ローカル起動確認

ローカルで起動してみて、問題なく動作することを確認してください。

```sh
make serve
make tools
make db-init
```

<!-- boilerplate-only:begin -->
## Phase 5: ローカライゼーションスクリプトの実行

<!-- lang-choice:begin -->
### ドキュメントの言語を選ぶ

このリポジトリは全ドキュメントを 2 部持っています。英語の正本と、その隣に置いた日本語訳
（`<name>.ja.md`）です。スキルも同じで（`SKILL.md` / `SKILL.ja.md`）、合わせて 422 組あります。
片方の言語しか読まないプロジェクトにとって、もう片方は誰のためでもない保守対象です。

これは下の置換より**先**に、そして以降のどの撤去よりも先に実行してください。撤去ツールは正本と
対訳の両方に宣言を持っており、その対の宣言は畳みが解決した時点で各ツール自身のマーカーが刈ります
—— 畳んだ後のツリーこそが後続の期待する形です。逆は成り立ちません。Phase 12 はこのツールが
文字列を宣言している workflow を削除するため、その後に畳もうとすると中止します。

作業ツリーはクリーンである必要があります。ここではまだクリーンで、下の置換を走らせると汚れます。

```bash
# プレビュー: 書き込みもコミットもしません。作業ツリーが汚れていても動きます。
DRY_RUN=1 make setup-remove-doc-language LANG_CHOICE=en

# 実行。作業ツリーはクリーンである必要があります（撤去を 1 コミットに畳むため）。
make setup-remove-doc-language LANG_CHOICE=en
```

| `LANG_CHOICE` | 起きること |
| --- | --- |
| `en` | `*.ja.md` を全て削除し、残る英語文書が対訳へ張っていた参照を落とす |
| `ja` | 英語正本を全て削除し、`<name>.ja.md` を `<name>.md` へ**改名**する。`SKILL.md` / `README.md` というファイル名の契約が保たれる。スキルの YAML フロントマターは正本から訳文へ移植され、これがスキルを読み込み可能に保つ |
| `both` | 対訳を残す。畳む位置を示すマーカーを解決し、ツール自身も撤去する —— 先送りではなく決定であり、開き直したければこのコミットを revert する |

`en` / `ja` を選ぶと `*.ja.md` は 1 つも残らないため、対訳の存在を要求する検査も同時に撤去されます
—— `doc-ref-lint` のペア検査、`skill-lint` の `SKILL.ja.md` 必須、ポータルの日本語エントリ、
ビューアーの言語切替。`canonicalize-doc` スキルも一緒に消えます。対訳の組を作って同期するための
スキルであり、言語が 1 つになれば語る主語が無くなるためです。

`ja` は高くつく選択なので、一度立ち止まる価値があります。英語正本が在るのはエージェントが
それを読むからで、日本語はおよそ 1.5〜2.0 倍のトークンを消費し、誤読のリスクも高くなります
（[ADR-0011](../adr/0011-docs-as-canonical-source.md)）。`ja` を選ぶことは、そのプロジェクトの
今後すべてのエージェント実行についてその代償を引き受けることです。チームが日本語を読み、
ドキュメントがまず人間のためにあるなら、それは妥当な選択です。

このスクリプトは推測せず、止まります。対訳規約そのものを説明している散文は機械的に畳めないため、
`doc-pair` マーカーにも `language-manifest.ts` の宣言にも載っていない箇所があると、その
ファイルと行番号を示して中止します。そのとき 1 ファイルも書き換えていません。

**先送りせず、ここで決めてください。** この先 —— 残りのセットアップと、その後のふつうの開発 ——
は 1 日ごとに両方の言語の代償を払います。ドキュメントの修正は 2 回書くことになり、エージェントの
実行は必要のない対訳を読み飛ばし、片方だけを直した瞬間に両者はずれます。代償は繰り返し発生
しますが、判断は 1 回きり、コマンド 1 本です。
<!-- lang-choice:end -->

### モジュール名とプロジェクトのメタデータを置換する

下記コマンドで、Goモジュール名を一括置換するスクリプトを実行してください。

ORG・REPO・CODE_OWNERS は適宜置き換えてください。派生設定は気になる箇所のみ変更してください。

```sh
export ORG=<your-org/git-user-name>
export REPO=<your-repo>

# CODEOWNERS の所有者。ユーザー(@name)かチーム(@org/team)を指定します。
# 組織そのものは所有者になれないため、組織が持つリポジトリではチームを指定してください。
export CODE_OWNERS=<@your-org/tech-leads>

export MODULE=${REPO}
export APP_NAME=${REPO}
export OPENAPI_TITLE=${REPO}
export COPILOT_TITLE=${REPO}
export COPYRIGHT_HOLDER=${ORG}
export COPYRIGHT_YEAR=$(date +%Y)

make setup-replace-module OLD_MODULE=go-boilerplate NEW_MODULE=$MODULE
make setup-replace-repository-reference REPOSITORY=$ORG/$REPO
make setup-replace-app-metadata APP_NAME=$APP_NAME OPENAPI_TITLE="$OPENAPI_TITLE" COPILOT_TITLE="$COPILOT_TITLE"
make setup-replace-license-copyright COPYRIGHT_HOLDER="$COPYRIGHT_HOLDER" COPYRIGHT_YEAR=$COPYRIGHT_YEAR
make setup-replace-codeowners OWNERS="$CODE_OWNERS"
make gen-api
make gen-sqlc
make tidy-lib

# 置換が当たったことを検証し、通ったら初期化ツールを撤去する。
# 最後に実行する。上のスクリプトは一度きりのもので、ここが通るまでは何度でも当て直せる。
make setup-verify
```

`setup-verify` は、`replace-module` が対象と宣言した全ファイルにボイラープレート名が残っていないこと、
および LICENSE・CODEOWNERS・README・OpenAPI が指定した値になっていることを確認する。通った場合にだけ
`scripts/setup/replace-*` と自身を削除する。初期化済みのリポジトリへ当て直すのは誤りで、
`replace-codeowners` は**全ルール**の所有者を単一の値へ書き換えてしまうため。`scripts/setup/lib` は、
一度きりのツール 2 つ（これとサンプル削除）のうち後に走った方が道連れにする。

## Phase 6: ローカライゼーションの検証

テストと静的解析、コード生成、ヘルスチェックなど、基本的な機能が問題なく動作することを確認してください。

```sh
make go-test
make go-lint
make gen
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## Phase 7: 手動書き換え

1. [README.md](../../README.md), [README.ja.md](../../README.ja.md) の内容をプロジェクトに合わせて書き換え、メンテナ方針節にあるこのリポジトリ固有のブランチ規則の例外は置き換えるか削除してください。
2. ドキュメントを 1 言語に絞るなら、対を畳んでも構いません（例えば [README.md](../../README.md) を [README.ja.md](../../README.ja.md) の内容で置き換える）。
    - [gen-docs-json.ts](../../scripts/portal/gen-docs-json.ts) と、それが生成元にする [manifest.yaml](../portal/manifest.yaml) はどちらも README.md を参照しているため、完全に置換する場合はこれらのスクリプトも書き換える必要があります。
    - portal の UI も En / Jp の切り替えを持つので、同じ手当てが要ります。
3. [openapi.yaml](../../openapi/openapi.yaml) の内容をプロジェクトに合わせて書き換えてください。
    - Infoセクション全体をプロジェクトに合わせて書き換えてください。
        - title
        - termsOfService
        - contact
        - version
        - description
        - license

<!-- boilerplate-only:end -->
## Phase 8: envファイルの書き換え

[env/](../../env) ディレクトリ内のファイルをプロジェクトに合わせて書き換えてください。

設定値の意味については、[env/README.ja.md](../../env/README.ja.md) を参照してください。

### clamp される設定値を確認する

いくつかのサブシステムは、範囲外の値を起動失敗にせず安全な既定値へ **clamp** します。設定を誤ったプロセスでも走り続けられるようにする回復性のための選択です。clamp が働くのは `0` / 負値 / 不正値を明示的に設定したときだけで（同梱の env 変数は妥当な `envDefault` を持ちます）、それでも補正は実行時に黙って適用されるため、対応する `WORKER_*` / `OUTBOX_*` / `SECURE_COOKIE_*` を調整する際は以下を確認してください。

- **ワーカーエンジン**（`WORKER_*`）— `Settings.normalize()`。[internal/controller/worker/README.ja.md](../../internal/controller/worker/README.ja.md)（設定値の clamp）を参照。
- **Outbox relay**（`OUTBOX_*`）— `provideRelaySettings`。[internal/controller/outbox/README.ja.md](../../internal/controller/outbox/README.ja.md)（Settings → clamp）を参照。
- **セキュアクッキー**（`SECURE_COOKIE_SAME_SITE`）— `normalizeSameSite` が `Lax` / `Strict` / `None` 以外の値を「上書きしない」へ clamp します。[internal/controller/httpstack/cookie/README.ja.md](../../internal/controller/httpstack/cookie/README.ja.md) を参照。

## Phase 9: リポジトリの初期化

ここまでの手順を完了したら、ファーストプッシュ後にリポジトリの初期化を行います。

`setup-repo` は workflow を有効化してから ruleset を適用します。既存リポジトリへ required status
check を適用する前に、宣言した各 context が PR で正常に報告されたことを確認してください。未報告の
context を GitHub が待ち続けるとマージが止まります。

### required status check を入れる前に auto-PR 用の App を登録する

自分で pull request を開く workflow が 3 本ある（[`auto-generate-docs.yaml`](../../.github/workflows/auto-generate-docs.yaml) /
[`graphify-sync.yaml`](../../.github/workflows/graphify-sync.yaml) /
[`graphify-extract.yaml`](../../.github/workflows/graphify-extract.yaml)）。workflow が自分の
`GITHUB_TOKEN` で起こしたイベントに対して GitHub は run を作らない。workflow が自分自身を呼び続ける
のを止めるための仕組みで、無効化できない。そのトークンで開いた pull request は check を 1 件も報告
せず、報告されない required context は上の警告そのものになる。待つ対象が無いままマージが止まり、
実行し直しても解消しない。App のトークンは別の identity なので、それが起こすイベントはただの
イベントとして扱われる。

status check を required にしないなら、この手順ごと省略できる。入れるなら `make setup-repo` が
ruleset を適用する**前**に済ませる。そうしないと最初の auto-PR がそのまま最初の詰まりになる。

#### App を作る（人手）

自動化できない。REST に作成の口が無く、秘密鍵は生成時に一度しか表示されない。

<https://github.com/settings/apps/new> で作る。

| 項目 | 値 |
| --- | --- |
| GitHub App name | `<現在のリポジトリ名>-auto-pr-app`（**GitHub 全体で一意**） |
| Description | `<現在のリポジトリ名> の生成物同期 PR 用` |
| Homepage URL | `https://github.com/<owner>/<現在のリポジトリ名>` |
| Webhook | **Active のチェックを外す** |
| Repository permissions → Contents | **Read and write** |
| Repository permissions → Pull requests | **Read and write** |
| その他の permissions | No access のまま |
| Where can this GitHub App be installed? | **Only on this account** |

Contents write が `auto/*` ブランチの push、Pull requests write が pull request の作成にあたり、
仕事はこの 2 つで全部。workflow 側で installation token をこの 2 つへ絞っているので、これより広い
App を作っても余分な権限は渡らない。ありふれたリポジトリ名では既に取られていることがあるので、
その場合は owner 名などを足す。名前は後から変えられる。

作成後、続けて 3 つ。作成直後に着地するのが **General** ページなので、上から順に済ませられる。

1. **App ID を控える** — General ページの上部に数字で出ている
2. **General → Private keys → Generate a private key** → `.pem` がダウンロードされる
3. **Install App** → **Only select repositories** で**このリポジトリだけ**

#### secret を登録する

```bash
gh secret set AUTO_PR_APP_ID --body '<1 で控えた App ID>'
gh secret set AUTO_PR_APP_PRIVATE_KEY < ~/Downloads/<ダウンロードした>.pem
```

**登録が済んだら `.pem` を消すこと。** ブラウザが落とした実体は、何にも守られていないディレクトリに
置かれた生きた秘密鍵そのものである。

```bash
gh secret list   # AUTO_PR_APP_ID / AUTO_PR_APP_PRIVATE_KEY が並ぶ
```

App が無い間も壊れない。各 workflow は警告を出して `GITHUB_TOKEN` へフォールバックするので pull
request は開く。ただし check が付かないため、required がある限りマージできない。その状態で開いて
しまった pull request は、後から App を登録しても救済できない。イベントは head コミットに対して
起こるもので、それが起きなかったという事実は変わらないからである。close して workflow に開き直させる。

### GitHubテンプレートから始めた場合

```sh
git add -A
git commit -m "Initial commit: setup boilerplate for $REPO"
git push origin main
make setup-repo
make branch-minor
```

### Git Cloneから始めた場合

```sh
git remote set-url origin ${ORG}/${REPO}
git add -A
git commit -m "Initial commit: setup boilerplate for $REPO"
git push -u origin main
make setup-repo
make branch-minor
```

## Phase 10: デプロイ設定の作成

このboilerplateでは、各社・各個人のクラウド環境やオンプレ環境に合わせて柔軟にデプロイできるよう、特定のクラウドプロバイダやデプロイ方法に依存しない構成を採用しています。

そのため、デプロイ設定には具体的なデプロイ先が反映されていません。プロジェクトのデプロイ先に合わせて、必要な設定を追加してください。

デプロイCI/CD: [.github/workflows/deploy-app.yaml](../../.github/workflows/deploy-app.yaml) を完成させてください。

`Note: Please modify this section according to your environment` と書かれている箇所が、環境に合わせて変更が必要な箇所になります。

## Phase 11: 認証・認可の実装

この boilerplate は認証（authn）と認可（authz）の双方について **開発用スタブのみ** を同梱しており、それらは `local` / `ci` / `test` 環境に **限って** 配線されています。`development` / `staging` / `production` では DI プロバイダが **fail-closed** です。スタブの配線を拒否してエラーを返すため、本物のコンポーネントを実装・配線するまでアプリケーションは **意図的に起動しません**。

これは意図的な強制装置です。署名を検証しない認証器や許可オールの認可器が本番環境に出荷されることを決して起こさないためのものです。**`development` / `staging` / `production` 向けに両方を実装することは、プロジェクト開始時の必須タスクです。**

> [!IMPORTANT]
> `Authorizer` は `InfrastructureModule` の内部で提供されるため、**usecase を構築するすべてのプロセス** — HTTP サーバ **と** バックグラウンドの job / worker プロセス — が設定済みの `Authorizer` を必要とします。後述の認可の手順が完了するまで、`APP_ENV=development` / `staging` / `production` でいずれを起動しても Fx 構築時に `no authorizer configured for environment` で終了します（authn も同様に `no authenticator configured for environment`）。本物のコンポーネントを実装する前にこれが表示されるのは想定内であり、バグではありません。

### 認証（authn）

この boilerplate には認証の実装例として JWT を使用したサンプルコードが含まれています。プロジェクトの要件に合わせて認証を実装してください。

usecase の [Authenticator](../../internal/usecase/boundary/auth/authenticator.go) インターフェースを実装する形で認証機能を作成します。

- 参照: [internal/infrastructure/auth/README.ja.md](../../internal/infrastructure/auth/README.ja.md)
- スタブ実装例（local・署名なし）: [internal/infrastructure/auth/local/auth_local.go](../../internal/infrastructure/auth/local/auth_local.go)
- `stg` / `prd` 実装（JWT / OAuth2 / OIDC / Cognito / Auth0 など）を `internal/infrastructure/auth/{stg,prd}/` 配下に追加します。
- 環境ごとの配線は [認証の DI モジュール](../../internal/di/module/core/auth.go)（`provideAuthenticator`）を編集し、`default` の fail-closed 分岐を `case config.EnvDevelopment / EnvStaging / EnvProduction` に置き換えて本物の `Authenticator` を返します。

### 認可（authz）

この boilerplate は開発用スタブとして **許可オール（allow-all）** の認可器を同梱しています。自プロジェクト向けに本物の Policy Decision Point（PDP）を実装してください。

usecase の [Authorizer](../../internal/usecase/boundary/authz/authorizer.go) インターフェースを実装する形で認可機能を作成します。

- 参照: [internal/infrastructure/authz/README.ja.md](../../internal/infrastructure/authz/README.ja.md)
- スタブ実装例（許可オール）: [internal/infrastructure/authz/allowall/authz_allowall.go](../../internal/infrastructure/authz/allowall/authz_allowall.go)
- `stg` / `prd` 実装（claims からの RBAC / 所有者チェック / OPA・Cedar などの外部ポリシーエンジン）を `internal/infrastructure/authz/{stg,prd}/` 配下に追加します。
- 環境ごとの配線は [認可の DI モジュール](../../internal/di/module/authz.go)（`provideAuthorizer`）を編集し、`default` の fail-closed 分岐を `case config.EnvDevelopment / EnvStaging / EnvProduction` に置き換えて本物の `Authorizer` を返します。

`Authorize(ctx, *auth.Authn, Action, *Resource)` のシグネチャは既に完全な `Authn`（subject / scopes / claims）と対象 `Resource`（任意の `OwnerID` 付き）を運ぶため、RBAC と所有者（オブジェクトレベル）モデルの双方を呼び出し箇所を変えずに表現できます。

<!-- boilerplate-only:begin -->
## Phase 12: ボイラープレートである間だけ成り立つ記述を落とす

このリポジトリには、テンプレートからリポジトリを作成した瞬間に真でなくなる記述が 2 種類ある。自分を*ボイラープレートと
名乗っている*記述と、ボイラープレート*だからこそ*従っている規約——ADR のその場改訂 regime、
統合パス、`setup-review` の仕掛け——である。いずれもテンプレートの足場であって、あなたの
プロジェクトのドキュメントではない。

```sh
DRY_RUN=1 make setup-remove-boilerplate-identity
make setup-remove-boilerplate-identity
```

リポジトリを走査して `boilerplate-only` マーカーをすべて解決し、
[boilerplate-only-conventions.ja.md](boilerplate-only-conventions.ja.md) とその正本、および
`docs/plan/**`（上流がまだ着手していないリリース線の要件）を削除し、自身の make ターゲットの
登録も外したうえで、ツール自身を撤去する。対象ファイルの一覧ではなく走査にしているのは、
一覧というものはその外側にマーカーを書けてしまうからで、誰も剥がさないマーカーは、何も告げない
ままあなたのプロジェクトへ生き残る前提になる。

`docs/project/roadmap.md` は**削除しない**。導入部が作成先向けの文面へ差し替わるよう書かれており、
自分たちの方向を記録する場所として引き継げる。

**触らないもの**: リポジトリ名・モジュール名（Phase 5 で置換済み）と、運用中も読み返す部分
——後述の clamp 設定レビューと除外 ADR。これらは複数のパッケージ README から参照されている。

除去のあとに残るのは各規則の一般形で、それを所有する文書
（[docs/adr/README.ja.md](../adr/README.ja.md)、[docs/rules.md](../rules.md)、各層の README）に
書かれている。単純な削除ではなく作成先向けの言い換えが要る箇所は、差し替え文が隣に置いてあり、
同じパスで入れ替わる。

> `boilerplate-only` と `sample-api` のマーカーを 1 回のパスで剥がさないこと。発火する契機が
> 違い（この Phase と Phase 15 のサンプル削除）、作成先が片方だけを行うことは十分あり得る。

<!-- boilerplate-only:end -->
## Phase 13: テンプレートの意図的な除外（ADR）のレビュー

認証・認可（Phase 11）やデプロイ（Phase 10）以外にも、このテンプレートはいくつかの**意図的な非選択**をしています。例：アプリ内レート制限器を持たない / 汎用 Cache 抽象を持たない / scheduled job の並走制御はスケジューラに委譲 / push・streaming ブローカーは worker の対象外。

これらは [docs/adr/](../adr) 配下の **exclusion ADR** として記録され、`setup-review` タグが付いています。次で一覧できます：

```sh
grep -rl "setup-review" docs/adr/
```

プロジェクトごとに各 ADR をレビューし、次を判断してください：

- **そのまま採用** — その除外が自プロジェクトに合う場合は ADR をそのままにする。
- **変更** — 逆の方針が必要な場合。セットアップはテンプレートから自プロジェクトの**ベースラインを確立**する場なので、**ADR を直接編集**（Decision / Consequences を書き換え、`deciders` / `date` を更新）して自プロジェクトの選択を記録し、実装する。

不変（元 ADR は編集せず supersede する新 ADR を追加）モデルは、**運用開始後**に決定を見直すときに適用します。セットアップ時の一度きりの再ベースライン化には適用しません。

## Phase 14: 依存ライセンス方針の決定

依存ライセンススキャン（`make trivy-license` と [.github/workflows/trivy-fs.yaml](../../.github/workflows/trivy-fs.yaml) の `trivy-license` ジョブ）は**恒久的に報告専用**です。全依存のライセンスをジョブサマリと PR コメントへ列挙するだけで、ビルドを落とすことはありません。

これは未完成のゲートではなく、**意図的な非選択**です。どのライセンスを許容するかは、このテンプレートを採用する組織が持つ法務判断です。配布されるバイナリでは失格となる copyleft が、バイナリが自社インフラの外へ出ないサービスでは全く問題にならないこともあり、答えは会社・製品・配布形態ごとに変わります。ここで閾値を決めることは、ある一社の法務スタンスを、これを基に作られる全プロジェクトへ焼き込むことになるため、テンプレートは棚卸しだけを提供し、判断は採用側に委ねます。

自組織に禁止ライセンス方針がある（あるいは必要な）場合は、次のように自分でゲート化してください：

1. 許容する集合を Trivy 自身の分類（`notice` / `unencumbered` / `permissive` / `reciprocal` / `restricted` / `forbidden` / `unknown`）で決め、**出荷物とビルド専用ツールに同じ基準を当てるか**を判断する。同じでなくてよい：本リポジトリで `notice` / `unencumbered` 以外に分類される依存は、出荷されないビルド専用の `docker/tools/` 由来です。
2. Trivy の分類は出発点であり権威ではないものとして扱う。`BlueOak-1.0.0` は OSI 承認の permissive ライセンスでありながら `unknown` に落ちるため、この種のケースは分類任せにせず明示的に決める。
3. [.makefiles/security/trivy.mk](../../.makefiles/security/trivy.mk) の `trivy-license-ci` へ閾値を追加し、`trivy-license` ジョブへ失敗させるステップを足す。パッケージ単位の例外は [.trivyignore.yaml](../../.trivyignore.yaml) へ記録する。
4. [.github/workflows/README.md](../../.github/workflows/README.md) のトリガーマトリクスと [ADR-0093](../adr/0093-multi-layer-security-scanning.md) のライセンス行を更新する（いずれも現状「方針なし」と記載しています）。

## Phase 15: サンプルAPIの削除

このboilerplateには、サンプルAPIが含まれています。プロジェクトの要件に合わせて、サンプルAPIを削除してください。

AI駆動開発を活用する場合は、サンプルAPIを残しておくと、AIがコードの構造や実装例を理解しやすくなります。必要に応じて、サンプルAPIをリファクタリングして、プロジェクトの要件に近づけることもできます。

### 削除手順

自動コマンドを使用します。`scripts/setup/remove-sample-api/sample-manifest.ts`（削除がこのファイル自身を持って行くため、手順が成功した瞬間にリンクが切れる。だからリンクにせず名前だけ挙げる）に宣言されたサンプルAPI（`user` / `product` / `order`）を削除し、共有ファイル（DI 4 モジュール＋ `openapi.yaml`）の `sample-api` マーカーブロックを除去したうえで、再生成・整形・Lint まで実行します。

> 実行前に **DB コンテナが起動している必要があります** — 末尾の `gen-query` は `pg_dump` で**ライブ**スキーマをダンプするため、DB 停止状態では `connection refused` で失敗します。

```bash
# 0. DB コンテナを起動（gen-query がライブスキーマをダンプするため）
docker compose up -d database

# 削除内容のプレビュー（変更は行いません）
DRY_RUN=1 make setup-remove-sample-api

# サンプル削除（ファイル削除＋マーカー除去 → gen-api → gen-query → fix → lint）
make setup-remove-sample-api

# サンプル削除後のマイグレーション集合で DB を再構築しスキーマを再ダンプ
# （削除済みの users テーブルが models.gen.go に残らないようにする）
make db-init-local db-init-test
make gen-query
```

補足:

- core の `idempotency_keys`（マイグレーション `000001`）と `outbox`（`000002`）は**残します**。それ以外のマイグレーションは、`prefecture` を含めて所属するサンプルと一緒に消えます。
- `gen-query` は**ライブ** DB の `pg_dump` から Go モデルを再生成します。上記の DB 再構築を省くと、残存する `users` テーブルが再ダンプされ `models.gen.go` に古い `Users` 型が再生成されます。再構築＋再 `gen-query` が実際に型を消す手順です。
- 共有生成物（`*.gen.go` / `openapi.gen.yaml` など）は直接削除せず、再生成ステップで更新されます。公開ドキュメント（`docs/openapi/**` / `docs/godoc/**` / `docs/coverage/**` / `docs/portal/**`）とコードグラフ（`graphify-out/**`）はこのステップの対象外で、再生成（`make gen-all-docs`）するまでサンプルを記述したままです。
- サンプルは複数のフルスタックドメインからなり、それぞれが `sample-manifest.ts` の 1 ブロックとして宣言されています。拡張したら該当ブロックへ新しいパスを追記し、共有ファイル内に混在するサンプル行を `// sample-api:begin` … `// sample-api:end`（行末なら `// sample-api:line`、mermaid フェンス内は `%%`、Markdown 散文は `<!-- -->`）で囲んでください。同じコマンドで自動的に削除対象に含まれます。

### 規則から例が消えるので、自分の例を置き直す

`docs/rules.md`・`docs/adr/**`・各層 `README` のいくつかの規則は、一般的な形で述べたうえでサンプル由来の
具体例で説明しています。**規則は撤去後も残りますが、例は残りません。** 残るのは正しい記述と、それが自分の
システムでどう見えるかを示さなくなった状態です。

該当箇所には、消える行の直上に HTML コメントで**なぜそこに例が要るのか・その例が何を示さなければならないか・
どう書き直すか**が置いてあります。探して順に埋めてください。

```bash
grep -rn "撤去後にこの箇所へ自分の例を置くための指針" docs/ internal/ pkg/
```

これは見栄えの話ではありません。例の無い抽象的な規則は、人が規則を適用しなくなる直前の姿です。読者それぞれが
何が対象なのかを一人で判断することになり、判断は割れます。コメントは、その判断を元の意図が見えているうちに
一度だけ下せるように置いてあります。

業務語彙には専用の家があります。[`docs/spec/glossary.md`](../spec/glossary.md) の用語表も同じ撤去で空に
なりますが、埋め直すための規則はページに残ります。

<details>
<summary>手動手順（参考・現在は不要）</summary>

1. [openapi.yaml](../../openapi/openapi.yaml) のサンプルAPI定義の削除
    - `サンプルAPI用のパス` の下に書かれているPath定義を削除し、そのリンク先のyamlファイルも削除してください。
    - `サンプルAPI用のパラメーター定義` の下に書かれているParameter定義を削除し、そのリンク先のyamlファイルも削除してください。
    - `サンプルAPI用の型定義` の下に書かれているSchema定義を削除し、そのリンク先のyamlファイルも回帰的に削除してください。
2. サンプルAPIのControllerとUsecaseの削除
    1. `make gen-api` でコードを再生成して、サンプルAPIのControllerコードを削除してください。
    2. サンプルAPIが参照している、Usecaseファイルとそのテストファイルを削除してください。
        - mockファイルも削除してください。
    3. [internal/integration](../../internal/integration) でエラーを起こしているファイルがあれば、そのファイルも削除してください。
    4. サンプルAPIの生成コードがないことで影響を出しているハンドラファイルおよびテストファイルを削除してください。
    5. この時、Infra層で参照エラー(QueryServiceやCommandServiceのインターフェースエラー)が出る場合は、これらのインターフェースからサンプルAPIで使っているファイルとそのテストコードを削除してください。
3. サンプルAPIのInfraコードの削除
    1. `make db-test-migrate-down` と `make db-local-migrate-down` を実行して、DBをクリーンな状態にする。
    2. `dml` にある実行SQLを削除する。
        - [database/dml/repository](../../database/dml/repository) の配下のディレクトリを削除してください。
        - [database/dml/query_service](../../database/dml/query_service) の配下のディレクトリを削除してください。
        - [database/dml/command_service](../../database/dml/command_service) の配下のディレクトリを削除してください。
    3. `make gen-query` を実行して、SQLCのコードを再生成して、サンプル用のSQLCコードを削除する。
    4. サンプル用のInfraコードがエラーになるので、そのコードとそのテストコードを削除する。
4. サンプルAPIのドメインコードの削除。
    - [internal/domain/](../../internal/domain) の配下のサンプルAPIで使っているコードとそのテストコードを削除してください。このディレクトリの配下のディレクトリはサンプルAPIのドメインコードのみなので、配下のディレクトリごと削除しても構いません。

</details>

## Phase 16: 自分の ADR regime を決める

上流自身の ADR 規約は、このリポジトリがボイラープレートであることに乗っていた記述もろとも除去されました。残るのはあなたにしか下せない決定です。この決定は、上流がどう出荷したかではなく、あなたのプロジェクトが自分の歴史をどう記録するかについてのものだからです。

継承されるのは [docs/adr/README.md](../adr/README.md)（日本語は [README.ja.md](../adr/README.ja.md)）に書かれたとおりのもの——ADR は不変の記録であり、変わった決定は新しい `accepted` な ADR で置き換え、古いものは `superseded` とする——です。これは [MADR](https://adr.github.io/madr/) の定める ADR の形であり、[ADR-0000](../adr/0000-record-architecture-decisions.ja.md) が決めたことです。

その場改訂を望むなら——生きられるのではなく出荷される設計文書にとっては正当な選択です——それはあなた自身の決定として、あなた自身の ADR に記録してください。上流がそうしていたという事実から導かないでください。

## Phase 17: DAST のセットアップを残すかを決める

DAST のセットアップだけ済ませてあります。[`.github/workflows/zap-api-scan.yaml`](../../.github/workflows/zap-api-scan.yaml) は GitHub-hosted runner の中でこのアプリケーションを起動し、OpenAPI 定義から得たエンドポイント一覧をもとに、認証済みの [OWASP ZAP](https://www.zaproxy.org/) API スキャンを当てます。週次と手動で走り、結果は code scanning へ上がり、検出でビルドを落とすことはありません。動的スキャンが要るならそのまま使ってください。追加で配線するものはありません。

**設定値は、このリポジトリに同梱されていた API を記述したものであって、あなたの API のものではありません。** [`.github/zap/rules.tsv`](../../.github/zap/rules.tsv) のしきい値、スキャンが名乗るアイデンティティ、そのスキャンが到達する面は、いずれもその API が `dast` 環境プロファイルで何を返すかから導いた値です（`ci` と違い、スタブの bearer トークンを受理するのではなく mock 認証サーバーが署名した実 JWT を検証します）。あなたの API について何かを主張するものではありません。エンドポイントが違えば、受容してよい検出も違います。最初の週次実行の前にワークフロー冒頭のコメントを読み、実際の API に対して両方のファイルを導き直してください。引き継いだままの `IGNORE` は、二度と誰の目にも触れない検出になります。

**スキャンの到達先を指す値が 2 つあり、Phase 15 は両方をリセットします。** ワークフローは ZAP を起動する前に、保護されたオペレーション 1 つ（`PROBE_PATH`）を 1 つのアイデンティティ（`SCAN_SUBJECT`）で叩き、資格情報が受理されることを確かめます。資格情報が拒否されたスキャンは全ての保護オペレーションで 401 を集め、それでも「完了したスキャン」として報告されるからです。どちらの値もこのリポジトリに同梱されていた API を指しているため、その削除とともに撤去されます — `PROBE_PATH` は `/health` に、`SCAN_SUBJECT` は空になります。

`/health` は資格情報なしで応答します。保護されたオペレーションがまだ 1 つも無い状態でもワークフローを動かせる代わりに、**何も証明しません。スキャンが未認証面しか見ていなくても実行は緑のままです。** ステップは黙って通さず毎回 warning を出しますが、読まれない warning は無いのと同じです。**保護されたオペレーションができ次第 `PROBE_PATH` をそこへ、シードに実在するアイデンティティへ `SCAN_SUBJECT` を向けてください。** それまでの DAST は、動いてはいても、存在意義である検査をしていません。

要らなければ、[`.github/workflows/zap-api-scan.yaml`](../../.github/workflows/zap-api-scan.yaml) と [`.github/zap`](../../.github/zap) を削除し、[`.github/actions-pin.toml`](../../.github/actions-pin.toml) からスキャナ用 action のエントリを、[`.github/egress.toml`](../../.github/egress.toml) から `zap-api-scan.yaml:dast` のジョブセクションを落としてください。`make pin-actions-check` と `make egress-check` はどちらも「どの workflow からも参照されないエントリ」で落ちるため、片方だけ忘れて黙って通ることはありません。有効/無効を切り替えるスイッチはありませんし、今後も設けません。残すとは残すことであり、設定されたまま無効なスキャナは、誰も読まず誰も保守しないものになります。

`dast` の環境プロファイルはこの削除の対象ではありません。スキャナではなく実行文脈を指す名前なので、mock された provider に対して実 authenticator を通したい他の用途にも `env/.env.dast` と `EnvDast` の分岐はそのまま使えます。

## Phase 18: 資格情報を要するスキャナを残すかを決める

ここには、このリポジトリだけでは供給できないものを必要とするスキャナが 2 つあります。1 つはベンダーのサービス用トークンを要求する [`sonarqube.yaml`](../../.github/workflows/sonarqube.yaml)、もう 1 つは GitHub Advanced Security を要する [`code-ql.yaml`](../../.github/workflows/code-ql.yaml) で、後者は public リポジトリでは無料、private では課金されます。このリポジトリは public なのでどちらも無料です。あなたのリポジトリは public とは限らず、支払う意思があるとも限りません。

決めるまでの間、壊れるものはありません。どちらもスキャン前に必要なものが揃っているかを確認し、無ければ自分をスキップして実行を緑のまま残します — 資格情報の欠如はスキャン結果ではなくセットアップの未了だからです。同じ経路が fork からの pull request も覆います。fork にはリポジトリの secret が一切渡らないためです。

### 残す場合

リポジトリへ secret を登録し、ベンダー側に対応するプロジェクトを作成してください。

| Secret | ベンダー側のセットアップ |
| --- | --- |
| `SONAR_TOKEN` | [SonarQube Cloud](https://sonarcloud.io) で組織とプロジェクトを作成し、**Automatic Analysis を off** にする — on のままだとサーバー側がリポジトリを自分で解析し、CI の解析と衝突する |

`sonar-project.properties` は SonarQube Cloud 上のプロジェクトを identify するものなので、設定というよりボイラープレート由来のアイデンティティです。Phase 5 の `make setup-replace-repository-reference` が README や OpenAPI の参照と併せて書き換えます。その Phase を飛ばした場合は、最初のスキャンの前に `sonar.projectKey` と `sonar.organization` を手で書き換えてください。さもないと解析結果が雛形側のプロジェクトへ送られます。

### 撤去する場合

```bash
# プレビュー: 何も書かず、何もコミットしない。
DRY_RUN=1 make setup-remove-licensed-scanners

# 撤去。スクリプトは進行に応じてコミットするため、作業ツリーはクリーンである必要がある。
make setup-remove-licensed-scanners
```

スクリプトは両方を削除し、**製品ごとに別のコミットへ分けます**。すでに保有しているライセンスがあれば `git revert` 1 回で戻せます。

```bash
git revert <「CI: SonarQube Cloud のワークフローを撤去する」のコミット>
```

この取り消しについて 2 点あります。README の行は製品ごとではなく最後の 1 コミットでまとめて外れます。2 製品が同じ表の隣り合う行を占めており、製品ごとに文書を編集すると最後の 1 つを除く全ての revert が衝突するためです — つまり復活させたスキャナは動きますが、文書には載らないままになります。もう 1 点、スキャナ間で共有される lockfile のエントリは、その最後の利用者を消したコミットが削除するため、revert すると未登録の参照で `make pin-actions-check` が赤くなることがあります。`make pin-actions-resolve` で戻り、どのエントリかは check が名指しします。

ワークフローを消しても secret は消えません。`SONAR_TOKEN` はリポジトリ設定から自分で削除してください — 何にも使われていないトークンも、盗まれうるトークンであることに変わりはありません。

有効/無効を切り替えるスイッチはありません。理由は Phase 17 と同じで、設定されたまま無効なスキャナは誰も読まず誰も保守しないものになるからです。`bearer.yaml` は意図的にこの集合の外です。Elastic License 2.0 は CI で走らせる分には何のコストも生まず、制約するのはサービスとしての再配布だけであり、それは別の問いとして [workflows の README](../../.github/workflows/README.ja.md#bearer-のライセンスと撤去) に自分の答えを持っています。
