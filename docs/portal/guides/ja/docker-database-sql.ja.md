# Database 初期化用 SQL

`docker/database/sql/` は、**データベース環境を初期化するための SQL ファイル**を格納するディレクトリです。

PostgreSQL コンテナの起動時に `docker-entrypoint-initdb.d` 経由で実行され、マイグレーション**以前**に必要なセットアップ（データベース作成、拡張機能の有効化等）を行います。

## 現在のファイル

|ファイル|説明|
|---|---|
|`001-create-local-db.sql`|ローカル開発用データベースの作成|
|`002-create-test-db.sql`|テスト用データベースの作成|
|`003-init-extensions-local-db.sql`|ローカル DB の拡張機能の有効化|
|`004-init-extensions-test-db.sql`|テスト DB の拡張機能の有効化|

## 実行順序

ファイル名の **連番プレフィックスの昇順**（例: `001-...`, `002-...`）で実行されます。

PostgreSQL の `docker-entrypoint-initdb.d` の仕組みにより、コンテナ初回起動時に自動実行されます。

これらのスクリプトは固定の `local` / `test` データベースのみを作成・拡張初期化します。DB worktree
プール（`cmd/db-slot`・コア `internal/cli/dbslot`）は per-worktree のデータベース（`wt<N>_local` / `wt<N>_test`）を起動**後**に
動的作成するため、同じ拡張を自身でブートストラップします。この設定は `internal/cli/dbslot`（`PgxAdmin.SetupDatabase`）
と同期させてください。詳細は `docs/maintenance/db-worktree-pool.md` を参照。

timezone はここでは意図的に設定しません。`database` サービスの `TZ` 環境変数が `initdb` 時にクラスタ既定と
なり、後から作られるデータベースもそれを継承するため、スクリプトやツールが個別に繰り返す必要がありません。
詳細は `env/README.md` の Changing the Timezone を参照。

## ここに置くもの

- データベース作成（`CREATE DATABASE`）
- PostgreSQL 拡張のセットアップ（`CREATE EXTENSION`）
- ローカル / CI 環境に必要なロールと権限

## ここに置かないもの

- **DDL（テーブル定義、スキーマ変更）** → `database/migrations/`
- **DML（データ操作、クエリ）** → `database/dml/`
- **シードデータ（テスト / 開発用初期データ）** → `database/seed/`

## ルール

- 3桁の連番プレフィックスで順序を制御（例: `001-...`, `002-...`）
- 可能な限り**冪等**にする（`IF NOT EXISTS`）— 初回起動時に実行されるが、冪等にしておくとトラブルシューティングが容易
- CI / 本番環境への直接適用はチームのポリシーに従う
- **環境初期化に必要な最小限**のファイルのみ配置する
