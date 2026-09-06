# Query Service 実装ガイド

## オニオンアーキテクチャにおける Query Service の位置づけ

オニオンアーキテクチャでは、永続化の抽象は Domain 層の **Repository interface** として定義されます。Repository は Aggregate 単位の CRUD を担い、Domain の不変条件を守ります。

一方、Query Service（QS）は **この原則に対する意図的な例外**です。

```mermaid
flowchart TB
    subgraph "Domain 層"
        RepoIF["Repository interface"]
    end
    subgraph "Usecase 層"
        QSIF["QueryService interface"]
    end
    subgraph "Infrastructure 層"
        RepoImpl["Repository 実装"]
        QSImpl["QueryService 実装"]
    end

    RepoImpl -. implements .-> RepoIF
    QSImpl -. implements .-> QSIF
```

### なぜ QS の interface を Domain ではなく Usecase に置くのか

|観点|Repository|Query Service|
|---|---|---|
|関心事|Aggregate の永続化|ユースケース固有の検索|
|粒度|Aggregate 単位|画面 / API レスポンス単位|
|返却型|Domain Entity|DTO（表示用の射影）|
|不変条件|Domain が保証|関与しない|
|interface 配置|Domain 層|Usecase 層|

QS が返すのは **Aggregate の完全な再構成ではなく、ユースケースが必要とする射影（projection）** です。これは Domain の関心事ではなく Usecase の関心事であるため、interface は Usecase 層（`internal/usecase/<aggregate>/query`）に配置します。

> **注記（Repository も小さな read model を返しうる）。** 上表の「返却型 = DTO」は*典型的な*切り分けで
> あって絶対ではありません。*固定の参照マスタ*を JOIN する Repository の読み取りは、解決済みの表示値を
> 持つ小さな read model を返しても、なお単一 Aggregate の Repository 読み取り（QS ではない）です。
> <!-- 撤去後にこの箇所へ自分の例を置くための指針。
>      目的: どの表からどの表への JOIN かが見えないと、例外の範囲が伝わらない。
>      意義: 効くのは「参照マスタであること」で、JOIN の向きや表の数ではない。
>      書き方: 集約の表 → 参照マスタの表、を矢印 1 本で示す。 -->
> <!-- sample-api:begin -->
> （サンプルでの例は `purchases` → `purchase_statuses`）
> <!-- sample-api:end -->
> 読み取りを QS へ動かすのは、*独立した* Aggregate をまたぐことや派生した射影であって、単に Entity 以外を
> 返すことではありません。`docs/rules.md` の「Repository / QueryService Rules」節を参照してください。

### CQRS との関係

QS の導入は **軽量 CQRS（Command Query Responsibility Segregation）** のアプローチです。

- **Command（書き込み）**: Repository を経由し、Domain Entity の不変条件を守る
- **Query（読み取り）**: QS を経由し、パフォーマンス最適化された検索クエリを直接実行

完全な CQRS（別 DB / イベントソーシング）ではなく、**同一 DB 上で読み書きの責務を分離する実用的な設計**です。

### QS を採用する判断基準

以下に該当する場合、Repository ではなく QS を検討します。

- 複数テーブルの JOIN が必要な検索
- ページネーション付きの一覧取得
- 全文検索やキーワード検索
- 集計・グルーピングが必要なクエリ
- 自然な形が projection であり、full Aggregate として再構成するのが無駄になる読み取り
  （重い Aggregate から数カラムだけ、または結合ビュー）

逆に、**単一 Aggregate の単純な読み取りは Repository に留めます** — ID による取得、および
Aggregate 自身の属性による単純なフィルタ・一覧・件数取得（`SELECT * FROM <table> ORDER BY ...`
のような無フィルタの全件一覧を含む）。多数の行を返すことや、結果をレスポンス DTO へ写像すること
それ自体では QS に移す理由にはなりません — Aggregate を横断するか、上記の検索複雑性のケースに
該当する場合のみ QS を使います。[`docs/rules.md`](../../../../docs/rules.md) の
§ "Repository / QueryService Rules" を参照してください。

## 役割

Query Service は **検索・一覧取得などの読み取り専用クエリーを提供する層**です。

```mermaid
flowchart TB
    Controller --> Usecase --> QS["QueryService"] --> Sqlc["sqlc"] --> DB["Database"]
```

Query Service の責務は次の通りです。

1. SQL検索の実行
2. Row → Domain Entity / DTO 変換
3. DBエラー正規化

Query Service は **ビジネスロジックを持ちません。**

## アーキテクチャ上の位置

QueryService 実装は次の場所に配置します。

`internal/infrastructure/rdb/query_service/<aggregate>/`

例

```txt
query_service/
 └ <aggregate>/
     └ <aggregate>_query_service.go
```

QueryService Interface は **Usecase 層**に配置します。

`internal/usecase/<aggregate>/query`

例

```txt
internal/usecase/<aggregate>/query/<aggregate>_query_service.go
```

Infra はこの Interface を **実装するのみ**です。

## QueryService の責務

QueryService は **検索専用のデータ取得**を担当します。

```mermaid
flowchart TB
    Query --> Sqlc --> Row --> Domain["Domain Entity or DTO"] --> Ret["return"]
```

QueryService は次を行いません。

- ビジネスルール
- Usecaseロジック
- Controller処理
- トランザクション管理

## sqlc の利用

QueryService は **sqlc 生成クエリ**を利用します。

```go
rows, err := db.Search<Entities>(ctx, &gen.Search<Entities>Params{...})
```

## SQL分割設計

検索条件（active / deleted / all）は SQL 内で分岐せず、クエリを分割して実装します。

例：

- Search<Entities>
- SearchActive<Entities>
- SearchDeleted<Entities>

理由：

- SQLの可読性向上
- インデックス効率の向上
- sqlc生成コードの単純化

## LIKE検索ヘルパー

キーワード検索では`internal/infrastructure/rdb/sqlc`のヘルパーを利用します。

例

```go
escaped := sqlc.EscapeForLike(keyword, sqlc.DefaultLikeEscapeChar)
pattern := sqlc.WrapContainsLikePattern(escaped)
```

目的

- LIKEインジェクション防止
- 検索パターン統一

キーワード検索は OR 条件（ILIKE ANY）で実行されます。

## 削除状態の制御

削除状態のフィルタリングは、Go側で分岐し、専用クエリを呼び分けます。

```go
switch {
case filter.Active == nil:
    // 全件
case *filter.Active:
    // active
case !*filter.Active:
    // deleted
}
```

この設計により：

- SQLの複雑化を防ぐ
- インデックス効率を維持する
- 可読性を向上させる

## Row → 返却型への変換

sqlc が返す Row 構造体は **Infrastructure 専用型**です。

ただし、本プロジェクトでは sqlc の override を利用して、生成時に次のような型変換を適用しています。

- nullable → pointer 型
- UUID → `pkg/uuid` 型

そのため QueryService では、追加の変換処理をほとんど行わず、生成済みの型をそのまま Domain constructor または DTO に渡せます。

```go
u, err := <aggregate>.New(
    row.ID,
    row.Field1,
    row.Field2,
    ...
)
```

重要ルール

- sqlc Row をそのまま上位層へ返さない
- Domain Entity または DTO に変換する

## UUID について

本プロジェクトでは sqlc override により、DB 上の UUID と Domain で利用する `pkg/uuid` を同一の扱いに寄せています。

そのため QueryService での明示的な UUID 変換は基本的に不要です。

```go
row.<Entity>.ID // そのまま利用可能
```

UUID の生成・比較・補助処理は `pkg/uuid` のラッパーを利用します。

## Nullable について

nullable 値は sqlc override により pointer 型として扱われます。

そのため QueryService では追加の変換処理は不要です。

```go
row.<Entity>.OptionalText  // *string
row.<Entity>.DeletedAt     // *time.Time
```

## DB アクセス（driver）

QueryService は `driver.DatabaseDriver` を通じて DB にアクセスします。

```go
db := gen.New(driver.New(ctx, s.db))
```

`driver.New(ctx, db)` は次を提供します。

- DB / Tx の透過切り替え（context に tx があればそれを採用）
- Context ベース接続取得

SQL のログ / トレースは driver の接続層に結線した pgx クエリトレーサーが透過的に付与します
（`driver/README.md` 参照）。そのため QueryService は **DB 接続状態を意識しない設計**になります。

## エラー正規化

PostgreSQL エラーは`internal/infrastructure/rdb/pgerror`で正規化します。

```go
return pgerror.NormalizeError(err)
```

主な変換

```mermaid
flowchart TB
    A["pgx.ErrNoRows"] --> B["ErrNotFound"]
    C["unique violation"] --> D["ErrConflict"]
    E["connection error"] --> F["ErrUnavailable"]
    G["others"] --> H["ErrInternal"]
```

## Observability（Tracing）

QueryService では

`observability.LayerTracer`

を利用します。

```go
ctx, endSpan := s.tracer.Start(ctx)
defer endSpan()
```

QueryService は

- span開始
- span終了

のみを責務とします。

### span名について

span名は LayerTracer 側で統一的に付与されるため、QueryService 側で明示的に指定する必要はありません。

### 設計意図

- トレーシングの一貫性確保
- 各レイヤーでの責務分離
- OpenTelemetry への直接依存排除

## DI（Dependency Injection）の仕組み（Query Service）

Query Service は **Uber Fx による DI** で生成されます。  
Repository と同様に、Infrastructure 層で実装し、Usecase 層の interface に注入されます。

### 全体構成

Query Service は `fx.Provide` により登録され、Usecase に注入されます。

```mermaid
flowchart TB
    Module["InfrastructureModule"]
    Provide["fx.Provide(<aggregate>qs.New)"]
    IF["query.<Aggregate>QueryService (interface)"]
    Usecase["Usecase"]

    Module --> Provide --> IF --> Usecase
```

### internal/di/module/persistence.go の役割

永続化系のプロバイダ（repository / query_service / system_cqrs）は `persistenceModule`
に登録され、`InfrastructureModule()` がこれを合成します。

```go
func persistenceModule() fx.Option {
    return fx.Module("persistence",
        fx.Module("query_service",
            fx.Provide(
                <aggregate>qs.New,
            ),
        ),
    )
}
```

- `fx.Provide`
  - Query Service のコンストラクタを登録
- 戻り値は **Usecase 層で定義された interface**
  - 例: `query.<Aggregate>QueryService`

### Query Service のコンストラクタ設計

```go
func New(
    db driver.DatabaseDriver,
    tf observability.TracerFactory,
) query.<Aggregate>QueryService {
    return &service{
        db:     db,
        tracer: tf.Infra(),
    }
}
```

ポイント：

- 戻り値は **interface（Usecase定義）**
- 依存はすべて引数で受け取る（new禁止）
- DB / Tracer などの外部依存は Infrastructure に閉じ込める

### DI の流れ

```mermaid
flowchart TB
    Provide["fx.Provide(<aggregate>qs.New)"]
    IF["query.<Aggregate>QueryService"]
    Usecase["Usecase (依存)"]

    Provide --> IF --> Usecase
```

Usecase 側では

```go
type service struct {
    qs query.<Aggregate>QueryService
}
```

のように interface で受け取ります。

### Repository との違い（DI観点）

||Repository|Query Service|
|---|---|---|
|interface 定義場所|domain|usecase|
|返却型|domain.Repository|query.QueryService|
|用途|永続化|検索|

### なぜ Usecase interface を返すのか

- Query は Usecase の関心事（ユースケース単位）
- Aggregate単位ではないため Domain に置かない
- 検索仕様変更に柔軟に対応できる

### AI / 開発者向けルール

- Query Service の constructor は必ず `New` で定義すること
- 戻り値は Usecase interface にすること
- Query Service 内で依存を new しないこと
- DI登録は `internal/di/module/persistence.go`（`persistenceModule`）に追加すること

## QueryService 構造体

QueryService は次の依存を持ちます。

```go
type service struct {
    db     driver.DatabaseDriver
    tracer observability.LayerTracer
}
```

constructor

```go
func New(
    db driver.DatabaseDriver,
    tf observability.TracerFactory,
) query.<Aggregate>QueryService {
    return &service{
        db:     db,
        tracer: tf.Infra(),
    }
}
```

## Repository との違い

Repository と QueryService の役割は異なります。

||Repository|QueryService|
|---|---|---|
|目的|Aggregate 永続化|検索|
|操作|CRUD|検索|
|責務|Aggregate単位|検索専用|
|配置|domain interface|usecase interface|

## Anti-Patterns

### 1. Repository に検索を書く

検索処理は Repository ではなく QueryService に実装します。

ただし Repository では、次のような単純なフィルタは許容されます。

- ID / 外部キーによる取得
- 単純な条件絞り込み
- 件数取得（COUNT）

それ以上の検索（複数条件・全文検索など）は QueryService に実装します。

### 2. ビジネスロジックを書く

QueryService は **データ取得のみ**です。

NG

```go
if entity.IsPremium() {
}
```

### 3. sqlc Row を返す

NG

```go
return rows
```

必ず Domain に変換します。

## 実装例

```go
// service は QueryService の実装です。
// DBアクセスとトレーシングを責務として持ちます。
type service struct {
    db     driver.DatabaseDriver
    tracer observability.LayerTracer
}

// New は QueryService のコンストラクタです。
// 依存はすべて外から注入し、内部で new は行いません。
func New(
    db driver.DatabaseDriver,
    tf observability.TracerFactory,
) query.<Aggregate>QueryService {
    return &service{
        db:     db,
        tracer: tf.Infra(),
    }
}

// buildLikeTokens は、キーワードを LIKE パターンへ変換します。空の場合は全件マッチの ["%"] を返します。
func buildLikeTokens(keywords []string) []string {
    if len(keywords) == 0 {
        return []string{"%"}
    }
    tokens := make([]string, len(keywords))
    for i, kw := range keywords {
        escaped := sqlc.EscapeForLike(kw, sqlc.DefaultLikeEscapeChar)
        tokens[i] = sqlc.WrapContainsLikePattern(escaped)
    }
    return tokens
}

// FindByFilter は、キーワードと削除状態に基づいて検索します。
// - キーワードは LIKE パターンに変換
// - 削除状態は Go 側で分岐
// - SQL は専用クエリを呼び分ける
func (s *service) FindByFilter(ctx context.Context, filter *query.<Aggregate>SearchFilter, limit, offset int32) (query.<Aggregate>SearchResults, error) {
    ctx, endSpan := s.tracer.Start(ctx)
    defer endSpan()

    tokens := buildLikeTokens(filter.Keywords)
    db := gen.New(driver.New(ctx, s.db))

    // 削除状態に応じてクエリを切り替える
    switch {
    case filter.Active == nil:
        return fetchSearchAll(ctx, db, &gen.Search<Entities>Params{
            PatternsParam: tokens,
            LimitParam:    limit,
            OffsetParam:   offset,
        })
    case *filter.Active:
        return fetchSearchActive(ctx, db, &gen.SearchActive<Entities>Params{
            PatternsParam: tokens,
            LimitParam:    limit,
            OffsetParam:   offset,
        })
    default:
        return fetchSearchDeleted(ctx, db, &gen.SearchDeleted<Entities>Params{
            PatternsParam: tokens,
            LimitParam:    limit,
            OffsetParam:   offset,
        })
    }
}

// fetchSearchAll は、全件を検索するヘルパー関数です。
// service メソッドからロジックを分離し、責務を明確にします。
func fetchSearchAll(
    ctx context.Context,
    db *gen.Queries,
    params *gen.Search<Entities>Params,
) (query.<Aggregate>SearchResults, error) {
    rows, err := db.Search<Entities>(ctx, params)
    if err != nil {
        return nil, pgerror.NormalizeError(err)
    }

    // Row → DTO 変換
    // sqlc override により nullable → pointer / UUID → pkg/uuid は変換済みのため、
    // 各カラムを最小限の追加変換で DTO へ詰め替えます。
    results := make(query.<Aggregate>SearchResults, len(rows))
    for i, row := range rows {
        results[i] = &query.<Aggregate>SearchResult{
            // Field: row.Field,
            // ...
        }
    }

    return results, nil
}
```
