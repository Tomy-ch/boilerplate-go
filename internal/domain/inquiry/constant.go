package inquiry

const (
	// minBodyLength は、メッセージ本文の最小文字数（rune 数）です。空の投稿を認めません。
	minBodyLength = 1

	// maxBodyLength は、メッセージ本文の最大文字数（rune 数）です。
	// 上限の根拠は docs/spec/domain/inquiry.md の Notes（placeholder 定数）。
	maxBodyLength = 4000
)

// 問い合わせフィールドの識別子。API リクエストのプロパティ名と一致させ、どの項目が不正かを呼び出し側へ返します。
const (
	// FieldBody は、メッセージ本文フィールドの識別子です。
	FieldBody = "body"
)
