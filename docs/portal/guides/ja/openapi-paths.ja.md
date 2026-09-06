# OpenAPI Paths

`openapi/paths/` は、**API エンドポイントのルーティング定義**をリソース・バージョン単位で格納するディレクトリです。

## ディレクトリ構成

パス 1 本につき 1 ファイルを置き、最後のセグメントを名前にする。子を持つパスは、自身のファイルと
並べて同名のディレクトリを持つ。したがってディレクトリ構造は URL の構造をそのまま写す。

## エンドポイントのカテゴリ

### 運用系エンドポイント

監視・オーケストレーション用のインフラエンドポイントです。ビジネス API には含まれず、`Skipper` により OpenAPI バリデーションと認証の対象外となります。

|パス|ファイル|説明|
|---|---|---|
|`/health`|`health.yaml`|ヘルスチェック|
|`/healthz`|`healthz.yaml`|Kubernetes liveness probe|
|`/ready`|`ready.yaml`|レディネスプローブ|
|`/version`|`version.yaml`|ビルドバージョン / リビジョン / 日時|
|`/metrics`|`metrics.yaml`|Prometheus メトリクス（Basic 認証保護）|

### バージョニング API（`v1/`）

**URL バージョニング戦略**に従うビジネス API エンドポイントです。

<!-- sample-api:replace-begin -->
```text
/v1/users             → users.yaml
/v1/users/{userId}    → users/userId.yaml
/v1/users/me          → users/me.yaml
/v1/users/search      → users/search.yaml
```
<!-- sample-api:replace-with -->
<!-- = ```text -->
<!-- = /v1/<resources>             → <resources>.yaml -->
<!-- = /v1/<resources>/{<id>}      → <resources>/<id>.yaml -->
<!-- = /v1/<resources>/me          → <resources>/me.yaml -->
<!-- = /v1/<resources>/search      → <resources>/search.yaml -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

> `v1/` の内容は**サンプル実装**です。サービス構築時には独自のリソースに置き換えてください。

`v1/` の中で 1 つだけ sample ではない path がある。`/v1/streams/{destination}`（`v1/streams/destination.yaml`）は Realtime Delivery 機構の generic な SSE endpoint で、sample 削除後も残り、Bearer ではなく query の `StreamTicket` scheme で認証し、`strict-server` 無しで生成される唯一の operation である。handler は `handler/` の下ではなく隣の `internal/controller/stream/` にある（同 package の README と `docs/design/realtime-delivery.md` §3.1 を参照）。

#### バージョニング戦略

本プロジェクトでは **URL パスバージョニング**（`/v1/`、`/v2/` 等）を推奨しています。

- URL とドキュメントで明示的かつわかりやすい
- 複数の API バージョンを並行運用可能
- 破壊的変更は新しいバージョンプレフィックスで導入
- 非破壊的な追加は現在のバージョン内で行う

破壊的変更を導入する場合は、`v1/` と並行して `v2/` ディレクトリを作成してください。

### 内部型

`internal/types/error_response.yaml` は `oapi-codegen` の型生成で使用されるエラーレスポンス型です。公開 API エンドポイントではありません。

## 命名・構造ルール

<!-- sample-api:replace-begin -->
|要素|規則|例|
|---|---|---|
|ファイル名|URL セグメントに一致（小文字）／パスパラメータファイルは param の `camelCase` 名に一致|`search.yaml`, `userId.yaml`|
|ディレクトリ構成|URL パスをミラー（1 path item = 1 ファイル）|`/v1/users/search` → `v1/users/search.yaml`|
|leaf と親|leaf = フラットな `<segment>.yaml`／エンドポイント＋子あり = `<segment>.yaml` と `<segment>/` を併存|`users.yaml` ＋ `users/`|
|パスパラメータ|param の `camelCase` 名に一致するファイル名（波括弧なし）|`{userId}` → `userId.yaml`|
|operationId|`{HTTPメソッド}{リソース名}`（PascalCase・動詞始まり）|`GetUsers`, `PostUsers`|
|tags|パスベースのグルーピング|`v1/users`, `health`|
<!-- sample-api:replace-with -->
<!-- = |要素|規則|例| -->
<!-- = |---|---|---| -->
<!-- = |ファイル名|URL セグメントに一致（小文字）／パスパラメータファイルは param の `camelCase` 名に一致|`search.yaml`, `<id>.yaml`| -->
<!-- = |ディレクトリ構成|URL パスをミラー（1 path item = 1 ファイル）|`/v1/<リソース>/search` → `v1/<リソース>/search.yaml`| -->
<!-- = |leaf と親|leaf = フラットな `<segment>.yaml`／エンドポイント＋子あり = `<segment>.yaml` と `<segment>/` を併存|`<リソース>.yaml` ＋ `<リソース>/`| -->
<!-- = |パスパラメータ|param の `camelCase` 名に一致するファイル名（波括弧なし）|`{<id>}` → `<id>.yaml`| -->
<!-- = |operationId|`{HTTPメソッド}{リソース名}`（PascalCase・動詞始まり）|`Get<リソース>`, `Post<リソース>`| -->
<!-- = |tags|パスベースのグルーピング|`v1/<リソース>`, `health`| -->
<!-- sample-api:replace-end -->

## パスとハンドラの対応

パス定義はハンドラ実装と対応します：

<!-- sample-api:replace-begin -->
```text
paths/v1/users.yaml             → handler/v1/users/
paths/v1/users/userId.yaml      → handler/v1/users/detail/
paths/v1/users/me.yaml          → handler/v1/users/detail/
paths/v1/users/search.yaml      → handler/v1/users/search/
```
<!-- sample-api:replace-with -->
<!-- = ```text -->
<!-- = paths/v1/<resources>.yaml             → handler/v1/<resources>/ -->
<!-- = paths/v1/<resources>/<id>.yaml        → handler/v1/<resources>/detail/ -->
<!-- = paths/v1/<resources>/me.yaml          → handler/v1/<resources>/detail/ -->
<!-- = paths/v1/<resources>/search.yaml      → handler/v1/<resources>/search/ -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

## ルール

- `$ref` でスキーマ・パラメータ・レスポンスを参照する — インライン定義しない
- パスファイルは責務ごとに分割する（一覧 vs 詳細 vs 検索）
- このディレクトリでは `#/` フラグメントポインタを使用しない
- `components/` との相互参照がしやすいよう命名を統一する
- 各パスファイルは単一のルートを定義する
