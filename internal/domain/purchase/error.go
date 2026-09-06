package purchase

import (
	"go-boilerplate/internal/apperror"
	"go-boilerplate/pkg/xerrors"
)

var (
	errInvalid = xerrors.Wrap(apperror.ErrValidation, "invalid purchase")
	// ErrEmptyDetails は、明細が空の場合のエラーです（422）。
	ErrEmptyDetails = xerrors.Wrap(errInvalid, "details must not be empty")
	// ErrDuplicateProductID は、明細に同一 productID が重複した場合のエラーです（422）。
	ErrDuplicateProductID = xerrors.Wrap(errInvalid, "duplicate product id in details")
	// ErrInvalidQuantity は、購入数量が最小値未満の場合のエラーです（422）。
	ErrInvalidQuantity = xerrors.Wrap(errInvalid, "quantity must be positive")
	// ErrProductNotFound は、明細の productID に対応する（ロック済みの）商品が存在しない場合のエラーです（422）。
	ErrProductNotFound = xerrors.Wrap(errInvalid, "product not found for detail")
	// ErrInvalidID は、購入 ID の検証に失敗した場合のエラーです。
	ErrInvalidID = xerrors.Wrap(errInvalid, "id failed")
	// ErrInvalidCode は、購入コードの検証に失敗した場合のエラーです。
	ErrInvalidCode = xerrors.Wrap(errInvalid, "code failed")
	// ErrInvalidUserID は、ユーザー ID の検証に失敗した場合のエラーです。
	ErrInvalidUserID = xerrors.Wrap(errInvalid, "userID failed")
	// ErrInvalidStatusID は、ステータス ID の検証に失敗した場合のエラーです（再構築時）。
	ErrInvalidStatusID = xerrors.Wrap(errInvalid, "statusID failed")
	// ErrInvalidAmount は、金額の検証に失敗した場合のエラーです（再構築時）。
	ErrInvalidAmount = xerrors.Wrap(errInvalid, "amount failed")

	// 以下は状態の衝突であってリクエストの不正ではないため、apperror.ErrConflict を基底に持ちます
	// （HTTP ステータスへの写像は internal/apperror/README.md の Mapping Table）。

	// ErrInsufficientStock は、在庫不足（売り越し）の場合のエラーです。
	ErrInsufficientStock = xerrors.Wrap(apperror.ErrConflict, "insufficient stock")

	// ErrAlreadyCanceled は、既にキャンセル済みの購入を再度キャンセルしようとした場合のエラーです。
	ErrAlreadyCanceled = xerrors.Wrap(apperror.ErrConflict, "purchase already canceled")

	// ErrCancelNotAllowed は、キャンセル不可の状態（完了・発送済み・配達済み）からキャンセルしようとした場合のエラーです。
	ErrCancelNotAllowed = xerrors.Wrap(apperror.ErrConflict, "purchase cannot be canceled in the current state")

	// ErrAlreadyPaid は、既に支払い済みの購入を再度支払おうとした場合のエラーです（二重支払い）。
	ErrAlreadyPaid = xerrors.Wrap(apperror.ErrConflict, "purchase already paid")

	// ErrPayNotAllowed は、支払い不可の状態（キャンセル済み・完了・発送済み・配達済み）から支払おうとした場合のエラーです。
	ErrPayNotAllowed = xerrors.Wrap(apperror.ErrConflict, "purchase cannot be paid in the current state")

	// ErrAlreadyShipped は、既に発送済みの購入を再度発送しようとした場合のエラーです（二重発送）。
	ErrAlreadyShipped = xerrors.Wrap(apperror.ErrConflict, "purchase already shipped")

	// ErrShipNotAllowed は、発送不可の状態（未払い相当・処理中・完了・キャンセル済み・配達済み）から発送しようとした
	// 場合のエラーです。
	ErrShipNotAllowed = xerrors.Wrap(apperror.ErrConflict, "purchase cannot be shipped in the current state")

	// ErrAlreadyDelivered は、既に配達済みの購入を再度配達完了にしようとした場合のエラーです（二重配達）。
	ErrAlreadyDelivered = xerrors.Wrap(apperror.ErrConflict, "purchase already delivered")

	// ErrDeliverNotAllowed は、配達不可の状態（未払い相当・処理中・支払い済み・完了・キャンセル済み）から配達完了に
	// しようとした場合のエラーです。
	ErrDeliverNotAllowed = xerrors.Wrap(apperror.ErrConflict, "purchase cannot be delivered in the current state")
	// ErrInvalidCouponID は、適用するクーポン ID の検証に失敗した場合のエラーです。
	ErrInvalidCouponID = xerrors.Wrap(errInvalid, "couponID failed")
	// ErrZeroDiscount は、クーポンを適用しても値引きが立たない場合のエラーです。
	// クーポンを消費して何も引かれない確定を作らないために拒みます。
	ErrZeroDiscount = xerrors.Wrap(errInvalid, "the coupon does not reduce this purchase")
	// ErrCouponAlreadyApplied は、既にクーポンを適用した購入へ再適用しようとした場合のエラーです。
	ErrCouponAlreadyApplied = xerrors.Wrap(errInvalid, "a coupon is already applied")
)
