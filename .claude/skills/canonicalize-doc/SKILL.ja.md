> **このファイルは `SKILL.md` の日本語訳です。**
> 直接編集しないでください。内容の変更が必要な場合は canonical な `SKILL.md`（英語版）を更新し、その後この日本語訳を同期してください。
> Claude Code のスキルとしては `SKILL.md` のみが読み込まれます。このファイルはスキル本体ではなく、レビューや学習用の翻訳ドキュメントです。

# Canonicalize Doc

このスキルは、指定された Markdown ドキュメント（README、SKILL など）について「英語 canonical + 日本語翻訳」のペアを生成または同期する。このリポジトリの規約に従って動作する。

## 利用シーン

以下のいずれかに該当する場合に使用する。

- 日本語版のみ存在する Markdown について、英語 canonical を作成したい（このリポジトリでは英語が canonical のため、典型的なケース）。
- 英語版のみ存在する Markdown について、日本語翻訳を作成したい。
- 両方存在するが内容が乖離しており、再同期したい。

対応ドキュメント種別:

- Claude Code スキルファイル: `SKILL.md` / `SKILL.ja.md`（`.claude/skills/<name>/` 配下）
- README: `README.md` / `README.ja.md`（同一ディレクトリ配置。例: `docker/server/`）
- 一般的な `*.ja.md` サフィックス規約の Markdown（同一ディレクトリ配置）
- `docs/**` のドキュメント（同じ同居規約。`docs/**/<name>.md` と `docs/**/<name>.ja.md`）

## 最初に行うこと: 入力の確認

このスキルでは、**スキル起動直後に必ず `AskUserQuestion` で以下を確認する**。

1. **対象ファイルパス** — ユーザーが指している元ファイル（canonical または翻訳）。
2. **方向** — 何を生成するか:
    - `canonical-from-translation`: 日本語を英訳し canonical を作成する。
    - `translation-from-canonical`: 英語を和訳し翻訳ファイルを作成する。
    - `sync-both`: 両方存在しており、差分を解消して片側を書き換える。

手順:

1. スキル引数や直近メッセージにファイルパスが含まれていれば候補として提示する。
2. 周辺ディレクトリを調査し、対となるファイルの有無と同居する `*.ja.md` の規約を確認する。
3. `AskUserQuestion` を呼び出す:
    - 質問 1: 「対象ファイルパスを確認してください。」（検出した候補を併記）
    - 質問 2: 「方向は？（canonical-from-translation / translation-from-canonical / sync-both）」 — 既存ファイル状況から推奨オプションを併記。
    - `sync-both` の場合はさらに「どちら側を source of truth として同期しますか？」を確認。

対象パスと方向が確定するまでは、翻訳のためのファイル読み書きを行わないこと。

## リポジトリ規約

ペアの両側を生成する際に、以下の規約を適用する。

### 言語

- **英語が canonical。** 英語ファイルが source of truth。
- **日本語は翻訳。** canonical な英語版との同期を維持する。

### SKILL ファイル（`.claude/skills/<name>/`）

- `SKILL.md`（canonical, 英語）:
  - YAML frontmatter（`name`, `description`）を必須とする。
  - `description` は英語で、スキル選択精度が最大化されるように記述する。
  - 冒頭付近に次の 1 行ポインタを含めること: `A Japanese reference translation of this skill is available at SKILL.ja.md in the same directory (not loaded as a skill; for human reference only).`
- `SKILL.ja.md`（翻訳, 日本語）:
  - YAML frontmatter を含めてはならない（ツールによる誤検出を避けるため。Claude Code は `SKILL.md` のみを skill としてロードするが、曖昧さをなくすため明示的に外す）。
  - 冒頭に「翻訳であること、直接編集禁止、更新は `SKILL.md` 起点」を示す blockquote ヘッダを日本語で記載する。

### README およびその他の一般ドキュメント

- `README.md` / `README.ja.md`（同一ディレクトリ配置）:
  - frontmatter の要件は無し。
  - 翻訳ファイル側は冒頭に「canonical の翻訳である」旨を短く記載する。
  - canonical 側から翻訳へのリンク（`See [README.ja.md](README.ja.md) for the Japanese version.`）は任意で追加可。

### `docs/**` パラレルツリー規約

- Canonical: `docs/<path>/<name>.md`
- 翻訳: `docs/<path>/<name>.ja.md`
- セクション構造、見出し、リンク先を両ファイル間で 1:1 で揃える。

## AI Modification Scope

AGENTS.md の "Exception: Skill Execution" 節に基づき、このスキル実行中は AI Modification Scope の縛りを解放する。ただしユーザーが最初のステップで確定した特定のドキュメントペアに限定する。

変更可能なパス:

- 確認済みの対象ファイル。
- その対となるファイル（新規作成または再生成）。
- それ以外は変更しないこと。

スキル実行中でも保護対象として維持されるもの:

- `AGENTS.md` / `CLAUDE.md`
- 生成ファイル（`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, `docs/` 配下の生成物）
- `.claude/settings.json` の `permissions.deny` に列挙された任意のパス

### 対訳側の読み取りについて

`AGENTS.md` はエージェントに `*.ja.md` を読むなと言っており、あの規則が言っているのは知識の出所のことである。翻訳は原文に遅れるので、そこから考えると古い答えが出る。読むべきは canonical の英語版である。

このスキルの subject は対訳ペアそのもので、その理屈が覆わない唯一の場合になる。更新する側を読めない同期は、目隠しで上書きする（そこに既にある訳語と、2 つのファイルが共有している構造を捨てる）か、作業を差し戻すかしかない。後者は、1 世代遅れた翻訳の隣に `SKILL.md` が出荷される道である。

この場合の例外は `AGENTS.md` が持っている。上の段落から推論するのではなく、そちらを読むこと。**この許可はこのスキルが与えるものではない。** ここで広げるものも無い。対象は最初に確認したペア、期間はこの実行の間、目的は変更の落ちる位置の特定と既存の訳語への追随に限られる。canonical は英語のままであり、翻訳がシステムに関する事実の出所になることは無く、確認したペアの外の `*.ja.md` は開かない。`AGENTS.md` にその例外が無いなら、止めて人間に聞くこと。スキルがリポジトリの規則からの自己免除を宣言することは、規則の穴を回る近道ではなく `AGENTS.md` が禁じている loophole そのものである。

## 実行ステップ

### 1. 元ファイルの読み込み

確認済みの元ファイルを全文読み込む。方向が `sync-both` の場合は両ファイルを読む。

### 2. 出力パスの決定

- `canonical-from-translation`:
  - `foo.ja.md` → 同一ディレクトリの `foo.md`。
  - `docs/<path>/<name>.ja.md` → `docs/<path>/<name>.md`。
- `translation-from-canonical`:
  - `foo.md` → 同一ディレクトリの `foo.ja.md`。
  - `docs/<path>/<name>.md` → `docs/<path>/<name>.ja.md`。
- `sync-both`:
  - source of truth ではない側を書き換える。

### 3. 翻訳（または同期）

- 見出し構造、リスト階層、コードブロック、リンク先を正確に保持する。
- 本文、コードブロック内コメント（元が翻訳されていた場合のみ）、インラインテキストを翻訳する。
- 翻訳しないもの: 識別子、ファイルパス、コマンド、コードサンプル（自然言語文字列で元が翻訳されていたものを除く）。
- SKILL ファイルでは、上記「リポジトリ規約」の frontmatter ルールと同期ノートルールも併せて適用する。

### 4. 出力ファイルの書き込み

- 生成したファイルを対象パスへ書き込む。
- 両方存在しユーザーが `sync-both` を選択した場合は、source of truth ではない側のみ書き込む。

### 5. 相互参照の追加

- `SKILL.md` を作成する場合、`SKILL.ja.md` への 1 行ポインタが含まれていることを確認する。
- `SKILL.ja.md` を作成する場合、冒頭の blockquote 同期ノートが含まれていることを確認する。
- README ペアでは、canonical から翻訳へのリンクはユーザーの要望時のみ追加する。

### 6. 検証

- 両ファイル間でセクション数と見出しテキストを diff し、1:1 で対応していることを確認する。
- コードブロックがバイト同一であることを確認する（コードブロック内の翻訳済み散文部分を除く）。
- きれいに対応付けられないセクションがあればユーザーに報告し、対応方針を確認する。

### 7. Markdown Lint による検証

生成ファイル（および `sync-both` モードでは同期側）の書き込み後、以下を実行する。

```sh
make md-fix
make md-lint
```

`make md-fix` はリポジトリ全体に対して `markdownlint-cli2 --fix` を実行し、よくある違反（見出し / リスト / コードブロック周辺の空行、行末空白、ファイル末尾の改行など）を自動修正する。続けて `make md-lint` で `.markdownlint-cli2.yaml` 準拠かを検証する。

`make md-lint` がエラーを報告する場合:

1. lint 出力を確認する。
2. 自動修正で解消できないルール（見出し階層、重複見出し、bare URL など）を手で修正する。
3. clean になるまで `make md-fix` → `make md-lint` を繰り返す。

`make md-lint` がクリーン終了するまでスキルを完了報告しない。

`make md-fix` はリポジトリ全体を対象にするため、確認済みペアとは無関係な Markdown も自動修正される可能性がある。その場合、変更された他ファイルの一覧を完了報告時にユーザーへ提示し、レビューできるようにする。

## チェックリスト

完了報告時に以下を確認すること。

- [ ] 対象ファイルパスを `AskUserQuestion` でユーザーに確認済み
- [ ] 方向（`canonical-from-translation` / `translation-from-canonical` / `sync-both`）を確認済み
- [ ] `sync-both` の場合は source of truth 側を確認済み
- [ ] 出力ファイルパスがそのドキュメント種別のリポジトリ規約に従っている
- [ ] frontmatter ルールが正しく適用されている（canonical SKILL には付与、翻訳 SKILL には付与しない）
- [ ] 翻訳側 SKILL ファイルに同期ノートのヘッダが含まれている
- [ ] セクション構造とコードブロックが 1:1 で一致している
- [ ] `make md-lint` がクリーン終了する
- [ ] 確認済みペア以外のファイルを変更していない

## 注意事項

- 確認済みのドキュメントペア以外のファイルを変更しないこと。
- 識別子、ファイルパス、コマンド、その他の技術トークンを翻訳しないこと。
- 日本語へ翻訳する際は AGENTS.md の "Output Language" ルール（人間に見える本文は日本語）に従う。英語へ翻訳する際は、AI のスキル description として適切な簡潔・宣言的な記述を心がける。
- "対応ドキュメント種別" に列挙されていない種別を依頼された場合は、想定する命名規約をユーザーへ確認してから進めること。
