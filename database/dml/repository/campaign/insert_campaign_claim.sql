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
