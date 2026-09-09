> このファイルは `SKILL.md`（canonical / 英語）の日本語参考訳です。スキルとしては読み込まれません（参考用）。

# 脆弱性を名指した依存の限定更新

advisory の一覧はユーザーのメッセージから受け取る。パッケージごとに 1 行（`package current → fixed (CVE/GHSA)`）の形式でも、audit の出力を貼り付けたものでもよい。パッケージ名・記載があればインストール済みバージョン・修正候補・advisory ID・重大度を解釈し、複数の advisory を持つパッケージは重複を排除する。**変更してよいのは advisory が名指したパッケージと、それによって機械的に必要となる出力だけである。**

このリポジトリには独立した解決面が 3 つある。

- **pnpm:** `scripts/` / `docs-viewer/`。それぞれが自前の `pnpm-lock.yaml` と `pnpm-workspace.yaml` を持つ。cooldown・`overrides`・release-age の除外を所有するのは workspace ファイルである。
- **Go:** `go.mod` と `go.sum`。indirect モジュールを含む。
- **PyPI:** `python/*.in` の CLI ツール宣言と、`python/*.txt` に sha256 ハッシュ付きで解決された結果。**該当箇所をここで特定するのはよいが、ここで更新することは決してしない** —— 宣言と `make py-lock` による再生成を所有するのは `/tools-upgrade` である。

定期的なツールバージョンの監査と PyPI 宣言の変更には `/tools-upgrade` を、Go 言語バージョンには `/go-upgrade` を、Go 依存の一般的な更新には `make tidy-lib` を使うこと。

## 書き込みを許す範囲

このスキルの実行中に変更してよいのは、advisory に関係する `**/package.json`、パッケージマネージャが生成する `**/pnpm-lock.yaml`、`**/pnpm-workspace.yaml` の `overrides` と明示的に承認された `minimumReleaseAgeExclude` キー、`go.mod`、`go.sum`、そして `vendor/modules.txt` が存在する場合に限り `go mod vendor` の出力としての `vendor/**` だけである。生成物の再生成は、依存起因のドリフト検査がその変化を証明した場合に、既存の `make` ターゲットを通してのみ行う。

生成出力・lockfile・vendor 配下・`node_modules/**`・`python/*.in`・`python/*.txt` を手編集しないこと。無関係なパッケージを変更しないこと。`minimumReleaseAge`・`minimumReleaseAgeStrict`・`minimumReleaseAgeIgnoreMissingTime`・`trustPolicy*`・`allowBuilds`・`blockExoticSubdeps`・`engineStrict` を変更しないこと。

## 1. 所在の特定と分類

名指されたパッケージを解決している箇所をすべて見つける。**名前からエコシステムを推測しないこと。** `node_modules` の外にある pnpm lockfile をすべて探す。pnpm では `importers:` が直接依存を示し、`packages:` / `snapshots:` が解決された推移的パッケージを記録する。Go では `go.mod` とその `// indirect` マーカーが直接性を決める。Python では `python/*.in` に一致すれば宣言されたツールであり、`python/*.txt` にだけ一致する場合は推移的な可能性がある。

```sh
find . -name pnpm-lock.yaml -not -path '*/node_modules/*'
grep -n "^  <pkg>@" <dir>/pnpm-lock.yaml
grep -n '"<pkg>"' <dir>/package.json
grep -n '<pkg>' python/*.in
grep -niE '^<pkg>==' python/*.txt
grep -n '<module-path>' go.mod
```

箇所ごとに、エコシステム・ディレクトリ / ファイル・インストール済みバージョン・直接 / 推移的 / indirect の別・advisory ID・修正候補を記録する。1 つのパッケージが複数の lockfile に現れることがある。3 つの面のいずれにも無いものは `not-present` として報告する。**所在を捏造しないこと。**

PyPI の advisory の場合:

- `python/*.in` がそのツールを宣言しているなら、修正バージョンと宣言のパスを報告し、更新自体は `/tools-upgrade` へ渡す。統べる方針は `tool-cooldown` であり、その逃げ道は `.github/tool-cooldown-bypass.toml` である。どちらも本スキルには書き込めない。
- `python/<tool>.txt` だけがそのパッケージを解決しているなら、それを抱えているツールの lockfile と、より新しいツールバージョンが advisory を超えて解決するかどうかを報告する。パッケージ単位の pin は存在しない —— 上流を待つか、ユーザーの判断のうえで `/tools-upgrade` から親ツールを引き上げるかである。
- **`.txt` を手編集しないこと。** `--require-hashes` により、ハッシュを編集した lockfile はインストールに失敗する。

## 2. 修正バージョンの選定とゲート

インストール済みのメジャー線上で、最も小さい修正バージョンを選ぶ。**決してダウングレードしないこと。** 上位メジャーにしか修正が無い場合は `major-bump` とし、ダウングレードにならない候補が無い場合は `needs-manual` とする。ゲートを掛ける対象は lockfile が実際に解決するバージョンである。このリポジトリの pnpm 直接依存は常に厳密バージョンを使い、範囲指定は使わない。

`<MIN_AGE_DAYS>` は pnpm lockfile ごとに、隣接する `pnpm-workspace.yaml` から解決する。分単位の `minimumReleaseAge`（`10080 / 1440 = 7` 日）、`minimumReleaseAgeStrict`、`minimumReleaseAgeExclude` の全エントリを読む。候補と厳密に一致する除外が既にあれば、それは `clear` である。Go およびゲートの無い箇所では、ユーザーが値を指定しない限り 7 日とする。ディスク上の権威ある方針に本当の曖昧さが残る場合にだけ尋ねること。

pnpm は install のたびに lockfile 全体を再検証する。`--frozen-lockfile` でも同様である。したがって窓の内側のバージョンは、新規解決では `ERR_PNPM_NO_MATURE_MATCHING_VERSION` で、frozen の再現では `ERR_PNPM_MINIMUM_RELEASE_AGE_VIOLATION` で失敗する。`docs/design/security.md` の Dependencies を参照。

候補ごとに公開日を取得し、分類する。

| 分類 | 条件 | 対応 |
| --- | --- | --- |
| `clear` | 十分に枯れている、または pnpm の除外リストに厳密一致で載っている | 適用可。 |
| `too-new` | pnpm の強制方針は無いが注意窓の内側 | 明示的な同意があるときにのみ適用可。 |
| `blocked` | pnpm の `minimumReleaseAge` の内側 | 見送る。`公開日 + 閾値` を報告する。 |
| `major-bump` | 修正バージョンがインストール済みメジャーを跨ぐ | clear であっても明示的な承認を要する。 |

**cooldown を下げることも迂回することも決してしない。** blocked な pnpm パッケージにはバージョン指定の除外を提示してよいが、**前例があることは新たに追加してよい承認ではない。**

## 3. 捕捉された候補のトリアージと判断

`too-new` または `blocked` の各エントリについて、判断を提示する前に `/supply-chain-triage` を 1 回起動する。エコシステム・パッケージ・候補・lockfile の baseline バージョン・lockfile ごとの閾値・分類・advisory を渡す。トリアージは読み取り専用で、0〜12 の帯と証拠を返すだけであり、**採用を承認するものでは決してない。**

分類ごとにまとめた日本語の要約を提示する。`major-bump` / `too-new` / blocked な pnpm エントリがある場合は、それらだけを列挙した**運用者判断のプロンプトを 1 つにまとめて**提示し、各件について明示的な選択を求める。既定ではどれも選択されていないものとして扱う。トリアージの帯（`1/12 LOW`、`7/12 HIGH`、`INSUFFICIENT-EVIDENCE` など）を併記する。pnpm の除外を選択肢として示す場合は、それが今そのバージョンをインストールすること、そしてその行が消されるまで**全チェックアウトが方針の例外を抱え続ける**ことを述べる。

- `clear` かつメジャー跨ぎでないものは、質問せず直ちに適用する。
- メジャー跨ぎまたは `too-new` は、選択されたときにのみ適用する。
- **blocked を一存で適用しないこと。** 解除時刻を報告し、除外するか待つかの判断を委ねる。
- 適用対象が 1 つも無ければ、何も書かずに報告へ進む。

## 4. 承認された変更の適用

承認されたエントリをパッケージディレクトリ単位でまとめる。生じた diff を読むこと。**無関係な再解決による揺れを受け入れないこと。**

**pnpm 直接依存:** `<dir>/package.json` に厳密バージョンを設定し、次を実行する。

```sh
cd <dir>
pnpm install --lockfile-only
```

解決された lockfile のバージョンを読み直し、再度ゲートを掛ける。このコマンドは `node_modules` を実体化せずに `pnpm-lock.yaml` を書き換える。diff が広い場合は調べること。

**pnpm 推移的依存:** override は `<dir>/pnpm-workspace.yaml` にだけ足す。`package.json` には決して書かない。範囲内のより新しいバージョンが壊れていると分かっている場合を除き、厳密 pin ではなく同一メジャー内の下限（`">=<fixed> <<next-major>"`）を使う。既存の override は保ったまま、lockfile のみのコマンドを実行する。**pnpm のセレクタは入れ子 override の翻訳ではない** —— ローカルの解決範囲の形に従い、必要なセレクタを明確に表現できない場合は人間の判断へ回す。**すべての override は暫定的な負債として扱うこと。**

**承認された blocked な pnpm バージョン:** 影響する各パッケージの `pnpm-workspace.yaml` の `minimumReleaseAgeExclude` へ `<pkg>@<version>` を足す。既存のエントリはすべて残す。**素のパッケージ名を使わないこと。** JST での撤去時刻（`公開日 + 経過閾値`）・advisory・実行時の暴露面（ブラウザバンドル / tool-runner のビルドステップ / サービスの実行時）を併記する。release-age の方針そのものは変更しない。

**Go:** 承認されたモジュールを 1 回の `go get module@version ...` にまとめ、続けて `go mod tidy` を実行する。`go mod vendor` は `vendor/modules.txt` が存在する場合にのみ実行する。

## 5. 検証

変更したエコシステムに応じた検査を実行し、結果をそれぞれ報告する。**失敗しても自動で戻さないこと。**

- pnpm: 変更した各パッケージディレクトリで `pnpm install --frozen-lockfile`、続けて `pnpm audit` を実行する。frozen install は lockfile が方針をなお満たしていることを証明する。age 違反が出た場合、影響を受けた workspace に必要な除外が無いということである。
- Go: `go build ./...` と、利用できるなら `govulncheck ./...` を実行する。Go 側の変更が十分に広いなら `make go-lint` と `make go-test` も足す。
- 生成器へ入力される依存については、既存の生成ターゲットを実行し、生成物のドリフトを確認する。`scripts/` の pnpm マニフェスト・lockfile・workspace ファイルを変更した場合は、コンテナ経由のゲートが通ったと主張する前に `make tool-runners-build` を実行する。

`pnpm audit` は advisory が名指したものより高い修正下限を示すことがある。**黙って too-new なバージョンを選ぶのではなく、その事実を表に出すこと。** 見送った / 対象外としたパッケージに advisory が残るのは想定どおりであり、理由を添えて未解決として報告する。

## 6. override の回収

親パッケージが修正を自前で出すようになったら、その親を更新し、冗長になった override を削除し、`pnpm install --lockfile-only` を実行して `pnpm audit` を回し直す。解消した暫定負債を報告する。

## 最終報告

日本語で報告する。エコシステムと箇所ごとの修正済みパッケージ、バージョンと advisory ID、暫定的な override とその回収条件、メジャー跨ぎ / too-new / 見送り / 除外の各判断、除外に必要な JST での撤去時刻、トリアージの帯と答えられなかった軸、PyPI と not-present / 手動対応の申し送り、生成物のドリフト、tool-runner イメージの再ビルド状況、そして各検証結果。

stage・commit・push はしない。advisory が名指した依存を超えて修正範囲を広げない。後続の同期作業を始めない。

## 完了チェックリスト

- [ ] pnpm / Go / PyPI の全面で advisory の所在を特定し、直接性を分類し、不在を報告した。
- [ ] lockfile ごとに pnpm の cooldown 方針と除外を読み、ダウングレードにならない同一メジャー内の最小修正を選んだ。
- [ ] `too-new` と `blocked` の全エントリを、判断の前にトリアージした。
- [ ] 自動適用したのは clear かつメジャー跨ぎでないものだけで、メジャー跨ぎ・too-new・pnpm 除外はすべて明示的に判断した。
- [ ] pnpm と Go の出力はそれぞれのツールを通して再生成し、ハッシュ lockfile も方針の制御項目も編集していない。
- [ ] エコシステムに応じた audit・build・frozen install・ドリフト検査を実行した。
- [ ] 結果とユーザーが担う撤去 / 申し送り作業を日本語で報告し、stage・commit・push はしていない。
