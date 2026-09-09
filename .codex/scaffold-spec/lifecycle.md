# Scaffold Lifecycle

オニオンアーキ準拠 endpoint scaffold の全体像 / 実行順序 / 各 skill workflow / 失敗時挙動を記述する。spec format 自体は `<layer>-spec.md` を参照。

## 前提

- 本リポジトリは **Onion Architecture**（Clean Architecture ではない）
- Application Service (usecase) は薄いオーケストレーターで、business rule は domain entity に押し込む
- spec は **lean** 構成: 本質的な設計判断（domain invariants / behavior、usecase Workflow）だけを spec 化し、controller / infra 層は OpenAPI / sqlc gen / 命名規約から導出
- OpenAPI YAML / SQL ファイル / migration / 2 layer spec は人間が事前に用意する前提

## なぜ 2 spec か

| layer | spec の必要性 | 理由 |
| --- | --- | --- |
| domain | ◎ domain spec 必須 | invariants / behavior methods / VO は SQL から導出不可、設計議論の本体 |
| usecase | ◎ usecase spec 必須 | Workflow（処理の中身、tx 境界、orchestration）は spec でしか表現できない |
| controller | × spec 不要 | handler は OpenAPI operationId + 命名規約から導出可能（pure template） |
| infra | × spec 不要 | Repository 実装は domain IF + sqlc gen 関数名マッピングから導出可能（pure template） |

controller / infra の "規約通り" は `arch-check`（controller / infra 監査）で強制する（命名規約、handler ボディの純粋性、Repository ボディの純粋性）。規約違反が起きた瞬間 scaffold が崩れるため、arch-check が安全網となる。

## スキル一覧

| skill | 責務 |
| --- | --- |
| `new-spec-{domain,usecase}` | layer 別 spec をユーザーへの明示的な質問でインタラクティブに作成 |
| `new-spec` | 上記 2 つを multi-select で chain する統合 spec 作成 skill |
| `spec-validator-{domain,usecase}`（`verify-spec` が fan-out する subagent） | 各 spec ファイルを format / 派生元（SQL / OpenAPI / sqlc gen） / cross-layer 参照整合性で検証 |
| `verify-spec` | 存在する spec を検出して上記を chain |
| `arch-auditor-{domain,usecase,controller,infra,pkg}`（`arch-check` が fan-out する subagent） | layer ごとに depguard + semantic check + 規約強制 |
| `arch-check` | 変更ファイル or 全 layer で上記を chain |
| `scaffold-domain` | entity + Repository IF + constant + error の全部を生成 |
| `scaffold-usecase` | application service + boundary + DTO を生成 |
| `scaffold-controller` | OpenAPI 生成 interface + usecase IF から handler 実装を導出（spec なし） |
| `scaffold-infra-db` | domain Repository IF + sqlc gen 関数から Repository 実装を導出（spec なし） |
| `scaffold-endpoint` | 起動時に自動で `verify-spec` を走らせ、scaffold-* 4 つを順番に呼ぶ統合 skill |
| `drift-detector-{domain,usecase,controller,infra,pkg}`（`back-prop` が fan-out する subagent） | layer ごとに README ↔ コード ↔ skill の 3 方向 drift を検出（A: README→Code、B: 未文書化 pattern、C: Skill↔README 重複）。検出のみで read-only、README / skill の編集は統合 skill `back-prop` が user 承認 + 理由明示後に行う |
| `back-prop` | 変更ファイル or 全 layer で上記を chain |

## 統合 skill の実行順序

```text
1. (前提: SQL / migration / OpenAPI YAML / domain spec / usecase spec は人間が用意済み)
2. verify-spec              (format / 派生元 / cross-layer 整合性チェック、失敗時は中断)
3. scaffold-domain          (entity + Repository IF + constant + error)
4. (前提: make gen-query が済んで sqlc 生成物がある)
5. scaffold-infra-db        (domain Repository IF + sqlc gen から Repository 実装を導出)
6. scaffold-usecase         (domain entity + repository IF + usecase spec の Workflow から実装)
7. scaffold-controller      (OpenAPI gen + usecase IF から handler を導出)
8. arch-check               (全 layer の規約適合性チェック)
9. make go-test                (build / test の cross-layer 整合性確認)
```

`back-prop` は scaffold-endpoint の chain には含まれない。実装フローの **後段** に位置する独立 hygiene skill で、推奨 trigger は次の通り:

- 該当 layer のコードを触った直後 / commit 前（`back-prop` を起動し scope 質問で対象 layer だけを選ぶ）
- 大規模 PR review 前 / multi-layer refactor 後（`back-prop` 統合）
- 定期 hygiene sweep（README / skill が実態と sync しているか確認）

優先度は **README > Code > SKILL**。README が canonical、コードは drift する可能性があるため、検出された drift は per-item で user 判断（コード修正 / README 緩和 / skill 簡略化 / 無視）。AI は自動解決せず surface + draft 提示までで止まる。

## spec パス規約

```text
docs/spec/
  ├── glossary.md                 # 業務語彙。層のツリーには入らない
  ├── domain/<pkgpath>.md         # 例: cart.md / product/category.md
  └── usecase/<pkgpath>.md        # 例: address.md / product/ranking.md / user/search.md
```

- `<pkgpath>` は `internal/domain/` / `internal/usecase/` 以下の **Go パッケージパス**
- **不変条件**: `docs/spec/<layer>/<rest>.md` ⇔ `internal/<layer>/<rest>`。spec 冒頭の `package:` 宣言と必ず一致する
- spec の列挙は `docs/spec/domain/**/*.md` / `docs/spec/usecase/**/*.md`。`glossary.md` は `docs/spec/` 直下なのでどちらにも入らない
- 統合 skill (`scaffold-endpoint`) には `<pkgpath>` を渡す。各 layer の spec は規約上のパスから探す
- 1 つの業務機能（feature）は domain / usecase の 2 ツリーに 1 本ずつ spec を持つのが基本形。ただし 1:1 とは限らない — 集約を持たない usecase（投影・検索）は domain 側に対応する spec を持たず、参照先の domain spec は usecase spec の `Dependencies` から解決する
- 単独 skill にはファイルパスを直接渡す（standalone 実行可）

## spec format

**D 案: Markdown + YAML コードブロック** で統一。narrative は Markdown、構造化要素（フィールド一覧 / メソッドシグネチャ等）は YAML ブロックで記述。

層ごとの節構成 / 自動派生ルール / テンプレ例は以下を参照:

- `domain-spec.md`
- `usecase-spec.md`

## spec ファイルのライフサイクル

**A. 残す（commit する）** を採用:

- `docs/spec/domain/**` / `docs/spec/usecase/**` は git commit してリポジトリに残す
- PR レビュー時に spec も一緒にレビュー可能
- spec ↔ コードの drift は人間の PR レビューで catch（自動 drift 検出は当面導入しない）
- spec は scaffold の input であると同時に **設計議論の永続的なアーティファクト**

## 各 scaffold-X skill の workflow

すべての scaffold-X skill は以下の手順で動く:

1. **入力読み込み**
    - `scaffold-domain` / `scaffold-usecase`: `docs/spec/<layer>/<pkgpath>.md`
    - `scaffold-controller`: OpenAPI gen (`internal/controller/handler/<path>/gen/server.gen.go`) + usecase IF
    - `scaffold-infra-db`: domain Repository IF + sqlc gen (`internal/infrastructure/rdb/sqlc/gen/*.gen.go`)
2. **README 参照** — 該当 layer README + 1〜2 個の近接 README（handler / boundary / repository 等）。既存規約に揃えるための template として利用
3. **test 観点 subagent を起動** — README の Test Strategy 節を観点定義として渡し、test 観点を立てさせる（詳細は `test-perspectives.md`）
4. **実装 + tests + 自層 DI 登録** — subagent の出力を input にして、既存類似コードを構造 template に実装
5. **失敗時** — 該当箇所に TODO / コメントとして問題点を記述し、最終 summary で user に FB。自動 rollback はしない

## Standalone 実行

各 skill は単独でも動く:

- 例 1: domain は手書き完成済み、usecase だけ scaffold したい → `scaffold-usecase` 単独起動
- 例 2: 全部一気に → `scaffold-endpoint` 経由（verify-spec も自動 chain）
- 単独実行時は spec ファイルパス or 派生元入力を直接指定。統合 skill 経由ならパッケージパスから自動解決

## 失敗時の挙動

- 該当箇所に TODO コメントを書き込む（実装途中で止まっても、user が読める形で残る）
- 最終 summary で user に問題点を FB し、続きの実装を促す
- 自動 rollback はしない（部分適用を許容、user 判断で fix-forward）

## scaffold-domain のスコープ

domain layer の構成要素を **全部** 担う:

- entity（Aggregate Root）
- value object
- Repository interface
- constant（命名規約・上限値等）
- error（domain 固有エラー）

spec に書かれていない要素は TODO 化しない（spec が source of truth）。

## 規約逸脱への対処（lean A の trade-off）

controller / infra に spec を持たない代わりに、以下の規約を arch-check で強制:

- `operationId` ↔ handler メソッド名一致（camelCase） — **strict**
- handler ボディは pure template（Bind → usecase 呼び出し → response 変換） — **strict（業務ロジック検出は violation、ambiguous は suggestion）**
- Repository method が 1+ の sqlc gen 関数を呼ぶ — **soft（不一致は suggestion）**
- Repository ボディは data orchestration のみ（sqlc → pgerror → 行→entity 変換、業務ロジック厳禁） — **strict（業務ロジック検出は violation）**
- entity フィールド ↔ SQL カラム対応（snake↔camel） — **soft（不一致は suggestion、VO ラップ / 計算メソッドは自動推論で対象外）**

「1:1 strict」ではなく **「pure / 業務ロジック禁止 strict + 構造 soft」** という二段階。理由: 1:1 は ideal model で、計算フィールド / VO 群化 / multi-query JOIN / N+1 解決 / switch dispatch 等の正当な逸脱がある。それらを violation 扱いすると正当な実装を阻害する一方で、「業務ロジックは domain / usecase の責務」の境界違反は厳密に守る。

### 実装コードに arch-check 専用 annotation を導入しない

**思想**: 実装コードはあくまで実装そのものが主体。AI / lint / scaffold ツールは補助でしかない。AI ツール用 annotation を実装コードに混入させると、コードベースが特定の AI tooling setup を前提とする構造になり、AI への依存が強まる。補助ツールに codebase が引きずられる関係性を避けたい。

**実装方針**: arch-check は実装コードを読んで **body 構造から自動推論** する。

- 多重 sqlc 呼び出し / switch dispatch / JOIN → body 内の `sqlc.*` 呼び出し回数 + switch 構文で検出
- 計算値 → メソッド形式（func receiver）で書けば struct field 検査の対象外、annotation 不要
- VO ラップ → field の型を VO にすれば arch-check が型解決して包含カラムを「対応済み」扱い
- pure-template チェックは一律適用、不確実なケースは `suggestion` 止まりで user 判断に委ねる

**注**: 実装の WHY を説明する人間向けコメントは OK（むしろ可読性に寄与）。NG なのは AI / 補助ツールに読ませるための機械可読タグ（独自 prefix の annotation 系）。

将来 escape hatch が真に必要なら、Go 標準慣習（`//nolint:<linter>` 等）に揃える検討余地は残すが、独自 prefix の annotation は導入しない。

### TODO hand-off モデル（suggestion レベル逸脱の取り扱い）

**思想**: スキルが解決できない問題は **人間が解決する**。AI は逸脱検出時に「これは意図的だから OK」と勝手に判断 / documentation 化せず、`// TODO:` で人間に hand-off する。AI は検出 + hand-off のみ、判定 / 解決は人間。

**動作**:

1. arch-check が suggestion レベルの逸脱を検出
2. user が「TODO 追加」を opt-in した場合（既定 ON）、逸脱位置に `// TODO:` コメントを挿入
3. コメント内容: **何が検出されたか + 人間向け解決選択肢**（修正案 / WHY 記述案 / 削除案など）
4. 標準 `// TODO:` 接頭辞のみ使用（`// TODO(arch-check):` 等の AI 識別子は付けない）
5. 既存コメントブロックが逸脱位置の直上 3 行以内にあれば skip（重複防止）
6. 人間が解決:
   - コード修正で逸脱解消 → TODO 削除
   - 意図的逸脱なら TODO を WHY 説明コメントに置き換え
   - 放置なら次回 arch-check で再度 surface（コメントは silent signal にならない）

**violation との区別**:

- `violation`（業務ロジック検出 / 禁止 import / pgerror skip 等）→ 修正一択、TODO で defer しない
- `suggestion`（multi-query / 計算フィールド / handler bloat 等）→ TODO で人間判断に委ねる

**対象 layer**:

| layer | TODO 書き込み |
| --- | --- |
| arch-check（domain） | ✓ entity ↔ SQL の suggestion |
| arch-check（controller） | ✓ pure-template の suggestion |
| arch-check（infra） | ✓ Repository ↔ sqlc gen / body composition の suggestion |
| arch-check（usecase） | ✗ violation 中心（業務ロジック検出は修正一択） |
| arch-check（pkg） | ✗ violation 中心（`internal/` 依存 / framework 依存は修正一択） |

**コメント例**:

```go
// TODO: User 構造体に phoneNumber フィールドあり、SQL カラム未定義。
// 永続化が必要なら migration 追加、計算値ならメソッド形式に書き換え、
// in-memory 保持なら本コメントを WHY 説明に置き換えてください。
phoneNumber string
```

```go
// TODO: handler が複数 usecase（ListUsers, CountUsers）を呼び出している。
// orchestration を usecase 側に集約するか、本コメントを WHY 説明に置き換えてください。
func (h *Handler) GetUsers(...) {
```

### 既存ケース別の対処

| 既存ケース | 対処 |
| --- | --- |
| handler が複数 usecase 呼び出し | usecase 側で orchestration メソッドを作って 1:1 維持を推奨。arch-check は suggestion 止まりで user 判断 |
| Repository が複数 sqlc 関数を switch | パラメータ駆動の Repository method として 1:1 維持 OK（既存 `FindByActive` パターン、arch-check が body 構造から自動許容） |
| Repository が JOIN / N+1 解決で複数 sqlc 呼び出し | body は data orchestration を維持。arch-check が body 構造から自動許容 |
| entity に計算フィールド（例: `FullName`） | **メソッド形式**（field ではなく関数）にする — 既存規約 |
| entity が複数 SQL カラムを VO でラップ | VO 型をフィールドにする（arch-check は VO ラップを自動認識） |
| 本当に特殊（複雑な error mapping 等） | コード本体で実装し、必要なら人間向け WHY コメントで意図記述。arch-check は suggestion で surface、user 判断 |
