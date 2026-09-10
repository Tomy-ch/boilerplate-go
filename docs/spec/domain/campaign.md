# Campaign — Domain Spec

> キャンペーンのドメイン spec。**コードを配り、受け取った利用者へクーポンを 1 枚ずつ配る取り組み**を表す。
> 配られた 1 枚はクーポン集約が表し、キャンペーンはそれを保持しない。

## Overview

キャンペーン集約（Campaign）は、コード・配るクーポンのテンプレート・配布期間・上限・発行済み枚数・停止状態を
保持するドメインエンティティ。**キャンペーンが持つのは配布の定義と制約だけである。**

**これはクーポンの配布の「引き」側にあたる。** 記名配布（押し出し型）は運営が受給者を決めて配るのに対し、
コード配布（引き取り型）は誰が受け取るかが事前に決まらない。前者は
[`docs/spec/usecase/coupon.md`](../usecase/coupon.md) が持ち、両者はクーポン集約を共有するが経路は別である。

**クーポン集約は変えない。** 受け取りが起きると、キャンペーンのテンプレートから
[`coupon.New`](coupon.md) がクーポンを 1 枚生む。廃番が `IssueDiscontinuationCoupons` を通してクーポンを
生むのと同じ形で、「別のものがトリガになってクーポンが生まれる」系統に収まる。

**配るクーポンの内容は受け取った時点でクーポンへ焼く。** 以後キャンペーンの定義が変わっても、既に配った
クーポンは動かない。`purchases` が適用済みクーポンの値引き・適用範囲を JOIN で解決できるのは
「発行済みのクーポンは書き換える口を持たない」からであり（[`docs/spec/usecase/purchase.md`](../usecase/purchase.md)）、
キャンペーンを参照する形にするとこの前提が崩れる。

**発行済み枚数は減らない。** 購入のキャンセルでクーポンが未使用へ戻っても、配った事実は取り消されない。
総枚数上限が数えるのは配った枚数であって、使われた枚数ではない。返却でキャンペーンの枚数を戻すと、
返却の経路がキャンペーン行のロック順序に入ることにもなる。

**種別の閉じた集合を所有するのはクーポン集約である。** キャンペーンはテンプレートの中に種別の**名前**を保つ
だけで、その名前が既知かどうかを判定しない。判定は usecase 層が行い、キャンペーンの定義時と受け取り時の
双方が同じ 1 本を通る（[`docs/spec/usecase/campaign.md`](../usecase/campaign.md)）。名前ではなく
クーポンの値オブジェクトを直接持たせない理由は [Notes](#notes) を参照。

`id` は UUIDv7（[ADR-0037](../../adr/0037-uuidv7-identifiers.md)）で、生成は usecase 層が行いドメインへ渡す。
配布期間も時刻境界から供給された値を受け取る。

## Entity

```yaml
package: internal/domain/campaign
struct: Campaign
constructors:
  - name: New          # 定義時。生成直後は 1 枚も配っておらず停止していない
  - name: Reconstruct  # 永続化済みの再構築（issuedCount / suspendedAt を受け取る）
fields:
  - name: id
    type: uuid.UUID
    required: true          # IsNil の場合は ErrInvalidID
  - name: code
    type: Code
    required: true          # ゼロ値の場合は ErrInvalidCode
  - name: template
    type: Template
    required: true          # ゼロ値の場合は ErrInvalidTemplate
  - name: startsAt
    type: time.Time
    required: true          # ゼロ値は ErrInvalidPeriod
  - name: endsAt
    type: time.Time
    required: true          # ゼロ値は ErrInvalidPeriod
  - name: totalLimit
    type: int
    required: true          # 0 以下は ErrInvalidTotalLimit
  - name: perUserLimit
    type: int
    required: true          # 0 以下・totalLimit 超過は ErrInvalidPerUserLimit
  - name: issuedCount
    type: int               # 生成時は 0。Claim だけが増やし、減る経路は無い
  - name: suspendedAt
    type: "*time.Time"      # nil 許容（停止していない）。Suspend が一度だけ設定する
```

```yaml
package: internal/domain/campaign
struct: Claim
constructors:
  - name: newClaim     # 非公開。受け取りの事実を生む口は Campaign.Claim ただ 1 つ
fields:
  - name: id
    type: uuid.UUID
    required: true          # IsNil の場合は ErrInvalidID
  - name: campaignID
    type: uuid.UUID
    required: true          # 受け取り元。識別子だけで参照する
  - name: userID
    type: uuid.UUID
    required: true          # 受け取った利用者
  - name: couponID
    type: uuid.UUID
    required: true          # 受け取りで生まれたクーポン。集約をまたぐが識別子だけを持つ
  - name: claimedAt
    type: time.Time
    required: true
```

## Cross-field Invariants

- `endsAt > startsAt`（違反は `ErrInvalidPeriod`）。長さを持たない配布期間は配布を表さないため、
  同時刻も許さない。
- `template.expiresAt > endsAt`（違反は `ErrInvalidCouponExpiresAt`）。**期間の終わりぎわに受け取った
  利用者へ、その時点で既に失効しているクーポンを渡さないため。** 同時刻も許さない。
- `perUserLimit <= totalLimit`（違反は `ErrInvalidPerUserLimit`）。1 人で総枚数を超えて受け取れる
  という宣言は、上限の組として矛盾している。
- `0 <= issuedCount <= totalLimit`（違反は `ErrInvalidIssuedCount`）。再構築でのみ課す。

**上限は 2 つとも必須である。** 「無制限」を nil で表さないのは、上限の無いコード配布が実運用で
成立しないためで、無制限にしたい場合は十分大きな値を宣言する。宣言が要ることそのものが制約である。

## Behavior Methods

```yaml
- name: IsSuspended
  signature: IsSuspended() bool
  behavior: |
    停止済みかどうかを返す。停止日時が設定されていることを指す。
- name: IsDistributing
  signature: IsDistributing(now time.Time) bool
  behavior: |
    渡された時点で受け取りを認めるかを返す。開始日時ちょうどは認める側、終了日時ちょうどは認めない側に
    含める（区間を [startsAt, endsAt) として閉じる。クーポンの有効期限と同じ向き）。
    停止済み、または総枚数上限に達している場合も認めない。時刻はドメインの外から渡す。

    読み取り専用の問いであり、受け取りの可否はこれではなく Claim が決める。理由を区別して返す必要が
    あるためで、bool へ畳んだ結果からは理由を復元できない。
- name: Claim
  signature: Claim(params ClaimParams) (*Claim, error)
  behavior: |
    受け取りを 1 件受け付け、発行済み枚数を 1 増やし、**受け取りの事実（Claim）を返す**。
    ClaimParams.ClaimedByUser は、その利用者が既にこのキャンペーンから受け取った枚数
    （呼び出し元が数えて渡す。ドメインは I/O を持たないため）。負の値は ErrInvalidClaimedByUser。

    停止済みなら ErrSuspended、配布期間の外なら ErrNotDistributing、総枚数上限に達していれば
    ErrTotalLimitReached、その利用者が 1 人あたり上限に達していれば ErrPerUserLimitReached を返し、
    いずれも状態を変えない。**判定の順はキャンペーン全体の理由が先、利用者個別の理由が後。**
    どちらも成り立つ場合、運営が知りたいのは全体の方だからである。

    **受け取りの事実を生む口はこのメソッドただ 1 つで、Claim の構築子は非公開である。**
    遷移に成功したときだけ記録を返すため、記録を得た呼び出し元は必ず遷移を通っており、
    記録とキャンペーンの状態が食い違わない。集約への変更はルートを通す
    （internal/domain/README.md の Aggregate consistency）という規約の帰結であり、
    purchase の遷移メソッドが Event を返すのと同じ形である。

    **呼び出す前にキャンペーン行の排他ロックを取ること。** ここが守る上限は「読んで判断して書く」形であり、
    ロックを条件の評価より後に取ると直列化されない（[ADR-0036](../../adr/0036-ordered-pessimistic-row-locks.md)
    の決定 2）。1 人あたり上限の判定に要る枚数を引数で受けるのは、ドメインが I/O を持たないためである。
- name: Suspend
  signature: Suspend(now time.Time) error
  behavior: |
    受け取りを打ち切る。**取り消せない一方向の遷移**で、既に停止済みなら ErrAlreadySuspended（409）を
    返し状態を変えない。要求の不正ではなく状態の衝突として扱うのは、同じ要求が時間の経過で
    成立しうるものではないからである。

    **既に配ったクーポンは無効にならない。** 停止が止めるのは配布であって、配った 1 枚ではない。
    発行済みのクーポンを失効させる経路はこの集約にもクーポン集約にも無い。
    日時は引数で受け取る（ドメインは時刻へ直接依存しない）。
```

## Value Objects

```yaml
- name: Code
  underlying_type: struct    # value string
  validation: |
    前後の空白を落とし大文字へ揃えたうえで、8〜32 文字・大文字英数字とハイフンのみを許す。
    違反は ErrInvalidCode。

    **同一視は生成時の正規化で済ませ、永続化にも比較にも正規化済みの値だけが出る。** 人が読み上げ、
    打ち込み、メールから貼る値なので大文字・小文字の違いは同じコードとして扱うが、その判断を
    照合順序や関数索引へ預けない（業務規則が永続化層へ移ると、規則の所在が読めなくなる）。

    最短の長さは、総当たりの抑止をアプリケーション内に持たないと決めている
    （[ADR-0108](../../adr/0108-no-in-app-rate-limiter.md)）ことの帰結である。コードは推測されにくさを
    自分の長さで持つ必要がある一方、人が転記できる範囲に留める必要もあり、下限と上限はその折り合い。
  factory: NewCode
  methods:
    - name: Value
      returns: string
    - name: IsZero
      returns: bool

- name: Template
  underlying_type: struct    # discountKindName / discountValue / discountMaxAmount / scopeKindName /
                             # scopeTargetID / minPurchaseAmount / usableFrom / expiresAt
  validation: |
    種別の名前が空でないこと、値引きの値が正であること、任意の金額が正であること、有効期限がゼロ値で
    ないこと、利用開始日時が有効期限より前であることを課す。

    **種別の名前が既知かどうかはここでは判定しない。** 閉じた集合を所有するのはクーポン集約であり、
    キャンペーンは名前を保つだけである。既知でない名前は usecase 層の解決がクーポン集約の検証エラーと
    して弾く。定義時にもその 1 本を通すため、誰も受け取れないキャンペーンが作られることはない。
  factory: NewTemplate
  methods:
    - name: DiscountKindName
      returns: string
    - name: DiscountValue
      returns: decimal.Decimal
    - name: DiscountMaxAmount
      returns: "*int64"
    - name: ScopeKindName
      returns: string
    - name: ScopeTargetID
      returns: "*uuid.UUID"
    - name: MinPurchaseAmount
      returns: "*int64"
    - name: UsableFrom
      returns: "*time.Time"
    - name: ExpiresAt
      returns: time.Time
    - name: IsZero
      returns: bool
```

## Repository Methods

```yaml
- name: Create
  signature: Create(ctx context.Context, c *Campaign) error
  behavior: |
    定義したキャンペーンを 1 件永続化する。呼び出し元のトランザクションがあればそれに参加する。
    コードが既に使われている場合は Conflict（一意制約違反の正規化）。
- name: FindList
  signature: FindList(ctx context.Context, params ListParams) (Campaigns, error)
  behavior: |
    配布期間の開始が新しい順で返す。停止済みも配布期間を過ぎたものも含む。
- name: CountAll
  signature: CountAll(ctx context.Context) (int, error)
  behavior: |
    キャンペーンの総件数を返す。ページングが引く。
- name: LockByCode
  signature: LockByCode(ctx context.Context, code Code) (*Campaign, error)
  behavior: |
    受け取りのために正規化済みコードから 1 件取得し、悲観ロック（FOR UPDATE）を取る。
    存在しない場合は NotFound。同一キャンペーンへの並行更新は、先行する更新が終わるまで待機したうえで
    最新の状態を取得する。

    **配布期間・停止・上限では絞らない。** 守るべき条件を判定するのはドメインであり、SQL に条件を置くと
    業務条件の著作権が永続化側へ移るうえ、「不在」と「対象外」が同じ 0 行に潰れて区別できなくなる
    （ADR-0036 の決定 5）。
- name: LockByID
  signature: LockByID(ctx context.Context, id uuid.UUID) (*Campaign, error)
  behavior: |
    停止のために ID から 1 件取得し、悲観ロックを取る。絞り込みを置かない理由は LockByCode と同じ。
- name: CountClaims
  signature: CountClaims(ctx context.Context, params ClaimCountParams) (int, error)
  behavior: |
    指定利用者が指定キャンペーンから既に受け取った枚数を返す。
    **キャンペーン行のロックを取ったあとで呼ぶこと**（ADR-0036 の決定 2）。
- name: RecordClaim
  signature: RecordClaim(ctx context.Context, claim *Claim) error
  behavior: |
    受け取りを 1 件記録する。発行済み枚数の加算と受け取り記録の挿入を 1 つの呼び出しで行い、
    片方だけが残る状態を作らない。

    加算は issued_count < total_limit を条件とし、0 行を ErrIssuedConcurrently（409）へ写す。
    これは行ロックを取らずに呼ばれた場合に備える二重防御であり、正しい呼び出し順では到達しない。
    0 行を NotFound へ正規化しないのは、行が無いのではなく上限が埋まったことを表すためである。
- name: UpdateSuspended
  signature: UpdateSuspended(ctx context.Context, id uuid.UUID, suspendedAt time.Time) error
  behavior: |
    停止済みにする。対象は LockByID で取得し Suspend が検証済み。
    suspended_at IS NULL を条件とし、0 行を ErrAlreadySuspended（409）へ写す（同じく二重防御）。
```

## Notes

**なぜテンプレートがクーポンの値オブジェクトを直接持たないか。** 集約は他の集約を import できず
（`internal/architest` の `TestDomainAggregateImportIsolation`）、`coupon.Discount` / `coupon.Scope` を
キャンペーンのフィールドに置けない。値オブジェクトを
[`internal/domain/lexicon`](../../../internal/domain/lexicon) へ昇格させる道はあるが、
`Scope.Covers(Line)` の連鎖で `Line` まで動くうえ、両集約の共同所有と言えるかの判断が別に要る。
本 spec では**検証済みプリミティブを保ち、解決を usecase 層へ置く**方を採った。
代償は「既知でない種別の名前をキャンペーン単体では弾けない」ことで、定義時にも同じ解決を
通すことでその窓を塞いでいる。

解決をドメインサービス（`internal/domain/service/**`）へ置く案も検討したが退けた。既存の
ドメインサービスはいずれも複数集約のロード済み状態を評価する**述語**であるのに対し、この解決は
別集約の値を**組み立てる**操作であり形が違う。加えて
[`internal/usecase/coupon`](../usecase/coupon.md) の一括発行と名指し発行が、同型の解決を
usecase の私有関数で共有する先例を既に持つ。同じ操作を 2 通りで書かないことを優先した。

**なぜ発行されたクーポンが出自を持たないか。** クーポン集約は「発行事由は根拠に入らない」と宣言しており
（[`coupon.md`](coupon.md) の Overview）、`coupons` に `campaign_id` を足すとその宣言に反する。
1 人あたり上限は受け取り記録（`Claim`）を数えて判定するため、クーポン側に出自は要らない。

**なぜ「受け取り」と呼ぶか。**「引き換え」は
[`docs/spec/glossary.md`](../glossary.md) が既に別の行為 —— 保有するクーポンを購入へ適用し使用済みにする
—— へ割り当てている。同じ語を当てると 1 語 2 義になり、spec・エラーコード・エンドポイント名・
authz Action・テスト名の 5 箇所で同時に衝突する。
