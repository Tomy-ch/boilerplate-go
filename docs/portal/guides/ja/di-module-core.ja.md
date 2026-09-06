# core モジュール

`internal/di/module/core` は、HTTP スタックで共通利用される **コアコンポーネントの DI モジュール群**を提供するパッケージです。

各モジュールは `fx.Option` を返し、対応するコンポーネントを DI コンテナに登録します。

## モジュール一覧

|関数|ファイル|提供するコンポーネント|
|---|---|---|
|`AuthnModule()`|`auth.go`|認証（Authenticator + Auth コントローラ）|
|`BasicAuthModule()`|`basicauth.go`|メトリクスエンドポイント用 Basic 認証バリデータ|
|`SecurityCookieModule()`|`security_cookie.go`|Cookie セキュリティ属性の設定|
|`SkipperModule()`|`skipper.go`|OpenAPI バリデーションの ops エンドポイントスキップ|
|`ValidatorModule()`|`validator.go`|OpenAPI スキーマバリデータ|
|`RedactionModule()`|`redaction.go`|OpenAPI の securityScheme から導出する、query 資格情報のログ秘匿|

## 設計方針

- 各モジュールは1ファイル = 1モジュールで分離
- 内部実装は `internal/controller/httpstack` 等のコンストラクタを `fx.Provide` でラップするだけ
- ビジネスロジックを含まない

## Test Strategy

ここのモジュールはいずれも起動に実インフラを必要としません。そのため上位層では負担できない 2 段目の検証が可能で、各モジュールは 2 本の sibling テストを持ちます。

- **`Test<Module>_GraphIsValid`** — [`../README.md`](../README.ja.md) が宣言する層のベースライン。`fx.ValidateApp` がモジュールの出力型を、コンストラクタやライフサイクルフックを **実行せずに** 解決します。何も実行されないため、この段は素のモックだけでモジュールの **全出力型** を要求できます。`AuthnModule` が `IdentityResolver` と `AuthenticationFunc` をここで解決しているのがその例で、実際に起動するテストでは使用可能なトレーサと DB ドライバが無ければ到達できません。検証は **両側から** 行います。`異常系` はモジュールを外して当該型が解決できなくなることを要求し、これがグラフ中の別の何かではなくそのモジュールこそが供給元であることを示します。この半分が無いと、引数を取らないコンストラクタだけのモジュールには失敗し得る要素が残りません。
- **`Test<Module>`** — 最小の `fx.New` アプリを起動し、`fx.Populate` した提供コンポーネントを取り出して、コンストラクタが実際に走り使える値を生成することを担保します。大半のモジュールはコンポーネントが非 nil であることだけを assert します。コンストラクタの結果が 1 通りしか無いためです。環境ごとに実装を選ぶ provider を持つ `AuthnModule` だけは、選ばれた値（`local.New()`）まで assert します。

2 段目が成立するのは、**ここのモジュールが起動に実インフラを必要としないから**です。基準はディレクトリではなくこの性質にあります。クロージャが実 DB やネットワーク接続を要求するモジュールは上位に置き、そこでは `fx.ValidateApp` だけが戦略になります。

6 つのうち 5 つは `internal/controller/httpstack` 等のコンストラクタを薄くラップするだけです。`AuthnModule` だけが例外で、`provideAuthenticator` は環境ごとに分岐し `httpclient.Client` を受け取ります。固有のロジックを持つ provider 本体（`provideAuthenticator` / `provideJWKSAuthenticator`）は、[`../../README.md`](../../README.ja.md) の DI 層ベースラインに従って直接ユニットテストします。グラフ検証はどちらにも到達せず、環境ゲートの拒否ケースこそが要点だからです。

## 注意点

- モジュールの追加・削除は `internal/di/server.go` の `applicationCoreOptions` から参照を変更する必要がある
