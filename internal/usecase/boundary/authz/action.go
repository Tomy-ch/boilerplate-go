package authz

// sample-api:begin
// サンプル EC の操作。Action 型と String は基盤として残す。
const (
	// ActionUserList は、ユーザーの列挙操作（一覧・フィード・検索、admin）を表します。
	ActionUserList Action = "user:list"
	// ActionUserGet は、ユーザー取得操作を表します。
	ActionUserGet Action = "user:get"
	// ActionUserUpdate は、ユーザー更新操作（全更新・部分更新）を表します。
	ActionUserUpdate Action = "user:update"
	// ActionUserDelete は、ユーザー削除操作を表します。
	ActionUserDelete Action = "user:delete"
	// ActionProductImageUpload は、商品画像のアップロード操作（admin）を表します。
	ActionProductImageUpload Action = "product:image:upload"
	// ActionProductCreate は、商品の作成操作（admin）を表します。
	ActionProductCreate Action = "product:create"
	// ActionProductUpdate は、商品の更新操作（admin）を表します。
	ActionProductUpdate Action = "product:update"
	// ActionProductStockUpdate は、商品在庫の増減操作（admin）を表します。
	ActionProductStockUpdate Action = "product:stock:update"
	// ActionProductDiscontinue は、商品の廃番操作と、その影響の見積もりの参照（admin）を表します。
	// 見積もりは廃番を実行できる主体だけが見るべき情報なので、実行と同じ権限で守ります。
	ActionProductDiscontinue Action = "product:discontinue"
	// ActionProductListLowStock は、在庫僅少商品一覧の参照操作（admin）を表します。
	ActionProductListLowStock Action = "product:low-stock:list"
	// ActionProductReadUnpublished は、未公開商品を含む商品の参照操作（admin）を表します。
	// 3 経路が同一の Action を共有する理由は docs/spec/usecase/product.md の Overview を参照。
	ActionProductReadUnpublished Action = "product:unpublished:read"
	// ActionPurchaseReadAll は、購入者を問わない購入の参照操作（admin）を表します。
	ActionPurchaseReadAll Action = "purchase:all:read"
	// ActionPurchaseListShippable は、発送待ち購入一覧の参照操作（admin）を表します。
	ActionPurchaseListShippable Action = "purchase:shippable:list"
	// ActionPurchaseShip は、購入の発送操作（admin）を表します。
	ActionPurchaseShip Action = "purchase:ship"
	// ActionPurchaseDeliver は、購入の配達完了操作（admin）を表します。
	ActionPurchaseDeliver Action = "purchase:deliver"
	// ActionDashboardRead は、ダッシュボード集計の参照操作（admin）を表します。
	ActionDashboardRead Action = "dashboard:read"
	// ActionInquiryList は、問い合わせの列挙操作（admin）を表します。
	ActionInquiryList Action = "inquiry:list"
	// ActionInquiryReadAll は、利用者を問わない問い合わせの参照操作（admin）を表します。
	ActionInquiryReadAll Action = "inquiry:read-all"
	// ActionInquiryReply は、問い合わせへの回答操作（admin）を表します。
	ActionInquiryReply Action = "inquiry:reply"
	// ActionInquiryFeedSubscribe は、問い合わせ更新フィードの購読操作（admin）を表します。
	ActionInquiryFeedSubscribe Action = "inquiry-feed:subscribe"
	// ActionCouponIssue は、受給者を名指ししたクーポンの発行（admin）を表します。
	ActionCouponIssue Action = "coupon:issue"
	// ActionCouponBulkIssue は、販促クーポンの一括発行（admin）を表します。
	// 受給者を名指しする発行とは別の Action です。理由は docs/spec/usecase/coupon.md の Overview を参照。
	ActionCouponBulkIssue Action = "coupon:bulk-issue"
	// ActionCampaignDefine は、キャンペーンの定義（admin）を表します。
	ActionCampaignDefine Action = "campaign:define"
	// ActionCampaignList は、キャンペーンの列挙操作（admin）を表します。
	ActionCampaignList Action = "campaign:list"
	// ActionCampaignSuspend は、キャンペーンの停止（admin）を表します。
	ActionCampaignSuspend Action = "campaign:suspend"
)

// sample-api:end

// Action は、認可対象の操作を表す値オブジェクトです（例: "user:delete"）。
type Action string

// String は、Action の文字列表現を返します。
func (a Action) String() string { return string(a) }
