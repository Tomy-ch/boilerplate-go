## アプリケーションジョブ関連
.PHONY: job ## Jobを実行（app コンテナ内で実行）

# COMPOSE_APP が撒く解決値へ実際に接続するため、serve と同じく所有者を要求する
# （不変条件は docs/maintenance/db-worktree-pool.md「The invariant」）。
job: require-db-owner
# sample-api:replace-begin
	@test -n "$(NAME)" || { echo "❌ NAME は必須です。例: make job NAME=usercount"; exit 1; }
# sample-api:replace-with
# = 	@test -n "$(NAME)" || { echo "❌ NAME は必須です。例: make job NAME=idempotency-gc"; exit 1; }
# sample-api:replace-end
	@echo "🏃 Jobを実行します: $(NAME) $(ARGS)"
	@$(MAKE) infra-up
	@$(COMPOSE_APP) run --rm api_server go run ./cmd/ job $(NAME) $(ARGS)
	@echo "✅ Jobが完了しました。"
