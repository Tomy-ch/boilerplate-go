## GolangCI-Lintのコマンド群
.PHONY: go-lint ## コードの静的解析
.PHONY: go-fix ## コードの自動修正
.PHONY: go-lint-fast ## 波及する違反だけを見る高速な静的解析（ゲートではない）
.PHONY: go-lint-config-check ## golangci の 3 設定が構文として妥当か検証する

GOLANGCI_LINT := $(shell mise which golangci-lint 2>/dev/null || command -v golangci-lint 2>/dev/null || echo golangci-lint)

# .golangci.yaml も .golangci-fast.yaml も固定 timeout を持たないため、ローカル実行のハング防止ガードはここが持つ。
# 開発機での実測は単独 2.5m だが、worktree 並行開発でホストが埋まると十数分（観測値 17.5m）まで伸びる。
# 検査そのものは完走するので、ここは所要の見積もりではなく「応答しなくなった場合の脱出口」として置く。
# CI は 0（無効）を渡し、打ち切りをジョブの timeout-minutes に委ねる。
GOLANGCI_LINT_TIMEOUT ?= 60m

# 並列度と優先度は .makefiles/load.mk が窓（worktree）の数から決める（LOAD_BAND がレシピ内で
# 解決する）。--concurrency は設定ファイルの concurrency を上書きするため、full では空になり
# 設定ファイルの値が活きる。
go-lint:
	@$(LOAD_BAND); $$GOBP_NICE $(GOLANGCI_LINT) run --config .golangci.yaml --timeout $(GOLANGCI_LINT_TIMEOUT) $$GOLANGCI_CONCURRENCY_FLAG

go-fix:
	@$(LOAD_BAND); $$GOBP_NICE $(GOLANGCI_LINT) run --fix --config .golangci.yaml --timeout $(GOLANGCI_LINT_TIMEOUT) $$GOLANGCI_CONCURRENCY_FLAG

# ゲートではなく、修正が他ファイルへ波及する違反（層境界・依存の向き）を早く知るための pass。
# 緑になっても lint を通ったことにはならない（ADR-0088）。
go-lint-fast:
	@echo "ℹ️  lint-fast は権威ゲートではありません。マージ可否は make go-lint / CI が決めます。"
	@$(LOAD_BAND); $$GOBP_NICE $(GOLANGCI_LINT) run --config .golangci-fast.yaml --timeout $(GOLANGCI_LINT_TIMEOUT) $$GOLANGCI_CONCURRENCY_FLAG

# ゲートが読むのは .golangci.yaml だけなので、残る 2 つが受ける唯一の検査（ADR-0088）。
go-lint-config-check:
	@for f in .golangci.yaml .golangci-fast.yaml .golangci-min.yaml; do \
		printf '  %-22s ' "$$f"; \
		$(GOLANGCI_LINT) config verify --config "$$f" >/dev/null || { echo "NG"; exit 1; }; \
		echo "ok"; \
	done
