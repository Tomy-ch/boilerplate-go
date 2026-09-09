# Go Version Upgrade Procedure

このドキュメントは、このプロジェクトで **Go のバージョンを更新する際の手順**を説明します。

Go のバージョン更新は以下に影響します。

- Go toolchain
- 依存パッケージ
- Go tools
- Code generation
- CI
- Docker images

そのため、以下の手順に従って更新してください。

## 1. Release Notes の確認

Go programming language

対象バージョンの Release Notes を確認します。

確認する主な項目

- language spec の変更
- standard library の breaking change
- `go vet` の変更
- toolchain の変更

例

```text
https://go.dev/doc/devel/release
```

## 2. `mise.toml` を更新し sync を実行

このプロジェクトでは Go バージョンを含む全ツールのバージョンを `mise.toml` を SSOT として管理しています。
Go ランタイムは `actions/setup-go` や `golang:X.Y.Z-alpine` base image との互換のために
いくつかのファイルへ複製する必要があり、`make sync-versions` でこの伝播を行います。

```toml
# mise.toml
[tools]
go = "1.26.3"
# ...
```

そのあと sync を実行します。

```sh
make sync-versions
```

これにより `mise.toml` の go 値が以下のファイルに反映されます。

- `go.mod` ― `go X.Y.Z` directive（CI の `actions/setup-go` が `go-version-file: go.mod` で読む）
- `docker/server/Dockerfile` ― `FROM golang:X.Y.Z-alpine` の行（builder + tooling）
- `docker/tools/Dockerfile` ― `FROM golang:X.Y.Z-alpine` の行（go_tools）

生成された差分は `mise.toml` の bump と一緒にコミットしてください。

## 3. ローカル Go 環境の更新

このプロジェクトではツール管理に **mise が必須** です。同じ mise でローカルの Go ランタイムも管理します。
ステップ 2 のあと、pin された Go をインストールします。

```sh
make go-update
```

内部では `mise install go` が走り、`mise.toml` の go 値を読みます。

確認

```sh
go version
```

### IDE / エディタ統合（VSCode + mise）

VSCode を Dock / Spotlight から起動した場合、shell の初期化処理（mise を
activate している箇所）が実行されないため、Go 拡張がシステム `PATH` から
古い `go` バイナリを拾ってしまうことがあります。プロジェクトの `mise.toml`
と editor を同期させるには、以下のいずれかを実施します:

1. **[mise VSCode 拡張](https://marketplace.visualstudio.com/items?itemName=hverlin.mise-vscode) をインストール（推奨）** —
   VSCode 内部でプロジェクトの mise 環境を自動 activate。
   `.vscode/extensions.json` の recommended extensions に登録済み。
2. **mise が active なターミナルから VSCode を起動する** —
   `code /path/to/repo` で shell の環境を継承。
3. **VSCode の User Settings で `go.alternateTools.go` を mise shim に設定** —

   ```json
   "go.alternateTools": {
     "go": "${env:HOME}/.local/share/mise/shims/go"
   }
   ```

   この設定は **User Settings** に書くこと（プロジェクト `.vscode/settings.json`
   ではなく、プロジェクトのポータビリティ維持のため）。

設定後 VSCode を再起動し、**コマンドパレット → Go: Locate Configured Go Tools**
で active な `go` バイナリが `mise current` の結果と一致することを確認してください。

## 4. CI は `go.mod` を自動参照

GitHub Actions の workflow は `actions/setup-go` の `go-version-file: go.mod` を使用しています。
ステップ 2 の `make sync-versions` が `go.mod` の `go` directive も書き換えるため、
workflow 自体の編集も `go mod edit -go=X.Y.Z` の手動実行も不要です。

## 5. 依存関係と vendor の更新

このプロジェクトでは依存関係の整理に **Makefile タスク `tidy-lib`** を使用します。

```sh
make tidy-lib
```

このタスクは以下を実行します。

- `go mod tidy`
- `go mod vendor`

## 5.5. （任意）Go モジュール依存の更新

Go ランタイムのアップグレードは依存ライブラリをまとめて更新する好機でもあります。このステップは任意で、更新の有無とレベルを選びます。

- **マイナー含む最新** — `go get -u ./...` で全 direct/indirect 依存を同一メジャー内の最新マイナー/パッチへ更新。
- **パッチのみ** — `go get -u=patch ./...` で現行マイナー内に留める（最も安全）。
- **スキップ** — 依存は触らない（Go directive の更新のみ）。

`go get -u` は仕様上メジャーを跨がないため、メジャー更新は別途の意図的な作業とします。

更新する場合:

```sh
go get -u ./...        # または: go get -u=patch ./...
make tidy-lib          # go mod tidy + go mod vendor を再実行
```

実行後は `go.mod` の差分を確認し、`go` directive はアップグレード後のバージョンのまま維持、意図しない `toolchain` 行が追加されていないことを確認します。以降の再ビルド / gen / test / lint で、ランタイム更新と依存更新をまとめて検証します。

本リポジトリは（実 DB を使う infrastructure テストを含む）厚い test + lint を備えており、グリーンならマイナー/パッチ更新は高い信頼度を持ちますが、保証ではありません。DB ドライバ・OpenTelemetry・Web フレームワーク等のランタイム挙動が効くコア依存は、グリーンでも CHANGELOG に目を通してください。

## 6. Go tools の再インストール

Go ランタイムを更新した場合、以前のランタイムでビルドされたツールは再ビルドが望ましいです。mise で再 install します。

```sh
make install-tools
```

インストールされる主なツール（バージョンは `mise.toml` で pin）

- gopls
- golangci-lint
- delve (dlv)
- lefthook
- gotests
- impl
- zizmor
- shellcheck

## 7. Docker イメージは sync で自動反映

`docker/server/Dockerfile` と `docker/tools/Dockerfile` は Go ランタイムが必要なステージで
`FROM golang:X.Y.Z-alpine` を使っています。ステップ 2 の `make sync-versions` がこの
`FROM` 行を自動で書き換えるため、Dockerfile を手動で編集する必要はありません。

Dockerfile 内の Go 以外のツール（air / dlv / golangci-lint 等）も `mise install <tool>` で導入されており、
バージョンは `mise.toml` から解決されます。

## 7.5. base image の digest pin を貼り直す

`make sync-versions` が書き換えるのは `FROM golang:` の**タグ**であり、その隣に固定されている
`@sha256:...` digest は古い Go イメージを指したままです。Docker は digest を優先するため、
放置するとビルドは黙って古いイメージを使い続けます。digest がタグに追随するよう、レジストリから
解決し直してください。

```sh
make pin-images-resolve   # Docker Hub が 429 を返す場合は先に `docker login`
make pin-images-apply
make pin-images-check
```

**Go のリリース当日にこの手順を完了できることは通常ありません。** 新しい `golang:` タグは
lockfile に前回のエントリを持たず、イメージは `PIN_IMAGES_MIN_AGE_DAYS`（既定 14 日）の
クールダウン期間内にあります。退行先となる枯れた digest が無いため、`pin-images-resolve` は
出来立ての digest を採用したりタグのみへ緩めたりせず fail-closed し、続く `pin-images-check` は
残った古い digest を `未登録` として弾きます。lockfile は `docker/images-pin.toml`、ターゲットと
クールダウンの設定値は `.makefiles/README.md` にあります。

つまり Go の更新と base image の pin は連動しています。片付くのは、新しい Go イメージが
クールダウンを抜けたときか、人が意図して `days=0` で bootstrap したときのいずれかです。これは
提起すべき判断であって自分で決めるものではありません。どちらにせよ、タグと digest が食い違った
状態をツリーに残してはいけません。

## 8. Docker コンテナ再ビルド

Go base image のタグが変わるとレイヤキャッシュが失効するため、新しい `golang:` イメージを確実に
pull するために `-clean`（`--no-cache --pull`）バリアントを使います。

サーバー系コンテナ

```sh
make serve-build-clean
```

ツール系コンテナ

```sh
make tool-runners-build-clean
```

## 9. Code generation の再実行

Go version の変更により生成コードが変わる可能性があります。

```sh
make gen
```

## 10. テスト実行

```sh
make go-test
```

または

```sh
go test ./...
```

## 11. Lint 実行

```sh
make go-lint
```

## 12. 最終確認

以下のコマンドがすべて成功することを確認してください。

```sh
make sync-versions
make pin-images-check
make tidy-lib
make install-tools
make gen
make go-test
make go-lint
make serve-build-clean
make tool-runners-build-clean
```

`make sync-versions` をここで再度回すのは、`mise.toml` とそこから導出されるファイルの間に取りこぼしたずれが残っていないかを、完了を報告する前に検出するためです。

## Upgrade Checklist

Go version を更新する際は以下を確認してください。

- [ ] Release Notes 確認
- [ ] `mise.toml` の `go = "..."` 更新
- [ ] `make sync-versions` 実行（`go.mod` の go directive + Dockerfile FROM 再生成）
- [ ] `make go-update` 実行（host 上の Go install）し `go version` で確認
- [ ] `make tidy-lib` 実行
- [ ] （任意）Go モジュール依存を更新するか判断。更新する場合は `go get -u[=patch] ./...` + `make tidy-lib` 実行（`go` directive は据え置き）
- [ ] `make install-tools` 実行
- [ ] base image の digest pin を貼り直す（`make pin-images-resolve` → `apply` → `check`）。クールダウンで `resolve` が fail-closed した場合は押し通さずに判断を提起する
- [ ] Docker コンテナ再ビルド（`make serve-build-clean`, `make tool-runners-build-clean`）
- [ ] code generation 再実行
- [ ] test 実行
- [ ] lint 実行
