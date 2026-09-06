package module

import (
	productdiscontinuecs "go-boilerplate/internal/infrastructure/rdb/command_service/product"     // sample-api:line
	dashboardqs "go-boilerplate/internal/infrastructure/rdb/query_service/dashboard"              // sample-api:line
	productimageqs "go-boilerplate/internal/infrastructure/rdb/query_service/product"             // sample-api:line
	productrankingqs "go-boilerplate/internal/infrastructure/rdb/query_service/product/ranking"   // sample-api:line
	purchasedetailqs "go-boilerplate/internal/infrastructure/rdb/query_service/purchase"          // sample-api:line
	purchasefeedqs "go-boilerplate/internal/infrastructure/rdb/query_service/purchase/feed"       // sample-api:line
	purchasesummaryqs "go-boilerplate/internal/infrastructure/rdb/query_service/purchase/summary" // sample-api:line
	cartrepo "go-boilerplate/internal/infrastructure/rdb/repository/cart"                         // sample-api:line
	couponrepo "go-boilerplate/internal/infrastructure/rdb/repository/coupon"                     // sample-api:line
	inquiryrepo "go-boilerplate/internal/infrastructure/rdb/repository/inquiry"                   // sample-api:line
	"go-boilerplate/internal/infrastructure/rdb/repository/prefecture"                            // sample-api:line
	"go-boilerplate/internal/infrastructure/rdb/repository/product"                               // sample-api:line
	productcategory "go-boilerplate/internal/infrastructure/rdb/repository/product_category"      // sample-api:line
	productstatusrepo "go-boilerplate/internal/infrastructure/rdb/repository/productstatus"       // sample-api:line
	purchaserepo "go-boilerplate/internal/infrastructure/rdb/repository/purchase"                 // sample-api:line
	purchasestatusrepo "go-boilerplate/internal/infrastructure/rdb/repository/purchasestatus"     // sample-api:line
	"go-boilerplate/internal/infrastructure/rdb/repository/user"                                  // sample-api:line
	"go-boilerplate/internal/infrastructure/rdb/system_cqrs/healthcheck"
	idempotencysq "go-boilerplate/internal/infrastructure/rdb/system_cqrs/idempotency"
	outboxsq "go-boilerplate/internal/infrastructure/rdb/system_cqrs/outbox"
	realtimesq "go-boilerplate/internal/infrastructure/rdb/system_cqrs/realtime"

	"go.uber.org/fx"
)

// persistenceModule は、RDB 背後の永続化系依存（repository / query_service / command_service /
// system_cqrs）を提供する fx.Module です。詳細は internal/di/module/README.md § Design Policy を参照。
func persistenceModule() fx.Option {
	return fx.Module("persistence",
		fx.Module("repository",
			fx.Provide(
				// sample-api:begin
				user.New,
				user.NewRoleRepository,
				user.NewLockRepository,
				prefecture.New,
				productstatusrepo.New,
				productcategory.New,
				product.New,
				purchaserepo.New,
				purchasestatusrepo.New,
				cartrepo.New,
				couponrepo.New,  // sample-api:line
				inquiryrepo.New, // sample-api:line
				// sample-api:end
			),
		),
		fx.Module("query_service",
			fx.Provide(
				// sample-api:begin
				// productimageqs: 商品を経由しない画像パスの参照照合（docs/spec/usecase/product.md § SweepOrphans）
				productimageqs.New,
				// productrankingqs: 商品売上ランキング（docs/spec/usecase/product/ranking.md § Overview）
				productrankingqs.New,
				// 同パッケージ: 廃番の影響見積もり（docs/spec/usecase/product.md § 廃番）
				productimageqs.NewDiscontinueImpactQueryService,
				// purchasedetailqs / purchasefeedqs: 集約跨ぎ read 投影（docs/spec/usecase/purchase.md § GET 詳細 / GET 一覧）
				purchasedetailqs.New,
				purchasefeedqs.New,
				// purchasesummaryqs: 認証主体自身の購入集計（docs/spec/usecase/purchase.md § GET 集計）
				purchasesummaryqs.New,
				// dashboardqs: 購入・商品横断の admin 集計（docs/spec/usecase/dashboard.md § Overview）
				dashboardqs.New,
				// sample-api:end
			),
		),
		fx.Module("command_service",
			fx.Provide(
				// sample-api:begin
				// productdiscontinuecs: 廃番に伴う代替クーポンの一括発行。受給者が述語でしか決まらず
				// 分解できないため CommandService に置く（docs/spec/usecase/product.md § 廃番、ADR-0034）
				productdiscontinuecs.New,
				// sample-api:end
			),
		),
		fx.Module("system_cqrs",
			fx.Provide(
				healthcheck.New,
				idempotencysq.New,
				outboxsq.New,
				realtimesq.NewSequenceAllocator,
			),
		),
	)
}
