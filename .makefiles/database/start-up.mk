## DB関連のコマンドの一括実行
.PHONY: db-init ## DBの初期化を行う(マイグレーション、シードデータ投入)
.PHONY: db-init-local ## LocalDBの初期化を行う(マイグレーション、シードデータ投入)
.PHONY: db-init-test ## TestDBの初期化を行う(マイグレーション、シードデータ投入)

db-init:
	@echo "🔄 DB初期化関連のコマンドを実行します..."
	@$(MAKE) db-init-local
	@$(MAKE) db-init-test
	@echo "✅ DB初期化関連のコマンドが完了しました。"

db-init-local:
	@echo "🔄 localDBを初期化します..."
	@$(MAKE) db-local-migrate-down
	@$(MAKE) db-local-migrate-up
	@$(MAKE) db-local-seed
	@$(MAKE) realtime-reset
	@echo "✅ localDBの初期化が完了しました。"

db-init-test:
	@echo "🔄 testDBを初期化します..."
	@$(MAKE) db-test-migrate-down
	@$(MAKE) db-test-migrate-up
	@$(MAKE) db-test-seed
	@echo "✅ testDBの初期化が完了しました。"

## ダーティなスキーマの再構築（db-init の migrate-down 非依存版）
.PHONY: db-drop-tables ## public の全テーブルを削除する（拡張は残す）。migrate-down が使えない状況の保守用
.PHONY: db-reinit ## drop-tables → migrate-up → seed でクリーン再構築（migrate-down 非依存）
.PHONY: db-local-reinit ## localDB を drop-tables → migrate-up → seed で再構築
.PHONY: db-test-reinit ## testDB を drop-tables → migrate-up → seed で再構築

# db-init（migrate-down → up）は down マイグレーションを要するため、サンプル削除等で down ファイルが
# 失われた「ダーティなスキーマ」には使えない。db-drop-tables / db-reinit はその代替で、public の全
# テーブルを CASCADE で削除してから migrate-up し、down に依存せずクリーンな状態へ戻す。
# 本リポジトリのマイグレーションは table のみを作るため、テーブル削除で十分（拡張は温存される）。
db-drop-tables: require-db-owner
	@echo "💥 public の全テーブルを削除します（拡張は残す）... (database=$(DB))"
	@docker compose exec -T database psql -U postgres -d $(DB) -v ON_ERROR_STOP=1 < database/maintenance/drop-all-tables.sql
	@echo "✅ 全テーブル削除が完了しました。 (database=$(DB))"

db-reinit:
	@$(MAKE) db-drop-tables DB=$(DB)
	@$(MAKE) db-migrate-up DB=$(DB)
	@$(MAKE) db-seed DB=$(DB)

# local DB を作り直す経路には realtime-reset が付く。採番は PostgreSQL、配送済み event は
# DynamoDB EventLog にあり、片方だけ巻き戻すと採番が既存 item の位置へ届いて stream が止まる
# （docs/maintenance/db-worktree-pool.md「A slot's two stores are reset together」）。
# test DB 側には対になる store が無いので付けない。
db-local-reinit: DB=$(DB_LOCAL)
db-local-reinit: db-reinit
	@$(MAKE) realtime-reset

db-test-reinit: DB=$(DB_TEST)
db-test-reinit: db-reinit
