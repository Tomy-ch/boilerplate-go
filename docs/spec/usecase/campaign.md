# Campaign — Usecase Spec

> キャンペーンの定義・列挙・停止と、コードによる受け取りのユースケース spec。
> **受け取りはキャンペーンとクーポンの 2 集約を 1 つのトランザクションで書く。**

## Overview

キャンペーンのユースケースは、admin がコード配布を定義・列挙・停止する 3 本と、利用者がコードを示して
クーポンを 1 枚受け取る 1 本からなる。

**受け取りは Repository 経由で足り、CommandService は要らない。** 書く行（キャンペーン 1 行・クーポン 1 行・
受け取り記録 1 行）はいずれも識別子で名指しでき、load-mutate-save に分解できる。
[`docs/design/data-access-pattern.md`](../../design/data-access-pattern.md) §4 Gate 1 により Repository へ載り、
CommandService へ落ちる branch のいずれにも当たらない。

**テンプレートからクーポン 1 枚を導く解決は、この usecase パッケージの私有関数が持つ。**
キャンペーンが保つ種別の名前をクーポン集約の語彙へ解決するもので、名前の解決も値の検証もドメインが行い、
私有関数は 2 つを繋ぐだけである（[`coupon.md`](coupon.md) の名指し発行と一括発行が共有する
`newDiscount` / `newScope` と同じ形）。**定義時と受け取り時が同じ 1 本を通る**ので、定義では通った
テンプレートが受け取りで落ちる窓が開かない。

**受け取りは認可を呼ばない。** 自分のためにクーポンを受け取る操作であり、admin の操作ではない。
定義・列挙・停止の 3 本は admin 限定で、資源は Campaign。

**押し出し型（販促クーポンの一括発行）とは並存する。** どちらもクーポンを生むが、受給者が事前に決まるか
どうかが違う。一括発行は [`coupon.md`](coupon.md) が持ち、キャンペーンを経由しない。

## Interface

```yaml
package: internal/usecase/campaign
interface: Usecase
methods:
  - name: DefineCampaign
    signature: DefineCampaign(ctx, authn, params DefineCampaignParams) (CampaignView, error)
  - name: ListCampaigns
    signature: ListCampaigns(ctx, authn, page *paging.Page) (CampaignListView, error)
  - name: SuspendCampaign
    signature: SuspendCampaign(ctx, authn, id uuid.UUID) (CampaignView, error)
  - name: ClaimCoupon
    signature: ClaimCoupon(ctx, authn, code string) (ClaimedCouponView, error)
```

## DTOs

```yaml
- name: DefineCampaignParams
  fields:
    - name: Code
      type: string            # 正規化はドメインが行うため、受け取った値をそのまま渡す
    - name: DiscountKind
      type: string            # "flat" / "rate"
    - name: DiscountValue
      type: decimal.Decimal
    - name: MaxAmount
      type: "*int64"          # 定率の値引き上限（USD セント）。nil なら上限なし
    - name: ScopeKind
      type: string            # "all" / "category" / "product"
    - name: ScopeTargetID
      type: "*uuid.UUID"
    - name: MinPurchaseAmount
      type: "*int64"          # 配るクーポンの最低購入金額。nil なら条件なし
    - name: UsableFrom
      type: "*time.Time"      # 配るクーポンの利用開始日時。nil なら発行時点から使える
    - name: CouponExpiresAt
      type: time.Time
    - name: StartsAt
      type: time.Time
    - name: EndsAt
      type: time.Time
    - name: TotalLimit
      type: int
    - name: PerUserLimit
      type: int

- name: CampaignView
  fields: [ID, Code, DiscountKind, DiscountValue, MaxAmount, ScopeKind, ScopeTargetID,
           MinPurchaseAmount, UsableFrom, CouponExpiresAt, StartsAt, EndsAt,
           TotalLimit, PerUserLimit, IssuedCount, SuspendedAt]

- name: CampaignListView
  fields: [Campaigns, Total]

- name: ClaimedCouponView
  fields: [ID, DiscountKind, DiscountValue, MaxAmount, ScopeKind, ScopeTargetID,
           MinPurchaseAmount, UsableFrom, ExpiresAt, UsedAt, IssuedAt]
  note: |
    保有クーポン一覧（internal/usecase/coupon の CouponView）と同じ内容を返すが、写像はこちらが持つ。
    Usecase 間の直接呼び出しは internal/usecase/README.md が禁じており、共有すると層の依存が生まれる。
```

## Dependencies

```yaml
- campaign.Repository  # Create / FindList / CountAll / LockByCode / LockByID / CountClaims / RecordClaim / UpdateSuspended
- coupon.Repository    # Create（受け取りが生むクーポン 1 枚）
- clock.Clock          # 定義・受け取り・停止の日時、および配布期間の判定
- tx.Manager           # 受け取りと停止のトランザクション境界
- authz.Authorizer     # ActionCampaignDefine / ActionCampaignList / ActionCampaignSuspend
```

## Workflow

```yaml
- name: DefineCampaign
  tx_required: false
  calls:
    - authz.Authorizer.Authorize   # ActionCampaignDefine（admin のみ）
    - campaign.NewCode
    - campaign.NewTemplate
    - resolveTemplate              # 私有関数。定義時にテンプレートを解決してみる
    - campaign.New
    - campaign.Repository.Create
  behavior: |
    admin がコード配布を 1 件定義し、定義したキャンペーンを返す。定義直後は 1 枚も配っておらず、
    停止していない。

    **テンプレートが実際にクーポンへ解決できるかを定義時に確かめる。** ここで確かめないと、
    定義は通ったのに誰も受け取れないキャンペーンが作れてしまう。解決には受け取りと同じ
    ドメインサービスを使う。

    コードは正規化して保存する（前後の空白を落とし大文字へ揃える）。書くのは campaigns の 1 行だけで、
    トランザクションを開かない。
  invariants:
    - 配るクーポンの有効期限は配布期間の終了より後（campaign.New が課す）
    - 1 人あたり上限は総枚数上限以下（同上）
    - 既知でない種別の名前は定義時に弾かれ、永続化まで到達しない
  errors:
    - 未認証: 401
    - 非 admin: 403
    - コードの形式が不正 / 期間や上限が不正 / 種別が既知でない: 422
    - コードが既に使われている: 409

- name: ListCampaigns
  tx_required: false
  calls:
    - authz.Authorizer.Authorize   # ActionCampaignList（admin のみ）
    - campaign.Repository.FindList
    - campaign.Repository.CountAll
  behavior: |
    キャンペーンを配布期間の開始が新しい順で返す。停止済みと配布期間を過ぎたものも含む。
    残枚数は totalLimit と issuedCount の差から読める。
  errors:
    - 未認証: 401
    - 非 admin: 403
    - ページ指定が不正: 422

- name: SuspendCampaign
  tx_required: true
  calls:
    - authz.Authorizer.Authorize   # ActionCampaignSuspend（admin のみ）
    - clock.Clock.Now
    - campaign.Repository.LockByID
    - campaign.Campaign.Suspend
    - campaign.Repository.UpdateSuspended
  behavior: |
    キャンペーンの受け取りを打ち切る。配布期間が残っていても、以後そのコードでは受け取れなくなる。
    配布事故を止める口がこれである。

    行ロックを取ってから停止の可否をドメインへ判定させる。**既に配ったクーポンは無効にならない。**
  invariants:
    - 停止は取り消せない一方向の遷移
    - lock_order は campaigns のみ（他のテーブルを触らないため順序の制約を生まない）
  errors:
    - 未認証: 401
    - 非 admin: 403
    - 存在しない: 404
    - 既に停止済み: 409

- name: ClaimCoupon
  tx_required: true
  calls:
    - campaign.NewCode
    - clock.Clock.Now
    - campaign.Repository.LockByCode      # ① 条件の評価より前にロックを取る
    - campaign.Repository.CountClaims     # ② ロック下でその利用者の受け取り枚数を数える
    - campaign.Campaign.Claim             # ③ 期間・停止・2 つの上限をドメインが判定し、枚数を増やして受け取り記録を返す
    - couponAttributesFor                 # ④ テンプレートから配るクーポン 1 枚の属性を組み立てる
    - coupon.New                          # ⑤
    - coupon.Repository.Create            # ⑥
    - campaign.Repository.RecordClaim      # ⑦ ③ が返した記録を書く。加算と記録の挿入は 1 呼び出しで
  behavior: |
    認証主体がコードを示してクーポンを 1 枚受け取る。受給者は認証主体自身で、受け取ったクーポンを返す。

    **認可を呼ばない。** 自分のためにクーポンを受け取る操作であり、admin の操作ではない。

    **ロックは条件の評価より前に取る。** 総枚数上限も 1 人あたり上限も「読んで判断して書く」形であり、
    判定のあとにロックを取っても直列化されない（ADR-0036 の決定 2）。1 人あたり上限の判定に要る枚数は
    ロックを取ったあとで数えるため、先行する受け取りが挿入した記録が見える。

    **在籍の確認に利用者行はロックしない。** 発行済みクーポンは害を生まないため、IssueCoupon と同じ扱い
    （ADR-0034 の分岐 1）。したがって受け取りが押さえるのは campaigns の 1 行だけである。
  invariants:
    - 総枚数上限と 1 人あたり上限は、同一コードへの並行受け取りの下でも超えない。
      証明は internal/infrastructure/rdb/repository/campaign の実 DB レーステスト 2 本
    - 配るクーポンの内容は受け取った時点でクーポンへ焼く。以後キャンペーンの定義が変わっても
      既に配ったクーポンは動かない
    - 発行済み枚数は減らない。購入のキャンセルでクーポンが未使用へ戻っても配った事実は取り消されない
    - lock_order は campaigns のみ。購入（users → coupons → products）と交差するテーブルを
      押さえないため、既存の順序は動かない
  errors:
    - 未認証: 401
    - 受け取れない（存在しない / 配布期間の外 / 停止済み / 総枚数上限 / 1 人あたり上限）:
        422 + details ["code"]
    - 受け取り記録の保存が競合した: 409（二重防御。正しい呼び出し順では到達しない）
```

## 受け取れない理由を区別しない

**5 つの拒否理由はすべて `422` + `details: ["code"]` に畳む。** 存在しないコードだけを 404 にすると、
応答の違いが「そのコードは存在する」という情報になり、有効なコードを言い当てる手がかりになる。
形式が不正なコードも同じ応答にするのは、形式の当たりだけを先に絞り込めないようにするためである。

畳んでも失うものが少ないのは、**次にすべきことがどの場合も同じ**（別のコードを使うか諦める）だからで、
これは購入時のクーポン適用が採った方針（[`purchase.md`](purchase.md) の 422 族）と同じである。

**秘匿するのは応答であって、ログとメトリクスではない。** ドメインは 5 つの理由を別々のエラーとして返し、
畳むのは HTTP 境界の 1 箇所だけである。運営が「なぜ受け取れなかったか」を追えなくなると、
配布事故の調査ができなくなる。

**応答時間は秘匿の対象に含めない。** 定数時間化の先例がリポジトリに無く、この op だけに入れると
他の op との一貫性を欠く。総当たり自体の抑止はエッジへ委譲している
（[ADR-0108](../../adr/0108-no-in-app-rate-limiter.md)）。

## Command Service

なし。理由は [Overview](#overview) を参照。

## Query Service

なし。一覧は Repository で足り、ドメインロジックを SQL へ持ち出す必要がない。

## Notes

**なぜ「受け取り」と呼ぶか。**「引き換え」は [`glossary.md`](../glossary.md) が既に別の行為へ割り当てている。
詳細は [`docs/spec/domain/campaign.md`](../domain/campaign.md) の Notes を参照。

**なぜ Idempotency-Key が必須か。** 受け取りは取り消せず、二度目の実行はもう 1 枚配る。
再送を安全にする手段が鍵しかないため必須とする（名指しの発行と同じ扱い）。
別の鍵で押し直した場合は、上限に達していなければ改めて 1 枚配られる。
