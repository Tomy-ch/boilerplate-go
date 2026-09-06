# コントローラー層（`internal/controller`）ガイド

## オニオンアーキテクチャでの役割

- **外界（HTTP/CLI）とアプリケーションの境界面**
- **プロトコル適応（Adapter）** を担い、入力を **アプリ語彙（DTO/Value）** に変換して **Usecase** を呼ぶ
- **出力整形（Presenter）** で Usecase の結果を **OpenAPI のレスポンス型** へ詰め替える
- 例外（`error`）を **HTTP ステータス** + **エラーコード** へマッピング（`apperror` → Status）

> ポイント：**ビジネスロジックは一切持たない**。持つのは「HTTP / CLI の解釈と整形」だけ。

## ディレクトリ構成

この層は**外部がどこからアプリケーションへ入ってくるか**で分かれる。入口は次の 4 種であり、
これ以外が入口になることはない。

- `handler/` — HTTP リクエスト
- `job/` — CLI からの起動
- `worker/` — キューのメッセージ
- `outbox/` — outbox を polling して publish する relay

`stream/` は 5 種目ではなく、ひとつの機構（Realtime Delivery の SSE transport）の HTTP 入口である。HTTP リクエストで
あることは変わらず、feature 中立・非 strict・機構自身の DI module が結線する、という理由で `handler/` の隣に置く
（`docs/design/realtime-delivery.md` §3.1）。

残りのディレクトリはこの 4 つに奉仕する。`server/` は Echo インスタンスの構築、`httpstack/` は
ミドルウェアスタック、`error/response/` はエラーボディ、`conv/` は生成型とドメイン型の境界、
`ctxhelper/` は Echo context のアクセサである。

## サブディレクトリの役割

|ディレクトリ|説明|詳細|
|---|---|---|
|`handler/`|HTTP リクエストを受け取り Usecase へ委譲するハンドラ|[README](handler/README.ja.md)|
|`stream/`|Realtime Delivery の SSE endpoint。確定前の検証（ticket・cursor）と `Streamer` seam|[README](stream/README.ja.md)|
|`job/`|CLI から起動されるジョブのコントローラ|[README](job/README.ja.md)|
|`worker/`|pull-ack メッセージキューを消費し Usecase へディスパッチするワーカーエンジン|[README](worker/README.ja.md)|
|`outbox/`|outbox を周期的に poll し未 publish メッセージを送るリレーエンジン|[README](outbox/README.ja.md)|
|`realtime/`|instance 自身の queue から Realtime Delivery の wakeup / revocation を受け取り、接続側へ渡す consumer エンジン。instance lease の heartbeat ループ|[README](realtime/README.ja.md)|
|`server/`|Echo インスタンスの初期化と DI ライフサイクルへの統合|[README](server/README.ja.md)|
|`httpstack/`|ミドルウェア群（CORS, セキュリティ, ログ, 認証等）|[README](httpstack/README.ja.md)|
|`error/response/`|統一的な HTTP エラーレスポンスの生成と apperror マッピング|[README](error/response/README.ja.md)|
|`conv/`|OpenAPI 生成型をドメイン型へ変換する境界ヘルパー|[README](conv/README.ja.md)|
|`ctxhelper/`|Echo コンテキストへの値の設定・取得ヘルパー|[README](ctxhelper/README.ja.md)|

## 依存関係ルール

```mermaid
flowchart TB
    Controller --> Usecase
    Controller --> apperror
    Controller --> Presenter

    Controller -. forbidden .-> Domain
    Controller -. forbidden .-> Infrastructure
    Controller -. forbidden .-> Database
```

Controller は **Usecase を通してのみ下位層にアクセス**します。

### doc コメントは HTTP 語彙で書く

上記の禁止された依存の矢印は doc コメントにも適用されます。ハンドラの doc コメントは HTTP レベルの契約
（そのエンドポイントが何を返すか、失敗がどのステータスコードへ写像されるか、認証が必要か）を述べ、
テーブル名・カラム名・SQL 断片・トランザクション境界の所在は述べません。それらはそれを所有する層に属します
（[`internal/usecase/README.md`](../usecase/README.md) § Doc comments: interface vs implementation および
[`internal/infrastructure/README.md`](../infrastructure/README.md) § Doc comments may name technical
detail を参照）。ハンドラのメソッドは非公開の `server` に対するメソッドであるため `revive` は doc コメントを
要求しません。生成された `ServerInterface` が既に OpenAPI の summary を持ち、HTTP レベルで足す価値のある
具体が無いなら、言い換えを書くのではなく省略します。

## テスト戦略

ここに書くのは層の基準である。controller は inbound adapter の総称であり、全てが HTTP を話すわけではない。以下を適用する前に、駆動方式に対応するサブセクションを読むこと。独自の節を持つサブツリーはその節が観点を専有する: [`handler/`](handler/README.ja.md)、[`stream/`](stream/README.ja.md)、[`job/`](job/README.ja.md)、[`httpstack/`](httpstack/README.ja.md)、[`server/`](server/README.ja.md)。

### HTTP ハンドラ

ハンドラテストは usecase を mock し、Echo 経由でハンドラを駆動する（`testkit/testecho` + `testkit/testassert`）。ビジネスロジックは usecase 側にありここでは再テストしない。各ハンドラテストが検証する観点:

- HTTP I/O 変換 — リクエストの bind（path / query / body）→ usecase 入力、usecase 出力 → レスポンス DTO / status
- リクエスト validation 経路（OpenAPI / bind 失敗 → 400 等）
- `apperror` → HTTP status マッピング（usecase のエラーが適切な status / code として表出する）
- middleware が乗せる context — ハンドラが context から読む値（auth principal / request id / idempotency）

境界レベルの HTTP 結線（Router → Middleware → Handler → Presenter）は `internal/integration` の HTTP 境界テストで別途カバーする。

### ループ駆動の controller（`outbox/` / `worker/` / `realtime/`）

これらの adapter はリクエストではなく poll / consume ループで駆動されるため、上記の Echo・bind・HTTP status に関する記述は一切当てはまらない。usecase と boundary ポート（`usecase/boundary/clock`・`usecase/boundary/worker`・`usecase/boundary/realtime`）を mock し、ロガーは `logging.NewTestLogger`、トレーサは `observability.NewNoopTracerFactory`、実ブローカの代わりに `usecase/boundary/worker/testkit` の in-memory fake を使う。テスト対象はループそのものなので、ループとして駆動して検証する。

- **1 反復の効果** — 処理対象を見つけた poll はディスパッチし、見つからなかった poll は設定された間隔だけバックオフすること。検証は mock した sleeper を通して行い、テスト内で実際に sleep しないこと。
- **停止のセマンティクス** — context のキャンセルでループが終了して return すること、処理中の要素が各パッケージの文書化された drain 契約どおりに完了 or 放棄されること。`SupervisedRunner` 駆動の shutdown が依存しているのがこの分岐。
- **反復ごとのエラー処理** — usecase のエラーはバックオフして継続し、ループを殺さないこと。パッケージが文書化する失敗経路（retry / circuit / failure handler）は「panic しない」ではなくその分岐固有の outcome で検証すること。
- **設定値の clamp** — 受理範囲外の値が文書化された境界へ clamp され、その clamp が黙って行われず観測可能（ログ／実効設定への反映）であること。
