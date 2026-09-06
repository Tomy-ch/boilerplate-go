# `Config` パッケージ

アプリケーション全体で利用するコンフィグファイルを定義しておくディレクトリです。

## 実装について

このパッケージは、環境変数からアプリケーション設定を読み取り、型付きの構造体としてアプリケーション全体に提供する責務を持ちます。主要な実装の流れは次のとおりです。

- `Loader`（テスト用の `Loader` を含む）に環境変数をパースし、`env.ParseAs[Loader]()` によって値を読み込みます（実装ファイル: `loader.go` / `envspec.go`）。
- 読み込んだ `Loader` の値を `Config` 型に変換し、内部で必要に応じたバリデーション（`validateConfig()`）を行います（実装ファイル: `config.go`, `model.go`）。
- 生成された `*Config` は不変（setter を公開しない）として扱い、DI コンテナに登録するためのセットアップ関数（`SetUpConfig`）を通じてアプリケーションに注入されます（DI 関連: `internal/di/module/config.go`）。

設計上のポイント:

- 設定は初期化時にまとめて読み取り、ランタイム中に値を変更しない（イミュータブルに扱う）。
- 必須値の欠落は `validateConfig()` で検出し、起動失敗（明示的なエラー返却）させることで不正な状態で稼働しないようにしています。
- テスト用のヘルパーやモック（`config_testing_mock.go`, `config_testing_setter.go`）を用意しており、テスト環境で環境変数を差し替えて `New()` の挙動を検証できます。

## Config Loading フロー

アプリケーション起動時の設定読み込みの流れは次の通りです。

```mermaid
flowchart TB
    Env[".env files"]
    Load["Load (godotenv)"]
    Parse["env.ParseAs[Loader]()"]
    Validate["validateConfig()"]
    Config["Config struct"]
    Sub["SubConfig Provider (NewServerConfig など)"]
    DI["DI (Uber Fx)"]

    Env --> Load --> Parse --> Validate --> Config --> Sub --> DI
```

### 各ステップの役割

- **Load()**
  - 埋め込み（`go:embed` の `root.FS`）の `env/.env` を godotenv で読み込み、まだ設定されていない変数のみをセットします（実行時に注入済みの環境変数を優先）。`env/.env.<ENV>` は読み込みません。

- **env.ParseAs[Loader]**
  - `envspec.go` に定義された `Loader` 構造体へ環境変数をマッピングします。

- **validateConfig()**
  - ポート範囲、CIDR、タイムアウト値などの妥当性を検証します。
  - 不正な設定がある場合は起動時にエラーを返します。

- **Config struct**
  - `Loader` の値を内部構造体 (`model.go`) に変換します。
  - 外部から直接変更できない **immutable な設定オブジェクト**として扱います。

- **SubConfig Provider**
  - `NewServerConfig` や `NewDatabaseConfig` などの関数を通じて
  - 必要な設定のみを各コンポーネントに注入します。

- **DI (Uber Fx)**
  - `internal/di/module/config.go` で `fx.Provide` に登録され
  - アプリケーション全体で利用されます。

## 設計原則

このConfigパッケージは、次の設計原則に基づいて実装されています。

- **Configuration is immutable**
  - 設定は起動時に一度だけ読み込まれ、ランタイム中に変更されません。

- **Configuration is loaded only at startup**
  - `.env` → `Loader` → `validateConfig()` → `Config` の順に初期化されます。

- **Domain / Usecase は環境変数に依存しない**
  - 環境変数の解釈は Config 層に閉じ込めます。

- **Typed SubConfig を通じて設定を取得する**
  - `NewServerConfig` や `NewDatabaseConfig` などの provider を通じて、
    必要な設定のみを各コンポーネントに渡します。

これにより、アプリケーションの設定依存を最小限にし、テスト容易性と保守性を高めています。

## メインで利用しているライブラリ

- `github.com/caarlos0/env/v11` — 環境変数を構造体へマッピングするために使用しています（タグベースで `env:"KEY,required"` を指定可能）。
- DI 登録には Uber Fx のパターンを採用しているため、設定値は `fx.Provide` を通じて注入されます（DI 実装: `internal/di/module/config.go`）。
- テストでは `testify/require` 等の断言ライブラリと、プロジェクト内の `config_testing_*` ヘルパーを併用して、環境変数ベースの振る舞いを確実に検証しています。

## 必要度

以下は現実運用での重要度と期待される影響です。

### 本番運用での必須度

- 必須度: 高
  - 理由: データベース接続情報、外部サービスの認証情報、リスニングポート、CORS 設定、セキュリティ設定（HSTS 等）など、稼働に必須な値が環境変数で提供されるため、正確に読み取れないと起動失敗や重大な障害につながります。
  - 対策: 必須フィールドは `validateConfig()` で起動前に検証し、欠落時は明示的にエラーを返す設計にしてください。

### 開発/テスト運用での必須度

- 必須度: 高（ただしデフォルトやモックで代替可能）
  - 理由: テストでは特定の ENV をセットして挙動を検証するため、環境変数の管理が重要です。プロジェクト内にテスト用ヘルパーがあるため、CI やローカルテストでは `t.Setenv` と組み合わせて再現可能にします。
  - 例: `config_testing_setter.go` を使ってテスト用の設定を注入し、`New()` が正しくバインドすることを確認します。

### 無効化した場合の影響

- 影響の範囲: 軽微〜致命的（設定項目による）
  - 軽微: ログレベルやデバッグフラグなど、一部のオプションはデフォルト値で安全に代替できます。
  - 致命的: DB 接続情報や外部 API キーが欠落すると起動失敗やランタイムエラーを引き起こします。
  - 推奨対応: 本番での運用前に `go build` と `go test ./...` を実行し、`validateConfig()` によるチェックが通ることを確認してください。CI パイプラインで必須 ENV を検証する習慣を付けると安全です。

### 新たなカテゴリ(例: AWSやGCPなど)を追加する方法

1. `model.go`に新しいカテゴリ用の公開構造体（例: `AWS`や`GCP`など）を追加してください。  
    - 例:

    ```go
    type aws struct {
        accessKey string
        secretKey string
        region    string
    }
    ```

2. 追加したカテゴリの構造体を `Config` 構造体にフィールドとして追加してください。
    - 例:

    ```go
    type Config struct {
        server server
        aws    aws // 新しいカテゴリの追加
    }
    ```

3. `envspec.go` に新しいカテゴリの構造体を定義してください。
    - 各フィールドには `env` タグを付与し、必要に応じて `required` や `envSeparator` なども指定してください。
    - `model.go` で定義した構造体の公開構造体を `envspec.go` にも追加してください。
    - 例:

    ```go
    type AWS struct {
        AccessKey string `env:"AWS_ACCESS_KEY,required"`
        SecretKey string `env:"AWS_SECRET_KEY,required"`
        Region    string `env:"AWS_REGION,required"`
    }
    ```

4. 追加したカテゴリの構造体を `Loader` 構造体にフィールドとして追加してください。
    - 例:

    ```go
    type Loader struct {
        Server Server
        AWS    AWS // 新しいカテゴリの追加
    }
    ```

5. `config.go` の `New` 関数で `env.ParseAs[Loader]()` を呼び出すことで、環境変数から`Loader`に自動的に値がバインドされます。
    - 必要に応じて、追加カテゴリのバリデーション処理を `validateConfig()` 関数内で実装してください。
    - 最後に、`Loader` から `Config` への変換を行い、`Config` 構造体を返すようにしてください。

    ```go
    func New() (*Config, error) {
        cfg, err := env.ParseAs[Loader]()
        if err != nil {
            return nil, xerrors.Join(ErrFailedToParseConfig, err)
        }

        if err := validateConfig(cfg); err != nil { // バリデーション処理を追加はこの関数内で行います
            return nil, err
        }

        return &Config{
            server: ServerConfig{
                host: cfg.Server.Host,
                port: cfg.Server.Port,
            },
            aws: AWSConfig{
                accessKey: cfg.AWS.AccessKey,
                secretKey: cfg.AWS.SecretKey,
                region:    cfg.AWS.Region,
            },
        }, nil
    }
    ```

6. 必要に応じて、カテゴリごとのgetterメソッドを実装し、外部から値を取得できるようにしてください。
    - setterメソッドの作成は禁止です。config.goの目的は、環境変数からの値の取得とバインドに限定されているため、設定値を変更することは想定されていません。
    - 例:

    ```go
    func (c *Config) AWSAccessKey() string {
        return c.aws.accessKey
    }

    func (c *Config) AWSSecretKey() string {
        return c.aws.secretKey
    }

    func (c *Config) AWSRegion() string {
        return c.aws.region
    }
    ```

7. `config_test.go` に追加したカテゴリのテストケースを追加してください。
    - テストケースでは、環境変数を設定し、`New()` 関数を呼び出して値が正しくバインドされていることを確認します。
    - 例:

    ```go
    func TestNewAWSConfig(t *testing.T) {
        expectedAccessKey := "test_access_key"
        expectedSecretKey := "test_secret_key"
        expectedRegion := "us-west-2"

        t.Setenv("AWS_ACCESS_KEY", expectedAccessKey)
        t.Setenv("AWS_SECRET_KEY", expectedSecretKey)
        t.Setenv("AWS_REGION", expectedRegion)

        cfg, err := New()
        require.NoError(t, err)

        require.Equal(t, expectedAccessKey, cfg.aws.accessKey)
        require.Equal(t, expectedSecretKey, cfg.aws.secretKey)
        require.Equal(t, expectedRegion, cfg.aws.region)
    }
    ```

    - validateConfig関数内でのバリデーションのテストも追加してください。

8. 既存の `NewHogehogeConfig` と同様に、DI が `*config.Config` を受け取り、公開型（`*config.AWSConfig`）を返すシグネチャにしてください。

```go
// internal/config/aws_config.go (例)
package config

type AWSConfig struct {
    AccessKey string
    SecretKey string
    Region    string
}

// NewAWSConfig は DI から渡された *config.Config を受け取り、
// サービスで使う公開型 *config.AWSConfig を返します。
func NewAWSConfig(cfg *Config) *AWSConfig {
    return &AWSConfig{
        AccessKey: cfg.aws.accessKey,
        SecretKey: cfg.aws.secretKey,
        Region:    cfg.aws.region,
    }
}
```

ポイント:

- フィールド名や構造体名はプロジェクトの命名規則に従ってください。
- DI に provider を追加したら、その型を受け取るコンポーネント（例: AWS クライアントの factory）を DI コンテナに登録してください。
- 追加漏れや型の不一致があるとビルド時にエラーになるため、`go build` や `go test ./...` で確認してください。

## テストサポート

### テストヘルパー

|関数|ファイル|説明|
|---|---|---|
|`MockConfigForTest(t)`|`config_testing_mock.go`|全項目にデフォルト値を設定したテスト用 `*Config` を生成|
|`NewTestLocation(t)`|`test_kit.go`|テスト用タイムゾーン `*time.Location` を生成|
|`ResolvedAuthIssuer(t)`|`test_kit.go`|この実行環境の `AUTH_ISSUER` を解決（実行時 env が先、無ければ埋め込み `env/.env`）|
|`EnsureRepoRootAndEnv(t, env)`|`test_kit.go`|リポジトリルートに移動し、ENV 環境変数を設定|

### テスト用 Setter

`config_testing_setter.go` に定義されたメソッドで、テスト中に SubConfig の値を一時的に変更できます。`t.Cleanup` で自動的に元の値に戻ります。

**本番コードでは使用しないでください。**

|メソッド|対象 SubConfig|
|---|---|
|`SetApplicationMode`|`ApplicationConfig`|
|`SetApplicationEnv`|`ApplicationConfig`|
|`SetApplicationLogLevel`|`ApplicationConfig`|
|`SetServerPort`|`ServerConfig`|
|`SetObservabilityMaskedDBQueryArgs`|`ObservabilityConfig`|
|`SetObservabilityTracesExporter`|`ObservabilityConfig`|
|`SetObservabilityMetricsExporter`|`ObservabilityConfig`|
|`SetObservabilityLogsExporter`|`ObservabilityConfig`|
|`SetObservabilityOTLPProtocol`|`ObservabilityConfig`|
|`SetObservabilityOTLPEndpoint`|`ObservabilityConfig`|
|`SetDatabaseHost`|`DatabaseConfig`|
|`SetDatabaseName`|`DatabaseConfig`|
|`SetMetricsPort`|`MetricsConfig`|
|`SetHealthListenAddr`|`WorkerConfig`|
|`SetMaxConns`|`DBConnectionConfig`|
|`SetCIDR`|`SecurityConfig`|
|`SetOutboxBatchSize`|`OutboxConfig`|
|`SetOutboxEndpoint`|`OutboxConfig`|
|`SetOutboxPollInterval`|`OutboxConfig`|
|`SetOutboxErrorBackoff`|`OutboxConfig`|
|`SetOutboxPublisher`|`OutboxConfig`|
|`SetOutboxQueue`|`OutboxConfig`|
|`SetConsumerQueue`|`ConsumerQueueConfig`|
|`SetAuthIssuer`|`AuthConfig`|
|`SetAuthAudience`|`AuthConfig`|
|`SetSameSite`|`SecureCookieConfig`|
|`SetDomain`|`SecureCookieConfig`|

### テスト例外（ユニットで完全被覆を求めないファイル）

本パッケージの大半（SubConfig の getter・`New()` のバインド・テストセッター）はユニットで
ほぼ 100% 被覆を維持する想定です。以下は**意図的な例外**で、未被覆部分は読み込み /
composition 境界のエラー分岐であり、失敗注入なしには通せません。実経路は boot-check CI
（`app-di-startup-check` / `worker-boot-check` / `job-boot-check` /
`outbox-relay-boot-check`。実バイナリを `SetUpConfig`
経由で起動）で E2E 検証済みです。

|ファイル|関数|ユニット非対象の理由|
|---|---|---|
|`loader.go`|`Load`|残る分岐は**埋め込み** env ファイルの I/O 失敗（`root.FS.ReadFile` / `godotenv.Parse` / `os.Setenv`）。実行時にはまず失敗せず、被覆には失敗する FS / env の注入が必要。|
|`setup.go`|`SetUpConfig`|`cmd/`（それ自体カバレッジゲート対象外）から呼ばれる composition-root の glue。未被覆は `Load()` 失敗時の early-return のみで、起動は boot-check CI が担保。|

> これらの行を塗るためだけの不自然な失敗注入テストは**追加しない**こと。これらの関数が
> （I/O エラー配線ではない）実際の分岐ロジックを持つようになった場合は、他と同様に
> ユニットテストで担保すること。
>
> **ガバナンス:** カバレッジ例外は**任意に追加しない**。新規エントリはアーキテクト等の
> **適切な承認者の承認を得た場合に限り**本節へ記録する。

## 注意点

- セキュリティ上の理由から、機密情報（例: APIキーやパスワード）は環境変数で管理し、コード内にハードコーディングしないでください。

## テスト戦略

config はプロセスの環境変数とファイルシステムを読む唯一の場所であり、そのテストは並列化必須という原則の例外側になる。

- **バリデータごとの境界** — 各 `validate*` 関数は自身の `TestXxx` を持ち、強制する各ルールについて両側（受理される値と、その具体的な拒否）を検証する。正常系しか通らないバリデータは設定ミスを実行時まで持ち越すが、それを防ぐために存在しているのがこれらの関数である。
- **環境変数 / CWD の変更は必然的に直列** — `t.Setenv` / `t.Chdir` を呼ぶ対象（config のロード処理と `EnsureRepoRootAndEnv` 等のテスト支援ヘルパ）は `t.Parallel()` を呼べない。`docs/testing-conventions.md` §3 に従い `//nolint:paralleltest` と一行の理由を付す。`os.Environ` を直接いじって回避しないこと。
- **ロードの優先順位** — 値の供給元が複数あり得る場合、値が生成されたことではなく **どちらが勝つか** を検証する。
- **埋め込み env ファイルは `envspec.go` と突き合わせる。写経しない** — `.env` は何もコンパイルされないデータであり、`envDefault` 無しで宣言されたのにファイルに無い変数と、改名で取り残されたキーは、いずれも振る舞いのテストからは見えない。`TestEmbeddedEnvConsistency` は `Loader` の構造体タグから reflect でキー集合を導出し、パース結果と **キーの存在** を双方向に突き合わせる。集合は必ず導出すること — 手書きのキー一覧は検証対象のファイルの複製でしかなく、一緒に乖離する。
- **値の乖離はキー単位で塞ぐ。一括では塞げない** — `config_testing_mock.go` の期待値は「どのキーを読んだか」をテストが証明できるよう意図的に別の番兵値になっている（`APP_NAME` は埋め込みファイルが持つ値ではなく番兵の `TestApp`）ため、値を一括比較することはできない。よって値はキー単位で、しかも期待値がファイルを写すことを意図しているものについてのみ固定する — 現状は `OBS_TARGET_STATUS_CODES` だけであり、他のキーの値は依然として無検出で乖離しうる。対象は埋め込みファイルだけ、すなわちローカルでは `env/.env`、`make materialize-env` 下の CI では `env/.env.ci` である。deploy 系（`dev` / `stg` / `prd`）は設計上異なる値を持ち、テスト時の埋め込みファイルになることはない。その差異が設計なのか伝播漏れなのかは本パッケージの問いではなく、`env/README.md` が文書化する環境別ポリシーに対して `internal/architest` の `TestEnvTargetStatusCodesPolicy` が env ファイル群を横断して決着させる。
- **埋め込み env は読み込むだけでなく素性を捕捉する** — `Load` は実行時 env のマージ**前**に埋め込みの `APP_ENV` を `embeddedAppEnv` へ記録し、production モードの素性ガードは実効値ではなくこの捕捉値を読む。よって検証は、実行時 `APP_ENV` に別値を置いた状態で、期待値を埋め込みファイルから導出して行う。読み込み後の値を読み返すテストでは、焼き込まれた素性と注入された値を区別できず、ガードが唯一区別しようとしているものを検証できない。
- **テストダブルは本パッケージが提供するもの** — 他層は `Config` を手で組まず `MockConfigForTest(t)` と `Set*(t, …)` セッタを使う。セッタは `t.Cleanup` で復元し、その復元自体も検証すべき契約である。一覧は [テストサポート](#テストサポート) を参照し、ここに再掲しないこと。
