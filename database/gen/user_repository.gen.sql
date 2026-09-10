
-- === source: database/dml/repository/user/count_user.sql ===
-- name: CountUsers :one
SELECT COUNT(*)
FROM users;

-- name: CountActiveUsers :one
SELECT COUNT(*)
FROM users AS u
WHERE u.deleted_at IS NULL;

-- name: CountDeletedUsers :one
SELECT COUNT(*)
FROM users AS u
WHERE u.deleted_at IS NOT NULL;

-- === source: database/dml/repository/user/count_users_by_keyword.sql ===
-- name: CountSearchUsers :one
SELECT COUNT(*)
FROM users AS u
WHERE u.search_text ILIKE ANY(sqlc.arg('patterns_param')::TEXT[]);

-- name: CountSearchActiveUsers :one
SELECT COUNT(*)
FROM users AS u
WHERE u.search_text ILIKE ANY(sqlc.arg('patterns_param')::TEXT[])
    AND u.deleted_at IS NULL;

-- name: CountSearchDeletedUsers :one
SELECT COUNT(*)
FROM users AS u
WHERE u.search_text ILIKE ANY(sqlc.arg('patterns_param')::TEXT[])
    AND u.deleted_at IS NOT NULL;

-- === source: database/dml/repository/user/delete_purged_users.sql ===
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

-- === source: database/dml/repository/user/insert_user.sql ===
-- name: CreateUser :exec
INSERT INTO users (
    id,
    first_name,
    last_name,
    email,
    phone,
    prefecture_id,
    city,
    street,
    building,
    postal_code,
    created_at,
    updated_at
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('first_name'),
    sqlc.arg('last_name'),
    sqlc.arg('email'),
    sqlc.arg('phone'),
    sqlc.arg('prefecture_id'),
    sqlc.arg('city'),
    sqlc.arg('street'),
    sqlc.arg('building'),
    sqlc.arg('postal_code'),
    sqlc.arg('created_at'),
    sqlc.arg('updated_at')
);

-- === source: database/dml/repository/user/lock_user_by_id.sql ===
-- name: LockUserByID :one
-- ID から未削除のユーザーを 1 件、悲観ロック（FOR UPDATE）して取得する。
-- 論理削除済み・不存在はいずれも 0 行（NotFound）。
-- 取得位置の不変条件は docs/spec/usecase/user.md の DeleteUser を参照（ADR-0036 (ordered-pessimistic-row-locks)）。
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.id = sqlc.arg('user_id_param')
    AND u.deleted_at IS NULL
FOR UPDATE;

-- === source: database/dml/repository/user/lock_user_share_by_id.sql ===
-- name: LockUserShareByID :one
-- ID からユーザーを 1 件、悲観ロック（FOR SHARE）して取得する。不存在は 0 行（NotFound）。
-- 退会済みを除外しないこと — ロックは機構で、在籍かどうかの判定はドメイン（User.IsActive）が持つ。
-- 退会との直列化は docs/spec/usecase/purchase.md の CreatePurchase を参照。
-- ADR-0036 (ordered-pessimistic-row-locks)。
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.id = sqlc.arg('user_id_param')
FOR SHARE;

-- === source: database/dml/repository/user/select_purge_candidate_users.sql ===
-- name: ListPurgeCandidateUserIDsFirst :many
-- 論理削除日時が cutoff より古いユーザーの ID を、ID 昇順の keyset で最大 limit_param 件取得する（先頭ページ）。
SELECT id
FROM users
WHERE deleted_at IS NOT NULL
    AND deleted_at < sqlc.arg('cutoff')
ORDER BY id ASC
LIMIT sqlc.arg('limit_param');

-- name: ListPurgeCandidateUserIDsAfter :many
-- 論理削除日時が cutoff より古いユーザーの ID を、ID 昇順の keyset で最大 limit_param 件取得する（after_id 以降）。
-- 境界を offset でなく ID で受け取る理由は docs/spec/domain/user.md の FindDeletedBefore を参照。
SELECT id
FROM users
WHERE deleted_at IS NOT NULL
    AND deleted_at < sqlc.arg('cutoff')
    AND id > sqlc.arg('after_id')
ORDER BY id ASC
LIMIT sqlc.arg('limit_param');

-- === source: database/dml/repository/user/select_roles_by_user_id.sql ===
-- name: GetUserRolesByUserID :many
-- 指定ユーザーのロールをマスタの表示順（sort_key 昇順）で返す。並び順の出所は code ではない。
SELECT
    r.id,
    r.name,
    r.code
FROM user_roles AS ur
INNER JOIN roles AS r ON ur.role_id = r.id
WHERE ur.user_id = sqlc.arg('user_id_param')
ORDER BY r.sort_key;

-- === source: database/dml/repository/user/select_user_by_id.sql ===
-- name: GetUserByID :one
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.id = sqlc.arg('user_id_param')
    AND u.deleted_at IS NULL;

-- === source: database/dml/repository/user/select_users.sql ===
-- name: ListUsers :many
SELECT sqlc.embed(u)
FROM users AS u
ORDER BY u.created_at DESC
LIMIT sqlc.arg('limit_param') OFFSET sqlc.arg('offset_param');

-- name: ListActiveUsers :many
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.deleted_at IS NULL
ORDER BY u.created_at DESC
LIMIT sqlc.arg('limit_param') OFFSET sqlc.arg('offset_param');

-- name: ListDeletedUsers :many
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.deleted_at IS NOT NULL
ORDER BY u.created_at DESC
LIMIT sqlc.arg('limit_param') OFFSET sqlc.arg('offset_param');

-- === source: database/dml/repository/user/select_users_by_keyword.sql ===
-- name: SearchUsers :many
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.search_text ILIKE ANY(sqlc.arg('patterns_param')::TEXT[])
ORDER BY u.created_at DESC
LIMIT sqlc.arg('limit_param') OFFSET sqlc.arg('offset_param');

-- name: SearchActiveUsers :many
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.search_text ILIKE ANY(sqlc.arg('patterns_param')::TEXT[])
    AND u.deleted_at IS NULL
ORDER BY u.created_at DESC
LIMIT sqlc.arg('limit_param') OFFSET sqlc.arg('offset_param');

-- name: SearchDeletedUsers :many
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.search_text ILIKE ANY(sqlc.arg('patterns_param')::TEXT[])
    AND u.deleted_at IS NOT NULL
ORDER BY u.created_at DESC
LIMIT sqlc.arg('limit_param') OFFSET sqlc.arg('offset_param');

-- === source: database/dml/repository/user/select_users_feed.sql ===
-- name: ListUsersFeedFirst :many
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.deleted_at IS NULL
ORDER BY u.created_at DESC, u.id DESC
LIMIT sqlc.arg('limit_param');

-- name: ListUsersFeedAfter :many
-- (created_at DESC, id DESC) の keyset 境界より過去の未削除ユーザーを返します。
SELECT sqlc.embed(u)
FROM users AS u
WHERE u.deleted_at IS NULL
    AND (
        u.created_at < sqlc.arg('after_created_at')
        OR (u.created_at = sqlc.arg('after_created_at') AND u.id < sqlc.arg('after_id'))
    )
ORDER BY u.created_at DESC, u.id DESC
LIMIT sqlc.arg('limit_param');

-- === source: database/dml/repository/user/update_user.sql ===
-- name: UpdateUser :execrows
UPDATE users
SET
    first_name = sqlc.arg('first_name'),
    last_name = sqlc.arg('last_name'),
    email = sqlc.arg('email'),
    phone = sqlc.arg('phone'),
    prefecture_id = sqlc.arg('prefecture_id'),
    city = sqlc.arg('city'),
    street = sqlc.arg('street'),
    building = sqlc.arg('building'),
    postal_code = sqlc.arg('postal_code'),
    updated_at = sqlc.arg('updated_at'),
    deleted_at = sqlc.arg('deleted_at')
WHERE id = sqlc.arg('id')
    AND deleted_at IS NULL;
