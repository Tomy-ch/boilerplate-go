package module

import (
	"go-boilerplate/internal/config"                                 // sample-api:line
	domainproduct "go-boilerplate/internal/domain/product"           // sample-api:line
	domaincategory "go-boilerplate/internal/domain/product/category" // sample-api:line
	domainstatus "go-boilerplate/internal/domain/product/status"     // sample-api:line
	domainpurchase "go-boilerplate/internal/domain/purchase"         // sample-api:line
	"go-boilerplate/internal/observability"
	addressuc "go-boilerplate/internal/usecase/address"                      // sample-api:line
	authzbd "go-boilerplate/internal/usecase/boundary/authz"                 // sample-api:line
	clockbd "go-boilerplate/internal/usecase/boundary/clock"                 // sample-api:line
	objectstoragebd "go-boilerplate/internal/usecase/boundary/objectstorage" // sample-api:line
	txbd "go-boilerplate/internal/usecase/boundary/tx"                       // sample-api:line
	cartuc "go-boilerplate/internal/usecase/cart"                            // sample-api:line
	checkoutuc "go-boilerplate/internal/usecase/checkout"                    // sample-api:line
	couponuc "go-boilerplate/internal/usecase/coupon"                        // sample-api:line
	dashboarduc "go-boilerplate/internal/usecase/dashboard"                  // sample-api:line
	exchangerateuc "go-boilerplate/internal/usecase/exchangerate"            // sample-api:line
	"go-boilerplate/internal/usecase/healthcheck"
	"go-boilerplate/internal/usecase/idempotency"
	inquiryuc "go-boilerplate/internal/usecase/inquiry" // sample-api:line
	"go-boilerplate/internal/usecase/outbox"
	prefectureuc "go-boilerplate/internal/usecase/prefecture"            // sample-api:line
	productuc "go-boilerplate/internal/usecase/product"                  // sample-api:line
	categoryuc "go-boilerplate/internal/usecase/product/category"        // sample-api:line
	productcommand "go-boilerplate/internal/usecase/product/command"     // sample-api:line
	productquery "go-boilerplate/internal/usecase/product/query"         // sample-api:line
	rankinguc "go-boilerplate/internal/usecase/product/ranking"          // sample-api:line
	statusuc "go-boilerplate/internal/usecase/product/status"            // sample-api:line
	purchaseuc "go-boilerplate/internal/usecase/purchase"                // sample-api:line
	purchasestatusuc "go-boilerplate/internal/usecase/purchase/status"   // sample-api:line
	purchasesummaryuc "go-boilerplate/internal/usecase/purchase/summary" // sample-api:line
	"go-boilerplate/internal/usecase/user"                               // sample-api:line
	userroleuc "go-boilerplate/internal/usecase/user/role"               // sample-api:line
	"go-boilerplate/internal/usecase/user/search"                        // sample-api:line

	"go.uber.org/fx"
)

// readinessProbeGroup は、/ready が状態を並べる依存の検査（[healthcheck.Probe]）の value group です。
// 空でも構いません。入れてよい依存の条件は [healthcheck.Probe] が定めます。
const readinessProbeGroup = "readiness.probes"

// UsecaseModule は、ユースケース層の依存関係を提供するfx.Moduleです。
func UsecaseModule() fx.Option {
	return fx.Module("usecase",
		fx.Provide(
			fx.Annotate(
				healthcheck.New,
				fx.ParamTags(``, ``, ``, `group:"`+readinessProbeGroup+`"`),
			),
			fx.Annotate(
				observability.NewIdempotencyMetrics,
				fx.As(new(idempotency.Metrics)),
				fx.As(new(idempotency.GCMetrics)),
			),
			idempotency.NewDeps,
			idempotency.NewGC,
			outbox.NewEmit,
			outbox.NewGC,
			outbox.NewReplay,
			// sample-api:begin
			user.New,
			user.NewPurge,
			user.NewArchive,
			userroleuc.New,
			search.New,
			exchangerateuc.New,
			addressuc.New,
			prefectureuc.New,
			statusuc.New,
			categoryuc.New,
			rankinguc.New,
			provideProductUsecase,
			productuc.NewImageGC,
			purchaseuc.New,
			purchasestatusuc.New,
			couponuc.New, // sample-api:line
			cartuc.New,
			inquiryuc.New, // sample-api:line
			checkoutuc.New,
			purchasesummaryuc.New,
			dashboarduc.New,
			// sample-api:end
		),
	)
}

// sample-api:begin
// provideProductUsecase は、商品ユースケースを tx / マスタ Repository / object storage / authz /
// アップロード上限（config 由来）/ 購入 Repository / 廃番の CommandService・QueryService とともに
// 構築します。
func provideProductUsecase(
	txm txbd.Manager,
	repo domainproduct.Repository,
	categoryRepo domaincategory.Repository,
	statusRepo domainstatus.Repository,
	storage objectstoragebd.Storage,
	authorizer authzbd.Authorizer,
	clk clockbd.Clock,
	cfg *config.ObjectStorageConfig,
	purchaseRepo domainpurchase.Repository,
	discontinueCmd productcommand.CommandService,
	discontinueImpactQuery productquery.DiscontinueImpactQueryService,
	tf observability.TracerFactory,
) productuc.Usecase {
	return productuc.New(
		txm, repo, categoryRepo, statusRepo, storage, authorizer, clk, cfg.MaxUploadBytes(),
		purchaseRepo, discontinueCmd, discontinueImpactQuery, tf,
	)
}

// sample-api:end
