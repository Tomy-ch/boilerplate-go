> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。直接編集せず、更新は `SKILL.md` から流します。

# Scaffold Endpoint

feature を「今いる地点」— ラフなアイデアでも、書き上げた spec でも — から、レビュー済みで稼働するオニオンアーキ endpoint まで運ぶトップレベルオーケストレータ。**feature-dev 由来の上流設計フェーズ**（明確化 → 探索 → 設計 → 入力ドラフト作成）を、無改変の**決定論的な spec 駆動コア**（検証 → 各層 scaffold → テスト → curl）に接ぎ木し、最後に**このリポジトリ自身のレビュースキルで構成した品質レビュー**で締める。

コアの強みは決定論性にある: spec + OpenAPI + SQL が生成コードを完全に決める。上流フェーズは、それらの入力がまだ無いときに*生成する*ためだけに存在し、コアを迂回・弱体化させることは決してない。

## 使うとき

- 新規 feature / endpoint を end-to-end で立ち上げる — アイデアしか無くても、2 spec（domain + usecase）+ OpenAPI YAML + SQL が用意済みでも。
- コードを書く*前*に上流の設計フェーズ（曖昧点の明確化 → 既存パターンの探索 → アプローチの比較）を回したい。
- 全層を同じ規約で構築し 1 つの統合レポートを得た上で、`impl-review` / `arch-check` / `test-review` でレビューしたい。

以下の用途には使いません:

- 既存の単一 layer の変更 — 該当 layer skill（`scaffold-domain` / `-infra-db` / `-usecase` / `-controller`）を単独実行。
- 空の spec テンプレートだけの scaffold — それは `new-spec`。
- 既存コードのレビューのみ — `impl-review` / `arch-check` / `test-review` を直接実行。

## 2 つのエントリモード（Phase 0 で自動判定）

| モード | トリガ | 上流フェーズ (Phase 1–4) | コア (Phase 5–7) |
| --- | --- | --- | --- |
| **A. idea-first** | アイデア / 要件から開始。domain spec または usecase spec がそれぞれのツリーに無い | **実行** — 明確化 → 探索 → 設計 → 入力ドラフト | 実行 |
| **B. specs-ready** | `docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md` + OpenAPI gen + sqlc gen が既に存在 | **スキップ**（fast path） | 実行 |

有効な入力を既に持つユーザーに上流フェーズを強制しない — 検出してスキップする。逆に、spec がまだ無いのに `verify-spec` へ直行してはいけない — そこが上流フェーズの埋める穴。

## 読み書き範囲

**読み込み（常時）**:

- `docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md`（child skill 経由、lean A: 2 spec のみ）。
- OpenAPI gen + sqlc gen + domain Repository IF — controller / infra の導出元（spec なし）。
- `.claude/scaffold-spec/lifecycle.md` — canonical workflow / scaffold execution order。
- layer `README.md` + `docs/` を実行時に参照（上流フェーズは既存パターンの真の出所として読む。drift する設計ルールをハードコードしない）。

**書き込み**:

- **モード B**: 直接はなし。全書き込みは child skill 内、各自のスコープ内で発生。
- **モード A（上流フェーズ、Phase 4 のみ）**: ユーザーレビュー用の*ドラフト*入力アーティファクト — `openapi/**`（OpenAPI YAML）、`database/migrations/**`（新規ファイルのみ）+ `database/dml/**`（SQL）、`docs/spec/domain/<domainPkg>.md` + `docs/spec/usecase/<usecasePkg>.md`（`new-spec` 経由 → 記入）。いずれも `CLAUDE.md` の AI 修正スコープ内。上流フェーズは Go ソースを書かない — それはコアの child skill の担当。

## コア（Phase 5+）の前提条件

これらは**コア**の実行前に真である必要がある。モード A では上流フェーズ（Phase 4）の*成果物*であり、モード B ではユーザーが既に満たしている。

| # | 前提 | 検証者 |
| --- | --- | --- |
| 1 | domain spec と usecase spec がそれぞれのツリーに存在（lean A: 2 spec のみ） | verify-spec |
| 2 | spec format 有効 + cross-spec 参照整合 + 命名規約充足 | verify-spec |
| 3 | OpenAPI YAML 書き込み済み + `make gen-api` で `internal/controller/handler/<path>/gen/` 生成済み | scaffold-controller 前提 |
| 4 | `database/dml/...` 配下に SQL 書き込み済み + `make gen-query` で sqlc gen 生成済み | scaffold-infra-db 前提 |
| 5 | DB 起動中 + migrate + seed 済み（DB スキーマが新 SQL と一致） | 手動: `make serve` → `make db-init` |
| 6 | usecase spec が依存する boundary interface が `internal/usecase/boundary/` 配下に存在 | scaffold-usecase 前提 |

コア開始時に前提未充足なら、該当 child skill が surface し本 skill が chain 中断。

> **環境に関する注記（前提 3〜5）:** `make gen-query` は `pg_dump` で稼働中の DB スキーマをダンプするため、**DB が起動している必要がある**（未起動だと `make gen-query` / `make go-test` が `could not translate host name "database"` で失敗する）。環境は**生 `docker compose` ではなく専用 make ターゲット**で起動すること: `make serve`（development プロファイル、`database` サービス含む）→ **`make db-init`**（local/test 両 DB を migrate **かつ seed**。テストは seed 前提のため、`db-*-migrate-up` 単体では不十分）→ その後に `make gen-query` / `make gen-api`。
>
> **ツールチェーンに関する注記（最終 `make go-fix` / `make go-test`）:** `make go-fix` や `make go-lint` が**ツールのバージョン不整合**（例: `golangci-lint` の "you are using a configuration file for golangci-lint v2 with golangci-lint v1"）で失敗した場合は回避策を取らず、`make install-tools` でローカルのツールを `mise.toml` 固定バージョンに揃えてから再実行する（`mise.toml` 自体を変更した場合は先に `make sync-versions`）。`PATH` の手動書き換えやバージョン指定バイナリの直叩きで代替しないこと。

---

## Phase 0. feature 確認 + モード判定

**起動直後に必ず `AskUserQuestion` を呼ぶ**（1 回の batched call）:

- 「対象 feature 名 (kebab-case)」 — フリーテキスト。レポートで使う束ねのラベルであり、パスではない
- 「domain パッケージパス（`internal/domain/` 以下。例: `cart`, `product/category`）」 — フリーテキスト
- 「usecase パッケージパス（`internal/usecase/` 以下。例: `address`, `product/ranking`, `user/search`）」 — フリーテキスト

2 つのパッケージパスは別々の回答であり、互いにも feature 名とも日常的に食い違う: spec のツリーはパッケージパスで引かれ（`docs/spec/<layer>/<rest>.md` ⇔ `internal/<layer>/<rest>`）、Go のパッケージ名にハイフンは無く、usecase パッケージは依存先の集約の下には無い（`internal/usecase/user/search` → `internal/domain/user`）。feature 名の `-` を `/` に置き換えたものを既定値とし、ユーザーに直させる。

続いてエントリモードを判定 — **ツリーごとに 1 回、計 2 回の存在確認**:

- `docs/spec/domain/<domainPkg>.md` と `docs/spec/usecase/<usecasePkg>.md` を確認。**両方**があり**かつ**ユーザーの依頼が「用意済み spec からの scaffold」と読めるなら、**モード B** を選び Phase 5 へ直行。
- それ以外は**モード A** を選び上流フェーズ（Phase 1–4）を回す。どちらかが無いのは通常の idea-first 開始 — エラー扱いしない。

モードが曖昧（spec はあるがユーザーはまだ設計中）なら、推測せずどちらか尋ねる。

---

## 上流の設計フェーズ（モード A のみ）— Phase 1–4

上流フェーズは公式 `feature-dev` ワークフローからの接ぎ木だが、このリポジトリのエージェントに配線し、アーキテクチャに拘束する。新エージェントを導入せず既存の **`Explore`** / **`Plan`** エージェントを流用する。

### Phase 1. Discovery + Clarifying Questions

**目的**: 探索・設計の前に、アイデアを具体的で曖昧さのない要件へ落とす。

1. feature 依頼と解こうとしている問題を再述し、スコープ境界をユーザーと確認。
2. オニオン endpoint に効く未指定点を洗い出す: リソースとその不変条件、操作（と HTTP 形）、エラー/エッジケース、認証（`security:`）要否、永続化形、冪等性/トランザクション要否、ページング、既存エンドポイントとの後方互換。
3. 未解決点を `AskUserQuestion` で提示（グルーピングし、各質問に既定案を推奨）。**回答を待つ** — このフェーズの主旨は下流を何も推測しないこと。「お任せ」と言われたら推奨を記録し明示確認を得る。

出力: Phase 2–4 が土台にする短い要件サマリ（実行コンテキストに保持、コミットファイルにはしない）。

### Phase 2. Codebase Exploration

**目的**: 設計をこのリポジトリの既存流儀に接地させ、並行パターンを発明せず自然に統合させる。

**`Explore`** エージェントを 2〜3 並列（単一メッセージ）で起動、各々別の観点を担当。例:

- 「`<feature>` に最も近い endpoint を特定し controller→usecase→domain→infra の全フローをトレース。重要 5〜10 ファイルを返す。」
- 「`<feature>` に関わる domain + usecase 規約（entity/VO/Repository IF 形、boundary interface、DTO マッピング）を把握。重要ファイルを返す。」
- 「`<feature>` のような endpoint が触る OpenAPI + sqlc + DI 配線（spec 配置、migration/dml 配置、`internal/di/module/*`）を特定。重要ファイルを返す。」

エージェント返却後、surface された**重要ファイルを実際に読み**一次情報を作る。設計前に、見つかったパターンを `file:line` 参照付きで簡潔にまとめて提示。

### Phase 3. Architecture Design

**目的**: この feature を*リポジトリのレール内で*どう作るかを、trade-off を明示して選ぶ。

**`Plan`** エージェント（1〜3 観点。例: 最小変更 / クリーン境界 / 実利バランス）で候補案を出す。**全案を固定アーキテクチャに拘束する** — 設計空間はレールの*内側*であり、外側は不可:

- オニオン層: `controller → usecase → domain`; infrastructure が domain interface を実装; 層迂回なし（depguard で強制）。
- lean A 憲法: spec 駆動は domain 層と usecase 層のみ; controller は OpenAPI gen、infra は sqlc gen から導出。
- HTTP 契約は OpenAPI-first; クエリは sqlc; 新フレームワーク・新アーキパターンなし（`CLAUDE.md` 準拠）。

したがって各案は*リポジトリ内で合法な*軸で差が出る — 計算値の置き場（domain メソッド vs VO）、読みパスの repository vs query-service、同期 write vs outbox、ページング方式、不変条件の強制方法など — フレームワークレベルの選択ではない。各案の trade-off と推奨を提示し、**`AskUserQuestion` でどの案を採るか**尋ねる。

### Phase 4. 入力アーティファクトのドラフト作成

**目的**: 確定した設計に沿って、コアの前提条件をレビュー可能なドラフトとして作り、生成へ引き渡す。

選択した案に沿って:

1. `new-spec` を連鎖して domain + usecase の spec テンプレートを起こし、設計内容を**記入する**（entity/不変条件/behavior/VO/Repository メソッド; usecase interface/DTO/依存/workflow）。`new-spec` は identity レベルのテンプレートしか作らない — 設計内容は Phase 1–3 由来。
2. **OpenAPI YAML** を `openapi/**`（OpenAPI-first）、**SQL** を `database/dml/**` + **新規** migration を `database/migrations/**`（新規ファイルのみ — 既存 migration は編集しない）にドラフト。
3. **ドラフトをユーザーレビューに出す**（`AskUserQuestion`: 「このドラフトで生成に進みますか？」 / 修正指摘 / キャンセル）。これはドラフト — ユーザーが author-of-record であり、生成前に承認が要る。
4. 承認後、`make gen-api` + `make gen-query` を実行（DB 起動必須 — 環境注記参照）し、コア向けに生成 `gen/` + sqlc ファイルを揃える。

生成ファイル（`*.gen.go`, `*.sql.go`, `*_mock.go`, `openapi.gen.yaml`）は**手書き・編集しない**。ユーザーにしか決められない判断でドラフトが完成しない場合は、業務内容を発明せず停止して尋ねる。

Phase 4 完了時に Phase 5+ の前提が成立し、（無改変の）コアへ続く。

---

## コア（両モード）— Phase 5–7

### Phase 5. spec 検証（自動 chain）

`verify-spec` skill を Phase 0 で収集した 2 つのパッケージパス指定で起動。`verify-spec` が `violations > 0` を報告したら chain 中断:

```text
scaffold can not safely proceed: verify-spec で <N> 件の違反が検出されました。
spec を修正してから再度 /scaffold-endpoint を実行してください。
```

warning のみなら継続（warning は block しない）。

### Phase 6. 依存順序で child skill chain

各 child skill を順に起動、その層のパッケージパスを context として渡し各 child がそこから spec パスを解決できるように（feature 名は spec パスを決めない）:

1. **`scaffold-domain`** — entity + Repository IF + VOs + constants + errors + tests（+ mock 用 `make gen-api`）。
2. **`scaffold-infra-db`** — sqlc gen ラップの Repository 実装（事前 `make gen-query` 実行済みを内部検証）。
3. **`scaffold-usecase`** — Application Service + DTOs + tests（+ Usecase mock 用 `make gen-api`）。
4. **`scaffold-controller`** — `ServerInterface` 実装 handler + tests。

child skill 間で成否ステータスを伝播:

- **child 成功** → 次へ。
- **child 失敗** → chain 停止、child の FB summary を surface、進めない。

各 child skill は独立に:

- 自身の plan 確認 `AskUserQuestion` を実施（layer ごとに user 判断を確保）
- 自身の test 観点 subagent を起動
- 必要なら `make gen-api` 実行
- 自身のファイルを書き込み
- 書き込み後 `make go-fix` + `make go-test` 実行
- 失敗時 TODO + FB を surface

layer ごとにユーザー確認を挟むので、判断を要する箇所で human-in-the-loop が保たれる。

### Phase 7. 統合検証（make go-test + ランタイム curl + o11y）

全 4 child skill 成功後、統合最終検査:

```sh
make go-fix
make go-test
```

cross-layer 統合（handler → usecase → domain → infra）が全体としてコンパイル / テスト通るか確認。本 scaffold が触った 4 パッケージのカバレッジ行を surface。ここで `make go-test` 失敗時（child が自身でテスト済みなのでまれ）は TODO + FB で surface して停止。

続いて**ランタイム動作確認（curl + o11y）**を実施。`make go-test` は **usecase / repository をモック**するため、実際の Fx グラフ・HTTP ミドルウェア（認証 / OpenAPI バリデーション）・DB を通らない。実機でしか出ないバグ群がある: `security:` 宣言漏れ（認証なしで到達）、`BindHandler` の未登録 / 配線ミス、DI provider 不整合、実 DB での SQL フィルタ挙動差など。**curl はここでやるのが正しい** — 全層 + DI がここで初めて揃う。per-layer スキルでは不可（下位層 / DI が無く起動すらしない）。

前提:

- `make serve` 稼働中で `api_server` ログが `[Fx] RUNNING` 到達（`scaffold-controller` の DI 起動確認）。
- `make db-init` 実行済み（local + test を seed する正準セットアップ）。

手順:

1. **既知状態の対象を用意する。** 既存行が要るなら seed 済み id を使うか、作成エンドポイントで先に作る。資格情報 / 状態依存の確認（例: パスワード変更）は、平文 / 状態を自分が把握する行を作る（ローカル検証なら既知 bcrypt ハッシュの `psql` insert でも可）。
2. **新エンドポイントを curl**（ローカル認証は `Authorization: Bearer debug:<subject>`）し、以下を確認:
   - ルート到達 — ルータ 404 でない応答 = ハンドラ登録 / DI 配線 OK の証拠；
   - 正常系が期待どおりの status / body；
   - 主要異常系: NotFound (404)、バリデーション (400/422)、**`security:` 宣言があるなら** no-token ⇒ 401（保護が効いているか）；
   - 変更が**実際に反映**されたか（新状態で再取得 / 再認証）。2xx だけで満足しない。
3. **o11y ログを 1 回確認**: 1 リクエスト分のトレースが全層（controller → usecase → infra）を貫き、発行 SQL が想定どおりかを見る。この 1 回の確認後は、再確認を curl ではなく o11y に頼れる。

共有スキーマの波及: 変更が**共有** OpenAPI コンポーネント（複数オペレーションから参照される `components/schemas/*` や `components/requests/*`）に及ぶ場合、ランタイム確認は**新規/変更したエンドポイントだけでなく、それを参照する全エンドポイント**を対象にする。編集したファイルへの `$ref` を spec から grep し、各 consumer を curl する。共有スキーマ編集は、モックテストが通らない兄弟エンドポイントを静かに壊しうる（例: `allOf` で使う基底からプロパティ削除、`additionalProperties: false` が兄弟のプロパティを拒否 → 新エンドポイントは正常に見えるのに `POST` 作成が壊れる）。

破壊的ガード: curl がデータを変更し、元に戻す手段が `make db-init`（等）しかない場合は、**実行前にユーザー確認**（`CLAUDE.md` 準拠）。検証用に作成した行は後始末する。

いずれか失敗時は TODO + FB を surface して停止（コミットしない）。

---

## レビュー・クロージング（両モード）— Phase 8–9

### Phase 8. 品質レビュー（repo のレビュースキル再利用）

scaffold + ランタイム確認は feature が*構築され起動する*ことを示す。このフェーズは*良いか*を判定する — 汎用 reviewer ではなくリポジトリ自身のレビュースキルを使う。これらはこのコードベースのルールを内包し、実装者とは別モデルで reviewer を走らせるため:

- **`impl-review`** — 新変更に対する敵対的な correctness / security / architecture / runtime-gap + コメント品質。
- **`arch-check`** — 触れた層の layer-compliance 監査（depguard レベルの境界、lean A 規約）。
- **`test-review`** — 生成テストの品質（構造準拠 + 観点カバレッジ + 意味的強度）。

この feature の変更にスコープして実行。指摘を集約し、最重要を surface し、**`AskUserQuestion`**: 「指摘を今修正 / 後で / このまま進む」。ユーザーの選択に沿って対応。本フェーズは read-only 検出であり、修正適用は別途明示確認するステップ（これら reviewer は自動編集しない）。

### Phase 9. クロージング

日本語サマリ:

```text
scaffold-endpoint 完了（feature: <feature>, mode: <A/B>）。

  [Mode A のみ] ✓ 要件整理 / 探索 / 設計案選択 / 入力ドラフト作成 + gen
  ✓ verify-spec: violations 0
  ✓ scaffold-domain: <N> ファイル作成、coverage 100%
  ✓ scaffold-infra-db: <N> ファイル作成、coverage <X>%
  ✓ scaffold-usecase: <N> ファイル作成、coverage 100%
  ✓ scaffold-controller: <N> ファイル作成、coverage 100%
  ✓ make go-test: 全体 OK
  ✓ ランタイム動作確認: curl 到達 / 認証 / 主要異常系 / o11y トレース OK
  ✓ 品質レビュー: impl-review / arch-check / test-review 実施（指摘 <n> 件、対応方針: <...>）

次のアクション:
  - /commit で 変更をコミット
  - /submit-pr で PR 作成
```

いずれかのフェーズ失敗時は失敗ステータス表 + 失敗フェーズ/child の FB を出力、user が fix-forward を判断。

commit しない。push しない。

## AI 修正スコープ

- **モード B**: 本 skill 自体はファイルを書かない。全スコープは child skill に委譲（各 SKILL.md の constraint 参照）。
- **モード A**: 加えて上流フェーズが Phase 4 で入力アーティファクトをドラフトする — `openapi/**`、`database/dml/**`、`database/migrations/**`（新規ファイルのみ）、`docs/spec/domain/**` / `docs/spec/usecase/**`（いずれも `CLAUDE.md` の AI 修正スコープ内）に限る。Go ソースは書かず、生成ファイルにも触れない。

## 制約事項

- ❌ モード B の入力が既に存在するのに上流フェーズ（Phase 1–4）を強制 — 検出してスキップ。
- ❌ spec がまだ無いのに `verify-spec` へ直行 — 上流フェーズが先に生成すべき（モード A）。
- ❌ アーキテクチャ案（Phase 3）を onion / lean A / OpenAPI-first / sqlc のレール外へ出す — 設計空間はレール内。
- ❌ Phase 4 で業務内容を発明 — 確定設計からドラフトし、本当の欠落は停止して尋ねる。
- ❌ 生成ファイル（`*.gen.go`, `*.sql.go`, `*_mock.go`, `openapi.gen.yaml`）や既存 migration を編集。
- ❌ Go ソースを直接変更（child skill に委譲）。
- ❌ `verify-spec`（Phase 5）をスキップ — spec 整合性の safety net。
- ❌ 失敗した child skill を素通り — 停止して FB surface。
- ❌ 後段フェーズ/child 失敗時に成功済み earlier の書き込みを自動 rollback — user 判断。
- ❌ feature 確認 `AskUserQuestion`（Phase 0）をスキップ。
- ✅ ユーザー向け出力は日本語。
- ✅ 上流フェーズは既存の `Explore` / `Plan` エージェントを流用（新エージェント型なし）。
- ✅ 依存順序（domain → infra-db → usecase → controller）で child 起動。
- ✅ 実行した全フェーズ + 最終 `make go-test` を統合した最終レポートを surface。
- ✅ 各 child skill が自身の確認を layer ごとに取る（judgment-heavy step で human-in-the-loop）。
- ✅ ランタイム curl + o11y 確認（Phase 7）を実施 — `make go-test` だけでは DI / ミドルウェア / DB を通らない。
- ✅ 品質レビュー（Phase 8）は汎用 reviewer でなく `impl-review` / `arch-check` / `test-review` を再利用。
- ✅ 元に戻す手段が `make db-init` しかない破壊的 curl は実行前にユーザー確認。

## チェックリスト

- [ ] feature 名を `AskUserQuestion` で確認; エントリモード（A/B）判定（Phase 0）
- [ ] **モード A**: 要件明確化（Phase 1）→ `Explore` で探索（Phase 2）→ レール内で `Plan` により案選択（Phase 3）→ 入力アーティファクトをドラフト・ユーザー承認・`make gen-api`/`gen-query` 実行（Phase 4）
- [ ] `verify-spec` 実行、違反時は chain 中断（Phase 5）
- [ ] `scaffold-domain` / `-infra-db` / `-usecase` / `-controller` 各々成功実行（または失敗時 chain 停止）（Phase 6）
- [ ] 全 child 成功後に最終 `make go-fix` + `make go-test`; ランタイム curl / 認証 / 主要異常系 / o11y トレース確認（Phase 7）
- [ ] 品質レビュー実行（`impl-review` + `arch-check` + `test-review`）; 指摘 surface と対応判断（Phase 8）
- [ ] layer ごとのファイル数 + カバレッジを含む統合日本語サマリ（Phase 9）
- [ ] commit / push なし
- [ ] いずれの失敗時も書き込み済みファイルを自動 rollback していない
