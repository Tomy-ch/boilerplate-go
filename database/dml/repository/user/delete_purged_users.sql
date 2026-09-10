-- name: DeleteUserIdentitiesByUserIDs :exec
-- users より先に呼ぶこと（FK 違反を避ける）。同じ順序制約は本ファイルの DeleteUserRolesByUserIDs /
-- DeleteCouponsByUserIDs / DeleteCampaignClaimsByUserIDs も持つ。論理削除済みに限る条件は DeleteUsersByIDs の WHERE と揃えること
-- — ずれると、削除されないユーザーの従属行だけが失われる。
DELETE FROM user_identities
WHERE user_id IN (
        SELECT u.id
        FROM users AS u
        WHERE u.id = ANY(sqlc.arg('user_ids')::UUID[])
            AND u.deleted_at IS NOT NULL
    );

-- name: DeleteUserRolesByUserIDs :exec
-- 順序制約と論理削除条件の理由は DeleteUserIdentitiesByUserIDs を参照。
DELETE FROM user_roles
WHERE user_id IN (
        SELECT u.id
        FROM users AS u
        WHERE u.id = ANY(sqlc.arg('user_ids')::UUID[])
            AND u.deleted_at IS NOT NULL
    );

-- name: DeleteUsersByIDs :execrows
-- 削除件数を返す。従属行の削除後に呼ぶこと（参照の残存はここでは検査しない）。
-- 論理削除済みを永続化側でも検査する理由は docs/spec/domain/user.md の PurgeByIDs を参照。
DELETE FROM users
WHERE id = ANY(sqlc.arg('ids')::UUID[])
    AND deleted_at IS NOT NULL;

-- name: DeleteCouponsByUserIDs :exec
-- 順序制約と論理削除条件の理由は DeleteUserIdentitiesByUserIDs を参照。
-- クーポンを残さない理由は docs/spec/domain/coupon.md の Notes を参照。
DELETE FROM coupons
WHERE user_id IN (
        SELECT u.id
        FROM users AS u
        WHERE u.id = ANY(sqlc.arg('user_ids')::UUID[])
            AND u.deleted_at IS NOT NULL
    );

-- name: DeleteCampaignClaimsByUserIDs :exec
-- coupons より先に呼ぶこと。受け取り記録は利用者とクーポンの両方を参照するため、
-- 残したまま DeleteCouponsByUserIDs を呼ぶと FK 違反になる。
-- 論理削除条件の理由は DeleteUserIdentitiesByUserIDs を参照。
DELETE FROM campaign_claims
WHERE user_id IN (
        SELECT u.id
        FROM users AS u
        WHERE u.id = ANY(sqlc.arg('user_ids')::UUID[])
            AND u.deleted_at IS NOT NULL
    );
