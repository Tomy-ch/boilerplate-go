package module

import (
	"go-boilerplate/internal/controller/handler/health"
	"go-boilerplate/internal/controller/handler/healthz"
	"go-boilerplate/internal/controller/handler/metrics"
	"go-boilerplate/internal/controller/handler/ready"
	addresseshandler "go-boilerplate/internal/controller/handler/v1/addresses"                             // sample-api:line
	cartshandler "go-boilerplate/internal/controller/handler/v1/carts"                                     // sample-api:line
	cartscouponshandler "go-boilerplate/internal/controller/handler/v1/carts/coupons"                      // sample-api:line
	cartsitemshandler "go-boilerplate/internal/controller/handler/v1/carts/items"                          // sample-api:line
	cartsmergehandler "go-boilerplate/internal/controller/handler/v1/carts/merge"                          // sample-api:line
	dashboardhandler "go-boilerplate/internal/controller/handler/v1/dashboard"                             // sample-api:line
	exchangeratehandler "go-boilerplate/internal/controller/handler/v1/exchangerate"                       // sample-api:line
	inquirieshandler "go-boilerplate/internal/controller/handler/v1/inquiries"                             // sample-api:line
	inquirydetailmessageshandler "go-boilerplate/internal/controller/handler/v1/inquiries/detail/messages" // sample-api:line
	inquiryfeedtickethandler "go-boilerplate/internal/controller/handler/v1/inquiries/feed/streamticket"   // sample-api:line
	inquirymemessageshandler "go-boilerplate/internal/controller/handler/v1/inquiries/me/messages"         // sample-api:line
	inquirymetickethandler "go-boilerplate/internal/controller/handler/v1/inquiries/me/streamticket"       // sample-api:line
	prefectureshandler "go-boilerplate/internal/controller/handler/v1/prefectures"                         // sample-api:line
	productshandler "go-boilerplate/internal/controller/handler/v1/products"                               // sample-api:line
	productcategorieshandler "go-boilerplate/internal/controller/handler/v1/products/categories"           // sample-api:line
	productscounthandler "go-boilerplate/internal/controller/handler/v1/products/count"                    // sample-api:line
	productsdetailhandler "go-boilerplate/internal/controller/handler/v1/products/detail"                  // sample-api:line
	productslowstockhandler "go-boilerplate/internal/controller/handler/v1/products/lowstock"              // sample-api:line
	productsrankinghandler "go-boilerplate/internal/controller/handler/v1/products/ranking"                // sample-api:line
	productstatuseshandler "go-boilerplate/internal/controller/handler/v1/products/statuses"               // sample-api:line
	purchaseshandler "go-boilerplate/internal/controller/handler/v1/purchases"                             // sample-api:line
	purchasesdetailhandler "go-boilerplate/internal/controller/handler/v1/purchases/detail"                // sample-api:line
	purchasescancelhandler "go-boilerplate/internal/controller/handler/v1/purchases/detail/cancel"         // sample-api:line
	purchasesdeliverhandler "go-boilerplate/internal/controller/handler/v1/purchases/detail/deliver"       // sample-api:line
	purchasespayhandler "go-boilerplate/internal/controller/handler/v1/purchases/detail/pay"               // sample-api:line
	purchasesshiphandler "go-boilerplate/internal/controller/handler/v1/purchases/detail/ship"             // sample-api:line
	purchasesshippablehandler "go-boilerplate/internal/controller/handler/v1/purchases/shippable"          // sample-api:line
	purchasestatuseshandler "go-boilerplate/internal/controller/handler/v1/purchases/statuses"             // sample-api:line
	"go-boilerplate/internal/controller/handler/v1/users"                                                  // sample-api:line
	"go-boilerplate/internal/controller/handler/v1/users/detail"                                           // sample-api:line
	"go-boilerplate/internal/controller/handler/v1/users/feed"                                             // sample-api:line
	usersmecouponshandler "go-boilerplate/internal/controller/handler/v1/users/me/coupons"                 // sample-api:line
	usersmepurchaseshandler "go-boilerplate/internal/controller/handler/v1/users/me/purchases"             // sample-api:line
	usersmeroleshandler "go-boilerplate/internal/controller/handler/v1/users/me/roles"                     // sample-api:line
	"go-boilerplate/internal/controller/handler/v1/users/search"                                           // sample-api:line
	"go-boilerplate/internal/controller/handler/version"

	"go.uber.org/fx"
)

// ControllerModule は、コントローラー層の依存関係を提供するfx.Moduleです。
func ControllerModule() fx.Option {
	return fx.Module("controller",
		fx.Invoke(
			health.BindHandler,
			healthz.BindHandler,
			ready.BindHandler,
			version.BindHandler,
			metrics.BindHandler,
			// sample-api:begin
			users.BindHandler,
			detail.BindHandler,
			feed.BindHandler,
			search.BindHandler,
			usersmepurchaseshandler.BindHandler,
			usersmeroleshandler.BindHandler,
			usersmecouponshandler.BindHandler, // sample-api:line
			exchangeratehandler.BindHandler,
			addresseshandler.BindHandler,
			prefectureshandler.BindHandler,
			productstatuseshandler.BindHandler,
			productcategorieshandler.BindHandler,
			productscounthandler.BindHandler,
			productshandler.BindHandler,
			productsdetailhandler.BindHandler,
			productsrankinghandler.BindHandler,
			productslowstockhandler.BindHandler,
			purchasestatuseshandler.BindHandler,
			purchaseshandler.BindHandler,
			purchasesdetailhandler.BindHandler,
			purchasescancelhandler.BindHandler,
			purchasespayhandler.BindHandler,
			purchasesshiphandler.BindHandler,
			purchasesdeliverhandler.BindHandler,
			purchasesshippablehandler.BindHandler,
			dashboardhandler.BindHandler,
			cartshandler.BindHandler,
			cartsitemshandler.BindHandler,
			cartsmergehandler.BindHandler,
			cartscouponshandler.BindHandler, // sample-api:line
			inquirieshandler.BindHandler,
			inquirymemessageshandler.BindHandler,
			inquirymetickethandler.BindHandler,
			inquirydetailmessageshandler.BindHandler,
			inquiryfeedtickethandler.BindHandler,
			// sample-api:end
		),
	)
}
