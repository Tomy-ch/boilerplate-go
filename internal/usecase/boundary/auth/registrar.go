//go:generate mockgen -source=$GOFILE -destination=mock/mock_$GOFILE.gen.go -package=mock_$GOPACKAGE
package auth

import (
	"context"

	"go-boilerplate/pkg/uuid"
)

// IdentityRegistrar は、認証済みの外部アイデンティティ（issuer + subject）を内部ユーザーへ結び付ける
// インターフェースです。IdentityResolver が読む対応関係を、こちらが作ります。
type IdentityRegistrar interface {
	// Register は、issuer と subject の組を userID へ結び付けます。
	// 呼び出し元のトランザクションがあればそれに参加します。
	// 同じ組が既に結び付いている場合は apperror.ErrConflict を返します。
	// 在籍しない userID への結び付けは apperror.ErrInvalidArgument を返します。
	Register(ctx context.Context, userID uuid.UUID, issuer, subject string) error
}
