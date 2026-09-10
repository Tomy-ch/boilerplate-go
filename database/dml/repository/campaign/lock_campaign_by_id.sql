-- name: LockCampaignByID :one
-- ID からキャンペーンを 1 件、悲観ロック（FOR UPDATE）して取得する。不存在は 0 行（NotFound）。
-- 停止のために使う。絞り込みを置かない理由は LockCampaignByCode を参照。
SELECT sqlc.embed(c)
FROM campaigns AS c
WHERE c.id = sqlc.arg('id')
FOR UPDATE;
