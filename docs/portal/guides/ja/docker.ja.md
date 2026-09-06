# docker

`docker/` は、開発・ビルド・運用に必要な **Docker 関連の設定ファイル**を格納するディレクトリです。

## ディレクトリ構成

イメージまたはサービスごとに 1 つのディレクトリを置き、その名前を付ける。そのユニットの Dockerfile と、
ビルド時・実行時に必要な設定を収める。

## compose の階層構成（infra / app）

compose のサービスは 2 層に分かれており、主 checkout と任意個数の worktree を同時に起動できます。

|層|compose プロジェクト|サービス|
|---|---|---|
|**infra**|`gobp-shared`（固定名）— **全 checkout で 1 インスタンス**|`database` / `observability` / `garage`（+ `garage_init`）/ `elasticmq` / `dynamodb_local`（+ `dynamodb_init`）/ `goaws`。補助サービスとツールランナーも `COMPOSE_PROJECT_NAME` の既定により同じプロジェクトで動く|
|**app**|`APP_PROJECT` — checkout 毎（`gobp-app-<ディレクトリ名>`、DB スロット保持時は `gobp-wt-N`）|`api_server` / `mock_auth_server`|

infra 層には固定ポートでしか動けないサービスだけが属し、そのためホスト上に 1 つだけ存在します。

|ファイル|役割|
|---|---|
|`docker-compose.yaml`|全サービスの定義|
|`docker-compose.attach.yaml`|app 層の override。**常に**重ねて適用する（`docker compose -f docker-compose.yaml -f docker-compose.attach.yaml`）|

`docker-compose.attach.yaml` は `api_server` のホスト公開ポートを `${API_HOST_PORT:-8080}` / `${DLV_HOST_PORT:-2345}` / `${PPROF_HOST_PORT:-6060}` にし、`depends_on` を `mock_auth_server` だけに絞り（infra 層は起動済みのため）、`DB_HOST=host.docker.internal` / `DB_NAME=${DB_NAME_LOCAL:-local}` / `ENDPOINT_OTLP=http://host.docker.internal:4318` / `ENDPOINT_OBJECT_STORAGE=http://host.docker.internal:3900` / `AUTH_ISSUER=http://localhost:${MOCK_AUTH_HOST_PORT:-2010}/default` の上書きで共有インフラを参照させます。プロバイダ側に対応する上書きは要りません——issuer は到達した `Host` から導出されるためです。

|目的|参照先|
|---|---|
|層の変数（`INFRA_PROJECT` / `APP_PROJECT` / `COMPOSE_INFRA` / `COMPOSE_APP`）|[`.makefiles/docker/compose.mk`](../.makefiles/docker/compose.mk)|
|ローカル開発環境の全体像|[`docs/maintenance/local-environment.md`](../docs/maintenance/local-environment.md)|
|ホスト公開ポート / DB 名のスロット割当|[`docs/maintenance/db-worktree-pool.md`](../docs/maintenance/db-worktree-pool.md)|

## サービス概要

docker-compose.yaml で定義されるサービスと、対応する Dockerfile / 設定の対応表です。

### 開発環境（profile: `development`）

|サービス|層|Dockerfile / Image|ポート|説明|
|---|---|---|---|---|
|`api_server`|app|`docker/server/Dockerfile` (target: `tooling`)|`${API_HOST_PORT:-8080}`, `${DLV_HOST_PORT:-2345}`, `${PPROF_HOST_PORT:-6060}`|開発用APIサーバ（air によるホットリロード）|
|`mock_auth_server`|app|`ghcr.io/navikt/mock-oauth2-server`|`${MOCK_AUTH_HOST_PORT:-2010}`|疑似 OIDC プロバイダ（開発専用）|
|`database`|infra|`postgres:18.4-trixie`|5432|PostgreSQL データベース|
|`observability`|infra|`grafana/otel-lgtm`|3000, 4317, 4318, 3200|ローカル o11y 検証用の可観測性スタック（OTLP 送出口 / Grafana）|
|`garage`|infra|`dxflrs/garage`|3900, 3902|S3 互換オブジェクトストレージ（S3 API / Web API）|
|`garage_init`|infra|`docker/garage/Dockerfile`|-|garage のレイアウト / バケット / アクセスキー / 公開配信の許可を one-shot でプロビジョニング（冪等）|
|`elasticmq`|infra|`softwaremill/elasticmq-native`|9324|SQS 互換のメッセージブローカー（ローカル開発用。テストは in-process の fake を使う）|
|`dynamodb_init`|infra|`amazon/dynamodb-local`|-|`dynamodb_data` volume の所有者を合わせ `dynamodb_local` を非 root のまま動かすための one-shot（冪等。`depends_on` で順序付け）|
|`dynamodb_local`|infra|`amazon/dynamodb-local`|8000|Realtime Delivery（EventLog / StreamTicket / InstanceLease）向けの DynamoDB 互換ストア（ローカル開発用。`-sharedDb`、`dynamodb_data` volume に永続化）|
|`goaws`|infra|`admiralpiett/goaws`（設定: `docker/goaws/goaws.yaml`）|4100|Realtime Delivery の instance fan-out 専用の SNS / SQS エミュレータ。Worker 側の outbox queue は `elasticmq` のまま|

DB スロットでずれるのは app 層のホスト公開ポート（`8080+N` / `2010+N` / `2345+N` / `6060+N`）だけで、コンテナ内部のポートは常に固定です。

### 補助サービス（profile: `tools`・infra 層）

|サービス|Dockerfile / Image|ポート|説明|
|---|---|---|---|
|`docs_server`|`docker/document/Dockerfile`|2001|ドキュメントポータル（nginx）|
|`sql_editor`|`sosedoff/pgweb`|2000|Web SQL エディタ|

### ツールランナー（profile: `generate`・infra 層）

|サービス|Dockerfile / Image|説明|
|---|---|---|
|`go_tool_runner`|`docker/tools/Dockerfile` (target: `go_tools`)|Go のコード生成 / lint / セキュリティ / ドキュメント生成ツール（[一覧](tools/README.ja.md#go_tools)）|
|`node_tool_runner`|`docker/tools/Dockerfile` (target: `node_tools`)|OpenAPI バンドル、Markdown / コミットの lint、ポータルのビルド（[一覧](tools/README.ja.md#node_tools)）|
|`python_tool_runner`|`docker/tools/Dockerfile` (target: `python_tools`)|SQL の lint（[一覧](tools/README.ja.md#python_tools)）|
|`er_diagram_generator`|`schemaspy/schemaspy`|ER図生成（SchemaSpy）|

## server

アプリケーションサーバのイメージです。1 つのマルチステージ Dockerfile が 3 つのターゲットを作ります。`builder`（Go バイナリ）、`runtime`（本番用・非 root。マイグレーションもこの同一イメージを command override で実行するため専用イメージは持たない）、`tooling`（ローカルのホットリロード開発）。ベースイメージと各ターゲットが持つツールは [`server/README.ja.md`](server/README.ja.md) を参照してください。

## tools

コード生成と lint 用のツールコンテナです。言語ごとに `go_tools` / `node_tools` / `python_tools` の 3 ステージに分かれています。各ステージが持つツールと用途は [`tools/README.ja.md`](tools/README.ja.md) を参照してください。

## document

ドキュメントポータル用のコンテナです。

- ベースイメージ: `nginx:1.31-alpine`
- `docs/` ディレクトリ全体をボリュームマウント
- ポータルアプリは `/portal/` で提供
- `http://localhost:2001/` にアクセスすると `/portal/` にリダイレクト

## garage

ローカル開発用の S3 互換オブジェクトストレージです（テストは in-process の gofakes3 を使います）。

- サーバ本体は公式の `dxflrs/garage` イメージをそのまま使います。ビルドが必要なのは `garage_init` の方です。公式イメージは `scratch` ベースで shell を持たずプロビジョニングスクリプトを実行できないため、そのために Dockerfile が `garage` バイナリを `alpine:3.24` へコピーします
- `garage.toml`: 単一ノード構成（`replication_factor = 1`、S3 API `3900` / Web API `3902`）。read-only でマウント
- `init.sh`: `garage_init` サービスが実行する冪等なプロビジョニング（レイアウト割当 → バケット作成 → 固定アクセスキー import → バケット許可 → 公開配信の許可）。同じくマウントで渡すため、編集にイメージの再ビルドは不要です。`OBJECT_STORAGE_*` は `env/.env` から直接読み（`env_file` で渡す）、未設定なら即座に失敗するため、バケットとキーが Go 側の接続先と食い違うことはありません

### 公開配信（匿名 read）

<!-- sample-api:replace-begin -->
Web API（`3902`、Garage の `[s3_web]`）はバケットのオブジェクトを**資格情報なしで**配信します。本番で CDN が S3 の前段に立つのと同じ形で、ブラウザがオブジェクトストレージから直接商品画像を読み込めます。書き込みは `POST /v1/products/images`（BearerAuth + admin）のままで、開くのは read だけです。
<!-- sample-api:replace-with -->
<!-- = Web API（`3902`、Garage の `[s3_web]`）はバケットのオブジェクトを**資格情報なしで**配信します。本番で CDN が S3 の前段に立つのと同じ形で、ブラウザがオブジェクトストレージから直接オブジェクトを読み込めます。書き込みは認証付きのアップロードエンドポイント経由のままで、開くのは read だけです。 -->
<!-- sample-api:replace-end -->

- 配信オリジン: `http://gobp-local.web.garage.localhost:3902` — オブジェクトは `<オリジン>/<オブジェクトキー>` で、例えば `http://gobp-local.web.garage.localhost:3902/<接頭辞>/{uuid}.png` です。フロントはこの値を配信オリジンの設定に入れます。API はフル URL を返さず、オブジェクトキーだけを返します
- **virtual-host 形式のみ。** Garage の web エンドポイントは `Host` ヘッダ（`<bucket>.<root_domain>` または `<bucket>`）からバケットを解決するため、パス形式（`localhost:3902/<bucket>/<key>`）は動きません。macOS と主要ブラウザは `*.localhost` を自前で `127.0.0.1` に解決するので `/etc/hosts` への追記は不要です。解決しない glibc の Linux コンテナ内からは、ヘッダを明示するか（`curl -H 'Host: gobp-local' http://<host>:3902/<接頭辞>/...`）、`/etc/hosts` へ `127.0.0.1 gobp-local.web.garage.localhost` を追記してください
- 一覧は閉じたままです。web エンドポイントは一覧を返さず、S3 API への匿名 `ListObjects` は署名が無いため拒否されます。オブジェクトキーは UUIDv7 で、ランダムな約 74 ビットにより列挙は非現実的です。ただし残りのビットはミリ秒精度のタイムスタンプであり、キーは opaque ではあっても secret ではありません。このバケットの中身は、キーを知る者に対しては誰でも読めるものとして扱ってください
- **公開配信の許可はバケット単位であってオブジェクト単位ではありません。** `gobp-local` のすべてのオブジェクトが匿名で読めるようになります。このバケットは公開して構わない資産しか持たないため許容していますが、非公開のオブジェクトを置くならバケットを分ける必要があります

## elasticmq

ローカル開発用の SQS 互換ブローカーです（テストは in-process の fake を使います）。ここにあるのは `elasticmq.conf` だけで、キューと DLQ の redrive 設定は起動時にそこから読まれるため、初期化用の one-shot コンテナは持ちません。ElasticMQ は環境変数を展開しないためキュー名は設定ファイルに直書きで、どの `*_QUEUE_URL` と対になるかは [`env/README.ja.md`](../env/README.ja.md) に記載しています。

## goaws

ローカル開発用の SNS / SQS エミュレータで、Realtime Delivery の instance fan-out（[`docs/design/realtime-delivery.ja.md`](../docs/design/realtime-delivery.ja.md)）だけが使います。Worker 側の outbox queue は `elasticmq` のままです。ここにあるのは `goaws.yaml` だけで、port・region・account id を固定します。account id は ElasticMQ の queue URL と同じ `000000000000` にし、ローカルの ARN が 1 つの account を名乗るようにしています。topic / queue は事前宣言しません。application が実行時に作り、その実行時作成が通ること自体を `make realtime-smoke` が確かめます。pin した GoAWS が何を受け付けるかの正本は `make realtime-smoke` で、image を上げたら再実行します。pin している版で拒否される唯一の呼び出しは queue 属性の `Policy`（`SetQueueAttributes` でも `CreateQueue` でも `InvalidParameterValue`）なので、production が設定する queue policy は local adapter 側で飛ばす必要があります。

## mock-auth-server

開発用の OIDC プロバイダです。ここでは何もビルドしません——上流の `ghcr.io/navikt/mock-oauth2-server` イメージを [`images-pin.toml`](images-pin.toml) で digest 固定して起動するため、このディレクトリが持つのは渡す設定だけです。

- `config.json` は `/etc/mock-oauth2-server/config.json` へ read-only でマウントし、`JSON_CONFIG_PATH` で渡します。トークンの契約はすべてここが宣言します——`issuerId`（issuer のパス要素と JWKS の `kid` を兼ねる）、リソースサーバーが要求する `at+jwt` 型ヘッダ（RFC 9068）、1 つの claim 集合でリソースサーバーと OIDC クライアントの双方を満たすための `aud` / `azp`（[`docs/design/auth.ja.md`](../docs/design/auth.ja.md) の 3.3.1 節）、そしてログインフォーム（または password grant）の `username` に解決され `sub` になる `${subject}`
- コンテナ内部のポートは常に `4000`（`SERVER_PORT`）。プロセスは上流イメージの非 root UID で動く
- issuer はトークンを取得したリクエストの `Host` から導出されるため、ここで宣言する必要はありません——ホスト公開ポート経由で取ったトークンは `AUTH_ISSUER` と一致する `iss` を持ちます。`docker-compose.attach.yaml` は API 側の `AUTH_ISSUER` をスロットのポートに追従させるだけで済みます
- 署名鍵は起動時に生成され、コミットしません。したがってトークンは再起動をまたいで再現しません。再現性に依存しているものはありません——リソースサーバーは実行時に JWKS から鍵を解決し、JWKS ローテーションテストが必要とする固定鍵はそのテスト自身が持ちます（`internal/integration/testdata/`）

## database

### sql

DB初期化用のSQLファイルを格納します。

- PostgreSQL コンテナ起動時に `docker-entrypoint-initdb.d` 経由で実行
- ファイル名の連番プレフィックス順に実行（例: `001-...`, `002-...`）
- DDL（テーブル定義）はここに置かず、`database/migrations/` で管理

### schemaspy

ER図生成ツール（SchemaSpy）の接続設定です。

|ファイル|用途|
|---|---|
|`schemaspy.properties`|ローカル環境用（host: `database`）|
|`schemaspy-ci.properties`|CI環境用（host: `localhost`）|

### sqlfluff

SQLリンター（sqlfluff）の設定ファイルです。対象ごとに異なるルールを適用します。

|ファイル|対象|特徴|
|---|---|---|
|`.dml.sqlfluff`|`database/dml/`（sqlc クエリ）|`@param` プレースホルダ対応、一部ルール除外|
|`.migrations.sqlfluff`|`database/migrations/`|行長制限150|
|`.seed.sqlfluff`|シードデータ|行長制限500|

共通ルール

- dialect: `postgres`
- キーワード: 大文字
- 識別子: 小文字
- 関数 / リテラル / 型: 大文字
- 3 ファイルとも `processes = 1`。並列実行すると CPython の `resource_tracker` のバグを踏み、
  `leaked semaphore` の警告や異常終了が起きるため（[python/cpython#131788](https://github.com/python/cpython/issues/131788)、
  [#142206](https://github.com/python/cpython/issues/142206)）。pin しているインタプリタで修正されたら見直す
