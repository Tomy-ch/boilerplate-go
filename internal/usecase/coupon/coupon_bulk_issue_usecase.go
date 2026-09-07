package coupon

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/internal/usecase/coupon/command"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// maxPromotionRecipients は、1 回の一括発行で配れる上限です。
//
// これは application policy であってドメイン不変条件ではありません。クーポン集約は「何枚まで配ってよいか」
// を知らず、知る必要もありません。上限が要るのは、誤った条件での配布が取り消せないためです。
// 値は placeholder で、実要件が立った時点で改めます。
const maxPromotionRecipients int64 = 10_000

// maxPromotionFlatDiscount は、1 回の一括発行で配れる定額値引きの上限です。
//
// これも application policy です。定率は 1 を超えられないという上限をドメインが持ちますが（値引きが
// 対象額を超えるのは値引きではないため）、定額の上限は業務が決める額であってクーポン集約の不変条件では
// ありません。上限が要るのは、この操作が取り消せず、誤った桁を 1 つ多く打つだけで全利用者へ届くためです。
// 値は placeholder で、実要件が立った時点で改めます。
var maxPromotionFlatDiscount = decimal.FromInt(100_000)

// ErrFlatDiscountTooLarge は、定額値引きが maxPromotionFlatDiscount を超えたことを表します。
var ErrFlatDiscountTooLarge = xerrors.Wrap(apperror.ErrValidation, "coupon: flat discount exceeds the bulk issuance cap")

// ErrTooManyRecipients は、受給対象が maxPromotionRecipients を超えたことを表します。
// 要求の形ではなく母集団の状態が原因なので 409 へ写します。
var ErrTooManyRecipients = xerrors.Wrap(apperror.ErrConflict, "coupon: too many recipients for a bulk issuance")

// IssuePromotionalCouponsParams は、販促クーポンを一括発行する要求の入力です。
type IssuePromotionalCouponsParams struct {
	// DiscountKind は、値引きの決まり方の名前です（"flat" / "rate"）。
	DiscountKind string
	// DiscountValue は、種別における値です。定額なら金額、定率なら率です。
	DiscountValue decimal.Decimal
	// ScopeKind は、適用範囲の決まり方の名前です（"all" / "category" / "product"）。
	ScopeKind string
	// ScopeTargetID は、適用範囲が絞る対象の識別子です。全体では nil です。
	ScopeTargetID *uuid.UUID
	// ExpiresAt は、発行するクーポンの有効期限です。キャンペーンの終了日を名指しするため絶対時刻で受けます。
	ExpiresAt time.Time
}

// IssuePromotionalCouponsView は、一括発行の実行結果です。
type IssuePromotionalCouponsView struct {
	// IssuedAt は、発行が確定した日時です。
	IssuedAt time.Time
	// ExpiresAt は、発行したクーポンの有効期限です。
	ExpiresAt time.Time
	// RecipientCount は、受給対象になった利用者の数です。
	RecipientCount int64
	// IssuedCouponCount は、実際に発行したクーポンの枚数です。
	IssuedCouponCount int64
}

// IssuePromotionalCoupons は、admin が退会していないすべての利用者へクーポンを一括発行し、
// 何が起きたかを件数で返します。
//
// 受給者は述語でしか決まらないため書き込みは CommandService が担います。原子性ではなく受給者を
// 識別子で名指しできないことがその理由で、判別根拠は
// ADR-0114 (predicate-defined-set-writes-on-commandservice) を参照。
func (u *usecase) IssuePromotionalCoupons(
	ctx context.Context,
	authn *auth.Authn,
	params IssuePromotionalCouponsParams,
) (IssuePromotionalCouponsView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionCouponBulkIssue, authz.NewResource("coupon", nil),
	); err != nil {
		return IssuePromotionalCouponsView{}, err
	}

	discount, err := newDiscount(params.DiscountKind, params.DiscountValue)
	if err != nil {
		return IssuePromotionalCouponsView{}, err
	}
	if err = ensureFlatDiscountWithinCap(discount); err != nil {
		return IssuePromotionalCouponsView{}, err
	}

	scope, err := newScope(params.ScopeKind, params.ScopeTargetID)
	if err != nil {
		return IssuePromotionalCouponsView{}, err
	}

	now := u.clock.Now()
	// 受給者が 0 人だとクーポンが 1 枚も構築されず coupon.New へ到達しないため、同じ規則をここでも
	// 問います。規則そのものはドメインが持ち、母集団の大きさで答えが変わらないことだけを保証します。
	if err = coupon.ValidateValidity(now, params.ExpiresAt); err != nil {
		return IssuePromotionalCouponsView{}, err
	}

	var view IssuePromotionalCouponsView
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		// 対象の存在確認は書き込みと同じトランザクションで行います。Idempotency-Key の有無で
		// idempotency.Run がトランザクションを開くかどうかが変わるため、外に置くと境界が要求次第になります。
		if serr := u.ensureScopeTargetExists(ctx, scope); serr != nil {
			return serr
		}

		// 上限は書き込みの前に判定します。CommandService の SQL では強制しません
		// （ドメイン不変条件から導出されない条件を CommandService へ持ち込まないため）。
		recipients, cerr := u.userRepo.CountByActive(ctx, ptr.To(true))
		if cerr != nil {
			return cerr
		}
		if recipients > maxPromotionRecipients {
			return ErrTooManyRecipients
		}

		result, ierr := u.bulkIssueCmd.IssuePromotionalCoupons(ctx, command.IssuePromotionalCouponsParams{
			Scope:     scope,
			Discount:  discount,
			ExpiresAt: params.ExpiresAt,
			IssuedAt:  now,
		})
		if ierr != nil {
			return ierr
		}

		// users 行はロックしないため、件数を数えてから挿入するまでの間に登録された利用者のぶんだけ
		// 上限を超え得ます。事前判定だけでは上限が破れるので、実際に書いた枚数で締めます。
		if result.IssuedCouponCount > maxPromotionRecipients {
			return ErrTooManyRecipients
		}

		view = IssuePromotionalCouponsView{
			IssuedAt:          now,
			ExpiresAt:         params.ExpiresAt,
			RecipientCount:    result.RecipientCount,
			IssuedCouponCount: result.IssuedCouponCount,
		}

		return nil
	})
	if err != nil {
		return IssuePromotionalCouponsView{}, err
	}

	return view, nil
}

// newDiscount は、要求が渡した名前をドメインの値引きへ写します。
// 名前の解決も値の検証もドメインが行い、ここは 2 つを繋ぐだけです。
func newDiscount(kindName string, value decimal.Decimal) (coupon.Discount, error) {
	kind, err := coupon.NewDiscountKindByName(kindName)
	if err != nil {
		return coupon.Discount{}, err
	}

	return coupon.ReconstructDiscount(kind, value)
}

// ensureFlatDiscountWithinCap は、定額値引きが一括発行の上限に収まることを確かめます。
//
// 定率はドメインが 1 を上限に持つため、際限なく大きくなり得るのは定額だけです。
func ensureFlatDiscountWithinCap(discount coupon.Discount) error {
	if discount.Kind() != coupon.DiscountKindFlat {
		return nil
	}
	if discount.Value().Cmp(maxPromotionFlatDiscount) > 0 {
		return ErrFlatDiscountTooLarge
	}

	return nil
}

// newScope は、要求が渡した名前と対象 ID をドメインの適用範囲へ写します。
// 対象 ID の要否は種別ごとに決まりますが、その規則はドメインが持ちます。
func newScope(kindName string, targetID *uuid.UUID) (coupon.Scope, error) {
	kind, err := coupon.NewScopeKindByName(kindName)
	if err != nil {
		return coupon.Scope{}, err
	}

	return coupon.ReconstructScope(kind, targetID)
}

// ensureScopeTargetExists は、適用範囲が指す対象の存在を確かめます。
//
// 存在しない対象を範囲にしたクーポンは誰にも使えません。発行は取り消せないため、配ってから
// 気づくことを避けます。全体の適用範囲は対象を持たないので何も確認しません。
func (u *usecase) ensureScopeTargetExists(ctx context.Context, scope coupon.Scope) error {
	targetID := scope.TargetID()
	if targetID == nil {
		return nil
	}

	switch scope.Kind() {
	case coupon.ScopeKindCategory:
		_, err := u.categoryRepo.FindByID(ctx, *targetID)

		return err
	case coupon.ScopeKindProduct:
		_, err := u.productRepo.FindByID(ctx, *targetID)

		return err
	default:
		return nil
	}
}
