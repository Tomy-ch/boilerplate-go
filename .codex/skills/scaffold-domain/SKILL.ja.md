> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# Scaffold Domain

`docs/spec/domain/<pkgpath>.md`（`<pkgpath>` は `internal/domain/` 以下のパッケージパス）を入力に、1 feature の domain 層を 1 パスで生成するスキル。entity / Repository IF / VO / constant / error / getter / test を生成する。

## 使うとき

- `new-spec` または手書きで domain spec が完成し、検証も済んだ後
- `scaffold-endpoint` の最初のステップ（自動 chain）
- domain 層だけを scaffold したい単独利用

以下の用途には使いません:

- 既存 domain パッケージの変更（スキルは新規 aggregate ディレクトリ前提）
- 既存 entity に新メソッド 1 つ追加 — 手編集
- spec に宣言されていない業務ロジックの記述

## 読み書き範囲

**読み込み（常時）**:

- `docs/spec/domain/<pkgpath>.md` — single source of truth
- `internal/domain/README.md` — layer 規約
- 既存の sibling aggregate package 1〜2 件（`internal/domain/<sibling>/`）— 構造 template
- `internal/domain/<aggregate>/` — 既存確認（あれば中断）

**書き込み（承認後）**:

- `internal/domain/<aggregate>/<aggregate>_domain.go`
- `internal/domain/<aggregate>/<aggregate>_repository.go`（`//go:generate mockgen` 付き）
- `internal/domain/<aggregate>/constant.go`
- `internal/domain/<aggregate>/error.go`
- `internal/domain/<aggregate>/<value_object>.go`（VO ごと）
- `internal/domain/<aggregate>/<aggregate>_domain_test.go`
- `internal/domain/<aggregate>/<value_object>_test.go`

**`make` トリガ**:

- `make gen-api` — `internal/domain/<aggregate>/mock/` 配下に Repository mock 再生成
- `make go-fix` + `make go-test` — 最終検証

**触らない**:

- 他 aggregate のディレクトリ
- `internal/domain/<aggregate>/` の外
- spec ファイル

## 最初のステップ: spec パス解決

`ask the user explicitly` を起動直後に呼ぶ（`scaffold-endpoint` から呼ばれて context にパッケージパスがある場合は除く）:

- 質問: 「対象 domain パッケージパス（`internal/domain/` 以下。例: `cart`, `product/category`）」
- フリーテキスト。`docs/spec/domain/<pkgpath>.md` として解決 — spec のツリーはパッケージのツリーを写すので、対象パッケージと spec パスは同じ 1 つの回答である

スタンドアロン代替: `--spec=<path>` 引数で規約外パスを指定可能。

spec ファイルが無ければ中断し、`/new-spec` を案内する。
`internal/domain/<aggregate>/` が既にあれば、手書きコードの clobber を避けるため中断する。

## Step 1. spec + README context 読み込み

1. `docs/spec/domain/<pkgpath>.md` を全文読み込み。YAML コードブロックを inventory にパース:
   - `entity`: package, struct, fields（name, type, required, min/max 等）
   - `cross_field_invariants`: 制約式リスト
   - `behavior_methods`: (name, signature, description) リスト
   - `value_objects`: (name, underlying_type, validation, factory, methods) リスト
   - `repository_methods`: (name, signature, behavior) リスト

2. `internal/domain/README.md` を読む（特に "Do / Don't"、"Handling time and ID"、"Invariants" 節）

3. `internal/domain/README.md` を権威的な convention 源として使う — naming / getter style / `ptr.Copy` 使用 / error wrap パターン / ファイル分離規約は README の `Implementation notes` / `Aggregate Design` / `Testing strategy` / `Do / Don't` 節が canonical。既存 aggregate コード（`internal/domain/<sibling>/*.go`）は import / file layout / フォーマットスタイルの **二次的** 参照のみ — README と衝突した場合は README が勝つ（README から drift した実装に skill が黙って従わない方針）。

YAML パース失敗時は中断 → `/verify-spec` を案内。

## Step 2. test 観点 subagent

実装前に Codex delegation で subagent を起動して domain 層の test 観点を列挙させる。これにより実装が domain 観点で test-driven になる。

- `subagent_type: general-purpose`
- prompt 内容（日本語）: パース済み spec inventory + `internal/domain/README.md` の `Test Strategy` 節 + domain 層期待観点リスト:
  - invariant 保護（constructor + 状態遷移メソッド）
  - 各フィールド境界値（min length / max length / min / max）
  - VO 境界値
  - immutability（pointer field の defensive copy）
  - cross-field invariant 検証（`updatedAt >= createdAt` 等）
  - error 分類 (`require.ErrorIs` での特定エラー検証)
- 期待出力: スキルが生成すべきテストケース一覧、entity / VO 別構造化

subagent が観点を返さない場合は最小デフォルトで継続し、user に警告。

## Step 3. 自動派生要素の決定

spec に**書かれていない**が convention で決まる要素を派生:

- **Errors**: validation のある全フィールドに `ErrInvalid<Field>` を `error.go` に生成。VO には `ErrInvalid<VO>`。group root `errInvalid := xerrors.Wrap(apperror.ErrValidation, "invalid <aggregate>")` で包む
- **Field identifiers + collect-all validation**: ユーザーが修正できる入力フィールド（クライアントが送信し修正可能なフィールド）ごとに、API リクエストのプロパティ名と一致する `Field<Name> = "<property>"` 定数を `constant.go` に生成し、入力フィールドの検証は失敗を**すべて**収集する形にする — 失敗フィールドごとに `xerrors.Wrap(ErrInvalid<Field>, msg)` と `Field<Name>` を append し、`return apperror.WithDetails(xerrors.Join(errs...), fields...)` — これにより API は不正フィールドを一度にすべて報告できる（`details`）。サーバ内部の不変条件（id・タイムスタンプ・パスワードハッシュ）は first-error return のまま: ユーザーが修正できる入力ではないため。正準の形は README の `Errors` 節、理由は ADR-0048 (error-metadata-code-message-details) にある。理由文はラップしたメッセージ側（ログ専用）に残し、識別子には決して入れない
- **Constants**: `min_length` / `max_length` 持つフィールドに `min<Field>Length` / `max<Field>Length`。`min` / `max` 数値フィールドに対応定数
- **Getters**: 全 unexported field に `func (e *Entity) Field() T { return e.field }` を 1 行で生成。pointer は `return ptr.Copy(e.field)`
- **ID validation**: `uuid.UUID` 型かつ `id` or `<x>ID` 名のフィールドに constructor で `if id.IsNil() { return nil, xerrors.Wrap(ErrInvalidID, "...") }`
- **単純な型検証**: string + min/max length は `if ok, msg := stringkit.ValidateInRange(field, minXLength, maxXLength); !ok`。nullable string は nil 許容 + 値があるとき範囲チェック（`if x != nil { if ok, msg := stringkit.ValidateInRange(*x, ...); !ok { ... } }`）

## Step 4. プランと承認

日本語で生成予定ファイル一覧 + `<aggregate>_domain.go` 冒頭 10 行プレビューを表示:

- 質問: 「以下の構成で domain 層を生成しますか？」
- 選択肢: 「生成する」 / 「修正したい箇所を指摘する」 / 「キャンセル」

## Step 5. ファイル書き込み

依存順序で書き込み（パッケージ内 cross-reference を整合させるため）:

1. `constant.go`（依存なし）
2. `error.go`（依存なし）
3. `<aggregate>_domain.go`（entity + constructor、constant/error に依存）
4. `<aggregate>_repository.go`（Repository interface + `//go:generate mockgen`）
5. `<value_object>.go`（VO ごと、error に依存）
6. `<aggregate>_domain_test.go`（subagent 観点リスト使用）
7. `<value_object>_test.go`（VO ごと）

各ファイルは Step 1 #3 で読んだ既存 aggregate の style（import、コメント、フォーマット）を踏襲。

## Step 6. mock 再生成

```sh
make gen-api
```

`<aggregate>_repository.go` の `//go:generate mockgen` を処理し `internal/domain/<aggregate>/mock/mock_<aggregate>_repository.go.gen.go` を生成。コマンド後にファイル存在確認。

## Step 7. 検証

```sh
make go-fix    # フォーマット
make go-test   # コンパイル + テスト + カバレッジ
```

`internal/domain/<aggregate>` のカバレッジ行を確認。domain テストはプロジェクト規約で 100% 必須。低下時は未テストブランチ（多くは invariant or VO factory パス）を特定し、テスト追記。

`make go-test` 失敗時:

- 失敗を surface
- 該当ファイルに TODO コメント書き込み
- 自動 rollback しない。FB summary を出して user に fix forward を依頼

## Step 8. クロージング

1 行サマリ:

```text
<Aggregate> domain 層を生成しました。<N> ファイル作成、make go-test OK、coverage 100%。
次は scaffold-infra-db で repository 実装、または scaffold-endpoint で残層を続行できます。
```

commit しない。次の scaffold skill を起動しない。

## AI 修正スコープ

"Exception: Skill Execution" 緩和:

- 書き込みスコープ: `internal/domain/<aggregate>/` のみ
- 既存ディレクトリがあれば skill 起動を拒否（clobber 防止）

保護対象:

- 他 aggregate のディレクトリ
- `docs/spec/` ファイル（read only）
- mock ファイルは `make gen-api` 経由のみ、手編集不可

## 制約事項

- ❌ コードを言い換える／*なぜ*その設計にしたかを説明するコメントを足す — コードコメントは最小（振る舞い・契約のみ）。理由は commit message / README に置きコードに書かない。宣言の godoc（unexported 含む）は1行で残す。**分量も対象**: このスキルが生成する面は構造上すべて慣用的であり、コンストラクタ / Params 構造体 / 行→エンティティ変換 / handler テンプレートに複数行の説明を付けるのはノイズ。契約を1行で述べて止める。`docs/rules.md` にある repo 全体のルールを書き写さない。抑制であって根絶ではなく、真に非自明な Why は残す。
- ❌ spec に無いフィールド / メソッド / error / constant を発明
- ❌ layer 規約をハードコード — 必ず `internal/domain/README.md` + 既存 aggregate を template に
- ❌ test 観点 subagent (Step 2) をスキップ
- ❌ spec 確認 `ask the user explicitly` をスキップ
- ❌ 既存 aggregate ディレクトリの上書き
- ❌ mock ファイルの手編集
- ❌ 失敗時の自動 rollback（TODO + FB 使用）
- ✅ ユーザー向け出力 / テストケース名は日本語（AGENTS.md 出力ルール）
- ✅ Step 3 の自動派生で spec を minimal に保つ
- ✅ domain パッケージのカバレッジ 100% 到達（プロジェクト規約）
- ✅ `make gen-api` で mock 再生成

## チェックリスト

- [ ] spec パスを解決しファイル存在確認
- [ ] aggregate ディレクトリが未存在（あれば中断）
- [ ] spec を読み全 YAML ブロックをパース成功
- [ ] `internal/domain/README.md`（canonical）+ 既存 sibling aggregate（二次 template）を読み込み
- [ ] test 観点 subagent を起動、書き込み前に観点取得
- [ ] 自動派生（constants / errors / getters / ID チェック）を spec から計算
- [ ] プラン表示 + `ask the user explicitly` 確認
- [ ] 依存順序（constant → error → entity → repository → VO → tests）で書き込み
- [ ] `make gen-api` 実行、mock ファイル存在確認
- [ ] `make go-fix` + `make go-test` 実行、カバレッジ 100% 確認（または失敗時 TODO + FB）
- [ ] commit / push なし
- [ ] 最終サマリは日本語
