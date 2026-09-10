package campaign

const (
	// codeMinLength は、キャンペーンコードの最短の長さです。
	//
	// 推測されにくさと、人が転記できる範囲との折り合いです
	// （理由は docs/spec/domain/campaign.md の Code を参照）。
	codeMinLength = 8
	// codeMaxLength は、キャンペーンコードの最長の長さです。人が転記できる範囲に留めるための上限で、
	// これを超える文字列はコードではなく別のものになっています。
	codeMaxLength = 32

	// FieldCode は、キャンペーンコードフィールドの識別子です。
	FieldCode = "code"
	// FieldCampaignID は、キャンペーン ID フィールドの識別子です。
	FieldCampaignID = "campaignId"
)
