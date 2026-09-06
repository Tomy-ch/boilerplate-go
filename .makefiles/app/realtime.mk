## Realtime Delivery 関連
.PHONY: realtime-init ## Realtime Delivery の table（EventLog / StreamTicket / InstanceLease）と fan-out の topic を作る（冪等 one-shot）
.PHONY: realtime-provision ## 共有インフラ起動済みを前提に Realtime Delivery の資源だけを用意する（serve から呼ぶ内部用）
.PHONY: realtime-reset ## この checkout が使う Realtime Delivery の table を削除する（作成は realtime-provision）
.PHONY: realtime-smoke ## DynamoDB Local / GoAWS へ AWS SDK v2 で native 接続できるかを smoke で確認する
.PHONY: realtime-contract-test ## Realtime Delivery の contract test を実行する（既定は DynamoDB Local / GoAWS。REALTIME_TEST_* で本番 AWS へ向け直す）

# table と topic は application の起動時に作らない（docs/design/realtime-delivery.md）。app コンテナ内で実行するので
# env/.env の ENDPOINT_REALTIME / ENDPOINT_REALTIME_PUBSUB（compose のサービス名）がそのまま使える。
realtime-init:
	@echo "🔄 Realtime Delivery の table と topic を作成します..."
	@$(MAKE) infra-up
	@$(MAKE) realtime-provision
	@echo "✅ Realtime Delivery の table と topic が揃っています。"

# serve は起動時に EventLog へ到達できることを確かめ、instance の受信先を topic へ subscribe する。
# どちらも資源が無ければ起動に失敗するので、garage のバケットと同じく「消費者が触る前」に用意する。
# 冪等なので毎回走ってよい。infra-up 側に置かないのは、realtime-smoke など app イメージを要さない
# 呼び出しにビルド待ちを持ち込まないため。
realtime-provision:
	@$(COMPOSE_APP) run --rm api_server go run ./cmd/ realtime-init > /dev/null

# slot-acquire が PostgreSQL を作り直すのと対で EventLog を空にする
# （理由は docs/maintenance/db-worktree-pool.md「A slot's two stores are reset together」）。
# host から実行するのは、接続先が公開ポートで足り、slot-acquire に app イメージのビルドを
# 持ち込まないため（table の作成は realtime-provision の担当なので、ここは削除だけ）。
realtime-reset: require-db-owner
	@echo "🧹 この checkout の Realtime Delivery の table を削除します..."
	@$(DB_SLOT_ENV); $(COMPOSE_INFRA) --profile development up -d --wait $(INFRA_NO_RECREATE_SH) dynamodb_local
	@$(DB_SLOT_ENV_EXPORTED); go run ./scripts/realtime-reset $(ARGS)

# 共有インフラ上で走るため、資源は実行ごとに一意な名前で作り終了時に消す（scripts/realtime-smoke）。
realtime-smoke:
	@echo "🔎 DynamoDB Local / GoAWS の互換 smoke を実行します..."
	@$(MAKE) infra-up
	@go run ./scripts/realtime-smoke $(ARGS)

# 接続先は internal/config/test_kit.go の NewRealtimeTestConnection が決める。既定は
# DynamoDB Local と GoAWS なので、その場合は先に make infra-up が要る。endpoint と資格情報の
# 4 変数を空文字にして REALTIME_TEST_REGION を与えると、同じテストが SDK 既定の chain で
# 本番 AWS を相手に走る:
#
#   REALTIME_TEST_ENDPOINT= REALTIME_TEST_PUBSUB_ENDPOINT= \
#   REALTIME_TEST_ACCESS_KEY_ID= REALTIME_TEST_SECRET_ACCESS_KEY= \
#   REALTIME_TEST_REGION=ap-northeast-1 make realtime-contract-test
#
# コンテナでなく host で走らせるのは、その chain（環境変数・プロファイル・SSO）が host 側に
# あるため。本番へ向けると実資源を作って消すので、専用アカウントで実行すること。
REALTIME_CONTRACT_PACKAGES = \
	./internal/infrastructure/eventlog/... \
	./internal/infrastructure/streamticket/... \
	./internal/infrastructure/instancelease/... \
	./internal/infrastructure/realtime/...

realtime-contract-test:
	@echo "🔎 Realtime Delivery の contract test を実行します（接続先は REALTIME_TEST_* が決めます）..."
	@go test -count=1 $(REALTIME_CONTRACT_PACKAGES) $(ARGS)
