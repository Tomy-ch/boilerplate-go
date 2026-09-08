package module

import (
	"go.uber.org/fx"
)

// InfrastructureModule は、全プロセス共通のインフラストラクチャ層の依存関係を集約して提供するfx.Moduleです。
// 含む concern は永続化 / clock / HTTP client / webapi gateway / object storage / auth（JWKS profile）/ authz です。
// outbox publisher は relay 専用のため含めず、OutboxRelayModule 側で提供します。
func InfrastructureModule() fx.Option {
	return fx.Module("infrastructure",
		persistenceModule(),
		identityModule(), // sample-api:line
		clockModule(),
		tokenModule(), // sample-api:line
		httpClientModule(),
		webapiModule(),
		objectStorageModule(),
		authModule(),
		authzModule(),
	)
}
