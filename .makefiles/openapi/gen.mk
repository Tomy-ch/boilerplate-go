## OpenAPI関連の生成コマンド群
# -----Dockerコンテナ内で実行するコマンド群-----
.PHONY: gen-bundle-oapi ## OpenAPIをバンドルしてOpenAPIファイルを一つにまとめます
.PHONY: gen-api-docs ## OpenAPIに基づき、APIドキュメントを生成します
.PHONY: oapi-lint ## OpenAPI定義を redocly lint で検証します
.PHONY: stamp-openapi-version ## リリースブランチ名(REF=release/vX.Y.Z)から info.version を書き換えます
# -----CI用ターゲット-----
.PHONY: stamp-openapi-version-ci ## リリースブランチ名から info.version を書き換えます（CI用）
.PHONY: gen-bundle-oapi-ci ## OpenAPIをバンドルしてOpenAPIファイルを一つにまとめます（CI用）
.PHONY: gen-api-docs-ci ## OpenAPIに基づき、APIドキュメントを生成します（CI用）
.PHONY: oapi-lint-ci ## OpenAPI定義を redocly lint で検証します（CI用）
.PHONY: oapi-security-lint-ci ## OpenAPI定義を OWASP API Security ルールセットで検証します（CI用）
.PHONY: openapi-client-check ## frontend generator（orval）が bundle 済み OpenAPI から component 型を生成できることを確認します
.PHONY: openapi-client-check-ci ## frontend generator（orval）が bundle 済み OpenAPI から component 型を生成できることを確認します（CI用）

# -----Dockerコンテナ内で実行するコマンド群-----
gen-bundle-oapi:
	@docker compose run --rm node_tool_runner make gen-bundle-oapi-ci

gen-api-docs:
	@docker compose run --rm node_tool_runner make gen-api-docs-ci

oapi-lint:
	@docker compose run --rm node_tool_runner make oapi-lint-ci

openapi-client-check:
	@docker compose run --rm node_tool_runner make openapi-client-check-ci

# REF 未指定なら環境変数 GITHUB_REF_NAME を使う。コンテナへは環境変数が渡らないため、
# 呼び出し側が読んだ値を make 変数として内側へ引き継ぐ。
stamp-openapi-version:
	@docker compose run --rm node_tool_runner make stamp-openapi-version-ci REF="$(or $(REF),$(GITHUB_REF_NAME))"

# -----CI用ターゲット-----
gen-bundle-oapi-ci:
	redocly bundle openapi/openapi.yaml -o openapi/openapi.gen.yaml

gen-api-docs-ci:
	redocly build-docs openapi/openapi.yaml --output docs/openapi/index.html

oapi-lint-ci:
	redocly lint openapi/openapi.yaml

# release/vX.Y.Z 以外の ref は no-op（スキップして正常終了）。
stamp-openapi-version-ci:
	$(TSX) scripts/stamp-openapi-version $(REF)

# OWASP API Security ルールセット（Spectral）による検証。redocly lint との棲み分けは
# .github/workflows/README.md の Overlapping surfaces 表。コンテナを介さず直接実行する理由と
# 事前準備（pnpm install --dir scripts --frozen-lockfile）は
# .makefiles/README.md の oapi-security-lint-ci 行。
oapi-security-lint-ci:
	$(PNPM_SCRIPTS) lint:openapi-security

# frontend generator（orval）の contract check。確認する型と生成物の扱いは
# .makefiles/README.md の openapi-client-check 行。事前準備は oapi-security-lint-ci と同じ。
openapi-client-check-ci:
	$(PNPM_SCRIPTS) check:openapi-client
