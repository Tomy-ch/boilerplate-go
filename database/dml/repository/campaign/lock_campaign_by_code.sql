-- name: LockCampaignByCode :one
-- 正規化済みコードからキャンペーンを 1 件、悲観ロック（FOR UPDATE）して取得する。不存在は 0 行（NotFound）。
-- 配布期間・停止・上限では絞らない（ADR-0036 (ordered-pessimistic-row-locks) の決定 5）。
-- ロックを条件の評価より前に取る理由は同 ADR の決定 2 を参照。
SELECT sqlc.embed(c)
FROM campaigns AS c
WHERE c.code = sqlc.arg('code')
FOR UPDATE;
