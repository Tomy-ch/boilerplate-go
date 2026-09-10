//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package coupon は、クーポンの発行と読み取りのユースケースを提供します。
//
// 「いまのカートに使えるか」と「いくら引かれるか」は集約をまたぐ問いですが、判定そのものは
// クーポンのドメインが持ちます。そのため QueryService ではなく、Repository を束ねて
// ドメインへ渡す通常のユースケースとして組み立てます（docs/rules.md の Repository / QueryService Rules）。
package coupon

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/cart"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/domain/product/category"
	"go-boilerplate/internal/domain/user"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/internal/usecase/boundary/clock"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/internal/usecase/coupon/command"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// CouponView は、保有クーポン 1 枚の出力です。
type CouponView struct {
	// ID は、クーポン ID です。
	ID uuid.UUID
	// DiscountKind は、値引きの決まり方の名前です。
	DiscountKind string
	// DiscountValue は、種別における値です。定額なら金額、定率なら率です。
	DiscountValue decimal.Decimal
	// ScopeKind は、適用範囲の決まり方の名前です。
	ScopeKind string
	// ScopeTargetID は、適用範囲が絞る対象の識別子です。全体では nil です。
	ScopeTargetID *uuid.UUID
	// MaxAmount は、定率の値引きが 1 回に引ける額の上限（USD セント）です。上限が無い場合は nil です。
	MaxAmount *int64
	// MinPurchaseAmount は、使うために必要な購入額の下限（USD セント）です。条件が無い場合は nil です。
	MinPurchaseAmount *int64
	// UsableFrom は、使えるようになる日時です。発行時点から使える場合は nil です。
	UsableFrom *time.Time
	// ExpiresAt は、有効期限です。
	ExpiresAt time.Time
	// UsedAt は、使用日時です。未使用の場合は nil です。
	UsedAt *time.Time
	// IssuedAt は、発行日時です。
	IssuedAt time.Time
}

// CartCouponView は、いまのカートに使えるクーポン 1 枚と、その値引き額の組です。
type CartCouponView struct {
	// Coupon は、使えるクーポンです。
	Coupon CouponView
	// DiscountAmount は、適用した場合に差し引かれる額（USD セント）です。
	DiscountAmount int
}

// Usecase は、クーポンの発行と読み取りのユースケースを定義します。
type Usecase interface {
	// ListMyCoupons は、認証主体が保有するクーポンを [coupon.Repository.FindByUserID] と
	// 同じ結果で返します。
	ListMyCoupons(ctx context.Context, authn *auth.Authn) ([]CouponView, error)
	// ListApplicableToMyCart は、認証主体のカートに対して使えるクーポンと、それぞれの値引き額を返します。
	// 使用済み・失効済みと、値引きが 0 になるクーポンは含みません。
	// カートを持たない場合も空を返します。
	ListApplicableToMyCart(ctx context.Context, authn *auth.Authn) ([]CartCouponView, error)
	// IssueCoupon は、受給者を名指ししてクーポンを 1 枚発行し、発行したクーポンを返します。
	// admin のみ実行できます。受給者は退会していない利用者に限り、存在しない場合と退会済みの場合は
	// いずれも NotFound を返します。適用範囲が指す対象が存在しない場合も NotFound です。
	IssueCoupon(ctx context.Context, authn *auth.Authn, params IssueCouponParams) (CouponView, error)
	// IssuePromotionalCoupons は、退会していないすべての利用者へ同一条件のクーポンを 1 枚ずつ発行し、
	// その実行が起こしたことを件数で返します。admin のみ実行できます。
	IssuePromotionalCoupons(
		ctx context.Context, authn *auth.Authn, params IssuePromotionalCouponsParams,
	) (IssuePromotionalCouponsView, error)
}

type usecase struct {
	tracer       observability.LayerTracer
	couponRepo   coupon.Repository
	cartRepo     cart.Repository
	productRepo  product.Repository
	categoryRepo category.Repository
	userRepo     user.Repository
	clock        clock.Clock
	txm          tx.Manager
	authorizer   authz.Authorizer
	bulkIssueCmd command.CommandService
}

// New は、クーポンの発行と読み取りのユースケースを生成して返します。
func New(
	couponRepo coupon.Repository,
	cartRepo cart.Repository,
	productRepo product.Repository,
	categoryRepo category.Repository,
	userRepo user.Repository,
	clk clock.Clock,
	txm tx.Manager,
	authorizer authz.Authorizer,
	bulkIssueCmd command.CommandService,
	tf observability.TracerFactory,
) Usecase {
	return &usecase{
		tracer:       tf.Usecase(),
		couponRepo:   couponRepo,
		cartRepo:     cartRepo,
		productRepo:  productRepo,
		categoryRepo: categoryRepo,
		userRepo:     userRepo,
		clock:        clk,
		txm:          txm,
		authorizer:   authorizer,
		bulkIssueCmd: bulkIssueCmd,
	}
}

// ListMyCoupons は、保有するクーポンをそのまま並べます。使えるかどうかで絞りません。
func (u *usecase) ListMyCoupons(ctx context.Context, authn *auth.Authn) ([]CouponView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	userID, err := requireUserID(authn)
	if err != nil {
		return nil, err
	}

	coupons, err := u.couponRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	views := make([]CouponView, len(coupons))
	for i, c := range coupons {
		views[i] = toCouponView(c)
	}

	return views, nil
}

// ListApplicableToMyCart は、カートの明細を対象にクーポンごとの値引き額を求め、0 になるものを落とします。
// 対象明細の絞り込みは docs/spec/usecase/coupon.md の Workflow を参照。
//
// 最低購入金額に満たないクーポンは [coupon.Coupon.DiscountFor] が 0 を返すため、
// 対象明細が 1 件も無い場合と同じ経路で落ちます。
func (u *usecase) ListApplicableToMyCart(ctx context.Context, authn *auth.Authn) ([]CartCouponView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	userID, err := requireUserID(authn)
	if err != nil {
		return nil, err
	}

	coupons, err := u.couponRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(coupons) == 0 {
		return []CartCouponView{}, nil
	}

	now := u.clock.Now()

	lines, err := u.buildLines(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return []CartCouponView{}, nil
	}

	views := make([]CartCouponView, 0, len(coupons))
	for _, c := range coupons {
		if c.IsUsed() || c.IsExpired(now) || c.IsNotYetUsable(now) {
			continue
		}

		amount, derr := c.DiscountFor(lines)
		if derr != nil {
			return nil, derr
		}
		if amount <= 0 {
			continue
		}

		views = append(views, CartCouponView{Coupon: toCouponView(c), DiscountAmount: amount})
	}

	return views, nil
}

// buildLines は、カートの明細のうち購入できるものだけをクーポンの対象明細へ写します。
// カートを持たない場合と期限切れの場合は、いずれも空を返します。
//
// Repository は期限切れのカートも返し、期限の判定は Cart.IsExpired が持ちます
// （internal/domain/cart/cart_repository.go）。期限切れのカートは購入へ進めないため、
// カート表示（internal/usecase/cart）と同じく「表示するカートが無い」に畳みます。
func (u *usecase) buildLines(
	ctx context.Context, userID uuid.UUID, now time.Time,
) ([]coupon.Line, error) {
	c, err := u.cartRepo.FindByOwnerID(ctx, userID)
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if c.IsExpired(now) {
		return nil, nil
	}

	items := c.Items()
	if len(items) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(items))
	for i, item := range items {
		ids[i] = item.ProductID()
	}
	products, err := u.productRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	byID := make(map[uuid.UUID]*product.Product, len(products))
	for _, p := range products {
		byID[p.ID()] = p
	}

	lines := make([]coupon.Line, 0, len(items))
	for _, item := range items {
		p, ok := byID[item.ProductID()]
		if !ok {
			continue
		}

		snapshot := cart.NewProductSnapshot(cart.ProductSnapshotAttributes{
			Quantity:     p.Quantity(),
			Price:        p.Price(),
			Published:    p.IsPublished(),
			Discontinued: p.IsDiscontinued(),
		})
		if len(item.Evaluate(&snapshot).Issues()) > 0 {
			continue
		}

		lines = append(lines, coupon.NewLine(coupon.LineAttributes{
			ProductID:  p.ID(),
			CategoryID: p.Category().ID(),
			Subtotal:   p.Price().Decimal().Mul(decimal.FromInt(int64(item.Quantity()))),
		}))
	}

	return lines, nil
}

// toCouponView は、クーポン集約を出力 DTO の語彙へ写します。種別は code ではなく名前で出します。
func toCouponView(c *coupon.Coupon) CouponView {
	return CouponView{
		ID:                c.ID(),
		DiscountKind:      c.Discount().Kind().Name(),
		DiscountValue:     c.Discount().Value(),
		ScopeKind:         c.Scope().Kind().Name(),
		ScopeTargetID:     c.Scope().TargetID(),
		MaxAmount:         c.Discount().MaxAmount(),
		MinPurchaseAmount: c.MinPurchaseAmount(),
		UsableFrom:        c.UsableFrom(),
		ExpiresAt:         c.ExpiresAt(),
		UsedAt:            c.UsedAt(),
		IssuedAt:          c.IssuedAt(),
	}
}

// requireUserID は、認証主体から内部ユーザー ID を取り出します。
func requireUserID(authn *auth.Authn) (uuid.UUID, error) {
	if authn == nil {
		return uuid.UUID{}, apperror.ErrUnauthenticated
	}

	return authn.UserID()
}
