# pkg

`pkg/` は、アプリケーション全体で共有される **汎用ユーティリティパッケージ群** を格納するディレクトリです。

## 方針

`pkg/` は、以下の基準を満たす場合にのみ、パッケージの追加を検討します。

- **複数箇所から参照される機能**であること
- **外部パッケージをラップ**し、アプリケーションコードが外部ライブラリに直接依存しないようにする目的であること

1つの機能からしか使われないヘルパーは、その機能のパッケージ内に配置してください。

外部 I/O を伴うパッケージ（例: `exec`, `fs`）は共通の形を取ります。すなわち、機能を
**インターフェース**として定義し、実依存を配線する具象実装（`OS{}` 等）を提供し、
`//go:generate mockgen` ディレクティブを付与してテストでモック注入を可能にします。

### `pkg/` とアプリ全体の横断的関心の違い

「複数箇所から参照される」だけでは **不十分** です。`pkg/` は **文脈非依存の汎用
ユーティリティ**（どのプロジェクトにもそのまま持ち出せ、このアプリのドメインやシステム上の
決定を一切知らないもの。例: `xerrors` / `uuid` / `ptr` / `stringkit`）のための場所です。

横断的ではあっても **このアプリ/システム固有** の関心 — アプリ全体のエラー taxonomy
（`internal/apperror`）、ロギング（`internal/logging`）、オブザーバビリティ
（`internal/observability`）、設定（`internal/config`） — は、層をまたいで使われていても
`pkg/` には **置きません**。これらはこのシステムの選択（エラー意味論、zap / otel 等の
フレームワーク）を内包するため、`internal/` 配下の横断的関心として置きます。domain 層が
これらのうち domain 層が依存してよい唯一の例外は `internal/apperror` です。

### 制約

- ビジネスロジックを含めてはならない
- `internal/` のパッケージに依存してはならない
- infrastructure やフレームワーク固有のパッケージに依存してはならない
- 他の `pkg/` パッケージに依存してはならない。例外は 2 つあり、いずれも `.golangci-full.yaml` の depguard `independent_pkg` で強制される。`pkg/xerrors` はどのパッケージからも import してよく、`testkit` サブパッケージは自身の親を import してよい（ルールのファイルパターンが `**/pkg/**/testkit/**.go` を除外している）
- 1パッケージ = 1責務を守ること

### doc コメントも状況非依存であること

上記の制約はコードだけでなく doc コメントにも及ぶ。`pkg/` は他プロジェクトへコピーしても成立することを
前提とするため、doc コメントに**このアプリケーション固有の文脈**を焼き込んではならない。特定の環境変数名、
現在の呼び出し元の名指し、このリポジトリのレイヤ構造をなぞった例のいずれも書かない。`DB_NAME` のような実名
ではなく `envutil.Override("SOME_KEY", "value")` と書き、リトライループの共有者は「tx リトライと外部 HTTP の
retry」ではなく「リトライ可能性を分類する任意の呼び出し側」と書く。現在の利用者の名指しは
[`docs/rules.md`](../docs/rules.md) § Comment Rules の「呼び出し元への言及」に該当するノイズでもある。

逆に、契約そのものは**過不足なく**書く必要がある。汎用ユーティリティは周囲のアプリケーションという文脈なしに
読まれるため、ポインタ引数の書き換え、`nil` の意味、範囲外入力の暗黙のクランプ、単位はいずれも doc コメントに
属する。

## パッケージ一覧

|パッケージ|概要|ラップ対象|
|---|---|---|
|`backoff`|指数バックオフの待機時間算出（純粋・時刻/乱数非依存）|なし|
|`datetime`|日時パース|標準ライブラリ `time`|
|`decimal`|exact-decimal 型（金額 / レート）|`github.com/shopspring/decimal`|
|`envutil`|環境変数の一時上書き（テスト補助）|標準ライブラリ `os`|
|`exec`|外部コマンド実行（インターフェース + モック）|標準ライブラリ `os/exec`|
|`fnmeta`|関数 / パッケージ名の抽出|なし|
|`fs`|ファイルシステム操作（インターフェース + モック）|標準ライブラリ `os`|
|`httpheader`|HTTP ヘッダ名の分類（資格情報を運ぶかどうか）|なし|
|`patch`|部分更新（PATCH）入力の 3 状態値|なし|
|`ptr`|ポインタ操作|なし|
|`retry`|有限リトライの行動層（backoff + full jitter, deadline-aware）|なし|
|`safecast`|オーバーフロー検出付き型変換|なし|
|`stringkit`|文字列長バリデーション|なし|
|`uuid`|UUID 型|`github.com/google/uuid`|
|`xerrors`|スタックトレース付きエラー|`github.com/cockroachdb/errors`|

## 各パッケージの詳細

### backoff

試行回数のみから指数バックオフの待機時間を算出する純関数で、時刻や乱数に依存しません（ジッタ付与は `retry` 側）。

|シンボル|説明|
|---|---|
|`Exponential`（struct）|`Initial` / `Max` / `Multiplier` の設定|
|`Duration(attempt)`|指定試行回数の基本待機時間を返す|

### datetime

複数の日時フォーマットに対応するパースユーティリティです。

主な関数

|関数|説明|
|---|---|
|`ParseRFC3339`|RFC3339 形式のパース|
|`ParseRFC3339UTC`|RFC3339 形式のパース（UTC）|
|`ParseRFC3339Nano`|RFC3339Nano 形式のパース|
|`ParseISO8601`|ISO8601 形式のパース|
|`ParseDateTime`|標準 datetime 形式のパース|
|`ParseDateOnly`|日付のみのパース|
|`ParseCustomLayout`|任意のレイアウトによるパース|

すべての関数に `ToLocation` バリアント（例: `ParseRFC3339ToLocation`）があり、タイムゾーンを指定したパースが可能です。

### decimal

`github.com/shopspring/decimal` をラップした exact-decimal 型です。vendor を seam の裏に隠蔽します（`pkg/uuid` の前例）。金額の意味論は持たず、通貨 / 非負 / 最小単位の選択は呼び出し側が所有します。本パッケージは純粋な十進算術・丸め・スケール変換と DB / ワイヤ境界だけを担います。ワイヤ表現は JSON 文字列です（JSON number は IEEE754 double として復元され精度を失うため）。

|シンボル|説明|
|---|---|
|`Parse` / `FromInt`|十進文字列 / `int64` から生成|
|`Add` / `Sub` / `Mul` / `Neg` / `DivRound`|正確な十進算術|
|`RoundHalfAwayFromZero` / `Truncate`|指定桁での丸め|
|`ToScaledInt64(n)`|n 桁で丸め `10^n` を掛けて最小単位 `int64` を返す（範囲外は `ErrOverflow`）|
|`Cmp` / `Equal` / `Sign` / `IsZero` / `IsNegative`|比較・検査|
|`MarshalJSON` / `UnmarshalJSON`|JSON 文字列のワイヤ表現（復元時は JSON number も受理）|
|`Scan` / `Value`|`NUMERIC` DB 境界（`sql.Scanner` / `driver.Valuer`）|

テストヘルパーは別パッケージ `pkg/decimal/testkit` にある（`MustParse`）。分離することで `testing` が本番バイナリへリンクされない。

### envutil

環境変数を一時的に上書きし、復元用の関数を返します（主にテストや設定読み込みで使用）。

|関数|説明|
|---|---|
|`Override(key, value)`|環境変数を設定し、元の状態へ戻す `func()` を返す|

### exec

外部コマンド実行をインターフェースで抽象化し、テストでモック注入を可能にします。本番は `OS{}` 実装を配線します。

|シンボル|説明|
|---|---|
|`Runner`（インターフェース）|`Output(ctx, dir, env, name, args)` — コマンドを実行し標準出力を返す|
|`OS`（構造体）|`os/exec` ベースの `Runner` 実装|

### fnmeta

`runtime` から取得したフル関数名を分解し、パッケージ名や関数名を抽出します。

|関数|説明|
|---|---|
|`ExtractFunctionName`|フル関数名からメソッド名を抽出|
|`ExtractPackageName`|フル関数名からパッケージ名を抽出|

### fs

ファイルシステム操作をインターフェースで抽象化し、テストでモック注入を可能にします。本番は `OS{}` 実装を配線します。

|シンボル|説明|
|---|---|
|`FS`（インターフェース）|`ReadFile` / `WriteFile` / `Glob`|
|`OS`（構造体）|`os` ベースの `FS` 実装|

### patch

部分更新（PATCH）入力の 3 状態を表す値です。「送られなかった（現在値を据え置く）」「`null` として送られた（クリアする）」「値付きで送られた（置き換える）」を区別します。素の `*T` ではこの区別が `nil` に潰れてしまいます。`Field[T]` のゼロ値は未指定のため、`Field` を並べた構造体の既定は「何も変更しない」になります。

|シンボル|説明|
|---|---|
|`Field[T]`（構造体）|部分更新における 1 フィールドの指定状態|
|`Unspecified[T]` / `Null[T]` / `Value[T]`|3 状態それぞれのコンストラクタ|
|`Field[T].Resolve`|現在値へ指定状態を適用する|

### ptr

ジェネリクスを利用したポインタ操作ユーティリティです。

|関数|説明|
|---|---|
|`To[T]`|値からポインタを生成|
|`Copy[T]`|ポインタのコピー（nil安全）|
|`Map[T,U]`|ポインタの指す値に関数を適用（nil は nil のまま）|
|`Deref[T]`|ポインタをデリファレンスし、nil の場合はフォールバック値を返す|

### retry

失敗分類を消費する有限リトライの行動層です（`classify → bounded attempts → backoff + full jitter → deadline-aware`）。乱数（full jitter）を本パッケージに閉じることで `backoff` の純粋性を保ちます。

|シンボル|説明|
|---|---|
|`Do`|分類関数がリトライ可能と判定する間、関数を有限リトライで実行|
|`Full`|full jitter（`[0, d]` の一様乱数）|
|`Policy`|`MaxAttempts` ＋ `Backoff`（`func(attempt int) time.Duration`）|
|`Sleeper`（インターフェース）|`Sleep(ctx, d)` 待機抽象（呼び出し側が持つ sleeper 型が構造的に充足）|

### safecast

オーバーフローを検出する安全な型変換を提供します。

|関数|説明|
|---|---|
|`UintToInt`|`uint` → `int` の安全な変換|
|`IntToInt32`|`int` → `int32` の安全な変換|
|`IntToInt16`|`int` → `int16` の安全な変換|
|`IntPtrToInt32Ptr`|`*int` → `*int32` の安全な変換（`nil` は変換対象なしとして `nil` を返す）|

オーバーフロー時は `ErrOverflow` を返します。

### stringkit

文字列の長さ（ルーン数）に基づくバリデーション関数群です。

|関数|説明|
|---|---|
|`RuneCount`|UTF-8 ルーン数を返す|
|`InRange`|長さが閉区間内か判定|
|`MaxOrLess`|長さが最大値以下か判定|
|`MinOrMore`|長さが最小値以上か判定|
|`StrictInRange`|長さが開区間内か判定|
|`LessThanMax`|長さが最大値未満か判定|
|`GreaterThanMin`|長さが最小値超過か判定|
|`ValidateInRange`|閉区間の長さ判定とエラーメッセージを同時に返す|

各関数に対応する `ErrorMsg` 関数があり、バリデーションエラーメッセージを生成できます。

### uuid

`github.com/google/uuid` をラップした UUID 型です。

テストヘルパーは別パッケージ `pkg/uuid/testkit` にあります（`NewTestFromSalt`）。分離することで `testing` が本番バイナリへリンクされません。

UUIDv7 を生成し、データベース連携（`sql.Scanner` / `driver.Valuer`）をサポートします。

|関数 / メソッド|説明|
|---|---|
|`New`|UUIDv7 を生成|
|`Parse`|文字列から UUID をパース|
|`NewTestFromSalt`|テスト用の決定的 UUID を生成|
|`String`|文字列表現を返す|
|`IsNil`|ゼロ値か判定|
|`Equal`|UUID の比較|
|`EqualPtr`|`*UUID` との比較（nil 安全）|
|`Bytes`|生の `[16]byte` を返す|
|`ToPtr`|値へのポインタを返す|
|`ToPrimitive` / `FromPrimitive`|`github.com/google/uuid` との相互変換（sqlc 連携など）|
|`MarshalJSON` / `UnmarshalJSON`|JSON 文字列のワイヤ表現（文字列以外は拒否、`null` は no-op）|
|`Scan` / `Value`|DB 連携用インターフェース実装|

### xerrors

`github.com/cockroachdb/errors` をラップし、スタックトレース付きのエラー操作を提供します。

|関数|説明|
|---|---|
|`New`|新しいエラーを生成|
|`Wrap`|既存エラーをラップ|
|`Is`|エラーの同一性を判定|
|`As`|エラーの型アサーション|
|`Join`|複数エラーを結合|
|`StackTrace`|スタックトレース文字列を取得|

## サブパッケージの `README.md` が持つもの

`pkg/<name>/` にはそれぞれ `README.md` があり、読み手が「このパッケージは求めているものか」を
判断するために最初に開くファイルです。次の節をこの順で持ちます。書くことが本当に無い節だけ省きます。

| 節 | 書くこと |
| --- | --- |
| `Role` | このパッケージが何のためにあり、責務の境界がどこにあるかを 1 段落で。 |
| `Public API` | 公開シンボルを全て、シグネチャと 1 行の契約付きで。ドリフト検査はここと突き合わせるので、ここに無い公開シンボルは未文書とみなされます。 |
| `Wraps` | ラップしている外部パッケージと、このパッケージが足す・隠すもの。何もラップしないなら省きます。 |
| `Notes` | シグネチャからは見えない落とし穴 — アロケーションの挙動、精度の限界、goroutine 安全性、ゼロ値を渡したときの振る舞いなど。 |

この 4 つより先はパッケージ側の裁量です。**省けないのは `Public API`** — `drift-detector-pkg` が
公開シンボルと突き合わせる対象なので、節名を変えたり落としたりすると検査が効きません。

## 新しいパッケージを追加する際のチェックリスト

- [ ] 複数箇所から参照される、または外部パッケージのラップである
- [ ] ビジネスロジックを含んでいない
- [ ] `internal/` に依存していない
- [ ] 1パッケージ = 1責務になっている
- [ ] テストが記述されている
- [ ] ドキュメントが記述されている
- [ ] この README にパッケージの概要が追加されている
- [ ] サブパッケージの `README.md` が `Role` / `Public API`（該当すれば `Wraps` / `Notes`）を持つ
