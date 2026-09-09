#checkov:skip=CKV_DOCKER_2:make から docker compose run --rm で起動して終了する使い捨てで、常駐しないため健全性の観測対象が無い
#checkov:skip=CKV_DOCKER_3:root 実行が要件。mise を /root 配下へ配置しており、非 root では mise と各ツールを解決できない（docker-compose.yaml 参照）
# 上の #checkov:skip は checkov 自身の判定からしか外れない。code scanning へは checkov.yaml の
# 「Drop suppressed results」が落としており、片方だけでは消えない。
# mise 本体の固定バージョン（全ツールステージ共通。bump 時はここだけ変更する）
ARG MISE_VERSION=v2026.6.0

# ------------------------------
# Go ツールビルダー：mise.toml のバージョン pin で Go 系ツール一式を install し、
# バイナリだけを /out に抽出する。最終 go_tools ステージに mise 本体を残さないための中継。
# ------------------------------
FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS go_tools_builder

# CGO_ENABLED=0: `go:` backend で install されるツール（mockgen）を静的バイナリにする
ENV CGO_ENABLED=0

RUN apk add --no-cache bash ca-certificates curl git

ARG MISE_VERSION
ENV MISE_DATA_DIR=/root/.local/share/mise \
    MISE_CONFIG_DIR=/root/.config/mise \
    PATH="/root/.local/bin:/root/.local/share/mise/shims:$PATH"
# パイプのままだとリトライ時に部分受信分が sh へ流れ込むため、ファイルへ落としてから実行する。
RUN curl -fsSL --retry 3 --retry-delay 2 --retry-all-errors https://mise.run -o /tmp/mise-install.sh \
    && MISE_VERSION=${MISE_VERSION} sh /tmp/mise-install.sh \
    && rm /tmp/mise-install.sh \
    && /root/.local/bin/mise --version

WORKDIR /app
COPY mise.toml ./
RUN mise trust mise.toml

# 未認証の GitHub API（60 req/hour・IP 単位）ではこの install 群を賄えず 403 になる。トークンは
# イメージ履歴に焼き込む build-arg を避け secret で渡す（未指定なら従来どおり未認証で成立）。
RUN --mount=type=secret,id=github_token \
    github_token="$(cat /run/secrets/github_token 2>/dev/null || true)"; \
    if [ -n "$github_token" ]; then export GITHUB_TOKEN="$github_token"; fi; \
    mise install aqua:golang-migrate/migrate \
    && mise install go:go.uber.org/mock/mockgen \
    && mise install oapi-codegen \
    && mise install sqlc \
    && mise install aqua:aquasecurity/trivy \
    && mise install actionlint \
    && mise install shellcheck \
    && mise install hadolint \
    && mise install gitleaks \
    && mise install go:golang.org/x/tools/cmd/godoc \
    && mise install go:codeberg.org/tslocum/godoc-static

RUN mkdir /out \
    && cp "$(mise which migrate)" /out/ \
    && cp "$(mise which mockgen)" /out/ \
    && cp "$(mise which oapi-codegen)" /out/ \
    && cp "$(mise which sqlc)" /out/ \
    && cp "$(mise which trivy)" /out/ \
    && cp "$(mise which actionlint)" /out/ \
    && cp "$(mise which shellcheck)" /out/ \
    && cp "$(mise which hadolint)" /out/ \
    && cp "$(mise which gitleaks)" /out/ \
    && cp "$(mise which godoc)" /out/ \
    && cp "$(mise which godoc-static)" /out/

# ------------------------------
# Go ツールボックス：migrate / mockgen / oapi-codegen / sqlc / trivy / actionlint / shellcheck / hadolint / gitleaks / godoc / godoc-static を mise.toml のバージョンで提供
# Go ランタイムは golang 公式 base image を使用（mise.toml の go 値と `make sync-go-version` で同期）
# ------------------------------
FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS go_tools

RUN apk add --no-cache \
    bash \
    ca-certificates \
    make \
    postgresql-client

# go_tools_builder からツールのバイナリだけを取り込む（mise 本体や install キャッシュは含まない）
COPY --from=go_tools_builder /out/ /usr/local/bin/

WORKDIR /app

ENV GOCACHE=/tmp/go-build \
    GOMODCACHE=/tmp/go-mod

ENTRYPOINT []
CMD []

# ------------------------------
# Node ツールボックス：redocly / markdownlint-cli2 / pnpm を mise.toml のバージョンで提供
# Node ランタイムは公式 node:X.Y-alpine（musl 対応のプリビルド）を使用。
# mise の Node ビルドは alpine 上でソースコンパイルを要求するため、ここでは公式 base に頼る。
# 補助スクリプトが依存するパッケージを pnpm でローカル node_modules にインストールしておく
# ------------------------------
FROM node:26.8.1-alpine@sha256:2d984a15c9b54fd0aeb608b8e0d0d83529eb34d2966db27a1fb4f1edc3d298a3 AS node_tools

RUN apk add --no-cache \
    bash \
    ca-certificates \
    curl \
    git \
    make

ARG MISE_VERSION
ENV MISE_DATA_DIR=/root/.local/share/mise \
    MISE_CONFIG_DIR=/root/.config/mise \
    PATH="/root/.local/bin:/root/.local/share/mise/shims:$PATH"
RUN curl -fsSL --retry 3 --retry-delay 2 --retry-all-errors https://mise.run -o /tmp/mise-install.sh \
    && MISE_VERSION=${MISE_VERSION} sh /tmp/mise-install.sh \
    && rm /tmp/mise-install.sh \
    && /root/.local/bin/mise --version

WORKDIR /app
COPY mise.toml ./
RUN mise trust mise.toml

# install 対象は npm backend だが、mise は対象に関わらず mise.toml 全体を解決するため GitHub を引く。
RUN --mount=type=secret,id=github_token \
    github_token="$(cat /run/secrets/github_token 2>/dev/null || true)"; \
    if [ -n "$github_token" ]; then export GITHUB_TOKEN="$github_token"; fi; \
    mise install npm:@redocly/cli \
    && mise install npm:markdownlint-cli2 \
    && mise install npm:@commitlint/cli \
    && mise install npm:pnpm \
    && rm -rf /root/.cache /root/.npm

# scripts の依存を in-tree（/app/scripts/node_modules）へ install する。補助スクリプト
# （TypeScript）は同じ場所の tsx / vitest で実行・検査する。
#
# 以降の pnpm install は、依存ライフサイクルスクリプトの実行可否を各 package の
# pnpm-workspace.yaml が宣言する（strictDepBuilds + allowBuilds）。宣言に無い依存が script を
# 持ち込んだ時点で install が失敗する。
WORKDIR /app/scripts
COPY scripts/package.json scripts/pnpm-lock.yaml scripts/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile \
    && pnpm store prune \
    && rm -rf /root/.cache

# docs-viewer の依存を in-tree（/app/docs-viewer/node_modules）へ install する。scripts とは
# 別パッケージで、パッケージマネージャも pnpm で分かれている。
#
# ここで install しておくのは、compose がこのパスへ匿名ボリュームを当てて host の node_modules を
# 隠すため。vite / rolldown はプラットフォーム別のバイナリを引くので、host（darwin）の解決結果が
# そのままコンテナ（linux）へ見えると実行できない。
WORKDIR /app/docs-viewer
COPY docs-viewer/package.json docs-viewer/pnpm-lock.yaml docs-viewer/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile \
    && pnpm store prune \
    && rm -rf /root/.cache

WORKDIR /app

ENTRYPOINT []
CMD []

# ------------------------------
# Python ツールボックス：sqlfluff を python/sqlfluff.txt のバージョンで提供
# Python ランタイムは公式 python:X.Y.Z-slim（プリビルド）を使用。
# mise の Python ビルドは python-build によるソースコンパイルが必要で時間がかかるため、ここでは公式 base に頼る。
# mise が入れるのは uv までで、sqlfluff 本体は sha256 付きの lockfile から uv が入れる。
# ------------------------------
FROM python:3.14.7-slim@sha256:cad9a2c871761c413caa6fdd6441c783451e740a48aaeba60ae62a8b53525ef6 AS python_tools

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        git \
        make \
    && rm -rf /var/lib/apt/lists/*

ARG MISE_VERSION
ENV PATH="/root/.local/bin:/root/.local/share/mise/shims:$PATH" \
    MISE_DATA_DIR=/root/.local/share/mise \
    MISE_CONFIG_DIR=/root/.config/mise
RUN curl -fsSL --retry 3 --retry-delay 2 --retry-all-errors https://mise.run -o /tmp/mise-install.sh \
    && MISE_VERSION=${MISE_VERSION} sh /tmp/mise-install.sh \
    && rm /tmp/mise-install.sh \
    && /root/.local/bin/mise --version

WORKDIR /app
COPY mise.toml ./
RUN mise trust mise.toml

# uv 自体が aqua:astral-sh/uv（GitHub Releases 解決）であるうえ、mise は mise.toml 全体を解決する。
RUN --mount=type=secret,id=github_token \
    github_token="$(cat /run/secrets/github_token 2>/dev/null || true)"; \
    if [ -n "$github_token" ]; then export GITHUB_TOKEN="$github_token"; fi; \
    mise install uv \
    && rm -rf /root/.cache

# --require-hashes は lockfile の全エントリが版とハッシュで固定されていることを要求するため、
# 記載漏れのある lockfile はここで install ごと失敗する。ハッシュ照合そのものが検査を兼ねる。
#
# --only-binary :all: が見ているのはその次の問いで、ハッシュが一致した sdist であっても
# setup script は install 時に走る。wheel だけを受け付ければその余地が無くなる。この lockfile は
# 全件 wheel から解決できるため失うものは無く、解決できない pin が入った時点で黙って任意コードを
# 実行するのではなくここで落ちる。.github/workflows/sql-lint.yaml が同じ lockfile を同じ形で入れる。
COPY python/sqlfluff.txt python/
RUN uv pip install --system --only-binary :all: --require-hashes --no-cache -r python/sqlfluff.txt \
    && sqlfluff --version

# graphify（知識グラフ）。lockfile を分けてあるのは、片方の版を動かしたときに
# もう片方の install レイヤまで作り直させないため。
#
# ここに入れるのは速度のためではなく再現性のためである。graphify の意味論抽出は
# 同梱の抽出プロンプトで結果が変わり、キャッシュの名前空間もそのプロンプトの
# fingerprint で決まる（.agents/graphify/spec-pin.toml）。プロンプトはツールに同梱されて
# 配られるので、ホストに入れた graphify を使うと「どのプロンプトで焼いたか」が
# マシンごとに変わる。このイメージ経由なら python/graphify.in が版を、graphify.txt の
# sha256 が中身を決める。
COPY python/graphify.txt python/
RUN uv pip install --system --only-binary :all: --require-hashes --no-cache -r python/graphify.txt \
    && graphify --version

ENTRYPOINT []
CMD []
