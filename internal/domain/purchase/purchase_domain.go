// Package purchase は、購入ドメインを定義します。購入（Purchase）集約と明細（PurchaseDetail）を持ち、
// 金額計算・売り越し検証・単価スナップショットの不変条件を保持します。
//
// 金額は 2 スケールモデル（ADR-0038 (two-scale-quantity-model)）で扱います。単価（unit_price）はドル主単位のサブセント可 decimal
// （価格スケール）を money.Price で保持し、決済額（小計・税・送料・合計）は整数セント（決済スケール）で保持します。
// 価格スケールから決済スケールへの丸めは切り捨てで、New 内の 1 箇所（小計算出）に集約します。
package purchase

import (
	"fmt"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/pkg/decimal"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// DetailInput は、購入明細の入力です。ID は採番済みの明細 ID、ProductID は購入対象商品、Quantity は数量です。
type DetailInput struct {
	// ID は、採番済みの明細 ID です（UUIDv7）。
	ID uuid.UUID
	// ProductID は、購入対象の商品 ID です。
	ProductID uuid.UUID
	// Quantity は、購入数量です。
	Quantity int
}

// LockedProduct は、在庫ロック取得直後の商品スナップショットです。Price は単価スナップショットの元値（価格スケール）です。
type LockedProduct struct {
	id       uuid.UUID
	price    money.Price
	quantity int
}

// PurchaseDetail は、購入明細を表す値オブジェクトです。UnitPrice は購入時点の単価スナップショット（価格スケール）です。
type PurchaseDetail struct {
	id        uuid.UUID
	productID uuid.UUID
	quantity  int
	unitPrice money.Price
}

// Purchases は、Purchase 集約のスライス型です。
type Purchases []*Purchase

// Purchase は、購入を表すドメイン集約です。決済額（小計・税・送料・合計）は整数セント（決済スケール）で保持します。
// 状態機械の現在状態は statusCode（安定した業務キー）を source of truth とし、timestamps
// （canceledAt / shippedAt / deliveredAt）は「イベントがいつ起きたか」の監査記録として併用します。
type Purchase struct {
	id             uuid.UUID
	code           string
	userID         uuid.UUID
	statusID       uuid.UUID
	status         Status
	subtotalAmount int
	couponID       *uuid.UUID
	discountAmount int
	taxAmount      int
	shippingFee    int
	totalAmount    int
	details        []PurchaseDetail
	orderedAt      time.Time
	paidAt         *time.Time
	canceledAt     *time.Time
	shippedAt      *time.Time
	deliveredAt    *time.Time
}

// PurchaseDetailAttributes は、明細の再構築に必要な属性一式です。id と productID は同型で隣接するため
// 構造体で受けます（docs/rules.md Function Signature Rules）。
type PurchaseDetailAttributes struct {
	ProductID uuid.UUID
	Quantity  int
	// UnitPrice は価格スケール（ドル decimal）です。
	UnitPrice money.Price
}

// Attributes は、購入の再構築に必要な属性一式です。金額 4 つ（int）とイベント日時 4 つ（*time.Time）が
// 同型のため構造体で受けます（docs/rules.md Function Signature Rules）。
type Attributes struct {
	Code           string
	UserID         uuid.UUID
	StatusID       uuid.UUID
	StatusCode     int
	SubtotalAmount int
	CouponID       *uuid.UUID
	DiscountAmount int
	TaxAmount      int
	ShippingFee    int
	TotalAmount    int
	Details        []PurchaseDetail
	OrderedAt      time.Time
	PaidAt         *time.Time
	CanceledAt     *time.Time
	ShippedAt      *time.Time
	DeliveredAt    *time.Time
}

// settlementBasis は、税・送料・合計を決める材料です。
// どちらも同じ尺度の金額なので、位置ではなく名前で渡します（docs/rules.md の Function Signature Rules）。
type settlementBasis struct {
	// Subtotal は、明細の小計（USD セント）です。
	Subtotal int
	// Discount は、クーポンによる値引き額（USD セント）です。
	Discount int
}

// NewLockedProduct は、ロック済み商品スナップショットを生成します。price は価格スケール（ドル decimal）です。
func NewLockedProduct(id uuid.UUID, price money.Price, quantity int) LockedProduct {
	return LockedProduct{id: id, price: price, quantity: quantity}
}

// NewPurchaseDetail は、永続化済みの明細を組み立てます（Repository の読み出しで使用）。
// 不変条件は集約へ入る時点（Reconstruct）で判定します
// （internal/domain/README.md § Do not set outside constructor）。
func NewPurchaseDetail(id uuid.UUID, attrs PurchaseDetailAttributes) PurchaseDetail {
	return PurchaseDetail{
		id:        id,
		productID: attrs.ProductID,
		quantity:  attrs.Quantity,
		unitPrice: attrs.UnitPrice,
	}
}

// validateDetails は、購入集約が保持する明細の集合が満たすべき不変条件を検証します。
// 明細 ID / 商品 ID が未設定、数量が最小値未満の場合はそれぞれ ErrInvalidID / ErrInvalidQuantity を、
// 同一商品 ID が重複する場合は ErrDuplicateProductID を返します。
// 空の集合は呼び出し元が ErrEmptyDetails で扱うため、ここでは許容します。
func validateDetails(details []PurchaseDetail) error {
	seen := make(map[uuid.UUID]struct{}, len(details))
	for _, d := range details {
		if d.id.IsNil() {
			return xerrors.Wrap(ErrInvalidID, "detail id is required")
		}
		if d.productID.IsNil() {
			return xerrors.Wrap(ErrInvalidID, "detail product id is required")
		}
		if d.quantity < minQuantity {
			return xerrors.Wrap(
				ErrInvalidQuantity, fmt.Sprintf("quantity must be %d or greater, got %d", minQuantity, d.quantity),
			)
		}
		if _, dup := seen[d.productID]; dup {
			return xerrors.Wrap(ErrDuplicateProductID, fmt.Sprintf("product %s appears more than once", d.productID))
		}
		seen[d.productID] = struct{}{}
	}

	return nil
}

// buildDetails は、明細の入力をロック済み在庫と突き合わせて購入明細を組み立て、単価スナップショットに
// よる小計を価格スケールで返します（検証条件は New を参照）。入力の順序を保った明細を返します。
func buildDetails(inputs []DetailInput, locked []LockedProduct) ([]PurchaseDetail, decimal.Decimal, error) {
	lockedByID := make(map[uuid.UUID]LockedProduct, len(locked))
	for _, l := range locked {
		lockedByID[l.id] = l
	}

	seen := make(map[uuid.UUID]struct{}, len(inputs))
	details := make([]PurchaseDetail, 0, len(inputs))
	subtotalDollars := decimal.FromInt(0)
	for _, in := range inputs {
		l, err := resolveLocked(in, lockedByID, seen)
		if err != nil {
			return nil, decimal.FromInt(0), err
		}
		seen[in.ProductID] = struct{}{}

		details = append(details, PurchaseDetail{
			id:        in.ID,
			productID: in.ProductID,
			quantity:  in.Quantity,
			unitPrice: l.price,
		})
		subtotalDollars = subtotalDollars.Add(l.price.Decimal().Mul(decimal.FromInt(int64(in.Quantity))))
	}

	return details, subtotalDollars, nil
}

// resolveLocked は、明細 1 件の入力を検証し、対応するロック済み商品を返します。
// seen は既出の商品 ID の集合で、呼び出し元が返却後に in.ProductID を追加します。
func resolveLocked(
	in DetailInput,
	lockedByID map[uuid.UUID]LockedProduct,
	seen map[uuid.UUID]struct{},
) (LockedProduct, error) {
	if in.ID.IsNil() {
		return LockedProduct{}, xerrors.Wrap(ErrInvalidID, "detail id is required")
	}
	if in.Quantity < minQuantity {
		return LockedProduct{}, xerrors.Wrap(
			ErrInvalidQuantity, fmt.Sprintf("quantity must be %d or greater, got %d", minQuantity, in.Quantity),
		)
	}
	if _, dup := seen[in.ProductID]; dup {
		return LockedProduct{}, xerrors.Wrap(
			ErrDuplicateProductID, fmt.Sprintf("product %s appears more than once", in.ProductID),
		)
	}

	l, ok := lockedByID[in.ProductID]
	if !ok {
		return LockedProduct{}, xerrors.Wrap(ErrProductNotFound, fmt.Sprintf("product %s is not available", in.ProductID))
	}
	if in.Quantity > l.quantity {
		return LockedProduct{}, xerrors.Wrap(
			ErrInsufficientStock,
			fmt.Sprintf("product %s: requested %d, in stock %d", in.ProductID, in.Quantity, l.quantity),
		)
	}

	return l, nil
}

// New は、購入明細の入力とロック済み在庫から購入集約を生成します。
// 明細が空、同一 productID が重複、数量が最小値未満、明細に対応するロック済み商品が無い場合はそれぞれ
// 検証エラー（422）を返し、要求数量がロック済み在庫を超える場合は ErrInsufficientStock（409）を返します。
// 単価は対応するロック済み商品の価格スナップショットとし、金額（小計・税・送料・合計）を整数で計算します
// （税・送料の丸めは切り捨てで本メソッド内 1 箇所に集約）。ステータスは「未処理」で生成します。
func New(
	id uuid.UUID,
	code string,
	userID uuid.UUID,
	inputs []DetailInput,
	locked []LockedProduct,
) (*Purchase, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if code == "" {
		return nil, xerrors.Wrap(ErrInvalidCode, "code is required")
	}
	if userID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidUserID, "userID is required")
	}
	if len(inputs) == 0 {
		return nil, ErrEmptyDetails
	}

	details, subtotalDollars, err := buildDetails(inputs, locked)
	if err != nil {
		return nil, err
	}

	// 価格スケール→決済スケールの丸め（切り捨て）。以降は整数セントで計算する（ADR-0038 (two-scale-quantity-model)）。
	subtotalCents, err := subtotalDollars.Truncate(minorUnitDigits).ToScaledInt64(minorUnitDigits)
	if err != nil {
		return nil, xerrors.Join(ErrInvalidAmount, xerrors.Wrap(err, "subtotal exceeds the settlement range"))
	}
	if subtotalCents > maxSubtotalCents {
		return nil, xerrors.Wrap(
			ErrInvalidAmount,
			fmt.Sprintf("subtotal %d exceeds the maximum that can carry tax and shipping (%d)", subtotalCents, maxSubtotalCents),
		)
	}
	subtotal := int(subtotalCents)
	tax, shipping, total := settle(settlementBasis{Subtotal: subtotal, Discount: 0})

	return &Purchase{
		id:             id,
		code:           code,
		userID:         userID,
		status:         StatusUnprocessed,
		subtotalAmount: subtotal,
		taxAmount:      tax,
		shippingFee:    shipping,
		totalAmount:    total,
		details:        details,
	}, nil
}

// settle は、小計と値引き額から税・送料・合計を決めます。課税の基礎は値引き後の額です
// （docs/spec/domain/purchase.md の Cross-field Invariants）。生成時と値引きの適用時が同じ関数を
// 通るので、両者で式がずれません。
func settle(basis settlementBasis) (int, int, int) {
	taxable := basis.Subtotal - basis.Discount
	tax := taxable * taxRatePercent / percentDivisor
	shipping := shippingFeeCents
	total := taxable + tax + shipping

	return tax, shipping, total
}

// Reconstruct は、永続化済みの購入を再構築します（Repository の読み出し・書き込み後の再検証で使用）。
// attrs.StatusCode は購入ステータスマスタで解決した現在状態、PaidAt / CanceledAt / ShippedAt / DeliveredAt
// は各イベントの発生日時（未発生は nil）です。ID / Code / UserID / StatusID が nil、StatusCode が不正、
// 金額が負、明細が空の場合は検証エラーを返します。StatusCode と timestamps の組み合わせが状態遷移では
// 到達し得ないもの（発送後のキャンセル、発送を伴わない配達 等）の場合も ErrInvalidStatusID を返します。
// 明細は New と同じ不変条件（ID / 商品 ID が設定済み、数量が最小値以上、同一商品 ID が重複しない）を課します。
func Reconstruct(id uuid.UUID, attrs Attributes) (*Purchase, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if attrs.Code == "" {
		return nil, xerrors.Wrap(ErrInvalidCode, "code is required")
	}
	if attrs.UserID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidUserID, "userID is required")
	}
	if attrs.StatusID.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidStatusID, "statusID is required")
	}
	status, err := NewStatus(attrs.StatusCode)
	if err != nil {
		return nil, err
	}
	if err := validateStatusTimestamps(
		status, attrs.PaidAt, attrs.CanceledAt, attrs.ShippedAt, attrs.DeliveredAt,
	); err != nil {
		return nil, err
	}
	if attrs.SubtotalAmount < 0 || attrs.TaxAmount < 0 || attrs.ShippingFee < 0 || attrs.TotalAmount < 0 {
		return nil, xerrors.Wrap(ErrInvalidAmount, "amounts must not be negative")
	}
	if err := validateDiscount(attrs.CouponID, settlementBasis{Subtotal: attrs.SubtotalAmount, Discount: attrs.DiscountAmount}); err != nil {
		return nil, err
	}
	if len(attrs.Details) == 0 {
		return nil, ErrEmptyDetails
	}

	copied := make([]PurchaseDetail, len(attrs.Details))
	copy(copied, attrs.Details)
	if err := validateDetails(copied); err != nil {
		return nil, err
	}

	return &Purchase{
		id:             id,
		code:           attrs.Code,
		userID:         attrs.UserID,
		statusID:       attrs.StatusID,
		status:         status,
		subtotalAmount: attrs.SubtotalAmount,
		couponID:       ptr.Copy(attrs.CouponID),
		discountAmount: attrs.DiscountAmount,
		taxAmount:      attrs.TaxAmount,
		shippingFee:    attrs.ShippingFee,
		totalAmount:    attrs.TotalAmount,
		details:        copied,
		orderedAt:      attrs.OrderedAt,
		paidAt:         ptr.Copy(attrs.PaidAt),
		canceledAt:     ptr.Copy(attrs.CanceledAt),
		shippedAt:      ptr.Copy(attrs.ShippedAt),
		deliveredAt:    ptr.Copy(attrs.DeliveredAt),
	}, nil
}

// validateDiscount は、クーポンの適用と値引き額の対応を検証します。
//
// クーポンを適用したことと値引きが立っていることは常に一致します
// （docs/spec/domain/purchase.md の Cross-field Invariants）。値引きは小計を超えません。
func validateDiscount(couponID *uuid.UUID, basis settlementBasis) error {
	if basis.Discount < 0 {
		return xerrors.Wrap(ErrInvalidAmount, "discountAmount must not be negative")
	}
	if (couponID != nil) != (basis.Discount > 0) {
		return apperror.WithDetails(
			xerrors.Wrap(ErrZeroDiscount, "couponID and a positive discountAmount must be set together"),
			FieldCouponID,
		)
	}
	if basis.Discount > basis.Subtotal {
		return xerrors.Wrap(ErrInvalidAmount, "discountAmount must not exceed the subtotal")
	}

	return nil
}

// ApplyCoupon は、クーポンの値引きを購入へ適用し、税と合計を計算し直します。
//
// 値引きが 0 以下の場合は ErrZeroDiscount（details に couponID）を返します。クーポンを消費して
// 何も引かれない確定を作らないためで、対象の明細が 1 件も無い場合と、定率の値引きが最小単位に
// 満たない場合がこれに当たります。
// 既に適用済みの購入への再適用は ErrCouponAlreadyApplied を返します。1 回の購入に適用できる
// クーポンは高々 1 枚です。
func (p *Purchase) ApplyCoupon(couponID uuid.UUID, discountAmount int) error {
	if couponID.IsNil() {
		return xerrors.Wrap(ErrInvalidCouponID, "couponID is required")
	}
	if p.couponID != nil {
		return ErrCouponAlreadyApplied
	}
	if discountAmount <= 0 {
		return apperror.WithDetails(
			xerrors.Wrap(ErrZeroDiscount, "the coupon does not reduce this purchase"),
			FieldCouponID,
		)
	}
	if err := validateDiscount(&couponID, settlementBasis{Subtotal: p.subtotalAmount, Discount: discountAmount}); err != nil {
		return err
	}

	p.couponID = &couponID
	p.discountAmount = discountAmount
	p.taxAmount, p.shippingFee, p.totalAmount = settle(settlementBasis{Subtotal: p.subtotalAmount, Discount: discountAmount})

	return nil
}

// CouponID は、適用したクーポンの ID を返します。未適用の場合は nil です。
func (p *Purchase) CouponID() *uuid.UUID { return ptr.Copy(p.couponID) }

// DiscountAmount は、値引き額を返します。未適用の場合は 0 です。
func (p *Purchase) DiscountAmount() int { return p.discountAmount }

// validateStatusTimestamps は、statusCode と各イベント日時の組み合わせの到達可能性を検証します
// （個々の理由は各分岐のコメント、契約は Reconstruct を参照）。
func validateStatusTimestamps(status Status, paidAt, canceledAt, shippedAt, deliveredAt *time.Time) error {
	// キャンセルと canceledAt は同時セット（双条件。docs/spec/domain/purchase.md Cancel invariants）。
	if (status == StatusCanceled) != (canceledAt != nil) {
		return xerrors.Wrap(ErrInvalidStatusID, "canceled status and canceledAt must be consistent")
	}
	// 支払い済みは paidAt 必須（一方向。docs/spec/domain/purchase.md Pay invariants）。
	if status == StatusPaid && paidAt == nil {
		return xerrors.Wrap(ErrInvalidStatusID, "paid status requires paidAt")
	}
	// 発送済みは shippedAt 必須（一方向。docs/spec/domain/purchase.md Ship invariants）。
	if status == StatusShipped && shippedAt == nil {
		return xerrors.Wrap(ErrInvalidStatusID, "shipped status requires shippedAt")
	}
	// キャンセルは発送前にしか行えない（Cancel が発送済み・配達済みを拒否する）ため、キャンセル status と
	// 発送の記録は同居しない。支払い後のキャンセルは正常なので paidAt は対象外。
	if status == StatusCanceled && shippedAt != nil {
		return xerrors.Wrap(ErrInvalidStatusID, "canceled status must not have shippedAt")
	}
	// 発送済みへは支払い済みからのみ遷移し、支払い済みは paidAt を必須とするため、paidAt を欠く発送済みは
	// 到達不能な状態である。
	if status == StatusShipped && paidAt == nil {
		return xerrors.Wrap(ErrInvalidStatusID, "shipped status requires paidAt")
	}
	// 配達は発送済みからのみ到達するため、shippedAt を欠く deliveredAt は到達不能な状態である。
	if deliveredAt != nil && shippedAt == nil {
		return xerrors.Wrap(ErrInvalidStatusID, "deliveredAt requires shippedAt")
	}
	// 配達済みと deliveredAt は同時セット（双条件。docs/spec/domain/purchase.md Deliver invariants）。
	if (status == StatusDelivered) != (deliveredAt != nil) {
		return xerrors.Wrap(ErrInvalidStatusID, "delivered status and deliveredAt must be consistent")
	}
	return nil
}

// ID は、商品 ID を返します。
func (l LockedProduct) ID() uuid.UUID { return l.id }

// Price は、単価（価格スケール・ドル decimal）を返します。
func (l LockedProduct) Price() money.Price { return l.price }

// Quantity は、ロック時点の在庫数を返します。
func (l LockedProduct) Quantity() int { return l.quantity }

// ID は、明細 ID を返します。
func (d PurchaseDetail) ID() uuid.UUID { return d.id }

// ProductID は、商品 ID を返します。
func (d PurchaseDetail) ProductID() uuid.UUID { return d.productID }

// Quantity は、購入数量を返します。
func (d PurchaseDetail) Quantity() int { return d.quantity }

// UnitPrice は、単価スナップショット（価格スケール・ドル decimal）を返します。
func (d PurchaseDetail) UnitPrice() money.Price { return d.unitPrice }

// LineTotal は、明細の小計（単価 × 数量）を価格スケールの十進量で返します。丸めません。
// 決済スケールへの丸めは、購入の小計と値引き額がそれぞれ 1 箇所で行います
// （ADR-0038 (two-scale-quantity-model)）。
func (d PurchaseDetail) LineTotal() decimal.Decimal {
	return d.unitPrice.Decimal().Mul(decimal.FromInt(int64(d.quantity)))
}

// ID は、購入 ID を返します。
func (p *Purchase) ID() uuid.UUID { return p.id }

// Code は、購入コード（UUIDv7 文字列）を返します。
func (p *Purchase) Code() string { return p.code }

// UserID は、購入したユーザーの ID を返します。
func (p *Purchase) UserID() uuid.UUID { return p.userID }

// StatusID は、購入ステータス ID を返します。New で生成した集約ではゼロ値で、再構築時に設定されます。
func (p *Purchase) StatusID() uuid.UUID { return p.statusID }

// Status は、購入のステータスを返します。New で生成した集約は未処理、再構築時は購入ステータス
// マスタで解決した現在状態です。名前・終端性・遷移可否はこの値に問い合わせます。
func (p *Purchase) Status() Status { return p.status }

// StatusCode は、永続化と外部公開に用いる業務キーを返します。
func (p *Purchase) StatusCode() int { return p.status.Code() }

// SubtotalAmount は、小計（USD セント）を返します。
func (p *Purchase) SubtotalAmount() int { return p.subtotalAmount }

// TaxAmount は、税額（USD セント）を返します。
func (p *Purchase) TaxAmount() int { return p.taxAmount }

// ShippingFee は、送料（USD セント）を返します。
func (p *Purchase) ShippingFee() int { return p.shippingFee }

// TotalAmount は、合計（USD セント）を返します。
func (p *Purchase) TotalAmount() int { return p.totalAmount }

// OrderedAt は、注文日時を返します。New で生成した集約ではゼロ値で、再構築時に設定されます。
func (p *Purchase) OrderedAt() time.Time { return p.orderedAt }

// PaidAt は、支払い日時を返します。未支払いの場合は nil です。
func (p *Purchase) PaidAt() *time.Time { return ptr.Copy(p.paidAt) }

// CanceledAt は、キャンセル日時を返します。未キャンセルの場合は nil です。
func (p *Purchase) CanceledAt() *time.Time { return ptr.Copy(p.canceledAt) }

// IsCanceled は、購入がキャンセル済みかどうかを返します。
//
// 判定に用いるのは source of truth である statusCode のみで、canceledAt は見ません（読み取り経路の
// SQL 述語との関係は internal/domain/README.md § Query and Aggregate）。
func (p *Purchase) IsCanceled() bool { return p.status == StatusCanceled }

// ShippedAt は、発送日時を返します。未発送の場合は nil です。
func (p *Purchase) ShippedAt() *time.Time { return ptr.Copy(p.shippedAt) }

// DeliveredAt は、配達日時を返します。未配達の場合は nil です。
func (p *Purchase) DeliveredAt() *time.Time { return ptr.Copy(p.deliveredAt) }

// IsShippable は、購入が発送可能な状態にあるかどうかを返します。
//
// 可否は Status.CanTransitionTo から導出します（読み取り経路の SQL 述語との関係は
// internal/domain/README.md § Query and Aggregate）。
func (p *Purchase) IsShippable() bool { return p.status.CanTransitionTo(StatusShipped) }

// Cancel は、購入をキャンセル状態へ遷移させます。キャンセル可能状態（未処理 / 受付中 / 確認中 / 処理中 /
// 支払い済み）からのみ遷移でき、statusCode をキャンセル（6）へ、canceledAt を now へ同時に更新します。
// 既にキャンセル済みなら ErrAlreadyCanceled、完了・配達済み・発送済み（shippedAt）なら
// ErrCancelNotAllowed をそれぞれ返します（いずれも 409）。
// 遷移に成功したときだけキャンセルの事実（Event）を返します。
func (p *Purchase) Cancel(now time.Time) (Event, error) {
	if p.IsCanceled() {
		return Event{}, ErrAlreadyCanceled
	}
	// 配達済みも deliveredAt と同値なので status で判定する。発送済みは status が配達済みへ進んでも
	// 遷移不可であり続けるため、status ではなく残り続ける shippedAt で判定する。
	if !p.status.CanTransitionTo(StatusCanceled) || p.shippedAt != nil {
		return Event{}, ErrCancelNotAllowed
	}
	p.status = StatusCanceled
	p.canceledAt = &now
	return newEvent(EventCanceled, p.id, now), nil
}

// Pay は、購入を支払い済み状態へ遷移させます。未払い相当（未処理 / 受付中 / 確認中）からのみ遷移でき、
// statusCode を支払い済み（7）へ、paidAt を now へ同時に更新します。支払い済み、または未払い相当でも
// paidAt が記録済みなら ErrAlreadyPaid、処理中・キャンセル済み・完了・配達済み・発送済み（shippedAt）なら
// ErrPayNotAllowed をそれぞれ返します（いずれも 409）。決済 SDK / PSP 連携は行わず、paidAt とステータスの
// 記録のみを担う擬似決済です。遷移に成功したときだけ支払いの事実（Event）を返します。
func (p *Purchase) Pay(now time.Time) (Event, error) {
	if p.status == StatusPaid {
		return Event{}, ErrAlreadyPaid
	}
	// status と shippedAt を併用する理由は Cancel を参照。
	if !p.status.CanTransitionTo(StatusPaid) || p.shippedAt != nil {
		return Event{}, ErrPayNotAllowed
	}
	// 記録済みの paidAt を上書きしないためのガード（二重支払い防止。docs/spec/domain/purchase.md Pay invariants）。
	if p.paidAt != nil {
		return Event{}, ErrAlreadyPaid
	}
	p.status = StatusPaid
	p.paidAt = &now
	return newEvent(EventPaid, p.id, now), nil
}

// Ship は、購入を発送済み状態へ遷移させます。支払い済みからのみ遷移でき、statusCode を発送済み（8）へ、
// shippedAt を now へ同時に更新します。既に発送済みなら ErrAlreadyShipped、それ以外の状態
// （未払い相当・処理中・完了・キャンセル済み・配達済み）なら ErrShipNotAllowed をそれぞれ返します（いずれも 409）。
// 配送追跡（追跡番号 / 配送業者）は扱わず、shippedAt とステータスの記録のみを担います。
// 遷移に成功したときだけ発送の事実（Event）を返します。
func (p *Purchase) Ship(now time.Time) (Event, error) {
	if p.status == StatusShipped {
		return Event{}, ErrAlreadyShipped
	}
	if !p.status.CanTransitionTo(StatusShipped) {
		return Event{}, ErrShipNotAllowed
	}
	p.status = StatusShipped
	p.shippedAt = &now
	return newEvent(EventShipped, p.id, now), nil
}

// Deliver は、購入を配達済み状態へ遷移させます。発送済みからのみ遷移でき、statusCode を配達済み（9）へ、
// deliveredAt を now へ同時に更新します。既に配達済みなら ErrAlreadyDelivered、それ以外の状態
// （未払い相当・処理中・支払い済み・完了・キャンセル済み）なら ErrDeliverNotAllowed をそれぞれ返します（いずれも 409）。
// 配達確認の証跡（署名 / 受領写真 / GPS 位置）は扱わず、deliveredAt とステータスの記録のみを担います。
// 遷移に成功したときだけ配達の事実（Event）を返します。
func (p *Purchase) Deliver(now time.Time) (Event, error) {
	if p.status == StatusDelivered {
		return Event{}, ErrAlreadyDelivered
	}
	if !p.status.CanTransitionTo(StatusDelivered) {
		return Event{}, ErrDeliverNotAllowed
	}
	p.status = StatusDelivered
	p.deliveredAt = &now
	return newEvent(EventDelivered, p.id, now), nil
}

// Details は、購入明細のスライスを返します（内部スライスの変更を防ぐためコピーを返します）。
func (p *Purchase) Details() []PurchaseDetail {
	copied := make([]PurchaseDetail, len(p.details))
	copy(copied, p.details)
	return copied
}
