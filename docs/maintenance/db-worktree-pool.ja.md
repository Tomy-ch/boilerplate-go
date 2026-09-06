# Shared Infra and the DB Slot Pool (parallel worktree development)

複数の git worktree（および主 checkout）が **単一の共有インフラ** を衝突なく並列利用するための仕組み。
compose のサービスを 2 層に分ける:

- **infra 層** — 固定ポートでしか動けないサービス（`database` 5432 / `observability` 3000・4317・4318・3200 /
  `garage` 3900・3903）。固定 compose プロジェクト `gobp-shared` に **全 checkout で 1 インスタンスだけ**置く。
- **app 層** — checkout 毎に要る `api_server` / `mock_auth_server`。checkout 毎の compose プロジェクトへ分離し、
  ホスト公開ポートをずらして並列起動する。

DB の worktree 分離は「別コンテナ・別ポート」ではなく「同一インスタンス内の別データベース」
（`wt<N>_local` / `wt<N>_test`）で行う。これにより DB 軸のホストポート割当が不要になり、「別 worktree が
5432 を握っていて `make test` が動かせない」も「主 checkout の serve が worktree の DB と衝突する」も起きない。
o11y は共有が利点になる（全 checkout のトレース / メトリクス / ログが 1 つの Grafana に集まる）。

## 不変条件: データベース : worktree = 1 : 0..1

**1 つのデータベースを 2 箇所から触らせない。** worktree はスロットを取っても取らなくてもよいが、
取らなかった worktree が所有するデータベースは *無い*。既定の `local` / `test` へフォールバックはしない。

| 誰が | 何を所有するか |
| --- | --- |
| 主 checkout | `local` / `test` / `gen_schema` |
| スロット N を保持する worktree | `wt<N>_local` / `wt<N>_test` / `gen_schema_wt<N>` |
| スロットを持たない worktree | 無し（データベースを触るターゲットは失敗する） |

既定値へフォールバックさせると主 checkout のデータベースに所有者が 2 つできる。しかもそれは*黙って*
起きるため、別ブランチの migration が混ざったデータベースでテストが通る、あるいは誰かが migrate 中の
スキーマから生成物が作り直される、という形で後になって表面化する。そこで
`make require-db-owner`（`.makefiles/database/pool.mk`）を、データベース名を解決する全ターゲットの
前提条件に置いている — `db-migrate-*` / `db-seed` / `db-drop-tables` / `db-ensure` / `dump-schema` に加え、
`make test` / `test-cached` / `gen-test-repo`（host 実行の `go test` が `DB_NAME_TEST` を読む）と
`make serve` / `serve-build` / `serve-build-clean`（app コンテナが `DB_NAME_LOCAL` を読む）。
判定の実体は `internal/cli/dbslot` にある。リンク worktree は `git-dir` ≠ `git-common-dir` の
食い違いで識別するため、主 checkout と CI は素通りする。`git` がそもそも答えられない場合も一律には
扱わない。git 実行ファイルが無い場合（ツールランナーのコンテナ）と、git リポジトリでない場合は
どちらも worktree ではありえないため素通りする。一方、git リポジトリではあるのに構成を読み取れない
場合は失敗する — そこでは worktree でないと断定できず、黙ってフォールバックしないことこそが
このガードの目的だからである。

知っておくべき帰結: worktree では `make slot-acquire` するまで `make test` が落ちる。それが狙いで、
このガードが無かった頃は共有 `test` データベースに対して黙って走っていた。

## 仕組み

- **infra 層** = 固定 compose プロジェクト `gobp-shared`（`GOBP_DB_SHARED_PROJECT`）。worktree はディレクトリ毎に
  compose の既定プロジェクト名が変わるため、共有側は明示の固定名に固定する。`make infra-up` で起動し、
  `make serve` / `make job` / `make worker` も冒頭で冪等に呼ぶ。ここでの冪等は非破壊も含む意味で、
  既に動いているコンテナは作り直さずそのまま残す（Caveats 参照）。
- **app 層** = `APP_PROJECT`。スロット未取得なら `gobp-app-<ディレクトリ名>`、取得時は `gobp-wt-N`。
  `docker-compose.attach.yaml` を重ね、共有インフラを `host.docker.internal` のホスト公開ポート経由で参照する
  （`DB_HOST` / `ENDPOINT_OTLP` / `ENDPOINT_OBJECT_STORAGE` を実行時 env で上書きする。`loader.go` は
  実行時 env を `env/.env` より優先する）。
- **スロット N** = 共有 DB 内のデータベース名ペア `wt<N>_local` / `wt<N>_test`（既定 MAX 12 = wt1〜wt12）と、
  スキーマ生成が作り直す使い捨て DB `gen_schema_wt<N>`。スロット取得は**並列作業のための opt-in** で、
  主 checkout には不要だし、データベースを要さない worktree も取らなくてよい。できないのは、
  所有していないデータベースを使うことだけ（上の不変条件を参照）。
- **実装** = ホスト実行の Go CLI `cmd/db-slot`（コアは `internal/cli/dbslot`）。リース判定・DB 作成・
  compose 起動をテスト可能な形で担う。make ターゲットは `go run ./cmd/ db-slot <sub>` を呼ぶ。
- **リース** = ホスト上のロックディレクトリ `${GOBP_DB_POOL_DIR:-~/.cache/gobp-db-pool}/slot-N.lock`。
  新規取得は `os.Mkdir` の原子性、stale 回収は `rename` で原子的に占有権を奪い、acquire の走査全体を
  `flock` で直列化するため、2 worktree が同一スロットを二重リースしない。`meta`（owner / heartbeat /
  branch）は `0600`、symlink を向いた pool dir は先読み攻撃対策として拒否する。
- **実行環境ガード** = `APP_ENV` が deploy 系（dev/stg/prd）のとき db-slot は実行を拒否する（DB を
  作成/破棄するため dev/test 専用）。`internal/config` のテスト設定も同様に deploy 系では DB_NAME 上書きを
  無視する。
- **占有情報** = acquire が worktree ルートに `.gobp-db-slot`（gitignore）を書き出す。`make` が
  `-include` して `.makefiles/docker/compose.mk` の既定値を上書きし、全ターゲットへ伝播する:
  - `DB_NAME_LOCAL` / `DB_NAME_TEST` = `wt<N>_local` / `wt<N>_test`（既定 `local` / `test`。host 実行の
    `go test` は共有 DB の localhost:5432 経由でこの名前の自 worktree DB へ繋ぐ。`internal/config` のテスト設定が参照）
  - `API_HOST_PORT` = `8080+N` / `MOCK_AUTH_HOST_PORT` = `2010+N` / `DLV_HOST_PORT` = `2345+N` /
    `PPROF_HOST_PORT` = `6060+N`（app 層のホスト公開ポートは全てスロット番号で相対化する）
  - `SERVE_PROJECT` = `gobp-wt-N`（app 層の compose プロジェクト = `APP_PROJECT`）
  - `COMPOSE_PROJECT_NAME` = `gobp-shared`（DB ツーリング migrate/seed/psql/gen が共有インフラのネットワークで
    動くよう既定プロジェクトを infra 層へ寄せる。未取得時も compose.mk が同じ既定を置く）
- **ずれたポートに追随する永続データ**: ホスト公開ポートは接続先であるだけでなく、DB に**保存される**値でも
  ある。JWT の issuer がそれで、mock 認証サーバーは `2010+N` で公開されるため発行トークンの `iss` はスロットで
  ずれ、resolver が `(issuer, subject)` で突き合わせる行も一緒にずれていなければならない
  （リテラル固定だと、スロットを取った worktree では認証を要求する全エンドポイントが 401 になる）。そのため
  seed ファイルは URL ではなく `${AUTH_ISSUER}` を持ち、`make db-seed` がそのスロットの値を渡す
  （`database/seed/README.md` を参照）。`db-reinit` / `db-seed` / `slot-acquire` のいずれを通っても環境に一致する
  identity が入る。この種のデータを足すときも、既定ポートを焼き込まず同じようにスロットへ追随させること。
  DB 名と同じく、この値が host 実行の `go test` に届くのは `make` 経由だけ（`make test` / `test-cached` が
  export する）。素の `go test` は `DB_NAME_TEST` も スロットの issuer も受け取らないため、DB を使うテストは
  これらのターゲットから実行すること。
- **拡張のブートストラップ**: acquire は `wt<N>_local` / `wt<N>_test` を CREATE DATABASE
  （存在ガード）した後、各 DB に `pg_trgm` 拡張を設定する（init スクリプトが `local` / `test` に施すのと
  同じもの。動的に作る worktree DB には明示設定が必要）。timezone は DB 単位では設定しない。`database`
  サービスの `TZ` が `initdb` 時に `postgresql.conf` へ書き込まれてクラスタ既定になり、後から作った DB も
  それを継承するため。結果として、`TZ` を設定する前に初期化された共有 volume は旧クラスタ既定を保持し、
  そこでリースしたスロットは `psql` でその timezone を表示する（アプリは接続ごとに DSN で timezone を
  指定するため影響を受けない）。新しい既定を反映するには、全スロットを解放した上で volume を作り直す
  （`docker compose -p gobp-shared down -v` → `make db-init`）。volume は全 worktree の共有物である点に
  注意。詳細は `env/README.md` の Changing the Timezone を参照。
- **スキーマ安全性**: acquire は取得後に `wt<N>_local` / `wt<N>_test` を drop→migrate→seed で
  自ブランチのスキーマへ作り直す。別ブランチが使ったスロットを引き継いでも安全。
- **スキーマ生成の隔離**: `make gen-query` の `dump-schema` は共有 `local` も自分の作業用データベースも
  ダンプせず、使い捨てデータベース（`SCHEMA_GEN_DB` — スロット保持時は `gen_schema_wt<N>`、主 checkout は
  `gen_schema`）を drop → 当該ブランチの migration で migrate-up してからダンプする。決定的なダンプには
  その直前の無条件な drop → migrate が要るが、それは作業用データベースへは打てない（`gen-query` の
  たびに seed が消え、起動中の `make serve` の足元からテーブルが消える）。使い捨てデータベースも他と
  同じく所有者ごとに持つため、2 つの checkout が同時に `make gen-query` を走らせても同じデータベースを
  互いに作り直すことは無くなった。ローカル専用のガードで、CI は fresh な postgres service を migrate 済みに
  して `dump-schema-ci` を直接呼ぶため本経路は通らない。
- **解放**: `slot-free` は、このスロットの app プロジェクト（`gobp-wt-N`）のコンテナを停止して
  から lease と `.gobp-db-slot` を削除する。データベースは warm 保持で次に貸す。
- **stale 回収の安全化**: heartbeat が TTL（既定 1800 秒、`GOBP_DB_POOL_TTL`）超過したリースは acquire 時に
  別 worktree が再取得できる（crash した worktree がスロットを握り続けない）。heartbeat は `make serve` 時に
  しか打たないため、起動しっぱなしの app を持つスロットも TTL 超過で stale になる。そこで DB を作り直す前に
  そのスロットが実際に使われていないかを 2 段で確かめ、使用中なら破壊せず skip する:
  1. app プロジェクト（`gobp-wt-N`）に稼働中コンテナが無いこと
  2. `pg_stat_activity` にそのスロットの DB への接続が無いこと

  接続プールはアイドルで空になるため、2 だけでは serve 中の worktree を見落とす。逆に 1 だけでは
  ホスト実行の `go test` を捉えられないため、両方を見る。

## 使い方

スロットを取らない checkout（主 checkout など）は、そのまま `make serve` すれば共有インフラへ繋がる。

```sh
make serve           # 共有インフラを起動し app を gobp-app-<dir> で起動 → curl localhost:8080
make serve-stop      # この checkout の app だけ停止（共有インフラは残す）
make infra-up        # 共有インフラだけ起動
make infra-down      # 共有インフラを停止（全 checkout に影響する）
```

worktree で並列に作業するときはスロットを取る。

```sh
make slot-acquire    # 空きスロットをリースし自 worktree DB を作成/再構築
make test            # ホストから localhost:5432 経由で wt<N>_test へ接続
make serve           # app を gobp-wt-N で起動 → curl localhost:$API_HOST_PORT（DB は共有の wt<N>_local）
make slot-status     # スロット占有状況（DB 名 / API ポート）を表示
make slot-free       # スロットだけを解放（データベースは warm 保持、worktree は残す）
```

作業が終わって worktree ごと畳むときは `slot-release` を使う。app の停止とローカルビルド
イメージの削除、スロット解放、worktree 削除をこの順で行う。

```sh
make slot-release    # app 停止+イメージ削除 → スロット解放 → worktree 削除
```

順序は入れ替えられない。`slot-free` が `.gobp-db-slot` を消すと `SERVE_PROJECT` が失われ、
`APP_PROJECT` が `gobp-app-<dir>` へフォールバックするため、先に解放すると app の停止が
別プロジェクトを対象にしてしまう。`git worktree remove` は cwd ごと消すので必ず最後に置く。
未コミット・未追跡ファイルがあれば git が削除を拒否するので、`--force` は付けていない。
主 checkout で誤って叩いた場合は、何もせずエラー終了する。

## 環境変数

| 変数 | 既定 | 意味 |
| --- | --- | --- |
| `GOBP_DB_POOL_DIR` | `~/.cache/gobp-db-pool` | リースレジストリの置き場所（symlink は拒否） |
| `GOBP_DB_SHARED_PROJECT` | `gobp-shared` | 共有インフラの固定 compose プロジェクト名 |
| `GOBP_API_POOL_BASE` | `8080` | `API_HOST_PORT` のベース（スロット N = ベース+N） |
| `GOBP_MOCK_AUTH_POOL_BASE` | `2010` | `MOCK_AUTH_HOST_PORT` のベース |
| `GOBP_DLV_POOL_BASE` | `2345` | `DLV_HOST_PORT` のベース |
| `GOBP_PPROF_POOL_BASE` | `6060` | `PPROF_HOST_PORT` のベース |
| `GOBP_DB_POOL_MAX` | `12` | スロット数（=同時並列数の上限） |
| `GOBP_DB_POOL_TTL` | `1800` | stale 判定の heartbeat 猶予（秒） |

## 注意

- **共有インスタンスの blast radius**: 全 checkout が 1 個の Postgres / o11y / オブジェクトストレージを共有する。
  データベースは分離されるため DDL は DB を跨げず、`db-init` / `db-local-reinit` / `db-test-reinit` も
  `DB=local` / `DB=test` の直書きをやめて保持スロットから対象を解決するようになったため、作り直すのは
  自分のデータベースになる。ただし `DB=` を明示すれば従来どおり上書きでき、他人の所有名かどうかは
  検査しない。他 checkout を壊しうる経路として残っているのは `make db-reinit DB=<名前>` だけである。
  `make infra-down` も同様に全 checkout を止める。
- **テストの並列実行が奪い合うのは容量ではなく接続の確立**: テストが同時に走ると、変更と無関係な
  パッケージが `failed to ping DB` で落ちる一方、`too many clients` は出ない。インスタンスの接続数には
  余裕があり、飽和するのは**同時に確立しようとしている本数**で、待たされている間に ping の予算が尽きる。
  worktree が 2 つ要るわけでもない — lefthook の `pre-commit` / `pre-push` は `parallel: true` なので、
  単一 checkout でも `make lint` と `make test` が重なる。テスト経路にはすでに対策が入っている — `ci` が
  何をどう設定しているかは `env/README.md` の `DBCONN_MIN_CONNS` / `DB_PING_TIMEOUT` を参照。env ファイルを
  読まない経路は `internal/config` のテスト用設定が同じ値を持つ。
  再発時の切り分けは、`pgrep -fl "go test"` → `lsof -a -p <pid> -d cwd` でどの checkout がテストを
  走らせているかを特定し、`pg_stat_activity` をサンプリングしてピークが定常ではなく一過性のスパイクかを見て、
  `go test -p 1` で green になるなら原因は負荷であって変更ではない、という順に見る。
- **infra 層の再作成**: compose はコンテナが最新かどうかを、解決後のサービス定義のハッシュで判定する。
  このハッシュには bind mount の source と build context が**絶対パス**で入るため、どの worktree も
  自分のディレクトリ配下へ解決した値を持つ。結果として、まったく同じコミットの checkout 同士でも
  ハッシュは一致しない。再作成はブランチが分岐したときの例外ではなく常態である。影響を受けるのは
  `database` と `garage`（`docker/database/sql` と `docker/garage/garage.toml` を bind mount する）で、
  何もマウントしない `observability` は受けない。そのため worktree から `gobp-shared` へ `up` する際は
  `--no-recreate` を渡し（`.makefiles/docker/compose.mk` の `INFRA_NO_RECREATE`）、他の checkout が
  使っているコンテナを置き換えずそのまま使う。
  代償として、image の digest pin 更新や `garage.toml` の編集といった**正当な定義変更も自動では
  反映されなくなる**。全 checkout が中断を許容できるタイミングで `make infra-down && make infra-up`
  を実行すること。`tools` プロファイルも同じで、`docs_server` は最初に作った checkout の `docs/` を
  配り続ける。単一 checkout には奪い合う相手が居ないためフラグは空で、compose は従来どおり
  定義変更へ再収束する。
- **オブジェクトストレージは共有**: `garage` のバケットは全 checkout で共通（DB と違いスキーマを持たないため
  ブランチ間で壊れない）。ブランチ毎に隔離したい場合は `OBJECT_STORAGE_BUCKET` を分ける。
- **キューは共有で、設定だけでは隔離できない**: `elasticmq` は全 checkout へ同じキュー群を提供する。
  オブジェクトストレージのバケットと違い `OUTBOX_QUEUE_URL` を別名に向けるだけでは足りない。
  ElasticMQ は `docker/elasticmq/elasticmq.conf` に宣言されたキューしか作らず、環境変数も展開しない
  ため、どこにも宣言の無い名前は単に存在しないからである。2 つの checkout が同時に
  `make outbox-relay` を回せば同じキューへ publish し、先に読んだ consumer がメッセージを取る。
  worker を登録している場合は、2 つの checkout が relay を回すと実際に競合する。ブランチ毎に隔離するなら `elasticmq.conf` へキューを追加し `OUTBOX_QUEUE_URL` を
  そこへ向ける。conf は起動時に読まれるので `make infra-down && make infra-up` が要り、全 checkout を
  止めることになる。スロット毎のキューを事前宣言していないのは、プールのサイズが可変
  （`GOBP_DB_POOL_MAX`）で、conf 側の静的な一覧はその値が変わった時点で黙ってプールを覆わなくなる
  ためである。
- **Realtime Delivery のエミュレータは共有インスタンスで、名前によって隔離される。** どちらも 2 つの checkout を
  分けるのに conf の変更も `infra-down` も要らない点で、上の `elasticmq` とは異なる。`dynamodb_local` は
  `-sharedDb` で動くため単一の table namespace を全員が使い、`goaws` は topic / queue を実行時に作る
  （`docker/goaws/goaws.yaml` は何も宣言しない）。どちらの場合もブランチは別々の名前を選ぶことで自分を
  隔離でき、`internal/cli/dbslot` の `Resolver` がその名前を選ぶ役を担う。`REALTIME_TOPIC`・
  `REALTIME_QUEUE_PREFIX`・`REALTIME_TABLE_SUFFIX` にスロットを付けて 3 つとも `db-slot env` から出すため、
  各 worktree の serve は自分の topic を購読し、自分の table を読む（`realtime_event_log_local_wt<N>` と
  その他 2 つの table。`make realtime-init` が同じ値から作成する）。`docker-compose.attach.yaml` は導出を
  二度書かずにその値を読むだけで、ホスト公開ポートと同じくスロット未取得のときの既定値だけを持つ。
  したがってスロットを持たない checkout は suffix なしの名前のままで、メイン checkout は影響を受けない。
  table の suffix だけ topic / queue が使う `-` でなく `_` で結合されるのは、`REALTIME_TABLE_SUFFIX` が
  小文字・数字・アンダースコアしか受け付けないためである。

  両方とも欠かせず、無い場合の壊れ方は異なる。table の suffix が無いと、ある worktree が書いた stream が
  別の worktree からも見えてしまう。topic の suffix が無い場合はより限定的だがより意外な壊れ方をする —
  知らない stream への wakeup は no-op だが、失効通知は `(subject, destination)` で照合され、どの worktree
  の DB も同じ fixture から seed されて subject が一致するので、ある窓で ticket を失効させると別の窓の
  同じ subject の接続が閉じてしまう。`make realtime-smoke` も同じ形で、実行ごとの乱数名で resource を作り
  終了時に削除する。
- **スロットの 2 つの store は一緒に作り直す。** Realtime Delivery は stream の採番を PostgreSQL で行い、
  配送済みの event を DynamoDB EventLog に置くため、片方だけ作り直すと採番が EventLog に既にある番号を
  発行することになる。`Append` はその位置を拒否する — 同じ sequence に別の event ID があるのは本物の
  衝突なので、それが正しい — が、そのあと relay の head-of-line blocking がその stream を止める。
  そこで **local** データベースを作り直す経路はすべて `make realtime-reset` で終わり、その checkout の
  3 table を落とす — 危険はスロットではなくデータベースに付いて回るので、`slot-acquire` も
  `db-local-reinit` も `db-init-local` も同じである。`realtime-provision` が次の `make serve` で
  作り直す。test データベースには対になる store が無いため、その再作成に reset は付かない。
  衝突し得るのは固定の識別子を持つ stream だけで（subject ごとの stream は再 seed で新しい UUID に
  なる）、この失敗が当たり前ではなく紛らわしいものになっているのはそのためである。
- `sql_editor` / `docs_server` / `er_diagram_generator` / `mock_auth_server` は、いずれも自前のデファクト
  ポートを持たないため `2000` 番台に置いている。規則とその帯が安全な理由は
  [`local-environment.ja.md`](local-environment.ja.md) にある。
- `docker/`・`internal/cli/dbslot`・`.makefiles/` を含む配線のため、変更時はこのドキュメントも更新すること。
