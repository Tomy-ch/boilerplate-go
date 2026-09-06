## Dockerfile / compose に対するLintコマンド群
# -----Dockerコンテナ内で実行するコマンド群-----
.PHONY: docker-lint ## Dockerfile の Lint を実行
.PHONY: compose-lint ## compose のサービス宣言をリポジトリの規則に照らして検査
# -----CI内で実行するコマンド群-----
.PHONY: docker-lint-ci ## Dockerfile の Lint を実行(CI用)

# -----Dockerコンテナ内で実行するコマンド群-----
docker-lint:
	@docker compose run --rm go_tool_runner make docker-lint-ci

# hadolint と違い判定が自前ツールなので、ツールランナーを経由せずホストで実行する
# （scripts/migration-lint と同じ扱い）。何を検査するかは scripts/compose-lint が持つ。
compose-lint:
	@go run ./scripts/compose-lint

# -----CI内で実行するコマンド群-----
docker-lint-ci:
	hadolint docker/*/Dockerfile
