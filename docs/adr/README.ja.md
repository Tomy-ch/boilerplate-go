# アーキテクチャ決定記録（ADR）

このディレクトリはプロジェクトの **アーキテクチャ上の決定** を保持する。1 ファイル = 1 つの不変な記録、[MADR-lite](https://adr.github.io/madr/) 形式。

ADR は 1 時点における単一の決定を記録する: コンテキスト、検討した選択肢、選んだもの、その影響。決定を上書きするとは ADR を編集することを **意味しない** — `Status` が `accepted` の *新しい* ADR を追加し、古い ADR を `superseded` にマークすることを意味する。*なぜかつて X を選んだのか* という記録は保存される。

## 何が対象か（そして何が対象でないか）

| 種別 | 例 | 置き場所 |
| --- | --- | --- |
| **decision** — 持続的な影響を持つ選択肢間の選択 | 「オニオンアーキテクチャを採用する」 | このディレクトリ（ADR） |
| **exclusion** — 意図的に「X をしない」という決定 | 「アプリ内レートリミッターなし」 | このディレクトリ（ADR） |
| **rule** — 日々強制される制約 / 決定の帰結 | 「コントローラーはインフラストラクチャをインポートしてはならない」 | `docs/rules.md`（ADR へのリンクを付ける場合がある） |
| **inventory** — コードと共に変化するカタログ | 直接依存関係テーブル | `docs/reference/dependencies.md`（生きたドキュメント） |

依存関係インベントリは ADR **ではない**: `go.mod` を追跡して継続的に変化し、不変の記録とは正反対である。依存関係を選定する *ポリシー* は決定（ADR）であり、その *一覧* は生きたリファレンスである。

## 規約

- **ファイル名**: `NNNN-kebab-title.md`、ゼロパディング 4 桁。supersession では番号は空かない——supersede された ADR は番号もファイルも保持し続ける——ので、退いた決定の番号が別の決定へ渡ることはない。
- **ファイル名**: `NNNN-kebab-title.md`（4 桁ゼロ埋め）。supersede しても番号は解放されない——superseded な ADR は番号もファイルも保持する——ため、退役した決定の番号が別の決定へ渡ることはない。
- **順序**: 新しい ADR を次の未使用番号へ末尾追記する。番号は安定した参照であり、対象の決定はタイトル、タグ、ログから探す。これは [AWS](https://docs.aws.amazon.com/prescriptive-guidance/latest/architectural-decision-records/adr-process.html) と [Microsoft](https://learn.microsoft.com/en-us/azure/well-architected/architect-role/architecture-decision-record) が説明する一般的な ADR 運用である。すなわち、採択済みの記録を保ち、テーマ上の整然さのために履歴順を並べ替えるのではなく、後続 ADR によって置換を表現する。
<!-- boilerplate-only:begin -->
- **正本の順序例外**: このリポジトリがボイラープレート正本として配布される間は、番号を発見順ではなく依存関係 / 基礎的な順序（原則 → コントラクト → レイヤー → サブシステム → 横断関心事 → exclusion）に従わせる。この順序を保つため、新しい ADR を**テーマ上の位置へ挿入し、後続の全番号を +1 シフト**してよい（純粋なリナンバー: シフトされた各 ADR の内容は不変で、リポジトリ内の全参照は同一変更内で更新する）。セットアップでこの例外は削除される。正本での再採番をまたいでも、kebab タイトルは安定識別子として残る。
<!-- boilerplate-only:end -->
- **ステータスライフサイクル**: `proposed` → `accepted` → (`superseded` | `deprecated`)。
- **誰が決定の記録を変更してよいか**: `accepted` な ADR を supersede する / deprecate することは、このリポジトリのアーキテクトまたはテックリードの判断であり、変更ごとに取る。`accepted` な ADR が実装と食い違っていると気づいたことは、その人に提起する理由であって変更の許可ではない——**実装側が誤りである可能性が同じだけある**。過去の変更は次の変更の常時承認にはならない。
- **テンプレート**: [`template.ja.md`](template.ja.md) をコピーする。
- **メタ**: [`0000-record-architecture-decisions.ja.md`](0000-record-architecture-decisions.ja.md) は ADR の使用とこの分類の決定を記録する。
- **翻訳**: 各 ADR は隣に `<name>.ja.md` を持つ（`canonicalize-doc` フローを経由）。
- **上流の逸脱**: 本リポジトリがボイラープレートとして配布される間は上記への例外の下で運用されており、それは [`docs/get-started/boilerplate-only-conventions.md`](../get-started/boilerplate-only-conventions.ja.md) に記録されている。そこから作られたプロジェクトには適用されない。 <!-- boilerplate-only:line -->

## ログ

`docs/decisions.md` およびリポジトリ全体の潜在的な決定から、すべての決定が ADR として具体化された。

<!-- boilerplate-only:begin -->
正本の番号付けは依存関係 / 基礎的な順序に従う（原則 → コントラクト → HTTP → 永続化 → DI/設定 → 非同期サブシステム → 可観測性 → ツールチェーン/CI → プロセス → バイナリ/デプロイ → exclusion）。

Exclusion ADR（意図的な「X はしない」）は、リポジトリセットアップフローが列挙できるよう `setup-review` タグを持つ。セットアップが済めばこのタグを読む者は居ない。

<!-- boilerplate-only:end -->

| # | 決定 | ステータス |
| --- | --- | --- |
| [0000](0000-record-architecture-decisions.ja.md) | アーキテクチャ上の決定を ADR として記録する | accepted |
| [0001](0001-avoid-lock-in.ja.md) | ロックイン回避を設計原則として採用する | accepted |
| [0002](0002-onion-architecture.ja.md) | 実用的なオニオンアーキテクチャを採用する | accepted |
| [0003](0003-interface-based-decoupling.ja.md) | 疎結合のためにインターフェースで境界を定義する（DIP） | accepted |
| [0004](0004-modular-monolith.ja.md) | モジュラーモノリスを採用する（マイクロサービスは非目標） | accepted |
| [0005](0005-driving-adapters-not-split-axis.ja.md) | REST / Worker / Job はドライビングアダプター、サービス分割の軸ではない | accepted |
| [0006](0006-structural-safety-via-tooling.ja.md) | ツールと CI で構造的安全性を強制する（depguard） | accepted |
| [0007](0007-agents-md-operational-contract.ja.md) | AI-first / manual-compatible な開発と、運用契約としての AGENTS.md | accepted |
| [0008](0008-agent-environment-alignment.ja.md) | エージェント環境を宣言された検査可能な性質へ整合させ、閉じたループで改善する | accepted |
| [0009](0009-long-running-agent-state.ja.md) | 永続的なエージェント状態を所有する正本の形に保つ | accepted |
| [0010](0010-development-window-as-feedback-unit.ja.md) | フィードバックループが観測する単位を開発の窓とする | accepted |
| [0011](0011-docs-as-canonical-source.ja.md) | ドキュメントを正典ソースとする戦略 | accepted |
| [0012](0012-openapi-first.ja.md) | API 契約を OpenAPI ファーストで定義する | accepted |
| [0013](0013-redocly-modular-spec-pipeline.ja.md) | 仕様をモジュラーな Redocly ファイルで作成し、バンドルしてから生成する | accepted |
| [0014](0014-oapi-codegen-strict-server.ja.md) | oapi-codegen の strict-server モードでタグ/ハンドラーごとに生成する | accepted |
| [0015](0015-retain-generated-openapi.ja.md) | バンドル済み openapi.gen.yaml をコミット済みのクロスリポジトリ契約アーティファクトとして保持する | accepted |
| [0016](0016-spec-driven-request-validation.ja.md) | リクエスト検証と認証を実行時に仕様から強制する。レスポンスは検証しない | accepted |
| [0017](0017-validation-value-authority.ja.md) | バリデーションのビジネス有効性における唯一の権威をドメイン層に定める | accepted |
| [0018](0018-boundary-value-ownership.ja.md) | OpenAPI はワイヤー契約であってドメインルールではない。リクエストはドメインのサブセット、ドメインはレスポンスのサブセット | accepted |
| [0019](0019-search-query-parameter-shape.ja.md) | 複数値フィルタはパラメータ名を繰り返し、自由入力はスカラのままにする | accepted |
| [0020](0020-metrics-endpoint-auth-exception.ja.md) | /metrics は認証例外 — OpenAPI 検証の外に置き、独立した BasicAuth ミドルウェアで保護する | accepted |
| [0021](0021-optional-authentication-fail-closed.ja.md) | 任意認証を許し、認証に失敗したリクエストは通さない | accepted |
| [0022](0022-echo-http-framework.ja.md) | HTTP フレームワークとして Echo を採用する | accepted |
| [0023](0023-priority-ordered-middleware-chain.ja.md) | ミドルウェアチェーンを優先順位付きのデータ駆動リストとして構築する | accepted |
| [0024](0024-outbound-http-resilience.ja.md) | アウトバウンドHTTPレジリエンス基盤の提供（リトライ / サーキットブレーカー / リトライバジェット / デュアルタイムアウト） | accepted |
| [0025](0025-egress-ssrf-guard.ja.md) | アウトバウンドHTTPに対するエグレスSSRF / ダイヤルガードセキュリティポスチャの採用 | accepted |
| [0026](0026-sql-first-data-access.ja.md) | SQLファーストのデータアクセス | accepted |
| [0027](0027-sqlc-type-safe-sql.ja.md) | sqlcによる型安全なSQLアクセスの生成 | accepted |
| [0028](0028-merged-dml-schema-as-sqlc-input.ja.md) | マージされたDMLおよびダンプされたスキーマをsqlcの単一入力として使用する | accepted |
| [0029](0029-append-only-immutable-migrations.ja.md) | マイグレーションを追記専用かつイミュータブルとして扱う | accepted |
| [0030](0030-sequential-migration-ids.ja.md) | CIで強制するギャップ・ペアチェックを伴う6桁連番マイグレーションIDの使用 | accepted |
| [0031](0031-master-data-via-migration.ja.md) | マスターデータをマイグレーション経由で投入する。トランザクショナルシードを本番から除外する | accepted |
| [0032](0032-lightweight-cqrs.ja.md) | 軽量CQRSの採用 — 書き込みにRepository、読み込みにQueryService | accepted |
| [0033](0033-system-cqrs-dml-category.ja.md) | CQRSの分割の外に位置する第4のDMLカテゴリとしてsystem_cqrsを導入する | accepted |
| [0034](0034-commandservice-atomicity-criterion.ja.md) | 集約を跨ぐ操作の判定 — 既定は usecase + outbox、ガードが陳腐化してはならないなら同期ロック、CommandService は単一トランザクション原子性のときだけ | accepted |
| [0035](0035-transaction-retry-idempotent-callers.ja.md) | シリアライゼーション競合時はトランザクションをリトライする。呼び出し元は冪等性を保証しなければならない | accepted |
| [0036](0036-ordered-pessimistic-row-locks.ja.md) | 競合する書き込みを、守る条件より前に取る単一順序の悲観行ロックで直列化する | accepted |
| [0037](0037-uuidv7-identifiers.ja.md) | すべてのエンティティ主キーに UUIDv7（時刻順）識別子を使用する | accepted |
| [0038](0038-two-scale-quantity-model.ja.md) | 量を 2 つのスケールで保持する — 精度は正確な十進、決済は最小単位の整数 | accepted |
| [0039](0039-domain-lexicon.ja.md) | 集約横断の値オブジェクトはキュレートされたドメイン語彙集に置く | accepted |
| [0040](0040-uber-fx-di.ja.md) | 依存性注入とライフサイクル管理に Uber Fx を採用する | accepted |
| [0041](0041-fx-neutral-di-abstraction.ja.md) | ニュートラルな DI 抽象（Registrar / Shutdowner）の背後に fx を封じ込める | accepted |
| [0042](0042-env-gated-wiring.ja.md) | DI を通じて環境ごとに実装を切り替える（環境ゲート結線） | accepted |
| [0043](0043-subsystem-typed-config-loaders.ja.md) | サブシステムスコープの envPrefix 型付き設定ローダー | accepted |
| [0044](0044-config-default-vs-required-governance.ja.md) | ガバナンス: コードデフォルト（不変）対ファイル必須（可変） | accepted |
| [0045](0045-immutable-fail-fast-config.ja.md) | 設定は不変、起動時に 1 回だけロード、フェイルファスト | accepted |
| [0046](0046-embedded-self-contained-binary.ja.md) | go:embed で設定（.env）とマイグレーションをバンドルし、自己完結型バイナリを実現する | accepted |
| [0047](0047-apperror-protocol-agnostic-errors.ja.md) | プロトコル非依存の集約エラー分類 (apperror) | accepted |
| [0048](0048-error-metadata-code-message-details.ja.md) | apperror の上に載せるプロトコル中立なエラーメタ情報（code / message / details） | accepted |
| [0049](0049-error-details-opt-in-gate.ja.md) | スキーマ分割によるエラー details の opt-in ゲート（0043 を精緻化） | accepted |
| [0050](0050-broker-agnostic-worker-scaffold.ja.md) | ブローカー非依存のプル・アック型ワーカースキャフォールド | accepted |
| [0051](0051-out-of-scope-push-streaming-brokers.ja.md) | プッシュ型ブローカーとストリーミングログ基盤はワーカーポートのスコープ外 | accepted (exclusion) |
| [0052](0052-sqs-adapter-opt-in.ja.md) | SQS アダプターはオプトインであり、デフォルトバイナリにリンクしない | superseded by [0052](0053-broker-sdk-isolation-measured-as-coupling.ja.md) |
| [0053](0053-broker-sdk-isolation-measured-as-coupling.ja.md) | ブローカー SDK の分離はリンクではなく結合で測る | accepted |
| [0054](0054-transactional-outbox.ja.md) | トランザクショナルアウトボックス — ビジネストランザクション内でイベントを発行する | accepted |
| [0055](0055-at-least-once-outbox-poll.ja.md) | ポーリングによる少なくとも1回のデリバリー（トランスポートレベルのリトライを無効化） | accepted |
| [0056](0056-skip-locked-outbox-relay.ja.md) | SELECT FOR UPDATE SKIP LOCKED を使った単一トランザクションリレー（複数インスタンス間で安全） | accepted |
| [0057](0057-message-id-idempotency-propagation.ja.md) | アウトボックスの message_id をレシーバーの Idempotency-Key として伝播する | accepted |
| [0058](0058-outbox-dead-on-permanent-error.ja.md) | outbox 行は恒久的なエラーで dead になり、一時的な失敗は message ごとの backoff で retry する | accepted |
| [0059](0059-outbox-retention-gc.ja.md) | 発行済み行の 7 日間保持 GC（10,000 件単位のバッチ） | accepted |
| [0060](0060-publisher-http-profile-isolation.ja.md) | パブリッシャーの非標準 HTTP プロファイルをリレー内に隔離する | accepted |
| [0061](0061-relay-resident-gc-oneshot.ja.md) | リレーは常駐プロセス、GC はワンショット cron ジョブ | accepted |
| [0062](0062-single-tx-at-most-once-idempotency.ja.md) | claim・ビジネス関数・complete を単一トランザクションで実行してアットモストワンスを保証する | accepted |
| [0063](0063-idempotency-scope-required.ja.md) | クロスユーザーのキー衝突を防ぐためすべての Store 呼び出しに明示的スコープを必須とする | accepted |
| [0064](0064-idempotency-fixed-ttl.ja.md) | 冪等性キーの TTL を 24 時間に固定しルート別設定を設けない | accepted |
| [0065](0065-idempotency-response-persistence.ja.md) | 決定論的リプレイを可能にするためレスポンスボディを JSON で永続化する（PII トレードオフを許容） | accepted |
| [0066](0066-idempotency-gc-separate-job.ja.md) | 冪等性キーのガベージコレクションを独立したワンショット CLI ジョブとして実行する | accepted |
| [0067](0067-idempotency-orthogonal-concerns.ja.md) | 冪等性をオプティミスティックロックおよびレート制限と直交に保つ | accepted (exclusion) |
| [0068](0068-job-fresh-fx-app-per-run.ja.md) | ジョブ起動ごとに新しい fx.App を構築する（ワンショットライフサイクル） | accepted |
| [0069](0069-job-no-worker-machinery.ja.md) | ジョブにはブローカー・サーキットブレーカー・ドレイン・ヘルス機構を意図的に設けない | accepted (exclusion) |
| [0070](0070-job-explicit-registration.ja.md) | Job は明示的に登録する（自動検出なし） | accepted |
| [0071](0071-realtime-delivery-driving-mechanism.ja.md) | server→client のイベント配送に、独立した駆動機構 Realtime Delivery を採用する | accepted |
| [0072](0072-postgres-state-dynamodb-eventlog.ja.md) | 現在状態は PostgreSQL が持ち、DynamoDB EventLog は有限の replay store であって event-sourced log ではない | accepted |
| [0073](0073-sns-sqs-instance-fanout.ja.md) | realtime の wakeup は SNS から instance ごとの SQS queue へ fan-out する | accepted |
| [0074](0074-query-ticket-stream-authentication.ja.md) | SSE stream は subject / destination / scope / expiry に bind した opaque な query ticket で認証する | accepted |
| [0075](0075-config-driven-observability-gating.ja.md) | 設定駆動によるオブザーバビリティゲーティング | accepted |
| [0076](0076-vendor-neutral-otlp-export.ja.md) | ベンダー中立の OTLP 専用エクスポート（バックエンドは Collector に委譲） | accepted |
| [0077](0077-official-otel-semconv.ja.md) | 公式 OpenTelemetry セマンティック規約のみを使用し、カスタム semconv の発明や型付き設定へのベンダーキー追加は行わない | accepted (exclusion) |
| [0078](0078-dual-path-metrics.ja.md) | メトリクスは 2 経路を通る — OTLP プッシュと Prometheus スクレイプ | accepted |
| [0079](0079-lifecycle-independent-provider.ja.md) | オブザーバビリティプロバイダーはライフサイクル非依存（ProviderShutdowner） | accepted |
| [0080](0080-fixed-default-sampling.ja.md) | SDK デフォルトサンプリングを固定し、サンプリングを環境変数ノブとして公開しない | accepted (exclusion) |
| [0081](0081-library-selection-policy.ja.md) | 単一責任のライブラリ選定ポリシー | accepted |
| [0082](0082-bridge-instrumentation-exceptions.ja.md) | ブリッジ / 計装ライブラリを有界な SRP 例外として認める | accepted |
| [0083](0083-containerized-pinned-toolchain.ja.md) | 再現性のために mise でバージョン固定されたコンテナ化ツールチェーンを使用する | accepted |
| [0084](0084-mise-ssot-drift-gate.ja.md) | mise が解決するバージョンは mise.toml を単一の情報源とし、下流に伝播させ CI でドリフトを検知する | accepted |
| [0085](0085-make-single-entrypoint.ja.md) | Make を単一のツールエントリポイントとし、.mk 登録とセルフドキュメンティングなヘルプを提供する | accepted |
| [0086](0086-scripts-in-node-go.ja.md) | 運用スクリプトは scripts/ に TypeScript または Go で配置し、シェルスクリプトは使用しない | accepted |
| [0087](0087-docker-compose-dev-environment.ja.md) | ローカル開発環境はプロファイルで分離されたサービスを持つ Docker Compose で提供する | accepted |
| [0088](0088-two-layer-golangci-config.ja.md) | 2 層の golangci 設定——最小デフォルトと完全な権威ゲート | accepted |
| [0089](0089-local-hooks-mirror-ci.ja.md) | ローカル git フックは CI 契約を複製する（local == CI、グロブスコープ、バイパス後に一度検証） | accepted |
| [0090](0090-coverage-hard-gate.ja.md) | 総カバレッジ 90% を CI のハードゲートとし、例外ガバナンスパスを設ける | accepted |
| [0091](0091-ci-real-graph-boot-check.ja.md) | CI は実際の Postgres に対して実際の fx グラフを起動する（スタートアップ検証） | accepted |
| [0092](0092-generated-artifact-drift-gate.ja.md) | 生成成果物ドリフトゲートとリリースブランチ集約型自動生成ボット | accepted |
| [0093](0093-multi-layer-security-scanning.ja.md) | 多層セキュリティスキャン——報告とゲートを分離し、ハードニングされたランナー上で行う | accepted |
| [0094](0094-sha-pinned-actions.ja.md) | GitHub Actions を SHA でピン留めし、サプライチェーン隔離を適用する | accepted |
| [0095](0095-malicious-package-detection-via-cooldown.ja.md) | 悪意あるパッケージへの主防御は公開クールダウンとし、専用の検知ツールは採用しない | accepted |
| [0096](0096-rollback-integration-tests.ja.md) | インフラ統合テストはリアル DB に対してセンチネルエラーロールバックで実行する | accepted |
| [0097](0097-multi-model-adversarial-review.ja.md) | ファインダー・ベリファイアーサブエージェントによるマルチモデル敵対的レビューを使用する | accepted |
| [0098](0098-lean-a-spec-scaffold.ja.md) | スペックファイルからドメインとユースケースのみスキャフォールドし、コントローラーとインフラは生成コードから導出する | accepted |
| [0099](0099-cli-humble-object-split.ja.md) | CLI ハンブルオブジェクト分割（薄い cmd/ シェル + テスト可能な internal/cli コア） | accepted |
| [0100](0100-single-multi-command-binary.ja.md) | すべてのロールを 1 つのマルチコマンドバイナリに集約する | accepted |
| [0101](0101-single-runtime-image.ja.md) | コマンドオーバーライドによる単一ランタイムイメージ（目的別イメージなし） | accepted |
| [0102](0102-hardened-alpine-runtime.ja.md) | ハードニング Alpine をランタイムベースとして使用し、distroless/scratch は使用しない | accepted (exclusion) |
| [0103](0103-per-environment-images.ja.md) | 環境別イメージ（.env マトリックス × APP_ENV ビルド引数、ビルド時に固定） | accepted |
| [0104](0104-predeploy-oneshot-migration.ja.md) | マイグレーションはデプロイ前のワンショットとして実行し、アプリケーション起動時の自動マイグレーションは行わない | accepted (exclusion) |
| [0105](0105-release-image-supply-chain.ja.md) | リリースイメージのサプライチェーン完全性（cosign 署名 + プロベナンス + SBOM） | accepted |
| [0106](0106-vendor-neutral-deploy-skeleton.ja.md) | デプロイはベンダー中立のスケルトン（ビルド/署名は実装済み；クラウド CD はスタブ；レジストリは固定しない） | accepted |
| [0107](0107-docs-via-github-pages.ja.md) | docs/ の静的コンテンツを GitHub Pages で公開（production プッシュ時にリリース） | accepted |
| [0108](0108-no-in-app-rate-limiter.ja.md) | アプリケーション内レートリミッターを提供しない | accepted (exclusion) |
| [0109](0109-scheduled-job-concurrency-delegated.ja.md) | スケジュールジョブの同時実行制御をアプリ内で行わず、スケジューラに委譲する | accepted (exclusion) |
| [0110](0110-no-generic-cache-abstraction.ja.md) | 汎用 Cache 抽象化を提供しない | accepted (exclusion) |
| [0111](0111-outbox-relay-hardening-delegated.ja.md) | outbox relay はバランス型で出荷し、ハードニング（多層 lease 再設計）は運用で得た事実に委ねる | accepted (exclusion) |
| [0112](0112-pnpm-as-the-only-node-resolver.ja.md) | Node パッケージはすべて pnpm で解決し、npm を使わない | accepted |
| [0113](0113-outbox-payload-kinds-and-parity-declaration.ja.md) | outbox payload の種別を宣言させ、snapshot は集約とのフィールド対応を検査する | accepted |
| [0114](0114-predicate-defined-set-writes-on-commandservice.ja.md) | 対象行が述語でしか名指しできない書き込みを CommandService に認める | accepted |

フロントマターフィールド: `status`、`date`、`deciders`、`supersedes` / `superseded-by`、`tags`。
Consequences は MADR 標準に従う（`Positive` / `Negative`; 任意で `Neutral`）。
