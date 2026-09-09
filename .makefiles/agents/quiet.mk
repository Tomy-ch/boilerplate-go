## エージェント向けの静音実行（`make ai-<target>`）
#
# `make ai-<target>` は <target> の出力をまるごと退避し、失敗したときだけログの場所を 1 行で
# 示す（成功時は無出力、終了コードは素通し）。接頭辞である理由、対象外のターゲット、ログを
# 成功時も残す理由は .makefiles/README.md の Conventions / Notes が所管する。

AI_LOG_DIR ?= tmp/ai-logs

.PHONY: clean-ai-logs ## エージェント用ログ（tmp/ai-logs）を削除する

# `make ai-<target>` で <target> を静かに実行する。ターゲット名は `ai-` で始めない
# （明示ルールはパターンルールより優先されるため壊れはしないが、読み手が迷う）。
ai-%:
	@mkdir -p $(AI_LOG_DIR)
	@$(MAKE) --no-print-directory $* >$(AI_LOG_DIR)/$*.txt 2>&1; \
	status=$$?; \
	if [ $$status -ne 0 ]; then \
		echo "$(AI_LOG_DIR)/$*.txt を読み込んでください（make $* が exit $$status で失敗）"; \
	fi; \
	exit $$status

clean-ai-logs:
	@rm -rf $(AI_LOG_DIR)
