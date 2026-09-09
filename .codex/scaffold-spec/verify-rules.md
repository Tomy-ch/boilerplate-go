# Verify-Spec Rules

`verify-spec` 系 skill が実施する検証の粒度と cross-layer / 派生元参照チェック方針。lean A 構成（domain spec + usecase spec の 2 層）前提。

spec のパスは `docs/spec/domain/<pkgpath>.md` / `docs/spec/usecase/<pkgpath>.md`（`<pkgpath>` は `internal/domain/` / `internal/usecase/` 以下の Go パッケージパス）。パス規約の正本は [`lifecycle.md`](lifecycle.md)。

## 検証スコープ

lean A では controller / infra に spec を持たないため、`verify-spec` の責務は:

1. **format 検査** — 必須節の有無、YAML パース可否
2. **派生元との整合性** — spec が SQL / OpenAPI / sqlc gen など外部入力と矛盾しないか
3. **cross-spec 参照整合性** — usecase spec の `calls:` / 展開形節の `dependencies:` が参照先の domain spec の Repository / Behavior / Domain Service、または Dependencies の boundary に存在するか、および `## Interface` の全メソッドに手順があるか

## 層ごとの責務

| skill | 担当 spec | 検査内容 |
| --- | --- | --- |
| `spec-validator-domain` | `docs/spec/domain/<pkgpath>.md` | format（節 / YAML パース）+ パス ↔ `package:` 宣言の一致 + Entity フィールド ↔ `database/migrations/` の `CREATE TABLE` カラム突き合わせ + 内部整合性（Behavior / Domain Service / VO / Repository methods） |
| `spec-validator-usecase` | `docs/spec/usecase/<pkgpath>.md` | format + パス ↔ `package:` 宣言の一致 + cross-spec 参照（依存先 domain spec の Repository / Behavior / Domain Service / boundary） + Interface ↔ 手順の網羅 + 命名規約（動詞接頭辞、Usecase interface 命名 — `internal/usecase/README.md` + sibling pkg 由来）+ Workflow 内部整合性（tx_required + boundary calls） |
| `verify-spec` | 統合 | 存在する spec を検出して上記を chain |

**依存方向に関する注意**: `spec-validator-usecase` は OpenAPI operationId カバレッジを **検査しない**。これは依存方向逆転（usecase は HTTP/OpenAPI を知らない、知るべきでない）になるため。OpenAPI ↔ usecase mapping の検証は **`scaffold-controller` の責務**（scaffold 時に「operationId に対応する usecase method があるか」を確認、無ければ scaffold 失敗で hand-off）。実装後の handler ↔ operationId 一致は `arch-check`（controller 監査）が担当。

## 参照先 domain spec の解決

usecase spec が突き合わせる domain spec は、**その usecase spec の `## Dependencies` が宣言する
Repository インターフェースのパッケージパスから解決する**。`internal/domain/<X>` に依存しているなら
`docs/spec/domain/<X>.md` を読む。

```text
Dependencies:  - user_repository   # domain/user.Repository   → docs/spec/domain/user.md
               - cart.Repository   # internal/domain/cart      → docs/spec/domain/cart.md
```

依存名の書き方は spec によって `<name>_repository` 形式とパッケージ名形式の両方があり、パッケージパスは
行のコメントか名前そのものから読む。**usecase spec 自身の位置からは導出しない** — usecase のパッケージ
パスと依存先集約のパッケージパスは一致しない（`internal/usecase/user/search` は `internal/domain/user`
を参照する）。

対応する domain spec が存在しない依存 — boundary IF、QueryService、domain spec 未整備のパッケージ —
は解決不能ではなく **「domain spec なし」として扱い、その依存に対する cross-spec 検証はスキップする**。
Repository 依存が 1 つも domain spec に解決しない usecase spec は、集約を持たない投影・検索であり、
それ自体は違反ではない。

## cross-spec 参照ルール（lean A）

| 参照 | チェック方法 |
| --- | --- |
| usecase spec `calls: <aggregate>.Repository.<Method>` | 解決した domain spec の Repository Methods 一覧に存在 |
| usecase spec `calls: <aggregate>.<BehaviorMethod>` / `<aggregate>.New` | 解決した domain spec の Behavior Methods / Entity factory に存在 |
| usecase spec `calls: <boundary>.<Method>` | usecase spec の Dependencies に boundary 名が存在（Method 自体は Go compiler 任せ） |
| 展開形節の `dependencies: domain/service/<name>` | 解決した domain spec の Domain Service に対応する定義が存在 |
| 展開形節の `dependencies:` のその他の項目 | 依存名として解釈できる（`<aggregate>.Repository` / boundary / QueryService / `pkg` パス）。**メソッド粒度では解決しない** |
| usecase spec `## Interface` の各メソッド | 集約形 Workflow entry か展開形 `##` 節のいずれかに手順がある |

**展開形の `dependencies:` は節スコープで完結する。** spec 全体の `## Dependencies` は集約形 Workflow の
ためのものなので、展開形節の依存がそこに載っていなくても違反ではない。逆に、展開形は `calls:` を
持たないため cross-spec の解決は依存名までで止まる — メソッド名の突き合わせは効かない。

**網羅チェックの向きで扱いが違う。** Interface に載っているのに手順が無いのは `violation`。手順があるのに
Interface に無いのは `suggestion` — その節が同じ feature の**別パッケージ**の usecase（読み取り専用の
集計など、`internal/usecase/<pkgpath>/<name>/` に分けたもの）を記述していることがあり、その場合は
そちらが自分の Interface を持つため、ここに載らないのが正しい。

## パス ↔ `package:` 宣言の不変条件

`docs/spec/<layer>/<rest>.md` は `internal/<layer>/<rest>` に 1 対 1 で対応し、spec 冒頭の YAML
（domain は Entity の `package:`、usecase は Interface の `package:`）と必ず一致する。これは機械的に
検査できる不変条件なので、両 validator が format 検査の一部として assert する。不一致は `violation` —
どちらが正かは spec の内容を読まないと決まらないため、自動では直さず両方を示して hand-off する。

## 派生元との整合性ルール

| 派生元 | 検査 | 担当 skill |
| --- | --- | --- |
| `database/migrations/<latest>.sql` の `CREATE TABLE` | domain spec の Entity フィールド名（snake_case → camelCase）+ Go 型マッピングが対応 | spec-validator-domain |
| `internal/usecase/README.md` + sibling pkg | usecase spec の Interface 命名規約（動詞接頭辞、Usecase interface 命名）に準拠 | spec-validator-usecase |
| OpenAPI gen (`internal/controller/handler/<path>/gen/server.gen.go`) | `ServerInterface` operationId ↔ usecase method の mapping 確立 | **scaffold-controller**（verify-spec ではなく、scaffold 時に検査。依存方向に整合） |
| sqlc gen (`internal/infrastructure/rdb/sqlc/gen/*.gen.go`) | Repository method ↔ sqlc 関数の対応 | **scaffold-infra-db**（compile 時 catch も併用） |

これらは scaffold-X skill が input として直接読むため、spec / 既存 doc が drift していると scaffold が崩れる。spec 内部の整合は verify-spec が、scaffold 入力との整合は scaffold-X 自身が scaffold 時に検査する役割分担。

## 検出粒度

format + cross-layer 参照整合性まで。sqlc gen 関数が実在するかは検査しない — compile 時に catch されるため。

## scaffold-X skill 内では pre-check しない

scaffold-X 内では入力の有効性を仮定する。事前検証は `verify-spec` の責務、事後検証は `make go-test` の責務。`scaffold-endpoint` から起動された場合は `verify-spec` が違反検出時に下流 chain を中断する。

## 参照する spec format

各 layer の必須節 / YAML schema は以下を参照（実行時に読み込む）:

- `domain-spec.md`
- `usecase-spec.md`
