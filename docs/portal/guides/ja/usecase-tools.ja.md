# tools

`internal/usecase/tools` は、**Usecase 層で再利用される小さなユーティリティ群**を格納するディレクトリです。

## サブディレクトリ

|パッケージ|説明|詳細|
|---|---|---|
|`paging/`|ページネーション（page/perPage → limit/offset 変換）と、top-N を含む共通の件数ポリシー|[README](paging/README.ja.md)|
|`search/`|検索キーワードのトークン化（分割、重複排除、上限制限）|[README](search/README.ja.md)|
|`money/`|マネー計算（最小単位整数・レート適用 half-up）|[README](money/README.ja.md)|
|`timewindow/`|注文日時の半開区間 `[After, Before)` と空区間の規則|[README](timewindow/README.md)|

## 設計方針

- 複数の Usecase から共通利用されるユーティリティ
- ビジネスロジックを含まない — 機械的な変換のみ
- Infrastructure 依存なし
- ここへパッケージを足す前に、既存のもの（`paging` / `search` / `money` / `timewindow`）から形を導出する。いずれも非公開フィールド・検証付きコンストラクタ・ハンドラでの呼び出しを共有している。[docs/rules.md](../../../docs/rules.md) の *New Type Derivation* を参照。
