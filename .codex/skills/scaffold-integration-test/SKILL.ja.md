> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Scaffold Integration Test

1 feature 分の HTTP 境界 integration テストを `internal/integration/` 配下に生成するスキル。

ここでいう integration は **DB や usecase 内部のテストではない**。`httptest` で実際の Echo サーバを起動し、**HTTP 経路全体（Router → Middleware → Handler → Presenter）** の振る舞いを、**usecase を mock した状態**で検証する。テスト戦略とスコープの canonical は `internal/integration/README.md`。

## 使うとき

- feature の controller handler を実装し、コンパイルが通った直後
- `scaffold-controller` の最終ステップとして自動 chain（handler 実装直後に HTTP 境界テストが付く）
- 既存 handler に対して integration テストだけ scaffold したい standalone 利用

以下には使わない:

- DB / Repository / usecase ロジックの検証 — それらは unit テスト（domain/usecase）と repository 自身の `make go-test`（実 DB）の担当。ここは HTTP 境界で止める
- handler / usecase / 生成ファイルの変更
- 既存 `<feature>_test.go` への 1 ケース追加 — 手で編集

## 読む / 書く

**読む（常に source of truth として）**:

- `internal/integration/README.md` — テスト戦略 + スコープ（ここで検証する/しない範囲）
- `internal/integration/helper_test.go` — このパッケージのテストヘルパー（役割: HTTP サーバ起動 / リクエスト実行 / JSON レスポンス assert / 認証ヘッダ付与）。**正確な名前・シグネチャは実行時にこのファイルから読む — スキルは一切 hardcode しない。** 存在するヘルパーを使い、HTTP 配線を自作しない
- `internal/integration/<sibling>_test.go`（例 `v1_users_test.go`）— パッケージの書き方の構造テンプレート
- `internal/controller/handler/<path>/<feature>_handler.go` — import する `BindHandler` シグネチャ + handler パッケージ名
- `internal/controller/handler/<path>/gen/server.gen.go` — operationId 一覧 + HTTP メソッド/パス（route コメント）+ request/response 型
- usecase mock パッケージ（例 `internal/usecase/<pkg>/mock`）— mapped usecase メソッド用の mock + `EXPECT()` メソッド名

**書く（確認のうえ）**:

- `internal/integration/<feature>_test.go` — `Test<Feature>_Integration` 1 関数、operationId ごとに 1 subtest

**トリガ（`make` 経由）**:

- `make go-fix` + `make go-test` — 最終検証

**触らない**:

- handler / usecase / domain / infra コード
- 生成 `*.gen.go`
- `internal/integration/helper_test.go`（読み取り専用。必要なヘルパーが無ければ surface するだけで、ここでは編集しない）

## 前提条件

書き込み前に検証:

1. `internal/integration/` が存在し `helper_test.go`（ヘルパー API）がある
2. 対象 handler パッケージが存在し `BindHandler` を公開（controller scaffold 済み + コンパイル可）
3. feature の usecase mock パッケージが存在（生成済み）
4. `internal/integration/<feature>_test.go` が**未存在**（あれば中断 — 手動編集に委ねる）

前提未充足時は明示メッセージで中断（`/scaffold-controller`、mock 用 `make gen-api`、手動 cleanup 等の案内付き）。

## 最初のステップ: identity 解決

このスキルは**起動直後に必ず `ask the user explicitly` を呼ぶ**（`scaffold-controller` から feature + handler パス + usecase パッケージを受け取って chain された場合は省略）:

1. 質問: 「対象 feature 名 (kebab-case) / 出力ファイル名 (`internal/integration/<feature>_test.go`)」 — free-text
2. 質問: 「handler package パス (`internal/controller/handler/` 配下)」 — free-text、ディレクトリ存在を検証
3. 質問: 「対応する usecase package 名 (mock の所在)」 — free-text、`internal/usecase/<name>/mock` 存在を検証

## Step 1. 入力読み込み

1. `internal/integration/README.md` を読み、スコープ（HTTP 境界のみ、usecase mock、DB なし）を再確認
2. `internal/integration/helper_test.go` を読み、利用可能なヘルパーとシグネチャを列挙（hardcode せず、その実行で読む）
3. sibling `<sibling>_test.go` を 1 つ読み、構造テンプレートとする（import、`echo.New()`、mock 配線、サーバ起動、assert、認証ヘッダ付与）
4. handler `gen/server.gen.go` を読み、operationId ごとに HTTP メソッド + パス（route コメント）/ request 型 / response 型 + 2xx レスポンスコンストラクタを把握
5. handler `<feature>_handler.go` を読み、`BindHandler` シグネチャ + 認証が必要な operation（認証コンテキスト参照の有無）を把握
6. usecase mock パッケージを読み、各 operation で stub する mock コンストラクタ + `EXPECT()` メソッド名を把握

## Step 2. operation ごとのテスト計画

handler `gen` の各 operationId について:

- HTTP メソッド + パス（例 `GET /v1/<リソース>/{<id>}`）
- handler が呼ぶ usecase メソッド（`<feature>_handler.go` から読む）→ 設定する mock `EXPECT()`
- 認証が必要か → 必要なら認証ヘッダ付与ヘルパーを使う
- 正常系の期待（status + 最小の JSON ボディ形状）と、安価に書ける範囲で代表的なエラー系 1 つ（例: usecase が `apperror.ErrNotFound` を返す → 404 を期待）で、HTTP 経路上の errorhandler middleware マッピングを確認

HTTP 境界に留めること: status コードとレスポンスシリアライズを assert し、業務的な結果は assert しない（usecase は mock）。

## Step 3. テスト観点 subagent

Agent ツール（`subagent_type: general-purpose`）で、書き込み前に HTTP 境界の観点を列挙。`internal/integration/README.md` + operation ごとの計画を渡す。期待観点:

- ルーティング到達性（メソッド + パスが handler に解決される）
- リクエストのデシリアライズ（path param / body）と 2xx レスポンスのシリアライズ形状
- 実際の middleware スタックを通した apperror → HTTP status マッピング（NotFound → 404、validation → 422/400 等）
- 認証必須 operation: 認証ヘッダあり / （意味があれば）なし
- HTTP 経路全体でしか現れない middleware 効果（request id、force-json、recovery）が関係する場合

出力: operation ごとの subtest リスト（正常系 + この層で assert する価値のあるエラー/認証系）。

## Step 4. 計画提示と確認

日本語サマリを表示: 出力ファイル、operation ごとの subtest（メソッド + パス + mock する usecase メソッド + 認証メモ + 期待 status）。その後:

- 「以下の構成で integration テストを生成しますか？」
- 選択肢: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. テストファイル書き込み

`internal/integration/<feature>_test.go` を書く:

- `package integration`
- `func Test<Feature>_Integration(t *testing.T)` 1 つ、先頭で `t.Parallel()`
- 計画した subtest ごとに `t.Run("<日本語の振る舞い説明>", ...)`。各 subtest は **sibling `<feature>_test.go` と Step 1 で読んだヘルパーを写して**組み立てる — ヘルパー名を仮定せず、`helper_test.go` が実際に公開しているものを使う。各 subtest の形:
  - 新しい Echo インスタンス + no-op tracer factory + `gomock` controller を用意
  - usecase mock を構築し、mapped メソッドの `EXPECT()` を設定（既定の DTO / error を返す）
  - パッケージの `BindHandler` で handler を bind
  - 認証必須 operation: 認証ヘッダ付与ヘルパーでヘッダを取得
  - リクエスト実行ヘルパーでエンドポイント（メソッド + パス + body + ヘッダ）を叩き、レスポンスを取得
  - status コードを assert。正常系は期待する `gen` レスポンス struct を組み立て JSON assert ヘルパーで比較
- 日本語の subtest 名。sibling ファイルのイディオム（実際のヘルパー名、`mock_<pkg>` 等の import エイリアス、tracer factory コンストラクタ）を正確に写す。すべてのヘルパー呼び出しの source of truth は sibling テスト + `helper_test.go` であり、上記は固定の識別子ではなく役割の記述。

## Step 6. 検証

```sh
make go-fix
make go-test
```

> **環境に関する注記:** `make go-test` は同一実行内の DB 依存スイートのために開発環境が起動している必要がある。**生 `docker compose` ではなく専用 make ターゲット**で起動する（`make serve` → **`make db-init`**。local/test 両 DB を migrate **かつ seed** する。スイートは seed 前提のため、`db-*-migrate-up` 単体では不十分）。`make go-fix` / `make go-test` がツールのバージョン不整合（例: `golangci-lint` の v1/v2 config エラー）で失敗した場合は、`PATH` の手動書き換えではなく `make install-tools` で揃えてから再実行する（`mise.toml` 変更時は先に `make sync-versions`）。

失敗時: 失敗テスト出力を surface + 該当ケースに `// TODO:` + FB サマリ。自動 rollback はしない。

## Step 7. 終了

```text
<Feature> の HTTP 境界 integration テストを internal/integration/<feature>_test.go に生成しました。
<N> 件の subtest、make go-test OK。
```

commit はしない。

## AI 改変スコープ

「Exception: Skill Execution」条項に従う:

- 書き込みスコープ: `internal/integration/<feature>_test.go` のみ
- 同ファイルが既存なら中断

保護対象のまま:

- `internal/integration/helper_test.go`（読み取り専用）
- handler / usecase / domain / infra コードと生成ファイル

## 制約

- ❌ ここで DB / Repository / usecase 業務ロジックを検証（HTTP 境界のみ、usecase は mock）
- ❌ `helper_test.go` の編集や HTTP 配線の自作（既存ヘルパーを使う）
- ❌ handler / usecase / 生成ファイルの変更
- ❌ 既存 `<feature>_test.go` の上書き
- ❌ テスト観点 subagent や確認 `ask the user explicitly` の省略
- ❌ 失敗時の自動 rollback（TODO + FB）
- ✅ 日本語の出力と日本語 subtest 名
- ✅ `t.Parallel()`、生成 mock、既存ヘルパー
- ✅ sibling `<feature>_test.go` を構造テンプレートとする
- ✅ operationId ごとに 1 subtest、status + レスポンスシリアライズを assert

## チェックリスト

- [ ] identity 確認（feature / handler パス / usecase パッケージ）または `scaffold-controller` から受領
- [ ] 前提検証（helper_test.go 存在、BindHandler 存在、mock 存在、対象ファイル未存在）
- [ ] `internal/integration/README.md` + `helper_test.go` + sibling テストをその実行で読んだ
- [ ] handler `gen` の operationId + メソッド/パス + mapped usecase メソッド + 認証フラグを導出
- [ ] テスト観点 subagent を起動、書き込み前に観点を捕捉
- [ ] 計画を提示し `ask the user explicitly` で確認
- [ ] `internal/integration/<feature>_test.go` を既存ヘルパー・日本語 subtest 名・`t.Parallel()` で書いた
- [ ] `make go-fix` + `make go-test` 実行（または失敗を TODO + FB で surface）
- [ ] commit / push なし
- [ ] 最終サマリは日本語
