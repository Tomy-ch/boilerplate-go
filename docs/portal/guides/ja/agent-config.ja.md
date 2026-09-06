# `.claude/` — 本リポジトリのエージェント設定

このディレクトリは、**リポジトリに同梱される** Claude Code 設定を保持します。再利用可能な skill、
subagent、scaffolding が読むスペックテンプレート、補助スクリプト、project スコープの権限 / プラグイン
宣言です。リポジトリを clone して信頼したメンバーは、全員が同じエージェント挙動を継承します。

## `AGENTS.md` との関係

`AGENTS.md`（リポジトリ直下）は人手で管理する **運用契約** で、エージェントが何に触れてよいか・どう振る舞う
べきかを定めます。それが真の出所であり、本 README はそのルールを再掲しません。特に `AGENTS.md` は
`.claude/**` を、ユーザーが明示的に変更を要求するか実行中の skill がその実行の間だけスコープを緩和しない限り、
**AI 編集のスコープ外** として扱います。まず `AGENTS.md` を読んでください。

## 構成

| パス | 内容 |
| --- | --- |
| `settings.json` | project スコープの権限（`allow` / `ask` / `deny`）、有効化プラグイン、既知マーケットプレイス。リポジトリを信頼した全員と共有されます。生成物は `Edit` / `Write` を deny、`AGENTS.md` とその `CLAUDE.md` シンボリックリンクは確認を要します。 |
| `skills/<name>/` | 再利用可能な skill。英語 canonical の `SKILL.md`（+ 参考訳 `SKILL.ja.md`）と、任意の同梱 `scripts/` / `references/` を持ちます。`/<name>` で起動。 |
| `agents/` | skill が使う subagent 定義（例: integrator skill が並列でファンアウトする読み取り専用のレイヤー別ワーカー `arch-auditor-*` / `drift-detector-*`）。 |
| `scaffold-spec/` | スペック形式定義（`domain-spec.md`・`usecase-spec.md`・`verify-rules.md` など）。`scaffold-*` / `verify-spec` / `new-spec-*` skill が **実行時に** 読むため、形式変更が skill を編集せずに伝播します。 |
| `scripts/` | リポジトリレベルのエージェントツールスクリプト（プロジェクトのビルドツールではありません。それは直下の `scripts/` にあります）。 |

## 初回セットアップ: 公式プラグイン

本リポジトリは Anthropic 公式プラグインに依存し、`settings.json` に **project スコープ** で宣言しています
（`enabledPlugins` + `extraKnownMarketplaces`）:

- `skill-creator` — skill の作成/更新のためにローカルの `manage-skill` skill がラップします。
- `feature-dev` — ガイド付き機能開発ワークフロー（`/feature-dev`）。`code-explorer` / `code-architect` /
  `code-reviewer` エージェント付き。

信頼済みの clone には既に入っています。もし無ければ（または一覧に新規追加したら）、冪等な bootstrap を
実行します:

```bash
bash .claude/scripts/bootstrap-plugins.sh
```

新たに有効化したプラグインは **次の** Claude Code セッションで読み込まれます。

## 初回セットアップ: 推奨する外部スキル

**外部スキル**は marketplace プラグインではないサードパーティ製 skill で、プラグインの bootstrap では
入りません。本リポジトリが公式に推奨するのは 1 つです:

- `graphify`（`/graphify`）— tree-sitter でリポジトリを解析して `graphify-out/` に知識グラフを作り、
  grep の繰り返しではなくグラフに対して構造の問い合わせ（`query` / `affected` / `god-nodes`）を行います。

本リポジトリが対象とするアシスタント（Claude Code + Codex CLI）の両方へ、冪等な bootstrap で入れます:

```bash
bash .claude/scripts/bootstrap-external-skills.sh
```

上のプラグインと異なる性質が 3 つあり、実行前に知っておく価値があります:

- **レビュー対象のファイルではなく依存物。** skill はチェックアウト内の
  `.claude/skills/graphify/` と `.codex/skills/graphify/` へ着地し、gitignore されます。pin から
  作られ、マシンごとに作り直され、レビューはされない — `vendor/` や `node_modules/` が依存物自身の
  ファイルを持つのと同じ扱いです。信頼済みの clone には**入らず**、マシンごとに一度 bootstrap を
  実行します。pin は `python/graphify.in` で、スクリプトは自分で選ばずそれを読みます。
- **`skill-lint` は検査しません。** 本リポジトリの skill 規約（frontmatter、`SKILL.ja.md` の対、
  解決できる参照）は、このリポジトリが書く skill を前提にしています。これは upstream が書いたもので、
  実行後にしか存在しない成果物を参照するため、`EXTERNAL_SKILLS`（`scripts/skill-lint/checks.ts`）へ
  宣言して検査から外します。誰も満たせない規約を課しても、読者にゲートを無視することを教えるだけです。
- **`graphify uninstall` は届きません。** アンインストーラが見るのはホームディレクトリです。上記
  2 つのディレクトリは手で削除します。`--purge` を付けると `graphify-out/` も削除されます。

**bootstrap はインストーラにチェックアウト内を書かせません。** このツールが持つどのモードも、本
リポジトリが保護している対象へ書き込みます。`install --project` は `CLAUDE.md`（ここでは
`AGENTS.md` への symlink）へ `## graphify` 節を追記し、`.claude/settings.json` へ PreToolUse フックを
登録します。既定の user スコープは `~/.claude/CLAUDE.md` を書き換えます。名前の似た
`<名前> install`（`graphify claude install`、`graphify codex install`、`opencode` / `aider` /
`kilo` も同様）と `graphify hook install` は `AGENTS.md`・git hook・merge driver を書きます。そのため
bootstrap は捨て `HOME` に対してインストールを実行し、skill ディレクトリだけを取り出します。
これらのサブコマンドは加えて `settings.json` の `ask` に振ってあり、`graphify --help` を読んだ
エージェントが実行するのではなく、人の判断を仰ぐ形で表に出ます。

グラフは派生物なので gitignore してあり、手元で生成します。`update` と問い合わせ系のコマンドは
AST のみで API キーを要しません。docs / PDF / 画像の抽出、`--mode deep` の推論、`--wiki`、
コミュニティの**命名**は LLM API を呼び、内容がマシン外へ出るため opt-in に留めます。

```bash
make graphify-update
```

**`mise exec "pipx:graphifyy[sql]"` ではなく固定版を走らせること。** あの形は `python/graphify.txt` を
経由せず pipx が返したものを使うので、ロックからずれる。固定版が入るのは 2 箇所 —— `python_tool_runner`
イメージ（上の make target が使う先）と、bootstrap の venv
`${XDG_CACHE_HOME:-$HOME/.cache}/go-boilerplate/graphify/bin/graphify`（`graph-affected.ts` が呼ぶ先）。
コマンドと残りの規律は `AGENTS.md` が持つ。

グラフから除外する対象（追跡下の生成物、日本語ミラー、ベンダリング）は `.graphifyignore` で
宣言します。変更した場合は全再抽出が必要です — 差分 `update` は fail-closed で、除外済みの
ノードを保持します。

### このリポジトリで効くもの・効かないもの

上流の主張ではなく、このリポジトリで実測した結果です。

- **`affected` が採算の取れるコマンド。** シンボルからの逆引きで、変更したら壊れる呼び出し元が
  relation ラベルと `file:line` 付きで出ます。影響範囲が読めない変更の計画時に使ってください。
  引数はシンボル名ではなくノード **id** なので、名前を解決し曖昧なときは候補を出すラッパー経由で
  叩きます。

  ```bash
  node .claude/scripts/graph-affected.ts NormalizeError --depth 2
  ```

- **`query` は `--budget` を上げるか、truncate 警告を読むこと。** 既定（約 2000 トークン）は
  この規模のリポジトリでは結果を切り捨て、その旨を出力します。答えが切り捨てた側にある場合があります。
- **`god-nodes` はここでは無視してよい。** エッジ数で順位づけするため、本リポジトリが機械強制する
  1:1 テスト規約の結果としてテスト足場（`Any()` / `NewTestFromSalt()` / `NewNoopTracerFactory()`）が
  production コードより上位に来ます。「エッジが最も多いもの」の答えであって、このリポジトリでは
  「中心にあるもの」の答えになりません。
- **グラフの鮮度は最後の `update` 時点。** 未コミットの作業についての問いなら、先に再構築するか
  `grep` を使ってください。小さな差分なら `grep` の方が安いです。

## 規約

- **英語が canonical。** skill / README 本文は命令形の英語で書き、対になる `*.ja.md` は `canonicalize-doc`
  skill で同期する人間向け参考訳です。ユーザーへの実行時出力は引き続き `CLAUDE.md` に従います（日本語）。
- **skill の著述。** skill の作成・更新には `/manage-skill` を使います。`skill-creator` をラップし、本
  リポジトリの配置・frontmatter・翻訳ペア・eval 成果物の規約を適用します。
- **既存物の探索。** 手書きの一覧を本 README で保守する代わりに、`/tool-map` を実行すると skill / agent /
  command の全インベントリと依存マップが得られます。
- **定義は実態と突き合わせて lint されます。** `make md-skill-lint`（`make md-lint` に含まれるため
  `pre-commit` フックで走ります）が、frontmatter・対訳ペアの見出し構造・本ディレクトリの本文が参照する
  `make` ターゲットとパスの実在性を検査します。skill は指示書であり、腐った参照はエージェントに誤った手順を
  実行させます。検査範囲と `<!-- skill-lint-ignore -->` ディレクティブは
  [`scripts/README.md`](../scripts/README.md) に記載しています。
