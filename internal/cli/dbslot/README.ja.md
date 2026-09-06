# db-slot

単一の共有インフラ上で per-worktree のスロットをリースし、複数の `git worktree` がホストポート衝突なく DB を使う並列作業を可能にする。各スロットは共有 DB コンテナ（固定 infra compose プロジェクト `gobp-shared`・ホスト 5432）内の専用データベース（`wt<N>_local` / `wt<N>_test`）と、ホスト公開ポートをスロット番号で相対化した専用の app 層 compose プロジェクト（`gobp-wt-N`）を占有する。スロットを取らない checkout も同じ共有インフラへ繋がるため、スロット取得は**並列作業を衝突なく行うための opt-in** に留まる。全体像は `docs/maintenance/db-worktree-pool.md` を参照。

**ホスト**で実行する（tool-runner コンテナ内ではない）: ホストのファイルシステム上のリースを管理し、ホストの `docker compose` を駆動し、共有 DB へは pgx で `localhost:5432` へ接続するため。

## コマンド

```text
db-slot acquire        # 空きスロットをリースし wt<N> DB を作成/設定して .gobp-db-slot を書き出す
db-slot release        # スロットの app コンテナを停止しリースを解放（DB は warm 保持）
db-slot heartbeat      # 保持スロットのリース heartbeat を更新
db-slot status         # スロット占有状況に続けて、この checkout の解決済み値を表示
db-slot env            # 解決済み値を make が eval するための KEY=VALUE として出力
db-slot require-owner  # この checkout がデータベースを所有していなければ失敗（終了コードが interface）
```

通常は make ラッパ（`make slot-acquire` / `slot-free` / `slot-release` / `slot-status`）を使う。`slot-acquire` はリースした DB のスキーマ再構築も行い、`slot-release` は worktree ごと撤収する。

## 設計

- **リース**（`Registry`）— `~/.cache/gobp-db-pool`（`GOBP_DB_POOL_DIR` で上書き可）配下のスロット毎ロックディレクトリ。`os.Mkdir` の原子性で新規取得、stale 回収は `rename` で原子的に占有権を奪い、`flock` の走査ロックで acquire ループ全体を直列化して 2 worktree が同一スロットを二重リースしないようにする。symlink を向いた pool dir は拒否（先読み攻撃対策）、meta は `0600`。
- **DB 管理**（`DBAdmin` / `PgxAdmin`）— 各 `wt<N>` DB への `CREATE DATABASE` ＋ `pg_trgm` 拡張設定、および stale スロットを reclaim してよいかの判断に使う `pg_stat_activity` の接続確認。timezone は含まない: `database` コンテナの `TZ` がクラスタ既定であり、後から作った DB もそれを継承する。
- **使用中判定** — stale スロットを reclaim する前に、app プロジェクト（`gobp-wt-N`）の稼働中コンテナと、その DB への接続の両方を確認する。接続プールはアイドルで空になるため接続確認だけでは serve 中の worktree を見落とし、コンテナ確認だけではホスト実行の `go test` を捉えられない。
- **Compose**（`Compose` / `ExecCompose`）— 共有 DB を固定 infra プロジェクトで起動（`--wait --no-recreate`）し、release 時にスロットの app プロジェクト（`gobp-wt-N`）を `docker-compose.yaml` だけで、app 層のサービスが属する `development` profile を付けて停止する。profile を渡さない `down` は対象を 1 つも選ばずに exit 0 で返るため、成功を報告した release のあともコンテナが生き残る。app 層の override をこの `down` に重ねないのは意図的である。override が変えるのは `api_server` の ports / `depends_on` / environment だけで `down` はそのいずれも参照しない一方、override の `REALTIME_*` は値を必須（`:?`）とし、それを撒くのは `make` の `COMPOSE_APP` だけなので、重ねると `slot-free` が補間段で落ちる。補間を通すためだけにダミー値を注入する案も同じ理由で却下した。`down` はその値を一切読まないので、このコマンドが必要としないファイルを満たすためだけに値が存在することになる。`--no-recreate` は、他の checkout が使っているコンテナを `acquire` が置き換えないためのもの。ここで条件を持たない理由は [`docs/maintenance/db-worktree-pool.md`](../../../docs/maintenance/db-worktree-pool.md) を参照。
- **スロットファイル** — `acquire` が `.gobp-db-slot`（gitignore・`make` が `-include` で読む `KEY=VALUE`）を書き出し、`.makefiles/docker/compose.mk` の既定値を上書きする。内容は `SLOT`、`DB_NAME_LOCAL` / `DB_NAME_TEST`、スロット相対のホスト公開ポート `API_HOST_PORT` / `MOCK_AUTH_HOST_PORT` / `DLV_HOST_PORT` / `PPROF_HOST_PORT`、`COMPOSE_PROJECT_NAME`（共有 infra プロジェクト）、`SERVE_PROJECT`（`gobp-wt-N`＝app 層プロジェクト）。このファイルはリースの証明ではない。stale として回収されたスロットの前保持者にも残り続けるため、`Resolver` も `release` も、ファイルの宣言に従って動く前にレジストリを確認する。
- **解決済み値**（`Resolver`）— *スロットの関数*であるものは `make` や compose ファイルではなくここで導出する: `DB_LOCAL` / `DB_TEST`、app 層の compose プロジェクト、mock-auth の issuer URL、`INFRA_NO_RECREATE`、そして 2 つの worktree を互いの stream から引き離す Realtime Delivery の名前（`REALTIME_TABLE_SUFFIX` / `REALTIME_QUEUE_PREFIX` / `REALTIME_TOPIC`）。Realtime の名前には消費者が 2 つある — app へ渡す `docker-compose.attach.yaml` と、その名前の table を落とす `make realtime-reset` — ので、まさにそれゆえ導出を 2 度書いてはならない。いずれもレジストリがリースを確認したスロット番号から導き、`.gobp-db-slot` に並んでいる文字列は読まない。その 3 つが継ぐ基底名は埋め込み `env/.env` だけから取り（`LoadRealtimeBase`）、プロセスの環境変数とは混ぜない。`db-slot env` は同じ 3 変数を撒くため、読み戻す resolver は、出力を export したシェルで 2 度目に走ったときに既にスロットを継いだ名前へさらに継いでしまう（`local_wt2` → `local_wt2_wt2`）。application の「実行時 env を優先する」規則（`config.Load`）が意図的に当てはまらない唯一の場所である。空の基底は素通しせず拒否する。`realtimeName` は空の基底にスロットを継がないため、撒かれる値も表示される値も主 checkout の資源を指してしまうからである。override 側も空を拒む（`:?`）が、そちらのエラーは compose の補間段のもので、埋め込み env のどのキーが欠けているかへは辿り着けない。1 つの導出が `db-slot env`（`make` が eval する値）と `db-slot status`（人間が読む値）の両方を賄うため、表示される値と実際に使われる値が食い違うことはない。`DB_LOCAL` / `DB_TEST` だけは `.gobp-db-slot` を読む `make` 変数としても保持する。target-specific な代入（`db-local-migrate-up: DB=$(DB_LOCAL)`）はパース時に評価され、どのレシピよりも先に決まるからである。
- **所有権ガード**（`RequireOwner`）— `make require-db-owner` の実体で、「リースを持たないリンク worktree」を検出する。判定は `.gobp-db-slot` の存在ではなくレジストリによる。依拠する区別は 2 値ではなく 3 値である。**git が無い**（ツールランナーのコンテナ）と**リポジトリでない**は素通りし、**リポジトリではあるのに構成を読み取れない**場合は失敗する。理由は [`docs/maintenance/db-worktree-pool.md`](../../../docs/maintenance/db-worktree-pool.md) を参照。
- **env ガード** — `APP_ENV` が空、または `local` / `ci` / `test` のいずれかでなければ実行を拒否する（`config.IsLocalClassEnv`）。pool は DB を作成/破棄するため dev/test 専用ツールに留める。許可リスト方式なので `dast` や未知の値も拒否される。

## Test Strategy

上位層の Testing Policy は、判断ロジックをダブルに対してテストできるよう、依存をすべて seam の裏へ押し出す。ここでも判断ロジックはそれに従う — ただし seam の実装自体はアダプタであり、アダプタは実物を駆動して初めて価値を持つ。したがって各コンポーネントは、その subject が実際に存在する層でテストする。

- **`Pool`**（判断ロジック） — 生成 mock（`MockDBAdmin` / `MockCompose`）に対する単体テスト。Postgres にも docker にも到達しない。acquire / release / reclaim の全分岐をここで固定する。「片方だけでは取りこぼす」ことこそが要点である 2 段構えの in-use 判定も含む。
- **`Resolver`**（判断ロジック） — スタブ `GitProbe` に対する単体テスト。1 台のマシンから git の 4 文脈すべてに到達する唯一の方法である。実機は常にそのうちのちょうど 1 つでしかなく、危険なケース（git が読み取れないリポジトリ）は望んだときに再現できない。所有権の分岐についても同様にスタブ `LeaseProbe` を使い、保持・回収済み・確認不能のそれぞれへ、実体のレジストリを用意せずに到達する。
- **`Registry`** — `t.TempDir()` 上の実ファイルシステムプリミティブ。subject が `os.Mkdir` / `os.Rename` の**原子性そのもの**なので、FS を fake にしても何も証明できない。二重リースはまさに fake が覆い隠してしまう類の欠陥である。
- **`ExecCompose`** — `PATH` の先頭に仕込んだスタブ `docker` が、組み立てられた引数列と `COMPOSE_PROJECT_NAME` を記録する。実 compose を走らせずにコマンド構築と環境変数注入を固定する。`PATH` への `t.Setenv` を使うため、これらのケースは `t.Parallel()` と両立しない。
- **`PgxAdmin`** — `DBAdmin` の唯一の実装。共有 Postgres（`localhost:5432`）に対してテストする。その SQL が実際に実行できることを証明する唯一の網である。到達不能ホストのケースはサーバ無しでエラー経路を固定し、作成した DB は cleanup で drop するので実行を繰り返せる。

基準はパッケージではなく subject にある。契約が**判断**であるコンポーネントはダブルに対して、契約が**外部基盤の振る舞い**であるコンポーネントはその基盤に対してテストする。結果としてここのテストは純粋な単体テストのパッケージより遅く、共有 infra の起動を必要とする。

## 環境変数

|変数|既定|説明|
|---|---|---|
|`GOBP_DB_POOL_DIR`|`~/.cache/gobp-db-pool`|リースレジストリの置き場所|
|`GOBP_DB_SHARED_PROJECT`|`gobp-shared`|共有インフラの固定 compose プロジェクト|
|`GOBP_DB_POOL_MAX`|`12`|スロット数（＝同時並列 worktree の上限）|
|`GOBP_DB_POOL_TTL`|`1800`|heartbeat stale 判定の猶予（秒）|
|`GOBP_API_POOL_BASE` / `GOBP_MOCK_AUTH_POOL_BASE`|`8080` / `2010`|API / mock 認証サーバーのホストポートのベース（スロット N = ベース + N）|
|`GOBP_DLV_POOL_BASE` / `GOBP_PPROF_POOL_BASE`|`2345` / `6060`|dlv デバッグ / pprof のホストポートのベース（スロット N = ベース + N）|
|`GOBP_DB_POOL_PGHOST` / `GOBP_DB_POOL_PGPORT`|`localhost` / `5432`|pool が管理する Postgres の接続先|
|`GOBP_DB_POOL_PGUSER` / `GOBP_DB_POOL_PGPASSWORD`|`postgres` / `postgres-password`|`CREATE DATABASE` に使う資格情報|
|`GOBP_DB_POOL_PGMAINTDB`|`postgres`|作成 / 破棄の際に接続する保守用データベース|

## 注意

- スロットを取らない checkout も同じ共有インフラへ繋がる（既定の `local` / `test` データベースと既定のホスト公開ポートを使うだけ）。衝突なく並列作業したいときにだけスロットを取る。
