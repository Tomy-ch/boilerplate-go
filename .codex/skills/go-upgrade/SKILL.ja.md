> **このファイルは `SKILL.md` の日本語訳です。**
> 直接編集しないでください。内容の変更が必要な場合は canonical な `SKILL.md`（英語版）を更新し、その後この日本語訳を同期してください。
> Claude Code のスキルとしては `SKILL.md` のみが読み込まれます。このファイルはスキル本体ではなく、レビューや学習用の翻訳ドキュメントです。

# Go バージョンアップグレード手順

このスキルは Go のバージョンを任意のバージョンへアップグレードするための作業手順を定義する。

正式な手順書は以下を参照すること。

- `docs/maintenance/go-upgrade.md`

## 最初に行うこと: 対象バージョンの確認

このスキルでは、**スキル起動直後に必ず `ask the user explicitly` で対象バージョンを確認する**。
スキル引数や直近メッセージにバージョンらしき文字列があっても、それを採用して即実行に進んではならない（誤指定を防ぐため、明示的な確認を必須とする）。

手順:

1. `mise.toml` の `[tools]` 配下の `go = "X.Y.Z"` を読んで現行バージョンを把握する。
2. **必ず** `ask the user explicitly` を呼び出して以下を確認する。
    - 質問: 「アップグレード先の Go バージョンを指定してください（例: `1.26.3`）」
    - 補足として、現行バージョン（`mise.toml` の `[tools] go` の値）を提示する。
    - スキル引数や直近メッセージにバージョン候補がある場合は、それを質問文に「候補: `X.Y.Z`」として併記し、ユーザーに確定させる。
3. 受け取った回答が `X.Y.Z` 形式であることを軽く検証し、以下の手順内で `<TARGET_VERSION>` として使用する。

確認が取れるまで、ファイル更新やコマンド実行は行わないこと。

## 前提

- 対象バージョン: `<TARGET_VERSION>`（上記で確認した値）
- 直接 `production` / `develop` / `staging` / `release/*` ブランチで作業しないこと（AGENTS.md の Git ルール参照）
- 作業ブランチを `release/*` の最新から切ること（例: `feature/go-upgrade-<TARGET_VERSION>`）

## AI Modification Scope について

このスキルは AGENTS.md の "Exception: Skill Execution" 節に基づき、スキル実行中に限り通常の AI Modification Scope の縛りを解放する。具体的には以下のパスへの変更がスキル実行中に許可される:

- `mise.toml`（`[tools]` 配下の `go` エントリ）
- `go.mod`, `go.sum`, `vendor/`（`make tidy-lib` が再生成）
- `docker/**/Dockerfile`（`make sync-versions` が自動書き換え。`FROM` の `@sha256:...` digest は `make pin-images-apply` が書き換え）
- `docker/**/README.md` / `README.ja.md`（`make sync-versions` が自動書き換え）
- `docker/images-pin.toml`（base image digest lockfile。`make pin-images-resolve` が書き換え）

ただし以下は引き続き保護対象（スキル実行中でも変更不可）:

- `AGENTS.md` / `AGENTS.md`
- 生成ファイル（`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, `docs/` の生成物）

## 実行ステップ

`docs/maintenance/go-upgrade.md` のステップに対応する形で以下を実施する。コマンド・ファイル内に現れる `<TARGET_VERSION>` は、ユーザーが指定したバージョンに置換すること。

### 1. リリースノートの確認

<https://go.dev/doc/devel/release> で `<TARGET_VERSION>` のリリースノートを確認する。

確認観点:

- 言語仕様の変更
- 標準ライブラリの破壊的変更
- `go vet` の挙動変更
- ツールチェインの変更

### 2. `mise.toml` の更新

`mise.toml` の `[tools]` 配下にある `go` エントリを編集する:

```toml
[tools]
go = "<TARGET_VERSION>"
# （その他のエントリは触らない）
```

`mise.toml` は Go ランタイムバージョン（および全ツールバージョン）の SSOT。下流ファイル群は次のステップで自動再生成される。

### 3. `make sync-versions` で伝播

```sh
make sync-versions
```

`scripts/sync-versions`（Go 実装）が走り、以下のファイルを一括書き換える:

- `go.mod` — `go X.Y.Z` directive
- `docker/server/Dockerfile` — `FROM golang:X.Y.Z-alpine` 行
- `docker/tools/Dockerfile` — `FROM golang:X.Y.Z-alpine`（および他ランタイムの `FROM node:` / `FROM python:`）
- `docker/README.md` / `docker/README.ja.md` — テーブルセル内 `golang:X.Y.Z-alpine`
- `docker/server/README.md` / `docker/server/README.ja.md` — 同上
- `docker/tools/README.md` / `docker/tools/README.ja.md` — 同上

スキルは `mise.toml` 編集後に必ずこのコマンドを自分で実行すること。`go.mod` / Dockerfile / README の手編集は不要。

### 4. ローカル Go 環境の更新

```sh
make go-update
```

内部で `mise install go` が走り、`mise.toml` の `go` エントリを読む。確認:

```sh
go version
```

ローカル環境の更新はユーザー作業となるため、AI エージェントは `mise install` を実行せずユーザーに依頼すること（`go version` が `<TARGET_VERSION>` と一致することの確認も依頼する）。

### 5. 依存関係と vendor の更新

```sh
make tidy-lib
```

（内部で `go mod tidy` と `go mod vendor` を実行）

### 6. （任意）Go モジュール依存の更新

Go ランタイムのアップグレードは依存ライブラリをまとめて更新する好機でもある。**更新するかをユーザーに確認**し、回答に従って実行する。

このステップでは **必ず `ask the user explicitly` を呼び出して**更新レベルを確認する。選択肢:

- **マイナー含む最新（`go get -u ./...`）** — 全 direct/indirect 依存を同一メジャー内の最新マイナー/パッチへ更新。新機能を取り込むが、挙動変化の小さなリスクあり。
- **パッチのみ（`go get -u=patch ./...`）** — 現行マイナー内に留める。最も安全。
- **スキップ** — 依存は触らない（Go directive の更新のみ）。

メジャーバージョンは自動で上げない（`go get -u` は仕様上メジャーを跨がない）。メジャー更新は別途の意図的な作業とする。

ユーザーが更新を選んだ場合:

```sh
go get -u ./...        # または: go get -u=patch ./...
make tidy-lib          # go mod tidy + go mod vendor を再実行
```

実行後は **`go.mod` の差分を確認**すること。`go` directive は `<TARGET_VERSION>` のまま維持し、依存により意図しない `toolchain` 行が追加されていないことを確かめる。以降の再ビルド / gen / test / lint で、ランタイム更新と依存更新をまとめて検証する。

補足: 本リポジトリは（実 DB を使う infrastructure テストを含む）厚い test + lint を備えており、グリーンならマイナー/パッチ更新は高い信頼度を持つ。ただし保証ではない。DB ドライバ・OpenTelemetry・Web フレームワークのようなランタイム挙動が効くコア依存は、グリーンでも CHANGELOG に目を通すこと。

### 7. Go ツールの再インストール

```sh
make install-tools
```

### 8. CI 自動検知の確認

`.github/workflows` 配下の workflow は `actions/setup-go` の `go-version-file: go.mod` を使用している。ステップ 3 で `go.mod` が書き換わるため、workflow 側の編集は不要。

もし Go バージョンを文字列リテラルで直接書いている workflow ファイルがあれば、それは `<TARGET_VERSION>` に手で揃えること。

### 9. Dockerfile / README 同期確認

ステップ 3 で Dockerfile の `FROM` タグおよび `docker/**/README.md` の image 参照は書き換え済み。手動編集は不要。

### 10. base image digest pin の再固定

ステップ 3 は `FROM golang:` の**タグ**を変えたが、以前 pin した `@sha256:...` digest は**旧** Go イメージを指したまま——タグ/digest 不整合になる（Docker は digest を優先するため、ビルドは旧イメージを黙って pull する）。digest が新タグに追従するよう registry から再 pin する。これは `images-pin` スキルの役目（姉妹関係によりここで chain）:

```sh
make pin-images-resolve   # Docker Hub が 429 を返す場合は先に `docker login`
make pin-images-apply
make pin-images-check
```

新しい Go イメージは公開直後のため、再固定は `images-pin` の **ルール 3** に当たる。新しい `golang:` tag には前回の lockfile エントリが無く、イメージは `PIN_IMAGES_MIN_AGE_DAYS`（既定 14 日）の cooldown 内なので、退行先となる aged な digest が存在しない。`pin-images-resolve` は出来立ての digest を採用することも pin を tag のみへ剥がすこともせず **fail-closed** で止まる（`❌ 退行先の無い出来立て image は採用できません`）。`apply` は走らず、`pin-images-check` は残った stale な digest を `未登録` として弾く。

したがって Go のアップグレードと base image の pin は結合しており、次のいずれかが成り立つまでこのアップグレードはきれいに着地しない——新しい Go イメージが窓を越えて古くなるか、ユーザーが意図的に `days=0` でブートストラップするか（`/images-pin` の手順 2.5 が `/supply-chain-triage` の証拠確認を挟む）。この選択は代わりに決めず提示すること。`resolve` を無理に通さず、tag と digest の食い違いをツリーに残さない。詳細は `images-pin` スキル参照。

### 11. Docker コンテナの再ビルド

Go バージョンアップでは base image タグが変わるため、新しいイメージを確実に pull・再ビルドできるよう `-clean`（`--no-cache --pull`）バリアントを使う:

```sh
make serve-build-clean
make tool-runners-build-clean
```

### 12. コード生成の再実行

```sh
make gen
```

### 13. テストの実行

```sh
make go-test
```

### 14. lint の実行

```sh
make go-lint
```

### 15. 最終確認

以下が全て成功することを確認する。

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

`make sync-versions` を最後にも入れているのは、作業中に `mise.toml` と下流ファイルの drift が残っていた場合に検知するため。

## チェックリスト

完了報告時に以下を確認すること。

- [ ] 対象バージョン `<TARGET_VERSION>` を `ask the user explicitly` でユーザーに確認済み
- [ ] リリースノート確認
- [ ] `mise.toml` の `[tools] go` を `<TARGET_VERSION>` に更新
- [ ] `make sync-versions` 実行（go.mod / Dockerfile / docker/**/README.md へ伝播）
- [ ] base image digest を再固定（`make pin-images-resolve` + `pin-images-apply` + `pin-images-check`）。公開直後のイメージでは新 `golang:` tag に対するルール 3 の fail-closed が想定どおりの結果であり、結合（古くなるのを待つか、トリアージのうえ `days=0` でブートストラップするか）とともに提示する。無理に通さず、tag と digest の食い違いを残さない
- [ ] ローカル Go の更新（`make go-update`、ユーザー作業）
- [ ] `make tidy-lib` 実行
- [ ] （任意）Go モジュール依存の更新を `ask the user explicitly` でユーザーに確認。実施する場合は `go get -u[=patch] ./...` + `make tidy-lib` を実行し、`go` directive は `<TARGET_VERSION>` のまま維持
- [ ] `make install-tools` 実行
- [ ] Docker コンテナの再ビルド
- [ ] コード生成の再実行
- [ ] テスト実行
- [ ] lint 実行

## 注意事項

- `go.mod` / Dockerfile / `docker/**/README.md` を手動編集しないこと — これらは `make sync-versions` の管轄。`mise.toml` を編集してから sync を再実行する。
- PR への push は明示的にユーザーから指示があった場合のみ実行すること
- `SKILL.md` を更新したら、日本語訳を同期するため `SKILL.ja.md` も更新すること
