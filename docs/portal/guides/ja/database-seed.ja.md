# seed

`database/seed` は、**非本番環境（ローカル / CI / ステージング）向けのトランザクションデータ（シードデータ）**を格納するディレクトリです。

## 役割

開発・テスト・デモ環境で、アプリケーションの動作確認に必要な **初期データ**を投入するために使用します。

<!-- sample-api:replace-begin -->
- ユーザーデータ
- 商品データ
- その他ビジネスデータ
<!-- sample-api:replace-with -->
<!-- = - ビジネスデータ -->
<!-- sample-api:replace-end -->

など、マスタデータではなく **トランザクション系のデータ**を対象とします。

## 実行方法

```bash
make db-seed
```

または CLI から：

```bash
./server db-seed
```

## 命名規則

```text
{6桁連番}_{説明}.sql
```

例：

<!-- sample-api:replace-begin -->
```text
000001_users.sql
000002_users_additional.sql
000007_products_electronic_equipment_01.sql
```
<!-- sample-api:replace-with -->
<!-- = ```text -->
<!-- = 000001_<集約>.sql -->
<!-- = 000002_<集約>_additional.sql -->
<!-- = 000007_<集約>_<グループ>_01.sql -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

- 連番の昇順で実行される
<!-- sample-api:replace-begin -->
- 依存関係がある場合は連番で順序を制御する（例: users → products → purchases）
<!-- sample-api:replace-with -->
<!-- = - 依存関係がある場合は連番で順序を制御する（参照される側の行を、参照する側より先に投入する） -->
<!-- sample-api:replace-end -->
- 1 ファイルに収まらないまとまりは 2 桁の連番（`_01`、`_02`、…）で分割する

## プレースホルダ

seed ファイルは環境依存の値を `${NAME}` で書ける。実行時に seed ランナーが展開する。リテラルで書くと
特定の環境でしか正しくない値（例: ローカルの認証プロバイダの公開ポートは worktree のスロットでずれる）に
用いる。

<!-- sample-api:replace-begin -->
```sql
-- issuer は環境に追従する（mock 認証サーバーの公開ポートは worktree のスロットでずれる）
INSERT INTO user_identities (id, user_id, issuer, subject) VALUES
('...', '...', '${AUTH_ISSUER}', 'user-john-doe') ON CONFLICT (id) DO NOTHING;
```
<!-- sample-api:replace-with -->
<!-- = ```sql -->
<!-- = -- issuer は環境に追従する（mock 認証サーバーの公開ポートは worktree のスロットでずれる） -->
<!-- = INSERT INTO <テーブル> (id, issuer, subject) VALUES -->
<!-- = ('...', '${AUTH_ISSUER}', '<subject>') ON CONFLICT (id) DO NOTHING; -->
<!-- = ``` -->
<!-- sample-api:replace-end -->

使える名前は `db-seed` コマンド（`cmd/seed.go`）が与える。現在は設定から取る JWT の issuer
`AUTH_ISSUER` のみ。値の無い（未定義または空の）プレースホルダは空文字で埋めずエラーになり、**その
ファイルは丸ごと未実行になる**（同じファイル内のプレースホルダを使わない行も投入されない）。環境依存の
値が空のまま入ったデータは、seed は成功したのに実行時だけ失敗する状態を作り原因を辿りにくいため、
fail-closed の単位をファイルに置いている。単一引用符を含む値も同様に拒否する。展開はテキスト置換で、
文字列リテラルを抜け出せる値は独立したステートメントとして実行されうるため。

## マイグレーションとの違い

|観点|migrations|seed|
|---|---|---|
|対象|DDL（スキーマ定義）|DML（データ投入）|
|本番利用|必須|非推奨|
|ロールバック|up / down ペア|なし（データは削除 or 再投入）|
|冪等性|必須|推奨（`ON CONFLICT` 等）|

## 注意点

- **本番環境での実行は想定していない** — シードデータは開発・テスト用
<!-- sample-api:replace-begin -->
- マスタデータ（都道府県、ステータス定義等）はマイグレーションで管理する
<!-- sample-api:replace-with -->
<!-- = - マスタデータ（独立した書き込みライフサイクルを持たない固定の参照テーブル）はマイグレーションで管理する -->
<!-- sample-api:replace-end -->
- テーブル間に外部キー制約がある場合は、投入順序（連番）に注意する
- 大量データを投入する場合は、ファイルを分割して連番で管理する。1 ファイルは 20000 バイト未満に
  収める（`make sql-lint` は上限を超えたファイルを解析せずスキップするため、ファイルごと lint の
  対象から静かに外れる）
- 可能な限り冪等にする（`INSERT ... ON CONFLICT DO NOTHING` 等）
