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

// maxPromotionRecipients は、1 回の一括発行で配れる上限です。この操作は取り消せないため、誤った
// 条件のまま事故的に配り切ることを防ぎます。placeholder であり、実要件が立った時点で改めます
// （分類は docs/spec/usecase/coupon.md の Workflow — IssuePromotionalCoupons の invariants を参照）。
const maxPromotionRecipients int64 = 10_000

// maxPromotionFlatDiscountValue は、[maxPromotionFlatDiscount] の値です。decimal は const に
// できないため、数値だけを const として持ちます（ドメインの maxDiscountRate と同じ形）。
const maxPromotionFlatDiscountValue int64 = 100_000

// maxPromotionFlatDiscount は、1 回の一括発行で配れる定額値引きの上限です。取り消せない操作で
// 桁を 1 つ多く打つだけで全利用者へ届くため、件数と同じく上限を置きます。placeholder であり、
// 実要件が立った時点で改めます（定率との非対称は [ensureFlatDiscountWithinCap]、分類は
// docs/spec/usecase/coupon.md の Workflow — IssuePromotionalCoupons の invariants を参照）。
var maxPromotionFlatDiscount = decimal.FromInt(maxPromotionFlatDiscountValue)

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
// 受給者は述語でしか決まらないため、書き込みは [command.CommandService.IssuePromotionalCoupons]
// が担います。
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

	var view IssuePromotionalCouponsView
	err = u.txm.Do(ctx, func(ctx context.Context) error {
		// 対象の存在確認は書き込みと同じトランザクションで行います。Idempotency-Key の有無で
		// idempotency.Run がトランザクションを開くかどうかが変わるため、外に置くと境界が要求次第になります。
		if serr := u.ensureScopeTargetExists(ctx, scope); serr != nil {
			return serr
		}

		// 上限は書き込みの前に判定します（分類は docs/spec/usecase/coupon.md の
		// Workflow — IssuePromotionalCoupons の invariants を参照）。
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

		// 事前判定だけでは上限が破れるため、実際に書いた枚数で締めます（母集団の性質は
		// docs/spec/usecase/coupon.md の Workflow — IssuePromotionalCoupons の invariants を参照）。
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

	return coupon.NewDiscount(kind, value)
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

	return coupon.NewScope(kind, targetID)
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
