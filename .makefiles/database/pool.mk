## DB スロットプール（複数 worktree が単一共有 DB を per-worktree データベースで共有）
.PHONY: slot-acquire ## DB スロットを取得しスキーマを作り直す（共有 DB 上に per-worktree DB を貸与）
.PHONY: slot-free ## 保持中の DB スロットだけを解放する（worktree は残す。DB は warm 保持）
.PHONY: slot-release ## worktree を撤収する（app 停止+イメージ削除 → スロット解放 → worktree 削除）
.PHONY: slot-status ## DB スロットプールの占有状況を表示する
.PHONY: require-db-owner ## 自分が所有するデータベースがあることを検証する（DB を触るターゲットの前提）

# ---- 不変条件: データベース : worktree = 1 : 0..1 --------------------------------
# 所有表と、未取得の worktree を既定値へフォールバックさせない理由は
# docs/maintenance/db-worktree-pool.md「The invariant」参照。所有者が居なければ
# 既定値へ落とさず require-db-owner で止める。
# ------------------------------------------------------------------------------

# スロット定義（db-slot が書き出す KEY=VALUE）。取得時だけ生成され、DB 名・ホスト公開ポート・
# app 層の compose プロジェクト名を上書きする。既定値は docker-compose.attach.yaml 側の
# ${VAR:-...}（ホスト公開ポート 8080/2010/2345/6060）と internal/cli/dbslot（app 層の
# compose プロジェクト名・AUTH_ISSUER）。
# ここでの -include はホスト実行の go test へ DB 名を渡すためのもので、app 層の compose 呼び出しは
# パース時では取りこぼすため LOAD_SLOT がレシピ内で読み直す（docker/compose.mk 参照）。
-include .gobp-db-slot
export DB_NAME_LOCAL
export DB_NAME_TEST
export SERVE_PROJECT

# 所有データベースの名前。local / test の所有者は主 checkout ただ 1 つで、未取得の worktree が
# ここへ落ちてこないことは require-db-owner が保証する。再帰展開なので、これらを参照する
# migrate.mk / seed.mk / gen.mk が本ファイルより前に include されていても解決順は問題にならない。
#
# ここだけはパース時に決める必要がある。migrate.mk / start-up.mk / seed.mk が
# `db-local-migrate-up: DB=$(DB_LOCAL)` のような target-specific 変数で使っており、これは
# レシピ時ではなくパース時に評価されるため、レシピ内で解決する db-slot env では間に合わない。
# 値そのものは db-slot が書いた .gobp-db-slot（上の -include）から来る。
DB_LOCAL = $(if $(DB_NAME_LOCAL),$(DB_NAME_LOCAL),local)
DB_TEST = $(if $(DB_NAME_TEST),$(DB_NAME_TEST),test)

# 所有データベースを持たない状態（リンク worktree かつスロット未取得）を検出して止める。
# 判定は internal/cli/dbslot（テストの当たる Go 側）が持つ。スロットを書いているのが db-slot で
# ある以上、スロットから導かれる所有権の判定も同じ側に置く。
require-db-owner:
	@go run ./cmd/ db-slot require-owner

slot-acquire:
	@go run ./cmd/ db-slot acquire
	@echo "🔄 取得したスロットのデータベースを作り直します..."
	@# DB 名は .gobp-db-slot を実行時に source して得る（$(...) の make 変数は parse 時評価で、
	@# 初回取得時はまだファイルが無く空になるため、シェル変数 $$DB_NAME_* を使う）。
	@# local / test は共通 prerequisite db-reinit を持つため 1 つの make 呼び出しにまとめると
	@# db-reinit が 1 度しか実行されない。個別の make 呼び出しに分け、各 worktree DB を作り直す。
	@set -a; . ./.gobp-db-slot; set +a; \
		$(MAKE) db-reinit DB=$$DB_NAME_LOCAL && $(MAKE) db-reinit DB=$$DB_NAME_TEST
	@# PostgreSQL の採番と DynamoDB EventLog は寿命を揃える必要があり、db-reinit と対で呼ぶ
	@# （理由は docs/maintenance/db-worktree-pool.md「A slot's two stores are reset together」）。
	@$(MAKE) realtime-reset
	@go run ./cmd/ db-slot heartbeat
	@echo "✅ DB スロットを取得しました。make test は自 worktree DB(wt<N>_test)、make serve は共有 DB の wt<N>_local を使います。"

slot-free:
	@go run ./cmd/ db-slot release

# 撤収は docker → slot → git の順。入れ替えられない理由は
# docs/maintenance/db-worktree-pool.md「The order cannot be rearranged」参照。
# --force は付けない（未コミット・未追跡があれば git 自身に拒否させる）。
slot-release:
	@test "$$(git rev-parse --git-dir)" != "$$(git rev-parse --git-common-dir)" \
		|| { echo "❌ 主 checkout では実行できません（撤収対象の worktree で実行してください）"; exit 1; }
	@$(COMPOSE_APP) down --rmi local
	@if [ -f .gobp-db-slot ]; then $(MAKE) slot-free; fi
	@echo "🧹 worktree を削除します: $(CURDIR)"
	@git worktree remove $(CURDIR)

slot-status:
	@go run ./cmd/ db-slot status
