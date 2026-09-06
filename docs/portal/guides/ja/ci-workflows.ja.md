# GitHub Actions Workflows

このディレクトリには CI/CD 用の GitHub Actions ワークフロー定義を格納しています。ワークフローは目的別にグルーピングされており、PR ゲート（lint / test / セキュリティスキャン）、push 起点のデプロイ、リリースブランチ起点のドキュメント再生成という構成です。

## トリガー戦略

| グループ | 発火タイミング | 目的 |
| --- | --- | --- |
| CI チェック | 全 PR | lint / test / 生成物整合性が失敗したらマージブロック |
| セキュリティ | ツールごとのマトリクス（後述） | コード / 依存 / イメージ / ワークフロー定義 / コミット済みシークレットの問題を surface |
| デプロイ | `production` / `staging` / `develop` への push | 成果物ビルド、マイグレーション実行、アプリ / docs portal をデプロイ |
| ドキュメント | `release/*` への push | OpenAPI / ER / portal ドキュメントを再生成し auto-sync PR を作成 |
| アシスタント | プルリクエストでの `@claude` メンション | オンデマンドで回答・調査する。書き込み権限を持つアカウントに限定 |

`gen-*-artifacts-check` 系のワークフローが守る不変条件は「コミットされた生成物が生成器から再現できる」ことです。この不変条件は「入力が変わったのに生成物が再生成されていない」「生成物が直接書き換えられた」の 2 方向で崩れるため、`on.pull_request.paths` には**入力と生成物の双方**を列挙する必要があります。入力側だけを見張ると、生成物のみを触った PR に対して構造的に盲目になり、壊れた生成物がそのままベースブランチへ入って次の無関係な PR が代わりに赤くなります。

これらのワークフローは生成器のバージョンを `mise.toml` で固定しているため、同ファイルは多くの検査にとって入力です。`paths` フィルタはファイル単位でしか判定できないので、この共有ロックファイル内の無関係なツールを更新しただけでも検査が起動します（Postgres を伴う `gen-db` ジョブを含む）。この過剰起動は意図的に受け入れています。生成器のバージョン更新こそ再検証すべき変更であり、トリガを絞るために `mise.toml` を分割するコストの方が、たまの余分な実行より大きいためです。

## 結果コメント

このリポジトリの PR では 30 件ほどの検査が走り、そのほとんどがコメントを投稿しえます。通過した検査ごとのコメントは誰も尋ねていないことを述べるだけで、言うべきことがある唯一のコメントを埋もれさせます。したがって結果コメントは**その検査に報告すべきことがあるときにだけ作成します**。この判定を運ぶのが `upsert-pr-comment` の `status` 入力で、コメントを抑止する値はリテラル `success` ただ 1 つです。それ以外の値は、打ち切られたジョブが残す空文字も含めて、すべて投稿します。

修正されたことは沈黙で伝えません。`success` が抑止するのはコメントの *作成* だけで、更新は抑止しません。以前の push で失敗した検査は、自分の赤いコメントを緑の結果でその場に上書きします。PR にはもう成り立たない失敗が残らず、読み手は失敗が報告された場所で解決を知ることができます（不在に気付く必要がありません）。削除にすると修正の記録が残らず、再発時にはコメントが、既に読み進められたスレッドの末尾へ移動してしまいます。

**上書きではなく削除する例外が 1 つだけあります。打ち切り（CUT OFF）通知です。** これは判定を記録しておらず「ある実行が判定に到達しないまま終わった」ことしか述べていないため、後続の `success` は所見を上書きしているのではなく、開いたままの問いに答えているだけで、消しても失われる情報がありません。上書きすると、キャンセル 1 回と引き換えに恒久的な緑コメントが PR に残ります。しかもキャンセルは日常的です — `cancel-in-progress` は 2 回目の push のたびに前の実行を打ち切り、出力を流しながら打ち切られたジョブは書きかけの本文を残すので、通知が作成されてそのまま緑で固定されます。打ち切り自体はどちらにせよ実行履歴に残ります。判定は `CUT OFF (no result produced)` の見出しで行います。本文を書きながら打ち切られたジョブは本文を持つため action 自身の通知文には到達せず、共通するのは呼び出し側が渡す見出しだけで、それを必須にしているのが `make actions-cutoff-lint` です。トークンが削除を拒否した場合は上書きへフォールバックするので、緑の実行に打ち切り表示が残ることはありません。

**判定は肯定形で導きます — 明示的にクリーンな信号のときだけ黙ります。** ステップが既に `status` 出力を持つ場合は呼び出し側がそれをそのまま渡し、クリーン状態が件数やフラグで表される場合は所見側ではなくその値そのものを検査します（`steps.<id>.outputs.count == '0' && 'success' || 'findings'`）。差が出るのは本文を作るステップが走らなかったときです。出力は空になり、それはクリーンな値ではないので検査は報告側へ倒れます。逆向きに書くと、完走しなかった実行がすべて合格に見えてしまいます。title の `✅` と `success` は構造上同じことを意味します — 非ブロッキングな所見を `✅` とする側（`osv-scan`）はそれについて黙り、`⚠️` とする側（`sast`）は黙りません。

定数 `report` を渡す呼び出しは `image-scan.yaml` の 3 件だけです。SBOM のインベントリと 2 つの Trivy の表は判定ではないので「言うべきことが無い」状態を持たず、またこのジョブはデプロイブランチ向けの PR でしか走らず、そこではイメージの中身こそがレビュー対象だからです。

コメントはそれを書いた実行より長く残ることがあります。`paths` フィルタにより後続の push でワークフローが走らないことがあり、赤いコメントが、検査していない head の上に立ったまま残ります。各コメントが持つ Commit / UpdatedAt のフッタが現行のものと区別する手がかりであり、権威ある状態はチェックラン側にあります。

## ジョブの打ち切り

ジョブは判定に到達しないまま止まることがあります（タイムアウト、キャンセル、ランナー障害）。そのとき PR 上で何が読めるかは、走っていたツールの性質ではなく、ジョブとコメントステップをどう宣言したかで決まります。しかもここでの既定値はどれも誤った側に倒れています。以下の規約は `make actions-cutoff-lint` で機械的に守っています。このディレクトリの全コメントステップと全ジョブを目視で維持することはレビューに要求できないためです。

**`upsert-pr-comment` を呼ぶステップは、キャンセル後にも到達できなければなりません。** Actions はステータスチェック関数を含まないカスタム `if:` に暗黙で `success() &&` を前置するため、打ち切られたジョブではコメントステップがスキップされ、PR には何の痕跡も残りません。一方で `Fail if …` 側は `always()` を持つことが多く、チェックは赤くなります。理由の読めない赤は、どちらか片方の欠落より悪い状態です。したがって条件には `always()` か `cancelled()` が要ります。`failure()` は**該当しません** — キャンセルされたジョブでは false になるためです。現在はすべての呼び出しが素の `always() && github.event_name == 'pull_request'` です。コメントすべきかどうかはステップをスキップすることではなく action 内の `status` が決めます。走らなかったステップは、以前の push が残したコメントを訂正できないためです。

**本文ファイルの不在は、ステップの失敗ではなく打ち切りとして報告します。** 早期に打ち切られたジョブはファイルを書くステップに到達しないので、不在はまさにコメントが生き残るべきケースの通常形です。`upsert-pr-comment` は打ち切りの通知を投稿し、呼び出し側の見出しを置き換えます — ジョブが走らなかったと書く本文に対して、呼び出し側が設定した title はもう何も説明していないためです。通知は原因を名指ししません。本文が無い理由には前段ステップがそのまま失敗した場合もあり、両者を区別できるのは実行ログだけだからです。代償として `body-file` のパスを誤配線すると、失敗ではなく緑の実行に打ち切り通知が出ます。これは気付ける程度に喧しく、打ち切り時の沈黙はそうではありません。`cancel-in-progress` のもとでは、打ち切られた実行がこの通知を投稿した直後に新しい実行が同じ marker を上書きすることがあります。これは機構が働いている姿であって、直すべき不具合ではありません。

**不在だけでは判定の半分にしかならないので、呼び出し側も打ち切り時の見出しを渡します。** 多くの検査ステップは出力を `tee` でそのまま本文ファイルへ流し、`title` は終了コードを見たあとに出力します。検査の途中で打ち切られると、そのジョブは**書きかけのファイル**を残すため、action からは完成した本文と区別がつきません。一方 title は出力されないままです。そこで呼び出し側が判定のもう半分を `${{ steps.<id>.outputs.title || '## ⚠️ <検査名>: CUT OFF (no result produced)' }}` の形で持ちます。フォールバックは本文を作るステップが自らの結論に到達しなかったときにちょうど発火し、書きかけのログはその下にそのまま残ります。見出しがステップ出力ではなくリテラルの箇所（`image-scan.yaml` / `sync-versions-check.yaml`）では、同じ判定をそのステップの `outcome` / 出力への条件として書きます。書くときは GitHub の式の罠に注意してください。`cond && '' || X` は空文字が falsy なので常に `X` になります。見出しは真の側の枝に置く必要があります。

**全ジョブに `timeout-minutes` を置きます。** 無いと GitHub 既定の 360 分まで走るため、1 つのハングがランナーを 6 時間占有します。値は「実測の最大所要 × 3 を 5 分単位で切り上げ、下限 10 分」です。下限は混雑したランナーでのセットアップ変動を吸収するためで、直近に完了実行が無いジョブは 15 分とします。この式から外れる値だけを以下に挙げます。他は全て式が与える値であり、一覧を引くのではなく式から再導出できます。

| ジョブ | 分 | 式から外した理由 |
| --- | --- | --- |
| `auto-generate-docs.yaml` `generate-docs` | 25 | 実測 約 7 分 |
| `go-test.yaml` `go-test` | 20 | 実測 約 5 分 |
| `image-scan.yaml` `build`、`deploy-app.yaml` `build` | 15 | レイヤキャッシュが冷えたイメージビルドは実測を大きく超えて振れる |
| `deploy-app.yaml` `deploy` | 30 | 現状はプレースホルダ。作成先が実デプロイを配線したときに 10 分の上限へ当てない |
| `fuzz.yaml`、`scorecard.yaml`、`notify.yaml`、`osv-release-gate.yaml`、`checkov.yaml`、`closed-loop-weekly.yaml`、`scan-issue-report.yaml` | 15 | 直近に完了実行が無く実測できない |
| `zap-api-scan.yaml` `dast` | 30 | 完了実行が無く実測できないうえ、スキャンの前にアプリケーションをビルドして起動し、スキャン自体の長さは OpenAPI 定義の規模で決まる |
| `code-ql.yaml` `codeql` | 30 | 上限は matrix の最も遅い leg に掛かるが、`go` 以外の leg には完了実行が無く実測できない。加えて `security-extended` は従前の値を測ったスイートより大きい |
| `secret-scan.yaml`、`trufflehog.yaml` | 15 | 実測は差分を見る PR 実行のみ。週次は全履歴を走査するが、その完了実行が一度も無く実測できない |
| `bearer.yaml` `bearer` | 20 | 完了実行が無く実測できないうえ、報告の前に自前ツリー全体のデータフローモデルを構築する |
| `sonarqube.yaml` `sonarqube` | 15 | ベンダー側の解析キューが最大 10 分待つため。テストとカバレッジのゲートはそれぞれの所有ワークフローで実行する |
| `app-di-startup-check.yaml`、`gen-go-artifacts-check.yaml` | 15 | 式より前から存在する値。動いている上限を下げてもリスクしか増えないためそのまま |
| `claude.yaml`、`go-lint.yaml`、`sample-removal-check.yaml` | 30 | 同上。`go-lint` は golangci-lint 自身の timeout を無効化して走らせているため、これがそのジョブ唯一の打ち切り点でもある |
| `graphify-extract.yaml` `graphify-extract` | 60 | 変更されたドキュメントの数だけアシスタントを走らせるうえ、後述の理由で `--max-turns` を持たないため、これがこのジョブ唯一の上限になる |
| `closed-loop-summarize.yaml` `summarize` | 20 | こちらもアシスタントを走らせる。完了実行が存在しない時点で置いた値 |
| `setup-scripts-check.yaml`、`doc-language-removal-check.yaml` | 20 | 実測できる実行が無い作成時点で置いた値。いずれも撤去を通しで実行したうえで撤去後のリポジトリを検証し直す |
| `eslint.yaml` `eslint` | 15 | 同上。その後の実測は大きく下回るが、動いている上限を下げてもリスクしか増えないためそのまま |

上限に当たり始めたジョブは実測を追い越しているということなので、数字を小突くのではなく測り直して式に掛け直します。reusable workflow を呼ぶジョブには `timeout-minutes` を書けません（invalid key）。検査はそれらを除外し、上限は呼ばれる側のジョブが持ちます。

3 つの規約を 3 本ではなく 1 本の検査に置いているのは、これらが 3 つのポリシーではないからです。上限の無いジョブが打ち切りを生み、コメント側の 2 つがそれを読めるようにします。どれか 1 つだけを直しても PR 上の状況は改善しないので、単独で走らせる理由がありません。

## ワークフロー一覧

### CI チェック（Pull Request）

|ワークフロー|ファイル|説明|
|---|---|---|
|Go Lint|`go-lint.yaml`|golangci-lint による Go コードの静的解析|
|Go Test|`go-test.yaml`|Go テスト実行とカバレッジレポート、およびカバレッジゲート対象外の `scripts/` ツールテスト|
|Module Tidy Check|`tidy-check.yaml`|go.mod / go.sum の整合性検証|
|SQL Lint|`sql-lint.yaml`|sqlfluff による migration / DML / seed SQL の検証|
|Actions Lint|`actions-lint.yaml`|actionlint によるワークフロー定義の検証、composite action の `run:` スクリプトの shellcheck 検査、PR コメント本文への secret 混入・固定長フェンスの検査、ジョブ打ち切り時の振る舞いの検査|
|Migration Check|`migration-check.yaml`|マイグレーションファイルの検証（重複、欠番、up/down ペア）|
|Sync Versions Check|`sync-versions-check.yaml`|mise.toml のバージョンが go.mod / 各 Dockerfile / README へ伝播済みか検証|
|Generated Go Artifacts Check|`gen-go-artifacts-check.yaml`|生成済み Go コードとコミット済み成果物の一致検証|
|Generated Database Artifacts Check|`gen-db-artifacts-check.yaml`|生成済み sqlc コードとコミット済み成果物の一致検証|
|Generated OpenAPI Artifacts Check|`gen-oapi-artifacts-check.yaml`|OpenAPI バンドルとドキュメントの一致検証|
|Portal Check|`portal-check.yaml`|ドキュメントポータルのビューアー（`docs-viewer/`）の型検査とテスト|
|Scripts Check|`scripts-check.yaml`|リポジトリの TypeScript 補助スクリプト（`scripts/**/*.ts`）の型検査と、判定ロジックを覆う単体テスト、および `docs-viewer/src/**` も走査する 1:1 テスト対応ゲートを実行します|
|OpenAPI Lint|`oapi-lint.yaml`|OpenAPI 定義を `redocly lint` で検証（命名 / casing / description / 未使用コンポーネント）|
|App Boot Check|`app-di-startup-check.yaml`|DB 付きでアプリケーションサーバが正常に起動するか検証|
|Job Boot Check|`job-boot-check.yaml`|ジョブのエントリポイントが起動し、未知のジョブを拒否するか検証|
|Worker Boot Check|`worker-boot-check.yaml`|worker のエントリポイントが起動（DI / DB）し、未知の worker を拒否するか検証|
|Outbox Relay Boot Check|`outbox-relay-boot-check.yaml`|outbox relay が realtime チャネルで起動（DI / DB）し、正常にシャットダウンするか検証|
|Dockerfile Lint|`docker-lint.yaml`|hadolint による Dockerfile の検証（go_tool_runner 経由）|
|Markdown Lint|`md-lint.yaml`|markdownlint による Markdown 体裁の検証、実 mermaid パーサによる ` ```mermaid ` フェンスの構文検証、`.claude/**` のスキル / エージェント定義の実態一致と `.codex/**` との存在対応の検証|
|Commitlint|`commitlint.yaml`|PR が base ブランチへ加えるコミットのメッセージを検証（`commit-msg` フックが覆えない経路を担う）|
|Pin Actions Check|`pin-actions-check.yaml`|GitHub Actions が SHA でピン留めされているか検証（サプライチェーン対策）|
|Pin Images Check|`pin-images-check.yaml`|Docker base image が lockfile 通り digest でピン留めされているか検証（サプライチェーン対策）|
|Egress Check|`egress-check.yaml`|各ジョブのインライン `allowed-endpoints` が SSOT 通りか検証（[ランナーのハードニング](#ランナーのハードニング)を参照）|
|Graphify Check|`graphify-check.yaml`|追跡している graphify の生成物が可搬なまま保たれているか検証。`.gitignore` のホワイトリスト外が追跡されていないこと、生成したマシンの絶対パスがどの生成物にも残っていないこと|
|Setup Scripts Check|`setup-scripts-check.yaml`|文書化されたローカライズ手順を実際に走らせ、結果を検証する。dry run が何も書かないこと、再実行で何も起きないこと、ツールとその `make` ターゲットが揃って自爆すること、サンプル撤去がまだ必要とする共有モジュールは残ること<!-- boilerplate-only:line -->|
|Sample Removal Check|`sample-removal-check.yaml`|サンプル API を撤去し、縮んだ spec から再生成したうえで、撤去後のリポジトリが build / lint / test / migrate を通り、参照の取りこぼしが無いことを検証<!-- sample-api:line -->|
|Licensed Scanners Removal Check|`licensed-scanners-removal-check.yaml`|ライセンス条件付きのスキャナを撤去し、その後も `make pin-actions-check` / `make egress-check` / Markdown lint が緑であることを検証する。いずれも「どの workflow も参照しないエントリ」で落ちるため、取りこぼしは撤去を走らせたリポジトリでしか現れない|
|Doc Language Removal Check|`doc-language-removal-check.yaml`|ドキュメントを 1 言語へ畳み、畳んだ後のリポジトリが緑であることを検証する。`ja` 経路では正本のフロントマターが移植されたことも含む — 失えばスキルが 1 本も読み込まれなくなるが、ファイルは全て在るため他の何も報告しない<!-- lang-choice:line -->|

<!-- boilerplate-only:begin -->
表の末尾にある初期化・撤去系の検査は、他と性質が違います。テンプレートから作られたリポジトリが
一度だけ走らせるツールを実際に動かすもので、どれもこのリポジトリを守りません。対象のコードが
走るのは他人の checkout の中で一度きりであり、そこで失敗してもこちらでは何も赤くなりません。
通常のチェックとして走らせることが、まだ直せる人の目の前へその失敗を出す唯一の手段になります。
いずれも撤去を本当に実行したうえで撤去後のリポジトリを検証し直します。壊れるのは、撤去が持ち
去った生成物を前提に書かれた検査の側だからです。各検査は自分が動かすツールと一緒に落ちるので、
作成先は対応する判断を下すたびに 1 本ずつ失っていきます。
<!-- boilerplate-only:end -->

### セキュリティ

|ワークフロー|ファイル|説明|
|---|---|---|
|CodeQL Scan|`code-ql.yaml`|`security-extended` スイートでの CodeQL 解析。言語ごとに matrix を分け、`go` / `javascript-typescript`（docs-viewer / scripts）/ `actions`（ワークフロー定義そのもの）を対象とする|
|Dependency Scan|`trivy-fs.yaml`|Trivy によるライブラリ脆弱性スキャン(開発者向け)|
|Release Dependency Scan|`trivy-release-gate.yaml`|develop/staging/production 向け PR での Trivy 依存スキャン|
|Grype Scan|`grype.yaml`|Trivy と同じ依存マニフェストを、別の脆弱性 DB と別のマッチャで走査する Anchore Grype のファイルシステムスキャン|
|Image Scan|`image-scan.yaml`|Docker イメージビルド + SBOM 生成（SPDX-JSON と CycloneDX-JSON の両形式）+ Trivy スキャン + ビルド済みイメージへの Dockle のプラクティス検査 + CycloneDX SBOM への `trivy sbom` による再照合|
|Vulnerability Scan|`vulnerability-check.yaml`|govulncheck による Go パッケージ脆弱性検出|
|OSV Scan|`osv-scanner.yaml`|OSV データベースによる Go モジュール / npm lockfile 横断の脆弱性スキャン|
|Release OSV Scan|`osv-release-gate.yaml`|develop/staging/production 向け PR での OSV スキャン。HIGH 以上で fail|
|Secret Scan|`secret-scan.yaml`|ワーキングツリーに対する 2 系統の独立したシークレット検出。gitleaks（正規表現 / エントロピーの広い網）と Trivy（誤検知の少ない固定ルール）を、判定を分けた別ジョブとして実行する|
|Secret Scan (TruffleHog)|`trufflehog.yaml`|TruffleHog による**検証済み**シークレット（実際に有効なクレデンシャル）の検出|
|Actions Static Analysis|`zizmor.yaml`|zizmor によるワークフロー / composite action 定義自体の静的解析（pre-commit フックと同じ `make` ゲートを共有）|
|Dependency Review|`dependency-review.yaml`|PR が新たに持ち込む脆弱な依存をマージ前にブロック|
|OpenSSF Scorecard|`scorecard.yaml`|リポジトリのセキュリティ姿勢のスコアリングと結果の公開|
|Go Cooldown|`go-cooldown.yaml`|cooldown 窓の内側で公開された direct Go モジュールを足す / 上げる PR をゲート|
|Tool Cooldown|`tool-cooldown.yaml`|cooldown 窓の内側で公開された CLI ツール版（`mise.toml` / `python/*.in` の宣言）を pin する PR をゲート|
|Pnpm Cooldown|`pnpm-cooldown.yaml`|期限が切れた・3 ヶ月を越えた・lockfile ともう一致しない `minimumReleaseAgeExclude` エントリを失敗させる|
|Config Scan|`trivy-config.yaml`|Trivy による Dockerfile の設定不備スキャン（HIGH 以上でゲート）|
|Checkov Scan|`checkov.yaml`|zizmor も Trivy も持たないルールセットによる、ワークフロー定義と Dockerfile への Checkov ポリシースキャン（報告専用）|
|SAST|`opengrep.yaml`|Opengrep（Semgrep 互換）による自前の Go / TypeScript ソースの解析（taint 追跡あり）|
|DevSkim Scan|`devskim.yaml`|言語を問わずツリー内の全ファイルに当たる DevSkim の正規表現スキャン|
|Bearer Scan|`bearer.yaml`|機微な値が sink へ到達する経路を追う Bearer のデータフロースキャン（報告専用。Elastic License 2.0 — [Bearer のライセンスと撤去](#bearer-のライセンスと撤去)を参照）|
|ESLint Scan|`eslint.yaml`|3 つの TypeScript ワークスペースに対する `eslint-plugin-security` の検査。matrix の 1 レグずつ（報告専用）|
|SonarQube Cloud Scan|`sonarqube.yaml`|SonarQube Cloud による一次ソースの解析。結果は Web API から読み戻して SARIF へ変換する（**Sonar の品質ゲートでブロックする**。issue の一覧は報告専用。`SONAR_TOKEN` が必要。[資格情報を要するスキャナの撤去](#資格情報を要するスキャナの撤去)を参照）|
|Lockfile Integrity|`lockfile-integrity.yaml`|npm の `resolved` URL が正規レジストリかつ HTTPS であることの検証|
|OpenAPI Security|`openapi-security.yaml`|Spectral + OWASP API Security ルールセットによる OpenAPI 定義の検証|
|Fuzz|`fuzz.yaml`|外部入力を受けるパーサに対する Go ネイティブ fuzzing|
|DAST|`zap-api-scan.yaml`|ランナー内で起動したアプリケーションに対する、OpenAPI 定義を入力とした OWASP ZAP の API スキャン（報告専用のサンプル。[DAST](#dast) を参照）|
|Capability Diff|`capability-diff.yaml`|capslock による Go 依存グラフの capability 差分報告（report-only）|
|Agent Config Scan|`trustabl.yaml`|AI エージェント設定—— `.claude/` 配下の subagent / skill 宣言と MCP サーバー宣言——に対する trustabl のスキャン（報告専用。[エージェント設定スキャン](#エージェント設定スキャン)を参照）|
|Notify|`notify.yaml`|定期実行の失敗、および非ブロッキングなスキャナの検出を人へ届ける `workflow_call` の再利用ワークフロー|
|Scan Issue Report|`scan-issue-report.yaml`|完了したスキャナの報告をツールごとの `Code Scan Report: <tool>` issue へ集約し、件数が 0 になったらその issue を閉じる。PR にとってのコメントに当たる、定期実行にとっての常設の記録（[検出通知のトリガー](#検出通知のトリガー)を参照）|

各スキャナは可能な限り SARIF を GitHub code scanning へ送り、検出は共通の `upsert-pr-comment` アクションで PR にコメントします（そもそもコメントを書くのがどういうときかは [結果コメント](#結果コメント) を参照）。

#### セキュリティのトリガーマトリクス

各ツールは「結果が実際に変わりうる場所」で走らせています。PR はその変更自身が持ち込むリスクを surface し、protected branch への push はブランチ保護が判断材料にする code scanning のベースラインを残し、定期実行は「コードが変わらなくても結果が変わる」種別（新規公表 CVE / 新規クエリ）にだけ設けます。

| 種別 | PR | protected branch への push | 定期 |
| --- | --- | --- | --- |
| gitleaks | 全 PR | 不要 | 週次で履歴全体 |
| Trivy secret | 全 PR・作業ツリー | 不要 | 週次 |
| TruffleHog | 全 PR の差分 | 不要 | 週次で履歴全体 |
| zizmor | Actions 関連ファイル変更時 | `develop` / `staging` / `production` / `release/*` | 週次（オンライン監査） |
| Dependency Review | 依存関係変更 PR | 不要 | 不要 |
| govulncheck | Go・依存変更 PR | 同上 | 週次 |
| Trivy FS | Go・依存変更 PR | 同上 | 週次 |
| OSV-Scanner | 依存関係変更 PR | 同上 | 週次 |
| CodeQL | Go / TypeScript / Actions 定義の変更 PR | 同上 | 週次 |
| OpenSSF Scorecard | 不要 | 既定ブランチのみ | 週次 |
| Image Scan | デプロイ先ブランチへの PR | 不要 | 週次 |
| リリースゲート（Trivy FS / OSV） | デプロイ先ブランチへの PR | 不要 | 不要 |
| Trivy config（設定不備） | Dockerfile 変更 PR | 同上 | 不要 |
| Checkov | Actions 定義 / Dockerfile 変更 PR | 同上 | 週次 |
| Dockle | デプロイ先ブランチへの PR | 不要 | 週次（Image Scan 内） |
| Trivy SBOM | デプロイ先ブランチへの PR | 不要 | 週次（Image Scan 内） |
| Trivy ライセンス | Trivy FS と同一トリガー | 同上 | 週次 |
| OSV diff | 依存関係変更 PR | 不要 | 不要 |
| Opengrep（SAST） | Go / TypeScript・依存・spec 変更 PR | 同上 | 週次 |
| Grype | Go・依存変更 PR | 同上 | 週次 |
| DevSkim | 全 PR | `develop` / `staging` / `production` / `release/*` | 週次 |
| Bearer | Go / TypeScript 変更 PR | 同上 | 週次 |
| ESLint（security） | TypeScript ワークスペース変更 PR | 同上 | 週次 |
| SonarQube Cloud | Go / TypeScript / `sonar-project.properties` 変更 PR | 同上 | — （下記参照） |
| lockfile-lint | lockfile 変更 PR | 不要 | 不要 |
| Spectral（OpenAPI） | spec 変更 PR | `release/*` / デプロイ先ブランチ | 不要 |
| capslock | `go.mod` 変更 PR | 不要 | 不要 |
| Go fuzzing | 不要 | 不要 | 週次 |
| OWASP ZAP（DAST） | `zap-api-scan.yaml` / `.github/zap/**` 変更時 | `develop` / `staging` / `production` / `release/*` | 週次 |
| trustabl（エージェント設定） | — | — | 週次 |

週次実行は月曜未明（UTC）に **15 分刻み**で 1 スロット 1 本ずつずらしています。同一時刻に全スキャナが並ぶのを避けるためです。スロットの割り当ては `00:00` Trivy FS、`00:15` govulncheck、`00:30` TruffleHog、`00:45` OSV-Scanner、`01:00` Scorecard、`01:15` CodeQL、`01:30` Image Scan、`01:45` gitleaks（全履歴）、`02:00` zizmor（オンライン監査）、`02:15` Go cooldown、`02:30` Opengrep、`02:45` fuzz、`03:00` ZAP（DAST）、`03:15` Grype、`03:30` DevSkim、`03:45` ESLint、`04:00` Bearer、`04:15` Checkov、`04:30` trustabl、`04:45` tool cooldown、`05:00` pnpm cooldown、`05:15` Closed Loop Weekly。

刻みが 1 時間でなく 15 分なのは、対象が 21 本まで増えたためです。1 時間刻みだと最後の 1 本が翌日の夜まで始まらず、並べて読むべき検出どうしが 1 日離れてしまいます。定期実行のワークフローを追加するときは次の空きスロットを取ります。2 本が同じスロットを共有しているのは好みの問題ではなく欠陥です。順序には意図があるので、追加は末尾ではなく相応しい位置へ入れます。

GitHub は指定時刻を厳密には守らず、負荷次第でスロットよりかなり遅れて始まることがあります。このずらしは重なりを減らすものであって無くすものではありません。スケジューラが保証しない間隔を細かく調整しても意味がありません。

DAST は `03:00` に入ります。スキャンの前にアプリケーションをビルドして起動する唯一のワークフローで、いちばん長く、他の前に並べても得るものが無いため、ファイルを読むだけのスキャナ群より後ろに置いています。

SonarQube Cloud にスロットはありません。無料プランは organization ごとに 1 ブランチしか解析せず他は拒否するため、定期実行のブランチ解析はここでは成功し得ません。ベンダーが例外としている pull request と、code scanning のベースラインを保つための保護ブランチへの push でだけ走ります。`05:00` は SonarQube Cloud が手放したスロットで、空いたままです。次に増える定期実行がそこを取ります。

#### 検出通知のトリガー

週次スケジュールを持つスキャナは、ジョブが `failure` または `cancelled` で終わったときに `notify.yaml` を呼び出します。PR の失敗は作成者に見えていますが、定期実行の失敗は誰にも見えないためです。`cancelled` を含めるのは、タイムアウトやランナー障害で打ち切られたジョブが `failure` ではなくこちらになるからです。

押し出す価値があるのは失敗だけではありません。報告専用のスキャナは検出してもジョブが green で終わるため、失敗モードは検出に対して決して発火しません。それらは代わりに `notify.yaml` を検出モードで呼び出し、actor / ref / commit と検出内容そのものを添えて通知します。どちらのモードも webhook secret が未設定なら送信をスキップして run を green のままにするため、送信先を持たない作成先が通知のせいで落ちることはありません。

検出通知をどのトリガーで発火させるかは、届けるべき相手が誰かで決まります。脆弱性スキャナは定期実行のみです。PR では検出内容が既に PR コメントとして依存を持ち込んだ作成者宛に出ている一方、週次の検出は「変わっていないコードに対して新たに公開された advisory」であり誰にも届かないからです。

| ワークフロー | 発火条件 | トリガー |
| --- | --- | --- |
| `trivy-fs.yaml` | 修正版のある CRITICAL / HIGH / MEDIUM | schedule |
| `vulnerability-check.yaml` | 到達可能な脆弱性 | schedule |
| `osv-scanner.yaml` | 昇格をブロックする検出 | schedule |
| `grype.yaml` | 脆弱性の検出 | schedule |
| `devskim.yaml` | 検出あり | schedule |

他の定期実行スキャナに検出通知は不要です。gitleaks / Trivy secret / TruffleHog / Opengrep / zizmor（high）/ image-scan のゲート / fuzzing はいずれも検出時にジョブが落ちるため、失敗モードが既に届けています。意図的に未接続のものが 4 つあります。Trivy のライセンス集計は「まだ誰も問題だと合意していないライセンス」を並べるもので（SARIF を書かないのと同じ理由）、CodeQL と Scorecard は結果を code scanning ダッシュボードへ publish するだけでワークフロー側に検出件数が出てきません。Scorecard の「スコア低下」通知には加えて前回スコアの保持が要りますが、それを持つ仕組みはここにありません。Checkov も同じ条件で未接続です。このリポジトリに対するベースラインが 20 件あり、その大半は 1 つのルールがワークフローファイルごとに 1 回ずつ出ているものです。Dockle と `trivy sbom` は自前の配線が要りません。どちらも `image-scan.yaml` の中で走り、その定期実行の失敗は既に人へ届きます。ESLint と Bearer と trustabl は理由が別で未接続です。ベースラインが 0 件ではない（ESLint は 100 件超の warning、Bearer は 14 件の検出、trustabl は `.claude/agents/` 配下の読み取り専用 subagent 1 本につき high が 1 件）ため「検出あり」で発火する通知は変更の内容によらず毎週鳴り続けます。それは人が読まなくなる形の通知です。SonarQube Cloud はもっと単純な理由で未接続です。通知の元になる定期実行を持っていません。

#### 検知が重なる面

複数のツールが同じ種類の指摘を出せます。**重複させてはならないのはゲートであって、ツールではありません**。1 つの問題で PR が 2 回赤くなるということは、抑止する場所が 2 箇所になり、抑止が腐る場所も 2 箇所になるということです。報告は別で、同じファイルを別の DB / 別のルールセットで読む 2 つ目のエンジンは、1 つ目がまだ知らないものを拾います。この冗長性は意図して買っています。

したがって 1 つの面に複数のツールが同時に乗って構いません。ゲートを 1 つに保っているのは「走るツールが 1 つだけ」だからではなく、**ゲートする 2 つのツールが同じ指摘を担当しないから**です。これを支える仕組みは 2 つあり、表はその両方を記録しています。同じルールを judge しうる 2 つのツールがある場合は片方をその面で切る（3 列目がそれを名指しし、検知可能なツールがなぜ使われないかを示す）。2 つのゲートが同居する場合は、互いに素なルールセットを judge する。`自前の Go ソース` の行がその違いを示す例です — Opengrep は Semgrep の ERROR 帯でゲートし、`gosec` は golangci-lint 経由でゲートしますが、対象ファイルは同じでもルールは決して重なりません。つまりここでの「担当 1 つ」は**ルール単位で 1 つ**という意味であり、ツールが 1 つという意味ではありません。

共有された面に乗るそれ以外のツールは報告専用で、その面の判定は下表で `(gate)` が付いたものが持ちます。`(gate)` の無い行はどこでもゲートしません。依存スキャナは報告に徹し、ブロック判定は[リリースゲート](#リリースゲート)が持ちます。

| 面 | 担当 | 検知可能だがここでは使わない |
| --- | --- | --- |
| Dockerfile のセキュリティポリシー | `trivy-config.yaml` **(gate, HIGH+)** + `checkov.yaml`（Checkov・報告専用） | Opengrep（`opengrep.yaml` で Dockerfile ルールを除外） |
| ワークフロー定義 | `zizmor.yaml`（zizmor） **(gate)** + `code-ql.yaml`（`actions` レグ）+ `checkov.yaml`（Checkov・報告専用） | — |
| Dockerfile のスタイル / 正しさ | `docker-lint.yaml`（hadolint） **(gate)** | —（層が違い重複ではない） |
| 自前の Go ソース | `opengrep.yaml`（Opengrep・ERROR 帯） **(gate)** + `go-lint.yaml` 経由の `gosec` **(gate)** — ルールセットは互いに素 + `sonarqube.yaml`（SonarQube Cloud） **(gate, 品質ゲート)** | — |
| OpenAPI の規約 / 命名 | `oapi-lint.yaml`（redocly） **(gate)** | Spectral |
| OpenAPI のセキュリティ姿勢 | `openapi-security.yaml`（Spectral） **(gate)** | redocly |
| 依存の脆弱性 | `trivy-fs.yaml`（Trivy）+ `osv-scanner.yaml`（OSV）+ `grype.yaml`（Grype） — すべて報告専用 | — |
| 自前の TypeScript ソース | `code-ql.yaml`（`javascript-typescript` レグ）+ `opengrep.yaml`（`p/typescript`） **(gate)** + `eslint.yaml`（`eslint-plugin-security`） + `sonarqube.yaml`（SonarQube Cloud） **(gate, 品質ゲート)** | — |
| 言語を問わない全ファイル | `devskim.yaml`（DevSkim） | — |
| AI エージェント設定（`.claude/**`、MCP 宣言） | `trustabl.yaml`（trustabl）——報告専用 | —（ツール付与を解釈するスキャナは他に無い） |
| sink へ到達する機微な値 | `bearer.yaml`（Bearer） — 報告専用。対象はアプリケーションコードのみで `/scripts` は除外（リポジトリのツーリングはユーザーデータを扱わず、この問いが訊いているのはそれだけであるため） | — |
| ランタイムイメージ | `image-scan.yaml`（Trivy） **(gate)** + Dockle（プラクティス検査・報告専用）+ `trivy sbom`（報告専用。ゲートと同じ DB を、Trivy 自身ではなく syft のパッケージ一覧で引く） | — |

`自前の Go ソース` と `自前の TypeScript ソース` の行にはベンダーホスト型のスキャナも乗っています。Sonar はこの表で唯一「ルール単位で担当 1 つ」から意図的に外れています。品質ゲートは静的解析・重複と Sonar 自身の issue 分類をまとめて判定し、カバレッジの閾値は Go / TypeScript のテストワークフローがそれぞれ担います。両者が認識する検出で PR が 2 回赤くなり得ますが、それを受け入れているのは、ベンダーの判定を捨てると「スキャンは報告するが run はそのままマージされる」状態になるためです。

#### Bearer のライセンスと撤去

`bearer/bearer` は **Elastic License 2.0** で公開されており、利用・改変・再配布は認めつつ、ソフトウェアを hosted / managed service として第三者へ提供することと、ライセンスキー機構の回避とを禁じています。このリポジトリ自身の CI で走らせる限りどちらにも触れません。第三者へ何も提供しておらず、CLI はキーを必要としないためです（`--api-key` フラグは legacy と明記され、提供を終えたクラウド製品のために残っているだけです）。

OSI の定義の外にあること自体は Bearer に固有ではありません。CodeQL も OSI 承認ではなく、専用の節を持っていません。書き留める価値があるのは、このテンプレートから作られたリポジトリがワークフローとともにライセンスも引き継ぐという点です。ツールをサービスの一部として提供したい利用者には、ここにある OSI ライセンスのスキャナには生じない判断が要ります。その答えは撤去であり、撤去は次のすべてを落とします。

| 落とすもの | 落とし忘れを捕まえるもの |
| --- | --- |
| `.github/workflows/bearer.yaml` | — |
| [`mise.toml`](../../mise.toml) の `aqua:Bearer/bearer` 行 | `make tool-cooldown-gate` はこの固定値を読む |
| [`.github/egress.toml`](../egress.toml) の `[job."bearer.yaml:bearer"]` セクション | `make egress-check` が「対応する workflow の無いジョブセクション」で落ちる |
| このファイルと対訳の `README.md` にある `bearer.yaml` の行 — timeout 表 / Security 表 / トリガーマトリクス / 週次のずらし方 / 検知が重なる面 — およびこの節 | `make md-lint` が見るのは対訳ペアであって行ではない |

`make pin-actions-check` は、`bearer.yaml` が使っていた Action がすべて他所からも参照されている限り何もする必要がありません。参照されないエントリが lockfile に残ると落ちるため、赤くなったらそこを先に見てください。summary ステップの `level` 補完はワークフローと一緒に消えます。Bearer は全ての結果に `level` を持たず、jq がソートキーで `//` へ落ちずにランタイムエラーになるために置いてあるものです。

#### 資格情報を要するスキャナの撤去

ここには、リポジトリ自身では用意できないものを必要とするスキャナが 2 つあります。SonarQube Cloud はベンダーのサービスへのトークンを要し、CodeQL は GitHub Advanced Security を要します。後者は public リポジトリでは無料、private では課金対象です。このリポジトリは public なので 2 つとも無料で回りますが、このテンプレートから作られたリポジトリが public とは限らず、課金を受け入れるとも限りません。

`make setup-remove-licensed-scanners` は 2 つを 1 回の実行でまとめて撤去し、**製品ごとに別のコミットを積みます**。どれか 1 つのライセンスを既に持っている利用者は、そのコミットだけを `git revert` すれば復活させられます。撤去を 2 つのスクリプトに分けず 1 つにしているのはこのためです。利用者が実際に一度だけ下す判断は「課金されるスキャナやベンダーへ送信するスキャナを使うか」であり、製品単位の選択は 2 つのスクリプトを覚えることよりも「取り消し」として表現するほうが適切です。

このファイルと対訳の編集は、製品ごとのコミットには**含めず**独立した最後の 1 コミットにまとめます。2 製品は同じ表の隣り合う行を占めるため、ドキュメント編集を製品側へ混ぜると最後の 1 つ以外の `git revert` が必ずここで衝突し、分割した意味がなくなるからです。代償として、復活させたスキャナは動きますが記述は戻りません。行はその最後のコミットから読み出せます。

`bearer.yaml` は意図的にこの対象**外**です。Elastic License 2.0 は CI での実行に費用を課さず、制約するのはサービスとしての再配布だけで、これは別の問いであり別の答えになります。[Bearer のライセンスと撤去](#bearer-のライセンスと撤去)を参照してください。そちらは手動手順のままです。

スクリプトが製品と一緒に持っていくもの、および残さなければならないもの:

| 製品と一緒に消えるもの | 残さなければならないもの |
| --- | --- |
| ワークフローファイル。Sonar は `sonar-project.properties` も | — |
| [`.github/egress.toml`](../egress.toml) の `[job."<workflow>:<job>"]` セクション | — |
| [`.github/actions-pin.toml`](../actions-pin.toml) のうち他から参照されなくなったエントリ | `github/codeql-action@v4` — SARIF をアップロードする他のすべてのワークフローが参照する |
| このファイルと `README.ja.md` の行および散文 | 残るスキャナの行 |
| CodeQL の `.github/codeql/**` | — |

lockfile の規則は例外リストではありません。スクリプトは残るワークフロー側の参照数を数え、0 になったエントリだけを削除します。`github/codeql-action@v4` は数えることがリストに勝つ理由を示す例です。CodeQL に紐づいて登録されたエントリですが、SARIF を publish するスキャナはいずれも同じアクションの `upload-sarif` を呼ぶため、CodeQL を撤去してもエントリは残ります。固定のリストなら消していたところです。`actions/download-artifact@v7` は逆の例で、いま使っているのは Sonar の report ジョブだけなので、Sonar の撤去が一緒に消します。`make pin-actions-check` と `make egress-check` はどちらも孤児で落ちるため、消し忘れは静かな残骸ではなく赤い run になります。

この参照数の数え方は、1 つのスキャナを revert したときに `make pin-actions-check` が孤児ではなく**未登録の参照**で赤くなる理由でもあります。撤去対象どうしで共有するエントリは「最後の利用者を消したコミット」が削除するため、より前のスキャナを戻すと、後のコミットが既に消したエントリを参照する `uses:` が復活します。いまの 2 件は共有するエントリを持たないため、この形で赤くなることは現状ありません。3 つ目が加わった時点で戻ります。`make pin-actions-resolve` で戻せますし、どのエントリかは検査が名指しします。

`SONAR_TOKEN` の登録は人手の作業として残り、ベンダー側でのプロジェクト作成も同様です。それが存在しない間はレグが自分をスキップし run は green のままです。資格情報の欠如をスキャン結果ではなくセットアップの未了として報告する理由は[結果コメント](#結果コメント)を参照してください。

#### 一覧に無い OSS スキャナの評価

GitHub の code scanning テンプレート一覧は、ここで走らせられるものの境界ではありません。一覧の外にある OSS ツール 8 件をまとめて評価し、4 件を採用、4 件を見送りました。下の表がその記録です。この表を残しているのは、**このテンプレートから作られるリポジトリはたいてい private で、そこでは答えが変わるから**です。ここでは費用のかからないライセンスが向こうでは受け入れられないことがあり、public リポジトリでは無料のサービスが向こうでは無料とは限りません。判断の経緯を発端となった issue ではなくこのファイルに置いているのは、テンプレートの利用者はこのファイルを読めても、その issue は読めないからです。

ライセンスは第三者の集計ではなく各プロジェクトのライセンスファイル本体から読み取りました。確定できなかったものは推測で埋めず、その旨をセルに書いています。

| ツール | ライセンス | private・社内 | public | 判断 |
| --- | --- | --- | --- | --- |
| Dockle | Apache-2.0 | 可 | 可 | **採用** — `image-scan.yaml` 内。ビルド済みイメージへのプラクティス検査で、ここの他のどのスキャナも読んでいない面。runner 内で完結する。最新リリースは 2025 年 1 月で止まっているが、本体へのコミットは続いている |
| `trivy sbom` | Apache-2.0 | 可 | 可 | **採用** — `image-scan.yaml` 内。新規ツールではなく既に pin 済み Trivy のサブコマンドなので、ライセンスと供給網の判断が増えない |
| Checkov | Apache-2.0（CLI・Action とも） | 可 | 可 | **採用** — `checkov.yaml`。CLI はアカウント不要で runner の外へ出ない。ベンダーの SaaS 連携は別建てのオプトイン機能で、使っていない。採用の理由は `github_actions` ルールで、これは CodeQL を撤去したあとほど重みが増す |
| KICS | 本体 Apache-2.0、**Action は GPL-3.0** | 可 | 可 | **見送り** — 理由はライセンスではなく配布形態。リリース書庫はバイナリ単体でクエリ本体を同梱せず、aqua パッケージは mise が扱えない `go_build` 形式のため、このリポジトリのバージョン SSOT の内側に収まる経路が無い。ワークフローから Action を呼ぶだけなら GPL の義務は生じないが、それでもツールは `mise.toml` と `tool-cooldown.yaml` の外に出る |
| detect-secrets | Apache-2.0 | 可 | 可 | **見送り** — gitleaks・Trivy secret・TruffleHog に続く 4 つ目のシークレットエンジンになる |
| Renovate | **AGPL-3.0**（v11 までは MIT、v12 から AGPL） | 可（self-host 形態） | 可（self-host 形態） | **見送り** — Dependabot と cooldown ゲートで足りており、Renovate が足すものを必要としていない。自リポジトリの依存更新に未改変で使う限り AGPL の開示義務は生じない。Mend のホスト型の条件は**確認できていない**。その形態を採る場合は採用側で確認が要る |
| OpenSSF Allstar | Apache-2.0 | 可（org の判断が要る） | 可 | **見送り** — 検出ではなく強制であり、組織単位の GitHub App なので、そもそもテンプレートがワークフローファイルとして配れる形をしていない。加えて強制対象は [`.github/settings/branch-protection.json`](../settings/branch-protection.json) が既に持っている。採用するならまずどちらが正本かを決める必要があり、外部が運用する App へ private リポジトリの読み取り権限を渡す判断は、このテンプレートではなく採用する組織のものである |

この表には限界が 2 つあります。報告しているのはライセンス条項とアカウントの要否までで、GPL / AGPL 系ツールの利用可否といった各組織の内部規程には踏み込んでいません。食い違う場合は組織側の規程が優先します。もう 1 つ、見送りの判断はこのリポジトリのもので、既にここで走っているものと突き合わせて下したものです。同じ重複を持たないリポジトリでは違う結論になり得ます。結果だけでなく理由を書いているのはそのためです。

#### DevSkim のバージョン固定

ここのワークフローが入れる他のツールはすべて [`mise.toml`](../../mise.toml) で固定されており、それがあるから `tool-cooldown.yaml` は「供給網クールダウンの窓の内側に公開された版」をゲートできます。DevSkim だけが例外です。`microsoft/DevSkim` はリリースバイナリを公開しておらず aqua パッケージも無いため、配布経路は NuGet のグローバルツールだけです。

mise はそれ自体には届きます（`dotnet:` バックエンドが NuGet パッケージを解決します）。それでも使っていない理由は 2 つです。このバックエンドは .NET ランタイム自体も mise 管理下のツールにすることを要求し、リンタ 1 本のためにバージョン SSOT へ言語ランタイムを丸ごと入れることになります。そして [`scripts/tool-cooldown`](../../scripts/tool-cooldown) は `dotnet:` バックエンドの公開時刻の取得経路を持たないため、宣言してもゲートされず *unresolved* として報告されるだけです。`mise.toml` へ移すと、検査されている見た目だけが手に入ります。

そのためバージョンは `devskim.yaml` の `env:` が持ち、これを守る仕組みはありません。更新はリリースノートを人が読んで判断します。

代替の `microsoft/DevSkim-Action` は、この軸ではむしろ悪化します。`Dockerfile` が浮動タグ `mcr.microsoft.com/dotnet/sdk:8.0` から起こしてバージョン未指定の `dotnet tool install` を走らせる Docker action なので、Action を SHA で固定してもレシピが固定されるだけで、実際に走るコードは固定されません。

#### リリースゲート

依存スキャナは二段構えです。通常の PR では報告のみに留めます。既存の依存ツリーから受け継いだ脆弱性はその PR が持ち込んだものではなく、そこでブロックしても更新作業が別途進む間、無関係な作業が止まるだけだからです。ブロックの判定は `develop` / `staging` / `production` 向けの PR で行います。そこでレビュー対象になっている依存の状態が、まさに昇格されようとしている状態だからです。

| ゲート | fail する条件 |
| --- | --- |
| `trivy-release-gate.yaml` | Trivy の全 finding（修正版が出ていないものを含む） |
| `osv-release-gate.yaml` | HIGH / CRITICAL 判定の OSV finding（修正版の有無を問わない）と、判定を持たないが修正版が存在する finding |

OSV ゲートの深刻度は advisory 自身の評価を使い、無ければ osv-scanner がグループ単位で集約する CVSS スコアへフォールバックします。Go 脆弱性データベース由来の advisory はそのどちらも公開しないため HIGH 閾値では測れず、修正版が存在する場合にのみゲート対象とします。評価もできず更新もできない advisory が、昇格のたびに恒久的な赤を生むのを避けるためです。両ゲートとも意図的に `paths` フィルタを持ちません。昇格 PR はマニフェストを一切変更しないことが多く、required check はまず実行されなければブロックできないからです。

#### required check は全 pull request で報告される

[`branch-protection.json`](../settings/branch-protection.json) が挙げた context はマージ前に**報告される**必要があります。GitHub は「報告が無い」を「該当しない」とは読まず、待ち続けます。その pull request は永久にマージ可能になりません。したがって required context を報告するワークフローは、`pull_request` トリガーにフィルタを持ってはいけません。`paths:` や `branches:` はそこでは run が起きるかどうかを決めるものであり、起きなかった run は何も報告しないからです。

そこで起動条件は job 側へ移します。GitHub はこちらでは逆の結論を出すためです。**`if:` で skip された job は `skipped` を報告し、required check はそれを成功として数えます。** path フィルタを持っていた 11 本は [`dorny/paths-filter`](https://github.com/dorny/paths-filter) を使う `changes` job で条件を計算し、その出力で報告 job をゲートします。リリースゲート 2 本は条件がブランチ名でファイル一覧を要さないため、`if:` で `github.base_ref` を直接読みます。

ゲートは **fail open** で書いてあります。

```yaml
    needs: changes
    if: ${{ !cancelled() && needs.changes.outputs.relevant != 'false' }}
```

`relevant` はフィルタが判定に到達しなかったところで空になります（pull_request 以外のイベントで skip された場合と、失敗した場合）。空は `false` ではないので、本体が走ります。逆向きだと静かに壊れます。skip は required context に誰も稼いでいない緑を渡すことになり、フィルタが壊れているときこそそれが効いてしまいます。

各トリガーの `push` 側は `paths:` をそのまま持っています。そちらは context を報告しないので、絞っても何もブロックしません。

`make required-check-lint` は、required context がちょうど 1 つの job から宣言されていること、およびその workflow の `pull_request` トリガーがフィルタを持たないことを検査します。**デッドロックを塞いでいるのは後半で**、これが検査できるのは条件が 1 箇所にしか書かれていないからです。フィルタとその反転に分けて書くと、不変条件は 2 つのリストの一致という、ここでは誰も比較できないものへ移ります。そして食い違いは、片側に足し忘れたパスを変更した pull request でだけ表面化します。

#### pnpm の cooldown 例外

pnpm は、Go と Tool の cooldown が番人との間に持つ関係を反転させています。解決器自身が公開直後の版を拒否し、しかも解決時だけでなく install のたびに lockfile 全体を照らし直す——`--frozen-lockfile` も含めて——ので、ここでの窓はパッケージマネージャが強制しており、記録されずに迂回することはできません。したがって `pnpm-cooldown.yaml` は窓の番人ではありません。

番人が居なかったのは逃げ道のほうです。[`minimumReleaseAgeExclude`](../../scripts/pnpm-workspace.yaml) は特定の 1 バージョンについて窓を外すもので、その期限は誰も読まない散文コメントにしかありませんでした。兄弟のバイパスファイルが `expires` を持つのは、誰も見直さない免除が静かに恒久 allowlist へ変わるのを防ぐためであり、pnpm の例外は同じ債務を日付なしで抱えていました。このワークフローが、同じ形のファイルでそれを与えます——[`pnpm-cooldown-bypass.toml`](../pnpm-cooldown-bypass.toml) が例外ごとに `expires` / `issue` / `reason` を持ちます。期限を `pnpm-workspace.yaml` の外に置くのは、それが pnpm にとって意味を持たず、pnpm が読むファイルを占める筋合いが無いからで、加えてそこを読む検査はコメント解釈になり、pnpm が honor するのに解釈側が取りこぼす書き方が期限の無い免除を通してしまうためです。期限の無い例外、期限切れや長すぎる期限、どの例外にも対応しないエントリ、lockfile がもう解決していない版で失敗します。

期限は `pnpm-workspace.yaml` が変わらなくても訪れるため、スケジュールが必要になります——Go と Tool の cooldown がスケジュールを持つのと同じ理由です。lockfile もトリガーのパスに入れてあります。解決から外れた版を指す例外は残骸であり、解決が動いた時点で表に出すべきだからです。

#### Go モジュールの cooldown

Go には `min-release-age` に当たるものがありません。`go get` に「新しすぎるから採るな」と言わせる手段が無い、ということです。これがツールと防御の関係を逆にします。pnpm は新しすぎるバージョンを依存解決の時点で拒否するのでリゾルバ自体が防御ですが、こちらは検査そのものが防御であり、報告に留めれば窓はどこにも存在しないままになります。

そこで `go-cooldown.yaml` は PR でゲートし、対象はその変更が追加 / 更新した require だけに絞ります。既に `go.mod` にあるものは grandfather するので、窓はこれから入るものに効き、引き継いだ状態で全ブランチが人質になることはありません。落とすのは **direct** だけです。indirect の版は MVS が選び、direct が要求する下限より上に固定されることがあります。それを下げるのは PR にできる操作ではないので、落としても打つ手の無い赤になります。よって indirect は報告に留めます。

窓は **7 日**で、この数字は npm からではなく本リポジトリの実績から採りました。Go モジュールには install script が無く `go mod download` は何も実行しないため、公開直後のバージョンが install 時点でマシンを奪うクラスは成立しません。窓が買うのは「悪意あるコードがビルドされ出荷されるまでの時間」です。履歴に当てると、7 日は `go.mod` を触った 47 コミットのうち 12 件を止め、14 日にしても 3 件しか増えないので両者の間に崖がありません。加えて「cooldown を満たす」と明示してバージョンを選んだ唯一のコミットが、実際には 7.4 日待っていました。

緊急の解除は [`go-cooldown-bypass.toml`](../go-cooldown-bypass.toml) が受け、エントリは必ず期限を持ちます。期限切れ・3 ヶ月より先の期限・`go.mod` の何にも当たらないエントリは検査を落とします。無効なエントリは効力も失うので、失効したバイパスが黙ってモジュールを通し続けることはありません。期限は `go.mod` が変わらなくても訪れます。定期実行があるのはそのためで、PR トリガーだけでは失効を一度も見られません。

定期実行はもう半分です。全 require を監査し、窓では決して落とさず、PR トリガーだけでは到来を見られないバイパスの期限を回収するために存在します。

3 つのパッケージはいずれも pnpm で解決します。`minimumReleaseAge` は窓内のバージョンを記録して後から警告するのではなく、解決の時点で拒否します（`minimumReleaseAgeStrict` により、窓が黙って外れるのではなくハード失敗になります）。事後に監査する対象が残らないため、この半分には監査ツールがありません。そのぶん重みは `pnpm-workspace.yaml` 自体のレビューへ全て乗るので、[`CODEOWNERS`](../CODEOWNERS) に登録しています。

#### DAST

`zap-api-scan.yaml` は、ここで唯一「動いているアプリケーション」を走査するワークフローです。他のセキュリティ検査がファイルを読むのに対し、これはサーバーをビルドし、シードを入れた Postgres に対して起動し、バンドル済みの OpenAPI 定義から得たエンドポイント一覧をもとに OWASP ZAP から HTTP を投げます。

ツールの選定はこの形から決まりました。GitHub の code scanning テンプレート一覧にある DAST 6 件のうち 4 件はベンダー側でスキャンが走り、GitHub-hosted runner の内部にしか存在しない API へは到達できません。runner 内で走る残り 2 件はいずれも有償トークンが必須です。ZAP は資格情報を要さず、ジョブの内部からスキャンできる唯一の選択肢で、そもそも短命な対象を見られるのはこの性質によります。

**スキャンは認証済みで走ります。そしてここが最も壊れやすい部分です。** 未認証のスキャンは保護されたオペレーションから 401 を集めて表層で止まり、完走したように見えて完走していません。ジョブは `dast` の環境プロファイルで走り、`ci` が使う dev 限定スタブではなく [`docs/design/auth.md`](../../docs/design/auth.md) が述べる JWKS backed の実 authenticator を配線します。したがって資格情報は mock 認証サーバーが実際に署名した JWT で、スキャンは毎リクエストで署名検証・`typ` 判定・`kid` 解決を通します。ジョブは ZAP を起動する前にその資格情報が通ることを確認します。この確認を失うと検査が赤くなるのではなく、スキャンの守備範囲だけが黙って縮みます。

**報告専用であることは、決めた結果であって省略ではありません。** [`.github/zap/rules.tsv`](../zap/rules.tsv) のしきい値は、この API が現に何を返すかから導いたものです。これでマージをゲートすれば、しきい値が想定していない検出で pull request を落とすことになります。検出は `zap-dast` カテゴリで code scanning と artifact に上がり、ジョブが落ちるのは「スキャン自体が実行できなかった」ときだけです。ZAP は SARIF を出力しないため、JSON レポートをワークフロー内で SARIF へ写像しています。各検出を OpenAPI バンドルへ紐づけるのは、そのファイルこそ検出対象の面を記述したものであり、実在するファイルを指すことが code scanning 上の辿りやすさになるからです。

しきい値とスキャン対象の面は、向ける先の API に対して導き直される前提です（[セットアップ手順の Phase 17](../../docs/get-started/setup-repository.md)）。

#### エージェント設定スキャン

`trustabl.yaml` は、ここの他のどの検査も読まない面——AI エージェント設定そのもの——を走査します。zizmor と Checkov はワークフロー定義を、CodeQL と Go の linter はソースを読みますが、subagent の `tools:` 付与や skill の `allowed-tools:` を解釈するものはありません。このリポジトリで効くのは Claude の subagent / skill ルールパックです。エンジンは OpenAI・Google ADK・LangChain・CrewAI・MCP 向けのパックも同梱しますが、ここでは何も検出しません。

**この 1 ステップでは別々にバージョン付けされた 3 つの成果物が動き、アクションを固定して固定できるのは最初の 1 つだけです。** アクションは実行時にベンダーのリリースからエンジンバイナリを取得し、エンジンはさらに 2 つ目のリポジトリからルールパックを clone します——どちらも既定は可動先です。既定のままだと、このリポジトリの他のピンが守っているクールダウン窓の外側で、未レビューの第三者コードが毎週ランナーに載ります。そのためワークフローは 3 つとも明示します。アクションは [`.github/actions-pin.toml`](../actions-pin.toml) 経由の SHA、エンジンはリリースタグ、ルールパックはタグです。弱いのはルールパックで、エンジンはこの入力をブランチとタグには解決しますがコミットには解決しないため、タグの張り替えは黙って取り込まれます。エンジンは実際に clone した SHA をログに出すので、張り替えはそこに現れます。

**報告専用であり、その理由はベースラインにあります。** subagent ルールは `Bash` の付与を一律に検出しますが、`.claude/agents/` 配下の読み取り専用レビュワーはいずれもそれを持っています——`git diff` や `go build` を走らせるための付与なので、検出が指しているのは欠陥ではなく設計です。severity ゲートを置けば初回からそのベースラインで落ちます。有用なのは skill パックの側で、狭い `Bash(git status:*)` 形を意図した箇所に裸の `Bash` が入っているのを捕まえます。`allowed-tools` は sandbox ではなく自動承認リストなので、この差は実在します。検出は step summary と `trustabl` アーティファクトで人に届きます。

SARIF アップロードと sticky な PR コメントはどちらも切っています。それぞれ書き込みスコープ——`security-events: write` と `pull-requests: write`——を第三者バイナリへ渡す代償を伴い、報告専用のスキャナにその要求権はありません。これによりジョブは `contents: read` のままに保たれます。

#### ランナーのハードニング

このディレクトリの全ジョブは `step-security/harden-runner` を `egress-policy: block` と、そのジョブ専用の `allowed-endpoints` とで先頭に置いています。外向き接続はすべてこの一覧と照合され、載っていない宛先は遮断されます。侵害されたアクションやツールの推移的ダウンロードは、そのジョブが本来必要としない宛先へ持ち出すことができません。ファイル改変の記録は従来どおり併走します。

このステップは**全ジョブにインラインのまま置きます**。これは好みではなく制約です。ローカルの composite action（`uses: ./.github/actions/*`）はリポジトリが checkout 済みでなければ解決できず、harden-runner は checkout の**前**に走る必要があります。checkout 自体が外向き通信であり、それを守るのが目的だからです。括り出せば、塞ごうとしている窓がそのまま開きます。この案は繰り返し再提案されますが、答えは「検討して見送った」ではなく「そもそも成立しない」です。

**固定されていないのは、その一覧を何処から持ってくるかです。** 正本は [`.github/egress.toml`](../egress.toml) です。`make egress-apply` が各ジョブへ書き込み、インラインのブロックが正本からずれていれば `make egress-check` が落とします（pre-commit フックと `egress-check.yaml` の両方）。

**ジョブが宣言するのは能力クラスであって、ホストの列挙ではありません。** ジョブが何処へ到達するかは、そのジョブが**何をするか**（ツールを入れる / イメージを作る / DB を起動する）から決まり、ジョブそれ自体の性質ではありません。実行は `make` → docker → コンテナ内 `mise` と潜っていくため、必要な宛先はジョブの YAML には現れません。クラスは 4 つで足り、ジョブは自分に当たるものを名指しします。

| クラス | エンドポイント | 対象 |
| --- | --- | --- |
| `base` | harden-runner 自身の agent、GitHub の API / web / codeload、`objects` / `raw` / `release-assets.githubusercontent.com`、`*.actions.githubusercontent.com`、`*.blob.core.windows.net` | **全ジョブへ暗黙に適用**（checkout、action の取得、artifact のアップロード）。`classes` へは書きません |
| `mise` | mise 自身の配布元と、`mise.toml` が解決に使う全 backend: aqua / GitHub リリース、Go のツールチェーンと module proxy、`downloads.sqlc.dev`、npm レジストリと `get.pnpm.io`、`astral.sh` と PyPI。加えて Sigstore（mise は各ツールの GitHub artifact attestation をここで検証します） | ツールを入れる全ジョブ。`setup-go` しか使わないジョブもこのクラスを名指しします。module proxy はここに居り、Go 専用の細いクラスを設ければ判定を 1 つ増やすだけだからです |
| `image` | Docker Hub の各ホストと CDN 2 種、`mirror.gcr.io`、`ghcr.io`、`pkg-containers.githubusercontent.com`、および Alpine / Debian のパッケージミラー。`mise` を継承します（イメージのビルドがコンテナ内で `mise install` を走らせるため） | イメージの build / push、service container、Trivy の DB と checks bundle、`make` 経由で docker を起こすもの全般 |
| `db` | PGDG の apt リポジトリと、Postgres の service container が導入時に参照する Ubuntu のアーカイブミラー | Postgres を起動するジョブ |

本当にそのジョブ固有のものは、当該ジョブの `extra` に書きます。スキャナのデータソース（`semgrep.dev`、`api.osv.dev`、`vuln.go.dev`）、デプロイ先、通知の `hooks.slack.com`、DAST の `zaproxy.org`（ZAP は起動時に add-on のマニフェストを解決します）などです。 2 つ目のジョブの `extra` にも現れたホストは、クラスへ移すべきものです。

**クラスは意図的に粗く保ちます。** 細かく割れば allowlist は締まりますが、そのぶん分類の判定点が増えます。そしてこの仕組みが取り除こうとしている失敗は、まさにジョブのクラス判定を誤ることです。必要より少し広いクラスでも、その外側は依然として全部遮断されます。クラスを取り違えたジョブは落ちます。

エンドポイントを足すときは `.github/egress.toml` を直します。能力から導けるものはクラスへ、導けないものはそのジョブの `extra` へ。そのうえで `make egress-apply` を実行し、生成されたブロックをコミットします。インラインのブロックを手で書き換えてはいけません。`make egress-check` が弾きます。

遮断された宛先は harden-runner の実行サマリに拒否接続として現れます。ジョブ自身のログからは理由が読めない失敗のとき読むべきはそこで、対処はクラスか `extra` を広げることであり、`audit` へ戻すことではありません。

**1 つだけ失敗の出方が逆転しているジョブがあり、その旨は当該ファイルに書いてあります。** `trufflehog.yaml` は候補の資格情報を発行元サービスへ問い合わせて検証しますが、その発行元の集合には上限がありません。ここでのエンドポイント漏れはジョブを赤くせず、本物の漏洩を「未検証」に変えます。このワークフローは検証済みのみを報告するため、結果として黙って緑になります。TruffleHog の検出が消えたときは allowlist の漏れをまず疑ってください。`egress-policy: audit` を持つ唯一のジョブで、SSOT 上もそう宣言してあるため `allowed-endpoints` を一切持ちません。両者が食い違えば `make egress-check` が落ちます。

テンプレート作成時に読み直す価値のある前提が 2 つあります。`deploy-app.yaml` の build ジョブは `ghcr.io` と公開 Sigstore インスタンスを前提にしており（その `extra`）、deploy ジョブは placeholder でクラスを 1 つも宣言しないため `base` だけを持ちます。実際のデプロイを結線する際は、その環境の control plane のホストをそのジョブの `extra` へ足す必要があります。`id-token` の交換も外向き通信だからです。

### デプロイ（Push）

|ワークフロー|ファイル|トリガー|説明|
|---|---|---|---|
|Deploy App|`deploy-app.yaml`|production/staging/develop への push|Docker イメージのビルド・プッシュ（cosign による image 署名 + provenance / SBOM attestation）、マイグレーション実行、デプロイ|
|Deploy Docs|`deploy-docs.yaml`|production への push（docs 変更時）|ドキュメントポータルを GitHub Pages にデプロイ|

### ドキュメント生成（Push）

|ワークフロー|ファイル|トリガー|説明|
|---|---|---|---|
|Auto-generate Docs|`auto-generate-docs.yaml`|release/* への push|`release/vX.Y.Z` のブランチ名から OpenAPI `info.version` を同期し、OpenAPI バンドル / 埋め込み spec / ドキュメント、ER 図、ポータルドキュメントを自動生成|
|Graphify Sync|`graphify-sync.yaml`|release/* への push|内容が変わったコードファイルをナレッジグラフへ再抽出し、結果を PR で出す。決定論的な半分だけを担い、併せて意味抽出がどれだけ溜まったかを報告する|
|Graphify Extract|`graphify-extract.yaml`|手動実行（`workflow_dispatch`）|変更されたドキュメントに対する意味抽出をアシスタントとして走らせ、PR で出す。sync と別の workflow なのは、書き込み権限を持つアシスタントを走らせるのがこれだけだから。`CLAUDE_CODE_TOKEN` が未設定なら自分をスキップするので、テンプレート由来のリポジトリが用意していないトークンを使うことはない|

### アシスタント（コメント）

|ワークフロー|ファイル|トリガー|説明|
|---|---|---|---|
|Claude|`claude.yaml`|プルリクエストのコメント / レビューでの `@claude`|オンデマンドでプルリクエストに対して Claude Code を実行|
|Closed Loop Summarize|`closed-loop-summarize.yaml`|issue に `feedback/needs-summary` ラベルが付いたとき、または手動実行|手元で読解できなかった Feedback Issue の、読解にあたる節を埋める。通常経路はトランスクリプトを持つマシン上で読むので、これが動くのは縮退経路だけ — `feedback` で待ち受けると読解済みの窓まで読み直す。観測ブロックには**触れない** — そちらは決定論的な半分であり、週次の集計が読むのはそこ|

どちらも `CLAUDE_CODE_TOKEN` が無ければ静かにスキップする。派生リポジトリはワークフローを
受け取るがシークレットは受け取らないため、そこで赤くなると任意レイヤの不在を失敗として
報告することになる。

### フィードバックループ（定期実行）

|ワークフロー|ファイル|トリガー|説明|
|---|---|---|---|
|Closed Loop Weekly|`closed-loop-weekly.yaml`|週次、または期間を指定した手動実行|期間内の Feedback Issue を集め、分類ラベルでまとめ、クラスタごとにスコアを付けて run summary へ出す。モデルは読まない — 数字はすべて Issue が既に持つ観測ブロックから数える。`closed-loop-summarize.yaml` と違ってトークンが要らないのはそのため|

`make closed-loop-weekly` に委ねず定期実行にしているのは、効果の測り直しが
[ADR-0008](../../docs/adr/0008-agent-environment-alignment.ja.md) がループの条件として置いた段であり、
誰にも促されない段は実行されなくなるからである。

## 共通 Composite Action

再利用可能な composite action は [`.github/actions/`](../actions/) に配置しています：

|アクション|目的|
|---|---|
|`setup-mise`|digest を検証したうえで固定版の mise バイナリを導入し、続いて呼び出し側が指定したツールを導入する（[Mise のセットアップ](#mise-のセットアップ)を参照）|
|`setup-postgres`|Postgres サービスコンテナの待機・初期化（DB 依存ジョブで使用）|
|`upsert-pr-comment`|マーカーで既存コメントを検出して update / create する PR コメントの upsert。Commit / UpdatedAt フッターを共通付与し、結果コメント系ワークフローで使用。`status: success` は既存コメントを更新するが新規作成はしない|
|`notify-detail`|スキャナの Markdown サマリを検出行だけへ削る。検出通知が「何を見つけたか」を名指ししつつ、PR コメント向けに書かれた散文を持ち込まないようにするため|
|`osv-scan`|osv-scanner を実行し、各 finding をリリースゲートの深刻度ポリシーで分類する。OSV の報告用ワークフローと OSV リリースゲートで共用|

## 補足

- `.github/workflows/**` と `.github/actions/**` のコメントおよびログ文言は **英語**で書く（`echo` の出力と
  `::error::` アノテーションを含む）。このリポジトリの日本語コメント規則は Go コード・テスト名・PR・応答を
  対象とし、読み手が workflow ログと Actions エコシステムである CI 定義には及ばない。内容基準
  （[`docs/rules.md`](../../docs/rules.md) § Comment Rules）はそのまま適用される — 手順のナレーション・
  開発経緯・言い換えは書かず、非自明な Why は残す
- `auto-generate-docs.yaml` は `auto/docs-update/<base>` というブランチ名で auto-PR を作成（release base ごとに 1 ブランチを `delete-branch: true` で再利用）。再帰実行を避けるため自己ブランチでは workflow をスキップ
- **アプリケーションや Realtime Delivery の contract test を起動するジョブは、service container を 2 つ追加する。** `dynamodb_local` は Realtime Delivery のストア、`goaws` はそのファンアウトが publish する先の SNS / SQS エミュレータで、それぞれ `docker-compose.yaml` 上の同名サービスと同じバージョンへ固定し、CI とローカル開発を乖離させない。service container は command も config file も受け取れないため、両者ともイメージの既定値で動く。そのため `dynamodb_local` は compose が与える永続ボリュームではなく `-inMemory` になり、`goaws` は compose が設定する `000000000000` ではなくイメージに焼き込まれたアカウント id `100010001000` で応答する。`env/.env` の topic ARN をそのまま CI ジョブへ持ち込んでも解決しないのはこのためである
- **実サーバを起動するジョブは、先に `go run ./cmd/ realtime-init` を走らせる。** Realtime Delivery の table と topic はこの one-shot が作成するのであってアプリ起動時には作られないため、これが無いと起動時 probe の接続先が存在せず、グラフが壊れているように見える形で失敗する
- **auto-PR は `GITHUB_TOKEN` ではなく GitHub App のトークンで作成する。** ワークフローが自分の `GITHUB_TOKEN` で起こしたイベントを GitHub は抑止するため、そのトークンで作成した pull request はワークフローを 1 本も起動しない。そして報告されない required context は「該当しない」とは読まれず、待つ対象の無いまま待ち続ける。そこで `auto-generate-docs.yaml` / `graphify-sync.yaml` / `graphify-extract.yaml` は `AUTO_PR_APP_ID` / `AUTO_PR_APP_PRIVATE_KEY` から installation token を発行し、`contents: write` と `pull-requests: write` に絞って渡す（ブランチを push して pull request を開くのが仕事のすべてなので）。App の登録は人間の手順で、未登録のうちは警告を出して `GITHUB_TOKEN` へフォールバックする。pull request は開くが、チェックは付かず、required がある限りマージはできない。3 本のコミットメッセージから `[skip ci]` を外したのも同じ理由による。このキーワードは `push` **と `pull_request`** の両方を抑止するが、これらの workflow はそもそも `auto/` ブランチを見ていない（`push` フィルタは `release/**`）ので、実際に抑止していたのは auto-PR 自身のチェックだけだった
- `graphify-sync.yaml` と `graphify-extract.yaml` は `auto/graphify-update/<base>` 上で同じ形を取る。2 本を別 workflow にしているのは意図的で、後者は `contents: write` を持つアシスタントを走らせるため、条件分岐として前者へ畳むとリリース push のたびに走るジョブへその権限が付く。`graphify-extract.yaml` が `--max-turns` を持たないのも同じ性質による。抽出は変更されたドキュメントの数で長さが決まる連続処理なので、ターン上限は途中で切ってグラフを書きかけのまま残す。ここに置くべき上限は `timeout-minutes` の方である。そもそもグラフをリリース線で更新している理由は、これがどのソースファイルの移動でも書き換わる 45 万行の単一ファイルであり、並行する feature ブランチが必ず衝突するうえ、生成物の常道である「正本から再生成」が意味抽出の半分にモデルを要するため使えないからである。そこで `graphify-check.yaml` は `graphify-out/` への変更を含む pull request を落とし、sync ブランチ自身を除くすべてで `BASE` を渡す。生成物をまだ持たない base はツール自身が設ける例外で、グラフの導入は何とも衝突せず、ここで止めると生成物に入り口が無くなる
- デプロイ系 workflow の target ブランチ（`production` / `staging` / `develop`）はすべてブランチ保護を有効化。マージは必ず PR レビュー経由
- セキュリティスキャンのトリガーは上記「セキュリティのトリガーマトリクス」でツールごとに定義。CodeQL / Trivy で high-severity が出るとブランチ保護ルールでマージブロック
- `trivy-fs.yaml` と `osv-scanner.yaml` は**チェックを落とさない**。修正版の有無に関わらず全 finding を code scanning と PR コメントへ載せ、ブロックの判定は上記のリリースゲートに委ねる。これにより、既知の脆弱性が黙って昇格に載ることはなく、かつ通常の PR がその PR の持ち込みでない脆弱性に足止めされることもない
- `trufflehog.yaml` は**検証済み**シークレットのみを報告し、生のシークレット値をジョブログ / PR コメント / artifact のいずれにも出さない。正規表現ベースの検知は `--redact` 付きの gitleaks が担当
- **PR にコメントするジョブには secret を渡さない**。シークレットマスキングが効くのは、ランナーがジョブ出力をログ表示用に捕捉する経路だけ。ステップが `tee` でファイルへ落としたバイトはそこを通らず、`upsert-pr-comment` は本文をまさにそのファイルから読む。つまりログ上はマスク済みに見える値が、公開コメントには生のまま載る。現状どの検査ステップにも secret は渡っていないが、それを維持するのが `make actions-comment-secret-lint` で、当該アクションを使うジョブに `GITHUB_TOKEN` 以外が渡ると失敗する。検査ステップに secret が要るなら、コメントしないジョブへ分離する。なお検査が読むのは secrets の直接参照だけなので、`needs.<job>.outputs` を経由すればすり抜ける。支えているのは lint ではなく規約の側
- **`upsert-pr-comment` は「投稿者が bot」かつ「本文冒頭がマーカー」で自分のコメントを同定する**。公開リポジトリではマーカー入りのコメントを誰でも投稿でき、しかも本リポジトリのワークフローはすべて同じ bot で投稿するため、マーカーだけでも投稿者だけでもコメントは同定できない。PR 提出者が、あるワークフローのログに別のワークフローのマーカーを混ぜられれば、その別ワークフローを誤ったコメントへ誘導できてしまう。アクションが書いた本文は必ずマーカーで始まるが、混入させたマーカーはそうならない。したがって `github-token` は bot として投稿するトークン（`GITHUB_TOKEN` か GitHub App トークン）である必要がある。PAT はユーザーとして投稿するので自分のコメントを見つけられず、実行のたびに新規コメントが増える
- zizmor の例外設定は `.github/zizmor.yml`。`ignore` はファイル単位であり、同じ audit を踏む新規ワークフローは意図どおり落ちる。恒久的な allowlist ではなく、元の指摘を直したらエントリを消す運用
- **`run:` 本文へ展開された式はコードであり、それを見ているのは zizmor だけ**。`${{ }}` の置換はシェルが構文を解釈するより前に起きるため、未クオートの `github.event.*` はコマンドを終わらせて攻撃者のコマンドを始められる。shellcheck を通すゲート群は構造的にこれを見られない（理由は [`scripts/README.md`](../../scripts/README.md) の `actions-shellcheck/` の行を参照）。zizmor の `template-injection` は代わりに展開位置そのものを判定し、式の出自が攻撃者制御可能かで重み付けする。`make actions-zizmor` が `make actions-lint` の中ではなくその隣に pre-commit フックへ載っているのはこのため。式を `env:` へ束ねてシェル側では `"$VAR"` を読む形にすれば、値はデータとして届く
- `auto-generate-docs.yaml` の `Detect changes` ステップはカバレッジ HTML / SchemaSpy のタイムスタンプ揺れを除外し、無意味な PR が発火しないよう設計
- GitHub は 60 日コミットが無いとスケジュール実行のワークフローを自動的に、しかも黙って無効化する。これを回避し続けることは本テンプレートの責任範囲外であり keepalive ジョブは用意しない。動きが止まった作成先では Actions タブから再有効化が必要になる前提で扱う
- テンプレート由来のリポジトリは全ワークフローが `disabled_fork` 状態で作られ、この状態では何も動かない。`make enable-workflows` が列挙して一括で有効化する（冪等なので再実行して差し支えない）
- **`claude.yaml` の認可**。誰が Claude を呼べるかはワークフロー側の allowlist ではなく action 自身の書き込み権限チェックで決まる。代替案はいずれも fork で破綻する。アカウントをワークフローに直書きすると fork 先のオーナーが自分のリポジトリで締め出され、リポジトリ変数に持たせても変数は fork に引き継がれないため空に解決されて誰も呼べない。権限チェックはワークフローが動いているリポジトリに対して解決されるので、どこでも設定なしで正しく振る舞う。これを無効化する 2 つの input は意図的に未設定である。`allowed_non_write_users` はチェック自体をバイパスし、`allowed_bots` はインストールも書き込み権限も不要な App を通す。ワークフローの `if:` は `github` コンテキストしか読まず、無関係なコメントで runner を起動させないためのものであって権限を与えない。なお「誰が呼べるか」を絞っても fork PR に仕込まれたプロンプトインジェクションは防げない（呼ぶ人間は信頼できても Claude が読む diff は信頼できない）。`contents` を read のまま据え置いているのはそのためである
- `.spectral.yaml` と `.trivyignore.yaml` は `.github/zizmor.yml` と同じ方針。一括無効化はせず、各エントリに根拠となる ADR か実装を書き、抑止はパス（または JSON ポインタ）単位に閉じる。これにより同じルールを踏む新規ファイルは引き続き落ちる
- `fuzz.yaml` は PR ではなく定期実行。fuzz はランダムな corpus を探索するため、マージ可否をそれに賭けさせないための判断。クラッシュの再現入力は `testdata/fuzz/` へコミットされ、通常の回帰テストとして再生される

### PR コメントのフェンス

**攻撃者が内容を制御できるテキストを囲むフェンスは、そのテキストから長さを決める。固定長にはしない**。`upsert-pr-comment` は本文中の最長バッククォート連 + 1 をフェンス長とするが、これが働くのは `details-summary` 経路だけである。この入力が無い呼び出しでは本文を素通しする — 見出し・表・自前の `<details>` をそのままレンダリングさせたい呼び出しが複数あるためで、一律フェンスは表示を壊す。したがって素通し経路で本文の一部を自前でフェンスする呼び出しは、フェンスの責任を自分で負う。固定 3 連は、ソース行をそのまま再現する本文には閉じられる — lint が PR 提出者の書いたファイルを引用すればブロック内に 3 連が入り、以降が bot 名義の生 Markdown としてレンダリングされる。`sql-lint.yaml` は自前でフェンスを組むためログごとに長さを計算し、`capability-diff.yaml` はフェンスをアクションへ委ね step summary だけを包む。あわせて本文はアクションの `max-length` を下回るよう呼び出し側で切り詰める。この切り詰めはフェンスより**前**に適用されるため、そこで削られた本文は閉じフェンスを失う。機械的に判定できる 3 点 — `run:` からリテラルのフェンスを出さないこと、複製された `fence_for` が同一であること、素通しのワークフローが inline code span へ値を補間しないこと — は `make actions-comment-fence-lint` が見るが、「その本文が攻撃者制御か」は判定できないので、支えているのは規約の側

**同じ規則は inline code span にも及ぶ。span は長さ 1 のフェンスでしかない**。補間した値にバッククォートが 1 つあれば span はそこで閉じ、以降は生 Markdown に戻る。刺さるのはパスの場合である。パスに使えない文字は NUL と `/` だけなので、バッククォート・`@`・リンク構文はいずれも使え、`git diff --name-only` で得たファイル名は手を加えられないままコメントへ届く（`core.quotePath` がエスケープするのは非 ASCII と制御文字であって、これらではない）。したがって素通しの呼び出しは、リポジトリ由来のパスを span にも裸の Markdown にも置かず、上と同じく一覧全体をその一覧から決めた長さのフェンスで包む。`gen-*-artifacts-check.yaml` 4 本と `sync-versions-check.yaml` のファイル一覧はこの形。本文をレンダリングさせ続ける必要がある場合、本文全体を包む解は採れない。`image-scan.yaml` の SBOM summary が見出しと強調ラベルから始まるのは、この本文がログではなくレビュアーが読むインベントリだからである。そこではテンプレートを生の Markdown のまま残し、代わりに値ごとに長さを値自身から決める。スカラーは自身の最長バッククォート連 + 1 の span へ入れ（CommonMark が剥がす空白で両端を埋めるので、値がバッククォートで始まっても終わってもよい）、一覧は上と同じくその一覧から決めた長さのフェンスで包み、digest は `^[0-9a-f]{64}$` に一致しなければ `unknown` へ落とす。SBOM 由来の文字列はパスと違って何にも有界でないため、長い連を潰し、値ごとに長さの上限を掛けてからフェンス長を決める。この上限こそが仕組み全体の土台である。フェンスから閉じ行を奪う `max-length` の切り詰めは、span からも同じように閉じデリミタを奪い、閉じられなかった span は以降の本文を生 Markdown へ戻すからである。したがって本文に寄与するものはすべて、合計が上限の内側に収まるよう有界にしてある。lint はこれらのいずれにも乗っていない本文のために、ファイル単位の除外機構を残している。運用は `.github/zizmor.yml` と同じで、エントリは追跡 issue を明記し、その指摘を直したら消す。恒久的な allowlist ではない。変数経由や `jq` の連結で組んだ span は検査から見えないので、ここでも規約が lint を上回る。なお検査は `details-summary` のキーの有無ではなく**値**を読む。この入力が空だとアクションは本文を素通しへ落とすためで、したがって `details-summary` には静的な非空文字列を渡す。空になり得る式は素通し扱いとして検査対象に含める

### Mise のセットアップ

mise 管理下のツールを要するワークフローはすべて [`.github/actions/setup-mise`](../actions/setup-mise/action.yaml) を使います。この action は固定版の mise バイナリをリトライ付きでファイルへダウンロードし、そのバイナリをバージョンと digest でキャッシュし、実行のたびに SHA256 を検証します。キャッシュから復元できたというだけで信頼することはありません。不一致なら捨てて再ダウンロードし、2 度目の不一致でジョブを落とします。

呼び出し側の任意入力 `tools` の導入は、バイナリが検証を通ったあとで行います。ツールのバージョンは [`mise.toml`](../../mise.toml) に残り、呼び出し側が渡すのは必要なツールの backend 修飾付き指定だけです。`make actions-mise-pin-lint` は Actions Lint の一部として走り、action のバージョン・digest・キャッシュキーの食い違いを弾きます。mise 自体を更新するときは、チェックサムをリリースの `SHASUMS256.txt` から取得し、3 つの値をまとめて更新してください。

### キャッシュの安全性

**キャッシュの安全性**。キャッシュは branch-scoped であり、run が復元できるのは自分の ref とデフォルトブランチのキャッシュだけなので、pull request の run が後続の `release/*` push が読むキャッシュを書くことはできない。通常の CI ワークフローでキャッシュを有効なままにしているのはこのため。汚染が成立する経路は 2 つある。1 つは、信頼できない PR のコードを信頼された scope で実行しつつキャッシュを保存する場合。`pull_request_target` と `workflow_run` は base ref の scope で動くため、そこで PR head を checkout するワークフローを書くと、そのキャッシュが特権 run の読む場所に残る。この 2 つを組み合わせてはならない。信頼できないコードを扱うワークフローではキャッシュを無効にする。もう 1 つは、**同じ branch scope を共有しながら権限が異なるワークフロー間**。protected branch への push で走る通常のワークフローが複数あるため、そのどれかが侵害されると `security-events: write` を持つジョブが復元・実行するツールキャッシュを残せてしまう。そのため当該権限を持つジョブはすべて `cache: false` とし、インストールが遅くなる代わりに、低権限の run が書きえた成果物を引き継がないようにしている
