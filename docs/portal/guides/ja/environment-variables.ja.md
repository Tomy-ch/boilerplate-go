# 環境変数一覧（対応表）

このディレクトリは、アプリケーションが読み込むすべての環境変数の正規リファレンスです。各変数は `internal/config/` 配下の型付き Go 構造体にロードされ、本 README ではサブシステム別（OS / Application / Server / Database / Security / …）にグルーピングしています。新規変数の追加、各サービスが読む変数の棚卸し、オンボーディング資料として活用してください。

## 命名・型の規約

- 変数名は `{SUBSYSTEM}_{NAME}` の UPPER_SNAKE_CASE
- 型欄は `internal/config/` で読み込まれる Go 型に対応。下記の語彙は閉じており、語彙に無い型欄はどこも読み込まない型を指している:
  - `string` → `string`、`int` → `int`、`bool` → `bool`
  - `duration` → `time.Duration`（`time.ParseDuration` でパース、例: `500ms`, `1h30m`）
  - `csv` → `,` 区切りのスライス（空白トリム後分割）。`[]string`、要素が数値なら `[]int`（`OBS_TARGET_STATUS_CODES`）
- 備考に **Secret management required** とあるものは本番で **必ずシークレットマネージャーから取得**。`.env` に平文で含めない
- **Secret management recommended** は定期ローテーションを推奨
- 例欄には**ローカル実効値**を書く。`env/.env` に記載があればその値、無ければ `internal/config/envspec.go` の `envDefault` タグの値。単一の値であって選択肢の集合ではないため、取りうる値は説明欄に書く（`APP_MODE` / `OBS_TRACES_EXPORTER` の書き方に倣う）。唯一の例外は備考に **Example is a placeholder** とある行で、実値を書くとシークレットスキャナが `env/.env` で既に追跡している資格情報を二重に持つことになる場合に限る（ローカルスタックが実際に使う値は `env/.env` を参照）。**Secret management required** であること自体は例外ではない — ローカルの `.env` にどのみち平文で入っている以上、それらの行も実値を載せる
- `env/.env.<env>` ファイル間で値が異なってよいものは、備考欄に **Per-environment value** と明記し、続けて異なる理由を書く。キーが環境別ポリシーなのか全環境で揃うべき値なのかを記録する場所は他に無く、理由の書かれていない差異は伝播漏れとして読まれる（実際にそう扱ってよい）。マーカーの無いキーは、それを記載する全 env ファイルで同じ値であり、値を揃えた後にマーカーだけ残った状態は陳腐化した宣言なので外す。差異は**実効値**で読む。`Code default` を持つキーは記載の無いファイルでもその既定値を持つため、1 つの env ファイルだけで既定から外した場合も差であり、同じくマーカーが要る。`Code default` を持たないキーは、記載の無いファイルでは値が外部から注入されて実効値が定まらないため、その不在は下の **Injected at deploy time** が扱う
- `envDefault` を持たないキーは全 env ファイルに記載が必須。唯一の正当な不在は、デプロイ基盤が実行時に注入する値で、備考欄に **Injected at deploy time** と明記して宣言する。宣言が無ければ、欠落と伝播漏れは見分けが付かず、その環境で実際にアプリを起動して `required` バリデーションが落ちるまで顕在化しない。マーカーのあるキーは `local` / `ci` に記載があり `dev` / `stg` / `prd` には無い。deploy 側のファイルに値が復活した場合や、`Code default` を持つキーにマーカーが付いている場合は、宣言が実態を説明できていない
- 本ファイルは正本（[README.md](README.md)）の対訳で、値まで含めて表を複製している。このリポジトリの読者の多くは日本語版を読む。散文は翻訳されるが、キー・型・例・`Code default` の値は言語に依らないため正本と一致させること。文書構造 — 表を区切るサブシステム見出しと、その周辺の節・箇条書き項目 — も揃える。`Code default` が空の場合はバッククォートで書けないため、正本では **Code default empty**、対訳では **Code default は空** と綴る
- 備考に **Code default `<値>`** とあるものは `internal/config/envspec.go` の `envDefault:` タグを持ち、`.env` ファイルには意図的に記載しない。boilerplate 派生プロジェクトが基本そのまま使うフレームワークレベルの定数で、既定値が自動適用される。プロジェクト側で上書きしたいときだけ該当 `.env` に明示エントリを追加する。それ以外の変数は `required` で、該当する env ファイルに必ず記載すること
- ローカルの値を env ファイルではなく compose スタックから受け取るキーは、本ディレクトリが記述しきれない唯一の経路である。`internal/config` はプロセスの環境変数を先に読むため、`docker-compose*.yaml` の `environment` 指定は埋め込みの `env/.env` に優先し、その経路でしか供給されないキーはどの env ファイルにも現れない — つまり `internal/architest` の突き合わせの外に出る。env ファイルでは値を持てない場合に限って使い、compose 抜きでバイナリが何をするかは読み取れるよう行の `Code default` を正確に保つこと。compose 側の記載が陳腐化してもテストは検知しない

## 新規変数を追加する手順

1. `internal/config/envspec.go` の対応する構造体にフィールドを追加（あわせて `model.go` の getter 構造体・`config.go` のマッピングも）
2. 該当サブシステムのテーブル（または新規サブシステム節）に変数を記載
3. 値の供給方法を決める:
   - **プロジェクト固有・環境ごとに変わる値** → フィールドを `required` にし、`env/.env`（ローカル既定）と各環境ファイル（`env/.env.<env>`）に追加。`env/.env.dev` / `.stg` / `.prd` から外してよいのはデプロイ基盤が実行時に注入する場合だけで、そのときは行に **Injected at deploy time** と明記する
   - **普遍的なフレームワーク既定値** → 代わりに `envDefault:` タグを付与し、`.env` ファイルには記載せず、テーブルに **Code default `<値>`** と明記
4. `make test` を実行して config 構造体がロードできることを確認

## タイムゾーンを変更する手順

タイムゾーンは**別の層を設定する 3 つの独立した機構**から供給されるため、`Asia/Tokyo` から移るプロジェクトはその全てを変更する必要がある。リポジトリの他のどこにも全体像が記録されていないため、ここに一覧を置く。

- **セッションの timezone** — `OS_TZ` は、アプリが DB へ接続するときの DSN の `timezone` パラメータになる（`internal/infrastructure/rdb/driver/config.go`）。アプリが読み書きする時刻を決めるのはこの機構だけで、DB 側の設定より優先される。
- **DB 側の既定値** — PostgreSQL コンテナの `TZ` 環境変数。`initdb` がこれを `postgresql.conf` へ書き込むためクラスタ既定となり、以降に作られる全 DB が継承する（worktree スロットプールの `wt<N>_local` / `wt<N>_test`、`make dump-schema` の `gen_schema`）。timezone を明示しないクライアントの表示にしか影響しないため、目的は `psql` / pgweb / SchemaSpy で時刻を直接読むときの開発体験である。
- **アプリコンテナのローカルタイム** — アプリイメージの `TZ` 環境変数（`docker/server/Dockerfile` の `runtime` / `tooling` の両ステージ。そのために `tzdata` を導入している）。Go が `time.Local` を解決するときに読む値であり、コンテナ内の `date` やログのローカル時刻表示を決める。`OS_TZ` はこの役目を果たせない。Go が読むのは素の名前の `TZ` であり、`TZ` も `/etc/localtime` も無いプロセスは UTC へ退行するためである。正しさをこの機構に預けてはいない。`time.Local` は `.golangci-full.yaml` の `forbidigo` で禁止されており、アプリのコードは注入された `*time.Location`（`config.NewTimeLocation`）からタイムゾーンを受け取る。この機構は、コンテナを覗いた開発者が設定と無関係なタイムゾーンを見ないためにある。

3 つとも必要である。1 つ目を外すとアプリがクラスタ既定に従ってしまい、2 つ目を外すと DB へ直接繋いだセッションが全て UTC 表示になり、3 つ目を外すとコンテナ内のシェルとログ行が全て UTC 表示になる。次を全て揃えて変更する。

1. `env/.env` と全ての `env/.env.<env>` — `OS_TZ` のエントリ（セッションの timezone。`required` なので 6 ファイル全てに存在する）。
2. `docker-compose.yaml` の `database` サービス — `TZ` と `PGTZ`。`TZ` は `initdb` 時にしか効かないため、volume を作り直すまで既存 volume は旧クラスタ既定を保持する。その手順と worktree 特有の注意は `docs/maintenance/db-worktree-pool.md` が所有する。`PGTZ` は `psql` セッション単位で効くため、古い volume でも即座に反映される。
3. `.github/workflows/` 配下で DB を用意する各 workflow の PostgreSQL サービス定義 — `TZ` と `PGTZ`。GitHub Actions はサービス定義を workflow 間で共有できないため、値は全ファイルに重複して書かれている。列挙は変数ではなくサービスの image で行うこと（`grep -rl 'image: postgres' .github/workflows/`）— `PGTZ` で grep すると既に設定済みの workflow しか見つからず、まだ設定が要る workflow を黙って取りこぼす。
4. `docker/server/Dockerfile` — `runtime` / `tooling` の両ステージの `ENV TZ`。両方を挙げているのは別のイメージだからである。`runtime` はデプロイ先が動かすもの、`tooling` は `make serve` が動かすものである。デプロイ先は再ビルドせず実行時に値を上書きできるため、`ENV` は唯一の供給元ではなく既定値として扱うこと。`ENV` はビルド時に焼かれるため、項目 2 の `initdb` と同じく、変更前にビルドしたイメージは旧い値を保持する。`make serve` はキャッシュ済みの `gobp-wt-<N>-api_server` イメージを再利用し、コンテナが旧いタイムゾーンを返したまま成功を報告するので、変更を反映するには `make serve-build` を実行すること。
5. 値をリテラルで固定しているテストの期待値 — `internal/config/config_testing_mock.go` の `expectedOSTimeZone`、および `internal/di/job_test.go` / `internal/di/server/hook/http_server_hook_test.go` / `internal/infrastructure/rdb/driver/config_test.go` のアサーション。

`internal/architest` は伝播漏れで失敗するため、本番の時刻を読んでではなく `make test` で捕まる。値が食い違う場合は 1 から 4 の項目について `TestTimezoneMechanismValuesMatch` が、宣言そのものが消えた場合は `TestPostgresProvisionersDeclareTimeZone` / `TestDockerfileTzdataStagesDeclareTimeZone` が失敗する。項目 5 は機械検証していない。陳腐化したリテラルは、それを固定しているテストのアサーションが落ちる形で表面化する。

## 変数一覧（サブシステム別）

### OS

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|OS_TZ|タイムゾーン設定|string|Asia/Tokyo|コンテナ / アプリの時刻基準。配置先の地域はプロジェクトごとに変わるため、コード側の既定値ではなく `required` として全 env ファイルに明記し、タイムゾーンを運用者の見る場所に置く。空文字は UTC へ無警告で退行するため拒否する。設定するのはセッションの timezone のみで、DB 側の既定値とコンテナのローカルタイムはそれぞれ別の `TZ` が持つため、`Asia/Tokyo` から移る前に[タイムゾーンを変更する手順](#タイムゾーンを変更する手順)を参照|

### Application

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|APP_MODE|実行モード（`development` または `production`）|string|development|ログや挙動切り替え。Per-environment value — `stg` 以降は `production` とし、本番前の環境を本番と同じログ形式・挙動で動かす。local / ci / dev は `development`|
|APP_LOG_LEVEL|ログ出力レベル（`debug` / `info` / `warn` / `error`）|string|debug|出力方式は Mode が決定。Per-environment value — `stg` までは本番前の調査のため `debug`、`prd` は本番のログ量を抑えるため `info`|
|APP_NAME|アプリケーション名|string|Boilerplate API|ログ・メトリクス識別|
|APP_ENV|環境識別子（`local` / `ci` / `dast` / `dev` / `stg` / `prd`）|string|local|環境区別用。埋め込み env の出所ガードにも使う（補足を参照）。Per-environment value — 環境識別子そのものであり、定義上すべて異なる。`dast` は DAST スキャンの実行文脈で、`ci` と違い JWKS backed の実 authenticator を配線して mock 認証サーバーが実際に署名したトークンを検証する。また worktree の DB スロットプールには参加しない（`IsLocalClassEnv` は false を返す）|
|APP_SHUTDOWN_TIMEOUT|Graceful shutdown時間|duration|65s|Code default `65s`。SIGTERM時の待機時間。HTTP サーバーでは `SERVER_REQUEST_TIMEOUT` 以上でなければならない（未満だとサーバー起動失敗）ため、drain が予算内のリクエストを打ち切ることはない|

### Server

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|SERVER_HOST|バインドホスト|string|localhost|Dockerでは0.0.0.0推奨。Per-environment value — 各環境の到達先ホスト|
|SERVER_PORT|ポート番号|int|8080||
|SERVER_READ_HEADER_TIMEOUT|ヘッダ読み取りタイムアウト|duration|5s|Code default `5s`。Slowloris対策|
|SERVER_READ_TIMEOUT|リクエスト読み取りタイムアウト|duration|10s|Code default `10s`|
|SERVER_WRITE_TIMEOUT|レスポンス書き込みタイムアウト|duration|65s|Code default `65s`。SERVER_REQUEST_TIMEOUT 以上であること必須。短いと deadline budget より先に net/http が接続を切断し budget 制御が無効化される|
|SERVER_IDLE_TIMEOUT|KeepAliveタイムアウト|duration|60s|Code default `60s`|
|SERVER_BODY_LIMIT_MB|リクエストボディ上限（MB, 10進・1MB=1,000,000 byte）。超過時 413|int|6|Code default `6`。Pre middleware。OpenAPI 検証がボディを読む前に適用。`OBJECT_STORAGE_MAX_UPLOAD_BYTES`（＋ multipart オーバーヘッド）を上回る値に保つこと。下回るとエンドポイント側のアップロード上限が到達不能になる。サーバー起動時に `config.ValidateUploadBodyLimit` が強制する|
|SERVER_REQUEST_TIMEOUT|リクエスト全体の deadline budget（入口で1点設定し ctx で全層伝播）|duration|60s|Code default `60s`。停止/期限の単一軸。statement_timeout 等は backstop|

### Metrics

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|METRICS_HOST|metrics bind host|string|0.0.0.0|Per-environment value — 各環境が metrics を公開するホスト|
|METRICS_PORT|metrics port|int|6060||
|METRICS_USERNAME|Basic認証ユーザー|string|metrics-user|シークレット管理必須 — ソース管理に入れない。`local` / `ci` / `dast` のみ commit。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|METRICS_PASSWORD|Basic認証パスワード|string|metrics-password|シークレット管理必須 — ソース管理に入れない。`local` / `ci` / `dast` のみ commit。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|

### Observability

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|OBS_TRACES_EXPORTER|trace の OTLP exporter（`otlp` で有効化／空・`none` で無効）|string|otlp|空でトレース無効（軽量構成）。Per-environment value — compose の可観測性スタックを持つのは `local` だけで、他の環境は collector を結線するまで空にする|
|OBS_METRICS_EXPORTER|metric の OTLP exporter（`otlp` で有効化／空・`none` で無効）|string|otlp|空でメトリクス無効（軽量構成）。Per-environment value — compose の可観測性スタックを持つのは `local` だけで、他の環境は collector を結線するまで空にする|
|OBS_LOGS_EXPORTER|log の OTLP exporter（`otlp` で有効化／空・`none` で無効）|string|otlp|空でログ送出無効（zap は stdout のみ）。Per-environment value — compose の可観測性スタックを持つのは `local` だけで、他の環境は collector を結線するまで空にする|
|OBS_OTLP_PROTOCOL|OTLP プロトコル（`http/protobuf` / `grpc`）|string|http/protobuf|Code default `http/protobuf`|
|OBS_MASKED_DB_QUERY_ARGS|DBパラメータマスク|bool|false|セキュリティ重要。Per-environment value — local / ci / dast だけ `false`。クエリやテスト失敗、スキャン結果の調査では生の SQL 引数が見えること自体が目的のため。`dev` 以降は `true` とし、実データがトレースバックエンドへ届かないようにする。上位環境をローカル側の値に揃えてはならない|
|OBS_TARGET_STATUS_CODES|トレース対象ステータス|csv|400,401,403,404,405,409,410,422,429,500,501,503|エラー監視用。Per-environment value — 本番に近い環境ほど単調に絞り込むため、ファイル間の不一致は伝播漏れではなく意図である。`local` / `ci` / `dast` は開発・テストでの可視性のため全件を監視し、`dev` / `stg` は `429` を落とし、`prd` はさらに `403` / `404` / `405` / `410` を落として、本番の監視をサーバー側の失敗と契約違反に寄せる（本番規模ではクライアント起因のノイズが支配的になるため）。下位の環境が上位の環境の無視するコードを監視することはない。ポリシーは `TestEnvTargetStatusCodesPolicy`（`internal/architest`）が機械検証しており、一部の env ファイルにだけコードを足せばビルドが落ちる。特定環境から意図的に外す場合は、同テストのポリシー宣言も更新する必要がある|

### Database

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|DB_DRIVER|DBドライバ|string|pgx|Code default `pgx`。固定推奨|
|DB_HOST|DBホスト|string|database|docker service名（環境ごとに変更）。Per-environment value — `local` は compose のサービス名、`ci` / `dast` は `localhost`。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|DB_PORT|DBポート|int|5432|Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|DB_USER|ユーザー|string|postgres|シークレット管理推奨。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|DB_PASSWORD|パスワード|string|postgres-password|シークレット管理必須。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|DB_NAME|DB名|string|local|シークレット管理推奨。Per-environment value — `local` は開発用、`ci` / `dast` はテスト用のデータベースを指す。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|DB_SSL_MODE|SSL設定|string|disable|本番はrequire推奨。Injected at deploy time — `dev` / `stg` / `prd` はデプロイ時に注入|
|DB_PING_TIMEOUT|接続確認タイムアウト|duration|5s|Per-environment value — `local` は DB が同一の compose サービスにあり、ping が遅いことは起動不全を意味するため、早く落として顕在化させる `5s`。`ci` / `dast` は 1 つのインスタンスへ多数のテストプロセスが同時にプールを張るため、順番待ちしているだけの接続を「DB が落ちている」と読み違えないよう、その競合を吸収するマージンとして意図的に置いた `30s`（他のタイムアウトから導出した値ではない）。`dev` 以降はネットワーク越しのマネージド DB で一時的な遅延がありうるため `10s`|
|DB_SLOW_QUERY_WARN_THRESHOLD|遅延クエリ警告閾値|duration|500ms|Code default `500ms`。observability連携|
|DB_STATEMENT_TIMEOUT|SQL 文ごとの実行時間上限（`statement_timeout`）|duration|30s|Code default `30s`。ctx を無視する query への SQL 層 backstop。0 で無効|
|DB_LOCK_TIMEOUT|ロック獲得待ちの上限（`lock_timeout`）|duration|10s|Code default `10s`。長時間ロック待ちへの backstop。0 で無効|
|DB_TX_MAX_RETRIES|serialization failure / deadlock 時の tx リトライ最大試行回数|int|3|Code default `3`。0 でリトライ無効（単発実行）|
|DB_TX_RETRY_BASE_BACKOFF|tx リトライ backoff の初期値|duration|5ms|Code default `5ms`。指数 backoff の基準値（×2）|
|DB_TX_RETRY_MAX_BACKOFF|tx リトライ backoff の上限値|duration|100ms|Code default `100ms`。1 試行あたりの上限|

### Database Connection Pool

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|DBCONN_MAX_CONNS|最大接続数|int|10|Code default `10`|
|DBCONN_MIN_CONNS|最小接続数|int|5|Code default `5`。Per-environment value — pgxpool はプール生成の直後にこの本数の接続確立を一斉に走らせるため、`ci` / `dast` だけ `0` へ上書きし他は既定値のまま。テストは事前に温めたプールを必要とせず、プロセスが並列に走る状況ではその一斉確立はインスタンスへの負荷にしかならない|
|DBCONN_MAX_LIFETIME|接続寿命|duration|30m|Code default `30m`|
|DBCONN_MAX_IDLE_TIME|アイドル時間|duration|10m|Code default `10m`|

### Security

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|SECURITY_ALLOWED_ORIGINS|CORS許可|csv|`http://localhost:3000,http://localhost:8000`|Per-environment value — 各環境のフロントエンド origin|
|SECURITY_CIDR|許可IPレンジ|string|127.0.0.0/8||
|SECURITY_CONTENT_TYPE_NOSNIFF|X-Content-Type-Options|string|nosniff||
|SECURITY_X_FRAME_OPTIONS|clickjacking対策|string|DENY||
|SECURITY_HSTS_MAX_AGE|HSTS期間|duration|0|Per-environment value — local / ci / dast は平文 http で提供するため `0` で HSTS を無効化する（一度ヘッダをキャッシュしたブラウザは以降ロードを拒否するため）。`dev` 以降は前段で TLS を終端するため `8760h`（1 年）。上位環境をローカル側の値に揃えてはならない（本番の HSTS が消える）|
|SECURITY_HSTS_EXCLUDE_SUBDOMAINS|サブドメイン除外|bool|false||
|SECURITY_HSTS_PRELOAD_ENABLED|preload有効|bool|false||
|SECURITY_REFERRER_POLICY|referrer制御|string|no-referrer||
|SECURITY_CROSS_ORIGIN_RESOURCE_POLICY|Cross-Origin-Resource-Policy|string|same-origin|空ならヘッダーを送出しません。`same-origin` は CORS と両立します（このポリシーが検査されるのは `no-cors` リクエストのみで、許可オリジンからの CORS モードの fetch は影響を受けません）|

### Cookie

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|SECURE_COOKIE_SECURE|HTTPS限定|bool|true|本番必須|
|SECURE_COOKIE_SAME_SITE|SameSite設定|string|Strict||
|SECURE_COOKIE_DOMAIN|Cookieドメイン|string|localhost|Per-environment value — 各環境の Cookie ドメイン|

### Worker

worker engine の engine-core 設定（broker 非依存）。

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|WORKER_CONCURRENCY|同時に Handle を実行する最大数|int|4|Code default `4`|
|WORKER_MAX_IN_FLIGHT|受信済み・未確定の最大メッセージ数|int|8|Code default `8`|
|WORKER_BATCH_SIZE|1 回の Receive で取得する最大件数|int|4|Code default `4`|
|WORKER_EXTEND_INTERVAL|Extend を呼ぶ周期（`0` 以下で無効）|duration|0s|Code default `0s`|
|WORKER_DRAIN_TIMEOUT|停止時に in-flight の完了を待つ上限|duration|30s|Code default `30s`|
|WORKER_RECEIVE_COUNT_WARN_THRESHOLD|再配送回数の警告閾値（`0` 以下で無効）|int|5|Code default `5`|
|WORKER_CIRCUIT_FAILURE_THRESHOLD|サーキットを Open にする連続失敗数（`0` 以下で無効）|int|10|Code default `10`|
|WORKER_CIRCUIT_OPEN_BACKOFF_INITIAL|Open の初回 cooldown|duration|1s|Code default `1s`|
|WORKER_CIRCUIT_OPEN_BACKOFF_MAX|Open の cooldown 上限|duration|30s|Code default `30s`|
|WORKER_CIRCUIT_HALF_OPEN_PROBE|Half-open 時に試行する最大件数|int|1|Code default `1`|
|WORKER_HEALTH_LISTEN_ADDR|liveness/readiness を公開する health listener の待ち受けアドレス|string|:8081|Code default `:8081`|
|WORKER_PROGRESS_STALE_AFTER|readiness 判定で「進捗なし」とみなすまでの時間|duration|60s|Code default `60s`|
|WORKER_NACK_BACKOFF_INITIAL|retryable 失敗時の per-message 再配送 backoff の初回待機|duration|1s|Code default `1s`|
|WORKER_NACK_BACKOFF_MAX|per-message 再配送 backoff の上限|duration|30s|Code default `30s`|

### Consumer Queue

worker が consume する broker の adapter 設定。`WORKER_*` は engine-core かつ broker 非依存と定めて
いるため別の軸に置く。`OUTBOX_QUEUE_*` が同じ対の publish 端で、こちらが consume 端にあたる。

全項目に code default があるため、これらを読むのは `worker` プロセスだけで、`serve` / `outbox-relay` /
`job` は consumer をそもそも組み立てない。worker を登録した時点からは、不完全な設定を起動時に弾く
（受信が一度も成立しない consumer を起動させないため）。`ci` / `dast` に疎通しないプレースホルダを置いて
あるのはそのためで、worker の DI グラフを起動するテストが無ければ起動時検査で落ちる。

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|CONSUMER_QUEUE_REGION|SigV4 署名に使うリージョン|string|us-east-1|Code default は空。worker を配線した時点から必須。**Per-environment value**: ブローカーが compose で動く local でのみ設定する|
|CONSUMER_QUEUE_URL|consume 対象キューの URL|string|`http://elasticmq:9324/000000000000/gobp-events`|Code default は空。worker を配線した時点から必須。ローカルのキュー名は `docker/elasticmq/elasticmq.conf` 由来。**Per-environment value**: ブローカーが compose で動く local でのみ設定する|
|CONSUMER_QUEUE_DLQ_URL|滞留量を収集する DLQ の URL|string|`http://elasticmq:9324/000000000000/gobp-events-dlq`|Code default は空。Permanent メッセージの退避先であり、滞留量の収集対象でもある。空にすると app 側の退避経路を持たず、broker の redrive policy に委ねる|
|CONSUMER_QUEUE_ACCESS_KEY_ID|静的資格情報のアクセスキー ID|string|local-dummy-access-key|Code default は空。シークレットとあわせて空にすると SDK 既定の chain へ資格情報の解決を委ね、それが IAM ロールの使い方になる。ElasticMQ は署名を検証しないためローカルは任意のダミーでよい|
|CONSUMER_QUEUE_SECRET_ACCESS_KEY|静的資格情報のシークレットアクセスキー|string|local-dummy-secret-key|Code default は空。アクセスキー ID とセットで設定すること — 片方だけの指定は起動時に落ちる。明示注入とも chain への委譲とも読めないため|
|CONSUMER_QUEUE_MAX_MESSAGES|1 回の受信で取得する最大件数|int|10|Code default `10`。SQS 自身の上限|
|CONSUMER_QUEUE_WAIT_TIME_SECONDS|long-poll の待機秒数（0〜20）|int|20|Code default `20`。上限にすると空ポーリングの往復が最も減る|
|CONSUMER_QUEUE_VISIBILITY_TIMEOUT|受信メッセージの可視性タイムアウト秒数|int|30|Code default `30`。ローカルの `elasticmq.conf` と対。`WORKER_EXTEND_INTERVAL` のハートビート無しでこれを超える Handler は、メッセージを再配送される|

### Outbox

transactional outbox relay の設定。

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|OUTBOX_PUBLISHER|publish 先の種別（`http` / `sqs`）|string|http|Code default `http`。未知の値は起動エラー（fail-closed）。publish 先はデプロイ先ごとの判断なのでどの env ファイルも値を固定せず、キューへ publish するデプロイが実行時に与える|
|OUTBOX_POLL_INTERVAL|pending を捌き切った後、次 poll まで待機する時間|duration|1s|Code default `1s`|
|OUTBOX_ERROR_BACKOFF|relay バッチがエラーを返した後に待機する時間|duration|5s|Code default `5s`|
|OUTBOX_BATCH_SIZE|1 回の poll で claim する pending 行数|int|100|Code default `100`|
|OUTBOX_QUEUE_REGION|SigV4 署名に用いるリージョン|string|us-east-1|Code default は空。`OUTBOX_PUBLISHER=sqs` のとき必須。**Per-environment value**: ブローカーが compose で動く local でのみ設定する。キューはデプロイ先ごとのリソースなので他環境は空のまま|
|OUTBOX_QUEUE_URL|publish 先キューの URL|string|`http://elasticmq:9324/000000000000/gobp-events`|Code default は空。`OUTBOX_PUBLISHER=sqs` のとき必須。ローカルのキュー名は `docker/elasticmq/elasticmq.conf` と対。**Per-environment value**: ブローカーが compose で動く local でのみ設定する。キューはデプロイ先ごとのリソースなので他環境は空のまま|
|OUTBOX_QUEUE_ACCESS_KEY_ID|静的資格情報のアクセスキー ID|string|local-dummy-access-key|Code default は空。シークレットとあわせて空にすると SDK 既定の chain へ資格情報の解決を委ね、それが IAM ロールの使い方になる。ElasticMQ は署名を検証しないためローカルは任意のダミーでよい|
|OUTBOX_QUEUE_SECRET_ACCESS_KEY|静的資格情報のシークレットアクセスキー|string|local-dummy-secret-key|Code default は空。アクセスキー ID とセットで設定すること — 片方だけの指定は起動時に落ちる。明示注入とも chain への委譲とも読めないため|

### Auth (JWT)

access token（JWT）検証の設定。CI / test は署名検証なしのスタブを配線し、`local` / `dast` / `development` は JWKS backed の実 JWT authenticator を配線する（`local` と `dast` は mock 認証サーバーを検証）。後者は `AUTH_ISSUER` / `AUTH_AUDIENCE` が欠けていると起動時に fail-closed になる。配線判断は DI（`internal/di/module/core/auth.go`）が担う。`ENDPOINT_JWKS` は override で、空の場合は `AUTH_ISSUER` から OIDC discovery 経由で `jwks_uri` を導出する（issuer 厳密一致 + https + 同一オリジン。`local` / `dast` は mock provider への平文 http を許容）。JWKS / discovery 取得は `httpclient` substrate を通すため、HTTP タイムアウト / リトライ / circuit breaker / budget は `jwks` downstream プロファイル（`NewDownstreamProfile`）由来で、env 変数では持たない。同プロファイルは `local` / `ci` / `test` / `dast` 以外では private 網宛て SSRF も遮断する。

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|AUTH_ISSUER|検証する `iss` クレームの期待値（OIDC issuer 兼用）|string|`http://localhost:2010/default`|Code default は空。Per-environment value — `local` / `ci` / `dast` は mock auth server を指し、deploy 環境は JWT authenticator を配線するまで既定の空のまま。`db-seed` が identity の seed へ展開するため、認証を stub する環境（CI）でも seed するなら必要|
|AUTH_AUDIENCE|検証する `aud` クレームの期待値|string|go-boilerplate-api|Code default は空。issuer と対で必須。Per-environment value — mock の audience を宣言するのは `local` / `dast` だけで、他は authenticator を配線するまで既定の空のまま|
|AUTH_ALLOWED_ALGORITHMS|許可する署名アルゴリズムの allowlist（カンマ区切り・非対称のみ）|csv|RS256|Code default `RS256`。`none` / 対称鍵は常に拒否|
|AUTH_CLOCK_SKEW|`exp` / `nbf` 検証のクロックずれ許容幅|duration|60s|Code default `60s`|
|AUTH_JWKS_CACHE_TTL|取得した JWKS をキャッシュする期間|duration|1h|Code default `1h`|
|AUTH_JWKS_DISCOVERY_TTL|OIDC discovery 文書をキャッシュする期間（鍵キャッシュとは別軸）|duration|24h|Code default `24h`。jwks_uri を discovery で導出する場合のみ使用|
|AUTH_JWKS_UNKNOWN_KID_COOLDOWN|未知 `kid` での JWKS 再取得の最小間隔（DoS 抑止）|duration|60s|Code default `60s`|

### Object Storage

アップロード資産用の S3 互換オブジェクトストレージ。usecase は vendor 中立の `objectstorage.Storage` 境界に依存し、infrastructure 実装は S3 アダプタ（AWS SDK v2 S3）。`local` は Garage コンテナへ接続し、deploy 環境は `ENDPOINT_OBJECT_STORAGE` を空にして AWS S3 を対象にする。アダプタは S3 だが env 名は vendor 中立に保つ。値は環境ごとに宣言（code default を持たない）し、資格情報はデプロイ時に注入する。

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|OBJECT_STORAGE_REGION|署名リージョン|string|us-east-1|`required,notEmpty`。Per-environment value — local / ci / dast は Garage 用のサンプルリージョン、`dev` 以降は各環境の AWS リージョン|
|OBJECT_STORAGE_BUCKET|オブジェクト格納先バケット|string|gobp-local|`required,notEmpty`。Per-environment value — 環境ごとに 1 バケット|
|OBJECT_STORAGE_ACCESS_KEY_ID|静的資格情報のアクセスキー ID|string|gobp-local-access-key|Code default は空。シークレットとあわせて空にすると SDK 既定の chain へ資格情報の解決を委ね、それが IAM ロールの使い方になる — ロール運用のデプロイはここへ何も注入しない。明示的に設定する場合はシークレット管理必須。例欄はプレースホルダ: `local` は `env/.env` が持つ Garage の固定資格情報（`GK` + 24 hex）を使う|
|OBJECT_STORAGE_SECRET_ACCESS_KEY|静的資格情報のシークレットアクセスキー|string|gobp-local-secret-key|Code default は空。アクセスキー ID とセットで設定すること — 片方だけの指定は起動時に落ちる。明示注入とも chain への委譲とも読めないため。例欄はプレースホルダ: `local` は `env/.env` が持つ Garage の固定資格情報（64 hex）を使い、README へ複製すると `.gitleaksignore` の登録がもう 1 件増える|
|OBJECT_STORAGE_USE_PATH_STYLE|path-style アドレッシング（Garage / MinIO は true、AWS S3 は false）|bool|true|`required`。Per-environment value — local / ci / dast は Garage が path-style を要求するため `true`、`dev` 以降は AWS S3 のため `false`|
|OBJECT_STORAGE_MAX_UPLOAD_BYTES|受理する最大アップロードサイズ（バイト）|int|5242880|`required,notEmpty`。サンプルは 5 MiB。グローバルな `SERVER_BODY_LIMIT_MB`（バイト・10進）から multipart オーバーヘッドを引いた値より小さく保つこと。上回るとグローバルの body limit が先に拒否し、この判定が発火しない。サーバー起動時に `config.ValidateUploadBodyLimit` が強制する|

配信はこれらの変数の管轄外です。API はオブジェクトキー（`imagePath`）だけを返しフル URL を返さないため、フロントが `<配信オリジン>/<オブジェクトキー>` を組み立てます。したがって配信オリジンの変数はこちら側に存在せず、フロントが持ちます（`local` は `http://gobp-local.web.garage.localhost:3902`、デプロイ環境では CDN のドメイン）。ローカルの配信エンドポイントを匿名 read で開く方法は [`docker/README.md`](../docker/README.md) を参照してください。

### Realtime Delivery

Realtime Delivery（[`docs/design/realtime-delivery.ja.md`](../docs/design/realtime-delivery.ja.md)）の背後にある DynamoDB 互換 store — EventLog（有界の replay）、stream ticket、instance lease — と、全 serve instance を起こす SNS / SQS 互換の fan-out の設定です。usecase は vendor 非依存の `realtime` 境界に依存し、adapter は DynamoDB と SNS / SQS（AWS SDK v2）なので、`local` / `ci` は DynamoDB Local と GoAWS へ、デプロイ環境は `ENDPOINT_REALTIME` / `ENDPOINT_REALTIME_PUBSUB` を空にして AWS へ繋ぎます。table 名は固定名（`realtime_event_log` / `realtime_stream_ticket` / `realtime_instance_lease`）に `_<REALTIME_TABLE_SUFFIX>` を付けた形で、1 つの DynamoDB（あるいは共有の DynamoDB Local）に複数環境を並べられます。table と topic は one-shot の `make realtime-init` が作り、application の起動時には作りません。instance ごとの queue だけは application 自身が serve 起動時に `REALTIME_QUEUE_PREFIX` の下で作ります。

|変数名|説明|型|例|Notes|
|---|---|---|---|---|
|REALTIME_REGION|署名に用いるリージョン|string|us-east-1|`required,notEmpty`。環境ごとの値 — local / ci / dast は DynamoDB Local 向けのサンプル値、`dev` 以降はその環境の AWS リージョン|
|REALTIME_TABLE_SUFFIX|全 table 名の末尾に付く環境識別子（`realtime_event_log_<suffix>` …）|string|local|`required,notEmpty`。小文字・数字・アンダースコア。環境ごとの値 — 環境名。worktree の slot はここに独自の suffix（`local_wt<N>`、`docker-compose.attach.yaml` が設定）を注入し、DynamoDB Local を共有する checkout 同士が table を共有しないようにする|
|REALTIME_TOPIC|wakeup と revocation の通知を全 serve instance へ運ぶ topic の識別子（AWS では ARN）|string|`arn:aws:sns:us-east-1:000000000000:realtime-fanout-local`|Code default は空。host ではなく resource 識別子なので `ENDPOINT_*` ではない。空のまま fan-out が結線されると起動に失敗する（`ENDPOINT_OUTBOX` と同じ扱い）。環境ごとの値 — `local` / `ci` は `make realtime-init` が GoAWS 上に作る topic を指す（`ci` の account id はイメージ既定の `100010001000`。service container は設定ファイルを取らないため）。`dast` は自前の GoAWS service container 上の topic を指す（`ci` と同じ形で suffix だけ `dast`）。DAST ジョブは実の serve graph を起動し、topic が空だと fail-closed で落ちるため。`dev` 以降は deployment が provisioning した topic の値を**デプロイ時に注入**する|
|REALTIME_QUEUE_PREFIX|instance ごとの queue 名の prefix（`<prefix>-<instance id>`）|string|realtime-local|`required,notEmpty`。英数字と `-` / `_`。full name が SQS の上限 80 文字に収まるよう最大 43 文字。環境ごとの値 — 環境名。1 つのアカウント（あるいは共有の GoAWS）上で複数環境の instance が衝突しないようにする|
|REALTIME_DLQ|instance ごとの queue が redrive する先の dead-letter queue の識別子（AWS では ARN）|string||Code default は空。空なら `RedrivePolicy` を付けない。wakeup は状態を持たず、1 つ失っても定期 catch-up が拾うので、DLQ は正しさの要件ではなく運用上の安全網である。環境ごとの値 — local / ci / dast では空（GoAWS は `RedrivePolicy` を設定すると receive-and-delete が壊れる）、`dev` 以降は deployment が provisioning した DLQ の値を**デプロイ時に注入**する|
|REALTIME_ACCESS_KEY_ID|静的資格情報のアクセスキー ID|string|localdummyaccesskey|Code default は空。secret と揃って空なら資格情報の解決を SDK 既定の chain に委ねる（IAM ロールの使い方）— ロール運用のデプロイは何も注入しない。DynamoDB Local は認証しないので local / ci / dast は SDK の署名を満たすだけのダミー — ただし**英数字のみ**: `-` を含む key は DynamoDB Local が `UnrecognizedClientException` で拒否する|
|REALTIME_SECRET_ACCESS_KEY|静的資格情報のシークレットアクセスキー|string|localdummysecretkey|Code default は空。アクセスキー ID と揃えて設定する — 片方だけの設定は明示注入とも chain 委譲とも読めないため起動に失敗する|
|REALTIME_MAX_CONNECTIONS|1 instance が同時に保持する SSE 接続数の上限|int|1000|Code default は `1000`。instance 単位の上限で、1 プロセスが長寿命レスポンスに割り当てるメモリとファイルディスクリプタを縛るため、環境ではなく機材に従う。到達すると新規接続に `503` を返し、既存の接続には手を触れない|
|REALTIME_REPLAY_CONCURRENCY|1 instance で replay と catch-up が同時に走る本数の上限|int|16|Code default は `16`。1 instance が EventLog に掛ける読み取り負荷を縛る。初回 replay は枠が空くのを有界に待ってから `503` を返し、catch-up は順番を待つだけ|

### Endpoint

このデプロイの接続先です。「どこへ繋ぐか」は独立した軸で、デプロイごとに変わる一方で各サブシステムの振る舞いは変わりません。そのため、叩く側のサブシステム設定ではなくここにまとめます。すべて `required` ですが空文字を許容し、空が何を意味するかは接続先ごとに異なるため Notes に記します。キューの `*_URL` はここに置きません（接続先ではなく API 引数として渡すリソース識別子のため）。`AUTH_ISSUER` も突き合わせる値であって接続先ではありません。

|変数名|説明|型|例|備考|
|---|---|---|---|---|
|ENDPOINT_OTLP|OTLP 送出先エンドポイント URL|string|`http://observability:4318`|exporter 有効時に使用。Per-environment value — 各環境の collector。exporter が無効な環境では空|
|ENDPOINT_JWKS|JWKS エンドポイント URL の override。空の場合は `AUTH_ISSUER` から OIDC discovery で `jwks_uri` を導出|string|`http://mock_auth_server:4000/default/jwks`|Code default は空。Per-environment value — compose 内部のサービス URL で上書きするのは `local` だけで、他は既定の空のまま `AUTH_ISSUER` から OIDC discovery で `jwks_uri` を導出する|
|ENDPOINT_OBJECT_STORAGE|S3 互換エンドポイント URL。空は SDK 既定解決（AWS S3）|string|`http://garage:3900`|`required`（空を許容）。Per-environment value — `local` は Garage の compose サービスを指し、他の環境は SDK が AWS S3 を解決するよう空にする|
|ENDPOINT_REALTIME|Realtime Delivery の store の DynamoDB 互換エンドポイント URL。空なら SDK 既定の解決（AWS DynamoDB）|string|`http://dynamodb_local:8000`|`required`（空を許容）。環境ごとの値 — `local` は compose の DynamoDB Local、`ci` と `dast` は service container の `localhost:8000`。他の環境は空にして SDK に AWS DynamoDB を解決させる|
|ENDPOINT_REALTIME_PUBSUB|Realtime Delivery の fan-out の SNS / SQS 互換エンドポイント URL（1 つの host が両方の API を提供する）。空なら SDK 既定の解決（AWS SNS / SQS）|string|`http://goaws:4100`|`required`（空を許容）。環境ごとの値 — `local` は compose の GoAWS、`ci` と `dast` は service container の `localhost:4100`。他の環境は空にして SDK に AWS SNS / SQS を解決させる。`ENDPOINT_OUTBOX_QUEUE` / `ENDPOINT_CONSUMER_QUEUE` と分けているのは意図的で、worker のブローカー emulator はこの機構と共有しない|
|ENDPOINT_OUTBOX|メッセージの送信先エンドポイント URL|string||Code default は空。`OUTBOX_PUBLISHER=http` のとき必須|
|ENDPOINT_OUTBOX_QUEUE|SQS 互換エンドポイント|string|`http://elasticmq:9324`|Code default は空。空なら SDK 既定の解決に委ねる（本番 AWS SQS 等）。**Per-environment value**: ブローカーが compose で動く local でのみ設定する。キューはデプロイ先ごとのリソースなので他環境は空のまま|
|ENDPOINT_CONSUMER_QUEUE|SQS 互換エンドポイント|string|`http://elasticmq:9324`|Code default は空。空なら SDK 既定の解決（本番 AWS SQS）に委ねる。**Per-environment value**: ブローカーが compose で動く local でのみ設定する。キューはデプロイ先ごとのリソースなので他環境は空のまま|
|ENDPOINT_EXCHANGE_RATE|為替レートサービスのベースURL|string||サンプルAPI。空はこの機能を使わないことを表し、当該エンドポイントは 503 を返します。**環境ごとの値**: 検査が立てる疑似サービスを指すのは `dast` だけで、これにより検査が 5xx を常態として学習しません。サンプルAPI削除時に一緒に消えます<!-- sample-api:line -->|

## 補足

- 例欄はローカルの値であってデプロイにそのまま使える値ではありません。列の定義は「命名・型の規約」を参照。どのキーを環境ごとに違う値にしているかは表の **Per-environment value** マーカーが記録しているので、変数の種類から推測せずマーカーを読むこと
- `csv` 型は `,` 区切りで空白トリム後に分割。値そのものに `,` を含めないこと
- `duration` 型は Go `time.ParseDuration` 構文（`500ms`, `1h30m`）。素の数値は不可
- 新規サブシステム節を作る際もテーブル列構成（`変数名 | 説明 | 型 | 例 | 備考`）を維持してスキャン性を保つこと
- env ファイルはビルド時にバイナリへ埋め込まれる（`embed.go`）。`env/.env` がローカル既定かつ唯一の埋め込み対象で、`env/.env.<env>` は各環境のソース。Docker の `builder` ステージが `APP_ENV` ビルド引数で対象を材料化する（`go build` 前に `cp env/.env.${APP_ENV} env/.env`）。Docker 以外（`go run` / `go test`）はコミット済みの local `env/.env` を埋め込むため、別環境が必要な CI は同様に焼き直す（例: `cp env/.env.ci env/.env`）。実行時の環境変数は埋め込み値より優先される
- 埋め込み env の素性ガード: ローカルの `env/.env`（`APP_ENV=local`）が既定の埋め込み対象であるため、本番ビルド前に材料化し忘れるとローカル既定が黙ってバイナリへ焼き込まれる。これを捕捉するため、config の検証はランタイム env のマージ前に埋め込み値の `APP_ENV` を確保し、実効 `APP_MODE` が `production` のときに非本番の素性を拒否する（deny-list: `local` / `ci` / `test` / `dast` / `dev` / 空）。deny 方式なので新しい環境ラベルは既定で許容され、`development` モードは無条件で通す（そこではランタイム注入を信頼する）。環境別の `APP_ENV` の値（`local` / `ci` / `dast` / `dev` / `stg` / `prd`）が正であり、`internal/config/constant.go` の `Env*` 定数はそれを写したもので乖離させてはならない。このガードはランタイムが `APP_MODE=production` を注入したときにのみ発火する。本番デプロイで注入し損ねると実効モードは `development` のままガードは沈黙するため、本番ランタイムでは必ず `APP_MODE=production` を設定すること
