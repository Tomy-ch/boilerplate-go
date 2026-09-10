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
