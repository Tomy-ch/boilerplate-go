// Package product は、商品ドメインを定義します。名称・価格・在庫などの不変条件を持つ Product エンティティと、
// 公開商品の cursor 一覧取得を含む Repository インターフェースを提供します。price はサブセント精度を保持する
// 価格スケール（Decimal）の値オブジェクト money.Price で保持します。
package product

import (
	"fmt"
	"slices"
	"time"

	"go-boilerplate/internal/apperror"
	"go-boilerplate/internal/domain/lexicon/money"
	"go-boilerplate/pkg/ptr"
	"go-boilerplate/pkg/stringkit"
	"go-boilerplate/pkg/uuid"
	"go-boilerplate/pkg/xerrors"
)

// Products は、Product エンティティのスライス型です。
type Products []*Product

// Product は、商品を表すドメインエンティティです。
type Product struct {
	id                    uuid.UUID
	name                  string
	description           *string
	price                 money.Price
	quantity              int
	stockWarningThreshold *int
	status                StatusRef
	category              CategoryRef
	publishedAt           *time.Time
	discontinuedAt        *time.Time
	images                []Image
	createdAt             time.Time
	version               int
}

// Attributes は、商品の属性一式です。
// ポインタのフィールドは未設定を nil で表し、不変条件は受け取り側が検証します。
type Attributes struct {
	Name                  string
	Description           *string
	Price                 money.Price
	Quantity              int
	StockWarningThreshold *int
	Status                StatusRef
	Category              CategoryRef
	PublishedAt           *time.Time
	DiscontinuedAt        *time.Time
	Images                []Image
}

// New は、商品エンティティの検証と生成を行います。Price は非負の money.Price（非負検証は Price VO が担保）、
// Quantity と StockWarningThreshold（指定時）は、いずれも在庫が保持できる範囲に収まる必要があります。
// PublishedAt は nil（未公開）を許容し、Images は空（画像未設定）を許容します。
// Images が maxImages を超える場合は ErrTooManyImages を返します。
// id が nil、Name が長さ制約外、Status / Category がゼロ値の場合はそれぞれ検証エラーを返します。
// createdAt は商品が登録された日時で、ゼロ値は検証エラーです。
// 生成直後のバージョンは initialVersion です。
func New(id uuid.UUID, attrs Attributes, createdAt time.Time) (*Product, error) {
	return newProduct(id, attrs, initialVersion, createdAt)
}

// Reconstruct は、永続化済みの商品を再構築します。
// version は永続化されている楽観ロックのバージョンで、initialVersion 未満の場合は検証エラーを返します。
// createdAt は永続化されている登録日時です。その他の検証は New と同一です。
// 保存済みデータのための緩和経路はありません。
func Reconstruct(id uuid.UUID, attrs Attributes, version int, createdAt time.Time) (*Product, error) {
	return newProduct(id, attrs, version, createdAt)
}

// newProduct は、生成・再構築に共通の検証を行い商品エンティティを構築します。
func newProduct(id uuid.UUID, attrs Attributes, version int, createdAt time.Time) (*Product, error) {
	if id.IsNil() {
		return nil, xerrors.Wrap(ErrInvalidID, "id is required")
	}
	if err := validateAttributes(attrs); err != nil {
		return nil, err
	}
	if version < initialVersion {
		return nil, xerrors.Wrap(
			ErrInvalidVersion,
			fmt.Sprintf("version must be %d or greater, got %d", initialVersion, version),
		)
	}
	if createdAt.IsZero() {
		return nil, xerrors.Wrap(ErrInvalidCreatedAt, "createdAt is required")
	}

	return &Product{
		id:                    id,
		name:                  attrs.Name,
		description:           ptr.Copy(attrs.Description),
		price:                 attrs.Price,
		quantity:              attrs.Quantity,
		stockWarningThreshold: ptr.Copy(attrs.StockWarningThreshold),
		status:                attrs.Status,
		category:              attrs.Category,
		publishedAt:           ptr.Copy(attrs.PublishedAt),
		discontinuedAt:        ptr.Copy(attrs.DiscontinuedAt),
		images:                sortImagesByDisplaySort(attrs.Images),
		createdAt:             createdAt,
		version:               version,
	}, nil
}

// validateAttributes は、商品属性の不変条件を検証します。生成時と更新時で同一の条件を課します。
// 最初の違反で打ち切らず、利用者が直せる項目をすべて検証してから束ねて返します
// （internal/domain/README.md の Validation を参照）。
func validateAttributes(attrs Attributes) error {
	var errs []error
	var fields []string

	if ok, msg := stringkit.ValidateInRange(attrs.Name, minNameLength, maxNameLength); !ok {
		errs = append(errs, xerrors.Wrap(ErrInvalidName, msg))
		fields = append(fields, FieldName)
	}
	if err := validateQuantity(int64(attrs.Quantity)); err != nil {
		errs = append(errs, err)
		fields = append(fields, FieldQuantity)
	}
	if err := validateStockWarningThreshold(attrs.StockWarningThreshold); err != nil {
		errs = append(errs, err)
		fields = append(fields, FieldStockWarningThreshold)
	}
	if attrs.Status.id.IsNil() {
		errs = append(errs, xerrors.Wrap(ErrInvalidStatusID, "status is required"))
		fields = append(fields, FieldStatusID)
	}
	if attrs.Category.id.IsNil() {
		errs = append(errs, xerrors.Wrap(ErrInvalidCategoryID, "category is required"))
		fields = append(fields, FieldCategoryID)
	}
	// 違反しているのは片方の項目ではなく組み合わせなので、名指しする項目はここで決まります。
	if err := validateDiscontinuedAt(attrs.DiscontinuedAt, attrs.PublishedAt); err != nil {
		errs = append(errs, err)
		fields = append(fields, FieldPublishedAt)
	}
	if err := validateImages(attrs.Images); err != nil {
		errs = append(errs, err)
		fields = append(fields, FieldImages)
	}

	if len(errs) > 0 {
		return apperror.WithDetails(xerrors.Join(errs...), fields...)
	}
	return nil
}

// Update は、商品の属性を更新します。生成時と同一の不変条件を課し、違反する場合はエンティティを
// 変更せずに検証エラーを返します。attrs は部分更新を解決した後の確定値であり、据え置く属性には
// 現在値が渡されます。Images は集合ごとの置き換えです。バージョンはここでは進めません
// （詳細は docs/spec/domain/product.md の Behavior Methods を参照）。
func (p *Product) Update(attrs Attributes) error {
	if err := validateAttributes(attrs); err != nil {
		return err
	}
	if err := p.ensureDiscontinuationKept(attrs.DiscontinuedAt); err != nil {
		return err
	}

	p.name = attrs.Name
	p.description = ptr.Copy(attrs.Description)
	p.price = attrs.Price
	p.quantity = attrs.Quantity
	p.stockWarningThreshold = ptr.Copy(attrs.StockWarningThreshold)
	p.status = attrs.Status
	p.category = attrs.Category
	p.publishedAt = ptr.Copy(attrs.PublishedAt)
	p.discontinuedAt = ptr.Copy(attrs.DiscontinuedAt)
	p.images = sortImagesByDisplaySort(attrs.Images)

	return nil
}

// AdjustStock は、在庫数を delta の分だけ増減します。delta は正で補充、負で差し引きを表します。
// 増減後の在庫が負になる場合は、在庫を変更せずに検証エラーを返します。
// バージョンの採番タイミングは Update と同じです。
func (p *Product) AdjustStock(delta int) error {
	// 増減の途中結果は在庫の表現範囲を超えうるため、検証を通すまでは広い幅で保持します。
	adjusted := int64(p.quantity) + int64(delta)
	if err := validateQuantity(adjusted); err != nil {
		return err
	}

	p.quantity = int(adjusted)

	return nil
}

// validateQuantity は、在庫数が保持できる範囲に収まることを検証します。
// 生成・更新・在庫の増減のいずれの変更点でも同一の条件を課します。
func validateQuantity(quantity int64) error {
	if quantity < minQuantity || quantity > maxQuantity {
		return xerrors.Wrap(
			ErrInvalidQuantity,
			fmt.Sprintf("quantity must be between %d and %d, got %d", minQuantity, maxQuantity, quantity),
		)
	}
	return nil
}

// validateStockWarningThreshold は、在庫警告閾値が保持できる範囲に収まることを検証します。
// nil は閾値の未設定を表すため検証を通します。
func validateStockWarningThreshold(threshold *int) error {
	if threshold == nil {
		return nil
	}
	if *threshold < minThreshold || *threshold > maxThreshold {
		return xerrors.Wrap(
			ErrInvalidStockWarningThreshold,
			fmt.Sprintf("stockWarningThreshold must be between %d and %d, got %d", minThreshold, maxThreshold, *threshold),
		)
	}
	return nil
}

// EnsureVersion は、更新要求が指すバージョンが現在のバージョンと一致することを確認します。
// 一致しない場合は、読み込み後に他者が更新したものとして ErrVersionConflict を返します。
func (p *Product) EnsureVersion(expected int) error {
	if p.version != expected {
		return xerrors.Wrap(
			ErrVersionConflict,
			fmt.Sprintf("expected version %d, but current version is %d", expected, p.version),
		)
	}
	return nil
}

// ID は、商品の ID を返します。
func (p *Product) ID() uuid.UUID { return p.id }

// Name は、商品名を返します。
func (p *Product) Name() string { return p.name }

// Description は、商品説明を返します。未設定の場合は nil です。
func (p *Product) Description() *string { return ptr.Copy(p.description) }

// Price は、価格（サブセント精度を保持する money.Price）を返します。
func (p *Product) Price() money.Price { return p.price }

// Quantity は、在庫数を返します。
func (p *Product) Quantity() int { return p.quantity }

// StockWarningThreshold は、在庫警告閾値を返します。未設定の場合は nil です。
func (p *Product) StockWarningThreshold() *int { return ptr.Copy(p.stockWarningThreshold) }

// Status は、商品ステータス参照（ID + 名称）を返します。
func (p *Product) Status() StatusRef { return p.status }

// Category は、商品カテゴリ参照（ID + 名称）を返します。
func (p *Product) Category() CategoryRef { return p.category }

// PublishedAt は、公開日時を返します。未公開の場合は nil です。
func (p *Product) PublishedAt() *time.Time { return ptr.Copy(p.publishedAt) }

// DiscontinuedAt は、廃番日時を返します。廃番でない場合は nil です。
func (p *Product) DiscontinuedAt() *time.Time { return ptr.Copy(p.discontinuedAt) }

// CreatedAt は、商品が登録された日時を返します。
func (p *Product) CreatedAt() time.Time { return p.createdAt }

// Images は、商品画像を表示順の昇順で返します。画像未設定の場合は空です。
func (p *Product) Images() []Image { return slices.Clone(p.images) }

// PrimaryImage は、商品を 1 枚で表すときに使う代表画像と、それを持つかどうかを返します。
// 画像を持たない商品は代表画像を持たないため、ok は false です。
// 何を代表とするかは docs/spec/domain/product.md の PrimaryImage が定めます。
func (p *Product) PrimaryImage() (Image, bool) {
	if len(p.images) == 0 {
		return Image{}, false
	}
	return p.images[0], true
}

// Version は、並行更新による上書き（lost update）を防ぐ楽観ロックのバージョンを返します。
func (p *Product) Version() int { return p.version }

// IsPublished は、商品が公開中かどうかを返します。公開中とは公開日時が設定されていることを指します。
// 「公開中」の定義は [IsPublished] が持ちます。SQL の絞り込みとの関係は
// internal/domain/README.md § Query and Aggregate を参照してください。
func (p *Product) IsPublished() bool { return IsPublished(p.publishedAt) }

// IsPublished は、公開日時から商品が公開中かどうかを判定します。
//
// 「公開中」の定義はここ 1 箇所にあります。集約を再構築しない読み取り——集計や射影を返す経路——にも
// 同じ定義を当てられるよう、エンティティのメソッドとは別に値に対する形でも公開します。
func IsPublished(publishedAt *time.Time) bool { return publishedAt != nil }

// Discontinue は、商品を廃番にします。同時に公開を取り下げます。既に廃番の商品は状態が変わらず、
// エラーも返しません（理由は docs/spec/domain/product.md の Cross-field Invariants を参照）。
//
// 日時は引数で受け取ります。ドメインは時刻へ直接依存せず、時刻境界から供給された now を使うためです。
// now がゼロ値の場合は ErrInvalidDiscontinuedAt を返します。
func (p *Product) Discontinue(now time.Time) error {
	if now.IsZero() {
		return xerrors.Wrap(ErrInvalidDiscontinuedAt, "now is required")
	}
	if p.IsDiscontinued() {
		return nil
	}

	p.discontinuedAt = &now
	p.publishedAt = nil

	return nil
}

// IsDiscontinued は、商品が廃番かどうかを返します。廃番とは廃番日時が設定されていることを指します。
func (p *Product) IsDiscontinued() bool { return IsDiscontinued(p.discontinuedAt) }

// IsDiscontinued は、廃番日時から商品が廃番かどうかを判定します。
//
// 「廃番」の定義はここ 1 箇所にあります。値に対する形でも公開する理由は [IsPublished] と同じです。
func IsDiscontinued(discontinuedAt *time.Time) bool { return discontinuedAt != nil }

// validateDiscontinuedAt は、廃番と公開が同時に成り立たないことを検証します。
// 名指しする項目は呼び出し元が決めます。理由は docs/spec/domain/product.md の
// Cross-field Invariants を参照してください。
func validateDiscontinuedAt(discontinuedAt, publishedAt *time.Time) error {
	if IsDiscontinued(discontinuedAt) && IsPublished(publishedAt) {
		return xerrors.Wrap(ErrDiscontinuedCannotBePublished, "publishedAt must be nil for a discontinued product")
	}
	return nil
}

// IsLowStock は、商品の在庫が補充を要する水準まで減っているかどうかを返します。
// 在庫警告閾値が未設定の商品は警告対象を持たないため、常に false です。
//
// 「在庫僅少」の定義はこのメソッドが持ちます（SQL の絞り込みとの関係は IsPublished と同じ）。
func (p *Product) IsLowStock() bool {
	if p.stockWarningThreshold == nil {
		return false
	}
	return p.quantity <= *p.stockWarningThreshold
}

// ensureDiscontinuationKept は、廃番の取り消しを拒みます。属性の一括置換は現在値を見ないため、
// この検証だけがレシーバの現在値と引数を突き合わせます（[Product.EnsureVersion] と同じ形）。
// 拒む理由は docs/spec/domain/product.md の Cross-field Invariants を参照してください。
//
// 未廃番から廃番への向きは拒みません。その向きの検証は [Product.Discontinue] が持ちます。
func (p *Product) ensureDiscontinuationKept(next *time.Time) error {
	if p.IsDiscontinued() && !IsDiscontinued(next) {
		return apperror.WithDetails(
			xerrors.Wrap(ErrDiscontinuationIrreversible, "discontinuedAt must not be cleared"),
			FieldDiscontinuedAt,
		)
	}
	return nil
}
