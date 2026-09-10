
-- === source: database/dml/repository/campaign/count_campaign_claims.sql ===
-- name: CountCampaignClaimsByCampaignIDAndUserID :one
-- 1 人あたり上限の判定が引く。キャンペーン行の排他ロックを取ったあとで呼ぶこと
-- （ADR-0036 (ordered-pessimistic-row-locks) の決定 2）。
SELECT COUNT(*)
FROM campaign_claims AS cc
WHERE cc.campaign_id = sqlc.arg('campaign_id')
    AND cc.user_id = sqlc.arg('user_id');

-- === source: database/dml/repository/campaign/insert_campaign.sql ===
-- name: CreateCampaign :exec
INSERT INTO campaigns (
    id,
    code,
    discount_kind,
    discount_value,
    discount_max_amount,
    scope_kind,
    scope_target_id,
    coupon_min_purchase_amount,
    coupon_usable_from,
    coupon_expires_at,
    starts_at,
    ends_at,
    total_limit,
    per_user_limit,
    issued_count
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('code'),
    sqlc.arg('discount_kind'),
    sqlc.arg('discount_value'),
    sqlc.narg('discount_max_amount'),
    sqlc.arg('scope_kind'),
    sqlc.arg('scope_target_id'),
    sqlc.narg('coupon_min_purchase_amount'),
    sqlc.narg('coupon_usable_from'),
    sqlc.arg('coupon_expires_at'),
    sqlc.arg('starts_at'),
    sqlc.arg('ends_at'),
    sqlc.arg('total_limit'),
    sqlc.arg('per_user_limit'),
    sqlc.arg('issued_count')
);

-- === source: database/dml/repository/campaign/insert_campaign_claim.sql ===
-- name: CreateCampaignClaim :exec
INSERT INTO campaign_claims (
    id,
    campaign_id,
    user_id,
    coupon_id,
    claimed_at
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('campaign_id'),
    sqlc.arg('user_id'),
    sqlc.arg('coupon_id'),
    sqlc.arg('claimed_at')
);

-- === source: database/dml/repository/campaign/lock_campaign_by_code.sql ===
-- name: LockCampaignByCode :one
-- 正規化済みコードからキャンペーンを 1 件、悲観ロック（FOR UPDATE）して取得する。不存在は 0 行（NotFound）。
-- 配布期間・停止・上限では絞らない（ADR-0036 (ordered-pessimistic-row-locks) の決定 5）。
-- ロックを条件の評価より前に取る理由は同 ADR の決定 2 を参照。
SELECT sqlc.embed(c)
FROM campaigns AS c
WHERE c.code = sqlc.arg('code')
FOR UPDATE;

-- === source: database/dml/repository/campaign/lock_campaign_by_id.sql ===
-- name: LockCampaignByID :one
-- ID からキャンペーンを 1 件、悲観ロック（FOR UPDATE）して取得する。不存在は 0 行（NotFound）。
-- 停止のために使う。絞り込みを置かない理由は LockCampaignByCode を参照。
SELECT sqlc.embed(c)
FROM campaigns AS c
WHERE c.id = sqlc.arg('id')
FOR UPDATE;

-- === source: database/dml/repository/campaign/select_campaigns.sql ===
-- name: SelectCampaigns :many
-- キャンペーンを配布期間の開始が新しい順で返す。admin の一覧が引く。
SELECT sqlc.embed(c)
FROM campaigns AS c
ORDER BY c.starts_at DESC, c.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountCampaigns :one
-- キャンペーンの総件数を返す。ページングが引く。
SELECT COUNT(*)
FROM campaigns;

-- === source: database/dml/repository/campaign/update_campaign_issued_count.sql ===
-- name: UpdateCampaignIssuedCount :execrows
-- 発行済み枚数を 1 増やす。更新件数を返す。
-- WHERE の issued_count < total_limit は、行ロックを取らずに呼ばれた場合に備える二重防御
-- （該当行なしは呼び出し側が競合として扱う）。上限の判定そのものはドメインが行う
-- （ADR-0036 (ordered-pessimistic-row-locks) の決定 5）。
UPDATE campaigns
SET
    issued_count = campaigns.issued_count + 1,
    updated_at = NOW()
WHERE campaigns.id = sqlc.arg('id')
    AND campaigns.issued_count < campaigns.total_limit;

-- === source: database/dml/repository/campaign/update_campaign_suspended.sql ===
-- name: UpdateCampaignSuspended :execrows
-- キャンペーンを停止する。更新件数を返す。
-- WHERE の suspended_at IS NULL は、行ロックを取らずに呼ばれた場合に備える二重防御
-- （該当行なしは呼び出し側が競合として扱う）。停止は取り消せないため遷移は一方向である。
UPDATE campaigns
SET
    suspended_at = sqlc.arg('suspended_at'),
    updated_at = NOW()
WHERE campaigns.id = sqlc.arg('id')
    AND campaigns.suspended_at IS NULL;
