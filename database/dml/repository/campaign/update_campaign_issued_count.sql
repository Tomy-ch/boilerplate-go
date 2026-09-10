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
