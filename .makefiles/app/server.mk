## アプリケーションサーバー関連
.PHONY: serve ## 開発環境(API/mock認証)を起動する
.PHONY: serve-build ## キャッシュを利用したビルド後、開発環境を起動する
.PHONY: serve-build-clean ## キャッシュ無効・base image を pull したクリーンビルド後、開発環境を起動する
.PHONY: app-up ## app コンテナを起動し API が応答するまで待つ（serve 系から呼ぶ内部用）
.PHONY: serve-stop ## この checkout の app コンテナを停止する
.PHONY: infra-up ## 共有インフラを起動する
.PHONY: infra-down ## 共有インフラを停止する
.PHONY: tools ## 開発支援サービスを起動する
.PHONY: all ## 全サービス(共有インフラ/開発/ツール)を一括起動する
.PHONY: tool-runners-build ## ツールランナー(go/node/python)をビルドする（キャッシュ利用・起動はしない）
.PHONY: tool-runners-build-clean ## ツールランナーをクリーンビルドする（--no-cache --pull・起動はしない）

# garage_init は完了して終了する one-shot のため --wait（正常終了も失敗と見なす）の対象から外し、
# 終了まで同期ブロックする run で実行する。app はプロジェクトを跨ぐため compose の依存で待てず、
# バケット未プロビジョニングのまま S3 を触ると 503 になる。
# garage_init の CLI は garage server と RPC バージョンが一致しないと接続できないため、server の
# image 更新に取り残された古いイメージで走らないよう --build を付ける（キャッシュは効く）。
#
# INFRA_NO_RECREATE: worktree では他 checkout が使用中のインフラを毎回作り直してしまうため、
# 既存インスタンスを優先する（判定と根拠は compose.mk）。この状態では定義変更の反映は
# infra-down → infra-up の明示操作になる。単一 checkout では空で、compose の既定どおり再収束する。
# --no-deps: run は依存サービスも converge するため、直前の up で残した garage を
# ここで作り直してしまう（garage_init の depends_on は garage のみ）。稼働は直前行の --wait が
# 保証済み。
infra-up:
	@echo "🔄 共有インフラを起動します... (project=$(INFRA_PROJECT))"
	@$(DB_SLOT_ENV); $(COMPOSE_INFRA) --profile development up -d --wait $(INFRA_NO_RECREATE_SH) $(INFRA_SERVICES)
	@$(COMPOSE_INFRA) --profile development run --rm --build --no-deps -T garage_init > /dev/null
	@echo "✅ 共有インフラが起動しています。Grafana: http://localhost:3000"

infra-down:
	@echo "🛑 共有インフラを停止します。全 checkout / worktree に影響します... (project=$(INFRA_PROJECT))"
	@$(COMPOSE_INFRA) down
	@echo "✅ 共有インフラを停止しました（データボリュームは保持されます）。"

# app コンテナは DB_NAME_LOCAL の指すデータベースへ接続する（docker-compose.attach.yaml）。
# 未設定なら共有 local へ落ちるため、スロット未取得の worktree では require-db-owner で止める
# （不変条件は .makefiles/database/pool.mk）。serve-stop / infra-* はデータベース名を要さないため対象外。
serve: require-db-owner
	@echo "🔄 開発環境を起動します。"
	@$(MAKE) infra-up
	@$(MAKE) realtime-provision
	@$(MAKE) app-up
	@# スロット保持時のみ heartbeat を更新する（未取得なら何もしない）。
	@go run ./cmd/ db-slot heartbeat
	@$(LOAD_SLOT); $(DB_SLOT_ENV); echo "✅ 開発環境の起動が完了しました。API: http://localhost:$${API_HOST_PORT:-8080} (project=$$APP_PROJECT)"

# realtime-provision は api_server イメージの中で go run するため、ビルドより先に置くと
# 古いイメージで走る。イメージが陳腐化しているときこそ serve-build が呼ばれるので、
# その順序では自力で回復できなくなる（serve-build-clean と同じ順序に揃えている）。
serve-build: require-db-owner
	@echo "🧰 ビルド後、開発環境を起動します。"
	@$(COMPOSE_APP) build $(APP_SERVICES)
	@$(MAKE) infra-up
	@$(MAKE) realtime-provision
	@$(MAKE) app-up
	@$(LOAD_SLOT); echo "✅ 開発環境の起動が完了しました。API: http://localhost:$${API_HOST_PORT:-8080}"

serve-build-clean: require-db-owner
	@echo "🧹 クリーンビルド後、開発環境を起動します（--no-cache --pull）。"
	@$(COMPOSE_APP) build --no-cache --pull $(APP_SERVICES)
	@$(MAKE) infra-up
	@$(MAKE) realtime-provision
	@$(MAKE) app-up
	@$(LOAD_SLOT); echo "✅ 開発環境の起動が完了しました。API: http://localhost:$${API_HOST_PORT:-8080}"

# 失敗時に出す api_server ログの行数。fx の起動ログは Provided 行が数十行続き、失敗本体と
# dlv の終了理由はその末尾に集中する。全文が要るときは make serve APP_LOG_TAIL=200 と上書きする。
APP_LOG_TAIL ?= 50

# air はアプリが落ちてもコンテナを生かしたままにするため、up -d の終了コードに起動の成否は現れない。
# api_server の healthcheck（docker-compose.yaml）を --wait で待って初めて成否になる。
# --wait-timeout は付けない。待ちの上限は healthcheck の start_period + retries × interval が既に
# 持っており、ここにも持つと片方だけ伸ばしたとき compose が先に切れ、まだビルド中のログを失敗として
# 見せることになる。compose の失敗メッセージは "container ... is unhealthy" までしか言わないので、
# 次に読むログをここで出す。$$APP_PROJECT は直前の $(COMPOSE_APP) が同じシェルで eval した値。
app-up: require-db-owner
	@$(COMPOSE_APP) up -d --wait $(APP_SERVICES) || { \
		echo "❌ API の起動に失敗しました。api_server のログ末尾 $(APP_LOG_TAIL) 行:"; \
		$(COMPOSE_APP) logs --no-log-prefix --tail=$(APP_LOG_TAIL) api_server; \
		echo "   全文: docker logs $$APP_PROJECT-api_server-1 ／ 停止: make serve-stop"; \
		exit 1; \
	}

serve-stop:
	@$(DB_SLOT_ENV); echo "🛑 app コンテナを停止します... (project=$$APP_PROJECT)"
	@$(COMPOSE_APP) down
	@echo "✅ app コンテナを停止しました（共有インフラは稼働したままです）。"

# tools profile は database / garage も含むため、INFRA_NO_RECREATE を落とすと make tools が
# 共有インフラの再作成経路として残る（理由は infra-up 参照）。
tools:
	@echo "🔄 開発ツールを起動します。"
	@$(DB_SLOT_ENV); $(COMPOSE_INFRA) --profile tools up -d --build $(INFRA_NO_RECREATE_SH)
	@echo "✅ 開発ツールの起動が完了しました。SQL editor: http://localhost:2000 / docs: http://localhost:2001"

all:
	@echo "🔄 全サービス(共有インフラ/開発/ツール)を一括起動します。"
	@$(MAKE) tools
	@$(MAKE) serve-build
	@echo "✅ 全サービスの起動が完了しました。Grafana: http://localhost:3000"

tool-runners-build:
	@echo "🧰 ツールランナーをビルドします。"
	@$(LOAD_GH_TOKEN); docker compose build go_tool_runner node_tool_runner python_tool_runner
	@echo "✅ ツールランナーのビルドが完了しました。"

tool-runners-build-clean:
	@echo "🧹 ツールランナーをクリーンビルドします（--no-cache --pull）。"
	@$(LOAD_GH_TOKEN); docker compose build --no-cache --pull go_tool_runner node_tool_runner python_tool_runner
	@echo "✅ ツールランナーのクリーンビルドが完了しました。"
