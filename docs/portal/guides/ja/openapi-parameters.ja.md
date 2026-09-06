# OpenAPI パラメータ

`openapi/components/parameters/` は、**再利用可能な OpenAPI パラメータ定義**を関心事ごとに整理して格納するディレクトリです。

## ディレクトリ構成

パラメータ群ごとに 1 つのディレクトリを置く。エンドポイントをまたいで共有するものは関心事の名前
（`pagination/` / `search/`）を、単一リソースが所有するものはそのリソース名を付ける。

## ページネーション戦略

本プロジェクトは**2つのページネーション方式**を提供し、それぞれが各方式で慣用的なパラメータ名を**あえて統一せず**保持しています：

- **オフセット**（`page` / `perPage`）— REST 流のページ移動。件数が有限でページ指定できる一覧向け。
- **カーソル / keyset**（`first` / `after`）— Relay 流の前方走査。offset が高コスト・不安定になる大規模・無限フィード向け。

カーソル側を `perPage` 等にリネームするのは非慣用的（カーソル走査に「ページ」概念はない）かつ API 破壊変更になるため、この分離は意図的です。

## 絞り込みパラメータの規約

いま存在するエンドポイントだけでなく、検索系すべてに適用します。

### 1 つの条件に複数の値を渡すときはパラメータ名を繰り返す

複数の値を取りうる条件は配列として宣言し、`categoryCodes=1&categoryCodes=2` のように同じ名前を
繰り返して送ります。これは OpenAPI の既定の直列化（`style: form` / `explode: true`）です。
配列として宣言していないパラメータでは「いずれかに一致」を表現できません。名前の繰り返しは
リクエスト検証が弾きます。

```yaml
explode: true
schema:
  type: array
  uniqueItems: true
  maxItems: 32
  items: { type: integer, format: int32, minimum: 1, maximum: 32767 }
```

`uniqueItems` と `maxItems` はワイヤ側の上限で、URL 長と生成される `IN` リストの大きさを抑えます。
<!-- sample-api:replace-begin -->
既存の例は `product/CategoryCodesParam.yaml` / `product/StatusCodesParam.yaml` と、
文字列 enum 配列の `purchase/PurchaseGroupByParam.yaml`（指定順に意味があり、繰り返しは順序を保ちます）。
<!-- sample-api:replace-with -->
<!-- = 指定順に意味のある文字列 enum 配列では、繰り返しても順序が保たれます。 -->
<!-- sample-api:replace-end -->

### 自由入力のテキストは配列にしない

利用者が入力したテキストを運ぶパラメータは、配列ではなく `maxLength` を持つ単一の `string` にします。
<!-- sample-api:replace-begin -->
`search/KeywordParam.yaml` がその形です。複数値を取ってよいのは、行への参照（`code`）と固定の `enum`
<!-- sample-api:replace-with -->
<!-- = 複数値を取ってよいのは、行への参照（`code`）と固定の `enum` -->
<!-- sample-api:replace-end -->
だけです。

上の規約が安く済むのはこの制限があるからです。percent-encode された UTF-8 は 1 文字あたり最大 9 文字に
なるため、自由入力パラメータ 1 本が URL 予算に与える影響は、配列の直列化形式の差より一桁大きくなります。
また区切り文字で連結する配列は、区切りが percent-decode の後に分割されるためエスケープが効かず、区切り
文字を含む値を運べません。配列を code と enum に限ると、この 2 つが同時に消えます。

これに収まらない検索 —— ファセットが多い、値が URL に載らないほど大きい —— は、クエリ文字列を伸ばすの
ではなく `POST` のボディへ移します。[ADR-0019 (search-query-parameter-shape)](../../../docs/adr/0019-search-query-parameter-shape.ja.md) を参照してください。

### マスタでの絞り込みは行の UUID ではなく `code` を受ける

クライアントがマスタ行を指すのに送る識別子は `code`——その行を入れた migration が固定した静的な別名です。
どの UUID を持つかは migration が決めるので、アプリケーションコードがそれを抱えてはいけません
（[ADR-0031](../../../docs/adr/0031-master-data-via-migration.md)）。同じ理屈が API 境界にも及びます。
`code` はクライアントが定数として持てますが、UUID は先にマスタのエンドポイントを叩いて解決する必要があり、
かつ 1 件 36 文字なので複数値の絞り込みを表現するコストが高くなります。

どのマスタ行にも一致しない code は 404 ではなく**0 件**を返します。取得対象ではなく絞り込み条件だからです。

### `code` は int16 の範囲で表す

`type: integer` / `format: int32` / `minimum: 1` / `maximum: 32767`。OpenAPI に int16 が無いため範囲は
境界値で表し、Go 側は `pkg/safecast` で絞り込みます。`SMALLINT` 列・sqlc が生成する `int16`・
ドメインの `minCode` / `maxCode` と一致します。

## 使い方

エンドポイント定義で `$ref` を使ってパラメータを参照します：

```yaml
parameters:
  - $ref: '../components/parameters/pagination/PageParam.yaml'
  - $ref: '../components/parameters/pagination/PerPageParam.yaml'
  - $ref: '../components/parameters/search/KeywordParam.yaml'
```

## 命名規則

|要素|規則|例|
|---|---|---|
<!-- sample-api:replace-begin -->
|ディレクトリ|小文字・関心事別|`pagination/`, `search/`, `stream/`, `user/`|
|ファイル名|PascalCase + `Param`|`PageParam.yaml`, `UserIdParam.yaml`|
<!-- sample-api:replace-with -->
<!-- = |ディレクトリ|小文字・関心事別|`pagination/`, `<リソース>/`| -->
<!-- = |ファイル名|PascalCase + `Param`|`PageParam.yaml`, `<リソース>IdParam.yaml`| -->
<!-- sample-api:replace-end -->

## ルール

- `in: path` パラメータは必ず `required: true` を設定する
- すべてのパラメータに `description` と `example` を記述し、ドキュメント品質を確保する
- クエリパラメータには適宜 `minimum`, `maximum`, `default` を設定する
- 再利用性を最大化するため、1ファイル = 1パラメータとする
