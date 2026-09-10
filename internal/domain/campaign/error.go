package campaign

import (
	"go-boilerplate/internal/apperror"
	"go-boilerplate/pkg/xerrors"
)

var (
	errInvalid = xerrors.Wrap(apperror.ErrValidation, "invalid campaign")
	// ErrInvalidID は、キャンペーン ID の検証に失敗した場合のエラーです。
	ErrInvalidID = xerrors.Wrap(errInvalid, "id failed")
	// ErrInvalidCode は、キャンペーンコードの検証に失敗した場合のエラーです。
	ErrInvalidCode = xerrors.Wrap(errInvalid, "code failed")
	// ErrInvalidTemplate は、配るクーポンのテンプレートが未設定の場合のエラーです。
	ErrInvalidTemplate = xerrors.Wrap(errInvalid, "template is required")
	// ErrInvalidDiscountKind は、テンプレートの値引き種別の検証に失敗した場合のエラーです。
	ErrInvalidDiscountKind = xerrors.Wrap(errInvalid, "discountKind failed")
	// ErrInvalidDiscountValue は、テンプレートの値引きの値の検証に失敗した場合のエラーです。
	ErrInvalidDiscountValue = xerrors.Wrap(errInvalid, "discountValue failed")
	// ErrInvalidMaxAmount は、テンプレートの値引き上限の検証に失敗した場合のエラーです。
	ErrInvalidMaxAmount = xerrors.Wrap(errInvalid, "maxAmount failed")
	// ErrInvalidScopeKind は、テンプレートの適用範囲種別の検証に失敗した場合のエラーです。
	ErrInvalidScopeKind = xerrors.Wrap(errInvalid, "scopeKind failed")
	// ErrInvalidMinPurchaseAmount は、テンプレートの最低購入金額の検証に失敗した場合のエラーです。
	ErrInvalidMinPurchaseAmount = xerrors.Wrap(errInvalid, "minPurchaseAmount failed")
	// ErrInvalidUsableFrom は、テンプレートの利用開始日時の検証に失敗した場合のエラーです。
	ErrInvalidUsableFrom = xerrors.Wrap(errInvalid, "usableFrom failed")
	// ErrInvalidCouponExpiresAt は、テンプレートの有効期限の検証に失敗した場合のエラーです。
	ErrInvalidCouponExpiresAt = xerrors.Wrap(errInvalid, "couponExpiresAt failed")
	// ErrInvalidPeriod は、配布期間の検証に失敗した場合のエラーです。
	ErrInvalidPeriod = xerrors.Wrap(errInvalid, "period failed")
	// ErrInvalidTotalLimit は、総枚数上限の検証に失敗した場合のエラーです。
	ErrInvalidTotalLimit = xerrors.Wrap(errInvalid, "totalLimit failed")
	// ErrInvalidPerUserLimit は、1 人あたり上限の検証に失敗した場合のエラーです。
	ErrInvalidPerUserLimit = xerrors.Wrap(errInvalid, "perUserLimit failed")
	// ErrInvalidIssuedCount は、発行済み枚数の検証に失敗した場合のエラーです。
	ErrInvalidIssuedCount = xerrors.Wrap(errInvalid, "issuedCount failed")
	// ErrInvalidClaimedByUser は、既に受け取った枚数の検証に失敗した場合のエラーです。
	ErrInvalidClaimedByUser = xerrors.Wrap(errInvalid, "claimedByUser failed")
	// ErrInvalidCampaignID は、受け取り記録が指すキャンペーン ID の検証に失敗した場合のエラーです。
	ErrInvalidCampaignID = xerrors.Wrap(errInvalid, "campaignID failed")
	// ErrInvalidUserID は、受け取り記録が指す利用者 ID の検証に失敗した場合のエラーです。
	ErrInvalidUserID = xerrors.Wrap(errInvalid, "userID failed")
	// ErrInvalidCouponID は、受け取り記録が指すクーポン ID の検証に失敗した場合のエラーです。
	ErrInvalidCouponID = xerrors.Wrap(errInvalid, "couponID failed")
	// ErrInvalidClaimedAt は、受け取り日時の検証に失敗した場合のエラーです。
	ErrInvalidClaimedAt = xerrors.Wrap(errInvalid, "claimedAt failed")

	// ErrCodeUnknown は、そのコードのキャンペーンが存在しない場合のエラーです。
	// 永続化層の NotFound をここへ写し替えるのは、応答で「存在しない」を「受け取れない」と
	// 同じ形に畳むためです（理由は docs/spec/usecase/campaign.md の ClaimCoupon を参照）。
	ErrCodeUnknown = xerrors.Wrap(errInvalid, "campaign code is unknown")
	// ErrNotDistributing は、配布期間の外で受け取ろうとした場合のエラーです。
	ErrNotDistributing = xerrors.Wrap(errInvalid, "campaign is not distributing")
	// ErrSuspended は、停止済みのキャンペーンから受け取ろうとした場合のエラーです。
	ErrSuspended = xerrors.Wrap(errInvalid, "campaign is suspended")
	// ErrTotalLimitReached は、総枚数上限に達したキャンペーンから受け取ろうとした場合のエラーです。
	ErrTotalLimitReached = xerrors.Wrap(errInvalid, "campaign reached its total limit")
	// ErrPerUserLimitReached は、1 人あたり上限に達した利用者が受け取ろうとした場合のエラーです。
	ErrPerUserLimitReached = xerrors.Wrap(errInvalid, "user reached the per-user limit")

	// ErrAlreadySuspended は、停止済みのキャンペーンを重ねて停止しようとした場合のエラーです。
	// 停止は取り消せない一方向の遷移であるため、要求の不正ではなく状態の衝突として扱います。
	ErrAlreadySuspended = xerrors.Wrap(apperror.ErrConflict, "campaign is already suspended")
	// ErrIssuedConcurrently は、受け取りの最中に他の書き手が総枚数上限を埋めた場合のエラーです
	// （到達条件は docs/spec/domain/campaign.md の Repository Methods > RecordClaim を参照）。
	ErrIssuedConcurrently = xerrors.Wrap(apperror.ErrConflict, "campaign was exhausted concurrently")
)
