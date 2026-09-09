# infrastructure/objectstorage

> このドキュメントは [README.md](README.md) の日本語訳です。内容の更新は英語版から反映してください。

オブジェクトストレージの seam（`internal/usecase/boundary/objectstorage`）を、具体的なストレージサービスに対して実装する adapter 群です。

## 役割

`objectstorage.Storage` ポートを実装します。実装の選択はこのパッケージの `New` が行い、S3 互換の実装は `s3/` が持ちます。vendor の語彙（bucket / region / endpoint / SDK の型）はここで止まり、上位のポートが露出するのはキー・バイト列・内容のメタデータだけです。

## ディレクトリ構造

|パス|役割|
|---|---|
|`objectstorage.go`|実装を選ぶ唯一の場所。DI グラフも CLI もここを呼ぶため、基盤の差し替えは 1 関数の書き換えで済む|
|`s3/`|S3 互換 adapter（AWS SDK v2）。`New(Config, TracerFactory)` がポートを返し、具体型は非公開のまま|

## レイアウト規約

- **基盤非依存の契約**は、この層より上の `internal/usecase/boundary/objectstorage`（seam）にあります。infrastructure はそのポートを実装するだけで、抽象を所有しません。
- **基盤固有の adapter** は `objectstorage/<substrate>/`（例 `objectstorage/s3`）に置きます。パッケージ名を基盤名にすることで、import 箇所で具体技術が見えるようにします。

## ポート対応

| seam | S3 |
| --- | --- |
| `Put(ctx, PutObject) (Path, error)` | `PutObject`。`Key` → `Key`（設定された `Bucket` 配下）、`Body` → body + `ContentLength`、`ContentType` → `ContentType`、`CacheControl` → `CacheControl`。`CacheControl` が空の場合は空ヘッダを送らずフィールドを未設定にするため、「キャッシュ指示が無い」と「空の指示がある」を区別できる |
| 戻り値 `Path` | 書き込んだキーをそのまま返す。adapter は URL を返さない。配信オリジンはストアの属性ではないため、URL の組み立ては呼び出し側の責務 |
| `List(ctx, ListQuery) (ListResult, error)` | `ListObjectsV2`。`Prefix` → `Prefix`、`Limit` → `MaxKeys`、`Cursor` → `ContinuationToken`。`Contents` の各要素は `Key` + `LastModified` から `Object{Key, ModifiedAt}` になる。`NextCursor` に `NextContinuationToken` を載せるのは **`IsTruncated` が true のときだけ**で、そうでなければ空にするため「続きがある」と「最終ページ」を区別できる。`Prefix` / `Cursor` が未指定の場合は空文字を送らずフィールドを立てない |
| `Delete(ctx, keys) error` | `DeleteObjects`（`Quiet: true`）。各キーを `ObjectIdentifier` へ写し、`keys` が空ならリクエストを送らず終了する。S3 はキー単位の失敗を呼び出しエラーではなくレスポンス本文で返すため、`Errors` が空でなければ失敗として扱う。消えていないキーを消えたものとして扱うと、回収ジョブが削除に失敗したオブジェクトを報告しなくなるため |

## エラー正規化

SDK の失敗は各呼び出し箇所（`Put` / `List` / `Delete`）で `apperror.ErrUnavailable` へ包みます。上位層は sentinel で分岐し、AWS のエラー型を検査することはありません。これは **RDB 側より意図的に粗い**作りです。`rdb/pgerror` が SQLSTATE を個別の sentinel へ写すのは、呼び出し側が一意制約違反とデッドロックで異なる振る舞いを取るからですが、このポートの失敗はいずれも「ストアが操作を受け付けなかった」に収束します。とりわけ `Delete` は区別すべき not-found を持ちません。S3 は存在しないキーの削除を成功として扱い、それが回収ジョブを再実行可能にしている性質です。not-found と denied を呼び出し側が区別しなければならない操作が加わった時点で、写像を分ける価値が生まれます。

## 設定

`s3.Config` は `OBJECT_STORAGE_*` から設定されます（[env/README.md](../../../env/README.md) を参照）。

- `Endpoint` — 空は SDK 既定の解決、すなわち実在の AWS S3 を意味します。空でない値は互換サービス（ローカルでは Garage コンテナ）を指します
- `UsePathStyle` — Garage / MinIO では `true`、AWS S3 では `false` である必要があります
- `Region` は AWS 以外のサービスに対してもリクエスト署名に使われます

## 可観測性

`Put` は注入された `TracerFactory` を通じて infrastructure 層のスパンを開きます。そのため、ストアの呼び出しは、それを起こした handler や usecase と同じトレース上に現れます。adapter 自身はログを出さず、失敗は正規化されたエラーとして表面化します。

## 既定で配線される — SQS adapter との違い

この adapter は既定の DI グラフに**入っています**。したがって `aws-sdk-go-v2/service/s3` と SDK コアは出荷バイナリにリンクされます。[`queue/sqs`](../queue/sqs/README.ja.md) は削除可能なサンプル群からのみ配線されるため、サンプルを削除すれば `service/sqs` はバイナリから外れます（[ADR-0053 (broker-sdk-isolation-measured-as-coupling)](../../../docs/adr/0053-broker-sdk-isolation-measured-as-coupling.ja.md)）。

この非対称性は意図的です。worker はブローカーが選ばれるまでブローカーを持ちませんが、オブジェクトストレージのポートは最初から使われており、宣言以上のものであるためには動く実装が要ります。何も保存しない構成は `InfrastructureModule()` から `objectStorageModule()` を外せます。

## テスト方針

- **単体テストは in-process の `gofakes3` に対して実行します**。コンテナ不要なので `make go-test` は何も起動していなくても通ります。fake はテストごとに起動し、adapter を動かすのに足るだけの S3 API を話します
- **Garage コンテナは `make serve` 用**であってテスト用ではありません。実配信について確認すること（公開 read・キャッシュヘッダ）は、そのコンテナに対して手で検証します
- `gofakes3` は受け取ったヘッダのすべてを保存するわけではありません（`Cache-Control` もその一つ）。そのため、adapter が**送出する**ヘッダに関する検証は、保存されたオブジェクトではなく送出リクエストを見ます

## S3 は実例の一つであって前提ではない

seam は基盤非依存ですが、背後に何も無いまま出された抽象は何も証明しません。ポートは、実物が 1 本通って初めて信用できるものになります。そこで 1 つの基盤だけを具体的に実装しており、S3 API がそれにあたります。これは**リファレンス**であって前提ではありません。S3 API はこの領域で最も lingua franca に近い存在であり、だからこそローカルコンテナが AWS 自身ではなく Garage になっています。

この主張を建前で終わらせないために、各クラウドのオブジェクトストレージをローカルで開発する際のコンテナを挙げます。差し替えた側が初日から同じ開発ループを立ち上げられるようにするためです。

|プロバイダ|サービス|ローカルコンテナ|ライセンス|提供元|
|---|---|---|---|---|
|AWS（および S3 互換全般）|S3|`dxflrs/garage`|AGPL-3.0|Deuxfleurs（非営利団体）|
|Azure|Blob Storage|`mcr.microsoft.com/azure-storage/azurite`|MIT|Microsoft|
|GCP|Cloud Storage|`fsouza/fake-gcs-server`|BSD-2-Clause|fsouza（個人メンテナ）|

選定基準はここでの他の依存と同じく「1 コンポーネント 1 責務」（[ADR-0081 (library-selection-policy)](../../../docs/adr/0081-library-selection-policy.md)）です。したがって、クラウド一式をエミュレートするスイートよりも単機能のエミュレータを優先します。各選択の補足は次のとおりです。

- **Garage** は S3 API だけを話し、checkout ごとに動かせる程度に小さいままです。AGPL-3.0 の義務は改変した Garage を配布する場合に生じるものであり、公開イメージを開発時の依存として動かす分には、それと通信するアプリケーション側に義務は生じません
- **Azurite** は Microsoft 自身のエミュレータで、Blob と Queue の**両方**を賄います。したがって Azure は、この seam と worker seam の 2 つに対して 1 コンテナで済みます
- **fake-gcs-server** は事実上の Cloud Storage エミュレータであり、実利用者を持つ唯一の選択肢です。ただしここに挙げた中で唯一、組織ではなく個人がメンテナンスしており、タグ付きリリースの間隔も他より長くなっています。CI で依存する前にその点を検討してください

S3 互換のもの（MinIO・Ceph RGW・Cloudflare R2・マネージドな S3）であれば adapter の変更は一切不要で、`ENDPOINT_OBJECT_STORAGE` と資格情報だけで足ります。S3 でない基盤には `objectstorage/` 配下の兄弟パッケージが必要ですが、この層より上は何も変わりません。

## 資格情報

資格情報は SQS adapter と共有する [`infrastructure/awsclient`](../awsclient/README.ja.md) が解決します。
`OBJECT_STORAGE_ACCESS_KEY_ID` / `OBJECT_STORAGE_SECRET_ACCESS_KEY` を空にしておけば SDK 既定の chain へ
解決を委ねられるため、IAM ロールで動くデプロイはコードを変えずに済みます。両方を設定すれば chain を
上書きし、ローカルの Garage の資格情報がそれにあたります。いずれの場合も解決は起動時に試みるため、
使えない設定は初回のアップロードではなくその場で落ちます。
