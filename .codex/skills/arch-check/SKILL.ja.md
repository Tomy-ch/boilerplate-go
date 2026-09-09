> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# アーキテクチャ適合チェック

このスキルはアーキテクチャに焦点を当てたレビューのためのものであり、整形・一般的なコードレビュー・仕様検証のためのものではない。

## 契約

| | |
| --- | --- |
| **所管** | 5 つの層 auditor によるコードとリポジトリ自身の規約の照合、および差分を持たないドメインモデリング監査 |
| **しないこと** | 一般的な好みを違反として扱うこと、または実装を修正すること。任意の `// TODO:` 引き渡しだけが書き込みとなる |
| **開始条件** | 監査対象の差分または既存の構造が存在するとき |
| **停止条件** | 規約の出所を特定できないとき。README 不在は `back-prop` の領分である |

## auditor の構成

5 つの読み取り専用 auditor が、それぞれの層へリポジトリの明文化された規則を適用する。

| パス | 層 | エージェント |
| --- | --- | --- |
| `internal/domain/` | domain | `.codex/agents/arch-auditor-domain.toml` |
| `internal/usecase/` | usecase | `.codex/agents/arch-auditor-usecase.toml` |
| `internal/controller/` | controller | `.codex/agents/arch-auditor-controller.toml` |
| `internal/infrastructure/` | infrastructure | `.codex/agents/arch-auditor-infra.toml` |
| `pkg/` | 共有パッケージ | `.codex/agents/arch-auditor-pkg.toml` |

さらに 2 つの auditor は、層ではなく対象となる論点に紐づく。

| エージェント | 起動条件 | 検査内容 |
| --- | --- | --- |
| `.codex/agents/ddd-origin-auditor.toml` | domain コードまたは ADR / README 群が変更されたとき | リポジトリが文書化した DDD 解釈と Evans 原義との差異、および逸脱宣言の有無 |
| `.codex/agents/ddd-modeling-reviewer.toml` | `internal/domain/**` が対象で、スコープがリポジトリ全体または 1 つ以上の指定 layer のとき | 集約境界とトランザクション境界、規則の所属（entity / 値オブジェクト / Domain Service / usecase）、集約間参照の規律、`docs/spec/glossary.md` に対するユビキタス言語 |

`ddd-origin-auditor` の判定は、他の 5 つの層監査とは性質が異なる。層 auditor はコードをこの
リポジトリ自身の規則と照合するが、これは文書を外部の物差しである Evans と比較する。出力は violation
ではなく 3 値のフラグであり、裁定を含まない。深い監査には `ddd-audit` スキルを使い、このスキルでは
変更に関係する `quick` スコープだけを使う。

`ddd-modeling-reviewer` のスコープ制限は意図的である。差分レビューでは `impl-review` が tier 1 として
このレンズを既に所有しているため、同じ変更に両方を実行すると、出所を区別できない同一 finding が二重に
報告される。一方、`impl-review` の 3 つのスコープはいずれも差分を必要とするため、変更が存在しない状態で
「現在の集約境界は妥当か」という問いには答えられない。このスキルがその入口となり、変更ファイルスコープ
では黙って `impl-review` に譲る。

このレンズは 5 つの層 auditor とも重ならない。`arch-auditor-domain` が検査するのは、禁止 import、
`time.Now()` の直接呼び出し、advisory な entity と SQL の対応といった機械的規則であり、境界そのものが
正しい場所に引かれているかを判断する auditor は他にいない。

## スコープと書き込み権限

既定の対象は変更された production の Go ファイルで、生成ファイル・モック・テストは除く。プルリクエストが既にあるなら、その `baseRefName` が権威である。無ければ base を `make base-branch`（`origin` の生の状態から得た最新の `release/*`）で解決する。`gh repo view --json defaultBranchRef` は決して使わない。base を解決できない場合は止め、その失敗を日本語で報告する。変更された production ファイルが無かった場合とは区別して報告すること。

リポジトリ全体を対象にするのは、そう要求されたときだけである。1 つ以上の層を名指しした要求は、監査をその層に限定する。

**このスキルは既定で読み取り専用である。** 元のスキルがその選択肢を提示していたというだけの理由で `TODO` コメントを足さないこと。足すのはユーザーの明示的な承認を得たあとだけであり、修正すべき violation に対して足すことは決してない。

## 手順

1. `AGENTS.md` を読み、対象のファイルと層を解決する。列挙された層の外にある変更 Go ファイルは別途報告する。監査したふりをしないこと。
2. `make go-lint` を 1 回だけ実行する。その findings を適切な層へ割り当てる。無関係なエラーで基準そのものが壊れている場合は、失敗を報告し、意味論的なレビューへは進まない。
3. 対象の各層について、上の auditor 表が名指すエージェント定義・層の README・該当する最も近いパッケージ README を読む。エージェント定義が与えるのは監査の役割と出力の契約であり、リポジトリの規則についての source of truth は `AGENTS.md` と README のままである。
4. 層ごとの観点をレビューする。

   - **domain:** 純粋性。依存は内向きのみ。フレームワーク・永続化・I/O・時刻 / 乱数 / ID 生成の直接呼び出し・不適切な context の使用が無いこと。エンティティとテーブルの対応は advisory な検査として扱い、値オブジェクトによる表現やメソッド形式の計算値を許容する。
   - **usecase:** オーケストレーションとトランザクションの所有。依存は domain のインターフェースのみ。時刻・乱数・外部 I/O は boundary の抽象を経由すること。domain に属する業務不変条件を持たないこと。
   - **controller:** OpenAPI の operation とハンドラの対応。リクエスト / レスポンスの適合のみ。インフラの import や業務のオーケストレーションが無いこと。ハンドラの大きさや自明でない分岐は、明文化された規則に反しない限り suggestion として扱う。
   - **infrastructure:** domain インターフェースの実装。データのオーケストレーションのみ。生成クエリの使用とエラー正規化を RDB / `pgerror` の README に従って行うこと。Repository とクエリの 1:1 対応は advisory として扱う。JOIN・複数クエリの操作・ディスパッチはいずれも妥当だからである。
   - **pkg:** `internal/` へ依存しないこと。フレームワーク非依存で再利用可能であること。feature 固有の業務ロジックを持たないこと。サブパッケージ README に明示された例外があればそれを尊重する。

5. 委譲が使えるなら、対象となる独立した役割を並列で fan-out する。使えない場合はその指示をインラインで実行する。各層 auditor には解決済みファイル一覧と共有 lint 出力を渡し、**`make go-lint` を 2 回以上実行しないこと。** 監査対象の変更が domain のコードまたは ADR / README 群に触れている場合は、`.codex/agents/ddd-origin-auditor.toml` を通して `ddd-audit` を `quick` スコープのプリセットで併せて実行する。このときスコープを再度尋ねさせてはならない。これは層ごとの監査とは分けて扱うこと。リポジトリが文書化した DDD の解釈を Evans と比較し、乖離を人間へ提起するものであって、裁定も修正も決して行わない。

   スコープがリポジトリ全体または指定 layer で、`internal/domain/**` を含む場合は、`.codex/agents/ddd-modeling-reviewer.toml` も fan-out する。domain の解決済み現行ファイル一覧を渡し、`baseRef` は渡さない。差分を渡すと、このレンズが `impl-review` の縮小版になるためである。変更ファイルスコープでは起動しない。DDD モデリングレンズを省略した理由として、差分レビューは `impl-review` の担当であることをレポートに 1 行記す。
6. 次の形式で日本語で報告する。

   ```text
   arch-check 結果（スコープ: <changed | full | layers>）

   [lint]
   - <file:line>: <message>

   [<layer>]
   - violation: <file:line> — <problem>
     source: <AGENTS.md or README location>
     remediation: <action>
   - suggestion: <file:line> — <advisory>
     source: <location>

   [ddd-modeling]
   - <リポジトリの DDD 文書を根拠とする finding、または変更ファイルスコープでの省略理由>

   総計: violations <n>, suggestions <n>
   ```

   **findings を捏造しないこと。** 問題が無ければ、監査した層を名指したうえで violation が見つからなかったと述べる。

## 任意の TODO 引き渡し

明示的な承認を得たあとにのみ、`internal/domain` / `internal/controller` / `internal/infrastructure` の suggestion に対して、素の日本語の `// TODO:` コメントを足す。

挿入前に直前の 3 行を確認する。既にコメントブロックがあるならスキップする。観測した逸脱と、人間に委ねる判断を記述する。AI 専用のマーカーは使わないこと。violation に対して TODO を書かないこと。

## 完了時の制約

- 自動修正・stage・commit・push はしない。
- findings ごとに、それを統べる README または `AGENTS.md` を引用する。
- 生成ファイルには触れない。
- 任意の引き渡しが承認された場合は、TODO を足した箇所とスキップした箇所を分けて報告する。
