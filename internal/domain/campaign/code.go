package campaign

import (
	"fmt"
	"strings"

	"go-boilerplate/pkg/xerrors"
)

// Code は、利用者がキャンペーンからクーポンを受け取るために示す符号を表す値オブジェクトです。
//
// 大文字・小文字と前後の空白は同じコードとして扱い、正規化は生成時に済ませます
// （理由は docs/spec/domain/campaign.md の Code を参照）。
type Code struct {
	value string
}

// NewCode は、キャンペーンコードを正規化して生成します。前後の空白を落とし、大文字に揃えます。
//
// 正規化したあとの長さが規定の範囲を外れる場合、または英数字とハイフン以外を含む場合は
// ErrInvalidCode を返します。
func NewCode(value string) (Code, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))

	if len(normalized) < codeMinLength || len(normalized) > codeMaxLength {
		return Code{}, xerrors.Wrap(
			ErrInvalidCode,
			fmt.Sprintf("length must be between %d and %d, got %d", codeMinLength, codeMaxLength, len(normalized)),
		)
	}
	for _, r := range normalized {
		if !isCodeChar(r) {
			return Code{}, xerrors.Wrap(
				ErrInvalidCode, fmt.Sprintf("contains a character that is not allowed: %q", r),
			)
		}
	}

	return Code{value: normalized}, nil
}

// isCodeChar は、キャンペーンコードに使える文字かどうかを返します。
// 大文字英数字とハイフンだけを認めます（正規化後の判定であるため小文字は現れません）。
func isCodeChar(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-':
		return true
	default:
		return false
	}
}

// IsZero は、生成を経ていないゼロ値かどうかを返します。
// 複合リテラルは検証を通らずに組み立てられるため、値を受け取る側はこれで拒否できます。
func (c Code) IsZero() bool { return c.value == "" }

// Value は、正規化済みのキャンペーンコードを返します。
func (c Code) Value() string { return c.value }
