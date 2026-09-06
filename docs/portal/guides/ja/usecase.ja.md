# ユースケース層（`internal/usecase`）ガイド

## オニオンアーキテクチャでの役割

- アプリケーションサービスとして、ユースケースの **手続き（ワークフロー）** を調整する層。
- 入力（DTO/VO）を受けて、**Domain（エンティティ/ドメインサービス）とRepository（ドメインの抽象）** を組み合わせ、結果（DTO）を返す。
- トランザクション境界と整合性の担保の単一起源（Tx開始/終了、リトライ方針など）。
- 外界（HTTP/DB/メッセージング）の詳細は知らない。純粋なアプリ語彙で完結する。

## ユースケース層の処理フロー

ユースケースは **アプリケーションのワークフローを調整するレイヤー**です。  
Domain と Repository を組み合わせて **処理の順序を定義**します。

```mermaid
flowchart TB

    DTO_in["DTO (Input)"]
    Usecase["Usecase (Application Service)"]
    Domain1["Domain (Business Rule)"]
    Repo["Repository (Persistence)"]
    Boundary["Boundary (Tx / Clock / Auth)"]
    Domain2["Domain (Re-evaluation / Composition)"]
    DTO_out["DTO (Output)"]

    DTO_in --> Usecase
    Usecase --> Domain1
    Domain1 --> Repo
    Repo --> Boundary
    Boundary --> Domain2
    Domain2 --> DTO_out
```

基本的な処理フローは以下です。

1. DTOを受け取る
2. 入力の整形 / ポリシー適用
3. Domain呼び出し
4. Repositoryによる永続化
5. DTOへ変換
6. 結果返却

Usecase は **ビジネスルールを実装する場所ではありません**。  
ビジネスルールの実装は **Domain層** に配置します。

ただし Usecase から **Domain のビジネスルールを呼び出すこと自体は許可されます**。  
Usecase の責務は、Domain の振る舞いを **組み合わせてユースケースのワークフローを構築すること**です。

つまり、【Domain のビジネスルールを **呼び出す**が、**新しいビジネスルールは定義しない**】という役割に留めます。

Usecaseは以下のみを担当します。

- ワークフロー制御
- トランザクション管理
- Domain / Repository の協調
- DTO変換

オーケストレーションには、Controller のために **複数の読み取りを1操作へ合成する**ことも含みます。例: ページング一覧のエンドポイントは `{ Items, Total }` を返す単一メソッドを公開し、handler に一覧と件数を別々に呼ばせて束ねさせません。

## アプリケーションサービス層の設計方針

プロジェクトの Usecase は **Application Service Pattern** を採用しています。

Application Service は **ユースケース単位のアプリケーションロジック**を表現します。

```mermaid
flowchart TB

subgraph Usecase Layer
    BoundaryInterface
    ApplicationService
end

Controller --> ApplicationService

ApplicationService--> Domain
ApplicationService --> BoundaryInterface

Infrastructure--> Domain
Infrastructure --> BoundaryInterface
```

Application Service の責務

- ユースケース単位の処理
- トランザクション境界
- ドメイン操作の順序制御
- DTO ↔ Domain変換

Application Service が **やってはいけないこと**

- ビジネスルールの実装
- インフラの直接利用
- フレームワーク依存

Application Service は **Domainの振る舞いを組み合わせるだけ**に留めます。

## Application Policy

Usecase 層では **Application Policy（アプリケーションポリシー）** を扱います。

Application Policy とは、**ドメインロジックではなくアプリケーションの振る舞いを決定するルール**です。

Domain と Usecase の責務は次のように分離されます。

|種類|内容|配置|
|-----|-----|-----|
|Domain Logic|ビジネスルール|Domain|
|Application Policy|ユースケースの処理手順|Usecase|

### Domain Logic の例

```mermaid
flowchart TB
    A["ユーザー名の制約"]
    B["メール形式ルール"]
    C["状態遷移"]
```

これらは **Domain 層に実装します。**

### Application Policy の例

```mermaid
flowchart TB
    A["トランザクション内で実行"]
    B["関連集約の属性を取得"]
```

これらは **Usecase 層に実装します。**

Usecase の役割は次の通りです。

```mermaid
flowchart LR
    Usecase["Usecase"] --> Policy["Application Policy"]
    Usecase --> Workflow["Workflow"]
    Domain["Domain"] --> Rule["Business Rule"]
```

## Boundaryのコンセプト

プロジェクトでは **Usecase が Infrastructure に直接依存しないようにするため Boundary を導入しています。**

Boundary とは **外部システムとの境界を表すインターフェース**です。

Usecase はこれらの **interface のみ参照**し、実装は Infrastructure 側で提供されます。

### 代表的な Boundary

```txt
Transaction Manager
Clock
Auth Context
Messaging / EventPublisher
Observability
```

### 時刻の扱い

プロジェクトでは **時刻の取得は Usecase 層で一元管理**します。

そのため **`time.Now()` を直接呼び出すことは禁止**します。

代わりに Boundary として提供される **`clock.Clock`** を利用します。

理由：

- テストを deterministic（再現可能）にするため
- タイムゾーンや時刻ソースの差異を吸収するため
- AI や開発者が `time.Now()` を直接使うことを防ぐため

Usecase では必ず次のように時刻を取得します。

```go
now := u.clock.Now()
```

例：

```go
now := u.clock.Now()
userEntity, err := user.New(..., now, now, nil)
```

### ルール

Usecase 層では以下を守ります。

```txt
Forbidden: time.Now()
Allowed:   clock.Clock.Now()
```

時刻の取得は **Usecase → Domain へ渡す**形で扱い、
Domain 側では新たに時刻を取得しない設計を推奨します。

これにより **時刻依存ロジックを完全にテスト可能に保つ**ことができます。

### 依存関係

```mermaid
flowchart LR

Controller --> Usecase

Usecase --> Domain
Usecase --> RepositoryInterface
Usecase --> BoundaryInterface

Infrastructure --> RepositoryInterface
Infrastructure --> BoundaryInterface
```

重要なルール

- Usecase は **Infrastructure に依存しない**
- Usecase は **interface のみ参照する**
- Infrastructure が **interface を実装する**

これにより **Dependency Inversion** を維持します。

## CQRSポリシー

プロジェクトでは **完全な CQRS 分離は採用していません。**

理由

- 小〜中規模サービスでは過剰設計になりやすい
- Query / Command を完全分離すると再利用性が下がる
- Repository が複雑になりやすい

そのため **軽量 CQRS ポリシー** を採用しています。

### Command

状態変更を伴う処理。

例

```txt
CreateUser
UpdateUser
DeleteUser
```

特徴

- Domain Entity を使用
- Transaction が必要
- Domain 不変条件を検証

### Query

読み取り専用の処理。

例

```txt
GetUser
ListUsers
SearchUsers
```

特徴

- DTO を直接返すことを許容
- Domain Entity に変換しない場合がある
- Transaction 不要

### Repository に許可する Query

```txt
FindAll
FindByID
FindByKeyword
CountAll
CountByActive
```

JOIN は **ドメイン境界を壊さない範囲で許可**します。

### Repository に含めないもの

```txt
GROUP BY
集計関数
WITH 句
複雑な分析クエリ
```

これらは

- Analytics
- Reporting
- Data Pipeline

など別レイヤーで扱います。

### インフラをドメインで検証する

**実行するのはインフラ、定義するのはドメイン。両者を突き合わせる場所が Usecase です。** これは
双方向に成り立ち、同じ規則を 2 回言っているにすぎません。

- **書き込み** — インフラが書き込みを実行し、その後 Usecase が対象集約を Repository 経由で読み直し、
  集約自身に検証させます（[ADR-0032 (lightweight-cqrs)](../../docs/adr/0032-lightweight-cqrs.ja.md)）。
- **読み取り** — インフラが絞り込みを実行し、その後 Usecase が返ったエンティティを、基準を定義する
  ドメインの述語で確かめます。インフラは基準を**実行**するのであって**著作**しません
  （[`internal/domain/README.md`](../domain/README.ja.md) の Query and Aggregate 節を参照）。
  述語を満たさない行は、両者が乖離したことを意味します。

乖離は握り潰さずエラーとして表に出します。黙って落とすと、欠陥が観測可能になったまさにその瞬間に
それを隠すことになり、読み取りは自ら掲げた定義と一致しない結果を返し続けます。

**この検証の限界を決めるのは、結果の形ではなく基準の形です。** 返ってくる行について何かを述べる基準は
検証できます——結果にドメインの述語を当てれば、ずれはそこに現れる。行を取り除く基準は検証できません。
取り除かれた行は結果に存在せず、不在は観測できないからです。確かめるにはフィルタ無しでもう一度引く
ことになり、それはその読み取りが避けるために最適化された作業そのものです。

したがって射影であることが免除の理由にはなりません。QueryService が基準の対象そのものを返すなら、
集約を再構築しなくても、述語が必要とするフィールドを載せて確かめます。定義はドメインに留まり、
インフラは実行するだけ、という形が保たれます。エンティティに対する述語とは別に、値に対する述語も
ドメインへ置き、両方の経路が 1 つの定義を共有するようにします——クエリ層に 2 つ目の写しを育てない
ためです。「レビューだけが支える」は、除外とその上の集計——**取り除く側の基準**——のためにとっておき、
それはクエリを書く場所に明記します。何が落とされたかを読者が見られるのはそこだけだからです。

## このプロジェクトでの役割

- Command / Query のサービスは `internal/usecase/<feature>/`（例: `user/`）配下に置く。
  - Command: 作成 / 更新 / 削除（Tx を開始し、Domain の不変条件を担保する）。
  - Query (QS): 読み取り最適化。DTO を直接返すことを許容する。
  - ページング / バリデーションなど、プロトコルに依存しないポリシーを集約する。
    - 例: `paging.NewPageFrom1Based(page, perPage)`
- エラーは `apperror.ErrXXX` で包み、Controller が HTTP レスポンスへ写像できるようにする。
- DI (fx) が Repository インターフェース / TxManager / Config などの依存を注入する。

## サードパーティを最小限に抑える

- Usecaseは原則 標準ライブラリのみ（context, time, errors, fmt など）。
- ORM・SQL実行・HTTPクライアント・EchoなどI/O系は一切持ち込まない。
- 横断的例外: `internal/logging.Logger` は `internal/apperror` と同様、専用 boundary を介さずコンストラクタ DI で直接注入してよい。純粋な mock 可能インターフェースであり、失敗ログが必要なバックグラウンドワーカー（例: outbox relay の dead-message 警告）に限って使用する。それ以外は `metrics`/boundary を優先する。
- 型定義やDTOもプロジェクト内型で閉じる。sqlc生成型/driver型やOpenAPI生成型は上位/下位の層に隔離。
- テストも`testify`/`mock`程度に留め、モックはinterfaceベースで注入。
- どうしても必要な場合は、[pkg/](../../pkg/)で薄いラッパーを作成する。

## 実装上の注意点

### 命名/構造

- インターフェイスは`Usecase`（例：`user.Usecase`）で統一。
- インスタンスの生成関数名は `New` で統一し、[di/module/usecase.go](../di/module/usecase.go) で登録する。

### doc コメント：インターフェイス側と実装側

本リポジトリではインターフェイスとその実装が同一パッケージに同居する（`Usecase` と非 export の
`usecase`）ため、双方が doc コメントを持つ。両者は**読み手が異なる**ので、互いの複製にしてはならない。

- **インターフェイス側 doc ＝ 呼出側契約**（`docs/rules.md` § Comment Rules に準拠）。記述は
  **アプリケーション語彙**に留めること。Repository / QueryService / Boundary の名指しは可（Usecase が正当に
  所有する内向きの抽象のため）だが、**インフラ語彙は層を漏らす**ため禁止 — SQL の断片
  （`SELECT … FOR UPDATE`）、テーブル名、カラム名、keyset の仕組みなど。それらは既にそう書いてある
  Infrastructure 側の doc コメントの担当である。
- **実装側 doc ＝ 次にここを触る実装者向け。** アプリケーション語彙のまま、契約より**一段具体**に踏み込ん
  でよい。どの協力者が保証を担っているか、なぜトランザクション境界がそこにあるか、何が失敗ではなく
  degrade するか、なぜその競合はリトライで解消しないか、など。
- **インターフェイス側 doc の逐語複製は書かない。** 複製は何も足さず、2 箇所で腐る。足す価値のある具体が
  無いなら**実装側 doc は省略してよい** — 実装型は非 export のため `revive` の `exported` ルールの対象外。

このインターフェイスと実装の書き分けは**この層固有ではない**。インターフェイスとその非 export な実装が同一
パッケージに同居する箇所すべてに適用され、`internal/logging` の `Logger` や `internal/observability` の
provider factory も同じ規則の下にある。層ごとに異なるのは下記の語彙の部分であり、逐語複製を書かないという
部分はリポジトリ全体の規則である。

同じアプリケーション語彙の規則は、**この層が所有する port インターフェイス** — Boundary / CommandService /
QueryService — にも適用される。port は外部との接ぎ目であり、だからこそ技術中立に記述しなければならない。
契約すべきは*保証*であって、現時点でそれを実現している機構ではない。`Lock<Aggregate>` の port は悲観ロックを取ること、
そのロックが何を直列化するかを述べる（呼出側はその両方に依存する）が、そのロックが `SELECT … FOR UPDATE`
であることは述べない。`QueryService` は所有権を自身の絞り込みで担保すると述べ、SQL の `WHERE` 述語とは
書かない。機構は Infrastructure 実装側の doc コメントの担当であり、そちらでは名指ししてよい
（[`internal/infrastructure/README.md`](../infrastructure/README.md) § Doc comments may name technical
detail を参照）。

コメントの内容基準そのもの（契約 + 非自明な Why、How のナレーション / 経緯 / 言い換えは書かない）は
`docs/rules.md` § Comment Rules にある。本節は**どの doc コメントが何を担うか**だけを定める。

### ビジネスロジックを実装しない？ → 誤解を避けて明確化

- “ドメインロジック”は Domain 層に置く（エンティティ/VO/ドメインサービスのメソッド）。
- Usecase は“手続きロジック”（順序・Tx・外部境界の協調・入力検証と方針適用）を担当。

### HTTP/DBの要素は持ち込まない

引数 / 戻り値に使ってはいけない型:

- `http.*`
- `*echo.Context`
- `sqlc` 生成型
- `sql.Null*`
- DB のカラム名
- OpenAPI 生成型

代わりに DTO / VO（Page / Filters / Actor）で表現する。

### エラー方針

操作の結果に応じて `internal/apperror` のセンチネルでラップする。`ErrXXX` から HTTP ステータスへの
対応表は [`internal/apperror/README.ja.md`](../apperror/README.ja.md) の対応表を参照。想定外のエラーは
そのまま返すか `apperror.ErrInternal` に包む。

`apperror.ErrXXX` センチネルでラップする場合は、標準の `fmt.Errorf("%w", ...)` ではなく
`pkg/xerrors.Wrap(apperror.ErrXXX, "context")` を使う。スタックトレースを保持しつつ
`xerrors.Is` でセンチネル判定が可能になる。

### GC / バッチ掃除

行やオブジェクトをバッチで掃除する Usecase は、決まった形に従う。

- 既定のバッチサイズは公開定数（`Default<Feature>BatchSize`）として公開し、呼び出し側から見えて上書きも
  できるようにする。0 以下の引数はエラーではなく「既定値を使う」と解釈する。フラグ無しで起動される
  ジョブは異常ではなく通常だからである。
- 1 バッチずつループし、返ってきた件数が要求したサイズを下回ったら終了する。
- **エラーを返す場合でも件数は返す。** 完了したバッチは既にコミット済みでロールバックできないため、
  失敗時に 0 を報告する `Result` は嘘になる。呼び出し側はそこまで通った分をログに出す（上記「出力 DTO
  の命名」の `Result` 側を参照）。

### 出力 DTO の命名

Usecase の戻り値 DTO は、そのメソッドが何を返すかによって 2 つのサフィックスを使い分ける。

- **`<概念>View`** — 呼び出し側向けに組み立てた状態の射影であって、集約そのものではない。射影が単一
  フィールドしか持たない場合でもサフィックスは落とさない。落とすと外向きの射影とドメイン型を一目で
  区別できなくなる。
- **`<概念>Result`** — 何かを変更した操作の結末。処理した件数、スキップした件数、バッチが停止するまでに
  通した件数など。「何であるか」ではなく「何が起きたか」を報告する。

この区別を保つのは、両者で失敗時の意味論が違うためである。`View` は返るか返らないかのどちらかだが、
`Result` は**メソッドがエラーも返す場合にこそ意味を持つ** — 途中で失敗したバッチも、停止するまでに
コミットした分を報告しなければならない。

唯一の例外が `healthcheck.CheckHealth` で、素の `DTO` を返す。ヘルスは集約の射影でも変更の結果でもなく、
プローブ自身の読み取りであり、接頭辞にすべき概念をこの型は持たない。ここで接尾辞を付けると、パッケージが
持たないものを名前で主張することになる。他の箇所で接尾辞を省く根拠にはしないこと。

### ページング

- NewPageFrom1Based(page, perPage) で既定値/上限/1→0変換を統一。
- ページ番号が許容最大を超えたら `apperror.ErrInvalidArgument` を返す（offset は int32 変換時にクランプ）。
- keyset（カーソル）ページングは `tools/paging.Cursor` を土台にし、フィーチャごとに
  `encode<Feature>Cursor` / `decode<Feature>Cursor` のコーデック対を実装する。カーソルは呼び出し側に
  とって不透明で、順序キーの組を符号化したものである。形式不正やキー数不一致は
  `apperror.ErrInvalidArgument` として返す。

## 呼び出せる層 / 呼び出せない層

### 呼び出せる層

- Domain（エンティティ/ドメインサービス/Repositoryインタフェース）
- Boundary（tx / clock / auth 等）
- QueryService（必要な場合）

### 呼び出せない層

- 他の**業務** Usecase を直接呼ばない（後述）。
- UsecaseからInfra/Controller/HTTP/OpenAPI/SQL"実装"は呼ばない。

#### なぜ業務 Usecase 同士を繋がないのか

禁じているのは呼ばれること自体ではなく、**連鎖**である。A が B を呼べる規則の下では、やがて C が A を
呼び、D が C を呼ぶ。どこまでが 1 つの業務操作なのかを誰も言えなくなり、トランザクション境界も
追跡できなくなる。

したがって A の業務と B の業務を組み合わせる必要があるときは、A から B を呼ばない。両者を**合成**する
Usecase D を導入する。D は両者の上位に立って業務操作を繋ぐため、形が連鎖ではなく合成になる。

**技術 Usecase はこの規則の対象外である。** それ自体は業務操作ではないが、どの業務操作からも同じ形で
必要になるもの——outbox への emit など——は、この規則が防ごうとしている連鎖を作りようがないため、
直接呼んでよい。

Usecaseは **Infrastructureに依存してはいけません。**

Infrastructureへのアクセスは次を経由します。

- Repository interface
- Boundary interface

## Test戦略

Usecase 層は **純粋な Unit Test** としてテストします。

Infrastructure や外部システムを使用せず、  
**Domain と interface のみを利用してテストします。**

### テストの依存関係

Usecase のテストでは次の依存関係を採用します。

|依存|テスト方法|
|---|---|
|Domain|実装を使用|
|Repository|mock|
|Boundary|mock|
|Infrastructure|使用しない|

### テストの基本方針

Usecase テストでは、**Usecase のワークフローとアプリケーションポリシー** を検証します。

具体的には次を確認します。

- Usecase が正しい順序で Domain / Repository / Boundary を呼び出すこと
- トランザクション境界が正しく適用されること
- Domain エラー / Repository エラー / Boundary エラーが期待通り返ること
- DTO の組み立てが正しいこと

### Domain は mock しない

Domain は **ビジネスルールの実体**であるため、Usecase テストでは **実装をそのまま使用します。**

```go
userDomain, err := user.New(...)
require.NoError(t, err)
```

これにより、Usecase が **本物の Domain ルール** を前提に正しく動作するかを確認できます。

### Repository / Boundary は mock する

Repository や Boundary は **interface** を通して注入されるため、Usecase テストでは mock を利用します。

```go
ctrl := gomock.NewController(t)

userRepo := mock_user.NewMockRepository(ctrl)
clock := mock_clock.NewMockClock(ctrl)
```

### テスト対象

正常系:

- 想定通りの DTO が返る
- Repository が正しく呼ばれる
- Boundary が正しく呼ばれる
- Transaction が正しく使われる

異常系:

- Boundary のエラーが返る
- Repository のエラーが返る
- Domain 生成失敗時にエラーが返る
- 結果が空 / zero value になることを確認する

### テスト構成

テストは **正常系 / 異常系** に分けて構成することを推奨します。

```text
TestCreateUser
  ├ 正常系
  └ 異常系

TestListUsers
  ├ 正常系
  └ 異常系
```

### Deterministic

Usecase テストでは **固定時刻** を利用し、`time.Now()` に依存しません。

```go
now := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.Local)
clock.EXPECT().Now().Return(now)
```

これにより、再現性のあるテストを保証します。

### Fail Fast

アサーションは `require` を基本とします。

```go
require.NoError(t, err)
require.Equal(t, expected, actual)
```

前提条件が崩れた時点で即座に失敗させ、  
テストの意図を明確に保ちます。

### テストでやらないこと

Usecase テストでは以下を扱いません。

- DB 接続
- SQL 実行
- HTTP リクエスト
- OpenAPI 型の検証
- Infrastructure 実装の詳細確認

これらは **Infrastructure / Controller の責務**です。

### まとめ

```txt
Usecase
 ├ Domain         -> real
 ├ Repository     -> mock
 ├ Boundary       -> mock
 └ Infrastructure -> not used
```

これにより、次を高速かつ安定して検証できます。

- ワークフロー
- アプリケーションポリシー
- トランザクション境界
- エラーの伝播
- DTO 変換

## やっていいこと / いけないこと(まとめ)

### Do

- **DTO/VO（Page, Filters, Actor）**で受け渡し
- Tx 境界をここで定義（TxManager 経由で Do(ctx, func(txCtx){ ... })）
- Usecase層で初出の`apperror`でエラー分類を付与（errors.Is で Controller が判定しやすく）
- QSはRow→DTOに最短でマップ、CommandはDomainを介す。
- 表駆動テストでユースケースの分岐とTxの挙動を確認（testify）

### Don’t

- DomainのEntityを直接返す
- `http.Status` や `*echo.Context` を引数に取る/返す
- `sqlc`生成型や`sql.Null*`を引数/戻り値に使う
- `openapi/gen`の型を直接返す（DTOに詰め替えるのはControllerの責務）
- Listで0件をエラー化（`apperror.ErrNotFound`(404)は単体取得のみ）
- 別のUsecaseを直接呼んで複雑化（必要なら`Orchestrator`を定義）

## Observability（Tracing）の使い方

この Usecase層で直接OpenTelemetrySDKを扱わず、
observability.LayerTracerを経由してspanの開始・終了を行います。

### 1. Usecase層での span の開始と終了

各ハンドラーの先頭で必ず次の 2 行を記述してください。

```go
ctx, endSpan := s.tracer.Start(ctx)
defer endSpan()
```

- Start(ctx)でspanが開始され、trace_id/span_idがcontextに紐づきます。
- endSpan()は、spanの終了（span.End）を行います。
- defer endSpan()により例外や早期returnがあっても必ず終了されます。

ポイント：Usecase は span の開始・終了だけを知り、
OpenTelemetry SDK の詳細には一切触れません。

### 2. TracerのDI（observability.LayerTracer）

Usecaseは以下のようにobservability.LayerTracerを依存として受け取ります。

```go
type server struct {
    tracer   observability.LayerTracer
    txm      tx.Manager
    userRepo user.Repository // それぞれのプロジェクト
    pftRepo  prefecture.Repository
}
```

New関数内では、`observability.NewUsecaseTracer`でUsecase専用のトレーサーを生成します。

```go
func New(
    tf observability.TracerFactory,
    txm tx.Manager,
    userRepo user.Repository,
    prefectureRepo prefecture.Repository,
) Usecase {
    return &usecase{
        tracer:   tf.Usecase(),
        txm:      txm,
        userRepo: userRepo,
        pftRepo:  prefectureRepo,
    }
}
```

ここではSDKの生インスタンスを直接使わず、
observability層がtracerの生成ルール（レイヤー名やパッケージ名・関数名の抽出）を内部で隠蔽します。

## 実装例

<!-- sample-api:replace-begin -->
> 以下の例は、恒久的なパターン（tracer による span の開始/終了、`clock.Now()` による時刻取得、
> `txm.Do` によるトランザクション境界、Domain → DTO 変換）**を示すためだけに**、サンプルの
> `<aggregate>`（`user` と関連する `prefecture`）を用いています。これらのサンプル集約は
> `make setup-remove-sample-api` で削除されるため、`user` / `prefecture` は各自の集約の
> 代替として読み替えてください。要点は具体名ではなくパターンです。
<!-- sample-api:replace-with -->
<!-- = > 以下の例は、恒久的なパターン（tracer による span の開始/終了、`clock.Now()` による時刻取得、 -->
<!-- = > `txm.Do` によるトランザクション境界、Domain → DTO 変換）**を示すためだけに**、1 つの -->
<!-- = > `<aggregate>` と関連する集約を用いています。具体名は各自の集約の代替として読み替えて -->
<!-- = > ください。要点は具体名ではなくパターンです。 -->
<!-- sample-api:replace-end -->

```go
//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE
// 唯一性のある名称
package user

import (
    "context"

    "go-boilerplate/internal/observability"
    // それぞれ実装で使うパッケージをimport
)

// 入力と出力が共有する可変属性一式。
type UserMutableFields struct {
    FirstName      string
    LastName       string
    Email          string
    Phone          string
    PostalCode     string
    PrefectureName string
    City           string
    Street         string
    Building       *string
}

// 入力 DTO。
type CreateUserParamsDTO struct {
    UserID uuid.UUID

    UserMutableFields
}

// 出力 DTO。`View` サフィックスが、呼び出し側へ返す射影であることを示す。
type UserView struct {
    UserID uuid.UUID

    UserMutableFields
}

// usecaseという名称は固定
type usecase struct {
    tracer    observability.LayerTracer
    txm       tx.Manager
    clock     clock.Clock
    userRepo  user.Repository
    pftRepo   prefecture.Repository
    userQS    query.UserQueryService
}

// Usecase は、ユーザーに関するユースケースを定義します。
type Usecase interface {
    // ListUsersByKeyword は、ユーザー一覧を取得します。
    ListUsersByKeyword(ctx context.Context, params *ListUsersByKeywordParams, page *paging.Page) ([]UserView, error)
    // CreateUser は、ユーザーを作成します。
    CreateUser(ctx context.Context, dto *CreateUserParamsDTO) (UserView, error)
    // CountUsers は、ユーザーの総件数を返します。
    CountUsers(ctx context.Context, active *bool) (int64, error)
}

// Newという名称は固定
func New(
    tf observability.TracerFactory,
    txm tx.Manager,
    clock clock.Clock,
    userRepo user.Repository,
    prefectureRepo prefecture.Repository,
    userQueryService query.UserQueryService,
) Usecase {
    return &usecase{
        tracer:    tf.Usecase(),
        txm:       txm,
        clock:     clock,
        userRepo:  userRepo,
        pftRepo:   prefectureRepo,
        userQS:    userQueryService,
    }
}

func (u *usecase) ListUsersByKeyword(ctx context.Context, params *ListUsersByKeywordParams, page paging.Page) ([]UserView, error) {
    // Spanの開始・終了呼び出して設定
    ctx, endSpan := u.tracer.Start(ctx)
    defer endSpan()

    var (
        us  user.Users
        err error
    )

    // ユーザー一覧取得（Domainエンティティのスライス）
    if params != nil {
        keywords := search.ParseSearchTokens(params.Keyword, search.DefaultMaxTokens)
        us, err = u.userQS.FindByKeyword(ctx, keywords, params.Active, page.Limit32(), page.Offset32())
    } else {
        us, err = u.userRepo.FindAll(ctx, page.Limit32(), page.Offset32())
    }

    if err != nil {
        return nil, err
    }

    // オプション: observability.RunWithSpanで処理単位のspanを作成
    // 可観測性を高めるために、Domain層の処理もspanとして切り出すことができます。
    // オプションなのでなくても構いません。
    // 第一引数のctxは、後続で使う場合は返り値を受け取って上書きしてください。
    ctx, prefectureMap, err := observability.RunWithSpan(
        ctx, u.tracer, "usecase", "user", "prefectureMap", func(ctx context.Context) (map[uuid.UUID]*prefecture.Entity, error) {
            // ユーザーの都道府県IDを集めて、一括で都道府県エンティティを取得
            pids := make([]uuid.UUID, len(us))
            for i, u := range us {
                pids[i] = u.PrefectureID()
            }

            // 都道府県エンティティの取得
            // IDsメソッドは複数IDで一括取得するプロジェクトメソッドの例
            ps, pftErr := u.pftRepo.FindByIDs(ctx, pids)
            if pftErr != nil {
              return nil, pftErr
            }

            // 取得した都道府県エンティティをマップに詰め替え
            // Mapにすることで、後続のループで高速に参照できるようにする
            prefectureMap := make(map[uuid.UUID]*prefecture.Entity, len(ps))
            for _, p := range ps {
                prefectureMap[p.ID()] = p
            }

            return prefectureMap, nil
      })
    if err != nil {
      return nil, err
    }

    // ctxは、後続でobservability.RunWithSpanを使わない場合は不要
    _, views, err := observability.RunWithSpan(
        ctx, u.tracer, "usecase", "user", "buildViews", func(ctx context.Context) ([]UserView, error) {
            // 結果をDTOに詰め替え
            views := make([]UserView, len(us))
            for i, u := range us {
                views[i] = UserView{
                    FirstName:  u.FirstName(),
                    LastName:   u.LastName(),
                    Email:      u.Email(),
                    Phone:      u.Phone(),
                    PostalCode: u.PostalCode(),
                    City:       u.City(),
                    Street:     u.Street(),
                    Building:   u.Building(),
                }
                // 都道府県名をマップから取得してセット
                if p, ok := prefectureMap[us[i].PrefectureID()]; ok {
                    views[i].PrefectureName = p.Name()
                }
            }
            return views, nil
        })

    return views, err
}

// CreateUser は、ユーザーを作成するユースケースです。
func (u *usecase) CreateUser(ctx context.Context, dto *CreateUserParamsDTO) (UserView, error) {
    ctx, endSpan := u.tracer.Start(ctx)
    defer endSpan()

    // 時刻の取得はUsecase層で一元管理するルールに従う
    now := u.clock.Now()

    var (
        userEntity *user.User
        pftDomain  *prefecture.Entity
    )
    // トランザクションの開始と終了をTxManagerに任せる
    err := u.txm.Do(ctx, func(ctx context.Context) error {
        var err error
        pftDomain, err = u.pftRepo.FindByName(ctx, dto.PrefectureName)
        if err != nil {
            return err
        }

        userEntity, err = user.New(
            dto.UserID,
            dto.FirstName,
            dto.LastName,
            dto.Email,
            dto.Phone,
            pftDomain.ID(),
            dto.City,
            dto.Street,
            dto.Building,
            dto.PostalCode,
            now,
            now,
            nil,
        )
        if err != nil {
          return err
        }

        err = u.userRepo.Create(ctx, userEntity)
        if err != nil {
          return err
        }
        return nil
    })
    if err != nil {
      return UserView{}, err
    }

    return UserView{
      FirstName:      userEntity.FirstName(),
      LastName:       userEntity.LastName(),
      Email:          userEntity.Email(),
      Phone:          userEntity.Phone(),
      PostalCode:     userEntity.PostalCode(),
      PrefectureName: pftDomain.Name(),
      City:           userEntity.City(),
      Street:         userEntity.Street(),
      Building:       userEntity.Building(),
    }, nil
}

```
