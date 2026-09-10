package campaign

import (
	"time"

	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// Claim は、受け取りが起きた事実を表すドメインエンティティです。誰がどのキャンペーンから
// いつ受け取り、その結果どのクーポンが生まれたかを 1 件で表します。
//
// 1 人あたり上限はこの記録を数えて判定します。発行されたクーポンの側に出自を持たせないのは、
// クーポン集約が発行事由を根拠に入れないと宣言しているためです
// （docs/spec/domain/coupon.md の Overview）。
type Claim struct {
	id         uuid.UUID
	campaignID uuid.UUID
	userID     uuid.UUID
	couponID   uuid.UUID
	claimedAt  time.Time
}

// ClaimAttributes は、受け取り記録の属性一式です。同型の識別子が 3 つ並ぶため構造体で受けます
// （基準は docs/rules.md の Function Signature Rules）。
type ClaimAttributes struct {
	// CampaignID は、受け取り元のキャンペーンです。
	CampaignID uuid.UUID
	// UserID は、受け取った利用者です。
	UserID uuid.UUID
	// CouponID は、受け取りで発行されたクーポンです。
	CouponID uuid.UUID
	// ClaimedAt は、受け取り日時です。
	ClaimedAt time.Time
}

// newClaim は、受け取り記録の検証と生成を行います。
// いずれかの識別子が未設定、または受け取り日時がゼロ値の場合は検証エラーを返します。
//
// 非公開である理由は [Campaign.Claim] を参照。
func newClaim(id uuid.UUID, attrs ClaimAttributes) (*Claim, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if attrs.CampaignID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidCampaignID, "campaignID is required")
	}
	if attrs.UserID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidUserID, "userID is required")
	}
	if attrs.CouponID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidCouponID, "couponID is required")
	}
	if attrs.ClaimedAt.IsZero() {
		return nil, xerrors.Wrap(ErrInvalidClaimedAt, "claimedAt is required")
	}

	return &Claim{
		id:         id,
		campaignID: attrs.CampaignID,
		userID:     attrs.UserID,
		couponID:   attrs.CouponID,
		claimedAt:  attrs.ClaimedAt,
	}, nil
}

// ID は、受け取り記録 ID を返します。
func (c *Claim) ID() uuid.UUID { return c.id }

// CampaignID は、受け取り元のキャンペーン ID を返します。
func (c *Claim) CampaignID() uuid.UUID { return c.campaignID }

// UserID は、受け取った利用者のユーザー ID を返します。
func (c *Claim) UserID() uuid.UUID { return c.userID }

// CouponID は、受け取りで発行されたクーポンの ID を返します。
func (c *Claim) CouponID() uuid.UUID { return c.couponID }

// ClaimedAt は、受け取り日時を返します。
func (c *Claim) ClaimedAt() time.Time { return c.claimedAt }
