> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。直接編集せず、更新は `SKILL.md` 側から流してください。

# Repo Ops Runbook

このリポジトリで繰り返し踏む運用 gotcha の具体的な復旧手順集。ワークフローではなくルックアップ表なので、症状を見つけて対応コマンドを打つ。破壊的な手順（DB データ削除・ツリーの `chown`・他 checkout が依存するコンテナの停止）は先にユーザーへ伝える（`CLAUDE.md` 準拠）。

以下はほぼすべて、次の3つの事実から説明できる:

1. **コード生成は Docker のツールランナーコンテナ内で root 実行される**（`go_tool_runner` / `node_tool_runner` / `python_tool_runner`）。`.:/app` バインドマウント越しなので生成物は root 所有で返り、ツールはホストではなくイメージ側に居る。
2. **インフラは単一の共有 compose プロジェクト**。`database` / `observability` / `garage` は固定プロジェクト `gobp-shared` に**全 checkout で 1 インスタンス**だけ起動し、checkout 毎に分かれるのは `api_server` / `mock_auth_server` のみ。worktree の分離軸はポートではなく**データベース名**（`wt<N>_local` / `wt<N>_test`）。設計の正本は `docs/maintenance/db-worktree-pool.md`、全体像は `docs/maintenance/local-environment.md`。
3. **`make go-lint` / `fix` / `test` はホスト実行**（mise 経由）。「全部 docker 化」の例外であり、ローカルと CI が食い違う原因になる。

## 契約

| | |
| --- | --- |
| **所管** | 既知の運用症状 → 是正手順の索引（§1-21）と、どの文書が正本かの所在（§0） |
| **しないこと** | 索引に無い手順を発明する / 「未定義」を宣告する / 設計判断 / 実行承認の代行 |
| **開始条件** | 症状・失敗したゲート・想定外の挙動が提示されたとき |
| **停止条件** | 症状が索引に無い（`how-to` / `repo-truth` へ）、資料が矛盾する、要求が許可範囲を超える |

## 症状インデックス

| 症状 | 修正がある場所 |
| --- | --- |
| `docker compose ps` が空 / "port 5432 already allocated" / サービスが見つからない | §1 |
| `schema.gen.sql` や sqlc 出力がドリフトし `gen-db-artifacts-check` が落ちる | §2 |
| `make gen-query` が DB に届かない / 複数 checkout が互いを壊す | §3 |
| `git add` / `restore` が生成ディレクトリで permission denied | §4 |
| `make slot-acquire` 後にテスト・マイグレーションが別 DB を見る | §5 |
| ブランチ切り替え直後に統合テストが落ちる | §5 |
| pre-push の `secret-scan` が自分の追加していない秘密を検出する | §6 |
| `sample-removal-check` が CI で落ちる | §99 <!-- sample-api:line --> |
| 触っていないのに `env/.env` が dirty | §7 |
| ローカル golangci-lint が CI と食い違う / `golangci-lint: not found` | §8 |
| `commitlint: not found` / `orval: not found` / ツールが古い | §9 → `tools-upgrade`（版の更新） |
| コンテナ経由のゲートが `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` で落ちる / Docker 内でだけ落ちる | §9 |
| 触っていないパッケージの依存が無いとコンテナ側のゲートが言う / `make` では落ちるのに素の `docker compose run` では通る | §9 |
| 自分の変更と無関係な理由でフックが落ちる | §10 |
| 複数の worktree を開いている状態で、変更と無関係にゲートが落ちる／異常に遅い | §19 |
| `make go-lint` がスキップ・低速化・CI 委譲された理由を知りたい | §19 |
| `pin-images-check` / `pin-actions-check` が未固定・未登録で落ちる | §11 → `images-pin` / `actions-pin` |
| `tool-cooldown` / `go-cooldown` が宣言したばかりの版を弾く | §20 → `supply-chain-triage` |
| ランタイムの昇格が、まだ上げられないツールを経由してゲートを壊す | §20 → `tools-upgrade` |
| pre-commit の "Migration version gap / duplicate" | §12 |
| ローカルの S3 呼び出しが 503 を返す | §13 |
| 特定環境向けイメージのビルド | §14 |
| CI で `sync-versions` のドリフト | §15 → `go-upgrade`（Go の版上げ） |
| 新しい worktree で air やイメージビルドが `go: inconsistent vendoring` になる（`vendor/` 未生成） | §18 |
| どのドキュメントが答えを決めるのか分からない / `grep` が対訳・生成物に埋もれる | §0 → `repo-truth` |
| ベースを取り込んだ後の衝突 / 生成物・pin lockfile の中にマーカーが出る | §21 → `resolve-merge` |
| マージ後、ブランチが触っていないファイルで `gen-*-artifacts-check` が落ちる | §21 → `resolve-merge` |

**スキルが修正を所有している場合、索引はそれを名指しし、節は境界で止まる。** 解決がコマンドではなく手順
である症状は、その手順の所有者のものである —— pin lockfile は `images-pin` / `actions-pin`、弾かれた版は
`supply-chain-triage`、版上げは `tools-upgrade` / `go-upgrade`、マージは `resolve-merge`、「どの文書が
これを決めるのか」は `repo-truth`。節は診断 —— 症状が何を意味するか、なぜゲートが鳴ったか、何が破壊的か
—— を保持し、手順は再掲せずに渡す。再掲された手順はコピーであり、所有者が変わったときに腐るのは
コピーの側だからである。

節の中からではなく索引から振り分けることには意味がある。節を読み終えてからスキルへの委譲を知る読者は、
その委譲が節約するはずだった読解を既に払い終えている。

<!-- sample-api:begin -->
**`make setup-remove-sample-api` が削除する節は連番の外（§99）に置く。** 連番の中にあると、削除が
このリポジトリの出荷する runbook の番号に恒久的な穴を残す。連番の外なら、削除しても他は何も変わらない。
削除される節はそのように採番し、新設の節には次の連番を与える。
<!-- sample-api:end -->

## 0. 正本の見つけ方

このツリーの Markdown の大半は、読んではいけない日本語ミラーか、コードに遅れて追随する生成物のどちらかである。そのためリポジトリ全体を素朴に検索すると、実際に答えを決めている唯一のファイルが埋もれる。追跡されている `*.md` はおよそ 1,000 件あり、その **4 割超が `*.ja.md` 対訳**、**72 件が README を写した生成物 `docs/portal/guides/**`**。さらに `docs/godoc/**` が約 1,800 件、`docs/db-schema/**` が約 400 件ある。

### 正本の所在

| 知りたいこと | 読む場所 | 正本に見えるが違うもの |
| --- | --- | --- |
| make ターゲットの実体 / どのターゲットが X をやるか | `.makefiles/**/*.mk`、索引は `.makefiles/README.md`、`make help` | `docs/portal/guides/make.md` — 生成物で `.mk` に遅れる |
| レイヤ境界・生成物 / DTO / tx / コメントの規則 | `docs/rules.md` | — |
| システム構造とレイヤの責務 | `docs/architecture.md` | — |
| 変更の進め方（API / DB / ビジネスロジック） | `docs/development-flow.md` | — |
| テスト規約 | `docs/testing-conventions.md` + 当該レイヤ README の *Test Strategy* | — |
| なぜその設計なのか | `docs/adr/`（100 件超のレコード。ログは `docs/adr/README.md`） | — |
| サブシステムの仕組み（rest / worker / job / outbox / idempotency / o11y / auth） | `docs/design/README.md` と同ディレクトリ | — |
| ローカルの構成・compose の層分け・ホットリロード | `docs/maintenance/local-environment.md` | — |
| worktree スロットリングと共有 DB | `docs/maintenance/db-worktree-pool.md` | — |
| パッケージ単位の詳細と設計意図 | 最も近い `internal/**/README.md` / `pkg/**/README.md`（100 個超） | スキル本体 — スキルが README に従うのであって逆ではない |
| 環境変数 | `internal/config/envspec.go` + `model.go`、`env/README.md` の表 | `env/.env*` の値だけ |
| CI ゲートやフックが実際に検査する内容 | `.github/workflows/*.yaml`、`.lefthook.yaml` | — |
| ツール / ランタイムのバージョン | `mise.toml`（他はすべて派生 — §15） | `go.mod`・Dockerfile・README — いずれも派生物 |
| 生成物・保護パス・スコープ | `AGENTS.md` | — |

### ノイズを除いた検索

```bash
rg "<pattern>" \
  -g '!**/*.ja.md' -g '!docs/portal/**' \
  -g '!docs/godoc/**' -g '!docs/db-schema/**' -g '!docs/openapi/**' -g '!docs/coverage/**'
```

典型的な効果: ヒット数は半分以下に減る（例: `gen-query`、`NormalizeError`）。`rg` は `.gitignore` を尊重するため `vendor/` と `node_modules/` は既に除外されるが、生成された `docs/` 配下は*追跡されている*ので明示的な glob が要る。`*.ja.md` にヒットすること自体は**所在の手がかり**として有用（その話題が文書化されている証拠）だが、`AGENTS.md` の「`*.ja.md` を読まない」規則どおり、隣の英語正本を読むこと。

### 情報源が食い違うとき

`AGENTS.md` の指示優先順位に従う: `AGENTS.md` → `docs/rules.md` → `docs/architecture.md` → ユーザー指示。設計意図と実装方針については **README > Code > SKILL**（`back-prop` が強制する規則）。README と矛盾するスキルのほうが古い。コードと README が食い違うならそれはドリフトであり、表面化させる価値がある（`back-prop` が検出する）。

## 1. compose のプロジェクト解決 — 素の `docker compose` は別プロジェクトを指す

compose はプロジェクト名をディレクトリから導出するため、このリポジトリ（や worktree）で素の `docker compose <cmd>` を打つと、共有インフラではなく**空のプロジェクト**を相手にする。`.makefiles/docker/compose.mk` が `COMPOSE_PROJECT_NAME=gobp-shared` を export しているので、これが効くのは `make` 経由のときだけ。

```bash
docker compose ps                  # ← 空。プロジェクト = <ディレクトリ名>
docker compose -p gobp-shared ps   # ← 実際の共有インフラ
```

取り違えたときの結果: `docker compose up -d database` は**2台目**の Postgres を起動してホストポート 5432 を奪い合う。`docker compose exec database psql …` はサービスが起動していないと言う。`docker compose run --rm go_tool_runner make db-migrate-ci-up` は一時コンテナが別ネットワークに繋がるため `database` ホストを解決できない。

対処: インフラは `make` 経由で操作する。素の compose が避けられないならプロジェクトを明示する。

```bash
make infra-up                                    # database / observability / garage（+ garage_init）を起動
make serve                                       # infra-up + この checkout の app 層
make serve-stop                                  # この checkout の app だけ停止
COMPOSE_PROJECT_NAME=gobp-shared docker compose exec -T database psql -U postgres -l
```

`make infra-down` は**全** checkout / worktree のインフラを止める。実行前に確認すること。

## 2. DB 生成物のドリフト → `gen-db-artifacts-check` が落ちる

`database/gen/schema.gen.sql` と sqlc 出力（`internal/infrastructure/rdb/sqlc/gen/*.gen.sql.go`・`database/gen/*.gen.sql`）は `make gen-query` が生成する。`database/migrations/**`・`database/dml/**`・`docker/database/**` に触れば内容が変わるため、ソースだけコミットすると CI チェック（再生成 → `git diff`）が落ちる。

```bash
make infra-up
make gen-query                     # dump-schema → merge-dml → sqlc generate → fmt
git add database/gen internal/infrastructure/rdb/sqlc/gen
```

原則: **スキーマ / DML に影響する変更と、その再生成物は同じ変更に含める。** PR が SQL ソースに触っていないのに CI がドリフトを報告する場合は generator 側のドリフト（sqlc バージョン差・base の生成物が古い）＝ローカルで再生成するか、base を最新化してマージする。

## 3. `make gen-query` — インフラ起動が前提、かつ `gen_schema` は共有

`dump-schema` はもう自分の `local` データベースをダンプしない。専用の **`gen_schema`** データベース（`.makefiles/database/gen.mk` の `SCHEMA_GEN_DB`）を用意してテーブルを削除し、*このブランチの* migration で migrate-up してからダンプする。よって作業用 DB の残骸テーブルが生成コードへ漏れることは無くなったが、次の2点は残る:

- 実体は `docker compose exec database` 経由で共有 Postgres を叩くので、インフラ起動は依然必須（§1）。落ちていれば接続エラーで死ぬ。
- **`gen_schema` は唯一の共有インスタンス上の単一データベース名**。複数 checkout が同時に `make gen-query` を走らせると同じデータベースを作り直し、互いの出力を壊す。スキーマ生成は 1 checkout ずつ行うこと。（この制約は現在 `gen.mk` のコメントにしか無く、`docs/` には載っていない。）

## 4. `git` が生成ディレクトリを拒否する — root 所有になっている

ツールランナーはバインドマウント上で root 実行されるため、*新規作成*したファイルはホスト側で root 所有になる: `make gen-portal-docs` の `docs/portal/{guides,docs.json}`、`make gen-go-code` が作る mock ディレクトリ（例 `pkg/fs/mock`）、`docs/godoc/`、その他新規に増えた生成物。以降ホストの `git add` / `git restore` が `permission denied` で失敗する。

対処: コンテナ経由で所有権を返す（ホストの `sudo` より優先）:

```bash
docker compose run --rm --user root node_tool_runner chown -R $(id -u):$(id -g) /app/docs/portal
docker compose run --rm --user root go_tool_runner   chown -R $(id -u):$(id -g) /app/pkg/fs/mock /app/pkg/exec/mock
```

罠: root 所有を掃除するための `git restore docs/portal` は、`docs/portal` 配下に**手で入れた編集も巻き戻す**。chown 後に別コミットで再適用すること。

## 5. スロット取得後・ブランチ切り替え後に別 DB を見てしまう

`make slot-acquire` は `.gobp-db-slot` を書き出し、make がそこから `DB_NAME_LOCAL` / `DB_NAME_TEST`（`wt<N>_local` / `wt<N>_test`）を伝播する。ここに2つの罠がある:

- **`db-init` / `db-local-reinit` / `db-test-reinit` は `DB=local` / `DB=test` を直書きしている。** スロット取得後にこれらを叩くと自分のではなく*共有*データベースを作り直す＝それを使っている相手に対して破壊的。自分の DB は明示して指定する:

  ```bash
  set -a; . ./.gobp-db-slot; set +a
  make db-reinit DB="$DB_NAME_LOCAL"      # drop tables → migrate-up → seed
  make db-reinit DB="$DB_NAME_TEST"
  ```

- **素の `go test ./...` には `DB_NAME_TEST` が渡らない** — export しているのは make であってシェルではない — ため、黙って共有 `test` データベースへ繋ぐ。そこは別ブランチの migration が当たっているかもしれない。`make go-test` を使う（あるいは自分で export する）。

統合テスト（`internal/integration`・`internal/infrastructure/rdb/**`）は自分でマイグレーションしない。migrate + seed 済みのデータベースを前提にする。migration の異なるブランチへ切り替えたら先に作り直すこと。スロットの DB は `make slot-acquire` が、共有 `test` は `make db-test-reinit` が担当する。`db-init` より `db-reinit` を優先すること: `db-init` は先に `migrate-down` を走らせるため、`.down.sql` が失われたダーティスキーマからは復帰できない。

## 6. pre-push の `secret-scan` が自分の追加していない秘密を検出する

`.gitleaksignore` のエントリは `<path>:<rule>:<line>` 形式のフィンガープリントで、**行番号を含む**（例: `docker-compose.yaml:generic-api-key:134`・`env/.env:generic-api-key:64`）。該当ファイルを編集して行がずれると旧フィンガープリントが一致しなくなり、意図的に許容していたサンプル資格情報が新規検出として報告される。

```bash
make secret-scan          # 再現する。出力に新しいフィンガープリントが載る（値は redact 済み）
```

そのうえで `.gitleaksignore` の該当行を新しい行番号へ更新する（説明コメントは残す）。これを行ってよいのは、そこに意図的なものとして既に記載されているエントリ（JWKS ローテーションテストの署名鍵・Garage の開発用資格情報）だけ。本当に新規の検出は実際の秘密なので、無視せずツリーから除去する。

## 7. 触っていないのに `env/.env` が dirty

`env/.env` は git 管理下にあり、`make materialize-env` がビルド時埋め込みのために `env/.env.$(APP_ENV)`（既定 `ci`）で上書きする。CI とイメージビルドがこれを行うため、ローカルで途中中断するとコピーが残る。

```bash
make restore-env          # git restore env/.env
```

材料化された `env/.env` はコミットしないこと。なおこのファイルを編集すると §6 の gitleaks フィンガープリントもずれる。

## 8. ホスト実行の lint / format / test と、2つの golangci 設定

`make go-lint` / `make go-fix` は golangci-lint を**ホスト**の mise 経由で解決し（`mise which golangci-lint`）、`make go-test` もホストで `go test` を走らせる。バイナリが無ければ `mise install`。ここでコンテナに手を伸ばさない。

設定ファイルは2本あり、素の `golangci-lint run` は誤ったほうを拾う:

```bash
golangci-lint run                                  # → .golangci.yaml（軽量。一部 linter が無効）
make go-lint                                          # → .golangci.yaml。CI と同じ
golangci-lint run --config .golangci.yaml     # 追加フラグを付けたいときはこちら
```

CI の失敗を再現するときは必ず full 設定を使う。

## 9. `commitlint: not found` / `orval: not found` / `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` / ツールが古い

ツールランナーのイメージは **`docker/tools/Dockerfile` と `mise.toml`、および node ランナーが in-tree で install する全パッケージ（`scripts/` と `docs-viewer/`）の `package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` の 3 点セットのビルド成果物**。ツールはホストではなくランナー内で解決される（コード生成と同じ再現性ルール。`docs/rules.md` 参照）。いずれかを変更した後、あるいはイメージがそれらより古いクローンでは、ランナーにツールが無い / 版が古い / そもそも何も実行できない状態になる。

対象が複数パッケージにまたがる点は意外に映る。`docs-viewer/` だけに閉じた変更でも、両方とも単一の node ランナーへ install されるため、`scripts/` 側のゲートが走るイメージが古くなる。

最後の症状は、変更との関係が見えにくい。`scripts/pnpm-workspace.yaml` は `verifyDepsBeforeRun: error` を設定しているため、その設定がイメージ内の `scripts/node_modules` を入れたときの設定と食い違うと、ランナー内の `pnpm run` はすべて `[ERR_PNPM_VERIFY_DEPS_BEFORE_RUN] The value of the <setting> setting has changed` で落ちる。倒れるのは変更と無関係に見えるゲート（`make md-lint` / `make actions-lint` / `make oapi-lint`）で、一方ホスト側の `-ci` ターゲットは緑のままである（ファイル変更時にホストのツリーは入れ直されているため）。`minimumReleaseAgeExclude` や `overrides` のエントリを 1 行足すだけで発火する。したがって**ホストで緑であることは、コンテナ側のゲートが通る証拠にならない**。先に再ビルドし、そのうえで報告するつもりのゲートを回し直すこと。

古いイメージは、**触ってすらいないパッケージの依存が見つからない、という形でコンテナ側のゲートを落とす**形でも現れる。1:1 テストゲート（`scripts/one-to-one.gate.test.ts`）は両パッケージを型検査するため、`docs-viewer/` が現在の manifest を得る前にビルドされたイメージは、`docs-viewer/src/**` に対して TS2307 `Cannot find module` を並べて返す。リポジトリが依存の宣言を忘れているように読めるが、探す木が違う。

**見分けの決め手は、`make` では落ちるのに同じコマンドを素の `docker compose run` で叩くと通ること。** `.makefiles/docker/compose.mk` は `COMPOSE_PROJECT_NAME` を共有インフラのプロジェクトとして export する（§1）ので、`make` は必ず `gobp-shared` のイメージに当たる。一方、素の呼び出しはディレクトリ名からプロジェクトを決めるため、別の——多くは新しい——イメージに当たる。この非対称は、コードについての証拠ではなくイメージの新旧差として読むこと。

**worktree をまたぐと、これは共有資源であり、一度に 1 ブランチ分の設定しか保持できない。** ランナーのイメージは単一の `gobp-shared` compose プロジェクトに属する（§1）ため、ある worktree での再ビルドは、他の全 worktree のコンテナ側ゲートを *そのブランチの* `scripts/pnpm-workspace.yaml` に向け直す。ブランチが食い違う 2 つの窓（片方は `minimumReleaseAgeExclude` を足し、片方は足していない）は交互に落ちることになり、どちらが落ちるかは最後に再ビルドした側で反転する。どちらの向きでも症状は同じ `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` なので、自分のブランチの不具合ではなく「イメージが他人のブランチに一致している」と読むこと。報告するゲートの直前に再ビルドし、もう一方の窓でも同じことが必要になると見込んでおく。

```bash
make tool-runners-build           # go / node / python ランナーを再ビルド（キャッシュ利用）
make tool-runners-build-clean     # --no-cache --pull。キャッシュ層が原因のとき
```

**コンテナ側の失敗の原因が特定できていないうちは、何かを結論づける前にクリーンビルドすること。** 「自分の変更と無関係」も「base ブランチに元からある失敗」も結論であり、古いイメージはそのどちらの証拠も作り出す。だから先に再ビルドを払う——`make tool-runners-build-clean` を回し、既に起動しているものは（落ちたランナーだけでなく `make serve` とツールスタックも）再起動し、そのうえでゲートを回し直す。数分で済み、他のあらゆる説明を模倣してしまう唯一の説明を消すには最も安い。原因不明のまま `--no-verify` に手を伸ばすのは、この確認を通過することではなく飛ばすこと。

node ランナーは `/app/scripts/node_modules` を匿名ボリュームとして持つ（バインドマウントによる shadow を防ぐため）。補助スクリプトはここからバイナリを解決するので、イメージが古いとこれらが壊れる。

commit-msg フックは `node_tool_runner` 経由で `make commitlint COMMIT_MSG_FILE={1}` を実行するため、コミット失敗の典型原因はこれ。`commitlint.config.js` は意図的に `type-case` を無効化し（prefix は Cap-first の `Feat`/`Fix`/… だが CI のメッセージは全大文字＝単一 case を強制できない）、`type-enum` をプロジェクトの prefix に固定している。`Merge` / `Revert` は既定で無視される。

この節が扱うのは*イメージの中でツールが欠けている・古い*場合である。固定された版を意図的に**上げる**のは
供給網の窓を伴う別の仕事で、`tools-upgrade` がそれを所有する —— この症状を消すために `mise.toml` の
pin を上げてはならない。

## 10. フック対応表 — 何がいつ走るか、自分の変更と無関係に落ちたときどうするか

`.lefthook.yaml` をトリガー別に整理すると:

| フック | glob → コマンド（抜粋） |
| --- | --- |
| pre-commit | `*.go` → `make gate-go`（`lint` + `go-test-cached` を束ねる）／`scripts/**/*.go` → `make go-test-scripts-cached`／`*.sql` → `make sql-lint`／`*.md` → `make md-lint`／`.github/workflows/**` → `make actions-lint`・`make pin-actions-check`／`openapi/**` → `make oapi-lint`／`docker/**/Dockerfile`・`docker-compose*.yaml` → `make docker-lint`・`make pin-images-check`／`database/migrations/*.sql` → migration の重複・ギャップ検査 |
| commit-msg | `make commitlint COMMIT_MSG_FILE={1}` |
| pre-push | `make secret-scan`／`*.go` → `make gate-go-push`（`test` + `go-test-scripts` を束ねる）／`*.go`・`openapi/**` → 再生成して `*.gen.go`・mock・`openapi.gen.yaml` を `git diff --exit-code`／`go.mod`・`go.sum` → `go mod tidy` + diff |

Go のゲートは 1 コマンドずつ並べず `gate-go` / `gate-go-push` に**束ねてある**。lefthook はフック内の commands を並列に走らせるため、ゲートごとにエントリを置くと、開いている窓の数に**加えて**ゲートの数だけホスト負荷が乗算されるためである。どれだけ全力で走るかは §19 が決める。

pre-push の `gen-go-check` は Docker で再生成して差分があれば落ちる。対処は再実行ではなく、再生成物をコミットすること（§2・§4）。自分の変更と無関係な理由で赤いとき（base ブランチに元からある失敗・環境要因）は `--no-verify` で push し、原因は別途つぶす。変更のほうを歪めない。ただし「元からある失敗」と断ずる前に、ツールランナーのイメージが古い線（§9）を消しておくこと。失敗メッセージだけでは両者を区別できない。

## 11. `pin-images-check` / `pin-actions-check` — fail-closed な lockfile

どちらも fail-closed。`docker/*/Dockerfile` の全 `FROM`、`docker-compose*.yaml` の全 `image:`、`.github/workflows/**` の全 `uses:` は、固定済みで**かつ** lockfile（`docker/images-pin.toml`・`.github/actions-pin.toml`）に登録済みでなければならない。compose サービスや action を新規追加すると、未登録（lockfile に無い）／未固定（tag のまま）でコミットがブロックされる。

固定済みの digest / SHA を手で書き換えないこと。専用スキル（images は `images-pin`、actions は `actions-pin`）を使う。サプライチェーンの cooldown（公開直後の digest は採用せず拒否する）と `resolve` → `apply` → `check` の手順はそちらが持っている。

## 12. pre-commit の "Migration version gap / duplicate"

`make check-migration-{up,down}-{version,gap}` は `database/migrations/**` の連番が up / down 双方で一意かつ連続であることを要求する。典型的な発火要因は、自分の番号を既に使っている base ブランチをマージしたとき。*自分の*新規ファイル側を採番し直す（`.up.sql` と `.down.sql` 両方）。コミット済みの既存 migration は `AGENTS.md` のとおり編集しない。新規作成は `make new-migrate-<name>` で行い、採番をツールに任せる。

## 13. ローカルの S3 呼び出しが 503 を返す

`garage` のバケット・レイアウト・アクセスキーは one-shot の **`garage_init`** コンテナがプロビジョニングする。プロジェクトを跨ぐため compose の `depends_on` では表現できず、`make infra-up` が起動して終了まで待つ。プロビジョニング完了前に上げた app は S3 エンドポイントから 503 を受ける。

```bash
make infra-up                     # 冪等。garage_init の終了まで待つ
```

バケットは全 checkout で共有される（データベースと違いスキーマを持たないため、ブランチ間で壊れない）。ブランチ毎に隔離したい場合は `OBJECT_STORAGE_BUCKET` を分ける。Go のテストは garage を使わず in-process の gofakes3 を使う。

## 14. 環境別 Docker イメージ

runtime イメージは、ビルド時に `--build-arg APP_ENV=<env>` で選んだ `env/.env.<env>` を1つだけ焼き込む（deploy ワークフローが注入する）。実行時 ENV で切り替える単一イメージ方式ではない。

```bash
go mod vendor   # builder ステージは vendor モード + GOPROXY=off。省くと失敗する（§18）
docker build --build-arg APP_ENV=stg --target runtime -t <img> -f docker/server/Dockerfile .
# .env.stg だけが焼き込まれたか検証する
```

マイグレーション専用イメージは無い。`env/` と `database/migrations` はバイナリに埋め込まれるため、マイグレーションは同じ `runtime` イメージの command override（`./server migrate-up`）で実行する。

## 15. `sync-versions` のドリフト

ツールバージョンの正本は `mise.toml` で、`go.mod`・各 Dockerfile・`docker/**` の README は派生物。派生側を直接編集したり、`mise.toml` を上げて伝播させないと CI チェックが落ちる。

```bash
make sync-versions                # mise.toml から伝播させ、結果をコミットする
```

Go バージョンの更新は `go-upgrade` スキルを使う（手順一式はそちらが持つ）。

## 16. 生成モック — 手書きせず Docker で再生成

各ソースが `//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE` を宣言し（`$GOFILE` ベースの destination はリポジトリ全体で統一されているため、どの interface ファイルにも同じ行をコピペできる）、`make gen-go-code` が `go_tool_runner` 内でピン留めされた mockgen（`mise.toml`。現在 `0.6.0`）で実行する。インターフェース変更後は `*_mock.go` を編集せず再生成する。新規 mock ディレクトリは root 所有で返ることがある（§4）。

```bash
make gen-go-code
```

## 17. `.makefiles` の DRY_RUN 規約

setup / teardown 系ターゲットは `$(if $(DRY_RUN),--dry-run,)` と `[ -n "$(DRY_RUN)" ]` で dry-run を判定するため、**空でない値はすべて真**＝`DRY_RUN=0 make <target>` でも dry-run になる。実際に実行するときは変数自体を付けない。プレビューは `DRY_RUN=1 make <target>`。`setup-repo` はプレビュー不可能なため `DRY_RUN` 自体を拒否する。

## 18. 新しい worktree の `go: inconsistent vendoring` — `make serve` は成功と出るが API が応答しない

`vendor/` は追跡外（`.gitignore`）である——サプライチェーン上の理由は `docs/design/security.md` に記録されている——ため、新しい worktree / clone には最初から存在しない。vendor モードを強制するビルド経路はちょうど 2 つで、air のホットリロードビルド（`.air.toml` の `go build --mod=vendor`）と runtime イメージビルド（`docker/server/Dockerfile` の `go build -mod=vendor`。`GOPROXY=off` 下なので取得へフォールバックできない）。それ以外——`make go-test`・`make go-lint`・ホストの `go run`——はモジュールキャッシュから解決して緑のままなので、アプリ本体がビルドされるまで何も警告してくれない:

```txt
go: inconsistent vendoring in /app:
        github.com/…: is explicitly required in go.mod, but not marked as explicit in vendor/modules.txt
```

`go.mod` と `vendor/modules.txt` の不一致が延々と並ぶ出力を、マージ事故や依存更新の失敗と読まないこと。通常の原因は単に `vendor/` がまだ無いだけである。なお `make serve` は成功行を出力する——`docker compose up -d` を `--wait` なしで実行するため、air のコンパイル前に戻ってくる。失敗は「API が応答しない」という形で現れ、エラーは `api_server` のログに居る。エディタの LSP も無関係なファイルにまで同じエラーを出す。

```bash
go mod vendor       # 対処はこれだけ
make serve
```

この手順は `make tidy-lib` が所管するが、先に `go mod tidy` を走らせるため `go.mod` を書き換えうる。欠けている `vendor/` を用意するだけなら素の `go mod vendor` を選ぶ。この状態を守る仕組みは無い: フックも CI チェックも `vendor/` を検査せず（`tidy-check.yaml` に明記がある）、イメージをビルドするワークフローは先に作り直すため落ちない。依存が変わるベース取り込みの後は、自分でコマンドを再実行する。同じ罠は手動のイメージビルド（§14）にも当たる——vendor モードの `Dockerfile` を直接叩くためである。

## 19. 変更と無関係な理由でゲートが落ちた — 開いている窓の数を見る

複数の worktree がそれぞれホスト全体を前提としたゲートを回すとホストが飽和し、ゲートは「変更の欠陥に見える形」で落ち始める。触っていないテストがタイムアウトし、`make go-lint` が 17 分かかり、`docker` が応答を返さなくなる（共有 DB 飽和・CPU 飽和の罠として現れることもある）。失われるのは所要時間ではなく、**ゲートの失敗がコードについての証拠でなくなること**である。

`.makefiles/load.mk` が make のパース時に `git worktree list` から重いゲートの規模を決めるため、誰も絞ることを覚えている必要はない。他を診断する前に、まず何が選ばれたかを訊く。

```bash
make load-status     # 帯・窓数・CPU シェア・各ツールへ渡るフラグ
```

| 帯 | 発動条件（既定） | 変わること |
| --- | --- | --- |
| `full` | worktree が 3 未満 | 何も変わらない（ツール既定・ホスト全体） |
| `low` | 3 以上 | `CPU / 窓数` の並列度・`nice -n 10`・重いゲートは 1 つずつ |
| `ci-first` | 5 以上 | 重いゲートはローカルで走らせない。push が CI へ運ぶ |

`ci-first` が手元に残すのは、**軽く、かつ push 後では取り返しがつかない**ゲートだけである（`commitlint`・`secret-scan`・ピン lockfile 検査・マイグレーション番号）。落とすのは CI が同一に再実行するものだけなので、検証されないものは生じない。検証の場所が動くだけである。

残りを委譲したまま重いゲートを 1 つだけ手で回したいときは、呼び出し単位で上書きする。

```bash
make go-lint GOBP_LOAD=low       # これだけ絞って走らせる
make go-test GOBP_LOAD=full      # 帯を無視する（単一窓のマシン向け）
```

閾値は `GOBP_LOW_THRESHOLD` / `GOBP_CI_FIRST_THRESHOLD`。絞る対象は**毎コミット・毎 push で走る**ゲートだけで、単発の重い処理（イメージビルド・コード生成・Trivy）は放置する。ループで回すものではないためである。

**窓が多いときに、CI の lint 失敗を再現するため `make go-lint` をローカルで回さないこと。** CI のログを読み、そこに名指しされた formatter / linter だけを当てる（config の選択は §8）。フルのローカル実行は、CI が既に出力した内容を再発見するために飽和したホストを数分間占有するだけである。

自分の変更と無関係な理由で既に赤いフックについては、§10 が `--no-verify` の例外を扱う。

## 20. 宣言したばかりの版を cooldown ゲートが弾いた

`make tool-cooldown-gate`（`mise.toml` と `python/*.in` の宣言）と `make go-cooldown-gate`（`go.mod` の
direct モジュール）は、追加した版が窓より若いと fail-closed で落ちます。

```txt
❌ uv@0.12.2（aqua:astral-sh/uv）は公開 13 日で cooldown 14 日を満たしていません
```

これは §11 の pin 検査とは別の機構です。`pin-actions` / `pin-images` は step-back を `resolve` に
組み込んでいて単に古いピンを維持しますが、この 2 つのゲートにその退避先は無く、宣言を直すのは
自分の仕事になります。

**窓は 1 つの数字ではありません。** その生態系が不正な公開をどれだけ速く検知・撤回するかから
backend ごとに決めており、影響範囲からではありません。定数は
`scripts/tool-cooldown/main.go`（`aqua:` / `ubi:` / `github:` 用の `releaseWindowDays` と、`go:` /
`npm:` / `pypi:` / `pipx:` 用の `registryWindowDays`）と `scripts/go-cooldown/main.go` の
`defaultWindowDays` が持ち、根拠は
[ADR-0094](../../../docs/adr/0095-malicious-package-detection-via-cooldown.ja.md) と
`docs/design/security.md` の Dependencies 節にあります。どこかに引用された数字（ここを含む）を
信じず、定数を読んでください。言語ランタイムは同じ ADR によりすべての窓の対象外です。

**対処は通常バイパスではなく step-back です。** 窓を既に満たしている最新の版を採ります。
`tools-upgrade` と `dep-vuln-upgrade` がこの選択を代行し、理由は宣言の隣のコメントに記録するのが
この repo の作法です。step-back が選べないとき — 修正が最新版にしか無い、対象メジャーに窓を
満たす版が 1 つも無い — は `supply-chain-triage` を回し、日数だけでなくその判定で決めます。

**バイパスは例外で、放置すると鳴るように作られています。** `.github/tool-cooldown-bypass.toml` と
`.github/go-cooldown-bypass.toml` がキーごとに 1 エントリを受け、`expires` / `issue` / `reason` の
3 つはすべて必須、`expires` の上限は 3 ヶ月先です。

```toml
"uv@0.12.2" = { expires = 2026-08-20, issue = 1272, reason = "…" }
```

`validateBypasses` は、期限切れのエントリ・上限を越えた `expires`・宣言から消えたものを指す
エントリを、同じゲートの違反に変えます。それが狙いです。期限は誰が宣言を触らなくても訪れ、
スケジュール実行が回収するため、素の allowlist と違ってバイパスが気づかれないまま
居座ることはありません。`expires` は余裕を持たせず、窓が実際に明ける日に置いてください。
不要になった瞬間から落ち始めるのが正しい挙動です。

**診断の前に知っておく罠が 2 つあります。**

ランタイムの昇格が、それ自体まだ窓の内側にある**ツール**の昇格を強いることがあります。ランタイムは
ゲートの対象外なので、宣言のどこを見てもこの連鎖は見えません。ピンしている `uv` がダウンロード
できる範囲を越えて `python` を上げると、`sql-lint` のホスト側レグで `uv venv --python <version>` が
落ち、直す道は新しい `uv` だけで、それをゲートが塞ぎます。コンテナ側の経路はイメージ内の処理系を
使ってこの経路を通らないため、ローカルではなく CI で表面化すると考えてください。

`python/*.in` のピンを上げて `.txt` を再生成し忘れた場合も落ちますが、これは窓ではなく
`verifyLocks` からの違反です。`make py-lock` を回して両方をコミットしてください。ピンと lockfile は
1 つの変更です。

## 21. ベースを取り込んだ後の衝突マーカー

このリポジトリのマージには自明でない点が 2 つあり、どちらも手順を `resolve-merge` が所有する理由になっている。

**マーカーの大半は、誰も編集してはいけないファイルに落ちる。** `*.gen.go`、`*.sql.go`、`*_mock.go`、
`openapi.gen.yaml`、`database/gen/*.gen.sql`、生成された `docs/` ツリー、pin lockfile —— どれも生成器か
resolver を持つので、正しい「側」が存在しない。片方を選ぶと、マーカーが無くレビューも通るのに、もはや
元から再現できないファイルができあがる。そして `gen-*-artifacts-check` が、他人の PR で、その人が
触ってもいないコードで落ちる。

**clean なマージは finished なマージではない。** `database/gen/*.gen.sql` は DML ファイルの連結物なので、
2 つのブランチがそれぞれ 1 つ追加すれば、衝突は起きないまま連結物だけが古くなる。派生生成物すべてに
同じことが言える。

どちらの場合も `resolve-merge` を回す —— パスをクラス分けし、生成物クラスを元から作り直し、lockfile を
resolver で解き直し、人間が要るものだけを差し戻す。ベースは PR の `baseRefName` から取り、rebase では
なく merge する（`CLAUDE.md`）。hotfix 中はベースが人間の判断なので明示的に渡す。

<!-- sample-api:begin -->
## 99. `sample-removal-check` が CI で落ちる

`scripts/setup/remove-sample-api/sample-manifest.ts` は、テンプレート利用者がサンプル API を剥がすときに `make setup-remove-sample-api` が削除する全パスを宣言している。サンプルドメイン（user / product / purchase / …）配下でファイルを追加・移動・改名したのに登録しないと、削除後に参照が宙に浮く。CI はこれを、実際に削除を実行してから build / lint / test することで検出する。ローカルでは何も落ちない＝自分で走らせない限り CI 専用で見つかる。 <!-- skill-lint-ignore -->

サンプルドメインにファイルを足したら（handler / usecase / domain / repository / DML / migration / seed / spec / 統合テスト / サンプル専用の生成物）、該当ドメインのエントリにパスを追記する。共有ファイルに混ざった行はパスではなく `sample-api` マーカーコメントで囲って扱う。削除せずに影響だけ見るには:

```bash
DRY_RUN=1 make setup-remove-sample-api
```

<!-- sample-api:end -->
## 制約

- ✅ 知識提供（read-only）: 正確なコマンドを提示し、実行はユーザーに依頼されたときだけ行う。
- ✅ 破壊的な操作の前に伝える: DB の作り直し（§5）・`chown -R`（§4）・`make infra-down` など全 checkout に影響する共有インフラ操作（§1）。
- ✅ 素の `docker compose` より `make` を優先する。素の compose が避けられないときは `COMPOSE_PROJECT_NAME=gobp-shared` / `-p gobp-shared` を明示する（§1）。
- ✅ ホストの `sudo` より docker の `--user root … chown` による復旧を優先する。
- ❌ 生成物（`*.gen.go`・`*.sql.go`・`*_mock.go`・`openapi.gen.yaml`・`schema.gen.sql`・`docs/portal/guides/**`・固定済み digest / SHA）を手編集しない＝再生成するか、所管スキルを使う。
- ❌ スキーマ / DML に影響する変更を再生成物なしにコミットしない（§2）。材料化された `env/.env` をコミットしない（§7）。
