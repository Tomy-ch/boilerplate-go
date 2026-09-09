> **このファイルは `SKILL.md` の日本語訳です。**
> 直接編集しないでください。内容の変更が必要な場合は canonical な `SKILL.md`（英語版）を更新し、その後この日本語訳を同期してください。
> Claude Code のスキルとしては `SKILL.md` のみが読み込まれます。このファイルはスキル本体ではなく、レビューや学習用の翻訳ドキュメントです。

# Dependency Vulnerability Upgrade

このスキルは **セキュリティ勧告リスト** を受け取り、名指しされた脆弱な依存だけを修正版へパッチする。本リポジトリの依存解決は 3 通りあり、勧告はそのいずれの package も名指しし得る:

- **pnpm** — `scripts/pnpm-lock.yaml`・`docs-viewer/pnpm-lock.yaml` に記録された依存。各パッケージが自前の `pnpm-workspace.yaml` を持ち、そこに cooldown ポリシーと `overrides` の両方が載る。
- **Go** — `go.mod` / `go.sum` のモジュール。**間接（indirect）** 依存を含む。
- **PyPI** — `python/*.in` が宣言し、`python/*.txt` が sha256 付きで解決を固定している CLI ツール。所在の特定はここの範囲だが、**引き上げは範囲外** — 2 つの宣言先を持つのは `/tools-upgrade`。後述の *PyPI の勧告はたいていこのスキルのものではない* を見ること。

**pnpm の `overrides` は npm のものを翻訳したものではない。** 直接依存については両者は一致する（宣言版を上げて
lockfile を再生成する）が、推移的な pin で分かれる。pnpm は `overrides` を `package.json` ではなく
`pnpm-workspace.yaml` に置き、その `parent>child` セレクタはその直接の辺しか制約しない（npm の入れ子
override はサブツリー全体に効く）。よって scoped な npm override をそのまま移すことはできない。本リポジトリの
pnpm パッケージはセレクタを親の名指しではなく **解決結果の版域**（`"fast-uri@<3.1.5": ">=3.1.5 <4"` =
「解決がこの版域に落ちたら引き上げる」）として書いている。対象の `pnpm-workspace.yaml` にある既存エントリに
倣うこと。推移的な pnpm pin がその形で素直に書けないときは、セレクタを発明せず保守者に委ねる。

このスキルは意図的に **対象を絞る**: 勧告で名指しされた package のみを変更し、「すべてを最新へ」の一括更新はしない。そうすることでセキュリティパッチをレビュー可能に保ち、無関係な変更と切り離す。一括更新には `/tools-upgrade`（mise ツール）や `make tidy-lib`（Go モジュール）を使うこと。

日本語参照訳は同じディレクトリの `SKILL.ja.md`（スキルとしては読み込まれない・人間参照用）。

## When to Use

次の場合に使う:

- ユーザーが脆弱性レポート（`npm audit` / Trivy / Dependabot alert / `govulncheck`、あるいは手書きの一覧）を貼り、指摘 package のパッチを望むとき。
- CVE / GHSA 勧告が `pnpm-lock.yaml` / `go.mod` に存在する package を名指しし、最小の修正版バンプ + 検証を行いたいとき。
- ツール（`redocly` / `orval` / `spectral` 等）が引き込む推移的依存を、`overrides` でパッチ版へ強制する必要があるとき。

次には使わない:

- 「pin 済みツールをすべて最新へ」のルーチン監査 — それは `mise.toml` `[tools]` 向けの `/tools-upgrade`。
- Go 言語バージョンの引き上げ — それは `/go-upgrade`（downstream 同期が異なる）。
- 特定勧告と無関係な `go.mod` 依存の一般的リフレッシュ — `make tidy-lib` を使う。
- 実体が mise 管理ツールである npm パッケージ — それは `/tools-upgrade` の管轄。
- **PyPI ツールの pin の引き上げ** — `python/*.in` を持つのは `/tools-upgrade` で、そこを変えるなら `make py-lock` による `python/*.txt` の再生成が伴う。ここでは所在を特定し、引き渡す。

## PyPI の勧告はたいていこのスキルのものではない

Python の勧告がこのリポジトリに届く形は 2 つあり、ここで手を動かせるのは前者だけである。

- **このリポジトリが宣言しているツールを名指しする場合**（`sqlfluff` / `graphifyy`）。所在を特定し、修正版とどこで宣言すべきかを報告して、**引き上げは `/tools-upgrade` へ渡す**。`python/*.in` を持つのはあちらで、そこを変えれば `make py-lock` が要ることも、`tool-cooldown` が何に対してゲートするかも知っている。ここで `python/*.in` や `python/*.txt` を編集しない。
- **`python/<tool>.txt` にしか現れない推移的な package を名指しする場合**。引き上げるべき個別の pin は存在しない。lockfile は解決結果なので、修正はそれを引くツール自体を上げることで届くか、上流が出すまで届かないかのどちらかである。どのツールの lockfile が抱えているか、より新しいツール版が勧告を越えて解決するかを報告し、判断はユーザーに委ねる。`.txt` を手編集しない — sha256 を持ち install 時に `--require-hashes` が強制するので、編集した行は install されずに失敗する。

いずれの場合も、移動を支配する cooldown は後述の *しきい値はエコシステムにより* の節が扱うものであり、逃げ道は `.github/tool-cooldown-bypass.toml` であって、このスキルの書き込み面には無い。

## First Step: 勧告のパースと caution しきい値の解決

勧告リストをパースし、supply-chain caution しきい値 `<MIN_AGE_DAYS>` を解決する — **スキルとツールチェーンを一致させるため、その lockfile に対してリポジトリが既に宣言している cooldown を優先** し、ディスク上に権威ある値が何も無いときだけユーザーに尋ねる。しきい値は全体で 1 つではなく lockfile ごとに決まる。現状はどちらも 7 日だが、npm と pnpm のパッケージは別々のファイルに支配されている。

手順:

1. スキル引数または直近のユーザーメッセージから勧告リストをパースする。各エントリから **package 名** / **現行 version**（あれば）/ **修正版候補**（major 系列ごとに複数あり得る）/ **CVE・GHSA id** / **深刻度** を取り出す。リストは自由形式でよい — よくある形（`- [HIGH] lodash 4.17.23 → 4.18.0 (CVE-...)`、`npm audit` ブロック、Trivy 行）を許容する。エコシステムや所在が曖昧なエントリは Step 1 で解決する（ここで推測しない）。
2. リポジトリの cooldown を検出する: 各 `pnpm-lock.yaml` と同階層の `pnpm-workspace.yaml` から `minimumReleaseAge`（**分指定**。`10080` = 7 日なので 1440 で割る）と `minimumReleaseAgeStrict`、既存の `minimumReleaseAgeExclude` 一覧を読む。その値をそのパッケージの `<MIN_AGE_DAYS>` として採用する。例外一覧は無関係に見えても必ず読むこと — 候補版を既に覆うエントリがあるなら、窓は意図的に開けられており disposition は `blocked` ではない。
3. ある変更にどの cooldown も効かない場合（Go モジュール、またはどちらの設定も無い lockfile）は、引数で値が渡されない限り `7` を既定 `<MIN_AGE_DAYS>` とする。しきい値の確認で `AskUserQuestion` を呼ぶのは真に曖昧なとき（支配ファイル同士が食い違う、またはユーザーが override を求めた）だけ。単一の宣言値や `7` 既定は質問不要 — 使った値とその出所を明記して進める。

しきい値はエコシステムにより 2 つの異なる役割を持つ:

- **`minimumReleaseAge` 下の pnpm**: **ハードブロック**で、しかも範囲が広い。pnpm は install のたびに **lockfile 全体**をポリシーで再検証し、それは `--frozen-lockfile` にも及ぶ。よって窓内エントリは新規解決（`ERR_PNPM_NO_MATURE_MATCHING_VERSION`）だけでなく再生経路でも落ちる（`ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION`）。npm と違い pnpm には版指定の正式な例外経路（`minimumReleaseAgeExclude`）がある。それは Step 5 で提示する選択肢であって、ここで取る判断ではない。挙動の詳細は [`docs/design/security.md`](../../../docs/design/security.md) の「Dependencies → pnpm」が持つ — この要約を信用せず、そちらを読むこと。
- **それ以外（Go、cooldown 無しの lockfile）**: **caution フラグでありハードブロックではない** — 既知脆弱性を直すのが目的なので、新しすぎる修正版は握り潰さず提示・確認する。

`<MIN_AGE_DAYS>` が解決するまでレジストリ取得やファイル編集を行わない。

## AI Modification Scope

`AGENTS.md` は通常 `docker/**` とリポジトリ直下ファイルを AI 編集の対象外とする。**このスキルの起動がそれを緩める明示指示** であり、依存パッチが触れる特定ファイルに限り、この実行の間だけ緩和される。これは `AGENTS.md` の「Skills must not be a loophole」条項に沿った、文書化済みの非抜け穴的例外である — 下記が全変更面であり、どれが変わったかはユーザーへ報告する。

このスキル実行中に変更可:

- `**/package.json` — 承認済み package の `dependencies` version の追加/調整に限る。
- `**/pnpm-lock.yaml` — 承認済み変更に対する当該 package ディレクトリでの `pnpm install --lockfile-only` の決定的出力。
- `**/pnpm-workspace.yaml` — `overrides` と `minimumReleaseAgeExclude` キー **のみ**。例外エントリは **Step 5 でユーザーがその版を承認した後にのみ** 書く。`minimumReleaseAge` / `minimumReleaseAgeStrict` / `minimumReleaseAgeIgnoreMissingTime` / `trustPolicy*` / `allowBuilds` / `blockExoticSubdeps` / `engineStrict` には決して触れない — それらはパッチではなくポリシーそのもの。
- `go.mod` / `go.sum` — 承認済み Go モジュールに対する `go get <module>@<version>` + `go mod tidy` の出力。
- `vendor/**` — `go mod vendor` の機械的出力として **のみ**、かつ **リポジトリが vendoring するとき（`vendor/modules.txt` が存在）のみ**。`go.mod` バンプは `vendor/modules.txt` を不整合のまま残し、ビルドが `inconsistent vendoring` で落ちる。よって再 vendoring は好みの編集ではなく必須の downstream 手順。vendored ファイルは手編集しない。
- これら依存が駆動する再生成物 — Step 7 の drift チェックで動いた場合 **のみ**（例: `make gen-bundle-oapi` 経由の `openapi/openapi.gen.yaml`）。リポジトリの `make` ターゲットで再生成する。生成物を手編集しない。

このスキル実行中でも保護（触れない）:

- `AGENTS.md` / `CLAUDE.md`
- `node_modules/**`（追跡対象外のビルド生成物）、および上記 `go mod vendor` 経由を **除く** `vendor/**`
- 手編集される生成ファイル（`**/*.gen.go`、`*.sql.go`、`*_mock.go`、`**/openapi.gen.yaml`、`docs/` 配下の生成物） — `make` 経由の再生成は可、手編集は不可。
- 勧告リストに名指しされていない package。このスキルは対象限定であり、隣の依存を便乗バンプしてはならない。

## Execution Steps

### 1. 各 package の所在特定

勧告エントリごとに、package が実際にどこに存在するかを見つけて分類する。推測せず、ツリーに対して検証する。

```sh
# そもそもどの lockfile が存在するか — エコシステムを決めるのは package 名ではなくこれ
find . -name pnpm-lock.yaml -not -path '*/node_modules/*'

# pnpm: 存在 + インストール済み version。冒頭付近の `importers:` ブロックが specifier 付きで
# 直接依存を列挙する。`snapshots:` / `packages:` 配下に裸で並ぶ `<pkg>@<ver>:` は推移的。
grep -n "^  <pkg>@" <dir>/pnpm-lock.yaml            # 解決された version
grep -n "\"<pkg>\"" <dir>/package.json              # 宣言があれば直接

# PyPI: 宣言は .in、推移依存まで含めた解決結果は .txt
grep -n '<pkg>' python/*.in                          # 宣言あり → ツール。/tools-upgrade の管轄
grep -niE '^<pkg>==' python/*.txt                    # 解決結果。推移的にしか出ないこともある

# Go: モジュールが go.mod にあるか、直接か間接か
grep -n '<module-path>' go.mod
```

エントリごとに記録:

| 項目 | 方法 |
| --- | --- |
| ecosystem | `pnpm` / `pypi` / `go` |
| location | `pnpm-lock.yaml` のディレクトリ、`python/<tool>.in` + `.txt`、または `go.mod` |
| installed version | lockfile / `go.mod` から |
| direct / transitive | その `package.json` の `dependencies`/`devDependencies` に宣言がある（pnpm）／`// indirect` が無い（go）→ direct、そうでなければ transitive/indirect |

1 つの package が **複数の lockfile** に載ることがある（本リポジトリでは `mermaid` / `zod` / `js-yaml` が該当）。所在ごとに 1 エントリを記録すること — それぞれ独立に動き、それぞれ自分の cooldown を持つ。

いずれの lockfile / `go.mod` にも見つからない package は **not-present**（既に除去済み、または本リポジトリに無い lockfile）として報告しスキップする — 所在を捏造しない。

### 2. 修正版の選定

存在する package ごとに対象 version を選ぶ:

- **既定: 現在インストール済みの major 系列に留まる最小の修正版。** 複数候補の勧告（`brace-expansion 1.1.15 → 5.0.7 / 1.1.16 / 2.1.2`）からは、インストール済み major に一致するもの（`1.1.15` → `1.1.16`）を選ぶ。breaking リスクを最小化する。
- **major 跨ぎが必要**（修正が上位 major にしか無い、またはインストール系列に patch 版が無い — 例 `@hono/node-server 1.19.14 → 2.0.5`）: **breaking の可能性** として明示的にフラグする。per-package 確認（Step 5）とより入念な検証（Step 7）を要する。
- **downgrade ガード**: インストール済みより厳密に低い version は決して選ばない。唯一の「修正」がより低くパースされる場合は適用せず `needs-manual` として提示する。
- **勧告の番号ではなく、実際に解決される version をゲートする。** `package.json` の `^`/`~` レンジは一致する最新 patch へ浮くため、勧告の修正版よりずっと新しく `too-new` になり得る。lockfile が landing する version の日付を計算し（Step 6 で再確認）、レンジが too-new/未検証版へ解決してしまう（かつ **cooldown で抑えられない**ディレクトリ）場合は、caret を浮かせず **承認済みの exact 版を pin** する。cooldown 下ならレンジのケースは両エコシステムとも無害で、最新ではなく **枯れた最新**へ静かに着地する。ただし裏を返せば、レンジが勧告の修正版に届かないまま黙って通ることもあるので、lockfile が実際に何を pin したか読み直すこと。

caution ゲート（Step 3）用に、選定各版の **公開日** を取得:

```sh
# npm
curl -fsSL https://registry.npmjs.org/<pkg> | jq -r '.time["<version>"]'
# Go（モジュール公開時刻）
curl -fsSL "https://proxy.golang.org/<module>/@v/<version>.info" | jq -r '.Time'
```

### 3. supply-chain caution ゲートの適用

選定各版について `now - publish_date` を計算し disposition を定める:

| フラグ | 条件 | 効果 |
| --- | --- | --- |
| **clear** | `>= MIN_AGE_DAYS`、または既に `minimumReleaseAgeExclude` に名指しされている | eligible — 既定で適用（Step 5） |
| **too-new** | `< MIN_AGE_DAYS` で、cooldown **無し**のディレクトリ、または Go | eligible だがフラグ付き。⚠️ 提示・既定では未適用 — ユーザーが opt-in する必要 |
| **blocked** | pnpm `minimumReleaseAge` 下で `< MIN_AGE_DAYS` | **そのままでは install 不可** — リポジトリ自身の cooldown がハード拒否。独断で適用せず **deferred** とし、解除時期（`publish_date + N 日`）を報告。pnpm なら Step 5 で例外の選択肢も併せて提示する |

npm / Go proxy への悪意ある publish は通常、数時間〜数日で検知・撤回されるため caution を設ける。セキュリティ修正は急ぐので `too-new` はユーザーが override できる警告だが、`blocked` はリポジトリ自身のポリシーをパッケージマネージャ自体が強制するものであり、スキルはそのポリシーを決して無効化しない。境界は現実的である点に注意: N 日窓の内側に数分でも入って公開された版は、窓がその正確な publish 時刻を跨ぐまで `blocked`。

**`blocked` のあと何が残るかは 2 つのエコシステムで違う。** npm には版指定の逃げ道が無いので、blocked な npm エントリは文字どおり「待つか、より古い熟成済みの修正版を採るか」になる。pnpm には `minimumReleaseAgeExclude` があり、本リポジトリの `pnpm-workspace.yaml` はそれを緊急のセキュリティ修正のための経路と説明し、`minimumReleaseAgeStrict: true` が意図的にそれを解決器の手から取り上げている。**この違いが変えるのは提示する選択肢であって、誰が決めるかではない。** 例外の追加は毎回人間の判断であり、ファイルに前例があることは次の 1 件を足してよい根拠にならない（`AGENTS.md` *Conflicting Authority*）。

### 4. ゲートが捕捉したものをトリアージする

disposition が `too-new` または `blocked` になった全エントリについて、Step 5 が判断を提示する前に **`/supply-chain-triage`** を chain する（エントリごとに 1 回）。

窓は四つの問い——発行者は変わったか、artifact は source と一致するか、実際に何が変わったか、新しい依存が増えたか——の代理指標であり、待つ代わりに直接答えることができる（`docs/design/security.md` → 「Dependencies」）。この Step がその実行場所である。`too-new` の opt-in はまさに、ユーザーの日数に対する許容度ではなく証拠に基づくべき判断であり、`blocked` エントリについては、待つことが単に不便なのか実際に防御的なのかを知る必要がある。

エコシステム、パッケージ、候補バージョン、**lockfile が現在保持している baseline**（差分のもう一方の端）、`<MIN_AGE_DAYS>`、disposition、移動を強いている CVE を渡す。トリアージは報告のみ——tarball / module zip を実行せずに読み、0–12 のスコア・バンド・根拠を返す。何も変更せず、何も採用しない。判断は Step 5 に残る。

フラグ付きが何も無い（全て `clear`）ときは飛ばす。この run でユーザーが既に見送ったエントリも飛ばす。各エントリのバンドは Step 5 のサマリと選択肢の説明へ引き継ぎ、証拠を添えたうえで選択させる。

### 5. サマリ表示・clear は自動適用・フラグ付きのみ確認

日本語サマリを disposition ごとに出す。例:

```text
依存脆弱性パッチ監査（min_age_days = 7, pnpm-workspace.yaml 由来）

✅ 適用（同一 major の最小修正版 / caution 通過 → 確認なしで適用）:
  - lodash 4.17.23 → 4.18.0  [docker/tools, 推移的]  (CVE-2026-4800, CVE-2026-2950 / HIGH)
  - js-yaml 4.2.0 → 4.3.0     [docker/tools, 直接]    (CVE-2026-59869, CVE-2026-53550 / HIGH)

⚠️ major 跨ぎ（breaking の可能性 / 別途確認）:
  - vite 7.4.2 → 8.0.1  [docs-viewer, 直接]  (GHSA-frvp-7c67-39w9 / MEDIUM)

⚠️ too-new（公開が min_age 未満 / 別途 opt-in）:
  - fast-uri 3.1.3 → 3.1.4  [docker/tools]  (公開 3 日 / CVE-2026-16221 / HIGH)
      トリアージ: 1/12 LOW（発行者同一・provenance 一致・差分は URL parser のみ・新規依存なし）

⛔ deferred（repo の cooldown に阻まれ install 不可）:
  - brace-expansion 1.1.16  [docker/tools, spectral-core 内]  (公開が cooldown 内 / 2026-07-22 頃に解除)
      トリアージ: 2/12 LOW（ただし cooldown により install 自体が不可。解除には minimumReleaseAgeExclude の記載が要る）
  - mermaid 11.16.1  [docs-viewer + scripts, 直接]  (公開 3 日 / 2026-08-12 00:09 JST に解除)
      トリアージ: 0/12 LOW（pnpm は minimumReleaseAgeExclude での版指定例外が選択肢）

❓ 未検出 / 要手動:
  - （lockfile に見つからない等）
```

確認ポリシーは意図的に非対称にする — clear なパッチはスキルを回す目的そのものなので、ユーザーにクリックさせない:

- **clear かつ非 major → 確認なしで適用。** これは既定パッチであり、1 件ずつ確認するのは、勧告に対しスキルを起動した時点でユーザーが既に暗黙承認している摩擦にすぎない。
- **major 跨ぎ（インストール済みより上位 major）→ 常に別途報告・確認**。version 自体が `clear` でも同様。major はそれを import するコードを壊し得るので、ユーザーが承知のうえ判断する。Step 7 でより入念に検証。
- **too-new（cooldown 無しの caution）→ 報告・確認（opt-in）**、既定は未適用。
- **blocked / deferred → 独断では決して適用しない。** 報告と解除時期を示す。**npm** エントリはそこで終わり。**pnpm** エントリはさらに `minimumReleaseAgeExclude` の選択肢を提示し、待つか・その 1 版だけ窓を開けるかをユーザーが選べるようにする — 提示はするが、前提にはしない。

`AskUserQuestion` を呼ぶのは、判断すべき **major 跨ぎ** / **too-new** / **blocked な pnpm** エントリがあるときだけ（それらだけを並べた 1 回の `multiSelect: true`、既定は全て未選択）。フラグ付き選択肢には Step 4 のトリアージ・バンド（`1/12 LOW` / `7/12 HIGH` / `INSUFFICIENT-EVIDENCE`）を説明として付ける。opt-in するか待つかの理由はそのバンドなのだから、上のサマリだけでなくクリックする場所に置くべきである。pnpm の例外の選択肢では、何を買い何を払うかを説明に書く: その版が今 install できるようになる代わりに、誰かがその行を消すまで全チェックアウトがポリシー例外を抱える。eligible が全て `clear` かつ非 major なら、質問せず即適用する。eligible が無ければ書き込みなしで Step 8 へ。

### 6. 更新の適用

承認 package を location ごとにまとめて適用する。`pnpm-lock.yaml` は決して手編集せず、`pnpm` に再生成させる。

ここで追跡されるのは `package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` のみ（`node_modules/` は git-ignore、toolbox イメージで再構築）なので、`--lockfile-only` で **lockfile だけ** を更新する。レジストリから解決し、ツリーを実体化せずに lockfile を書き換える。

**pnpm — 直接依存**: `<dir>/package.json` の宣言版をバンプし、lockfile だけ再生成する。本リポジトリの pnpm パッケージはレンジではなく **exact 版**で pin しているので、その形を保つこと。

```sh
# <dir>/package.json を編集: "<pkg>": "<new-version>"
cd <dir>
pnpm install --lockfile-only
```

`--lockfile-only` は `node_modules/` を実体化せずに解決して `pnpm-lock.yaml` を書き直す。差分を読み戻し、意図した package だけが動いたことを確認する — pnpm の lockfile バンプは通常数行で、差分が広ければ他の何かが再解決されている。

**pnpm — 推移的依存**: `<dir>/pnpm-workspace.yaml`（`package.json` ではない）に `overrides` エントリを追加し、本ファイル冒頭の注意に従って解決版域として書いてから `pnpm install --lockfile-only`。同一 major フロアの原則と暫定債務の原則は上で述べたままに適用する。

**pnpm — cooldown が阻む版を、Step 5 でユーザーが例外を承認した後に入れる場合**: そのパッケージの `pnpm-workspace.yaml` の `minimumReleaseAgeExclude` へ、既存エントリと同じ形式で追加する。

```yaml
minimumReleaseAgeExclude:
  - <pkg>@<version> # <解除日時> 以降に削除する。<対象 advisory> の修正版で、<どこで動くか>。
```

エントリをレビュー可能にする要素は 4 つあり、すべて必須:

- **`<pkg>@<version>` であり、パッケージ名だけにしない** — 名前だけの免除はそのパッケージの将来のすべての公開を見逃す。
- **削除日**。`publish_date + MIN_AGE_DAYS` として計算し JST で書く。これは両方向に効く: その時刻より前に消せば全 install が壊れ、その時刻を過ぎて残せば誰も必要としないポリシー例外になる。
- **対象 advisory**。読み手が事案を再導出せずに判断できるようにする。
- **どこで動くか**（ブラウザバンドル / tool-runner のビルド時 / サービス実行時）。それが、この例外で受け入れている暴露面だから。

その版を取り込む **すべての** パッケージに同じエントリを入れること — 例外は `pnpm-workspace.yaml` 単位なので、片方だけ入れるともう片方の install が落ちたままになる。install を通すために `minimumReleaseAge` や `minimumReleaseAgeStrict` を編集しない。それはポリシーであり、下げると全依存の窓が一斉に、しかも黙って開く。

**Go モジュール**（直接 / 間接）:

```sh
go get <module>@<version>
go mod tidy
go mod vendor        # リポジトリが vendoring するとき（vendor/modules.txt が存在）のみ
```

承認済み Go モジュールは 1 回の `go get`（複数 `module@version` 引数）+ 1 回の `go mod tidy` にまとめ、`go.sum` を一度で収束させる。`vendor/modules.txt` が存在するなら後で `go mod vendor` を走らせる — これが無いと `go.mod` と `vendor/modules.txt` が食い違い、ビルドが `inconsistent vendoring` で落ちる。

### 7. 検証

実際に変わったものに合わせてチェックを走らせる — 依存パッチが一次ソースに触れることは稀なので、常にフルスイートを回すのではなく、動いたエコシステムに検証をスコープする。各々を OK / FAIL で報告し、失敗しても自動ロールバックしない — ユーザーが判断する。

Go 変更は最低限ビルド + vuln スキャンが clean であること（`go build ./...` + `govulncheck ./...`）。Go 変更がフルスイートに値するほど広ければ `make go-lint` / `make go-test` を回す。**pnpm パッケージの major バンプ** はより入念に検証する — その package 自身の `typecheck` + テスト（例 `docs-viewer/` で `pnpm typecheck` + `pnpm test`）を回す。major は呼び出す API を変え得るため。

pnpm 変更 — 意図は同じで、変更した各 package ディレクトリで `pnpm audit`。加えて npm には不要な手順が 1 つある:

```sh
cd <dir> && pnpm install --frozen-lockfile   # lockfile がなおポリシーを満たすことの証明
cd <dir> && pnpm audit                       # パッチ対象 CVE が出なくなること
```

ここでの実質的なゲートは frozen install のほうである。pnpm は再生時にも lockfile 全体をポリシーで再検証するので、これが「CI と他の全チェックアウトで install できる」ことの証明になる — `--lockfile-only` だけでは示せない。`ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION` で落ちたら、覆われていない窓内エントリがある。そのパッケージに例外が無いか、2 つのうち片方にしか入れていないかのどちらか。

`pnpm audit` が返す情報は薄い。真の fix floor や勧告ごとの影響版域が要るときは、要約から推測せず勧告データベースを直接引く（`gh api "/advisories?ecosystem=npm&affects=<pkg>"`）。

勧告データベースが **真の fix floor** の source of truth であり、ユーザーのリストと食い違い得る。黙って解決せず提示すべき 2 ケース:

- **勧告が挙げた版より高い fix floor。** その package が *別の* 勧告を抱え、初修正版がユーザーの貼った版より高いことがある（例: リストは「2.0.5 で修正」だが、`2.0.0 - 2.0.9` に影響し `2.0.10` でのみ修正される別の moderate 勧告が残っている）。その高い版が `too-new` なら黙って飛びつかない — 衝突と、そもそも脆弱経路が到達可能か（例: WS ハンドラ未登録のサーバに対する WebSocket 限定の DoS は非該当の可能性が高い）を提示し、too-new な完全修正への opt-in か部分修正の受容かをユーザーに委ねる。
- **deferred/スキップした package に残る勧告** — 想定内。失敗ではなく、理由（cooldown で deferred / ユーザーがスキップ）付きで未解消と報告する。

Go 変更:

```sh
govulncheck ./...                # 利用可能なら。GHSA が解消すること
```

生成物 drift — これら依存はコード生成器を駆動するため、バンプで生成出力が動き得る。変更した lockfile が生成器を養う package のものなら、その生成器を走らせて drift を確認する（CI と同じチェック）:

- `docker/tools/**` 依存（toolbox イメージ: redocly / orval / sqlc 周辺ツール）→ 依存が変わったツールがそれらであるときに限り、そのイメージが生成する成果物（`make gen-*-oapi`、`make gen-query` 等）を再生成し drift を確認。

再生成で変更が出たら含める — 生成出力を黙って変えるセキュリティバンプが、CI の drift チェックを落とす状態でツリーを残してはならない。

**tool-runner イメージの drift（pnpm 変更時のみ）。** runner イメージは `scripts/node_modules` を焼き込むため、`scripts/package.json` + `scripts/pnpm-lock.yaml` + `scripts/pnpm-workspace.yaml` のビルド生成物である。それらを変えた後、コンテナ経由のゲート（`make md-lint` / `make actions-lint` / `make oapi-lint` 等）はイメージを再ビルドするまで `ERR_PNPM_VERIFY_DEPS_BEFORE_RUN` で落ちる:

```sh
make tool-runners-build
```

ホスト側の実行（CI が使う `make md-lint-ci` 等）は再ビルド前でも緑になるが、それはコンテナ側のゲートが通る証拠に **ならない**。再ビルドしてから、通ったと報告するつもりのゲートを回し直すこと。`repo-ops` §10 が同じ事象を逆方向から扱っている。

### 8. 最終報告

日本語で要約:

- パッチした package を ecosystem / location ごとにまとめ、version 差分と CVE id 付きで。
- 追加した `overrides` は、親が修正版を native に取り込んだら **回収すべき暫定 pin** として明示する（使った specifier — フロアか例外的 exact pin か、およびその理由）。
- 適用した `too-new` / `major-bump` エントリ（およびその確認）、または意図的にスキップしたもの。
- 追加した `minimumReleaseAgeExclude` エントリ: どのパッケージに入れたか、どの版を免除するか、そして **いつ削除しなければならないか** — 完了項目ではなく、ユーザーが負うフォローアップとして書く。
- cooldown が捕捉した各エントリについて、そのトリアージ・バンドとそれを決めた軸。採用か待機かの判断が証拠に基づいたことを記録に残すため。答えられなかった軸があればそれも挙げる。
- ユーザーが別手段で対処すべき `not-present` / `needs-manual` エントリ。
- 検証結果（`make go-lint` / `make go-test` / `npm audit` / `pnpm audit` / frozen install / `govulncheck` / drift チェック）。
- 再生成物（あれば）と、tool-runner イメージを再ビルドしたかどうか。

commit / stage / push はしない。ユーザーがツリーをレビューし `/commit` を手動実行する。コミットを求められた場合、セキュリティパッチは `docker/**` + `go.mod` + 再生成物にまたがることが多い旨を伝え、CVE を説明する明確な `Build:` / `Fix:` コミットにまとめる。

## Notes

- **一括ではなく対象限定。** このスキルの本質は勧告で名指しされた package のみに触れること。実行中に無関係な古い依存に気づいても、言及はしてもここでバンプしない。
- **聞きすぎない。** `clear` かつ非 major のパッチは、まさにユーザーがスキルを起動した目的 — 確認クリックなしで適用する。`AskUserQuestion` は本当に重い判断のためだけに取っておく: **major 跨ぎ**（breaking リスク）、**too-new** の opt-in、そして **pnpm の cooldown 例外**。それらは別途・明確に報告する。
- **リポジトリの cooldown を尊重し、無効化しない。** pnpm の `minimumReleaseAge` は意図的な supply-chain 制御。いずれかが修正版をブロックしたら、その版は `blocked`/deferred — 解除時期（`publish_date + N 日`）を報告し、窓を下げたり `--min-release-age=0` を渡したり `--before` を足したり `minimumReleaseAgeStrict` を `false` にしたり迂回したりしない。LOW のトリアージ・バンドもこれを変えない。トリアージが供給するのは証拠であって許可ではない。勧告の唯一の cooldown 内修正版が同日公開の新しい patch であることが多い。窓が明けるのを待つ、より古く既に枯れた修正版を選ぶ、あるいは（pnpm に限り）版指定の例外を足す — *いずれもユーザーが承認した場合のみ*。
- **版指定の例外は「窓を下げること」ではなく、その区別こそが要点である。** `minimumReleaseAgeExclude` が免除するのは 1 つの `pkg@version` だが、`minimumReleaseAge` を下げると全依存が一斉に、黙って、無期限に免除される。後者を前者の代替として提示してはならない。同様に、例外をパッケージ名だけへ広げてはならない。
- **Vendoring。** `vendor/` があるなら、`go.mod` の変更は `go mod vendor` が `vendor/modules.txt` を再同期するまで完了しない。さもなくばビルドが `inconsistent vendoring` で落ちる。
- **推移的 vs 直接。** 修正を含む互換な直接依存版が既にあるなら直接バンプが望ましい。親がまだ release していない純粋な推移的依存にはスコープ付き `overrides` を使う。スコープ付き override（`"parent": {"pkg": ">=<fixed> <<next-major>"}`）は、既に修正済みの top-level コピーを downgrade せずに脆弱な nested コピーを直す。各 package がどちらを使ったか報告に明記する。
- **override は暫定的な負債 — フロアで書き、後で回収する。** `overrides` エントリは手動の sticky な pin で、リゾルバは失効も通知もしないため、推移的依存への静かなキャップとして腐っていく。健全に保つ 2 つの規則: (1) exact version ではなく **同一 major のフロア**（`">=<fixed> <<next-major>"`）で書き、依存を凍結せずに修正を *最低ライン* として強制する — exact pin は range 内のより新しい版が既知の壊れで留めざるを得ないときだけに取っておく。(2) すべての override を **暫定** として扱う — **親** が修正版を native に引き込む release を出したら回収する: 親をバンプし、不要になった override を削除し、`pnpm install --lockfile-only`、そして `pnpm audit` を再実行して pin 無しでも修正が保たれることを確認する。腐った exact override は、pin した版自体が指摘されると脆弱性を再導入すらしうる。
- **複数 CVE の package。** 1 つの package が複数勧告行に出ることがある（例 `lodash` が 2 つの CVE 下）— すべてを満たす 1 回のバンプに dedup し、解消する全 CVE を挙げる。
- **lockfile のみ。** `node_modules/` は追跡されない。`pnpm install --lockfile-only` はフルインストールなしでマニフェストと lockfile を更新する。バンプは解決器に lockfile 内の兄弟コピーを再配置させ得る — diff は確認するが、同一 package の `4.17.x → 4.18.x` 的な dedup churn の増加は想定内で無害。
- **同じ package が複数の lockfile に載ることがある。** 勧告が届くすべての所在にパッチを当て、理由が無い限り版を揃える。2 つの pnpm パッケージは意図的に設定を揃えているので、片方だけ直すのは慎重さではなく drift である。
- **冪等性。** 適用成功後の再実行は、package が既に修正版であること（audit / `govulncheck` clean）を示し、書き込みをしない。
- スキルは決して自動 push しない。ユーザーがレビューし、手動でコミット・push する。

## Checklist

完了報告の前に確認:

- [ ] 勧告リストを（package, current, 修正版候補, CVE/GHSA, ecosystem）にパースした
- [ ] `<MIN_AGE_DAYS>` を **lockfile ごとに** 宣言元から解決した（`pnpm-workspace.yaml minimumReleaseAge` は分なので ÷1440。それ以外は `7` 既定）。既存の `minimumReleaseAgeExclude` を読んだ。質問は真に曖昧なときだけ
- [ ] 各 package を、それを保持する **すべての** lockfile（`pnpm-lock.yaml` / `go.mod`）で特定し direct vs transitive/indirect を分類、not-present を提示した
- [ ] 修正版を同一 major の最小として選び、major 跨ぎを breaking の可能性としてフラグ、downgrade ガードを適用した
- [ ] 公開日を取得し disposition を設定した（clear / too-new / blocked-by-cooldown）
- [ ] `too-new` / `blocked` の全エントリを `/supply-chain-triage` でトリアージした（baseline = lockfile の現行版）。バンドをサマリと `AskUserQuestion` の選択肢説明へ引き継いだ
- [ ] 日本語サマリを表示し、**clear 非 major は確認なしで適用**、`AskUserQuestion` は major-bump / too-new / pnpm 例外のみに使用、blocked を独断で適用していない
- [ ] pnpm 直接は `package.json` の exact 版 + `pnpm install --lockfile-only`、推移的は `pnpm-workspace.yaml` の解決版域 `overrides`、lockfile は手編集しない
- [ ] 承認された `minimumReleaseAgeExclude` は `pkg@version` 形式で削除日 + advisory + 動作箇所を添えて書き、影響する **すべての** パッケージに入れ、窓の設定自体には触れていない
- [ ] 追加した override を報告で暫定と明示した（親が修正版を native に出したら回収 — 親バンプ・override 削除・再 audit）
- [ ] Go は 1 回にまとめた `go get module@ver ...` + `go mod tidy`、vendoring するなら `go mod vendor`
- [ ] pnpm 変更に `pnpm install --frozen-lockfile` + `pnpm audit`、Go 変更に `govulncheck` + build、スコープに応じ `make go-lint` / `make go-test`（major バンプはより入念に — typecheck / package テスト）
- [ ] 生成器を養う依存には drift を確認し再生成物を含めた。`scripts/` の pnpm 変更後、コンテナ経由のゲートが通ったと報告する前に `make tool-runners-build` を実行した
- [ ] 最終日本語報告: 適用したセット、major/too-new/例外の判断と削除日、deferred/スキップ項目、検証結果
- [ ] `SKILL.md` 更新後に `SKILL.ja.md` を再同期した
- [ ] commit / stage / push を行っていない
