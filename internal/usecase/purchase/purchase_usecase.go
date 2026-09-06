//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE

// Package purchase は、購入の作成と状態遷移のユースケースを提供します。単価は価格スケール（ドル decimal）、
// 決済額は決済スケール（整数セント）で扱います（ADR-0038 (two-scale-quantity-model)）。
package purchase

import (
	"context"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/coupon"
	"go-boilerplate/internal/domain/product"
	"go-boilerplate/internal/domain/purchase"
	"go-boilerplate/internal/domain/service/membership"
	"go-boilerplate/internal/domain/user"
	"go-boilerplate/internal/observability"
	"go-boilerplate/internal/usecase/boundary/auth"
	"go-boilerplate/internal/usecase/boundary/authz"
	"go-boilerplate/internal/usecase/boundary/clock"
	outboxbndry "go-boilerplate/internal/usecase/boundary/outbox"
	"go-boilerplate/internal/usecase/boundary/tx"
	"go-boilerplate/internal/usecase/outbox"
	"go-boilerplate/internal/usecase/purchase/event"
	"go-boilerplate/internal/usecase/purchase/query"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

const (
	// aggregateType は、outbox の集約種別です。
	aggregateType = "purchase"
)

// DetailParam は、購入明細の入力（商品 ID と数量）です。
type DetailParam struct {
	ProductID uuid.UUID
	Quantity  int
}

// CreatePurchaseParams は、購入作成の入力パラメータです。
type CreatePurchaseParams struct {
	// UserID は、購入者（認証済みの内部ユーザー ID）です。
	UserID uuid.UUID
	// Details は、購入明細の配列です。
	Details []DetailParam
	// CouponID は、適用するクーポンの ID です。nil の場合は値引きを行いません。
	CouponID *uuid.UUID
}

// PurchaseDetailView は、購入明細のユースケース出力 DTO です。UnitPrice は価格スケール（ドル decimal）です。
type PurchaseDetailView struct {
	ProductID uuid.UUID
	Quantity  int
	UnitPrice decimal.Decimal
}

// PurchaseView は、購入 1 件分のユースケース出力 DTO です。金額はすべて USD セント単位の整数です。
type PurchaseView struct {
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	SubtotalAmount int
	DiscountAmount int
	AppliedCoupon  *AppliedCouponView
	TaxAmount      int
	ShippingFee    int
	TotalAmount    int
	Details        []PurchaseDetailView
	OrderedAt      time.Time
}

// AppliedCouponView は、購入に適用したクーポンのユースケース出力 DTO です。
type AppliedCouponView struct {
	ID            uuid.UUID
	DiscountKind  string
	DiscountValue decimal.Decimal
	ScopeKind     string
	ScopeTargetID *uuid.UUID
}

// CancelPurchaseParams は、購入キャンセルの入力パラメータです。
type CancelPurchaseParams struct {
	// PurchaseCode は、キャンセル対象の購入コードです。
	PurchaseCode string
	// UserID は、キャンセルを要求した認証済みの内部ユーザー ID です。所有権の検証に用います。
	UserID uuid.UUID
}

// CancelPurchaseView は、キャンセル後の購入 1 件分のユースケース出力 DTO です。ステータスは購入ステータス
// マスタで解決済みの ID と名称、CanceledAt はキャンセル日時です。金額はすべて USD セント単位の整数です。
type CancelPurchaseView struct {
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	StatusCode     int
	StatusName     string
	SubtotalAmount int
	TaxAmount      int
	ShippingFee    int
	TotalAmount    int
	Details        []PurchaseDetailView
	OrderedAt      time.Time
	CanceledAt     *time.Time
}

// PayPurchaseParams は、購入支払いの入力パラメータです。
type PayPurchaseParams struct {
	// PurchaseCode は、支払い対象の購入コードです。
	PurchaseCode string
	// UserID は、支払いを要求した認証済みの内部ユーザー ID です。所有権の検証に用います。
	UserID uuid.UUID
}

// PayPurchaseView は、支払い後の購入 1 件分のユースケース出力 DTO です。PaidAt は支払い日時です。
// ステータス・金額の表現は CancelPurchaseView を参照。
type PayPurchaseView struct {
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	StatusCode     int
	StatusName     string
	SubtotalAmount int
	TaxAmount      int
	ShippingFee    int
	TotalAmount    int
	Details        []PurchaseDetailView
	OrderedAt      time.Time
	PaidAt         *time.Time
}

// ShipPurchaseView は、発送後の購入 1 件分のユースケース出力 DTO です。ShippedAt は発送日時です。
// ステータス・金額の表現は CancelPurchaseView を参照。
type ShipPurchaseView struct {
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	StatusCode     int
	StatusName     string
	SubtotalAmount int
	TaxAmount      int
	ShippingFee    int
	TotalAmount    int
	Details        []PurchaseDetailView
	OrderedAt      time.Time
	ShippedAt      *time.Time
}

// DeliverPurchaseView は、配達完了後の購入 1 件分のユースケース出力 DTO です。DeliveredAt は配達日時です。
// ステータス・金額の表現は CancelPurchaseView を参照。
type DeliverPurchaseView struct {
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	StatusCode     int
	StatusName     string
	SubtotalAmount int
	TaxAmount      int
	ShippingFee    int
	TotalAmount    int
	Details        []PurchaseDetailView
	OrderedAt      time.Time
	DeliveredAt    *time.Time
}

// Usecase は、購入の作成・参照・状態遷移のユースケースを定義します。
type Usecase interface {
	// CreatePurchase は、明細から購入を作成します。在庫の引当・購入の成立・イベント発行は単一 tx で
	// 原子的に成立し、売り越しは 409 で成立させません。
	CreatePurchase(ctx context.Context, params CreatePurchaseParams) (PurchaseView, error)
	// GetPurchases は、購入履歴を注文日時降順（cursor ページネーション）で取得します。
	// 一覧は概要（code / totalAmount / status / orderedAt）のみを返します。
	// 既定の母集団は認証主体の購入のみで、params.IncludeOtherUsers が true のときだけ他ユーザーの購入も
	// 含みます。その指定は管理者のみが通り、管理者でない場合は 403 を返します。
	// params.Window で注文日時の対象期間を、params.StatusCodes でステータスを絞り込めます
	// （いずれもゼロ値は絞り込みなし）。
	GetPurchases(ctx context.Context, authn *auth.Authn, params ListPurchasesParams) (*PurchaseListView, error)
	// CancelPurchase は、本人の購入をキャンセルし、明細分の在庫を復元します。適用していたクーポンは
	// 有効期限内なら未使用へ戻し、失効していれば使用済みのまま残します。キャンセル・クーポンの返却・
	// 在庫復元・イベント発行は単一 tx で原子的に成立します。他ユーザーの購入・不存在はいずれも存在秘匿のため
	// NotFound（404）、不正遷移は 409 を返します。
	CancelPurchase(ctx context.Context, params CancelPurchaseParams) (CancelPurchaseView, error)
	// PayPurchase は、本人の購入を支払い済みへ遷移させます。決済 SDK / PSP 連携は行わない擬似決済です。
	// 状態遷移とイベント発行は単一 tx で原子的に成立します。他ユーザーの購入・不存在はいずれも
	// NotFound（404）、二重支払い・不正遷移は 409 を返します（存在秘匿の理由は CancelPurchase を参照）。
	PayPurchase(ctx context.Context, params PayPurchaseParams) (PayPurchaseView, error)
	// ShipPurchase は、購入を発送済みへ遷移させます。実行できるのは管理者のみで、購入の所有者であるかは問いません。
	// 状態遷移とイベント発行は単一 tx で原子的に成立します。管理者でない場合は 403、不存在は 404、
	// 二重発送・不正遷移は 409 を返します。
	ShipPurchase(ctx context.Context, authn *auth.Authn, purchaseCode string) (ShipPurchaseView, error)
	// DeliverPurchase は、購入を配達済みへ遷移させます。管理者専用・所有者不問の扱いは ShipPurchase を参照。
	// 状態遷移とイベント発行は単一 tx で原子的に成立します。管理者でない場合は 403、不存在は 404、
	// 二重配達・不正遷移は 409 を返します。
	DeliverPurchase(ctx context.Context, authn *auth.Authn, purchaseCode string) (DeliverPurchaseView, error)
	// GetPurchaseDetail は、本人の購入 1 件を明細（商品名込み）とともに取得します。
	// 他ユーザーの購入・不存在はいずれも NotFound（404）です（存在秘匿の理由は CancelPurchase を参照）。
	GetPurchaseDetail(ctx context.Context, authn *auth.Authn, purchaseCode string) (PurchaseGetDetailView, error)
	// ListShippablePurchases は、発送可能な購入を、まとめて発送してよい組に分けて取得します。
	// 実行できるのは管理者のみで、管理者でない場合は 403 を返します。
	ListShippablePurchases(
		ctx context.Context, authn *auth.Authn, params ListShippablePurchasesParams,
	) (PurchaseShippableListView, error)
}

type usecase struct {
	tracer      observability.LayerTracer
	txm         tx.Manager
	repo        purchase.Repository
	productRepo product.Repository
	couponRepo  coupon.Repository
	userLock    user.LockRepository
	detailQS    query.PurchaseDetailQueryService
	feedQS      query.PurchaseFeedQueryService
	emit        outbox.EmitUsecase
	clock       clock.Clock
	authorizer  authz.Authorizer
}

// purchaseDraft は、購入作成のトランザクションへ持ち込む採番済みの入力です。
type purchaseDraft struct {
	purchaseID uuid.UUID
	code       string
	inputs     []purchase.DetailInput
	productIDs []uuid.UUID
}

// New は、購入ユースケースを生成します。
func New(
	txm tx.Manager,
	repo purchase.Repository,
	productRepo product.Repository,
	couponRepo coupon.Repository,
	userLock user.LockRepository,
	detailQS query.PurchaseDetailQueryService,
	feedQS query.PurchaseFeedQueryService,
	emit outbox.EmitUsecase,
	clock clock.Clock,
	authorizer authz.Authorizer,
	tf observability.TracerFactory,
) Usecase {
	return &usecase{
		tracer:      tf.Usecase(),
		txm:         txm,
		repo:        repo,
		productRepo: productRepo,
		couponRepo:  couponRepo,
		userLock:    userLock,
		detailQS:    detailQS,
		feedQS:      feedQS,
		emit:        emit,
		clock:       clock,
		authorizer:  authorizer,
	}
}

// detailProductIDs は、明細が参照する商品 ID を重複なく返します。
func detailProductIDs(details []purchase.PurchaseDetail) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(details))
	ids := make([]uuid.UUID, 0, len(details))
	for _, d := range details {
		if _, ok := seen[d.ProductID()]; ok {
			continue
		}
		seen[d.ProductID()] = struct{}{}
		ids = append(ids, d.ProductID())
	}

	return ids
}

// newPurchaseDraft は、購入・購入コード・各明細の ID を採番し、ドメイン入力と商品 ID 列を組み立てます。
// 採番はトランザクションの外で行うため、リトライされても同じ ID が再利用されることはありません。
func newPurchaseDraft(details []DetailParam) (*purchaseDraft, error) {
	purchaseID, err := uuid.New()
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to generate purchase id")
	}
	codeUUID, err := uuid.New()
	if err != nil {
		return nil, xerrors.Wrap(err, "failed to generate purchase code")
	}

	draft := &purchaseDraft{
		purchaseID: purchaseID,
		code:       codeUUID.String(),
		inputs:     make([]purchase.DetailInput, len(details)),
		productIDs: make([]uuid.UUID, len(details)),
	}
	for i, d := range details {
		detailID, derr := uuid.New()
		if derr != nil {
			return nil, xerrors.Wrap(derr, "failed to generate purchase detail id")
		}
		draft.inputs[i] = purchase.DetailInput{ID: detailID, ProductID: d.ProductID, Quantity: d.Quantity}
		draft.productIDs[i] = d.ProductID
	}
	return draft, nil
}

func (u *usecase) CreatePurchase(ctx context.Context, params CreatePurchaseParams) (PurchaseView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	draft, err := newPurchaseDraft(params.Details)
	if err != nil {
		return PurchaseView{}, err
	}

	now := u.clock.Now()

	var (
		created       *purchase.Purchase
		appliedCoupon *coupon.Coupon
	)
	// nested で最外 tx に乗る（tx 所有については docs/spec/usecase/purchase.md 冒頭を参照）。
	if txErr := u.txm.Do(ctx, func(ctx context.Context) error {
		entity, redeemed, cerr := u.createPurchaseInTx(ctx, params, draft, now)
		if cerr != nil {
			return cerr
		}
		created, appliedCoupon = entity, redeemed

		return nil
	}); txErr != nil {
		return PurchaseView{}, txErr
	}

	return toPurchaseView(created, appliedCoupon), nil
}

// applyRedeemedCoupon は、引き換えたクーポンの値引きを購入へ適用します。
// 対象明細の組み立てと拒否の条件は docs/spec/usecase/purchase.md の § クーポンの適用（CreatePurchase）を参照。
func applyRedeemedCoupon(entity *purchase.Purchase, redeemed *coupon.Coupon, products product.Products) error {
	if redeemed == nil {
		return nil
	}

	categories := make(map[uuid.UUID]uuid.UUID, len(products))
	for _, p := range products {
		categories[p.ID()] = p.Category().ID()
	}

	details := entity.Details()
	lines := make([]coupon.Line, len(details))
	for i, d := range details {
		lines[i] = coupon.NewLine(coupon.LineAttributes{
			ProductID:  d.ProductID(),
			CategoryID: categories[d.ProductID()],
			Subtotal:   d.LineTotal(),
		})
	}

	amount, err := redeemed.DiscountFor(lines)
	if err != nil {
		return err
	}

	return entity.ApplyCoupon(redeemed.ID(), amount)
}

func (u *usecase) CancelPurchase(ctx context.Context, params CancelPurchaseParams) (CancelPurchaseView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	now := u.clock.Now()

	var detail *purchase.Detail
	// tx 境界は PayPurchase のコメントを参照。二重キャンセル対策は
	// docs/spec/usecase/purchase.md § PATCH キャンセル を参照。
	if txErr := u.txm.Do(ctx, func(ctx context.Context) error {
		reread, cerr := u.cancelPurchaseInTx(ctx, params, now)
		if cerr != nil {
			return cerr
		}
		detail = reread

		return nil
	}); txErr != nil {
		return CancelPurchaseView{}, txErr
	}

	return toCancelPurchaseView(detail), nil
}

// cancelPurchaseInTx は、購入キャンセルのトランザクション本体です。
//
// ロック順序（購入行 → クーポン行 → 商品行、id 昇順）は docs/spec/usecase/purchase.md の
// § クーポンの返却（CancelPurchase）を参照。書き込み後は Repository 経由で再検証します
// （internal/usecase/README.md の Verifying infrastructure against the domain）。
func (u *usecase) cancelPurchaseInTx(
	ctx context.Context, params CancelPurchaseParams, now time.Time,
) (*purchase.Detail, error) {
	locked, lerr := u.repo.LockByCode(ctx, params.PurchaseCode)
	if lerr != nil {
		return nil, lerr
	}

	// 存在秘匿のため NotFound へ畳む理由は CancelPurchase の doc コメントを参照。
	if locked.UserID() != params.UserID {
		return nil, xerrors.Wrap(apperror.ErrNotFound, "purchase not found")
	}

	domainEvent, cerr := locked.Cancel(now)
	if cerr != nil {
		return nil, cerr
	}

	if rerr := u.restoreCoupon(ctx, locked.CouponID(), now); rerr != nil {
		return nil, rerr
	}

	if serr := u.restoreStock(ctx, locked.Details()); serr != nil {
		return nil, serr
	}

	if perr := u.repo.UpdateCancelled(ctx, locked); perr != nil {
		return nil, perr
	}

	payload, berr := event.BuildCanceled(locked)
	if berr != nil {
		return nil, berr
	}
	eventType, terr := event.WireType(domainEvent.Type())
	if terr != nil {
		return nil, terr
	}
	if _, eerr := u.emit.Emit(ctx, outbox.EmitInput{
		AggregateType: aggregateType,
		AggregateID:   locked.ID().String(),
		EventType:     eventType,
		Payload:       payload,
		Channel:       outboxbndry.ChannelHTTP,
	}); eerr != nil {
		return nil, eerr
	}

	return u.repo.FindDetailByID(ctx, locked.ID())
}

func (u *usecase) PayPurchase(ctx context.Context, params PayPurchaseParams) (PayPurchaseView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	now := u.clock.Now()

	var detail *purchase.Detail
	// この Do が最外 tx（本経路は Idempotency-Key を配線しない）。
	// 二重支払い対策は docs/spec/usecase/purchase.md § PATCH 支払い を参照。
	if txErr := u.txm.Do(ctx, func(ctx context.Context) error {
		locked, lerr := u.repo.LockByCode(ctx, params.PurchaseCode)
		if lerr != nil {
			return lerr
		}

		// 存在秘匿のため NotFound へ畳む理由は CancelPurchase の doc コメントを参照。
		if locked.UserID() != params.UserID {
			return xerrors.Wrap(apperror.ErrNotFound, "purchase not found")
		}

		domainEvent, perr := locked.Pay(now)
		if perr != nil {
			return perr
		}

		if perr := u.repo.UpdatePaid(ctx, locked); perr != nil {
			return perr
		}

		payload, berr := event.BuildPaid(locked)
		if berr != nil {
			return berr
		}
		eventType, terr := event.WireType(domainEvent.Type())
		if terr != nil {
			return terr
		}
		if _, eerr := u.emit.Emit(ctx, outbox.EmitInput{
			AggregateType: aggregateType,
			AggregateID:   locked.ID().String(),
			EventType:     eventType,
			Payload:       payload,
			Channel:       outboxbndry.ChannelHTTP,
		}); eerr != nil {
			return eerr
		}

		reread, rerr := u.repo.FindDetailByID(ctx, locked.ID())
		if rerr != nil {
			return rerr
		}
		detail = reread
		return nil
	}); txErr != nil {
		return PayPurchaseView{}, txErr
	}

	return toPayPurchaseView(detail), nil
}

func (u *usecase) ShipPurchase(
	ctx context.Context, authn *auth.Authn, purchaseCode string,
) (ShipPurchaseView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if authn == nil {
		return ShipPurchaseView{}, apperror.ErrUnauthenticated
	}
	// 管理者専用・所有者不問の根拠は ShipPurchase の doc コメントを参照。
	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionPurchaseShip, authz.NewResource("purchase", nil),
	); err != nil {
		return ShipPurchaseView{}, err
	}

	now := u.clock.Now()

	var detail *purchase.Detail
	// tx 境界は PayPurchase のコメントを参照。二重発送対策は
	// docs/spec/usecase/purchase.md § PATCH 発送 を参照。
	if txErr := u.txm.Do(ctx, func(ctx context.Context) error {
		locked, lerr := u.repo.LockByCode(ctx, purchaseCode)
		if lerr != nil {
			return lerr
		}

		domainEvent, serr := locked.Ship(now)
		if serr != nil {
			return serr
		}

		if uerr := u.repo.UpdateShipped(ctx, locked); uerr != nil {
			return uerr
		}

		payload, berr := event.BuildShipped(locked)
		if berr != nil {
			return berr
		}
		eventType, terr := event.WireType(domainEvent.Type())
		if terr != nil {
			return terr
		}
		if _, eerr := u.emit.Emit(ctx, outbox.EmitInput{
			AggregateType: aggregateType,
			AggregateID:   locked.ID().String(),
			EventType:     eventType,
			Payload:       payload,
			Channel:       outboxbndry.ChannelHTTP,
		}); eerr != nil {
			return eerr
		}

		reread, rerr := u.repo.FindDetailByID(ctx, locked.ID())
		if rerr != nil {
			return rerr
		}
		detail = reread
		return nil
	}); txErr != nil {
		return ShipPurchaseView{}, txErr
	}

	return toShipPurchaseView(detail), nil
}

func (u *usecase) DeliverPurchase(
	ctx context.Context, authn *auth.Authn, purchaseCode string,
) (DeliverPurchaseView, error) {
	ctx, endSpan := u.tracer.Start(ctx)
	defer endSpan()

	if authn == nil {
		return DeliverPurchaseView{}, apperror.ErrUnauthenticated
	}
	// 管理者専用・所有者不問の根拠は ShipPurchase の doc コメントを参照。
	if err := u.authorizer.Authorize(
		ctx, authn, authz.ActionPurchaseDeliver, authz.NewResource("purchase", nil),
	); err != nil {
		return DeliverPurchaseView{}, err
	}

	now := u.clock.Now()

	var detail *purchase.Detail
	// tx 境界は PayPurchase のコメントを参照。二重配達対策は
	// docs/spec/usecase/purchase.md § PATCH 配達完了 を参照。
	if txErr := u.txm.Do(ctx, func(ctx context.Context) error {
		locked, lerr := u.repo.LockByCode(ctx, purchaseCode)
		if lerr != nil {
			return lerr
		}

		domainEvent, derr := locked.Deliver(now)
		if derr != nil {
			return derr
		}

		if uerr := u.repo.UpdateDelivered(ctx, locked); uerr != nil {
			return uerr
		}

		payload, berr := event.BuildDelivered(locked)
		if berr != nil {
			return berr
		}
		eventType, terr := event.WireType(domainEvent.Type())
		if terr != nil {
			return terr
		}
		if _, eerr := u.emit.Emit(ctx, outbox.EmitInput{
			AggregateType: aggregateType,
			AggregateID:   locked.ID().String(),
			EventType:     eventType,
			Payload:       payload,
			Channel:       outboxbndry.ChannelHTTP,
		}); eerr != nil {
			return eerr
		}

		reread, rerr := u.repo.FindDetailByID(ctx, locked.ID())
		if rerr != nil {
			return rerr
		}
		detail = reread
		return nil
	}); txErr != nil {
		return DeliverPurchaseView{}, txErr
	}

	return toDeliverPurchaseView(detail), nil
}

// ensurePurchaserActive は、購入者を共有ロック付きで読み出し、購入してよい状態かの判定を
// ドメインサービスへ委ねます。退会（排他ロック）と直列化されるため、確認を通った購入者は
// tx の終了まで退会できません。エラー方針は docs/spec/usecase/purchase.md の Workflow を参照。
func (u *usecase) ensurePurchaserActive(ctx context.Context, userID uuid.UUID) error {
	purchaser, err := u.userLock.LockShareByID(ctx, userID)
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return membership.ErrPurchaserWithdrawn
		}
		return err
	}
	return membership.EnsurePurchasable(purchaser)
}

// toAppliedCouponView は、適用したクーポンを出力 DTO の語彙へ写します。未適用の場合は nil です。
func toAppliedCouponView(c *coupon.Coupon) *AppliedCouponView {
	if c == nil {
		return nil
	}

	return &AppliedCouponView{
		ID:            c.ID(),
		DiscountKind:  c.Discount().Kind().Name(),
		DiscountValue: c.Discount().Value(),
		ScopeKind:     c.Scope().Kind().Name(),
		ScopeTargetID: c.Scope().TargetID(),
	}
}

func toPurchaseView(p *purchase.Purchase, applied *coupon.Coupon) PurchaseView {
	details := p.Details()
	views := make([]PurchaseDetailView, len(details))
	for i, d := range details {
		views[i] = PurchaseDetailView{
			ProductID: d.ProductID(),
			Quantity:  d.Quantity(),
			UnitPrice: d.UnitPrice().Decimal(),
		}
	}
	return PurchaseView{
		Code:           p.Code(),
		UserID:         p.UserID(),
		StatusID:       p.StatusID(),
		SubtotalAmount: p.SubtotalAmount(),
		DiscountAmount: p.DiscountAmount(),
		AppliedCoupon:  toAppliedCouponView(applied),
		TaxAmount:      p.TaxAmount(),
		ShippingFee:    p.ShippingFee(),
		TotalAmount:    p.TotalAmount(),
		Details:        views,
		OrderedAt:      p.OrderedAt(),
	}
}

func toCancelPurchaseView(d *purchase.Detail) CancelPurchaseView {
	views := make([]PurchaseDetailView, len(d.Details))
	for i, detail := range d.Details {
		views[i] = PurchaseDetailView{
			ProductID: detail.ProductID(),
			Quantity:  detail.Quantity(),
			UnitPrice: detail.UnitPrice().Decimal(),
		}
	}
	return CancelPurchaseView{
		Code:           d.Code,
		UserID:         d.UserID,
		StatusID:       d.StatusID,
		StatusCode:     d.StatusCode,
		StatusName:     d.StatusName,
		SubtotalAmount: d.SubtotalAmount,
		TaxAmount:      d.TaxAmount,
		ShippingFee:    d.ShippingFee,
		TotalAmount:    d.TotalAmount,
		Details:        views,
		OrderedAt:      d.OrderedAt,
		CanceledAt:     d.CanceledAt,
	}
}

func toShipPurchaseView(d *purchase.Detail) ShipPurchaseView {
	views := make([]PurchaseDetailView, len(d.Details))
	for i, detail := range d.Details {
		views[i] = PurchaseDetailView{
			ProductID: detail.ProductID(),
			Quantity:  detail.Quantity(),
			UnitPrice: detail.UnitPrice().Decimal(),
		}
	}
	return ShipPurchaseView{
		Code:           d.Code,
		UserID:         d.UserID,
		StatusID:       d.StatusID,
		StatusCode:     d.StatusCode,
		StatusName:     d.StatusName,
		SubtotalAmount: d.SubtotalAmount,
		TaxAmount:      d.TaxAmount,
		ShippingFee:    d.ShippingFee,
		TotalAmount:    d.TotalAmount,
		Details:        views,
		OrderedAt:      d.OrderedAt,
		ShippedAt:      d.ShippedAt,
	}
}

func toDeliverPurchaseView(d *purchase.Detail) DeliverPurchaseView {
	views := make([]PurchaseDetailView, len(d.Details))
	for i, detail := range d.Details {
		views[i] = PurchaseDetailView{
			ProductID: detail.ProductID(),
			Quantity:  detail.Quantity(),
			UnitPrice: detail.UnitPrice().Decimal(),
		}
	}
	return DeliverPurchaseView{
		Code:           d.Code,
		UserID:         d.UserID,
		StatusID:       d.StatusID,
		StatusCode:     d.StatusCode,
		StatusName:     d.StatusName,
		SubtotalAmount: d.SubtotalAmount,
		TaxAmount:      d.TaxAmount,
		ShippingFee:    d.ShippingFee,
		TotalAmount:    d.TotalAmount,
		Details:        views,
		OrderedAt:      d.OrderedAt,
		DeliveredAt:    d.DeliveredAt,
	}
}

func toPayPurchaseView(d *purchase.Detail) PayPurchaseView {
	views := make([]PurchaseDetailView, len(d.Details))
	for i, detail := range d.Details {
		views[i] = PurchaseDetailView{
			ProductID: detail.ProductID(),
			Quantity:  detail.Quantity(),
			UnitPrice: detail.UnitPrice().Decimal(),
		}
	}
	return PayPurchaseView{
		Code:           d.Code,
		UserID:         d.UserID,
		StatusID:       d.StatusID,
		StatusCode:     d.StatusCode,
		StatusName:     d.StatusName,
		SubtotalAmount: d.SubtotalAmount,
		TaxAmount:      d.TaxAmount,
		ShippingFee:    d.ShippingFee,
		TotalAmount:    d.TotalAmount,
		Details:        views,
		OrderedAt:      d.OrderedAt,
		PaidAt:         d.PaidAt,
	}
}

// restoreCoupon は、キャンセルした購入が適用していたクーポンを未使用へ戻します。
// クーポンを適用していない購入では何もしません。失効したクーポンは戻さず、それは失敗ではありません
// （規則は docs/spec/usecase/purchase.md の § クーポンの返却（CancelPurchase））。
func (u *usecase) restoreCoupon(ctx context.Context, couponID *uuid.UUID, now time.Time) error {
	if couponID == nil {
		return nil
	}

	c, err := u.couponRepo.LockByID(ctx, *couponID)
	if err != nil {
		return err
	}

	restored, rerr := c.Restore(now)
	if rerr != nil {
		return rerr
	}
	if !restored {
		return nil
	}

	return u.couponRepo.UpdateUnused(ctx, c.ID())
}

// restoreStock は、キャンセルした明細ぶんの在庫を対象商品へ戻します。
// 対象商品をロックしてからドメインで適用します。順序（id 昇順）は LockByIDs が担保します
// （ADR-0036 (ordered-pessimistic-row-locks)）。
func (u *usecase) restoreStock(ctx context.Context, details []purchase.PurchaseDetail) error {
	products, err := u.productRepo.LockByIDs(ctx, detailProductIDs(details))
	if err != nil {
		return err
	}

	return u.applyStockDelta(ctx, products, details, 1)
}

// createPurchaseInTx は、購入作成のトランザクション本体です。
//
// ロック順序（ユーザー行 → クーポン行 → 商品行、id 昇順）は docs/spec/usecase/purchase.md の
// § クーポンの適用（CreatePurchase）を参照。書き込み後は Repository 経由で再検証します
// （internal/usecase/README.md の Verifying infrastructure against the domain）。
func (u *usecase) createPurchaseInTx(
	ctx context.Context, params CreatePurchaseParams, draft *purchaseDraft, now time.Time,
) (*purchase.Purchase, *coupon.Coupon, error) {
	if uerr := u.ensurePurchaserActive(ctx, params.UserID); uerr != nil {
		return nil, nil, uerr
	}

	redeemed, rerr := u.redeemRequestedCoupon(ctx, params, now)
	if rerr != nil {
		return nil, nil, rerr
	}

	products, lerr := u.productRepo.LockByIDs(ctx, draft.productIDs)
	if lerr != nil {
		return nil, nil, lerr
	}
	locked := make([]purchase.LockedProduct, len(products))
	for i, p := range products {
		locked[i] = purchase.NewLockedProduct(p.ID(), p.Price(), p.Quantity())
	}

	entity, nerr := purchase.New(draft.purchaseID, draft.code, params.UserID, draft.inputs, locked)
	if nerr != nil {
		return nil, nil, nerr
	}
	if aerr := applyRedeemedCoupon(entity, redeemed, products); aerr != nil {
		return nil, nil, aerr
	}

	// 充足（在庫超過が無いこと）は直前の purchase.New が locked と突き合わせて検証済み。
	if serr := u.applyStockDelta(ctx, products, entity.Details(), -1); serr != nil {
		return nil, nil, serr
	}
	if cerr := u.repo.Create(ctx, entity); cerr != nil {
		return nil, nil, cerr
	}
	if uerr := u.markCouponUsed(ctx, redeemed, now); uerr != nil {
		return nil, nil, uerr
	}

	reread, frerr := u.repo.FindByID(ctx, draft.purchaseID)
	if frerr != nil {
		return nil, nil, frerr
	}

	if eerr := u.emitCreated(ctx, reread); eerr != nil {
		return nil, nil, eerr
	}

	return reread, redeemed, nil
}

// emitCreated は、購入作成のイベントを outbox へ積みます。受け取るのは書き込み後に読み直した集約です
// （順序の理由は docs/spec/usecase/purchase.md の Workflow ⑥ を参照）。
func (u *usecase) emitCreated(ctx context.Context, entity *purchase.Purchase) error {
	payload, err := event.BuildCreated(entity)
	if err != nil {
		return err
	}

	if _, eerr := u.emit.Emit(ctx, outbox.EmitInput{
		AggregateType: aggregateType,
		AggregateID:   entity.ID().String(),
		EventType:     event.TypeCreated,
		Payload:       payload,
		Channel:       outboxbndry.ChannelHTTP,
	}); eerr != nil {
		return eerr
	}

	return nil
}

// redeemRequestedCoupon は、要求にクーポンの指定があればそれを引き換えます。指定が無ければ nil を返します。
func (u *usecase) redeemRequestedCoupon(
	ctx context.Context, params CreatePurchaseParams, now time.Time,
) (*coupon.Coupon, error) {
	if params.CouponID == nil {
		return nil, nil //nolint:nilnil // 「クーポンの指定が無い」を表す正当な状態で、エラーではない
	}

	return u.redeemCoupon(ctx, *params.CouponID, params.UserID, now)
}

// markCouponUsed は、引き換えたクーポンを使用済みとして確定させます。購入行を書いたあとに呼びます
// （順序の理由は docs/spec/usecase/purchase.md の § クーポンの適用（CreatePurchase）を参照）。
func (u *usecase) markCouponUsed(ctx context.Context, redeemed *coupon.Coupon, now time.Time) error {
	if redeemed == nil {
		return nil
	}

	return u.couponRepo.UpdateUsed(ctx, redeemed.ID(), now)
}

// redeemCoupon は、指定されたクーポンを行ロックのもとで検証し、使用済みへ遷移させます。
// エラーの畳み方は docs/spec/usecase/purchase.md の § クーポンの適用（CreatePurchase）を参照。
func (u *usecase) redeemCoupon(
	ctx context.Context, couponID, userID uuid.UUID, now time.Time,
) (*coupon.Coupon, error) {
	c, err := u.couponRepo.LockByID(ctx, couponID)
	if err != nil {
		if xerrors.Is(err, apperror.ErrNotFound) {
			return nil, notHeldError()
		}
		return nil, err
	}
	if !c.IsHeldBy(userID) {
		return nil, notHeldError()
	}

	if rerr := c.Redeem(now); rerr != nil {
		return nil, apperror.WithDetails(rerr, coupon.FieldCouponID)
	}

	return c, nil
}

// notHeldError は、保有していないクーポンを指した場合のエラーを組み立てます。
func notHeldError() error {
	return apperror.WithDetails(coupon.ErrNotHeld, coupon.FieldCouponID)
}

// applyStockDelta は、明細の数量 × sign を対象商品の在庫へ適用して永続化します。
// sign は購入時に -1、キャンセルによる復元時に 1 を取ります。products はロック済みである必要があり、
// 在庫の範囲判定は商品集約の AdjustStock が行います。
func (u *usecase) applyStockDelta(
	ctx context.Context, products product.Products, details []purchase.PurchaseDetail, sign int,
) error {
	byID := make(map[uuid.UUID]*product.Product, len(products))
	for _, p := range products {
		byID[p.ID()] = p
	}

	for _, d := range details {
		p, ok := byID[d.ProductID()]
		if !ok {
			return xerrors.Wrap(apperror.ErrNotFound, "product not found for purchase detail")
		}
		if aerr := p.AdjustStock(sign * d.Quantity()); aerr != nil {
			return aerr
		}
		if _, uerr := u.productRepo.UpdateStock(ctx, p); uerr != nil {
			return uerr
		}
	}

	return nil
}
