> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Scaffold Infra DB

1 feature の infrastructure (RDB) 層 Repository を生成するスキル。**lean A: spec ファイルなし** — Repository は domain Repository IF + sqlc gen 関数から命名規約経由で導出。

## 使うとき

- `scaffold-domain` が Repository interface を作成済み AND `make gen-query` で sqlc gen 関数生成済み
- `scaffold-endpoint` の 2 番目の step として自動 chain（`scaffold-domain` の後）
- infra 層のみ scaffold する standalone 利用

以下の用途には使いません:

- SQL 生成 / `make gen-query` 実行（スコープ外、前提）
- 既存 Repository 実装に 1 method 追加 — 手で edit
- 非 DB infrastructure 実装

## 読み書き範囲

**読み込み（常時）**:

- `internal/domain/<aggregate>/<aggregate>_repository.go` — 実装対象 interface（method 一覧 + signature）
- `internal/infrastructure/rdb/sqlc/gen/*.gen.go` — 利用可能 sqlc 関数（mapping 導出用）
- `internal/infrastructure/rdb/README.md` — 命名規約 + 実装規則
- `internal/infrastructure/README.md` — infra 層規約
- `internal/infrastructure/rdb/pgerror/README.md` — エラー正規化規則（SQLSTATE mapping、single-normalization-point 原則）
- `internal/infrastructure/rdb/repository/<sibling>/<sibling>_repository.go` — **de facto reference 実装**（infra READMEs は principles を文章で記述するが完全な impl snippet はないため、sibling コードが最も近い具体例。ただし README ルールと衝突した場合は README が勝つ）
- `internal/di/module/persistence.go` — DI 登録対象

**書き込み（承認後）**:

- `internal/infrastructure/rdb/repository/<aggregate>/<aggregate>_repository.go`
- `internal/infrastructure/rdb/repository/<aggregate>/<aggregate>_repository_test.go`
- `internal/di/module/persistence.go`（`fx.Provide(<aggregate>.New)` 追加）

**Triggers (via `make`)**:

- `make go-fix` + `make go-test` — 最終検証

**触らない**:

- SQL ファイル / sqlc gen 出力（read-only 参照）
- 他 aggregate の repository ディレクトリ
- domain 層

## 前提条件

skill が書き込み前に検証:

1. `internal/domain/<aggregate>/<aggregate>_repository.go` 存在、Repository interface 含む
2. `internal/infrastructure/rdb/sqlc/gen/` 存在、sqlc gen 関数あり
3. `internal/infrastructure/rdb/repository/<aggregate>/` **未存在**（あれば中断）

前提未充足時は明示メッセージで中断（`/scaffold-domain`、`make gen-query`、手動 cleanup 等の案内付き）。

> **環境に関する注記:** `make gen-query` は `pg_dump` で稼働中の DB スキーマをダンプするため、先に DB が起動している必要がある。準備は**生 `docker compose` ではなく専用 make ターゲット**で行うこと: `make serve`（development プロファイル、`database` サービス含む）→ **`make db-init`** → `make gen-query`。`make db-init` は local/test 両 DB を一括で migrate **かつ seed** する。本 skill が書く integration テスト（`make go-test`）も稼働中かつ **seed 済み**の test DB を要するため、`db-*-migrate-up` 単体ではなく `db-init` が正しい準備手順。
>
> **ツールチェーンに関する注記:** 最終の `make go-fix` / `make go-test`（または `make go-lint`）がツールのバージョン不整合（例: `golangci-lint` の v1/v2 config エラー）で失敗した場合は、`PATH` の手動書き換えではなく `make install-tools` でローカルのツールを揃えてから再実行する（`mise.toml` を変更した場合は先に `make sync-versions`）。

## 最初のステップ: identity 確認

`AskUserQuestion` を起動直後に必ず呼ぶ（`scaffold-endpoint` から呼ばれて context に aggregate 名がある場合は除く）:

- 質問: 「対象 aggregate 名 (PascalCase, e.g., `User`)」 — `internal/domain/<aggregate-lower>/` + `internal/infrastructure/rdb/repository/<aggregate-lower>/` に解決

## Step 1. 入力読み込み

1. `internal/domain/<aggregate>/<aggregate>_repository.go` を読んで Repository interface method 一覧抽出（signature 付き）
2. `internal/infrastructure/rdb/sqlc/gen/*.gen.go` を読んで sqlc 生成関数一覧抽出（`Queries` メソッド）
3. `internal/infrastructure/rdb/README.md` から命名規約（Repository method 名がどう sqlc gen 関数名にマップされるか）と実装規則を取得
4. `internal/infrastructure/README.md` から layer 規約取得
5. `internal/infrastructure/rdb/pgerror/README.md` を読んで SQLSTATE → apperror mapping と single-normalization-point 原則（全 sqlc 呼び出しの error は必ず `pgerror.NormalizeError` 経由）を確認
6. 1 個の sibling repository（`internal/infrastructure/rdb/repository/<sibling>/<sibling>_repository.go` 等）を **具体 reference** として参照 — tracer 配線、`gen.New(driver.New(ctx, r.db))` 利用、pgerror 正規化位置、変換ヘルパー pattern。infra READMEs に完全 code snippet 無いため sibling が最も近い具体例。衝突時は READMEs が勝つ

## Step 2. mapping 導出（lean A の核）

各 Repository method について、対応する sqlc gen 関数を name match heuristic で探す:

- exact match: `Repository.Save` ↔ `Queries.SaveUser` → ✓ mapped
- stem match: `FindByActive` ↔ `Queries.ListUsers` / `ListActiveUsers` / `ListDeletedUsers`（switch dispatch 用に多 target） → ✓ mapped (multi)
- aggregate 認識: `Repository.Find` ↔ `Queries.GetUser` → ✓ mapped (synonyms)

mapping 不能な Repository method について:

- 中断せず、**TODO 付き method stub を生成**:

  ```go
  func (r *repository) CountByActive(ctx context.Context, status user.ActiveStatus) (int, error) {
      ctx, endSpan := r.tracer.Start(ctx)
      defer endSpan()

      // TODO: CountByActive に対応する sqlc gen 関数が見当たりません。
      // 解決方法:
      //   1. database/dml/repository/<aggregate>/*.sql に CountByActive query を追加
      //   2. make gen-query を実行
      //   3. 本 TODO を消して sqlc gen を呼ぶ実装に置き換え
      return 0, errors.New("not implemented")
  }
  ```

- 最終レポートで TODO stub が付いた method を surface

scaffold-controller（mapping 失敗で中断）と異なり、Repository は部分実装でも compile 可能（mapped method のみ動作）。

## Step 3. test 観点 subagent

Agent tool を起動して infra 層 test 観点を実装前に列挙:

- `subagent_type: general-purpose`
- prompt（日本語）: 導出した mapping + `internal/infrastructure/rdb/README.md` `Test Strategy` + 期待される infra 観点:
  - real DB + rollback isolation per test
  - sqlc gen wrap の正当性（param mapping）
  - pgerror 正規化パス（unique violation、no rows、connection error）
  - observability span 発行
  - 並列実行安全性（Tx serialization）
- 出力: method ごとの test case 構造化リスト

## Step 4. 計画と承認

日本語サマリ表示:

- 作成ファイル + DI モジュール更新
- 各 Repository method: signature + 対応 sqlc gen 関数（or unmapped なら TODO マーカー）
- subagent 由来の test method list

質問:

- 「以下の構成で infra-db 層を生成しますか？」
- 選択肢: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. ファイル書き込み

順序:

1. `<aggregate>_repository.go` — 主実装
2. `<aggregate>_repository_test.go` — testkit backed integration test
3. `internal/di/module/persistence.go` 更新 — `repository` module に `<aggregate>.New` 追加

実装ファイル規約:

- `package <aggregate>`
- `type repository struct { db driver.DatabaseDriver; tracer observability.LayerTracer }`
- `func New(db driver.DatabaseDriver, tf observability.TracerFactory) <domain>.Repository { return &repository{db: db, tracer: tf.Infra()} }`
- 各 method (mapped):
  - `ctx, endSpan := r.tracer.Start(ctx); defer endSpan()`
  - `db := gen.New(driver.New(ctx, r.db))`（SQL ログ / トレースは driver の接続層で付与。repository ごとのラッパーは不要）
  - domain params → sqlc params マップ（名前ベース 1:1、型調整）
  - `db.<SqlcFunc>(ctx, params)` 呼び出し
  - sqlc 行 → domain entity 変換（sibling pattern 準拠）
  - エラーを `pgerror.NormalizeError(err)` で wrap
- 各 method (unmapped): Step 2 の TODO stub
- 複雑なヘルパー（多行 → slice）: sibling pattern（同ファイル内のヘルパー関数）

テストファイル規約:

- `testkit` で real DB + rollback
- 日本語 subtest 名
- mapped method のみテスト; unmapped TODO method は skip stub

## Step 6. 検証

```sh
make go-fix
make go-test
```

Repository package coverage 確認。infra 層は ≥85% target。失敗時は TODO + FB、自動 rollback なし。

> **DI 検証（runtime）:** `go build` / `make go-test` は Fx グラフを構築しない — provider 欠落・`New` の未登録・コンストラクタのシグネチャ不整合は、コンパイル/テストではなく**アプリ起動時**に初めて失敗する。DI 登録（`fx.Provide(<aggregate>.New)`）後はアプリが実際に起動するか確認する: `make serve` 稼働中なら保存で `air` が再ビルドするので、`api_server` のログが `[Fx] RUNNING`（"http server started"）に到達し、Fx の `provide` / `invoke` エラーが無いことを確認する。新規環境の注意: コンテナは **vendor モード**でビルドするため先に `make tidy-lib`（`vendor/` 生成）を実行する — 未生成だと Fx 実行前に `inconsistent vendoring` で失敗する。

## Step 7. クロージング

```text
<Aggregate> infra-db 層を生成しました。<N> ファイル作成 + DI 1 行追加。
  mapped: <X> methods (sqlc gen 経由)
  unmapped: <Y> methods (TODO stub、SQL 追加 + make gen-query 後に再 scaffold or 手動実装)
make go-test OK、coverage <Z>%。
次は scaffold-usecase で application service、または scaffold-endpoint で残層を続行できます。
```

commit しない。

## AI 修正スコープ

「Exception: Skill Execution」clause により:

- 書き込み scope: `internal/infrastructure/rdb/repository/<aggregate>/`（新規 dir）+ `internal/di/module/persistence.go`（1 行追加）
- aggregate ディレクトリ既存時は中断

触らない:

- SQL / sqlc gen 出力（read-only 参照）
- 他 repository / query_service / system_query ディレクトリ
- domain 層

## 制約事項

- ❌ コードを言い換える／*なぜ*その設計にしたかを説明するコメントを足す — コードコメントは最小（振る舞い・契約のみ）。理由は commit message / README に置きコードに書かない。宣言の godoc（unexported 含む）は1行で残す。**分量も対象**: このスキルが生成する面は構造上すべて慣用的であり、コンストラクタ / Params 構造体 / 行→エンティティ変換 / handler テンプレートに複数行の説明を付けるのはノイズ。契約を1行で述べて止める。`docs/rules.md` にある repo 全体のルールを書き写さない。抑制であって根絶ではなく、真に非自明な Why は残す。
- ❌ Repository に業務ロジック発明（データ orchestration のみ）
- ❌ SQL 生成 / `make gen-query` 実行（スコープ外）
- ❌ sqlc 生成ファイル手 edit
- ❌ test 観点 subagent スキップ
- ❌ identity 確認 `AskUserQuestion` スキップ
- ❌ 既存 aggregate repository ディレクトリ上書き
- ❌ 他 aggregate の repository に触る
- ❌ 失敗時 auto-rollback（TODO + FB）
- ❌ unmapped method に dummy sqlc 関数を auto 生成（TODO stub + hand-off）
- ✅ ユーザー向け出力 + テストケース名は日本語
- ✅ 既存 `repository/<sibling>/` を具体 reference として利用（infra READMEs に完全 impl snippet 無いため）。ルール衝突時は READMEs が勝つ
- ✅ mapping 導出は命名規約 + sibling pattern のみ
- ✅ 同 skill 実行内で DI 登録更新
- ✅ 部分実装 OK — mapped method は生成、unmapped method は TODO stub

## チェックリスト

- [ ] identity (aggregate 名) 確認
- [ ] 前提検証（domain IF 存在、sqlc gen 存在、対象ディレクトリ未存在）
- [ ] domain Repository IF を read（authoritative signature）
- [ ] sqlc gen 関数一覧抽出
- [ ] `internal/infrastructure/rdb/README.md` + `pgerror/README.md` + sibling repo を read（READMEs canonical、sibling は具体 reference）
- [ ] mapping 導出完了（mapped + unmapped 両リスト準備）
- [ ] test 観点 subagent 起動
- [ ] 計画表示（mapped + unmapped 明確に区別）し承認
- [ ] 実装ファイル書き込み; mapped method は sqlc gen 呼び出し、unmapped method は TODO stub
- [ ] テストファイル書き込み; mapped method のみテスト
- [ ] `internal/di/module/persistence.go` 更新（新 `fx.Provide`）
- [ ] `make go-fix` + `make go-test` 実行; coverage 報告（or 失敗 surface）
- [ ] 最終サマリで mapped count + unmapped count + 次手順案内を明示
- [ ] commit / push なし
