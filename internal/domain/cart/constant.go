package cart

const (
	// minQuantity は、明細 1 件あたりの最小数量です。0 は削除ではなく不正な数量として扱います。
	minQuantity = 1

	// maxQuantityPerItem は、明細 1 件あたりの最大数量です。マージによる数量合算はこの値でクランプされます。
	maxQuantityPerItem = 99

	// maxItems は、1 カートが保持できる明細数の上限です。
	maxItems = 50

	// subtotalMinorUnitDigits は、小計を決済スケール（USD セント）へ落とすときの小数桁数です。
	subtotalMinorUnitDigits = 2

	// sessionTokenLength は、ゲストセッショントークンの長さです。
	// 256 ビットを base64url（パディング無し）で表現した長さに一致します。
	sessionTokenLength = 43
)

// カートフィールドの識別子。API リクエストのプロパティ名と一致させ、どの項目が不正かを呼び出し側へ返します。
const (
	// FieldQuantity は、数量フィールドの識別子です。
	FieldQuantity = "quantity"
)
