## pin している CLI ツール(mise.toml / python/*.in)の供給網 cooldown
# -----ホスト上で実行するコマンド群-----
.PHONY: tool-cooldown-gate ## 宣言の差分で追加/更新したツールが cooldown を満たすか検査(違反でfail)
.PHONY: tool-cooldown-audit ## 宣言全件の cooldown 状況を棚卸し(警告のみ・ゲートしない)
.PHONY: tool-cooldown-outdated ## 上流の新版を調べ窓を満たす更新先を報告(ゲートしない)

# -----ホスト上で実行するコマンド群-----
# mise 自身に短縮名の backend を解決させるため、mise が PATH に居ることを前提とする。
# GitHub API は未認証だと 60 req/hour（IP 単位）で 1 回の実行を賄えないため、GITHUB_TOKEN を
# 渡す。CI はワークフローが渡し、手元では gh auth token を使う。
#
# base に既定を置かない理由は go-cooldown-gate と同じ（.makefiles/README.md 参照）。
tool-cooldown-gate:
	@test -n "$(BASE)" || { echo "❌ BASE が要ります: make tool-cooldown-gate BASE=origin/release/vX.Y.0"; exit 1; }
	@GITHUB_TOKEN="$${GITHUB_TOKEN:-$$(gh auth token 2>/dev/null)}" go run ./scripts/tool-cooldown gate --base=$(BASE)

# 棚卸しのみ。バイパスの期限切れだけ失敗させる扱いは go-cooldown-audit と同じ。
tool-cooldown-audit:
	@GITHUB_TOKEN="$${GITHUB_TOKEN:-$$(gh auth token 2>/dev/null)}" go run ./scripts/tool-cooldown audit

# 上流に新版が出たことは gate も audit も見ない（どちらも宣言済みの版の齢しか測らない）ため、
# 検出はこのターゲットが担う。更新の適用はせず、報告だけを出す。
tool-cooldown-outdated:
	@GITHUB_TOKEN="$${GITHUB_TOKEN:-$$(gh auth token 2>/dev/null)}" go run ./scripts/tool-cooldown outdated $(if $(OUT),--summary-out=$(OUT),)
