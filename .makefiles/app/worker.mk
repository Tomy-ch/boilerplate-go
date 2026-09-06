## 常駐プロセス(worker / outbox-relay)関連
.PHONY: worker ## worker を起動（app コンテナ内で常駐実行。停止は Ctrl-C）
.PHONY: outbox-relay ## outbox relay を起動（app コンテナ内で常駐実行。停止は Ctrl-C）

# job と同じ理由で所有者を要求する（.makefiles/app/job.mk 参照）。
worker: require-db-owner
	@test -n "$(NAME)" || { echo "❌ NAME は必須です。例: make worker NAME=sampleworker"; exit 1; }
	@echo "🏃 worker を起動します: $(NAME) $(ARGS)（停止は Ctrl-C）"
	@$(MAKE) infra-up
	@$(COMPOSE_APP) run --rm api_server go run ./cmd/ worker $(NAME) $(ARGS)

outbox-relay: require-db-owner
	@test -n "$(ARGS)" || { echo "❌ ARGS は必須です。例: make outbox-relay ARGS=\"--channel=http\""; exit 1; }
	@echo "🏃 outbox relay を起動します: $(ARGS)（停止は Ctrl-C）"
	@$(MAKE) infra-up
	@$(COMPOSE_APP) run --rm api_server go run ./cmd/ outbox-relay $(ARGS)
