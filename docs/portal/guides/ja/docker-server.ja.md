# server Dockerfile

アプリケーションサーバーの Docker イメージを定義する Dockerfile です。マルチステージビルドにより、本番・ローカル開発の各ターゲットを提供します。

## 役割

`docker/server/Dockerfile` はこのプロジェクトで使用するサーバーサイドコンテナすべての single source of truth です。1 つの Dockerfile から 3 ターゲット（`builder` / `runtime` / `tooling`）を生成することで、本番デプロイとローカル開発のホットリロード環境が同じベースレイヤーと Go ツールチェインバージョンに揃った状態を保ちます。本番と開発者ローカルの drift を回避しつつ、各ターゲットは必要なものだけを追加する設計（例: 開発ツールは `tooling` にのみ含める）です。スキーママイグレーションは `runtime` イメージを command override で実行するため、専用のマイグレーションターゲットは持ちません。

## ビルドターゲット

|ターゲット|ベースイメージ|用途|
|---|---|---|
|`builder`|`golang:1.27.1-alpine`|Go バイナリのビルド（`ldflags` でバージョン / リビジョン / ビルド日時を埋め込み）|
|`runtime`|`alpine:3.24`|本番実行用コンテナ（非 root ユーザー `app`）。command override でマイグレーションも実行|
|`tooling`|`golang:1.27.1-alpine`|ローカル開発環境（ホットリロード + デバッグ）|

## runtime

- 非 root ユーザー（`app`）で実行
- `vendor` モードでビルド（`GOPROXY=off`）
- `-ldflags` でバージョン / リビジョン / ビルド日時を埋め込み
- `env/.env` と `database/migrations` をバイナリへ埋め込み。`builder` ステージが `APP_ENV` ビルド引数で対象 env を材料化する（`go build` 前に `cp env/.env.${APP_ENV} env/.env`）
- デフォルトコマンド: `./server serve`
- マイグレーションは同一イメージを command override（`./server migrate-up`）でデプロイ前に一度だけ実行。専用イメージは不要

## tooling（ローカル開発）

プリインストールされるツール：

|ツール|用途|
|---|---|
|`air`|ホットリロード|
|`dlv`|Go デバッガ|
|`golines`|行長制限付き整形|
|`gofumpt`|gofmt 強化版|
|`golangci-lint`|Go リンター|
|`lefthook`|Git フックランナー|

OS レベルパッケージ: `build-base`, `binutils-gold`, `bash`, `curl`, `git`, `upx`, `libc6-compat`, `gcompat`, `tzdata`, `make`

デフォルトコマンド: `air -c .air.toml`

## docker-compose サービス

```yaml
api_server:
  dockerfile: docker/server/Dockerfile
  target: tooling
  ports: 8080 (API), 2345 (dlv), 6060 (pprof)
```

## 注意点

- `tooling` ターゲットはツールを `mise.toml`（バージョンの SSOT）で pin されたバージョンで install し、ローカルと CI のバージョンを揃える
- すべてのターゲットで作業ディレクトリは `/app`
