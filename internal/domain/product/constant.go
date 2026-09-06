package product

import "math"

const (
	minNameLength = 1
	maxNameLength = 255
	minQuantity   = 0
	// maxQuantity は、在庫数の上限です。在庫は 32bit 整数幅で表現するため、
	// 増減の結果がこれを超える在庫は保持できません。
	maxQuantity  = math.MaxInt32
	minThreshold = 0
	// maxThreshold は、在庫警告閾値の上限です。閾値は在庫数と突き合わせる値であり、在庫数と同じ
	// 32bit 整数幅で表現します。
	maxThreshold = math.MaxInt32
	// maxImages は、1 商品が保持できる画像の枚数の上限です。
	// placeholder の根拠は docs/spec/domain/product.md の Cross-field Invariants を参照してください。
	maxImages = 20
	// minImageDisplaySort は、商品画像の表示順の下限です。表示順は 1 から数えます。
	minImageDisplaySort = 1
	// maxImageDisplaySort は、商品画像の表示順の上限です。表示順は 16bit 整数幅で表現します。
	maxImageDisplaySort = math.MaxInt16
	// minRefNameLength / maxRefNameLength は、ステータス／カテゴリ参照が保持する名称の長さ範囲です。
	// 参照元マスタ（商品ステータスマスタ／商品カテゴリマスタ）の名称長制約に揃えています。
	minRefNameLength = 1
	maxRefNameLength = 100
	// initialVersion は、生成直後の商品が持つ楽観ロックのバージョンです。
	initialVersion = 1
)

// 商品フィールドの識別子。API リクエストのプロパティ名と一致させ、どの項目が不正かを呼び出し側へ返します。
const (
	// FieldName は、商品名フィールドの識別子です。
	FieldName = "name"
	// FieldPrice は、価格フィールドの識別子です。値を検証するのは横断語彙の
	// [go-boilerplate/internal/domain/lexicon/money.NewPrice] で、そちらはどのリクエストの
	// どの項目として渡されたかを知りません。付けるのは呼び出し元です。
	FieldPrice = "price"
	// FieldQuantity は、在庫数フィールドの識別子です。
	FieldQuantity = "quantity"
	// FieldStockWarningThreshold は、在庫警告閾値フィールドの識別子です。
	FieldStockWarningThreshold = "stockWarningThreshold"
	// FieldCategoryID は、カテゴリフィールドの識別子です。
	FieldCategoryID = "categoryId"
	// FieldStatusID は、ステータスフィールドの識別子です。
	FieldStatusID = "statusId"
	// FieldImages は、画像フィールドの識別子です。画像は集合ごと置き換わるため、要素ではなく
	// 集合を指します。
	FieldImages = "images"
	// FieldDelta は、在庫増減フィールドの識別子です。増減後の在庫を検証するのは
	// [Product.AdjustStock] ですが、クライアントが送るのは増減量なので、範囲を外したときに
	// 名指しできるのはこちらです。
	FieldDelta = "delta"
	// FieldPublishedAt は、公開日時フィールドの識別子です。
	FieldPublishedAt = "publishedAt"
	// FieldDiscontinuedAt は、廃番日時フィールドの識別子です。
	FieldDiscontinuedAt = "discontinuedAt"
)
