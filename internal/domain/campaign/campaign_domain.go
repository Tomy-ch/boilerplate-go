// Package campaign は、キャンペーンドメインを定義します。コードを配り、受け取った利用者へ
// クーポンを 1 枚ずつ配る取り組みを表す Campaign エンティティを提供します。
//
// キャンペーンが持つのは配布の定義と制約だけです。配られた 1 枚はクーポン集約が表し、
// キャンペーンはそれを保持しません。受け取りが「クーポンを 1 枚発行させる」形になるのは
// そのためです（購入への適用を指す「引き換え」とは別の行為です）。
//
// 配るクーポンの内容は [Template] が持ちます。値引きと適用範囲の種別を解決するのはクーポン集約であり、
// その橋渡しは usecase 層が行います（docs/spec/usecase/campaign.md）。
package campaign

import (
	"time"

	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// Campaigns は、Campaign エンティティのスライス型です。
type Campaigns []*Campaign

// Campaign は、キャンペーンを表すドメインエンティティです。
//
// 発行済み枚数は増える一方で、減ることはありません（docs/spec/domain/campaign.md の Overview）。
type Campaign struct {
	id           uuid.UUID
	code         Code
	template     Template
	startsAt     time.Time
	endsAt       time.Time
	totalLimit   int
	perUserLimit int
	issuedCount  int
	suspendedAt  *time.Time
}

// Attributes は、キャンペーンの属性一式です。同型の日時と上限が並ぶため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type Attributes struct {
	// Code は、利用者が示す符号です。
	Code Code
	// Template は、配るクーポン 1 枚の内容です。
	Template Template
	// StartsAt は、配布期間の開始日時です。
	StartsAt time.Time
	// EndsAt は、配布期間の終了日時です。
	EndsAt time.Time
	// TotalLimit は、総枚数上限です。
	TotalLimit int
	// PerUserLimit は、1 人あたり上限です。
	PerUserLimit int
}

// ClaimParams は、受け取り 1 件を受け付けるための入力です。同型の識別子が並ぶため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type ClaimParams struct {
	// ClaimedAt は、受け取り日時です。
	ClaimedAt time.Time
	// ClaimedByUser は、その利用者が既にこのキャンペーンから受け取った枚数です。
	// 呼び出し元が数えて渡します（ドメインは I/O を持たないため）。
	ClaimedByUser int
	// ClaimID は、生まれる受け取り記録の ID です。
	ClaimID uuid.UUID
	// UserID は、受け取る利用者です。
	UserID uuid.UUID
	// CouponID は、この受け取りで発行されたクーポンです。
	CouponID uuid.UUID
}

// New は、キャンペーンエンティティの検証と生成を行います。生成直後は 1 枚も配っておらず、停止していません。
//
// id・Code・Template が未設定、上限が 0 以下、配布期間が逆順の場合は検証エラーを返します。
// 配るクーポンの有効期限は配布期間の終了より後である必要があります
// （理由は docs/spec/domain/campaign.md の Cross-field Invariants を参照）。
func New(id uuid.UUID, attrs Attributes) (*Campaign, error) {
	return newCampaign(id, attrs, 0, nil)
}

// Reconstruct は、永続化済みのキャンペーンを再構築します。issuedCount は発行済み枚数、
// suspendedAt は停止済みなら停止日時、停止していなければ nil です。その他の検証は New と同一です。
func Reconstruct(id uuid.UUID, attrs Attributes, issuedCount int, suspendedAt *time.Time) (*Campaign, error) {
	return newCampaign(id, attrs, issuedCount, suspendedAt)
}

// newCampaign は、生成・再構築に共通の検証を行いキャンペーンエンティティを構築します。
func newCampaign(id uuid.UUID, attrs Attributes, issuedCount int, suspendedAt *time.Time) (*Campaign, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if attrs.Code.IsZero() {
		return nil, xerrors.Wrap(ErrInvalidCode, "code is required")
	}
	if attrs.Template.IsZero() {
		return nil, ErrInvalidTemplate
	}
	if err := validatePeriod(attrs); err != nil {
		return nil, err
	}
	if err := validateLimits(attrs, issuedCount); err != nil {
		return nil, err
	}

	return &Campaign{
		id:           id,
		code:         attrs.Code,
		template:     attrs.Template,
		startsAt:     attrs.StartsAt,
		endsAt:       attrs.EndsAt,
		totalLimit:   attrs.TotalLimit,
		perUserLimit: attrs.PerUserLimit,
		issuedCount:  issuedCount,
		suspendedAt:  ptr.Copy(suspendedAt),
	}, nil
}

// validatePeriod は、配布期間と、配るクーポンの有効期限との関係を検証します。
func validatePeriod(attrs Attributes) error {
	if attrs.StartsAt.IsZero() {
		return xerrors.Wrap(ErrInvalidPeriod, "startsAt is required")
	}
	if attrs.EndsAt.IsZero() {
		return xerrors.Wrap(ErrInvalidPeriod, "endsAt is required")
	}
	if !attrs.EndsAt.After(attrs.StartsAt) {
		return xerrors.Wrap(ErrInvalidPeriod, "endsAt must be after startsAt")
	}
	if !attrs.Template.ExpiresAt().After(attrs.EndsAt) {
		return xerrors.Wrap(ErrInvalidCouponExpiresAt, "coupon expiresAt must be after endsAt")
	}

	return nil
}

// validateLimits は、上限と発行済み枚数を検証します。
func validateLimits(attrs Attributes, issuedCount int) error {
	if attrs.TotalLimit <= 0 {
		return xerrors.Wrap(ErrInvalidTotalLimit, "totalLimit must be positive")
	}
	if attrs.PerUserLimit <= 0 {
		return xerrors.Wrap(ErrInvalidPerUserLimit, "perUserLimit must be positive")
	}
	if attrs.PerUserLimit > attrs.TotalLimit {
		return xerrors.Wrap(ErrInvalidPerUserLimit, "perUserLimit must not exceed totalLimit")
	}
	if issuedCount < 0 {
		return xerrors.Wrap(ErrInvalidIssuedCount, "issuedCount must not be negative")
	}
	if issuedCount > attrs.TotalLimit {
		return xerrors.Wrap(ErrInvalidIssuedCount, "issuedCount must not exceed totalLimit")
	}

	return nil
}

// ID は、キャンペーン ID を返します。
func (c *Campaign) ID() uuid.UUID { return c.id }

// Code は、キャンペーンコードを返します。
func (c *Campaign) Code() Code { return c.code }

// Template は、配るクーポン 1 枚の内容を返します。
func (c *Campaign) Template() Template { return c.template }

// StartsAt は、配布期間の開始日時を返します。
func (c *Campaign) StartsAt() time.Time { return c.startsAt }

// EndsAt は、配布期間の終了日時を返します。
func (c *Campaign) EndsAt() time.Time { return c.endsAt }

// TotalLimit は、総枚数上限を返します。
func (c *Campaign) TotalLimit() int { return c.totalLimit }

// PerUserLimit は、1 人あたり上限を返します。
func (c *Campaign) PerUserLimit() int { return c.perUserLimit }

// IssuedCount は、発行済み枚数を返します。
func (c *Campaign) IssuedCount() int { return c.issuedCount }

// SuspendedAt は、停止日時を返します。停止していない場合は nil です。
func (c *Campaign) SuspendedAt() *time.Time { return ptr.Copy(c.suspendedAt) }

// IsSuspended は、キャンペーンが停止済みかどうかを返します。
func (c *Campaign) IsSuspended() bool { return c.suspendedAt != nil }

// IsDistributing は、渡された時点でキャンペーンが受け取りを認めるかどうかを返します。
//
// 開始日時ちょうどは認める側に、終了日時ちょうどは認めない側に含めます（有効期限の判定と同じ向きです）。
// 停止済み、または総枚数上限に達している場合も認めません。
func (c *Campaign) IsDistributing(now time.Time) bool {
	return !c.IsSuspended() &&
		!now.Before(c.startsAt) && now.Before(c.endsAt) &&
		c.issuedCount < c.totalLimit
}

// Claim は、受け取りを 1 件受け付け、発行済み枚数を 1 増やし、受け取りの事実を返します。
//
// 停止済みなら ErrSuspended、配布期間の外なら ErrNotDistributing、総枚数上限に達していれば
// ErrTotalLimitReached、その利用者が 1 人あたり上限に達していれば ErrPerUserLimitReached を返し、
// いずれも状態を変えません。受け取り枚数が負の場合は ErrInvalidClaimedByUser を返します。
//
// **受け取りの事実（[Claim]）はこのメソッドだけが生みます。** 遷移に成功したときだけ記録を返すため、
// 記録を得た呼び出し元は必ず遷移を通っています（理由は internal/domain/README.md の
// Aggregate consistency と docs/spec/domain/campaign.md の Claim を参照）。
//
// 呼び出す前にキャンペーン行の排他ロックを取ること。ここが守る上限は「読んで判断して書く」形であり、
// ロックを条件の評価より後に取ると直列化されません（ADR-0036 (ordered-pessimistic-row-locks) の決定 2）。
func (c *Campaign) Claim(params ClaimParams) (*Claim, error) {
	if params.ClaimedByUser < 0 {
		return nil, xerrors.Wrap(ErrInvalidClaimedByUser, "claimedByUser must not be negative")
	}
	if c.IsSuspended() {
		return nil, ErrSuspended
	}
	if params.ClaimedAt.Before(c.startsAt) || !params.ClaimedAt.Before(c.endsAt) {
		return nil, ErrNotDistributing
	}
	if c.issuedCount >= c.totalLimit {
		return nil, ErrTotalLimitReached
	}
	if params.ClaimedByUser >= c.perUserLimit {
		return nil, ErrPerUserLimitReached
	}

	record, err := newClaim(params.ClaimID, ClaimAttributes{
		CampaignID: c.id,
		UserID:     params.UserID,
		CouponID:   params.CouponID,
		ClaimedAt:  params.ClaimedAt,
	})
	if err != nil {
		return nil, err
	}

	c.issuedCount++

	return record, nil
}

// Suspend は、キャンペーンを停止します。停止は取り消せません。
//
// 既に停止済みなら ErrAlreadySuspended を返し、状態を変えません。
func (c *Campaign) Suspend(now time.Time) error {
	if c.IsSuspended() {
		return ErrAlreadySuspended
	}

	c.suspendedAt = &now

	return nil
}
