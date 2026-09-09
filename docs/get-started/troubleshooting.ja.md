# トラブルシューティング

リポジトリのセットアップとローカル実行で遭遇しやすい失敗を、**実際に目にする症状**から引けるように
並べたものです。各項目は原因を示したうえで、その機構を所有するドキュメントへ引き渡します。説明の
本体はそちらにあり、ここには置きません。

**想定どおりの失敗**をそうと認識できることには価値があります。以下のいくつかは環境が仕事をしている
だけであり（fail-closed なプロバイダ、CI へ委譲したゲート）、それをバグとして読むと失敗そのものより
時間を失います。

## セットアップ

### `mise` を入れたのに固定したツールが "not found" になる

```txt
make: golangci-lint: No such file or directory
```

`mise` をインストールしただけで、**シェルで有効化していない**状態です。すべての Make ターゲットは
mise の shim 経由でツールを解決し、shim が `PATH` に載るのは有効化を通してのみです。インストールでは
足りません。有効化手順と確認方法は [setup-repository.ja.md](setup-repository.ja.md) の Phase 1 です。

### Git フックが一度も動かない

`lefthook` は `make install-tools` で入りますが、`.git/` への配線は別ステップの `make activate-tools`
が行います。それまではコミットもプッシュもローカルゲートを素通りし、最初に気づくのは CI です。

### Docker イメージのビルドが `403 Forbidden` で落ちる

ツールイメージは `mise` でツールを解決し、mise は GitHub Releases API を読みます。未認証の呼び出しは
1 回のビルドが必要とする量をはるかに下回る上限で頭打ちになり、リトライのたびに新しい枠を食い潰します。
make はトークンを見つけられれば（`GITHUB_TOKEN`、無ければ `gh auth token`）ビルドへ渡すので、対処は
トークンを用意することです。`gh auth login` で足ります。詳細は
[local-environment.ja.md](../maintenance/local-environment.ja.md)。

## ローカル実行

### 起動時に `no authorizer configured for environment` で終了する

想定どおりであり、バグではありません（`no authenticator configured for environment` の形でも出ます）。
認証・認可のプロバイダは `local` / `ci` / `test` に**限って**配線されており、`development` /
`staging` / `production` では fail-closed です。実装を配線するまでプロセスは起動を拒否します。これは
署名を検証しない認証器や全許可の認可器を実環境へ出さないための強制装置です。両者の実装は
[setup-repository.ja.md](setup-repository.ja.md) の Phase 11、設計は
[auth.ja.md](../design/auth.ja.md) にあります。

### ホストポートが既に使われている

`database` と可観測性のサービスは**全チェックアウトで共有**されます（作業ツリーごとではなく単一の
インスタンスです）。したがってポートの使用中は、壊れているのではなく別のチェックアウトが共有インフラを
起動していることを意味するのが普通です。どのポートが固定で、どれがスロット単位（`8080+N` など）か、
なぜその番号なのかは [local-environment.ja.md](../maintenance/local-environment.ja.md)。複数 worktree を
同時に動かすときは 2 つ目のスタックを立てず、スロットを借りてください:
[db-worktree-pool.ja.md](../maintenance/db-worktree-pool.ja.md)。

### 生成物が root 所有になり `git` が触れなくなった

コード生成と lint はリポジトリをバインドマウントした tool-runner コンテナ内で走り、その中のプロセスは
root です。復旧コマンドは `repo-ops` スキルにあり、機構の説明は
[local-environment.ja.md](../maintenance/local-environment.ja.md) にあります。

## データベースとテスト

### テーブルや行が無いことでテストが落ちる

テストスイートは `make db-init` の実行を前提とします。これは local / test の両データベースに対して
マイグレーション**とシード**を行います。マイグレーションだけのターゲットはスキーマを上げてもシードを
入れないため、失敗はもっと後の、DB と無関係に見えるテストで表面化します。

### `make gen-query` が `connection refused` で落ちる

`gen-query` は生成前に `pg_dump` で**実データベース**のスキーマをダンプするため、データベースコンテナが
起動している必要があります（`docker compose up -d database` または `make serve`）。

### `CREATE DATABASE` が collation version mismatch で落ちる

```txt
ERROR: template database "template1" has a collation version mismatch (SQLSTATE XX000)
```

データボリュームを残したまま `database` イメージのベース OS が変わった状態です。失敗するのは
データベースを**作成する**経路だけで、既存のものへの接続は警告に留まります。データベースの作り直しは
回避策になりません。対処は共有インスタンス全体に対する一度きりの REINDEX と collation の refresh で、
そのコマンドと、共有インスタンスゆえに二度嚙まれる理由は
[local-environment.ja.md](../maintenance/local-environment.ja.md) にあります。

## ゲートと生成物

### ローカルではビルドが通るのに CI が生成物で落ちる

生成物はコミットされており、CI は再生成して差分があれば落とします。したがって OpenAPI 定義や SQL を
変更したあとに `make gen`（または個別の `make gen-api` / `make gen-query`）を回していないと、ビルド時
ではなくそこで捕まります。再生成して結果をコミットしてください。生成ファイルの手編集は行いません。

### エディタの lint 結果が `make go-lint` と食い違う

意図的です。`golangci-lint` は暗黙に `.golangci.yaml` を拾い、これはエディタの応答性に合わせた最小構成
です。権威あるゲートは `.golangci.yaml` で、`make go-lint` / `make go-fix` が明示的に渡し、レイヤー境界の
depguard ルールを持つのもこちらです。エディタが静かなことは根拠になりません。理由は
[ADR-0088（layered-golangci-config）](../adr/0088-layered-golangci-config.ja.md)。

### ローカルのゲートが動かなくなったように見える

失われたのではなく、繰り延べられています。ローカルで何をどこまで走らせるかは開いている worktree の数
から決まり、閾値を超えると重いゲートは CI へ委ねられ、そこで同一に再実行されます。`make load-status` が
解決された帯域と各ツールへ渡る内容を表示します。帯域そのものの説明は
[.makefiles/README.ja.md](../../.makefiles/README.ja.md)。

### ブランチを切り替えたあとに `vendor/` の不整合が出る

`vendor/` は gitignore されているため、壊れるのは他人の `go.mod` 変更を**受け取っただけ**のチェック
アウトです。`post-checkout` / `post-merge` フックが `make vendor-sync` を走らせるのはまさにこのため
なので、フックが有効でなかった場合は手で実行してください（上の「Git フックが一度も動かない」を参照）。

## 関連ドキュメント

- [setup-repository.ja.md](setup-repository.ja.md) — このページが前提とする段階的セットアップ
- [local-environment.ja.md](../maintenance/local-environment.ja.md) — コンテナ・ポート・tool-runner
- [db-worktree-pool.ja.md](../maintenance/db-worktree-pool.ja.md) — 共有データベースに対する複数 worktree の運用
- [.makefiles/README.ja.md](../../.makefiles/README.ja.md) — 領域別の全ターゲット
