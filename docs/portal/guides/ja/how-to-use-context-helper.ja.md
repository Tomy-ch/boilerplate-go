# ctxhelper

ctxhelperは「contextの利用を制御する境界レイヤ」です。

このパッケージは、リクエストスコープの値を context.Context で受け渡すためのヘルパー関数を提供します。

## 実装方法

値を格納するだけの単純なキーはコード生成で作成します。生成機構の詳細については、以下を参照してください：

- `scripts/genctxkey/README.ja.md`

`Authn` のヘルパー（`authn.go`）は手書きです。OpenAPI の `AuthenticationFunc` は context を前方伝播できないため、認証前にミドルウェアが仕込む可変スロットで受け渡します。

## 提供するヘルパー

手書き（`authn.go`）— `Authn` スロット:

- `WithAuthn(ctx) context.Context` — 空の `Authn` スロットを仕込む（認証前に呼ぶ）
- `SetAuthn(ctx, authn) bool` — スロットへ書き込む。スロットが無ければ `false`
- `GetAuthn(ctx) (auth.Authn, bool)` — スロットから読む。未設定なら `ok=false`
- `RequireAuthn(ctx) (auth.Authn, error)` — スロットから読む。未設定なら `ErrUnauthenticatedUser` を返す
- `RequireUserID(ctx) (uuid.UUID, error)` — スロットの `Authn` から解決済みの内部ユーザー ID を取り出す
- `SetAuthnFailure(ctx, err) bool` — 認証の失敗を記録する。スロットが無ければ `false`
- `AuthnFailure(ctx) error` — 記録された失敗を読む。認証が失敗していなければ `nil`

手書き（`stream_grant.go`）— `StreamGrant` スロット。`StreamTicket` security scheme（ADR-0074 (query-ticket-stream-authentication)）が検証済み ticket の束縛を書き込む。拒否された ticket は `SetAuthnFailure` で記録され、同じ fail-closed の段で拒否される:

- `WithStreamGrant(ctx) context.Context` — 空のスロットを仕込む（認証前に呼ぶ）
- `SetStreamGrant(ctx, grant) bool` — 検証済みの `realtime.StreamGrant` を書き込む。スロットが無ければ `false`
- `GetStreamGrant(ctx) (realtime.StreamGrant, bool)` — 読む。未設定なら `ok=false`
- `RequireStreamGrant(ctx) (realtime.StreamGrant, error)` — 読む。未設定なら `ErrStreamGrantMissing` を返す
- `SetStreamRevalidator(ctx, fn) bool` / `GetStreamRevalidator(ctx) (fn, bool)` — 同じスロットは、*同じ* ticket をもう一度検証する手段も運ぶ。stream 接続の確立は ticket の検証 → 外部 I/O → 登録の順に進み、その間に届いた失効通知はまだ索引に載っていない接続を取りこぼす。登録の後に一度だけ検証し直すことでその窓が閉じる。資格情報そのものは security scheme の中に留まり、渡っていくのは closure だけ

スロットが成功だけでなく失敗も運ぶのは、spec が認証を任意と宣言することがあり、その operation の
バリデーションは提示された資格情報が拒否されても成功してしまうためです。失敗がスロットに残ることで、
後段がリクエストを拒否できます。

生成（`genctxkey`、`generate.go` に定義）— リクエストスコープの真偽値フラグ。各名前は `context.Context` 用と `*echo.Context` 用のペアを提供します:

- `ErrorHandled` — `SetErrorHandled` / `GetErrorHandled`、`SetErrorHandledToEcho` / `GetErrorHandledFromEcho`
- `Recovered` — `SetRecovered` / `GetRecovered`、`SetRecoveredToEcho` / `GetRecoveredFromEcho`

## 使用方法

ctxkeyを追加する場合は、以下のように `generate.go` に定義を追加します。

```go
//go:generate go run ../../../scripts/genctxkey --name UserID --type string --out .
```

外部型を使用する場合：

```go
//go:generate go run ../../../scripts/genctxkey --name Actor --type "auth.Authn" --import go-boilerplate/internal/usecase/boundary/auth --out .
```

上記は外部型指定の構文例にすぎません。本パッケージの実際の `Authn` スロットは手書き（`authn.go`）であり、このコマンドでは生成**しません**。

その後、以下を実行します。

```bash
make gen-go-code
```

## type の指定方法

### 基本型 / 同一パッケージ型

```bash
--type string
--type UserID
```

- import は不要
- そのまま型として扱われます

### 外部パッケージの型

```bash
--type "auth.Authn"
--import go-boilerplate/internal/usecase/boundary/auth
```

- `--type` は Go の型式で指定
- `--import` でパッケージを明示
- `--alias` は任意

### 複雑な型

```bash
--type "*[]auth.Authn"
--type "map[string]auth.Authn"
```

- pointer / slice / map / generic に対応

## 注意

- import path のみ（例: `github.com/foo/bar`）は指定不可
- 外部型は必ず `--type` と `--import` をセットで指定

## テスト戦略

この package は inbound adapter でも middleware でもないので、[`../README.ja.md`](../README.ja.md) の観点は当てはまらない。観点はここで持つ。

- **スロットの 3 状態を両側から** — 全アクセサを、スロット未仕込み / 仕込み済み未設定 / 書き込み済みの 3 状態で表明する。`With*` は*空の*スロットを仕込む（直後の `Get*` は未設定を返す）、`Set*` はスロットが無ければ `false`・あれば `true`、`Get*` / `Require*` は未設定と書き込み済みを区別する。効くのは「仕込み直後は未設定」のケースで、ゼロ値で設定済みに見えるスロットは、未認証の request が認証済みとして通る道そのもの。
- **成功と並ぶ失敗** — `Authn` スロットでは記録した失敗が後の読み取りまで残ることを、成功経路と独立に表明する。
- **生成されたフラグ** — `genctxkey` の生成物は生成されたテストが固定する。ここで重複させない。

## 編集について

本ディレクトリ内の `.gen.go` ファイルは自動生成されたコードです。

- 原則として手動編集は禁止
- 変更は `scripts/genctxkey` を通して行ってください

手書きのヘルパー（`authn.go` など）は直接編集します。
