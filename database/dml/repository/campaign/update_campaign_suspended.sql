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
