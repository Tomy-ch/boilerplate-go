-- name: CountCampaignClaimsByCampaignIDAndUserID :one
-- 1 人あたり上限の判定が引く。キャンペーン行の排他ロックを取ったあとで呼ぶこと
-- （ADR-0036 (ordered-pessimistic-row-locks) の決定 2）。
SELECT COUNT(*)
FROM campaign_claims AS cc
WHERE cc.campaign_id = sqlc.arg('campaign_id')
    AND cc.user_id = sqlc.arg('user_id');
