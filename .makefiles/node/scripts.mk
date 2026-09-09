## Node 補助スクリプト(TypeScript)のテスト/型検査コマンド群
#
# テストは Go 側（test / go-test-cached）と同じくフルとキャッシュ有効の 2 系統に分ける。
# フル側はカバレッジ閾値（scripts/vitest.config.mts の thresholds）まで見るため、判定分岐を
# 足してテストを書き忘れた変更をここで止められる。キャッシュ有効側は毎回走る経路（pre-push）
# 用で、閾値の判定は持たない。
# -----Dockerコンテナ内で実行するコマンド群-----
.PHONY: scripts-test ## 補助スクリプト(scripts/**/*.ts)のテストをカバレッジ付き・キャッシュ無効で実行する
.PHONY: scripts-test-cached ## 補助スクリプトのテストをキャッシュ有効で実行する（pre-push向け）
.PHONY: scripts-typecheck ## 補助スクリプト(scripts/**/*.ts)の型検査を実行する
# -----CI内で実行するコマンド群-----
.PHONY: scripts-test-ci ## 補助スクリプトのテストをカバレッジ付き・キャッシュ無効で実行する（CI用）
.PHONY: scripts-test-cached-ci ## 補助スクリプトのテストをキャッシュ有効で実行する（CI用）
.PHONY: scripts-typecheck-ci ## 補助スクリプトの型検査を実行する（CI用）

# -----Dockerコンテナ内で実行するコマンド群-----
scripts-test:
	@echo "🔍 補助スクリプトのテスト（カバレッジ付き）を開始します..."
	@docker compose run --rm node_tool_runner make scripts-test-ci
	@echo "✅ 補助スクリプトのテストが完了しました。"

scripts-test-cached:
	@echo "🔍 補助スクリプトのテスト（キャッシュ有効）を開始します..."
	@docker compose run --rm node_tool_runner make scripts-test-cached-ci
	@echo "✅ 補助スクリプトのテストが完了しました。"

scripts-typecheck:
	@echo "🔍 補助スクリプトの型検査を開始します..."
	@docker compose run --rm node_tool_runner make scripts-typecheck-ci
	@echo "✅ 補助スクリプトの型検査が完了しました。"

# -----CI内で実行するコマンド群-----
# vitest / tsc は設定ファイルと同じ scripts/ を起動位置とするため、設定の探索も走査範囲も
# 呼び出し元のカレントディレクトリに左右されない。
scripts-test-ci:
	$(PNPM_SCRIPTS) test

scripts-test-cached-ci:
	$(PNPM_SCRIPTS) test:cached

scripts-typecheck-ci:
	$(PNPM_SCRIPTS) typecheck
