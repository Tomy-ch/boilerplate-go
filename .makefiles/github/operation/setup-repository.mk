## リポジトリの初期化
.PHONY: setup-repo ## リポジトリの初期化
# setup-localize:begin
.PHONY: setup-replace-module ## node_tool_runnerでGoモジュール名の一括置換を実行
.PHONY: setup-replace-app-metadata ## node_tool_runnerでAPP_NAMEやOpenAPIタイトルの置換を実行
.PHONY: setup-replace-repository-reference ## node_tool_runnerでリポジトリ参照の置換を実行
.PHONY: setup-replace-license-copyright ## node_tool_runnerでLICENSEの著作権表示更新を実行
.PHONY: setup-replace-codeowners ## node_tool_runnerでCODEOWNERSの所有者の一括置換を実行
.PHONY: setup-verify ## 初期化の完了を検証し、通れば初期化ツール一式を撤去する
# setup-localize:end
# boilerplate-only:begin
.PHONY: setup-remove-boilerplate-identity ## テンプレート自身を語る記述を落としボイラープレートの顔を消す
# boilerplate-only:end
.PHONY: setup-remove-sample-api ## サンプルAPI(user/product/order)を一括削除し再生成・検証まで実行 # sample-api:line
.PHONY: setup-remove-licensed-scanners ## 資格情報/課金を要するスキャナ2件を撤去し製品ごとにコミット
# lang-choice:begin
.PHONY: setup-remove-doc-language ## ドキュメント/スキルの対訳ペアを選んだ言語1本へ畳む
# lang-choice:end

SETUP_DRY_RUN_FLAG := $(if $(DRY_RUN),--dry-run,)

# git / gh の手順（タグの作り直し・ブランチ整備・デフォルトブランチ移動・リリースノート整理）は
# scripts/repo-setup（テスト付き）が持つ。取り消しの効かない操作ばかりで、分岐を実地で確かめようと
# するとリポジトリを本当に壊すしかないため、手順をシェルに置かない。
# ラベル・ルールセット・ワークフローの初期化は個別の make ターゲットなので、連鎖はここに残す。
# ホストの認証情報を使うためツールランナーは経由しない（cmd/db-slot と同じ扱い）。
REPO_SETUP := go run ./scripts/repo-setup

setup-repo:
	@if [ -n "$(DRY_RUN)" ]; then echo "❌ setup-repo は DRY_RUN 未対応です（ローカル/リモートを破壊的に変更します）。DRY_RUN を外して実行してください。"; exit 1; fi
	@$(REPO_SETUP) preflight

	@echo "🔧 ghコマンドのログインを開始します..."
	@$(MAKE) gh-login
	@echo "✅ ghコマンドのログインが完了しました。"

	@$(REPO_SETUP) bootstrap

	@echo "🔧 ワークフローの有効化を開始します..."
	@$(MAKE) enable-workflows
	@echo "✅ ワークフローの有効化を終了します。"

	@echo "🔧 ラベルの初期化を開始します..."
	@$(MAKE) delete-all-labels
	@$(MAKE) create-default-labels
	@echo "✅ ラベルの初期化を終了します。"

	@$(REPO_SETUP) prune-release-notes

	@echo "🔧 ルールセットの適用を開始します..."
	@$(MAKE) apply-branch-protection
	@echo "✅ ルールセットの適用を終了します。"

	@git remote remove upstream || true
	@echo "✅ Initialization complete. Default branch: production"

# setup-localize:begin
setup-replace-module:
	@if [ -z "$(OLD_MODULE)" ] || [ -z "$(NEW_MODULE)" ]; then \
		echo "❌ OLD_MODULE と NEW_MODULE を指定してください。例: make setup-replace-module OLD_MODULE=go-boilerplate NEW_MODULE=example-api"; \
		exit 1; \
	fi
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/replace-module $(OLD_MODULE) $(NEW_MODULE) $(SETUP_DRY_RUN_FLAG)

setup-replace-app-metadata:
	@if [ -z "$(APP_NAME)" ] || [ -z "$(OPENAPI_TITLE)" ] || [ -z "$(COPILOT_TITLE)" ]; then \
		echo "❌ APP_NAME, OPENAPI_TITLE, COPILOT_TITLE を指定してください。"; \
		echo "例: make setup-replace-app-metadata APP_NAME='Example API' OPENAPI_TITLE='Example API with Onion Architecture' COPILOT_TITLE='example-api Copilot Instructions'"; \
		exit 1; \
	fi
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/replace-app-metadata \
		--app-name "$(APP_NAME)" \
		--openapi-title "$(OPENAPI_TITLE)" \
		--copilot-title "$(COPILOT_TITLE)" \
		$(SETUP_DRY_RUN_FLAG)

setup-replace-repository-reference:
	@if [ -z "$(REPOSITORY)" ]; then \
		echo "❌ REPOSITORY を指定してください。例: make setup-replace-repository-reference REPOSITORY=example-org/example-api"; \
		exit 1; \
	fi
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/replace-repository-reference $(REPOSITORY) $(SETUP_DRY_RUN_FLAG)

setup-replace-license-copyright:
	@if [ -z "$(COPYRIGHT_HOLDER)" ]; then \
		echo "❌ COPYRIGHT_HOLDER を指定してください。例: make setup-replace-license-copyright COPYRIGHT_HOLDER='Example Inc.' COPYRIGHT_YEAR=2026"; \
		exit 1; \
	fi
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/replace-license-copyright \
		--holder "$(COPYRIGHT_HOLDER)" \
		$(if $(COPYRIGHT_YEAR),--year $(COPYRIGHT_YEAR),) \
		$(SETUP_DRY_RUN_FLAG)

setup-replace-codeowners:
	@if [ -z "$(OWNERS)" ]; then \
		echo "❌ OWNERS を指定してください。例: make setup-replace-codeowners OWNERS='@example-org/tech-leads'"; \
		exit 1; \
	fi
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/replace-codeowners \
		--owners "$(OWNERS)" \
		$(SETUP_DRY_RUN_FLAG)

# 初期化（Phase 5 の replace-* 連鎖）の最終地点。置換の過不足を検証し、通ったときだけ
# 初期化ツールを撤去する。撤去を各スクリプトの実行直後に置かないのは、5 本の連続実行の途中で
# 引数を打ち間違えた利用者が、直すためのツールを既に失っている状態を作らないため。
# 期待値は Phase 5 で export した環境変数をそのまま渡す（検証側が同じ値を突き合わせる）。
setup-verify:
	@docker compose run --rm \
		-e MODULE -e REPOSITORY -e COPYRIGHT_HOLDER -e COPYRIGHT_YEAR -e CODE_OWNERS \
		node_tool_runner $(TSX) scripts/setup/verify-setup

# setup-localize:end
# boilerplate-only:begin

# テンプレート自身を語る散文（README の 2 節・設定ガイドのインスタンス化手順）を落とす。
# 一度きりの操作なのでツール自身も撤去し、このターゲットの宣言も同じマーカーで一緒に消える。
setup-remove-boilerplate-identity:
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/remove-boilerplate-identity $(SETUP_DRY_RUN_FLAG)

# boilerplate-only:end
# sample-api:begin

# サンプルAPIの削除はコンテナ内（node_tool_runner）で行い、削除後の再生成・整形・検証・DB 再構築は
# Go ツールチェーンが必要なためホスト側の make ターゲットを連鎖させる。
# プレビューは DRY_RUN=1 を付ける（削除も再生成も行わない）。
# make は起動時に makefile を全読込するため、手順1のスクリプトがこの .mk からターゲットを strip（自消滅）
# しても、実行中のレシピは継続し regen まで走る。
# サンプル削除で未使用になる直接依存が go.mod に残ると、後日 go.mod を触った無関係な PR で
# tidy-check が落ちる。tidy-lib は import が確定する gen の後、整理後の状態を lint で検証して
# 終えられるよう fix/lint の前に置く。各手順は && で連鎖し、途中の失敗が完了メッセージに隠れない。
# 最後の検証はツールランナーを経由しない。git status と make help を読むため、ホストの git と
# この makefile 自身が要る（worktree では .git がマウント外の実体を指すファイルで辿れない）。
# ここに置くのは、検証器が通ったときだけ自分と snapshot を撤去する設計だからで、呼ばずに終えると
# 削除ツールの片割れと、削除対象パスを全て書き出した記録が作成先へ居座る。
# ホストで走らせる以上、scripts の依存もホスト側に要る。無いと pnpm が依存の鮮度を判定できず、
# 削除は済んだのに検証だけ走らないという、検証の失敗と区別が付かない状態で止まる。
setup-remove-sample-api:
	@docker compose run --rm node_tool_runner $(TSX) scripts/setup/remove-sample-api $(SETUP_DRY_RUN_FLAG)
	@if [ -n "$(DRY_RUN)" ]; then \
		echo "🟡 DRY_RUN のため再生成・整形・検証はスキップしました。"; \
	else \
		echo "🔧 再生成・整形・検証・DB 再構築を実行します..." && \
		$(MAKE) db-local-reinit db-test-reinit && \
		$(MAKE) gen-api gen-query && \
		$(MAKE) tidy-lib && \
		$(MAKE) go-fix go-lint && \
		pnpm install --dir scripts --frozen-lockfile && \
		$(TSX) scripts/setup/verify-sample-removal && \
		echo "✅ サンプルAPIの削除・再生成・検証が完了しました。"; \
	fi
# sample-api:end

# 資格情報 / ライセンス費用を要するスキャナ 2 件の撤去。
# 他の setup ターゲットと違いツールランナーを経由せずホストで走らせる。このスクリプトは
# 製品ごとに git commit を積むため、setup-repo と同じくホストの git を使う必要がある
# （worktree では .git がマウント外の実体を指すファイルなので、コンテナ内からは辿れない）。
# プレビューは DRY_RUN=1 を付ける（書き込みもコミットも行わない）。
setup-remove-licensed-scanners:
	@$(TSX) scripts/setup/remove-licensed-scanners $(SETUP_DRY_RUN_FLAG)

# lang-choice:begin

# ドキュメント / スキルの対訳ペアを LANG_CHOICE で解決する（en|ja|both）。
# 他の setup ターゲットと違いツールランナーを経由せずホストで走らせる。400 件超のファイルを
# 消す・改名するため撤去を 1 コミットに畳んでおり、setup-repo と同じくホストの git が要る
# （worktree では .git がマウント外の実体を指すファイルなので、コンテナ内からは辿れない）。
# 手順の先頭に置くのは、他の撤去ツールが正本と対訳の対で宣言を持ち、畳みが解決した時点で
# 自分の対の宣言を刈るためである。逆向きは成り立たない。boilerplate 撤去はこのツールが
# 文字列を宣言している workflow を削除するので、その後では畳みが中止する。
# プレビューは DRY_RUN=1 を付ける（書き込みもコミットも行わない）。
setup-remove-doc-language:
	@if [ -z "$(LANG_CHOICE)" ]; then \
		echo "❌ LANG_CHOICE を指定してください。例: make setup-remove-doc-language LANG_CHOICE=en"; \
		echo "   en   = 英語 1 本に畳む（日本語の対訳を撤去）"; \
		echo "   ja   = 日本語 1 本に畳む（英語正本を撤去し対訳を正本の名前へ改名）"; \
		echo "   both = 両方残す（マーカーだけ解決してツールを撤去）"; \
		exit 1; \
	fi
	@$(TSX) scripts/setup/remove-doc-language --lang $(LANG_CHOICE) $(SETUP_DRY_RUN_FLAG)

# lang-choice:end
