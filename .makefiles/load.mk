# ホスト負荷の配分（並行 worktree 前提の CPU シェアと、ゲートの委譲先）
#
# 帯の設計（full / low / ci-first の意味と閾値、絞る対象の線引き）は .makefiles/README.md の
# `.makefiles/load` group が持つ。
#
# 判定そのものは scripts/load-band が持ち、ここは設定値を渡して結果を受け取るだけにする。
# 判定をシェルへ書き戻さないこと（理由は scripts/load-band の package doc）。
#
# 呼び出しはレシピ時に限る。パース時（$(shell ...)）に置くと解決のためのプロセス起動が
# make の全呼び出しに乗り、ゲート自身の起動を遅らせる（実測で `make -n help` のパースが
# 倍になる）。消費側は全てレシピ内なので、レシピの先頭で 1 回解決して eval すれば足りる。

# 帯の設定（GOBP_LOAD は auto / full / low / ci-first、閾値は窓の数）。
# 既定値は scripts/load-band が持つため、ここでは上書き値だけを素通しする。
GOBP_LOAD ?=
GOBP_LOW_THRESHOLD ?=
GOBP_CI_FIRST_THRESHOLD ?=

LOAD_BAND_TOOL = go run ./scripts/load-band
LOAD_BAND_ARGS = $(if $(GOBP_LOAD),--load='$(GOBP_LOAD)',) \
	$(if $(GOBP_LOW_THRESHOLD),--low='$(GOBP_LOW_THRESHOLD)',) \
	$(if $(GOBP_CI_FIRST_THRESHOLD),--ci-first='$(GOBP_CI_FIRST_THRESHOLD)',)

# レシピ内で負荷帯を解決し、GOBP_LOAD_RESOLVED / GOBP_SHARE / GOBP_NICE /
# GOLANGCI_CONCURRENCY_FLAG / GO_TEST_P_FLAG / GO_TEST_LOAD_ENV をシェル変数として読む
# （.gobp-db-slot を読み直す LOAD_SLOT と同じイディオム。docker/compose.mk 参照）。
# 解決に失敗したときは `exit 1` を eval させて止める。空の解決結果をそのまま使うと、
# 絞る指定が黙って外れた full として走ってしまう。
LOAD_BAND = eval "$$($(LOAD_BAND_TOOL) env $(LOAD_BAND_ARGS) || echo 'exit 1')"

.PHONY: load-status ## 現在の負荷モードと CPU シェアを表示する
.PHONY: gate-go ## pre-commit の Go ゲート（負荷帯に応じて並列/逐次/CI委譲を切り替える）
.PHONY: gate-go-push ## pre-push の Go ゲート（負荷帯に応じて並列/逐次/CI委譲を切り替える）
.PHONY: gate-fix ## 自動フォーマットの委譲先（毎回走る経路から呼ぶ。ci-first では実行しない）
.PHONY: gate-heavy-skip ## 重いゲートを CI へ委ねる帯かを返す（exit 0 = 委譲する）

# lefthook の skip 条件から呼ぶ述語。終了コードだけが意味を持つ（0 = スキップ）。
# 判定を make 側に置くことで、フックの各 command と make ターゲットが同じ 1 つの
# 負荷帯定義を見る（.lefthook.yaml へ閾値を書き写さない）。
gate-heavy-skip:
	@$(LOAD_BAND); test "$$GOBP_LOAD_RESOLVED" = "ci-first"

load-status:
	@$(LOAD_BAND_TOOL) status $(LOAD_BAND_ARGS)

# pre-commit / pre-push の Go ゲート。lefthook は commands を parallel で走らせるため、
# 重いゲートを個別の command として並べると「窓の数 × フックの同時実行数」で負荷が乗算される。
# Go 系だけを 1 つの command へ束ね、並列度の判断をここへ集約する。
#
# full  : 従来どおり並列（1 窓ならホスト全体を使い切るのが最速）
# low   : 逐次（share で絞ったうえで、同時に走るゲートを 1 つに保つ）
# ci-first: 実行せず CI へ委ねる
define run_go_gates
	@$(LOAD_BAND); \
	if [ "$$GOBP_LOAD_RESOLVED" = "ci-first" ]; then \
		echo "⏭  重い Go ゲート($(1))は CI へ委譲します（窓 $${GOBP_WINDOWS} 個 / GOBP_LOAD=$${GOBP_LOAD_RESOLVED}）。"; \
		echo "   手元で回すなら: make $(1) GOBP_LOAD=low"; \
	elif [ -n "$$GOBP_THROTTLED" ]; then \
		echo "🐌 低負荷モード（share $${GOBP_SHARE}）で $(1) を逐次実行します。"; \
		$(MAKE) $(1); \
	else \
		$(MAKE) -j$(words $(1)) $(1); \
	fi
endef

gate-go:
	$(call run_go_gates,go-lint go-test-cached)

gate-go-push:
	$(call run_go_gates,go-test go-test-scripts)

# 自動フォーマットの委譲先。`/commit` のように人手を介さず毎回走る経路はこちらを呼ぶ。
# `make go-fix` を直接叩いたときは帯に関わらず実行する（明示したコマンドは書いたとおりに動く）。
#
# fix は lint と同じ full config を回すため、委譲しないとここだけが帯をすり抜ける。
# 委譲すると CI が赤くなってから直すことになり往復は増えるが、目的は負荷の低減であり、
# フォーマットのずれは CI の lint が確実に捕まえる（見落として壊れる類の指摘ではない）。
gate-fix:
	@$(LOAD_BAND); \
	if [ "$$GOBP_LOAD_RESOLVED" = "ci-first" ]; then \
		echo "⏭  自動フォーマット(fix)は委譲します（窓 $${GOBP_WINDOWS} 個 / GOBP_LOAD=$${GOBP_LOAD_RESOLVED}）。"; \
		echo "   フォーマットのずれは CI の lint が指摘します。手元で直すなら: make go-fix GOBP_LOAD=low"; \
	else \
		$(MAKE) go-fix; \
	fi
