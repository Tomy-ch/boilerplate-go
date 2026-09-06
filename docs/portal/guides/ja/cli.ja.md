# CLI コア

`internal/cli` は、アプリケーションの CLI コマンドの**純粋でテスト可能なコアロジック**を保持します。

Cobra や infrastructure の結線には依存しません。Cobra コマンド定義と、実依存（config / DB / DI /
OS シグナル / golang-migrate）を結線する composition root は `cmd/`（package `main`）に置きます。
この分割により、コアはユニットテスト可能なまま、結線は薄く保たれます。

## コマンド一覧

|コマンド|コアパッケージ|Cobra 殻|説明|
|---|---|---|---|
|`serve`|`server/`|`cmd/serve.go`|HTTP サーバーとメトリクスサーバーを起動|
|`migrate-up`|`migrate/`|`cmd/migrate.go`|DDL をアップグレード（`--steps` / `--database` 指定可）|
|`migrate-down`|`migrate/`|`cmd/migrate.go`|DDL をダウングレード（`--steps` / `--database` 指定可）|
|`db-seed`|`seed/`|`cmd/seed.go`|データベースに初期データを投入|
|`job`|`job/`|`cmd/job.go`|登録済みジョブを実行（`job <job-name> [args...]`）|
|`fix-collation`|`fixcollation/`|`cmd/fix_collation.go`|PostgreSQL の照合順序バージョン不一致を修正|
|`dump-schema`|`dumpschema/`|`cmd/dump_schema.go`|DB スキーマをダンプして整形|
|`merge-dml`|`mergedml/`|`cmd/merge_dml.go`|DML ディレクトリの SQL ファイルを種別ごとにマージ|
|`worker`|`worker/`|`cmd/worker.go`|登録済み worker を起動（`worker <worker-name> [args...]`）|
|`outbox-relay`|`outbox/`|`cmd/outbox_relay.go`|outbox relay を起動。`replay` サブコマンドは dead 行を pending へ戻す|
|`realtime-init`|`realtimeinit/`|`cmd/realtime_init.go`|Realtime Delivery の table（EventLog / StreamTicket / InstanceLease）を DynamoDB 互換 store に、fan-out の topic を SNS 互換 broker に作る。冪等な one-shot で、application の起動時には走らない|
|`db-slot`|`dbslot/`|`cmd/db-slot.go`|共有 worktree プールから DB スロットをリースする（`acquire` / `release` / `heartbeat` / `status` / `env` / `require-owner`）|

## 構造

サブコマンドごとに 1 つのディレクトリを置き、その名前を付ける。配線する `cmd/<name>.go` と対になる。
この対応こそが要点である。`cmd/` は Cobra の殻と合成ルートに留めてカバレッジ対象から外し、テスト可能な
中身はこちらに置いて他のパッケージと同様に計測する。

`cmd/commands.go` の `registerCommands` で全サブコマンドを Cobra のルートコマンドに登録します。

## 設計方針

- 各コマンドは `internal/cli/` 配下の1コアパッケージ + `cmd/` 配下の1薄殻ファイルで構成。
- コアは Cobra・`internal/di`・OS シグナル・infrastructure（`infrastructure/rdb/driver` の型を除く）を
  import してはならない。注入された interface / 関数シームに対して動作する。
  `internal/config` は許可される。強制されている境界は `.golangci-full.yaml` の `independent_cli`
  depguard ルールであり、上位の層は deny するが `config` は deny しない。
- CLI 層は feature のビジネスロジックを持たない（それは usecase / domain の責務）。
- コマンド追加手順: `cmd/<command>.go`（Cobra 定義 + 実依存の結線）を追加し、コアロジックを
  `internal/cli/<command>/` に追加し、`registerCommands` に登録する。

## テスト方針

コアは **humble object**: 判断ロジックは全てここに置きユニットテストし、結線は `cmd/` へ押し出す。
パッケージ境界＝テスト境界。

- **殻に silent-wrong なロジックを置かない。** 判断（エラー処理・分岐・整形・削除可否・タイムアウト分岐）は
  `internal/cli/*` に置き、**分岐網羅でユニットテスト**する。`internal/cli/*` はカバレッジゲート対象で 90%+ を満たす。
- **OS / FS / 外部プロセス / DB / ロガー依存は注入する**（interface または関数シーム）。プロダクションは
  `cmd/` で実装を結線し、ユニットテストはフェイクを渡す。よって**判断ロジックのテストは実ファイルシステムに
  触れず、外部バイナリ（`pg_dump` / `psql`）を実行せず、DB も開かない**。唯一の例外は**シームの裏にある
  実装アダプタ**で、その契約は「外部基盤が期待どおり振る舞うこと」であり、フェイクでは確立できない。
  よって実物に対してテストしてよい。これを行うパッケージは自身の README でその切り分けを宣言する
  —— [`dbslot/`](dbslot/README.ja.md) を参照。
- **薄い `cmd/` 殻はカバレッジゲートから除外**（`gen|cmd|mock|apperror|scripts`）。その実行時の正しさは
  CI boot チェックで担保: `app-di-startup-check`（serve → `/ready`）、`job-boot-check`（job dispatch）、
  `worker-boot-check`（worker dispatch）、`outbox-relay-boot-check`（relay の起動と drain）、
  `migration-check`（up/down 往復）、`gen-*-artifacts-check`
  （codegen の dogfooding）——いずれも実 Postgres。
  DB アクセス挙動は実 Postgres に当てた repository テスト（`internal/infrastructure/rdb/testkit`）で担保。

### コマンドを追加するとき

- `cmd/` 殻は薄く保つ: フラグ解釈 → 依存の組み立て → コア関数へ委譲。
- フラグは**ローカル変数**に束縛する（パッケージグローバルにしない）。並列テスト安全のため。
- OS / FS / exec / DB / ロガーは interface または引数で注入し、実装は `cmd/` で渡す。
- コアを分岐網羅でユニットテストし、薄い `cmd/` 殻は CI ゲートに委ねる。

## 注意点

- マイグレーションやシード操作は、実行前にバックアップを取ることを推奨。
- サーバー起動時の設定は環境変数で管理（`internal/config` 参照）。
