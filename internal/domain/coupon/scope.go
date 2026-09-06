package coupon

import (
	"fmt"

	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// 既知の適用範囲種別の業務キー（大小は広さの順序を意味しません）。
const (
	scopeKindAll      = 1
	scopeKindCategory = 2
	scopeKindProduct  = 3
)

// 既知の適用範囲種別。
var (
	// ScopeKindAll は、対象を絞らない「全体」です。
	ScopeKindAll = ScopeKind{code: scopeKindAll, name: "all"}
	// ScopeKindCategory は、特定のカテゴリに属する明細だけを対象とする「カテゴリ限定」です。
	ScopeKindCategory = ScopeKind{code: scopeKindCategory, name: "category"}
	// ScopeKindProduct は、特定の商品の明細だけを対象とする「商品限定」です。
	ScopeKindProduct = ScopeKind{code: scopeKindProduct, name: "product"}
)

// ScopeKind は、適用範囲の決まり方を表す値オブジェクトです。code の扱いは [DiscountKind] と同じです。
type ScopeKind struct {
	code int
	name string
}

// Scope は、値引きの対象になる明細の範囲を表す値オブジェクトです。
//
// 対象は商品カテゴリ ID または商品 ID で名指しします。クーポンは商品集約もカテゴリ集約も参照せず、
// 識別子だけを持ちます（集約をまたぐ参照は識別子に限る。internal/domain/README.md の Aggregate Design）。
type Scope struct {
	kind ScopeKind
	// targetID は、カテゴリ限定なら商品カテゴリ ID、商品限定なら商品 ID です。全体では未設定です。
	targetID *uuid.UUID
}

// allScopeKinds は、既知の適用範囲種別一覧です。code からの解決に用います。
func allScopeKinds() []ScopeKind {
	return []ScopeKind{ScopeKindAll, ScopeKindCategory, ScopeKindProduct}
}

// NewScopeKind は、永続化されている code から適用範囲種別を解決します。
// 既知でない code は ErrInvalidScopeKind を返します。
func NewScopeKind(code int) (ScopeKind, error) {
	for _, k := range allScopeKinds() {
		if k.code == code {
			return k, nil
		}
	}
	return ScopeKind{}, xerrors.Wrap(ErrInvalidScopeKind, fmt.Sprintf("unknown scope kind code: %d", code))
}

// Code は、永続化と外部公開に用いる業務キーを返します。
func (k ScopeKind) Code() int { return k.code }

// Name は、適用範囲種別の名前を返します。
func (k ScopeKind) Name() string { return k.name }

// IsZero は、未設定の適用範囲種別かどうかを返します。
func (k ScopeKind) IsZero() bool { return k.code == 0 }

// NewAllScope は、対象を絞らない適用範囲を生成します。
func NewAllScope() Scope { return Scope{kind: ScopeKindAll} }

// NewCategoryScope は、商品カテゴリで絞る適用範囲を生成します。
// categoryID が未設定の場合は ErrInvalidScopeTarget を返します。
func NewCategoryScope(categoryID uuid.UUID) (Scope, error) {
	if categoryID.IsNil() {
		return Scope{}, xerrors.Wrap(ErrInvalidScopeTarget, "categoryID is required for a category scope")
	}
	return Scope{kind: ScopeKindCategory, targetID: &categoryID}, nil
}

// NewProductScope は、商品で絞る適用範囲を生成します。
// productID が未設定の場合は ErrInvalidScopeTarget を返します。
func NewProductScope(productID uuid.UUID) (Scope, error) {
	if productID.IsNil() {
		return Scope{}, xerrors.Wrap(ErrInvalidScopeTarget, "productID is required for a product scope")
	}
	return Scope{kind: ScopeKindProduct, targetID: &productID}, nil
}

// ReconstructScope は、永続化されている種別と対象 ID から適用範囲を再構築します。
// 検証は生成時と同一です。全体の適用範囲は対象 ID を持ってはなりません。
func ReconstructScope(kind ScopeKind, targetID *uuid.UUID) (Scope, error) {
	switch kind {
	case ScopeKindAll:
		if targetID != nil {
			return Scope{}, xerrors.Wrap(ErrInvalidScopeTarget, "an all scope must not have a target")
		}
		return NewAllScope(), nil
	case ScopeKindCategory:
		if targetID == nil {
			return Scope{}, xerrors.Wrap(ErrInvalidScopeTarget, "categoryID is required for a category scope")
		}
		return NewCategoryScope(*targetID)
	case ScopeKindProduct:
		if targetID == nil {
			return Scope{}, xerrors.Wrap(ErrInvalidScopeTarget, "productID is required for a product scope")
		}
		return NewProductScope(*targetID)
	default:
		return Scope{}, xerrors.Wrap(ErrInvalidScopeKind, "scope kind is required")
	}
}

// Kind は、適用範囲の決まり方を返します。
func (s Scope) Kind() ScopeKind { return s.kind }

// TargetID は、範囲を絞る対象の識別子を返します。全体の適用範囲では nil です。
func (s Scope) TargetID() *uuid.UUID { return ptr.Copy(s.targetID) }

// IsZero は、未設定の適用範囲かどうかを返します。
func (s Scope) IsZero() bool { return s.kind.IsZero() }

// Covers は、明細がこの適用範囲に入るかどうかを返します。
//
// 「どの明細が対象か」の定義はこのメソッドが持ちます。全体はすべての明細を対象とし、
// カテゴリ限定は明細の商品カテゴリが、商品限定は明細の商品が、それぞれ対象と一致するときだけ対象です。
// 未設定の適用範囲はどの明細も対象にしません。
func (s Scope) Covers(line Line) bool {
	switch s.kind {
	case ScopeKindAll:
		return true
	case ScopeKindCategory:
		return s.targetID != nil && *s.targetID == line.CategoryID()
	case ScopeKindProduct:
		return s.targetID != nil && *s.targetID == line.ProductID()
	default:
		return false
	}
}
