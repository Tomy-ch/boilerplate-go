# コントリビューションガイド

このページが扱うのは**変更がどう流れるか**です。どこからブランチを切り、どうコミットし、何が緑で
なければならず、レビュアーが何を尋ねるか。アーキテクチャ上のルールはここには置きません。それらは
[docs/rules.md](docs/rules.md) と各パッケージの `README.md` にあり、それらと食い違う変更はレビュー
コメントではなく設計の議論です。

このリポジトリで最初の変更を書くなら、まず [docs/architecture.md](docs/architecture.md) と
[docs/development-flow.md](docs/development-flow.md) を読んでください。変更の種類ごとにどこから
着手するかを述べているのは後者です。API の変更は OpenAPI 定義から、スキーマの変更はマイグレーション
から始まり、どちらも Go からは始まりません。

## 着手する前に

[docs/get-started/setup-repository.md](docs/get-started/setup-repository.md) の手順でリポジトリを
セットアップし、git フックが配線されていること（`make activate-tools`）を確認してください。フックを
入れていないことは、手元なら 1 秒で捕まえられたものでプルリクエストが落ちる最も多い原因です。

そのセットアップで想定どおりに動かないものがあれば、調べ始める前に
[docs/get-started/troubleshooting.md](docs/get-started/troubleshooting.md) を見てください。いくつかの
失敗は環境が設計どおりに働いている結果です。

**エージェント設定も入れてください**（同ページの Phase 3）。AI 支援開発はこのリポジトリの標準経路です。
`docs/development-flow.md` のフローは `.claude/` / `.codex/` のスキルという実行形態を持ち、レビュアーが
指摘する規約は、それらのスキルが既に適用しているものです。これら無しで作業することは許されていますが
互換経路であり、本来スキルが駆動する作業を手で行うことになります。手動側に同等のツールは保守されて
いません。一方で **結果は任意ではありません**。以下のゲートはどちらの経路で作られた変更にも同じように
適用され、決定論的な検査はアシスタントの報告に優先します。根拠:
[ADR-0007 (agents-md-operational-contract)](docs/adr/0007-agents-md-operational-contract.ja.md)。

## ブランチ

フィーチャーブランチは**最新の `release/*` ブランチ**から切ります。保護ブランチからは切らず、古い
ベースからも切りません。どれが最新かは `make base-branch` がリモートの生きた状態から解決します。
記憶で選んだベースは、他の全員が既に持っているファイルがブランチに無い、という形で表面化します。

ブランチ名は何をするものかを表し、issue 番号があるときは含めます:

```txt
feature/1234-add-authentication-check
feature/add-authentication-check
```

`production` / `staging` / `develop` / `release/*` への直接コミットはブランチ保護が拒否します。作業中に
ベースが進んだときは、rebase ではなく**マージ**で取り込んでください。生成物の衝突をどう解消するかを
含む完全な規則は [docs/rules.md](docs/rules.md) にあります。

## コミット

コミットメッセージは種別のプレフィックスで始め、その集合は `commit-msg` で `commitlint` が強制します:

```txt
Feat | Fix | Refactor | Perf | Docs | Test | Build | CI | Chore | Style | Revert
```

機械的に強制されるのはプレフィックスと件名が空でないことだけです。それ以外は規約です。1 コミット
1 スコープ（revert が 1 つの事柄についての判断になるように）、そして件名はどのファイルが動いたかでは
なく何が変わったかを述べること。このリポジトリのコミットメッセージは日本語で書かれていますが、強制
される部分は言語に依存しないので、テンプレートから作成したプロジェクトは自分で決められます。

## プッシュする前に

`pre-commit` / `pre-push` で走るゲートは CI が走らせるものと同じです。最も速いレビューは、先にそれらを
自分で通したものです:

```sh
make go-fix     # フォーマット + lint 自動修正
make go-lint    # 権威ある golangci-lint ゲート
make go-test    # カバレッジ付きテスト（先に `make db-init`）
make gen     # OpenAPI・SQL など生成対象を変更したときだけ
```

このうち実際に手元で走る量は、開いている worktree の数から決まります。閾値を超えると重いゲートは CI へ
繰り延べられ、そこで同一に再実行されます。どの帯域にいるかは `make load-status` が答えます。

変更が「完了」なのはコンパイルが通ったときではなく、**テストされた**ときです。カバレッジの基準、
レイヤーごとに検証するという前提、ユニットテストでは代替できないランタイム検証は
[docs/rules.md](docs/rules.md)（*Testing & Definition of Done*）と
[docs/testing-conventions.md](docs/testing-conventions.md) が定義しています。

## プルリクエスト

テンプレートが求める 3 つの節（概要・変更内容・動作確認方法）を埋めてください。レビュアーが頼りにする
のは 3 つ目です。あなたの検証を再現できないレビュアーは、差分だけを見ていることになります。

- **必須チェック**が通る必要があります。デプロイブランチへの昇格ゲートはプルリクエスト単位のものより
  厳しく、どちらも [.github/workflows/README.ja.md](.github/workflows/README.ja.md) に一覧があります。
- **CODEOWNERS レビュー**が必須であり、「この判断はこの役割のものだ」を助言ではなく実効にしています。
- **生成ファイルは手編集しません。** CI が再生成して差分で落とすため、編集された生成物はレビューを
  生き延びません。

## 判断の記録が要るとき

システムが今の形をしている*理由*を変える変更 — 境界が動く、技術が入れ替わる、制約が外れる — は、
コードだけでなく [docs/adr/](docs/adr/README.md) の ADR としても残します。基準は再発性です。将来
蒸し返される判断か、後の読み手が差分から逆算する羽目になる判断かどうかです。

ドキュメントは同じ変更の中でコードに追随させます。正本は英語で、それぞれ日本語訳（`*.ja.md`）と対に
なります。対とドキュメントポータルの整合を保つ規則は
[docs/maintenance/docs-structure.md](docs/maintenance/docs-structure.md) にあります。

## セキュリティ

脆弱性を issue やプルリクエストで報告しないでください。[.github/SECURITY.ja.md](.github/SECURITY.ja.md) の
非公開の窓口を使ってください。
