# Tools コンテナ

プロジェクトの**コード生成・バンドル用ツールコンテナ**を定義する Dockerfile です。マルチステージビルドにより Go / Node.js / Python のツール環境を提供します。

## 役割

`docker/tools/Dockerfile` はビルドで必要となるすべてのコード生成・lint・セキュリティ・ドキュメント生成ツールを、言語ごとに 1 ステージずつ隔離したランナーイメージにパッケージングします。開発者と CI はこれらのコンテナを `make` ターゲット（`make gen-api` / `make gen-query` / `make sql-lint` 等）経由で起動するため、誰も Go / Node / Python ツールチェインをローカルにインストールする必要がありません。マシン間でツールバージョンが再現可能になり、生成物が既知のツールチェインに固定されます。

## ビルドターゲット

|ターゲット|ベースイメージ|担当範囲|
|---|---|---|
|`go_tools`|`golang:1.27.1-alpine`|Go のコード生成・lint・セキュリティスキャン・ドキュメント生成（[ツール](#go_tools)）|
|`node_tools`|`node:24.19.0-alpine`|OpenAPI バンドル、Markdown / コミットの lint、ポータルのビルドとスクリプトのテスト（[ツール](#node_tools)）|
|`python_tools`|`python:3.14.7-slim`|SQL の lint（[ツール](#python_tools)）|

## go_tools

Go 用のコード生成・lint・セキュリティ・ドキュメント生成ツール：

|ツール|用途|
|---|---|
|`oapi-codegen`|OpenAPI 仕様から Go サーバー / 型を生成|
|`mockgen`|Go interface からモックを生成|
|`sqlc`|SQL から型安全な Go コードを生成|
|`migrate`|データベースマイグレーション CLI|
|`trivy`|脆弱性・設定ミスのスキャナー|
|`actionlint`|GitHub Actions ワークフローの Lint|
|`shellcheck`|シェルスクリプトの Lint|
|`hadolint`|Dockerfile の Lint|
|`gitleaks`|コミットされた認証情報を検出するシークレットスキャナー|
|`godoc`|Go パッケージドキュメントの配信 / 生成|
|`godoc-static`|godoc 出力から静的 HTML を生成|

## node_tools

OpenAPI ドキュメント処理とポータル生成用のツール：

|ツール|用途|
|---|---|
|`redocly-cli`|OpenAPI YAML のバンドル（`$ref` 解決）と HTML ドキュメント生成|
|`markdownlint-cli2`|ドキュメント用の Markdown リンター（`make md-lint`）|
|`@commitlint/cli`|コミットメッセージのリンター（`make commitlint`。`commit-msg` フックに配線）|
|`js-yaml`|ポータルドキュメント生成スクリプト用の YAML 処理|
|`pnpm`|リポジトリ内の 2 つの Node パッケージを解決する。lockfile も `node_modules` もそれぞれ別で、`scripts/`（`/app/scripts/node_modules` へ install）と、ポータルフロントエンドを `docs/portal/` へビルドする `docs-viewer/`（`make gen-portal-build` / `make portal-test`）。|
|`tsx`|リポジトリの TypeScript 補助スクリプト（`scripts/**/*.ts`）をビルドなしで実行する|
|`typescript`|その型検査（`make scripts-typecheck`）|
|`vitest`|補助スクリプトの判定ロジックの単体テスト（`make scripts-test`）|
|`mermaid`|`scripts/mermaid-lint/index.ts` が ` ```mermaid ` フェンスを本物のパーサで構文検証するために使う（`make md-lint`）|
|`linkedom`|`mermaid.parse` を Node で動かすためのヘッドレス DOM。Markdown 内 mermaid の構文 Lint（`scripts/mermaid-lint/index.ts`）で使用|

## python_tools

SQL リンティングツール：

|ツール|用途|
|---|---|
|`sqlfluff`|migration / DML / seed ファイル用の SQL リンター|

他の 2 つのステージと違い、ツール本体は `mise.toml` 由来ではありません。mise が入れるのは `uv` だけで、
`sqlfluff` はその `uv` が [`python/sqlfluff.txt`](../../python/sqlfluff.txt) から `--require-hashes` 付きで
install します。これにより推移依存まで含めてバージョンとハッシュが固定されます
（[ADR-0084 (mise-ssot-drift-gate)](../../docs/adr/0084-mise-ssot-drift-gate.ja.md)）。

## docker-compose サービス

```yaml
go_tool_runner:     # target: go_tools,    profile: generate
node_tool_runner:   # target: node_tools,  profile: generate
python_tool_runner: # target: python_tools, profile: generate
```

すべてのツールランナーはプロジェクトルートを `/app` にマウントし、root で実行します。

## 実行方法

```bash
make gen        # すべてのコード生成を実行
make gen-api    # OpenAPI バンドル + Go コード生成
make gen-query  # sqlc コード生成
```

## ツールを追加する場合

1. 適切な Dockerfile ステージにツールをインストール
2. `docker-compose.yaml` に `profiles: [generate]` で新しいサービスを追加
3. 必要に応じて Makefile ターゲットを追加

## 注意点

- すべてのターゲットで作業ディレクトリは `/app`
- Go ツールはビルダーステージでインストールし、ランタイムステージにコピーしてイメージサイズを最小化（`go_tools`）
- ツールのバージョンは `mise.toml`（バージョンの SSOT）で固定 — 更新はそこで行い、ローカルと CI のイメージを一致させること。PyPI のツールだけは例外で、詳細は上の [python_tools](#python_tools) を参照
- このイメージが install する Node 依存を宣言するのは `scripts/` / `docs-viewer/` の 2 つ（それぞれ独自の `package.json` / `pnpm-lock.yaml` / `pnpm-workspace.yaml` を持つ）で、ビルドはそれぞれのマニフェストを本来の場所へコピーしてその場で install する。いずれもこのディレクトリには無いため、依存の変更は使う側のコードと並べてレビューされる
