package module

import (
	useridentity "go-boilerplate/internal/infrastructure/auth/useridentity"

	"go.uber.org/fx"
)

// identityModule は、外部アイデンティティの結び付け（IdentityRegistrar）の依存を提供する fx.Module です。
// IdentityRegistrar は登録ユースケースから参照されるため、usecase に依存を供給する
// InfrastructureModule の一部として提供します。認証ミドルウェアだけが使う IdentityResolver は
// server 限定の core.AuthnModule 側に残ります。
func identityModule() fx.Option {
	return fx.Module("identity",
		fx.Provide(
			useridentity.NewRegistrar,
		),
	)
}
