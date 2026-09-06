// Package cart は、カートドメインを定義します。カート（Cart）集約と明細（CartItem）を持ち、
// 所有者とセッションの排他、同一商品の重複禁止、数量と明細数の上限を不変条件として保持します。
//
// カートは在庫を押さえず、確定単価も持ちません。買うつもりの控えであって、売り越しの禁止も
// 請求額の確定も購入成立時の関心です。
package cart

import (
	"bytes"
	"fmt"
	"slices"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// Carts は、Cart 集約のスライス型です。
type Carts []*Cart

// Cart は、カートを表すドメイン集約です。
// 所有者は後から決まります。ゲストの間は sessionToken で追跡され、ログインによって ownerID が確定し、
// 同時に sessionToken は破棄されます。
type Cart struct {
	id           uuid.UUID
	ownerID      *uuid.UUID
	sessionToken *SessionToken
	items        []CartItem
	expiresAt    time.Time
	createdAt    time.Time
	updatedAt    time.Time
}

// Attributes は、カートの再構築に必要な属性一式です。有効期限と監査時刻は同じ time.Time のため、
// 取り違えを型で防ぐ目的で構造体で受けます
// （docs/rules.md の Function Signature Rules、internal/domain/README.md の
// Bundle attributes into a struct when positional arguments can be swapped）。
type Attributes struct {
	// OwnerID は、確定した所有者です。ゲストの間は nil です。
	OwnerID *uuid.UUID
	// SessionToken は、ゲスト追跡用のトークンです。所有者確定後は nil です。
	SessionToken *SessionToken
	// Items は、明細の集合です。空を許します（空カートは正当な状態）。
	Items []CartItem
	// ExpiresAt は、有効期限です。
	ExpiresAt time.Time
	// CreatedAt は、作成日時です。
	CreatedAt time.Time
	// UpdatedAt は、更新日時です。
	UpdatedAt time.Time
}

// SetItemAttributes は、明細の数量設定に必要な属性一式です。ItemID と ProductID は同じ uuid.UUID の
// ため、取り違えを型で防ぐ目的で構造体で受けます（docs/rules.md の Function Signature Rules）。
type SetItemAttributes struct {
	// ItemID は、明細を新たに追加するときに採番済みの ID です。既存商品の置換では使いません。
	ItemID uuid.UUID
	// ProductID は、対象の商品です。明細の自然キーでもあります。
	ProductID uuid.UUID
	// Quantity は、設定する数量です。
	Quantity int
}

// MergeResult は、マージで失われた分の報告です。永続化しません。
type MergeResult struct {
	clamped []uuid.UUID
	dropped []uuid.UUID
}

// Clamped は、数量合算が上限を超えクランプされた商品 ID を返します。
func (r MergeResult) Clamped() []uuid.UUID { return slices.Clone(r.clamped) }

// Dropped は、明細数の上限により切り捨てられた商品 ID を返します。
func (r MergeResult) Dropped() []uuid.UUID { return slices.Clone(r.dropped) }

// NewForGuest は、所有者が未確定のカートを生成します。明細は空で始まります。
// id が未設定なら ErrInvalidID を、expiresAt がゼロ値なら ErrInvalidExpiresAt を返します。
func NewForGuest(id uuid.UUID, token SessionToken, expiresAt time.Time) (*Cart, error) {
	return newCart(id, Attributes{SessionToken: &token, ExpiresAt: expiresAt})
}

// NewForOwner は、所有者が確定したカートを生成します。
// OwnerID の設定は必須で、SessionToken を併せて設定した場合は ErrInvalidOwner を返します。
// id または OwnerID が未設定なら ErrInvalidID / ErrInvalidUserID を、ExpiresAt がゼロ値なら
// ErrInvalidExpiresAt を返します。
func NewForOwner(id uuid.UUID, attrs Attributes) (*Cart, error) {
	return newCart(id, attrs)
}

// Reconstruct は、永続化済みのカートを再構築します。
// NewForGuest / NewForOwner と同じ不変条件を課します。保存済みデータのための緩和経路はありません。
// 加えて、監査時刻が設定されている場合は expiresAt が createdAt より後であることを要求します。
func Reconstruct(id uuid.UUID, attrs Attributes) (*Cart, error) {
	c, err := newCart(id, attrs)
	if err != nil {
		return nil, err
	}
	if !attrs.CreatedAt.IsZero() && !attrs.ExpiresAt.After(attrs.CreatedAt) {
		return nil, xerrors.Wrap(ErrInvalidExpiresAt, "expiresAt must be after createdAt")
	}
	return c, nil
}

// newCart は、3 つの入口が共有する検証ゲートです。
// 入口ごとに検証が乖離しないよう、不変条件の判定はここ 1 箇所に閉じます。
func newCart(id uuid.UUID, attrs Attributes) (*Cart, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if err := validateOwnership(attrs.OwnerID, attrs.SessionToken); err != nil {
		return nil, err
	}
	if attrs.ExpiresAt.IsZero() {
		return nil, xerrors.Wrap(ErrInvalidExpiresAt, "expiresAt is required")
	}

	copied := make([]CartItem, len(attrs.Items))
	copy(copied, attrs.Items)
	if err := validateItems(copied); err != nil {
		return nil, err
	}

	return &Cart{
		id:           id,
		ownerID:      ptr.Copy(attrs.OwnerID),
		sessionToken: ptr.Copy(attrs.SessionToken),
		items:        copied,
		expiresAt:    attrs.ExpiresAt,
		createdAt:    attrs.CreatedAt,
		updatedAt:    attrs.UpdatedAt,
	}, nil
}

// validateOwnership は、所有者とセッショントークンがちょうど一方だけ設定されていることを検証します。
// どちらの側も構築を許しません（排他の理由: docs/spec/domain/cart.md の Cross-field Invariants）。
func validateOwnership(ownerID *uuid.UUID, token *SessionToken) error {
	if (ownerID == nil) == (token == nil) {
		return ErrInvalidOwner
	}
	if ownerID != nil && ownerID.IsNil() {
		return xerrors.Wrap(ErrInvalidUserID, "ownerID is required")
	}
	if token != nil && token.IsZero() {
		return xerrors.Wrap(ErrInvalidSessionToken, "sessionToken is required")
	}
	return nil
}

// validateItems は、明細の集合が満たすべき不変条件を検証します。
// 明細 ID / 商品 ID が未設定、数量が範囲外、同一商品 ID の重複、明細数の上限超過をそれぞれ検出します。
func validateItems(items []CartItem) error {
	if len(items) > maxItems {
		return xerrors.Wrap(ErrTooManyItems, fmt.Sprintf("items must be %d or fewer, got %d", maxItems, len(items)))
	}
	seen := make(map[uuid.UUID]struct{}, len(items))
	for _, i := range items {
		if i.id.IsNil() {
			return xerrors.Wrap(ErrInvalidID, "item id is required")
		}
		if i.productID.IsNil() {
			return xerrors.Wrap(ErrInvalidProductID, "item product id is required")
		}
		if err := validateQuantity(i.quantity); err != nil {
			return err
		}
		if _, dup := seen[i.productID]; dup {
			return xerrors.Wrap(ErrDuplicateProductID, fmt.Sprintf("product %s appears more than once", i.productID))
		}
		seen[i.productID] = struct{}{}
	}
	return nil
}

// validateQuantity は、数量が許容範囲に収まっているかを検証します。
func validateQuantity(quantity int) error {
	if quantity < minQuantity || quantity > maxQuantityPerItem {
		return xerrors.Wrap(
			ErrInvalidQuantity,
			fmt.Sprintf("quantity must be between %d and %d, got %d", minQuantity, maxQuantityPerItem, quantity),
		)
	}
	return nil
}

// ID は、カート ID を返します。
func (c *Cart) ID() uuid.UUID { return c.id }

// OwnerID は、所有者のユーザー ID を返します。所有者が未確定の場合は nil です。
func (c *Cart) OwnerID() *uuid.UUID { return ptr.Copy(c.ownerID) }

// SessionToken は、ゲスト追跡用のトークンを返します。所有者が確定済みの場合は nil です。
func (c *Cart) SessionToken() *SessionToken { return ptr.Copy(c.sessionToken) }

// Items は、明細のスライスを返します（内部スライスの変更を防ぐためコピーを返します）。
func (c *Cart) Items() []CartItem { return slices.Clone(c.items) }

// ExpiresAt は、有効期限を返します。
func (c *Cart) ExpiresAt() time.Time { return c.expiresAt }

// CreatedAt は、作成日時を返します。生成直後の集約ではゼロ値で、再構築時に設定されます。
func (c *Cart) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt は、更新日時を返します。生成直後の集約ではゼロ値で、再構築時に設定されます。
func (c *Cart) UpdatedAt() time.Time { return c.updatedAt }

// SetItem は、指定商品の数量を設定します。
// 同一商品の明細が既にあれば数量を置換し（addedAt は保持）、無ければ ItemID で追加します。
// 数量が範囲外なら ErrInvalidQuantity を、追加により明細数が上限を超えるなら ErrTooManyItems を返します。
// 数量 0 は削除ではなくエラーです。削除は RemoveItem が担います。
func (c *Cart) SetItem(attrs SetItemAttributes, now time.Time) error {
	if attrs.ProductID.IsNil() {
		return xerrors.Wrap(ErrInvalidProductID, "productID is required")
	}
	// 識別子はここで付けます。validateQuantity は明細の再構築からも呼ばれ、そちらの数量は
	// クライアントが送った項目ではありません。
	if err := validateQuantity(attrs.Quantity); err != nil {
		return apperror.WithDetails(err, FieldQuantity)
	}

	if idx := c.indexOf(attrs.ProductID); idx >= 0 {
		c.items[idx].quantity = attrs.Quantity
		return nil
	}

	if attrs.ItemID.IsNil() {
		return xerrors.Wrap(ErrInvalidID, "itemID is required to add a new item")
	}
	if len(c.items)+1 > maxItems {
		return xerrors.Wrap(ErrTooManyItems, fmt.Sprintf("items must be %d or fewer", maxItems))
	}

	c.items = append(c.items, CartItem{
		id: attrs.ItemID, productID: attrs.ProductID, quantity: attrs.Quantity, addedAt: now,
	})
	return nil
}

// RemoveItem は、指定商品の明細を取り除きます。
// 該当する明細が無い場合も成功を返します（削除は冪等であり、「無かった」と「消した」を区別しません）。
func (c *Cart) RemoveItem(productID uuid.UUID) error {
	if productID.IsNil() {
		return xerrors.Wrap(ErrInvalidProductID, "productID is required")
	}
	if idx := c.indexOf(productID); idx >= 0 {
		c.items = append(c.items[:idx], c.items[idx+1:]...)
	}
	return nil
}

// Clear は、明細をすべて取り除きます。空カートは正当な状態のため、カート自体と有効期限は維持されます。
func (c *Cart) Clear() { c.items = nil }

// Merge は、別のカートの明細を自身へ取り込みます。
//
// 同一商品は数量を合算し、上限を超える場合は上限へクランプします。自身に無い商品は追加し、明細数の
// 上限を超える分は追加が新しいものから切り捨てます（先に入っていたものを優先して残す）。
// 失われた分は戻り値で報告されます。
//
// error を返しません（理由: docs/spec/domain/cart.md の Behavior Methods の Merge）。
// 取り込み元は変更しません。
func (c *Cart) Merge(other *Cart, now time.Time) MergeResult {
	if other == nil {
		return MergeResult{}
	}

	result := MergeResult{}
	incoming := make([]CartItem, 0, len(other.items))
	for _, item := range other.items {
		if idx := c.indexOf(item.productID); idx >= 0 {
			sum := c.items[idx].quantity + item.quantity
			if sum > maxQuantityPerItem {
				sum = maxQuantityPerItem
				result.clamped = append(result.clamped, item.productID)
			}
			c.items[idx].quantity = sum
			continue
		}
		incoming = append(incoming, item)
	}

	// 切り捨ての順序は渡された順ではなく addedAt の値で決める。呼び出し側の並べ方に結果が左右されると、
	// 同じ 2 つのカートをマージしても取り込まれる明細が変わってしまう。
	slices.SortFunc(incoming, compareByAddedAt)

	for _, item := range incoming {
		if len(c.items) >= maxItems {
			result.dropped = append(result.dropped, item.productID)
			continue
		}
		merged := item
		merged.addedAt = now
		c.items = append(c.items, merged)
	}

	return result
}

// Touch は、有効期限を now から ttl だけ先へ延長します。
// 使われている間のカートが掃除の対象にならないようにするためです。
func (c *Cart) Touch(now time.Time, ttl time.Duration) {
	c.expiresAt = now.Add(ttl)
	c.updatedAt = now
}

// MarkSeen は、利用者へ提示した価格を明細へ記録します。
// prices に現れない商品の明細は変更しません（非公開化などで価格を引けなかった場合）。
//
// ここに記録される値は表示の履歴であって約束ではありません
// （詳細: docs/spec/domain/cart.md の Behavior Methods の MarkSeen）。
func (c *Cart) MarkSeen(prices map[uuid.UUID]money.Price) {
	for idx := range c.items {
		price, ok := prices[c.items[idx].productID]
		if !ok {
			continue
		}
		c.items[idx].lastSeenPrice = &price
	}
}

// IsExpired は、有効期限を過ぎているかを返します。
//
// 「期限切れ」の定義はこのメソッドが持ちます。有効期限そのものの時点は期限切れではありません。
// 掃除ジョブの削除条件はこの述語の実行形であって定義ではなく、片方だけを変更してはなりません。
func (c *Cart) IsExpired(now time.Time) bool { return now.After(c.expiresAt) }

// IsEmpty は、明細を 1 件も持たないかを返します。
func (c *Cart) IsEmpty() bool { return len(c.items) == 0 }

// IsOwnedBy は、指定ユーザーが所有者かを返します。所有者が未確定のカートは常に false です。
func (c *Cart) IsOwnedBy(userID uuid.UUID) bool {
	return c.ownerID != nil && *c.ownerID == userID
}

// indexOf は、指定商品の明細の位置を返します。存在しない場合は -1 を返します。
func (c *Cart) indexOf(productID uuid.UUID) int {
	for idx, item := range c.items {
		if item.productID == productID {
			return idx
		}
	}
	return -1
}

// compareByAddedAt は、明細を追加時刻の昇順へ並べるための比較関数です。
// 同時刻は商品 ID のバイト列で決着させ、入力の並び順が結果に漏れないようにします。
func compareByAddedAt(a, b CartItem) int {
	if !a.addedAt.Equal(b.addedAt) {
		return a.addedAt.Compare(b.addedAt)
	}
	aID, bID := a.productID.Bytes(), b.productID.Bytes()
	return bytes.Compare(aID[:], bID[:])
}
