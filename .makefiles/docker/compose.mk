# docker compose のプロジェクト構成（infra 層 / app 層）
#
# 固定ポートでしか動けないサービスは全 checkout で 1 インスタンスを共有して INFRA_PROJECT に置き、
# checkout 毎に要る api_server / mock_auth_server だけを per-checkout のプロジェクトへ分ける。
# 各変数の役割は .makefiles/README.md「Compose project definitions (compose.mk)」参照。
INFRA_PROJECT ?= $(if $(GOBP_DB_SHARED_PROJECT),$(GOBP_DB_SHARED_PROJECT),gobp-shared)
INFRA_SERVICES ?= database observability garage elasticmq dynamodb_local goaws
APP_SERVICES ?= api_server mock_auth_server

# DB ツーリング（go_tool_runner / docker compose exec database）は共有 DB と同じネットワークで
# 動く必要があるため、プロジェクト名を明示しない compose 呼び出しは INFRA_PROJECT に寄せる。
COMPOSE_PROJECT_NAME ?= $(INFRA_PROJECT)
export COMPOSE_PROJECT_NAME

# スロット定義はレシピ内シェルで都度読む。make のパース時に読む pool.mk の -include だけでは、
# slot-acquire がレシピ実行時に書き出した .gobp-db-slot を同一呼び出し内の後続ゴールへ反映できず、
# `make slot-acquire serve` が既定ポート・既定 DB のままサイレントに起動してしまう。
LOAD_SLOT = set -a; if [ -f .gobp-db-slot ]; then . ./.gobp-db-slot; fi; set +a

# ホストの gh からトークンを借りる理由は
# docs/maintenance/local-environment.md「Image builds borrow the host's GitHub token」参照。
LOAD_GH_TOKEN = export GITHUB_TOKEN="$${GITHUB_TOKEN:-$$(gh auth token 2>/dev/null || true)}"

# 起動対象は常にサービス名で明示するため、profile 指定は対象を絞る用途ではなく有効化のためだけに置く
# （COMPOSE_INFRA は profile 無指定。tools 等でサービス名を省く呼び出しがあり、development を混ぜられない）。
COMPOSE_INFRA = $(LOAD_GH_TOKEN); docker compose -p $(INFRA_PROJECT)
COMPOSE_APP = $(LOAD_SLOT); $(DB_SLOT_ENV); $(LOAD_GH_TOKEN); docker compose -p "$$APP_PROJECT" \
	-f docker-compose.yaml -f docker-compose.attach.yaml --profile development

# スロットと git 文脈から導かれる値（DB_LOCAL / DB_TEST / APP_PROJECT / AUTH_ISSUER /
# INFRA_NO_RECREATE / REALTIME_*）の導出は internal/cli/dbslot だけが持つ。レシピ内で 1 回解決して
# 読む（パース時に置くと make の全呼び出しにビルドが乗るため）。
# make 側に同じ導出を書き写さないこと。両方に置くと片方だけを直したときに黙ってずれ、
# 例えば mock 認証サーバーの既定ポートを変えても make 側のリテラルが追従しない。
# LOAD_SLOT が読むのはスロットの生の値（API_HOST_PORT など）で、こちらは導出済みの値を読む。
# `set -a` で囲むのは、compose ファイルの ${REALTIME_*} と子プロセスの go run が環境変数として
# 読むため。シェル変数のままだとどちらにも届かず、compose 側は既定値へ黙って落ちる。
# 解決に失敗したときは `exit 1` を eval させて止める。空の解決結果で走らせると、リンク worktree の
# 検出が黙って外れたまま共有インフラを触ることになる。
DB_SLOT_ENV = set -a; eval "$$(go run ./cmd/ db-slot env || echo 'exit 1')"; set +a

# 共有インフラの稼働中コンテナを作り直させないフラグ。config-hash が checkout ごとに一致しない
# 理由は docs/maintenance/db-worktree-pool.md「Re-creation of the infra layer」参照。
# 渡すのは worktree のときだけで（判定は db-slot）、単一 checkout では空。この判定で拾えない構成は
# `make infra-up INFRA_NO_RECREATE=--no-recreate` と明示する（明示値は db-slot の判定へ落とさない）。
INFRA_NO_RECREATE_SH = $(if $(filter undefined,$(origin INFRA_NO_RECREATE)),$${INFRA_NO_RECREATE},$(INFRA_NO_RECREATE))
