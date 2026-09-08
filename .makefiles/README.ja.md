# Make コマンド一覧

## 役割

`.makefiles/` はプロジェクトで使用するすべての `make` ターゲットの中央レジストリです。各 `.mk` ファイルは関連ターゲットを領域別（application / database / sql / go / openapi / docs / github / tools）にグルーピング。トップレベルの `makefile` はそれらを `include` するだけなので、新規ターゲット追加は該当グループファイルへの追記だけで完結し、トップレベル編集は不要です。

Make ターゲットは主に以下の単位で整理されています。

- `.makefiles/app` : アプリケーション起動・常駐プロセス(worker / outbox-relay)・Job 実行・埋め込み env 材料化
- `.makefiles/database` : DB 初期化 / マイグレーション / シード / DML / スキーマ
- `.makefiles/sql` : SQL Lint / Fix
- `.makefiles/markdown` : Markdown Lint / Fix
- `.makefiles/security` : Trivy 依存脆弱性スキャン
- `.makefiles/docker` : compose プロジェクト / ホストポート定義・Dockerfile Lint（hadolint）・image digest 固定
- `.makefiles/openapi` : OpenAPI バンドル / API ドキュメント生成
- `.makefiles/go` : Go コード生成 / フォーマット / Lint / テスト / ツール管理
- `.makefiles/python` : PyPI ツールの lockfile 生成
- `.makefiles/agents` : Closed Loop 開発フィードバック / エージェント向け静音実行のログ
- `.makefiles/graphify` : ナレッジグラフのエクスポート / 追跡成果物の可搬性チェック
- `.makefiles/docs` : Portal / ツール情報などのドキュメント生成
- `.makefiles/gen` : 各種生成処理の一括実行
- `.makefiles/github` : GitHub 初期設定 / リリース / ラベル / ルール設定

## 命名規約

- ターゲット名はハイフン区切りの小文字（`make new-migrate-<name>`、`make gen-api`）
- ターゲットは 2 種類:
  - **通常ターゲット**: 開発者がローカルで呼ぶ。再現性のため Docker コンテナ経由で実行。ただし一部はホスト上のツールを解決する（`lint` / `fix` の `golangci-lint`、`actions-zizmor` の `zizmor`、`go-cooldown-gate` / `go-cooldown-audit`、`tool-cooldown-gate` / `tool-cooldown-audit`、`pnpm-cooldown-check`）。tool-runner が Alpine であり、上流が musl ビルドを配布していないためである。これは規約の例外ではなく [ツールチェイン実行ルール](../docs/rules.ja.md#ツールチェイン実行ルール) が定める最終手段であり、供給は `make install-tools` が担い、イメージが担うはずだった再現性は `mise.toml` のピンが引き受ける
  - **`-ci` ターゲット**: CI ランナー、またはツールをローカルインストール済みの開発者向け低レベルコマンド
  - **`ai-` ターゲット**: 上記のいずれをも包む `ai-%` パターンルール。`make ai-<target>` は `<target>` を実行し、出力をすべて `tmp/ai-logs/<target>.txt` へ退避する。成功時は何も表示せず、終了コードはそのまま返し、失敗したときだけログの場所を 1 行で示す。AI エージェントがコマンドの出力を全文コンテキストへ読み込むうえ、ハーネスは*失敗した*コマンドを保存せず抜粋するため、ノイズは毎回支払われ、切り落とされるのは診断そのものになる。`ai-` が前に付くのには理由がある。接尾辞 `%-ai` は基底が既にパターンルールのターゲットだと一致先が割れ（`new-migrate-add-ai` は `new-migrate-%` にも `%-ai` にも当たる）、どちらが勝つかは競合ルールの前提条件が満たせるかで変わり、外れたほうはエラーにならず黙って別のことをする。出力そのものが成果物のターゲット（`help` / `load-status` / `base-branch`）や、完了しないターゲット（`serve` / `worker` / `outbox-relay`）には使わない。また新規ターゲット名に `ai-` 接頭辞を付けないこと
- すべて `.PHONY` 指定し、末尾 `##` コメントで `make help` 出力に載せること

## 補足

- `tmp/ai-logs/` は gitignore 済みで worktree ごとに独立しており、`make clean-ai-logs` が空にする。成功時もログを残すため、生成系の実行内容を回し直さずに読み返せる
- 既存グループファイルへのターゲット追加ならトップレベル編集は不要。ただし新規 `.mk` ファイルを追加する場合は、トップレベル `makefile` へ `include` 行の追記が必要（ワイルドカードではなく個別 include のため）
- ファイル直接作成より `make new-migrate-<name>` 等のヘルパーを優先（命名規約と番号採番を自動化）
- 一回限りの運用コマンド（`make setup-repo` 等）は `.makefiles/github/operation/` 配下に置き、開発者向けターゲットと分離する

## `.makefiles/app` 系

アプリケーションの開発環境起動や Job 実行に関するターゲット群です。

compose のサービスは 2 層に分かれます（後述の `.makefiles/docker` 系を参照）。共有の **infra 層**
（`database` / `observability` / `garage` / `elasticmq` / `dynamodb_local` / `goaws`）は固定プロジェクト `gobp-shared` に 1 インスタンスだけ置き、
checkout 毎の **app 層**（`api_server` / `mock_auth_server`）は自 checkout の `APP_PROJECT` で起動します。

### アプリケーション起動関連

| コマンド | 説明 | 主な用途 |
| --- | --- | --- |
| `make serve` | 共有インフラを起動（`infra-up`）したうえで、自 checkout の app サービスを `app-up` 経由で起動し、DB スロットの heartbeat を更新します。 | 通常のローカル開発開始 |
| `make serve-build` | app イメージをキャッシュ利用で再ビルドし、共有インフラを起動したうえで app サービスを `app-up` 経由で起動します。 | Dockerfile や依存変更の反映 |
| `make serve-build-clean` | app イメージを `--no-cache --pull` でクリーンビルドし、共有インフラを起動したうえで app サービスを `app-up` 経由で起動します。 | base image 更新の取り込み（例: Go バージョンアップ） |
| `make app-up` | 自 checkout の app サービスを起動し、`api_server` の healthcheck が通るまで待ちます（`up --wait`）。呼び出し元の成功メッセージが「コンテナが存在する」ではなく「API が応答できる」を意味するようになります。失敗時は `api_server` ログの末尾 `APP_LOG_TAIL` 行（既定 `50`）を出して非ゼロで終了します。 | 内部用 — 3 つの `serve` 系ターゲットが呼びます。起動失敗後にインフラ起動と provisioning を飛ばして app だけ上げ直すのにも使えます |
| `make serve-stop` | 自 checkout の app プロジェクトだけを停止します。 | 共有インフラや他 checkout に触れず API を止める |
| `make infra-up` | 共有インフラのサービス（`--wait`）と one-shot の `garage_init` を `gobp-shared` プロジェクトで起動します。 | 共有インフラだけを起動する（`serve` / `job` / `worker` が冪等に呼びます）。worktree では `INFRA_NO_RECREATE` も渡し、他の checkout が使っている可能性のある稼働中コンテナは残します。このとき定義変更の反映は `infra-down` → `infra-up` になります |
| `make infra-down` | 共有インフラのプロジェクトを停止します（名前付きボリュームは保持）。 | インフラを落とす。**全 checkout / worktree に影響します** |
| `make tools` | `tools` プロファイルの開発支援ツール群を共有インフラのプロジェクトで起動します。 | 開発ツール利用時（SQL editor `:2000` / docs viewer `:2001`）。こちらも `INFRA_NO_RECREATE` を渡します（プロファイルに `database` / `garage` が含まれるため） |
| `make all` | `tools` → `serve-build` の順に全サービスを一括起動します。 | ローカルスタック全体を一度に立ち上げる |
| `make tool-runners-build` | オンデマンド実行のツールランナー画像(go/node/python)をキャッシュ利用でビルドします（起動はしません）。 | ツールランナーの Dockerfile や依存変更の反映 |
| `make tool-runners-build-clean` | ツールランナー画像を `--no-cache --pull` 付きでクリーンビルドします（起動はしません）。 | ツールランナーの base image 更新の取り込み |

#### `make job NAME=<job名> ARGS="<引数>"`

アプリケーションの Job を実行します。
共有インフラを起動したうえで、自 checkout の app プロジェクトの使い捨て `api_server` コンテナ
（`run --rm`）で `cmd/main.go job` を呼び出します。

- `NAME`: 実行する Job 名
- `ARGS`: Job に渡す追加引数（任意）

例:

```sh
make job NAME=sample-job
make job NAME=batch-import ARGS="--target=local --dry-run"
```

### 常駐プロセス(worker / outbox-relay)関連

いずれも `SIGTERM` / `Ctrl-C` まで常駐するデーモンで、共有インフラを起動したうえで自 checkout の
app プロジェクトの使い捨て `api_server` コンテナ内（`make job` と同じ `go run ./cmd/` 方式）で
実行します。

#### `make worker NAME=<worker名> ARGS="<引数>"`

pull-ack worker を起動します。`NAME` は worker 名（必須）、`ARGS` は任意です。

> 既定では worker が 1 つも登録されていません（`WorkerModule()` は空の seam）。
> そのため実 worker を配線するまでは `unknown worker` で失敗します。worker 追加後の
> ローカル動作確認用の起動口として置いています。

```sh
make worker NAME=sampleworker
```

#### `make outbox-relay ARGS="<引数>"`

outbox relay を起動します（outbox テーブルを周期 poll して未 publish メッセージを送出）。
relay はちょうど 1 つの配送チャネルを担当し既定のチャネルを持たないため、`ARGS` は必須です。
`replay` サブコマンドにも渡ります。

```sh
make outbox-relay ARGS="--channel=http"
make outbox-relay ARGS="replay --message-id=<id>"
```

### 埋め込み env 材料化関連

サーバーバイナリは `env/.env` を埋め込みます。CI および Docker ビルドはビルド前に
環境別ファイルを `env/.env` へ材料化するため、その手順（と、ドリフト判定向けの取り消し）を
これらのターゲットへ集約します。

| コマンド | 説明 | 主な用途 |
| --- | --- | --- |
| `make materialize-env` | `env/.env.$(APP_ENV)` を `env/.env` へコピーします（既定は `APP_ENV=ci`）。 | CI / ビルドで `go build` / `go run` 前に埋め込み対象を材料化する |
| `make restore-env` | `git restore` で `env/.env` を git 管理の内容へ戻します。 | 生成物ドリフト / コミット判定の前に材料化を取り消す |

### Realtime Delivery smoke 関連

| コマンド | 説明 | 主な用途 |
| --- | --- | --- |
| `make realtime-init` | 共有インフラを起動し、app コンテナ内から Realtime Delivery の table（EventLog / StreamTicket / InstanceLease）を DynamoDB Local に、fan-out の topic を GoAWS に作ります（`go run ./cmd/ realtime-init`）。冪等 — 何度実行しても同じ状態に収束します。 | app を起動せずに資源だけ用意したいとき。`make serve` は同じ one-shot（`realtime-provision`）を自分で走らせるので、通常の経路では個別に呼ぶ必要はありません |
| `make realtime-reset` | `dynamodb_local` を起動し、host から `scripts/realtime-reset` を実行してこの checkout の 3 つの table を削除し、消え切るまで待ちます。作成は `realtime-provision` の担当のままです。データベースの所有者であることを要求し、host を持たない `-endpoint` と AWS の host を拒否します（ダミー署名鍵が第 2 の制御なので、この deny だけが防御ではありません）。 | local データベースを作り直す経路すべて（`slot-acquire` / `db-local-reinit` / `db-init-local`）から呼ばれ、採番と EventLog を一緒に作り直すため（[db-worktree-pool.md](../docs/maintenance/db-worktree-pool.ja.md) を参照）。古い採番で stream が詰まったときに手で叩くのにも使えます |
| `make realtime-smoke` | 共有インフラを起動し、`scripts/realtime-smoke` を AWS SDK Go v2 で DynamoDB Local / GoAWS に対して実行して、呼び出しごとの判定（互換 / 非互換 / 未対応 / 検証不能）を表にします。resource は実行ごとの乱数名で作り終了時に削除します。`ARGS` で flag を渡します（`-format markdown` / `-subscribers N` / `-keep` / `-strict`）。 | Realtime Delivery が行う呼び出しをエミュレータが今も受け付けるかの確認（image を上げたときなど） |
| `make realtime-provision` | 共有インフラが起動済みであることを前提に、Realtime Delivery の資源だけを用意します。 | 内部用。`serve` が呼びます。`api_server` コンテナで `go run ./cmd/ realtime-init` を実行し、その出力は捨てます。データベースの所有を必要とします。 |
| `make realtime-contract-test` | Realtime Delivery の contract test を実行します。 | ホスト上で `go test` を走らせます。既定の接続先は DynamoDB Local / GoAWS で、`REALTIME_TEST_*` を指定すると実 AWS へ向け直します。`ARGS` でフラグを渡せます。 |

## `.makefiles/database` 系

DB 操作全般を扱うターゲット群です。
マイグレーション、シード投入、DML マージ、スキーマ生成、DB 初期化などを提供します。

### DB 初期化関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make db-init` | 所有している local / test データベースの初期化をまとめて実行します。 | `db-init-local` と `db-init-test` を順に呼び出します。 |
| `make db-init-local` | 所有している local データベースを初期化します。 | `db-local-migrate-down` → `db-local-migrate-up` → `db-local-seed` を実行します。 |
| `make db-init-test` | 所有している test データベースを初期化します。 | `db-test-migrate-down` → `db-test-migrate-up` → `db-test-seed` を実行します。 |
| `make db-reinit` | `DB` が指すデータベースを `db-drop-tables` → `db-migrate-up` → `db-seed` で再構築します。 | `migrate-down` を経由しないため、マイグレーション履歴がこのブランチと食い違ったデータベースでも復旧できます。 |
| `make db-local-reinit` | 共有 `local` データベースを同じ手順で再構築し、続けて `realtime-reset` を実行します。 | `DB=$(DB_LOCAL)`。 |
| `make db-test-reinit` | 共有 `test` データベースを同じ手順で再構築します。 | `DB=$(DB_TEST)`。 |
| `make db-drop-tables` | `public` の全テーブルを削除します（拡張は残します）。 | `database/maintenance/drop-all-tables.sql` を `ON_ERROR_STOP=1` 付きの `psql` へ流します。`migrate-down` が使えない状況の保守用です。データベースの所有を必要とします。 |
| `make require-db-owner` | この checkout が所有するデータベースがあることを検証します。 | データベース名を解決する全ターゲットの前提条件です。DB スロットを持たないリンク worktree では、主 checkout の `local` / `test` へフォールバックせず失敗します。`docs/maintenance/db-worktree-pool.ja.md` を参照。判定の実体は `internal/cli/dbslot` にあり、git 実行ファイルが無い場合と git リポジトリでない場合は素通り、git リポジトリではあるのに構成を読み取れない場合は失敗します。 |

### DB マイグレーション関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make new-migrate-<name>` | 新しいマイグレーションファイルを生成します。 | `database/migrations` 配下に連番付きの `.up.sql` / `.down.sql` を作成します。 |
| `make check-migration-up-version` | `up` 側マイグレーションのバージョン重複をチェックします。 | `scripts/migration-lint` を実行します。連番の判定規則はシェル片ではなくそちらにテスト付きで置いています。 |
| `make check-migration-down-version` | `down` 側マイグレーションのバージョン重複をチェックします。 | `scripts/migration-lint` を実行します。 |
| `make check-migration-up-gap` | `up` 側マイグレーションの連番ギャップをチェックします。 | `scripts/migration-lint` を実行します。マイグレーションが 1 件も無い場合は通過するため、マイグレーション集合が空でもゲートは落ちません。 |
| `make check-migration-down-gap` | `down` 側マイグレーションの連番ギャップをチェックします。 | `scripts/migration-lint` を実行します。 |
| `make db-migrate-up DB=<database>` | 指定した DB に対して、全マイグレーションを最新まで適用します。 | 例: `make db-migrate-up DB=local` |
| `make db-migrate-up-<steps> DB=<database>` | 指定した DB に対して、現在位置から指定段数だけマイグレーションを適用します。 | 例: `make db-migrate-up-2 DB=local` |
| `make db-migrate-down DB=<database>` | 指定した DB に対して、全マイグレーションを初期状態までダウングレードします。 | なし |
| `make db-migrate-down-<steps> DB=<database>` | 指定した DB に対して、指定段数だけダウングレードします。 | なし |
| `make db-local-migrate-up` | 所有している local データベースに対して全マイグレーションを最新まで適用します。 | `DB=$(DB_LOCAL)`（`local`、スロット保持中は `wt<N>_local`）を指定した `db-migrate-up` のエイリアスです。 |
| `make db-local-migrate-up-<steps>` | LocalDB に対して指定段数だけマイグレーションを適用します。 | なし |
| `make db-local-migrate-down` | 所有している local データベースを初期状態までダウングレードします。 | `DB=$(DB_LOCAL)` を指定した `db-migrate-down` のエイリアスです。 |
| `make db-local-migrate-down-<steps>` | LocalDB を指定段数だけダウングレードします。 | なし |
| `make db-test-migrate-up` | 所有している test データベースに対して全マイグレーションを最新まで適用します。 | `DB=$(DB_TEST)`（`test`、スロット保持中は `wt<N>_test`）を指定した `db-migrate-up` のエイリアスです。 |
| `make db-test-migrate-up-<steps>` | TestDB に対して指定段数だけマイグレーションを適用します。 | なし |
| `make db-test-migrate-down` | 所有している test データベースを初期状態までダウングレードします。 | `DB=$(DB_TEST)` を指定した `db-migrate-down` のエイリアスです。 |
| `make db-test-migrate-down-<steps>` | TestDB を指定段数だけダウングレードします。 | なし |
| `make db-migrate-ci-up DB=<database>` | Docker を介さず、直接 `cmd/main.go migrate-up` を実行します。 | CI 用ターゲットです。 |
| `make db-migrate-ci-up-<steps> DB=<database>` | Docker を介さず、指定段数だけ `migrate-up` を実行します。 | CI 用ターゲットです。 |
| `make db-migrate-ci-down DB=<database>` | Docker を介さず、直接 `cmd/main.go migrate-down` を実行します。 | CI 用ターゲットです。 |
| `make db-migrate-ci-down-<steps> DB=<database>` | Docker を介さず、指定段数だけ `migrate-down` を実行します。 | CI 用ターゲットです。 |

例:

```sh
make new-migrate-create_users_table
make db-migrate-up DB=local
make db-migrate-up-10 DB=local
```

### DB シード関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make db-seed DB=<database>` | 指定した DB に対してシードデータを投入します。 | Docker コンテナ内で `cmd/main.go db-seed` を実行します。 |
| `make db-seed-ci DB=<database>` | Docker を介さず、直接シード投入処理を実行します。 | CI 用ターゲットです。 |
| `make db-local-seed` | 所有している local データベースにシードデータを投入します。 | `DB=$(DB_LOCAL)` を指定した `db-seed` のエイリアスです。 |
| `make db-test-seed` | 所有している test データベースにシードデータを投入します。 | `DB=$(DB_TEST)` を指定した `db-seed` のエイリアスです。 |

### DB 生成・補助関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make gen-db-schema` | DB スキーマドキュメントを生成します。 | ER 図やスキーマ出力の更新に使用します。 |
| `make gen-db-schema-ci` | SchemaSpy コンテナを直接実行してスキーマドキュメントを生成します。 | CI 用ターゲットです。 |
| `make dump-schema` | スキーマダンプを実行します。 | SQLC 生成や DML マージの前処理として利用します。所有者ごとの使い捨てデータベース（`gen_schema`、スロット保持中は `gen_schema_wt<N>`）を当該ブランチの migration から作り直してダンプします。 |
| `make dump-schema-ci` | Docker を介さず、直接 `cmd/main.go dump-schema` を実行します。 | CI 用ターゲットです。 |
| `make fix-collation` | データベースのコラテーションを修正します。 | なし |
| `make fix-collation-ci` | Docker を介さず、直接コラテーション修正処理を実行します。 | CI 用ターゲットです。 |
| `make db-ensure` | `DB` が指すデータベースを、無ければ作成し、`pg_trgm` 拡張を初期化します。 | 冪等です。データベースの所有を必要とします。 |

### DML マージ関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make merge-dml` | すべての DML マージ処理を実行します。 | `merge-dml-repo` → `merge-dml-qs` → `merge-dml-sysq` → `merge-dml-cs` を順に実行します。 |
| `make merge-dml-repo` | Repository 用 DML をマージします。 | なし |
| `make merge-dml-qs` | Query Service 用 DML をマージします。 | なし |
| `make merge-dml-cs` | Command Service 用 DML をマージします。 | なし |
| `make merge-dml-sysq` | System Query 用 DML をマージします。 | なし |
| `make merge-dml-core type="<type>" work-dir="<dir>"` | 指定した種別の DML マージを実行します。 | Docker コンテナ経由で `make merge-dml-ci-core` を呼び出します。 |
| `make merge-dml-ci` | すべての DML マージ処理を直接実行します。 | CI 用ターゲットです。 |
| `make merge-dml-ci-repo` | Repository 用 DML をマージします。 | CI 用ターゲットです。 |
| `make merge-dml-ci-qs` | Query Service 用 DML をマージします。 | CI 用ターゲットです。 |
| `make merge-dml-ci-cs` | Command Service 用 DML をマージします。 | CI 用ターゲットです。 |
| `make merge-dml-ci-sysq` | System Query 用 DML をマージします。 | CI 用ターゲットです。 |
| `make merge-dml-ci-core type="<type>" work-dir="<dir>"` | `cmd/main.go merge-dml` を直接実行します。 | CI 用ターゲットです。 |

例:

```sh
make merge-dml-core type="repository" work-dir="/app"
```

### DB スロットプール（worktree）関連

共有 Postgres 1 インスタンスをすべてのチェックアウトが使い、worktree はそこに**スロット**を貸与されます。スロットの実体は専用の `wt<N>_local` / `wt<N>_test` データベースと、アプリが bind するホストポートです。貸与を受けていない場合、各ターゲットは既定ポートの `local` / `test` へ落ちます。これが単一チェックアウトの構成です。不変条件は [db-worktree-pool.md](../docs/maintenance/db-worktree-pool.ja.md) が持ちます。

| コマンド | 説明 | 備考 |
| --- | --- | --- |
| `make slot-acquire` | DB スロットを取得し、貸与されたデータベースを作り直します。 | `go run ./cmd/ db-slot acquire` を実行したのち、スロットのデータベースごとに `db-reinit` を呼びます。2 回の再構築を別々の `make` 呼び出しに分けているのは意図的です。両者は `db-reinit` という共通の前提条件を持つため、1 回の呼び出しにまとめると `db-reinit` が一度しか実行されず、2 つ目のデータベースが手つかずで残ります。 |
| `make slot-free` | 保持中のスロットだけを解放します。worktree は残します。 | データベースは保持されるため、取り直しは warm な状態から始まります。 |
| `make slot-release` | worktree を撤収します。app の停止とローカルイメージの削除 → スロット解放 → worktree 削除の順で実行します。 | 主 checkout では実行を拒否します。`--git-dir` と `--git-common-dir` を比較し、一致する場合は非ゼロで終了します。 |
| `make slot-status` | スロットプールの現在の占有状況を表示します。 | `slot-acquire` が失敗した後に有用です。再構築が失敗していても、貸与自体は成功していることが多いためです。 |

## `.makefiles/sql` 系

SQL ファイルに対する静的検査と自動修正を扱うターゲット群です。
対象は Migration / DML / Seed SQL です。

### SQL Lint 関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make sql-lint` | SQL の Lint を一括実行します。 | `sql-lint-migrations` → `sql-lint-dml` → `sql-lint-seed` を順に実行します。 |
| `make sql-lint-migrations` | マイグレーション SQL の Lint を実行します。 | なし |
| `make sql-lint-dml` | DML SQL の Lint を実行します。 | なし |
| `make sql-lint-seed` | シードデータ SQL の Lint を実行します。 | なし |
| `make sql-lint-ci` | 全カテゴリの SQL Lint を 1 コンテナで実行します。 | CI 用ターゲット。`sql-lint-migrations-ci` / `sql-lint-dml-ci` / `sql-lint-seed-ci` に依存します。 |
| `make sql-lint-migrations-ci` | `database/migrations/` に対して `sqlfluff lint` を実行します。 | CI 用ターゲットです。 |
| `make sql-lint-dml-ci` | `database/dml/` に対して `sqlfluff lint` を実行します。 | CI 用ターゲットです。 |
| `make sql-lint-seed-ci` | `database/seed/` に対して `sqlfluff lint` を実行します。 | CI 用ターゲットです。 |

### SQL Fix 関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make sql-fix` | SQL の自動修正を一括実行します。 | `sql-fix-migrations` → `sql-fix-dml` → `sql-fix-seed` を順に実行します。 |
| `make sql-fix-migrations` | マイグレーション SQL の自動修正を実行します。 | なし |
| `make sql-fix-dml` | DML SQL の自動修正を実行します。 | なし |
| `make sql-fix-seed` | シードデータ SQL の自動修正を実行します。 | なし |
| `make sql-fix-ci` | 全カテゴリの SQL 自動修正を 1 コンテナで実行します。 | CI 用ターゲット。`sql-fix-migrations-ci` / `sql-fix-dml-ci` / `sql-fix-seed-ci` に依存します。 |
| `make sql-fix-migrations-ci` | `database/migrations/` に対して `sqlfluff fix` を実行します。 | CI 用ターゲットです。 |
| `make sql-fix-dml-ci` | `database/dml/` に対して `sqlfluff fix` を実行します。 | CI 用ターゲットです。 |
| `make sql-fix-seed-ci` | `database/seed/` に対して `sqlfluff fix` を実行します。 | CI 用ターゲットです。 |

## `.makefiles/markdown` 系

Markdown ファイルに対する Lint と自動修正を扱うターゲット群です。

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make md-lint` | Markdown の Lint を実行します（markdownlint + mermaid 構文 + スキル定義の意味検査）。 | `node_tool_runner` コンテナ内で `make md-lint-ci` を呼び出します。 |
| `make md-fix` | Markdown ファイルの Lint 自動修正を実行します。 | `node_tool_runner` コンテナ内で `make md-fix-ci` を呼び出します。 |
| `make md-mermaid-lint` | ` ```mermaid ` フェンスのみを構文検証します。 | `node_tool_runner` コンテナ内で `make md-mermaid-lint-ci` を呼び出します。 |
| `make md-skill-lint` | `.claude/**` のスキル / エージェント定義と、その `.codex/**` 対応のみを検証します。 | `node_tool_runner` コンテナ内で `make md-skill-lint-ci` を呼び出します。 |
| `make md-premise-lint` | テンプレート作成後も残る文書が、作成とともに失効する前提に乗っていないことのみを検査します。 | `node_tool_runner` コンテナ内で `make md-premise-lint-ci` を呼び出します。  <!-- boilerplate-only:line --> |
| `make md-doc-ref-lint` | ADR 参照と対訳ペアが実在するかを検査します。 | `node_tool_runner` コンテナ内で `make md-doc-ref-lint-ci` を呼びます。 |
| `make md-doc-ref-fix` | ADR 参照に canonical slug を補います。 | `node_tool_runner` コンテナ内で `make md-doc-ref-fix-ci` を呼びます。 |
| `make md-lint-ci` | `markdownlint-cli2` を実行後、mermaid 構文 Lint、スキル定義 Lint の順に実行します。 | CI 用ターゲットです。`vendor/`、`node_modules/`、`.git/` を除外します。 |
| `make md-markdownlint-ci` | `MD_GLOBS` に対して `markdownlint-cli2` を直接実行します。 | CI 用ターゲット |
| `make md-mermaid-lint-ci` | `scripts/mermaid-lint/index.ts`（実 `mermaid.parse`）で ` ```mermaid ` フェンスを検証します。 | CI 用ターゲット。markdownlint は図の文法を見ません。 |
| `make md-skill-lint-ci` | `scripts/skill-lint/index.ts` で `.claude/**` の定義（frontmatter / 対訳ペアの構造 / 参照の実在性）と、`.codex/**` との対応（skill / agent の存在対応、Codex skill の構造）を検証します。 | CI 用ターゲット。markdownlint は記述と実態の一致を見ず、片側の環境にだけ入った skill も他の誰も気づきません。 |
| `make md-premise-lint-ci` | [docs/rules.md](../docs/rules.md) の *No premise the document will outlive* を `scripts/premise-lint/index.ts` で機械化したものです。テンプレート作成後も残る文書に、そこでは真でなくなる自己参照があると落ちます。探す言い回しは `scripts/premise-lint/rules.ts` が宣言します。 | CI 用ターゲット。前提を書いてよいのは、セットアップが書き換え・削除する `README*` / `docs/get-started/**` と、`boilerplate-only` / `sample-api` マーカーで囲った領域だけです。同じ語の別語義は `scripts/premise-lint/allowances.ts` へ理由付きで宣言します。  <!-- boilerplate-only:line --> |
| `make md-fix-ci` | `markdownlint-cli2 --fix` で `**/*.md` を直接修正します。 | CI 用ターゲットです。`vendor/`、`node_modules/`、`.git/` を除外します。 |

## `.makefiles/security` 系

CI のセキュリティ指摘をローカルで再現するためのスキャン（Trivy の依存 / シークレットスキャン、gitleaks シークレットスキャン、zizmor による Actions 定義の監査）です。image スキャンは CI 専用（`image-scan.yaml`）です。

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make trivy-fs` | ライブラリ依存を Trivy fs でスキャンします。 | `go_tool_runner` コンテナ内で `make trivy-fs-ci` を呼び出します。 |
| `make trivy-fs-ci` | `trivy fs` を直接実行します。 | CI 用ターゲット。CI と揃えるため `vendor/` を除外します。 |
| `make trivy-fs-release-ci` | 修正版のない脆弱性も含めて `trivy fs` を実行します。 | 昇格ゲート用の CI ターゲット。`trivy-fs-ci` との差は `--ignore-unfixed` の有無だけです。 |
| `make trivy-config` | Dockerfile の設定不備をスキャンします。 | `go_tool_runner` コンテナ内で `make trivy-config-ci` を呼び出します。 |
| `make trivy-config-ci` | `trivy config` を直接実行します。 | CI 用ターゲット。`CRITICAL,HIGH` でゲートし、許容する例外は `.trivyignore.yaml` に置きます。 |
| `make trivy-license` | 依存ライブラリのライセンスを列挙します。 | `go_tool_runner` コンテナ内で `make trivy-license-ci` を呼び出します。 |
| `make trivy-license-ci` | `trivy fs --scanners license` を直接実行します。 | CI 用ターゲット。禁止ライセンス方針が未策定のため報告専用で、severity では絞りません。 |
| `make trivy-secret` | ワーキングツリーのシークレットを Trivy でスキャンします。 | `go_tool_runner` コンテナ内で `make trivy-secret-ci` を呼び出します。 |
| `make trivy-secret-ci` | `trivy fs --scanners secret` を直接実行します。 | CI 用ターゲット。脆弱性側のターゲットは `--scanners vuln` を明示しているため、これは報告の追加ではなく検査そのものの追加です。severity では絞らず、許容する検出は `.trivyignore.yaml` に path 固定で置きます。gitleaks との重複は意図的で、Trivy は誤検知が少なく、gitleaks の正規表現 / エントロピー網は取りこぼしが少ない、という差を残します。 |
| `make trivy-image-ci` | ビルド済みイメージの脆弱性をスキャンします。 | CI 用ターゲット。対象イメージは `TRIVY_IMAGE=` で渡します。 |
| `make trivy-image-gate-ci` | ビルド済みイメージの修正版のある `CRITICAL` / `HIGH` で失敗します。 | CI 用ターゲット。対象イメージは `TRIVY_IMAGE=` で渡します。 |
| `make trivy-sbom-ci` | 生成済みの SBOM を脆弱性データベースと突き合わせます。 | CI 用ターゲット。対象ファイルは `TRIVY_SBOM_FILE=` で渡します。 |
| `make secret-scan` | ワーキングツリーのシークレットを gitleaks でスキャンします。 | `go_tool_runner` コンテナ内で `make secret-scan-ci` を呼び出します。 |
| `make secret-scan-ci` | `gitleaks dir . --redact` を直接実行します。 | CI 用ターゲット。生成ファイルは `.gitleaks.toml` で allowlist。 |
| `make secret-scan-history-ci` | `gitleaks git . --redact` を直接実行します。 | CI 用ターゲット。週次実行が使用。`dir` は作業ツリーしか見ないためコミット後に消したシークレットを取りこぼすが、`git` は履歴全体を走査する。 |
| `make go-cooldown-gate BASE=<ref>` | `BASE` との `go.mod` 差分が、cooldown 窓の内側で公開された **direct** モジュールを追加 / 更新している場合に失敗します。 | ホスト上で実行。`BASE` に既定を置かないのは意図的で、古い base は差分を黙って狭め、gate が何も見ていない状態へ縮退させるためです。Go には解決時の cooldown が無いため、この検査は検知器ではなく防御そのものです。 |
| `make go-cooldown-audit` | `go.mod` のうち窓の内側で公開されたものを報告し、期限切れ・3 ヶ月超・対象不在のバイパスエントリがあれば失敗します。 | ホスト上で実行。窓そのものではここで落ちません（既存依存は grandfather）が、失効したバイパスでは落ちます。期限は `go.mod` が変わらなくても訪れるためです。 |
| `make tool-cooldown-gate BASE=<ref>` | `BASE` との宣言差分（`mise.toml` と `python/*.in`）が、backend の窓（GitHub リリース 14 日 / パッケージレジストリ 7 日）の内側で公開されたツール版を pin している場合に失敗します。`python/*.in` の宣言と `python/*.txt` の lockfile が別の版を指している場合にも失敗します。 | ホスト上で実行。短縮名の backend 解決に `mise` を、未認証では 1 回の実行を賄えない GitHub API のために `GITHUB_TOKEN` を使います。言語ランタイムは受容したリスクとして対象外です。 |
| `make tool-cooldown-audit` | 宣言しているツールのうち窓の内側で公開されたものを報告し、期限切れ・3 ヶ月超・対象不在のバイパスエントリがあれば失敗します。 | ホスト上で実行。grandfather と失効バイパスでの失敗は Go 版と同じです。 |
| `make pnpm-cooldown-check` | `minimumReleaseAgeExclude` のうち `.github/pnpm-cooldown-bypass.toml` に期限が無いもの、期限が切れているか 3 ヶ月より先を指すもの、どの例外にも対応しないバイパスエントリ、対になる `pnpm-lock.yaml` がもう解決していない版を名指ししている例外があれば失敗します。 | ホスト上で実行。`gate` の相方が無いのは、窓そのものは pnpm の解決器が install のたびに強制しているためです。ここが見るのは免除のほうで、その期限はどちらのファイルが変わらなくても訪れます。 |
| `make actions-zizmor` | ワークフロー / composite action の定義を zizmor で監査し、`high` の指摘で失敗します。 | ホスト上で実行。`--offline` なので pre-commit フックはネットワークも `GH_TOKEN` も不要で、オンライン監査は CI に委ねます。例外設定は `.github/zizmor.yml`。 |
| `make actions-zizmor-sarif-ci` | zizmor の全指摘を SARIF として標準出力へ書き出します。 | CI 用ターゲット。severity で絞らないため code scanning には全体像が残ります。`make -s` で呼ぶこと。 |
| `make actions-zizmor-gate-ci` | zizmor の `high` の指摘で失敗します。 | CI 用ターゲット。ゲート条件は `actions-zizmor` と同じで、`GH_TOKEN` を要するオンライン監査が加わります。 |

## `.makefiles/docker` 系

全ターゲットが共有する compose プロジェクト / ホストポートの定義を持ち、`go_tool_runner` コンテナ経由で
hadolint により Dockerfile を lint し、`FROM` の base image を不変の digest へ固定します
（サプライチェーン対策）。

### compose プロジェクト定義（`compose.mk`）

`compose.mk` はターゲットを持たず、app / database 系が土台にする変数を定義するため、トップレベル
`makefile` の冒頭（「依存されるファイル」セクション）で `include` されます。DB スロット保持時は
`.gobp-db-slot` が既定値を上書きします（`internal/cli/dbslot/README.ja.md` 参照）。

| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `INFRA_PROJECT` | `gobp-shared` | 共有インフラの唯一のインスタンスを置く固定 compose プロジェクト。 |
| `APP_PROJECT` | `gobp-app-$(notdir $(CURDIR))` | app 層の checkout 毎 compose プロジェクト。DB スロット保持時は `SERVE_PROJECT`（`gobp-wt-N`）になります。 |
| `INFRA_SERVICES` | `database observability garage elasticmq dynamodb_local goaws` | 固定ポートでしか動けないため共有するサービス。 |
| `APP_SERVICES` | `api_server mock_auth_server` | checkout 毎に起動するサービス。 |
| `COMPOSE_INFRA` | `docker compose -p $(INFRA_PROJECT)` | infra 層向けの compose 呼び出し。 |
| `INFRA_NO_RECREATE` | worktree では `--no-recreate`、それ以外は空 | 他の checkout が使っている共有インフラのコンテナを作り直さずそのまま使います。単一 checkout では空で、compose は従来どおり定義変更へ再収束します。独立した clone を複数持つなど worktree 判定で拾えない構成では明示的に指定してください。解決は make のパース時ではなく、レシピ内の `db-slot env` が行います。 |
| `COMPOSE_APP` | `docker compose -p $(APP_PROJECT) -f docker-compose.yaml -f docker-compose.attach.yaml --profile development` | app 層向けの compose 呼び出し。`docker-compose.attach.yaml` が app サービスの接続先を `host.docker.internal` 経由の共有インフラへ差し替えます。 |
| `APP_LOG_TAIL` | `50` | API が起動できなかったとき `app-up` が出す `api_server` ログの行数。失敗が末尾より古いときに増やす。 |
| `API_HOST_PORT` / `MOCK_AUTH_HOST_PORT` | `8080` / `2010` | API / mock 認証サーバーのホスト公開ポート。 |
| `DLV_HOST_PORT` / `PPROF_HOST_PORT` | `2345` / `6060` | dlv デバッグ / pprof のホスト公開ポート。 |
| `COMPOSE_PROJECT_NAME` | `$(INFRA_PROJECT)` | `-p` を渡さない compose 呼び出しの既定プロジェクト。DB ツーリングが共有インフラのネットワークで動くようにします。 |

### Dockerfile Lint / image 固定関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make docker-lint` | `docker/*/Dockerfile` を hadolint で lint します。 | `go_tool_runner` コンテナ内で `make docker-lint-ci` を呼び出します。 |
| `make docker-lint-ci` | `hadolint docker/*/Dockerfile` を直接実行します。 | CI 用ターゲット。無効化ルールは `.hadolint.yaml`。 |
| `make compose-lint` | compose のサービス宣言を [`scripts/compose-lint`](../scripts/README.md) の規則に照らして検査します。現在の規則は「`APP_SERVICES` の各サービスが `healthcheck` を宣言していること」。 | 判定が導入した linter ではなく自前ツールなので、`migration-lint` と同じくホストで実行します（`go run`）。CI でも走ります（`compose-lint.yaml`）。 |
| `make pin-images-resolve` | registry を指す参照すべて——`FROM`、`docker-compose*.yaml` の `image:`、workflow の `uses: docker://` と `services.*.image`——を現在の digest へ解決し `docker/images-pin.toml` lockfile を更新します。 | `PIN_IMAGES_MIN_AGE_DAYS`（既定 14、0 で無効）より新しい digest は quarantine します。registry への到達（`docker`）が要ります。 |
| `make pin-images-apply` | 同じ 4 種の参照を lockfile を元に `image:tag@sha256:...` へ固定します（quarantine 中の image は tag のまま）。 | なし |
| `make pin-images-check` | 同じ 4 種の参照が lockfile 通り固定済みか検証します（書き換えなし）。`${{ }}` で組み立てた `image:` は固定できる参照ではないので対象外です。 | CI / pre-commit gate。 |

## `.makefiles/openapi` 系

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make gen-bundle-oapi` | 分割された OpenAPI 定義をバンドルし、単一の OpenAPI ファイルを生成します。 | `openapi/openapi.yaml` をもとに `openapi/openapi.gen.yaml` を生成します。 |
| `make gen-api-docs` | OpenAPI 定義をもとに API ドキュメントを生成します。 | なし |
| `make lint-oapi` | OpenAPI 定義を `redocly lint` で検証します。 | `node_tool_runner` コンテナ内で `make lint-oapi-ci` を呼び出します。 |
| `make gen-bundle-oapi-ci` | `redocly bundle` により `openapi/openapi.gen.yaml` を生成します。 | CI 用ターゲットです。 |
| `make gen-api-docs-ci` | `redocly build-docs` により `docs/openapi/index.html` を生成します。 | CI 用ターゲットです。 |
| `make lint-oapi-ci` | `redocly lint openapi/openapi.yaml` を直接実行します。 | CI 用ターゲットです。 |
| `make stamp-openapi-version` | リリースブランチ名から `info.version` を書き換えます。 | `node_tool_runner` コンテナ内で `make stamp-openapi-version-ci` を実行します。`REF=release/vX.Y.Z` を取り、未指定なら `GITHUB_REF_NAME` を使います。それ以外の ref は何もしません。 |
| `make stamp-openapi-version-ci` | `scripts/stamp-openapi-version/index.ts` を直接実行します。 | CI 用ターゲットです。 |
| `make lint-oapi-security-ci` | Spectral + OWASP API Security ルールセットで検証します。 | CI 用ターゲット。spec だけを見る検査のためにツールランナーのイメージを起こさないので、コンテナを介さず実行します。事前に `pnpm install --dir scripts --frozen-lockfile` が必要です。 |
| `make openapi-client-check` | frontend generator（orval）が bundle 済み spec から SSE の契約型（`DeliveryEvent` / `ControlEvent` / `StreamCursor`）を生成できることを確認します。 | `node_tool_runner` コンテナ内で `make openapi-client-check-ci` を呼び出します。生成物は `tmp/openapi-client/` に出し、コミットしません。 |
| `make openapi-client-check-ci` | `tsx scripts/openapi-client-check` を直接実行します。 | CI 用ターゲット。事前準備は `lint-oapi-security-ci` と同じです。 |

## `.makefiles/load` 系

ホストの CPU は有限ですが、そこへ同時にぶら下がる checkout の数は有限ではありません。複数の worktree
がそれぞれホスト全体を前提としたゲートを回すとマシンが飽和し、ゲートは「変更内容と無関係な理由」で
落ち始めます。触っていないテストがタイムアウトし、`golangci-lint` が 17 分かかり、`docker` が応答を
返さなくなる。失われるのは所要時間ではなく、**ゲートの失敗がコードについての証拠でなくなること**です。

`.makefiles/load.mk` は開いている窓の数（`git worktree list`）から重いゲートの規模を決めます。誰かが
絞ることを覚えている必要がないよう、パース時に自動で決まります。帯は 3 つです。

| 帯 | 発動条件（既定） | 挙動 |
| --- | --- | --- |
| `full` | worktree が 3 未満 | 従来どおり。ツール自身の既定値でホスト全体を使う |
| `low` | 3 以上 | 重いゲートを `CPU / 窓数` の並列度に絞り、`nice -n 10` で、かつ同時に 1 つずつ走らせる |
| `ci-first` | 5 以上 | 重いゲートはローカルで走らせない。push が CI へ運ぶ |

`ci-first` が手元に残すのは、**軽く、かつ push 後では取り返しがつかない**ゲートだけです（`commitlint`・
`secret-scan`・ピン lockfile 検査・マイグレーション番号）。落とすのは CI が同一に再実行するものだけなので、
検証されないものは生じません。検証の場所が変わるだけです。

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make load-status` | 解決された帯・窓数・CPU シェア・各ツールへ渡るフラグを表示します。 | ゲートの挙動が不審なときはまずここを見ます |
| `make gate-go` | `pre-commit` の Go ゲート（`lint` + `test-cached`）。帯が並列/逐次/委譲を決められるよう束ねてあります。 | lefthook が呼びます |
| `make gate-go-push` | `pre-push` の Go ゲート（`test` + `test-scripts`）。同じく束ねてあります。 | lefthook が呼びます |
| `make gate-heavy-skip` | lefthook の `skip:` から呼ぶ述語。exit 0 が「CI がやる」を意味します。 | 終了コードだけが interface です |
| `make gate-fix` | 自動フォーマットの委譲先です。毎回走る経路は `fix` を直接呼ばずこちらを呼びます。 | 負荷帯が `ci-first` でなければ `make fix` を実行し、`ci-first` のときは委譲した旨だけを表示します。フォーマットのずれは CI の lint が曖昧さなく捕まえられる数少ない対象であり、それがここを委譲してよい理由です。 |

帯は `GOBP_LOAD=full|low|ci-first` で明示的に上書きできます（例: 残りは委譲したまま重いゲートを 1 つだけ
手で回すなら `make lint GOBP_LOAD=low`）。閾値は `GOBP_LOW_THRESHOLD` と `GOBP_CI_FIRST_THRESHOLD` です。
これらの既定値と帯の解決そのものは `scripts/load-band` にあり、make のパース時ではなくゲートのレシピ実行時に評価されます。

**ゲートを `.lefthook.yaml` に個別に並べず束ねている理由**: lefthook はフック内の commands を並列に
走らせるため、ゲートごとにエントリを置くと、窓の数に**加えて**ゲートの数だけ負荷が乗算されます。束ねる
ことで、並列か逐次かの判断を「帯を既に知っている 1 箇所」に置けます。

絞る対象は **毎コミット・毎 push で走るゲートだけ** です。単発の重い処理（イメージビルド・コード生成・
Trivy スキャン）は放置します。ループで回すものではない以上、ホストを飽和させる原因にならないためです。

## `.makefiles/go` 系

### Go 生成関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make gen-go-code` | Go のコード生成を実行します。 | Docker コンテナ内で `go generate ./...` を実行します。 |
| `make gen-go-code-ci` | Docker を介さず、`go generate ./...` を直接実行します。 | CI 用ターゲットです。 |
| `make gen-sqlc` | SQLC のコード生成をまとめて実行します。 | `remove-generated-sqlc` → `sqlc-generate` を順に実行します。 |
| `make remove-generated-sqlc` | 既存の SQLC 生成コードを削除します。 | なし |
| `make sqlc-generate` | SQLC によるコード生成を実行します。 | なし |
| `make remove-generated-sqlc-ci` | `$(SQLC_OUT)` 配下の `*.gen.sql.go` を削除します。 | CI 用ターゲットです。 |
| `make sqlc-generate-ci` | `sqlc generate -f sqlc.yaml` を直接実行します。 | CI 用ターゲットです。 |

### Go フォーマット・Lint・依存更新関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make fmt` | Go コードをフォーマットします。 | `go fmt ./...` を実行します。 |
| `make lint` | GolangCI-Lint による静的解析を実行します。 | なし |
| `make fix` | GolangCI-Lint の自動修正を実行します。 | なし |
| `make tidy-lib` | Go モジュール依存関係を整理し、`vendor` を更新します。 | `go mod tidy` と `go mod vendor` を順に実行します。 |
| `make vendor-sync` | `vendor` が `go.mod` からずれていれば再生成します。 | Go 自身の vendor 整合検査が失敗したときだけ `go mod vendor` を実行するため、通常は何もしません。`post-merge` / `post-checkout` フックから呼ばれます。`vendor` は gitignore されているため、他人の `go.mod` 変更を受け取っただけの checkout が壊れる側になります。 |

### Go テスト関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make test` | CI 用のテストを実行します。 | `gen` / `cmd` / `mock` / `apperror` / `scripts` を除外したパッケージ群に対して `go test` を実行します（`internal/cli` コアは計測対象に含まれます）。 |
| `make test-cached` | ローカル用にテストキャッシュを有効にしてテストを実行します。 | pre-commit のローカル実行向け。除外パッケージは `test` と同じですが、`-count=1` を付けずキャッシュ結果を再利用します。 |
| `make gen-test-repo` | テストを実行し、HTML カバレッジレポートを生成します。 | 出力先は `docs/coverage/index.html` です。 |
| `make test-cover-ci` | カバレッジ付きでテストを実行します。 | CI 用ターゲットで、`coverage.out` を出力します。 |
| `make cover-gate` | 総カバレッジが閾値を下回ると fail します。 | CI ゲート。`COVERAGE_THRESHOLD`（既定 90）。`coverage.out` が必要（先に `test-cover-ci`）。 |
| `make test-scripts` | CI 用に `scripts/` 配下ツールのテストを実行します。 | `scripts/` は上記のカバレッジ対象から除外されているため、専用の実行経路が必要です。`cover-gate` の対象には入りません。`actions-shellcheck` のテストは host の `shellcheck`（`install-tools` が導入）を必要とし、無ければ自分で skip します。CI は `REQUIRE_SHELLCHECK` を立てて、その skip を失敗に変えます。 |
| `make test-fails` | `go test` のログから、成功しか報告していない行 — `ok` 行 / `[no test files]` 行 / 単独の `coverage:` 行 — をすべて落として表示します。絶対パスはリポジトリ相対へ縮め、`gh run view --log` が各行へ付ける `<job>/<step>/<timestamp>` 接頭辞も剥がします（1 行が短くなるうえ、接頭辞に潰されていた行頭アンカーが復活します）。`LOG=` でログを指定でき（既定は `make ai-test` が残す `tmp/ai-logs/test.txt`）、`LOG=-` は標準入力を読むため、`gh run view --log-failed \| make test-fails LOG=-` で CI のログにも同じ絞り込みを当てられます。 | `coverage:` 行こそがこのターゲットの存在理由です。`test-cover-ci` と `gen-test-repo` は対象パッケージ全部を `-coverpkg` に渡すため、`go test` はカバレッジを報告するたびにそのリスト全体を複製します。結果として 1 回の失敗 run が数 MB を出力し、そのなかで失敗は数十バイトしかありません。捨てても失うものはありません — `cover-gate` が読むのは標準出力ではなく `coverage.out` です。許可リストではなく拒否リストなので、落とすのは「通った」としか言っていない行だけであり、想定外の panic / ビルドエラー / race レポートはそのまま通ります。元のログもディスクに残ります。ログを読むだけで、何も実行しません。 |
| `make test-scripts-cached` | ローカル用にテストキャッシュを有効にして `scripts/` 配下ツールのテストを実行します。 | pre-commit のローカル実行向け。対象パッケージは `test-scripts` と同じで、`-race -count=1` は付けません。 |
| `make cover-scripts` | `scripts/` 配下ツールの総カバレッジを計測し、`SCRIPTS_COVERAGE_THRESHOLD` を下回ったら警告します。 | 失敗させず警告に留めます（`-warn`）。開発ツールのカバレッジが出荷物のマージを止めないためです。Actions 上では `-github` が付きます。プロファイルは実行後に削除されます。 |
| `make build-scripts` | `scripts/` 配下のツールを `scripts/bin/` へビルドします。 | `-o scripts/bin/` を固定しているのは、リポジトリ直下で `go build ./scripts/<tool>` を叩くとパッケージ名の実行ファイルが数十 MB のまま追跡対象外でルートに落ちるためです。出力先は gitignore 済みです。 |

### Go ツールインストール関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make go-update` | `mise.toml` に記載された Go ランタイムを mise でインストールします。詳細は `docs/maintenance/go-upgrade.md` を参照。 | mise が必須 |
| `make install-tools` | host 開発用のツール群を mise でインストールします（バージョンは `mise.toml` から解決）。 | `gopls`、`gotests`、`impl`、`dlv`、`lefthook`、`golangci-lint`、`zizmor`、`shellcheck` を導入します。`golangci-lint` と `zizmor` は、Alpine の tool-runner 向け musl ビルドが無いため pre-commit フックがホストで実行するツールです。`shellcheck` は、フックの `test-scripts` がホストで走らせる `actions-shellcheck` のテストが実物のバイナリを呼ぶためです。 |
| `make activate-tools` | `lefthook install` を実行し、Git フックをセットアップします。 | なし |
| `make sync-versions` | `mise.toml` の go / node / python バージョンを `go.mod` と Dockerfile の `FROM` に反映します。 | `docs/maintenance/go-upgrade.md` の手順で参照されます。`scripts/sync-versions` を実行します。 |

## `.makefiles/node` 系

リポジトリの補助スクリプトは TypeScript で、`scripts/node_modules/.bin` の `tsx` 経由で実行します。
判定ロジックを `scripts/lib/**` に置いてあるのは、検査対象のリポジトリ無しでテストできるようにするためです。
これらの一部はゲートであり、壊れたときはエラーではなく「違反なし」を報告する向きに倒れます。

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make scripts-test` | `scripts/**/*.ts` の単体テストをカバレッジ付き・キャッシュ無効で実行します。 | `node_tool_runner` コンテナ内で `make scripts-test-ci` を実行します。`scripts/vitest.config.mts` の閾値を下回ると失敗します。 |
| `make scripts-test-cached` | 同じテストをキャッシュ有効・カバレッジ無しで実行します。 | `node_tool_runner` コンテナ内で `make scripts-test-cached-ci` を実行します。`pre-push` 向けの系統です。 |
| `make scripts-typecheck` | `scripts/**/*.ts` の型検査を実行します。 | `node_tool_runner` コンテナ内で `make scripts-typecheck-ci` を実行します。 |
| `make scripts-test-ci` | `pnpm --dir scripts run test`（`vitest run --coverage --no-cache`）を実行します。 | CI 用ターゲットです。 |
| `make scripts-test-cached-ci` | `pnpm --dir scripts run test:cached`（`vitest run`）を実行します。 | CI 用ターゲットです。 |
| `make scripts-typecheck-ci` | `pnpm --dir scripts run typecheck`（`tsc --noEmit`）を実行します。 | CI 用ターゲットです。 |

## `.makefiles/python` 系

このリポジトリが PyPI から入れる CLI ツールは `python/*.in` で宣言し、パッケージごとの sha256 付きで `python/*.txt` に固定します（[ADR-0084 (mise-ssot-drift-gate)](../docs/adr/0084-mise-ssot-drift-gate.md)）。ここのターゲットはその lockfile を再生成するものです。`.in` から直接 install する経路はありません。

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make py-lock` | `python/*.in` から `python/*.txt` をすべて再生成します。 | `python_tool_runner` コンテナ内で `make py-lock-ci` を実行します。pin を変えたら実行し、両方のファイルをコミットしてください。 |
| `make py-lock-ci` | 宣言ごとに `uv pip compile --generate-hashes --universal` を実行します。解決の対象は `mise.toml` が宣言する Python のバージョンです。 | CI 用ターゲットです。 |

## `.makefiles/agents` 系

アプリケーションではなく AI 支援開発そのものに奉仕するターゲット群です。Closed Loop のフィードバックサイクルと、[命名規約](#命名規約)で述べた `ai-` ターゲットが書き込むログディレクトリを扱います。

打刻は `.agents/closed-loop/marks.sh` が hook / スキル / git フックから行い、ここのターゲットは読む側と送る側です。コンテナ実行なのは `closed-loop-report` だけで、理由はそれぞれの見に行く先が違うことにあります。report が読む打刻はリポジトリ内にあり既にマウントされていますが、`send` が読むトランスクリプトは利用者のホーム配下にあり、`weekly` が叩く `gh` は利用者の認証を要します。どちらもコンテナからは届きません。

| コマンド | 説明 | 備考 |
| --- | --- | --- |
| `make closed-loop-report` | 打刻された開発の窓について、フェーズ区間と異常を報告します。 | `node_tool_runner` コンテナ内で `make closed-loop-report-ci` を呼びます。 |
| `make closed-loop-send` | 閉じたが未送出の窓を Feedback Issue へ送ります。 | ホスト上で `.agents/closed-loop/send.sh` を実行します。 |
| `make closed-loop-send-dry` | 送出せず、送る内容だけを表示します。 | `send.sh --dry-run`。 |
| `make closed-loop-weekly` | 期間内の Feedback Issue を集計し、そこから挙がる検討課題を並べます。期間は `FROM=` / `TO=` で指定します。 | ホスト実行。利用者の認証で `gh` を叩くためです。 |
| `make closed-loop-report-ci` | `tsx scripts/closed-loop` を直接実行します。 | CI 用ターゲット |
| `make clean-ai-logs` | `tmp/ai-logs/` を削除します。`make ai-<target>` がコンソールへ出さずに退避した出力の置き場です。 | このディレクトリは gitignore 済みで worktree ごとに独立するため、影響は実行したチェックアウトに限られます。 |

## `.makefiles/graphify` 系

[graphify](https://github.com/graphify/graphify) はリポジトリを `graphify-out/` 配下のナレッジグラフへ変換します。このディレクトリには 2 種類のファイルが混ざります。どのチェックアウトでも同じ意味を持つ成果物と、生成したマシン上でしか意味を持たない状態です。抽出キャッシュは渡されたパスからノード ID を組み立てるため、絶対パスでの実行はホストのユーザー名を ID へ焼き込みます。そのため `.gitignore` は無視する対象を列挙するのではなく成果物をホワイトリストで許可しており、ここのターゲット群がその 2 つを分けて扱います。

セマンティックキャッシュは `cache/` のうち唯一共有される部分です。キーが内容ハッシュなので、フルリビルド時に LLM 抽出をやり直さずに済みます。一方でキーになって*いない*のが、その内容を生んだ抽出プロンプトです。プロンプトは graphify に同梱されるため、ビルドをピン留めすることが「誰がたまたま実行したか」ではなく `python/graphify.in` の性質へと変えます。決定的な側は `python_tool_runner` イメージが、抽出ワークフローは lockfile を指した `UV_CONSTRAINT` が担います（後者のスキルは、放っておくと PyPI の最新リリースから自前のインタプリタを解決してしまいます）。[`.agents/graphify/spec-pin.toml`](../.agents/graphify/spec-pin.toml) が結果のフィンガープリントを記録するので、CI は graphify を一切インストールせずにコミット済みキャッシュを検査でき、`graphify-check` はそれ以外で焼かれたキャッシュを拒否します。

ビルドはモデルを要するかどうかで分かれ、2 つの半分は別々の場所で生成されます。`graphify update` は変更されたコードを tree-sitter で再抽出するもので、決定的であり、リリースラインで無人実行されます（`graphify-sync.yaml`）。ドキュメントに対するセマンティック抽出はモデルを要しアシスタントが駆動するため手動です。`Graphify Extract` ワークフローをディスパッチしてください。これが正規の経路で、リポジトリ自身のトークンで走ります。

ローカルで走らせるのは既定ではなく逃げ道です。抽出の実体は自分のアシスタントセッションでの `/graphify --update` であり、プロジェクトではなく*あなたの*プラン枠を消費します。make はアシスタントのコマンドを呼び出せないので、そのための make ターゲットは意図的に存在しません。ローカル実行が更新するのは自分の作業コピーだけで、`graphify-check BASE=<ref>` が出力を抱えたフィーチャーブランチを弾くため、共有グラフはリリースラインから来続けます。マニフェストがファイルごとに `ast_hash` と `semantic_hash` を持つのはまさにこのためで、決定的な側は何度走ってもセマンティック側に判を押しません。保留中のドキュメント作業は隠されるのではなく積み上がります。`make graphify-pending` はその蓄積を読み、ファイル数ではなく変更行数で報告します。誤字修正と書き直された ADR とでは、再抽出の手間が同じではないからです。

| コマンド | 説明 | 備考 |
| --- | --- | --- |
| `make graphify-update` | 内容が変わったコードファイルを `graph.json` へ再抽出します。決定的でモデルを必要としません。 | `python_tool_runner` コンテナ内で `make graphify-update-ci` を呼びます。リリースライン上のグラフは `graphify-sync.yaml` が更新するため、ローカル実行は自分の作業コピーを最新に保つためのものです。 |
| `make graphify-export` | `graphify-out/graph.json` を決定的な `nodes.json` / `edges.json` / `metadata.json` の 3 点へ変換します。 | `go_tool_runner` コンテナ内で `make graphify-export-ci` を呼びます。 |
| `make graphify-check` | ホワイトリスト外のファイルが追跡されていないこと、追跡成果物が生成マシンの絶対パスを抱えていないこと、追跡されたセマンティックキャッシュが `.agents/graphify/spec-pin.toml` のピンする抽出プロンプトに属することを検証します。 | ホストで実行します。対象がツールチェインではなく git インデックスだからです。`pre-commit` フックと `Graphify Check` ワークフローから呼ばれます。 |
| `make graphify-check SPEC=<path>` | 指定された `extraction-spec.md` のフィンガープリントも取り、ピンと異なれば失敗します。 | 抽出の前に実行します。プロンプトはアシスタントのスキルディレクトリにあり、`bootstrap-external-skills.sh` がチェックアウトへ書き込むため、抽出ワークフローもそこで検証します。 |
| `make graphify-check BASE=<ref>` | `<ref>` との差分が出力ディレクトリに触れている場合も失敗します。 | プルリクエストのゲートが使います。グラフは単一の blob で並行ブランチ間で衝突するため、その更新はリリースラインに留めます。 |
| `make graphify-pending` | セマンティック抽出がどれだけ待っているかを報告します。前回の抽出以降に変更されたドキュメントと、その変更行数です。 | 報告のみで、失敗させることはありません。しきい値は `GRAPHIFY_PENDING_THRESHOLD`（既定 3000 変更行。約 92,500 行のドキュメント群に対しておよそ 3%）で、`/graphify --update` を走らせる判断のために存在します。これはここのどのワークフローも起動できないモデルを必要とします。件数とファイル別の内訳は毎回必ず表示されるため、小さくとも重要な書き換えはしきい値以下でも見えます。 |
| `make graphify-update-ci` | `graphify update .` を直接実行します。 | CI 用ターゲット。スキャンのルートを明示的に渡しているのは、渡さないと graphify が `graphify-out/.graphify_root` から復元してしまうためです。これはホストのパスを保持しており、追跡対象ではありません。 |
| `make graphify-export-ci` | `go run ./scripts/graphify-export` でエクスポートを直接実行します。 | CI 用ターゲット |

## `.makefiles/docs` 系

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make gen-portal-docs` | Portal 用ドキュメントを生成します。 | なし |
| `make gen-docs-json` | Portal 用ドキュメントリンク JSON を生成します。 | なし |
| `make gen-portal-build` | Portal フロントエンド（`docs-viewer/`）を Vite で `docs/portal/` へビルドします。 | なし |
| `make portal-test` | `docs-viewer/` のテストを実行します。 | なし |
| `make portal-typecheck` | `docs-viewer/` の型検査を実行します。 | なし |
| `make gen-portal-docs-ci` | Node.js スクリプトで Portal 用ドキュメントを直接生成します。 | CI 用ターゲットです。 |
| `make gen-docs-json-ci` | Node.js スクリプトで Portal 用 JSON を直接生成します。 | CI 用ターゲットです。 |
| `make gen-portal-build-ci` | pnpm を直接実行して Portal フロントエンドをビルドします。 | CI 用ターゲットです。 |
| `make portal-test-ci` | pnpm を直接実行して Portal フロントエンドのテストを実行します。 | CI 用ターゲットです。 |
| `make portal-typecheck-ci` | pnpm を直接実行して Portal フロントエンドの型検査を実行します。 | CI 用ターゲットです。 |
| `make gen-godoc` | godoc の静的 HTML を `docs/godoc/` に生成します。 | なし |
| `make gen-godoc-ci` | godoc-static を直接実行して静的 HTML を生成します。 | CI 用ターゲットです。 |

## `.makefiles/gen` 系

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make gen` | 各種コード・ドキュメント生成をまとめて実行します。 | `gen-api` → `gen-query` → `gen-docs` を順に実行します。 |
| `make gen-api` | API 関連の生成処理をまとめて実行します。 | `gen-bundle-oapi` →  `gen-api-docs` → `gen-go-code` を実行します。 |
| `make gen-docs` | ドキュメント関連の生成処理をまとめて実行します。 | `gen-api-docs`、`gen-portal-docs`、`gen-docs-json` を実行します。 |
| `make gen-all-docs` | すべてのドキュメント生成処理を実行します。 | `gen-docs`、`gen-db-schema`、`gen-test-repo` を実行します。 |
| `make gen-query` | SQLC コード生成をまとめて実行します。 | `dump-schema` → `merge-dml` → `gen-sqlc` → `fmt` を順に実行します。 |
| `make gen-query-repo` | Repository 用 SQLC コード生成を実行します。 | `dump-schema` → `merge-dml-repo` → `gen-sqlc` を実行します。 |
| `make gen-query-qs` | Query Service 用 SQLC コード生成を実行します。 | `dump-schema` → `merge-dml-qs` → `gen-sqlc` を実行します。 |
| `make gen-query-sysq` | System Query 用 SQLC コード生成を実行します。 | `dump-schema` → `merge-dml-sysq` → `gen-sqlc` を実行します。 |

## `.makefiles/github` 系

### GitHub Actions lint / pin 関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make actions-lint` | ワークフロー定義を actionlint で lint し、全 composite action の `run:` スクリプトを shellcheck で検査したうえで、node による 3 つの検査（PR コメントを投稿するジョブへの secret 混入、コメント本文の固定長フェンス、ジョブ打ち切り時の振る舞いが定義されているか）を実行します。 | 各段が 2 つの tool-runner に跨る唯一の lint グループ。actionlint と shellcheck ランナーは Go ツール、残りは node スクリプトのため、単一コンテナ内で 1 つの `-ci` を呼ぶのではなく `go_tool_runner` で `make actions-actionlint-ci` と `make actions-shellcheck-ci`、`node_tool_runner` で `make actions-node-lint-ci` を呼び出します。node の 3 検査は 1 ターゲットに束ねてコンテナ起動 1 回に収めており、これは `md-lint` と同じ形です。 |
| `make actions-comment-secret-lint` | PR コメント本文への secret 混入検査のみを実行します。 | `node_tool_runner` コンテナ内で `make actions-comment-secret-lint-ci` を呼び出します。 |
| `make actions-comment-fence-lint` | PR コメント本文の固定長フェンス検査のみを実行します。 | `node_tool_runner` コンテナ内で `make actions-comment-fence-lint-ci` を呼び出します。 |
| `make actions-cutoff-lint` | ジョブ打ち切り時の振る舞い検査のみを実行します。 | `node_tool_runner` コンテナ内で `make actions-cutoff-lint-ci` を呼び出します。 |
| `make actions-shellcheck` | `.github/actions/**` の composite action から `runs.steps[].run` を抽出し、`bash` / `sh` のスクリプトを `shellcheck` で検査します。それ以外の shell のステップは skip として報告します（`scripts/actions-shellcheck`）。 | `go_tool_runner` コンテナ内で `make actions-shellcheck-ci` を呼び出します。`actionlint` は `.github/workflows` しか走査せず、`action.yaml` を直接渡すとワークフローとして解釈するため、その死角を埋めます。`run:` をブロック折り畳み（`>`）で書いた場合はエラーになります（リテラル `\|` で書いてください）。折り畳みは指摘の位置を写し戻す基準である改行を落とすためです。 |
| `make shell-lint` | リポジトリ内のすべての `*.sh` を shellcheck で検査します。 | `go_tool_runner` コンテナ内で `make shell-lint-ci` を呼びます。 |
| `make actions-mise-pin-lint` | `setup-mise` の版 / digest / キャッシュキーが互いに整合しているかを検査します。 | `node_tool_runner` コンテナ内で `make actions-mise-pin-lint-ci` を呼びます。 |
| `make required-check-lint` | Ruleset の required context を報告する job と、それを起動する `pull_request` の条件を検査します。 | `node_tool_runner` コンテナ内で `make required-check-lint-ci` を呼びます。 |
| `make actions-lint-ci` | actionlint、composite action の shellcheck、束ねた node 検査をこの順で直接実行します。 | CI 用ターゲット。actionlint を先に置くのは意図的で、node 側の検査はワークフロー構造を桁で読むため、入力がそもそも YAML としてパースできることに依存します。 |
| `make actions-node-lint-ci` | node の 3 検査（secret / フェンス / 打ち切り）を直接実行します。 | CI 用ターゲット。 |
| `make actions-actionlint-ci` | `actionlint` を直接実行します。 | CI 用ターゲット。 |
| `make actions-shellcheck-ci` | `scripts/actions-shellcheck` を直接実行します。 | CI 用ターゲット。 |
| `make actions-comment-secret-lint-ci` | `upsert-pr-comment` を使うジョブに `GITHUB_TOKEN` 以外の secret が渡っていれば失敗します（`scripts/pr-comment-secret-lint/index.ts`）。 | CI 用ターゲット。規約の理由は [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) を参照。 |
| `make actions-comment-fence-lint-ci` | `run:` ブロックが PR コメント本文を固定長 Markdown フェンスで囲んでいる場合、または複製された `fence_for` の実装が食い違う場合に失敗します（`scripts/pr-comment-fence-lint/index.ts`）。 | CI 用ターゲット。規約の理由は [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) を参照。 |
| `make actions-cutoff-lint-ci` | ジョブに `timeout-minutes` が無い場合、または `upsert-pr-comment` を呼ぶステップの `if:` がキャンセルされたジョブから到達できない場合に失敗します（`scripts/actions-cutoff-lint/index.ts`）。 | CI 用ターゲット。規約の理由は [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) を参照。 |
| `make shell-lint-ci` | `go run ./scripts/shell-lint` を直接実行します。 | CI 用ターゲット |
| `make actions-mise-pin-lint-ci` | `tsx scripts/actions-mise-pin-lint` を直接実行します。 | CI 用ターゲット |
| `make required-check-lint-ci` | `tsx scripts/required-check-lint` を直接実行します。 | CI 用ターゲット |
| `make pin-actions-resolve` | 各 `uses:` のタグを commit SHA に解決し `.github/actions-pin.toml` lockfile を更新します。 | `PIN_ACTIONS_MIN_AGE_DAYS`（既定 14・0 で無効）より新しい解決先を quarantine。 |
| `make pin-actions-apply` | lockfile を元に `uses:` を `@<sha> # <tag>` へ固定します。 | なし |
| `make pin-actions-check` | `uses:` が lockfile 通り固定済みか検証します（書き換えなし）。 | CI / pre-commit ゲート。 |
| `make egress-apply` | `.github/egress.toml` を各ジョブのインライン `allowed-endpoints` ブロックへ反映します。 | クラスの意味と追加手順は [`.github/workflows/README.ja.md`](../.github/workflows/README.ja.md) の「ランナーのハードニング」節。 |
| `make egress-check` | 各インライン `allowed-endpoints` が SSOT 通りか検証します（書き換えなし）。 | CI / pre-commit ゲート。 |

### コミットメッセージ Lint 関連

| コマンド | 説明 | 備考 |
| --- | --- | --- |
| `make commitlint COMMIT_MSG_FILE=<file>` | コミットメッセージを commitlint で検証します。 | `node_tool_runner` コンテナ内で `make commitlint-ci` を呼び出します。`commit-msg` フックに配線。`git worktree` では git がフックへ渡すパスがコンテナのマウント範囲 `.:/app` の外にあるため、メッセージファイルを `tmp/` へ写して相対パスで渡します。`COMMIT_MSG_FILE` 既定は `git rev-parse --git-path COMMIT_EDITMSG`。 |
| `make commitlint-ci COMMIT_MSG_FILE=<file>` | `commitlint --edit <file>` を直接実行します。 | CI 用ターゲット。 |
| `make commitlint-range-ci COMMITLINT_FROM=<ref> COMMITLINT_TO=<ref>` | 範囲内の全コミットのメッセージを commitlint で検証します。 | CI 用ターゲットであり、`commit-msg` フックをバイパスして作られたメッセージに届く唯一の経路です。いずれかの ref が未指定のとき、および範囲が空のときは exit 2 で落ちるため、参照解決が壊れた状態が「合格」として通ることはありません。`node_tool_runner` 経由のラッパーはありません。コンテナのマウントは `.:/app` だけで `git worktree` の gitdir はその外にあり、履歴はメッセージファイルのように写して渡せないためです。 |

### GitHub 設定関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make gh-login` | `gh` コマンドで GitHub にログインします。 | ブラウザ認証方式でログインを行います。 |
| `make delete-all-labels` | GitHub リポジトリ上の既存ラベルをすべて削除します。 | なし |
| `make create-default-labels` | `.github/settings/labels.json` をもとに、デフォルトラベルを作成します。 | なし |
| `make apply-branch-protection` | `.github/settings/branch-protection.json` をもとに、対象リポジトリへブランチルールセットを適用します。 | 一方向の適用です。適用後に JSON を再適用する仕組みも実ルールセットと突き合わせる仕組みも無いため、このファイルが表すのは強制されている状態ではなく意図です。`.github/settings/README.ja.md` を参照してください。 |
| `make enable-workflows` | `disabled_fork` 状態のワークフローを一括で有効化します。 | 冪等です。新規に作成されたリポジトリは全ワークフローが無効の状態で始まります。 |

### GitHub リポジトリ初期化関連

#### `make setup-repo`

リポジトリの初期化処理をまとめて実行します。
以下を順に行います。

- `gh` ログイン
- 初期タグ `v0.0.0` の作成と push
- `develop` / `staging` / `production` ブランチの作成
- GitHub デフォルトブランチの設定
- ブランチルールセット適用
- ラベル初期化

`git` / `gh` を使う部分は `scripts/repo-setup`（`preflight` / `bootstrap` /
`prune-release-notes`）が担い、ラベル・ルールセット・ワークフローの各手順は個別の `make`
ターゲットのまま残しているため、このターゲットは両者の連鎖です。

新規リポジトリを立ち上げる際の初期セットアップ用コマンドです。

#### セットアップ補助コマンド

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make setup-replace-module OLD_MODULE=<old> NEW_MODULE=<new>` | Go モジュール名を一括置換します。 | `node_tool_runner` を使用して `go.mod` や import パスを更新します。  <!-- setup-localize:line --> |
| `make setup-replace-app-metadata APP_NAME=<name> OPENAPI_TITLE=<title> COPILOT_TITLE=<title>` | アプリケーション名や OpenAPI タイトルなどのメタデータを一括置換します。 | README や OpenAPI 定義などに反映されます。  <!-- setup-localize:line --> |
| `make setup-replace-repository-reference REPOSITORY=<org/repo>` | リポジトリ参照（GitHub URL など）を一括置換します。 | README やドキュメント内のリンクを更新します。  <!-- setup-localize:line --> |
| `make setup-replace-license-copyright COPYRIGHT_HOLDER=<name> [COPYRIGHT_YEAR=<year>]` | LICENSE の著作権表記を更新します。 | 年は省略可能です。  <!-- setup-localize:line --> |
| `make setup-replace-codeowners OWNERS='<owners>'` | `.github/CODEOWNERS` の全ルールの所有者を一括置換します。 | `@user` / `@org/team` / メールアドレスを指定でき、空白区切りで複数指定できます。コメント行は対象外なので、ヘッダーの記載例は書き換わりません。  <!-- setup-localize:line --> |
| `make setup-verify` | 初期化が当たったことを検証し、通れば初期化ツールを撤去します。 | `node_tool_runner` で `scripts/setup/verify-setup` を実行します。Phase 5 の値を環境変数で渡します。  <!-- setup-localize:line --> |
| `make setup-remove-boilerplate-identity` | ボイラープレートである間だけ成り立つ記述を削除します。 | `node_tool_runner` でリポジトリを走査して `boilerplate-only` マーカーをすべて解決し、ボイラープレート限定の規約ドキュメントを削除したうえで、ツール自身も撤去します。`DRY_RUN=1` でプレビューできます。 <!-- boilerplate-only:line --> |
| `make setup-remove-sample-api` | サンプルAPI(`user`/`product`/`order`)を一括削除します。 | `node_tool_runner` で削除後、`db-local-reinit` / `db-test-reinit` → `gen-api` → `gen-query` → `tidy-lib` → `fix` → `lint` を実行します。DB 再構築により削除済みテーブルが生成モデルに残らず、`tidy-lib` によりサンプルAPIだけが使っていた直接依存が go.mod から落ちます。**DB コンテナ(`database`)の起動が必要**（`gen-query` がライブスキーマをダンプ）。`DRY_RUN=1` で変更せずプレビューできます（`0` を含む空でない値はすべてプレビュー扱いになるため、実行時は変数自体を付けません）。 <!-- sample-api:line --> |
| `make setup-remove-doc-language` | ドキュメント / スキルの対訳ペアを `LANG_CHOICE` で解決します。`en` / `ja` はその 1 言語へ畳み、`both` は対訳を残してマーカーだけを解決します。 | ツールランナーを経由せずホストで実行します（撤去を 1 コミットに畳むためホストの git が要る）。他のすべての撤去より**先**に実行してください。各撤去ツールは正本と対訳の対で宣言を持ち、畳みが解決した時点で自分の対の宣言を刈ります。逆に Phase 12 はこのツールが文字列を宣言している workflow を削除するため、その後に畳もうとすると中止します。`DRY_RUN=1` でプレビューできます（作業ツリーが汚れていても動作し、実行時はクリーンが必要）。 <!-- lang-choice:line --> |
| `make setup-remove-licensed-scanners` | 資格情報または課金を要するスキャナ 2 件を撤去し、製品ごとにコミットします。 | `SETUP_DRY_RUN_FLAG` を渡すと、書き込まずに撤去内容だけを報告します。 |

### ベースブランチ解決関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make base-branch` | 最新のリリースライン(`release/vX.Y.Z`)のブランチ名を 1 行で出力します。 | `git ls-remote` で `origin` の実状態を読むため(`scripts/base-branch`)、`git fetch` では更新されないローカルの `refs/remotes/origin/HEAD` が古くても、GitHub のデフォルトブランチが前のリリースラインを指したままでも答えは変わりません。「最新」の定義はコミット日時ではなくバージョン番号の数値比較で、その理由はパッケージコメントにあります。出力は装飾を持たないので `$(make base-branch)` でそのまま受けられます。プルリクエストが既にある場合はその `baseRefName` が正で、これはその fallback です。 |

### リリースブランチ関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make hotfix-patch` | `production` から hotfix ブランチを作成し、GitHub のデフォルトブランチに設定します。 | 現在の最新タグを基準に patch を 1 つ進めます。`scripts/release branch` を実行し、`origin` に同名ブランチが既にある場合や作業ツリーが汚れている場合は中止します。 |
| `make branch-patch` | `production` から patch リリース用ブランチを作成し、デフォルトブランチに設定します。 | 現在の最新タグを基準に patch バージョンを進めます。`scripts/release branch` を実行します。 |
| `make branch-minor` | `production` から minor リリース用ブランチを作成し、デフォルトブランチに設定します。 | 現在の最新タグを基準に minor バージョンを進めます。`scripts/release branch` を実行します。 |
| `make branch-major` | `production` から major リリース用ブランチを作成し、デフォルトブランチに設定します。 | 現在の最新タグを基準に major バージョンを進めます。`scripts/release branch` を実行します。 |

### リリースタグ関連

| コマンド | 説明 | 補足 |
| --- | --- | --- |
| `make tag-patch` | patch バージョンを 1 つ進めたタグを作成し、GitHub Release を作成します。 | 現在の最新タグを基準とし、リリースノートには `.github/release/<version>.md` を使用します。`scripts/release tag` を実行します。このファイルを探す**前に** `production` を `origin` へ同期します（タグは `production` HEAD に打つため、ノートはそちらに在る必要があります）。 |
| `make tag-minor` | minor バージョンを進めたタグを作成し、GitHub Release を作成します。 | 現在の最新タグを基準にします。`scripts/release tag` を実行します。 |
| `make tag-major` | major バージョンを進めたタグを作成し、GitHub Release を作成します。 | 現在の最新タグを基準にします。`scripts/release tag` を実行します。 |
