## DBに対する修正コマンド群
# -----Dockerコンテナ内で実行するコマンド群-----
.PHONY: sql-fix-collation ## データベースのコラテーションを修正
# ----CI用ターゲット-----
.PHONY: sql-fix-collation-ci ## データベースのコラテーションを修正（CI用）

# -----Dockerコンテナ内で実行するコマンド群-----
sql-fix-collation:
	@echo "🔄 データベースのコラテーションを修正します..."
	@docker compose run --rm go_tool_runner make sql-fix-collation-ci
	@echo "✅ データベースのコラテーション修正が完了しました。"

# ----CI用ターゲット-----
sql-fix-collation-ci:
	go run ./cmd/ fix-collation
