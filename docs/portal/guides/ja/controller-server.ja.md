# server

`server` は、アプリケーションの **HTTP サーバーを生成・設定する**とともに、Echo コンテキスト向けの HTTP リクエストログ／パラメータ抽出ユーティリティを提供するパッケージです。

Echo インスタンスと、それを配信する `ServerConfig` のタイムアウトを反映した `http.Server` を構築します。ミドルウェアの適用と DI ライフサイクル（起動・停止）への登録は別パッケージが担います（役割を参照）。

## 役割

- Echo インスタンスの生成（`NewAppServer`）
- HTTP サーバーの生成（`NewHTTPServer`）—— Echo を配信し、`ServerConfig` のタイムアウトを適用
- HTTP リクエストのログ入力の組み立て（`BuildHTTPRequestLogInput`）—— エラー／リカバリのログ経路の共通生成点
- Echo コンテキストからのパラメータ抽出ユーティリティ（`ExtractPathParams` / `ExtractQueryParams`）とレスポンス取得（`ResponseOf`）

このパッケージでは **ミドルウェアを直接定義しません**。ミドルウェアの適用は `internal/controller/httpstack` と `internal/di/server/extension` が担います。サーバーの起動・停止処理の DI ライフサイクル（`lifecycle.Registrar`）への登録は、本パッケージではなく `internal/di/server/hook` が担います。

## テスト戦略

本パッケージには性質の異なる 2 種類の対象があり、それぞれ別の方針でテストする。

### Echo コンテキストのユーティリティ

`BuildHTTPRequestLogInput` / `ExtractPathParams` / `ExtractQueryParams` / `ResponseOf` は実体のコンテキスト（`echo.New().NewContext(httptest.NewRequestWithContext(...), httptest.NewRecorder())`）に対して駆動する —— モックにする対象は無い。

- **空と非空** — パス値やクエリが無い場合は nil ではなく **非 nil の空マップ**を返すこと（呼び出し側は無条件に range する）。値がある場合は全件抽出され、同名クエリキーは全ての値を保持すること。
- **フィールドの写像** — `BuildHTTPRequestLogInput` が、指定イベント種別に対しリクエストの各属性を対応するログ入力フィールドへ写すこと。フィールド単位で検証する。`EventAt` は時刻由来のため、リテラル固定ではなくゼロ値でないことを検証する。
- **`ResponseOf` の unwrap 連鎖** — ミドルウェアに包まれたレスポンスライタでも *同一の* `*echo.Response` に辿り着くこと（`assert.Same`）。Echo へ辿れないライタでは `nil` を返すこと。各呼び出し側の縮退経路（`logging` / `redmetrics` / `forcejson` / `cookie` / `errorhandler`）はこの nil 分岐に依存しており、本番スタックではこの分岐が発生しないため、呼び出し側ではなく定義側の本テストで固定する。

### サーバーの構築

- `NewHTTPServer` が `ServerConfig` の各タイムアウトを `http.Server` へ写し、渡された Echo を `Handler` として保持すること。フィールド単位で検証する —— タイムアウトの写し違いは実行時に無症状のため。
- リッスン・配信・graceful shutdown は **ここでは検証しない**。それらは [`internal/di/server/hook`](../../di/server/hook/README.ja.md) のライフサイクルフックの担当。

## 注意点

- `NewAppServer` で生成した Echo インスタンスには、後続の extension でミドルウェアが適用される —— 本パッケージは **ミドルウェアを直接定義しない**
- Echo v5 はサーバの起動 / 停止を `echo.StartConfig` へ集約するが、そのブロッキングモデルは DI コンテナの起動 / 停止フック分離と噛み合わないため、本プロジェクトは `http.Server` を自前で持つ（`NewHTTPServer`）。リクエストのタイムアウトもそこに置く（[ADR-0022 (echo-http-framework)](../../../docs/adr/0022-echo-http-framework.ja.md) 参照）
- Echo v5 の `Context.Response()` は `http.ResponseWriter` を返すため、Echo 固有のステータスや `Before` / `After` フックが必要な場合は `ResponseOf` を使う
- ログには `logging.Logger` を使用し、zap の直接利用は禁止（sealed layer）
- Graceful shutdown の Timeout は `ServerConfig` に従う —— 設定が正しいことを確認すること
- かつて本パッケージで公開していた「パニックが上流で復旧済みか」のフラグ（`MarkRecovered` / `IsRecovered`）は `internal/controller/ctxhelper` に移し、typed helper の `SetRecoveredToEcho` / `GetRecoveredFromEcho` として提供しています。利用側は本パッケージ経由ではなく `ctxhelper` を直接参照してください
