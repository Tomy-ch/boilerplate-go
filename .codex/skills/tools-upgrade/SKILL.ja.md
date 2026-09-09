> **このファイルは `SKILL.md` の日本語訳です。**
> 直接編集しないでください。内容の変更が必要な場合は canonical な `SKILL.md`（英語版）を更新し、その後この日本語訳を同期してください。
> Claude Code のスキルとしては `SKILL.md` のみが読み込まれます。このファイルはスキル本体ではなく、レビューや学習用の翻訳ドキュメントです。

# ツールバージョン更新

このスキルはピン留めされた全ツールについて、upstream 最新版との差分を監査し、**サプライチェーン隔離ゲート（supply-chain quarantine gate）** 付きで適用候補を提示する。`min_age_days` 未満の新しいリリースは「通知のみ」として扱い、自動適用しない。

理由: npm / PyPI / Go module proxy への悪意あるリリースの大半は、公開後 24〜72 時間以内に検知・取り下げが行われる。一定期間（既定 7 日）待つことで、コミュニティが検知する前に取り込んでしまうリスクを抑える。

監査対象は宣言の **2 か所どちらも**である。読まれないツールは永久に更新されないためである。

- `mise.toml` の `[tools]` — mise が解決するもの全部。
- `python/*.in` — PyPI のツール。解決結果は `python/*.txt` にハッシュ付きで固定される（[ADR-0084 (mise-ssot-drift-gate)](../../../docs/adr/0084-mise-ssot-drift-gate.ja.md)）。こちらの bump は 2 ファイルの変更になる（pin を書き換えてから `make py-lock`）。

## 使用タイミング

以下のような場合に使用する。

- 定期（月次・四半期）のツールバージョン棚卸し
- リリース直前の、既知 CVE 修正版が出ていないかの確認
- セキュリティアドバイザリ後の、対象ツールに更新があるかの確認

以下の用途では使用しない。

- Go 自体のアップグレード → `/go-upgrade` を使う（`make sync-versions` 経由の伝播範囲が異なる）
- Go module 依存のアップデート（`go.mod` の `require` ブロック）→ `make tidy-lib` を直接使う
- 単発のアドホックなバージョン bump → 宣言を直接編集（`mise.toml` なら `make sync-versions`、`python/*.in` の pin なら `make py-lock`）

## 最初に行うこと: `min_age_days` の確認

このスキルでは、**スキル起動直後に必ず `ask the user explicitly` でしきい値を確認する**。

手順:

1. スキル引数に値があれば（例 `/tools-upgrade 14`）候補として質問文に併記する（「候補: `14`」）。
2. 必ず `ask the user explicitly` を呼ぶ。
    - 質問: 「自動適用候補と判定するための最小経過日数を指定してください（推奨: `7`）」
    - 既定候補: `7`
3. 受け取った回答が 0 以上の整数であることを軽く検証し、以下の手順で `<MIN_AGE_DAYS>` として使う。

`<MIN_AGE_DAYS>` 確定までは upstream API へのアクセスや宣言の読み込みは行わない。

## AI Modification Scope について

`AGENTS.md` の "Exception: Skill Execution" 節に基づき、スキル実行中に以下のパスへの変更が許可される。

- `mise.toml`（`[tools]` table のみ、ユーザーが承認したエントリだけを書き換え）
- `python/*.in`（バージョンの pin のみ、ユーザーが承認したエントリだけ）と `python/*.txt` — 後者は `make py-lock` の出力としてのみで、手書きはしない
- `go.mod`, `docker/**/Dockerfile`, `docker/**/README.md`, `docker/**/README.ja.md` — `make sync-versions` の下流出力としてのみ（スクリプトが atomic に処理）
- `docker/**/Dockerfile` の `FROM` `@sha256:...` digest ＋ `docker/images-pin.toml` — `go` / `node` / `python` ランタイム bump で base image タグが変わった場合のみ、`make pin-images-apply` / `pin-images-resolve` 経由で

以下は引き続き保護対象（スキル実行中でも変更不可）。

- `AGENTS.md` / `AGENTS.md`
- 生成ファイル（`**/*.gen.go`, `*.sql.go`, `*_mock.go`, `**/openapi.gen.yaml`, `docs/` の生成物）
- バージョン bump と無関係な全てのファイル

## 実行ステップ

### 1. 宣言のパース

`mise.toml` を読んで `[tools]` 配下の全 key を列挙し、続いて `python/*.in` を読んでその `==` の pin を列挙する。各エントリについて backend を判定する。

| Key format | 宣言元 | Backend | 最新バージョンの取得元 |
| --- | --- | --- | --- |
| `aqua:owner/repo` | `mise.toml` | aqua (GitHub Releases) | `gh api repos/owner/repo/releases/latest` |
| `go:path/to/module` | `mise.toml` | go install | GitHub Releases（hosted されていれば）または `go list -m -versions path/to/module` |
| `npm:package` | `mise.toml` | npm | `https://registry.npmjs.org/{package}` |
| `pipx:package` | `mise.toml` | pipx (PyPI) | `https://pypi.org/pypi/{package}/json` |
| `package[extras]==X.Y.Z` | `python/<tool>.in` | PyPI（`uv` が `python/<tool>.txt` から install） | `https://pypi.org/pypi/{package}/json`（extras は落とす） |
| 短い名前（例 `golangci-lint`） | `mise.toml` | mise registry のデフォルト | `mise registry` で resolve、続いて該当 backend を query |
| `go`（ランタイム） | `mise.toml` | 公式 download manifest | `https://go.dev/dl/?mode=json` |
| `node`（ランタイム） | `mise.toml` | 公式 download manifest | `https://nodejs.org/dist/index.json` |
| `python`（ランタイム） | `mise.toml` | 公式 download manifest | `https://www.python.org/api/v2/downloads/release/` |

各ツールについて以下を取得する。

- **stable な最新バージョン**（`-rc` / `-beta` / `-alpha` / `-pre` / `-dev` 等の pre-release タグは除外）
- **公開日時**（ISO 8601）

GitHub Releases 系は `gh api` を優先する（`GITHUB_TOKEN` 経由で認証され rate limit が緩和される）。それ以外のエンドポイントは `curl -fsSL` で取得する。

### 2. 分類

各ツールを以下のクラスに分類する。

| クラス | 条件 |
| --- | --- |
| **up-to-date** | `pinned == latest`（先頭 `v` の有無を正規化したうえで一致） |
| **eligible** | `pinned != latest` かつ `now - release_date >= MIN_AGE_DAYS` |
| **pending** | `pinned != latest` かつ `now - release_date < MIN_AGE_DAYS` |
| **resolution_failed** | backend lookup が失敗（ネットワークエラー / 404 / parse 失敗） |

セーフガード: semver で「downgrade」になる場合は `resolution_failed` 扱い（reason: "potential downgrade"）。

### 3. サマリ表示

分類結果を日本語で見出し別にまとめて表示する。例:

```text
ツールバージョン監査結果（min_age_days = 7）

✅ 更新候補（公開から 7 日以上経過 / supply-chain quarantine 通過）:
  - golangci-lint: 2.12.2 → 2.13.0 （公開 2026-05-18, 17 日前）
  - sqlc: 1.31.1 → 1.32.0 （公開 2026-04-29, 36 日前）

⚠️ supply-chain quarantine（公開から 7 日未満、通知のみ）:
  - air: 1.65.3 → 1.66.0 （公開 2026-06-02, 2 日前）
      ※ 直接証拠での評価が必要なら /supply-chain-triage を実行できます（既定では未実行）

✓ 既に最新:
  - oapi-codegen 2.7.0
  - lefthook 2.1.8
  ... (省略可)

❌ 取得失敗:
  - sqlfluff（python/sqlfluff.in）: PyPI への接続失敗
```

`python/*.in` のエントリは、上のように宣言元のファイルを添えて示す。どのファイルが pin を持つかで適用時の作業が変わり、ユーザーはステップ 5 でそれを承認しようとしているためである。

### 4. 隔離が捕捉した pending リリースをトリアージする

`pending` 分類は、隔離が経過日数だけを理由にリリースを留めていることを意味する。それは四つの問い——発行者は変わったか、artifact は source と一致するか、実際に何が変わったか、新しい依存が増えたか——の代理指標であり、直接答えることで置換できる（`docs/design/security.md` → 「Dependencies」）。答えがユーザーの行動を変える場合に、pending ツールごとに **`/supply-chain-triage`** を chain する。

- pending リリースがこのスキルを起動した理由そのものであるとき（当該ツールを名指しするセキュリティ勧告）は常に実行する。
- それ以外は要求されたときのみ。3 つの pending を報告する定例の月次監査で 3 回のトリアージは不要である——まだ誰も何も判断しておらず、来月にはただ eligible になる。run を費やす代わりに「トリアージが可能である」と述べる。

backend、ツールキー、候補バージョン、**`mise.toml` で現在ピンしているバージョン**（差分のもう一方の端）、`<MIN_AGE_DAYS>`、公開日を渡す。トリアージは報告のみ——リリース artifact を実行せずに読み、バンドと根拠を伴う 0–12 のスコアを返し、`mise.toml` の編集も適用も行わない。pending のツールは pending のままである——スコアはユーザーが秤にかけるものであり、採用は別の明示的な判断（ステップ 5）として残る。

### 5. 適用候補の per-tool 確認

**eligible** が空で、かつトリアージから判断へ進んだ pending も無ければ、ステップ 7 へスキップし書き換えは行わない。

そうでなければ `ask the user explicitly` を `multiSelect: true` で呼ぶ。各 option は 1 つの eligible ツールに対応し、description にバージョン差分と公開日を載せる。既定状態: 全選択。

ユーザーは個別 deselect 可能（特定 bump が既知の壊れもの等）。

**pending** のツールをこの質問に載せてよいのは、ステップ 4 でトリアージ済みで、かつ早期採用するかをユーザーが明示的に判断している場合のみ。その場合は別枠に並べ、**既定では未選択**とし、description にバンドを載せる。pending を既定選択の eligible 集合へ混ぜてはならない——隔離の価値は、経過日数による eligible が既定であり早期採用が意図的な行為であることそのものにある。

### 6. 宣言の更新

`mise.toml` で宣言されている承認済みツールについて:

- `mise.toml` 内の該当行を特定する
- バージョンリテラルだけを置換する。key（`aqua:owner/repo` / `go:path/to/module` / 短い名前）と、もとが `v` prefix を使っていた場合はその慣習を保持する
- key の並び順を変えない、無関係な key を触らない、`[settings]` table も触らない
- **pin が最新より前にある理由は決して記録せず、そのようなメモに遭遇したら削除する。** upstream の最新版とその公開からの経過日数を示すメモは、[ADR-0095](../../../docs/adr/0095-malicious-package-detection-via-cooldown.md) が定め、`tool-cooldown` が強制するポリシーを繰り返すうえ、そこに加えた二つの事実はいずれも陳腐化する。経過日数は翌日には誤りになり、名指しした最新版は次の upstream リリースで古くなる。`mise outdated --bump` なら同じ問いに毎回答えられ、陳腐化しない。残すべきものはバージョンに依存しない制約——ある backend を別の backend より選んだ理由や、下限とそれを必要とする対象——であり、それらは維持する。

全承認分の置換を memory 上で計算したあと、`mise.toml` を **1 回だけ書き出す**（atomic single-pass）。

`python/*.in` で宣言されている承認済みツールについて:

- `==` の後ろのバージョンだけを置換する。パッケージ名と extras（`graphifyy[sql]`）は保持する
- pin が最新より前にある理由を述べるメモが pin の上にあれば、上記の `mise.toml` のルールに従って削除する
- そのうえで lockfile を再生成する:

  ```sh
  make py-lock
  ```

  `.in` と `.txt` は 1 つの変更である。`make tool-cooldown-gate` は lockfile が古い版のままの pin を失敗にする。再生成の忘れが「実際には入らない版に対して隔離が通る」状態を作らないためである。2 つのファイルは必ず一緒にコミットし、`.txt` を手書きしない。

  `py-lock` は `python/*.txt` を **すべて** 再生成するため、触っていないツールでも推移依存の新リリースによって差分が出ることがある。その差分は本物で、変更の一部である——確認して残すこと。ただし今回の run の隔離判断は直接の pin ごとに下したものであり、この差分はその外にある。最終報告でその旨を述べる。

### 7. 必要なら `make sync-versions` を実行

`go` / `node` / `python` のいずれかが更新されていれば `make sync-versions` を実行する。これにより `go.mod` と Dockerfile / `docker/**/README.md` 群の `FROM golang:` / `FROM node:` / `FROM python:` ハードコードが伝播する。

非ランタイムのツールだけが更新された場合は `make sync-versions` は不要。

### 8. ランタイムが変わったら base image digest を再固定

ステップ 7 で `make sync-versions` が走った（＝`go` / `node` / `python` bump で `FROM` の**タグ**が変わった）場合、以前 pin した `@sha256:...` digest は**旧**イメージを指したまま——タグ/digest 不整合になる（Docker は digest を優先）。registry から再 pin する（`images-pin` スキルの役目、ここで chain）:

```sh
make pin-images-resolve   # Docker Hub が 429 を返す場合は先に `docker login`
make pin-images-apply
make pin-images-check
```

`sync-versions` が書き換えるのは tag のバージョン部分だけで、末尾の `@sha256:...` はそのまま残り、*古い* イメージを指したままになる。Docker は digest を優先するため、ツリーは tag と digest が食い違った状態にあり、この状態でコミットしてはならない。

これを解消しようとすると `images-pin` の **ルール 3** に当たる。新しい tag には前回の lockfile エントリが無く、イメージは公開直後なので、退行先となる aged な digest が存在しない。`pin-images-resolve` は出来立ての digest を採用することも pin を tag のみへ剥がすこともせず、**fail-closed** で止まる（`❌ 退行先の無い出来立て image は採用できません`）。`apply` は走らず、`pin-images-check` は stale な digest を `未登録` として弾く。

明言すべき帰結はこれである。**ランタイム bump と digest pin は結合している。** 新イメージが既に `PIN_IMAGES_MIN_AGE_DAYS` を越えている場合を除き、この run はきれいに終われず、選択はユーザーのものである——意図的に `days=0` でブートストラップするか（`/images-pin` の手順 2.5 が `/supply-chain-triage` の証拠確認を挟む）、イメージが古くなるまでランタイム bump 自体を保留するか。`resolve` を無理に通さないこと。tag と digest の食い違いをツリーに残さないこと。

ステップ 7 をスキップした場合（ランタイム変更なし → タグ変更なし → digest は有効）は本ステップも丸ごとスキップする。

### 9. 検証

```sh
make go-lint
make go-test
```

`python/*.in` の pin を変えた場合は、併せて以下も実行する。

```sh
make tool-cooldown-audit
```

pin と lockfile が一致しているかを見る検査であり、いま宣言している版に対して窓を測り直す。pull request が回すのと同じゲートである。

結果テーブル（OK / FAIL）をユーザーに報告する。失敗しても自動ロールバックはしない — どう扱うか（修正コミット追加 / revert / そのまま）はユーザーが判断する。

### 10. 最終レポート

以下をまとめて報告する。

- 更新したツール数（PyPI ツールについては lockfile を再生成したかどうかも）
- `make py-lock` が取り込んだ推移依存の移動があれば、今回の run の pin ごとの隔離判断の外にあるものとして明記
- quarantine（pending）で見送ったツール数と、各々が窓を出る時期
- トリアージした pending ツールについて、そのバンドとそれを決めた軸（答えられなかった軸があればそれも）、およびユーザーが早期採用したか pending のまま残したか
- 検証結果
- 失敗があれば内容

コミット / stage / push は行わない。ユーザーが working tree をレビューしたうえで `/commit` 等を手動実行する。

## 注意事項

- **supply-chain quarantine の根拠**: 典型的な dependency confusion / malicious release インシデント（npm `ua-parser-js` 2021、PyPI `ctx` 2022 等）は公開後 24〜72 時間以内に検知・yank されている。7 日 quarantine は大半をカバーしつつルーチン bump にも追従できるバランス点。
- **pre-release の除外**: 常に最新の **stable** リリースを選ぶ。upstream が pre-release タグを出していても latest として選択しない。
- **calendar versioning**: `2024.12.30` のような calendar versioning を使うツールは lexicographic + semver fallback で比較する。downgrade ガードは常時有効。
- **rate limit**: GitHub API は anonymous で 60 req/h（IP 単位）。本スキルは `gh api` を経由して `GITHUB_TOKEN` 認証で 1000 req/h に上げる。
- **idempotency**: 複数回起動しても安全。適用後に再実行すると、適用済みツールは up-to-date として表示される。
- スキルは auto-push しない。ユーザーが working tree をレビューし、必要なら `make sync-versions` を回したうえでコミット・push する。

## チェックリスト

完了報告時に以下を確認すること。

- [ ] `<MIN_AGE_DAYS>` を `ask the user explicitly` でユーザーに確認済み
- [ ] 宣言の 2 か所（`mise.toml` の `[tools]` と全 `python/*.in`）を列挙済み
- [ ] 全エントリの backend を resolve（不能なら理由付きで resolution_failed に分類）
- [ ] 各ツールを up-to-date / eligible / pending / resolution_failed のいずれかに分類
- [ ] 分類結果テーブルをユーザーに提示
- [ ] 勧告が run の契機なら pending リリースを `/supply-chain-triage` でトリアージ（baseline = `mise.toml` のピン済みバージョン）。そうでなければ実行せず提示にとどめる
- [ ] eligible が非空なら、per-tool 適用候補を `ask the user explicitly` で確定。早期採用する pending は別枠・既定未選択・バンド付きで提示
- [ ] `mise.toml` を承認分のみ atomic に書き換え、key 形式と `v` prefix 慣習を保持
- [ ] 承認された `python/*.in` の pin を書き換え（パッケージ名と extras を保持）、`make py-lock` を実行し、両方のファイルを残す。`.txt` は手書きしない
- [ ] pin が最新より前にある理由を記録するメモを書かず、途中で見つけたものは削除し、バージョンに依存しない制約は残す
- [ ] `python/*.in` の pin を変えたなら `make tool-cooldown-audit` を実行
- [ ] go / node / python が更新されたなら `make sync-versions` を実行
- [ ] ランタイム bump 時は base image digest を再固定（`make pin-images-resolve` + `pin-images-apply` + `pin-images-check`）。公開直後のイメージでは新 tag に対するルール 3 の fail-closed が想定どおりの結果であり、結合（トリアージのうえ `days=0` でブートストラップするか bump を保留するか）とともに提示する。無理に通さず、tag と digest の食い違いを残さない
- [ ] `make go-lint` + `make go-test` を実行
- [ ] 最終結果テーブルをユーザーに報告
- [ ] `SKILL.md` 更新時は `SKILL.ja.md` も同期
- [ ] コミット / stage / push は一切実行しない
