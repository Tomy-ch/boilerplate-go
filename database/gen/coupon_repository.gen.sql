
-- === source: database/dml/repository/coupon/lock_coupon_by_id.sql ===
-- name: LockCouponByID :one
-- ID からクーポンを 1 件、悲観ロック（FOR UPDATE）して取得する。不存在は 0 行（NotFound）。
-- 使用済み・失効・受給者では絞らない理由は docs/spec/domain/coupon.md の
-- Repository Methods > LockByID を参照。
-- 取得位置の不変条件は docs/spec/usecase/purchase.md の CreatePurchase を参照
-- （ADR-0036 (ordered-pessimistic-row-locks)）。
SELECT sqlc.embed(c)
FROM coupons AS c
WHERE c.id = sqlc.arg('id')
FOR UPDATE;

-- === source: database/dml/repository/coupon/select_coupons_by_user_id.sql ===
-- name: ListCouponsByUserID :many
-- 指定利用者が保有するクーポンを発行日時の新しい順（同時刻は ID 降順）で返す。
-- 使用済み・失効済みで絞らない理由は docs/spec/domain/coupon.md の
-- Repository Methods > FindByUserID を参照。
SELECT sqlc.embed(c)
FROM coupons AS c
WHERE c.user_id = sqlc.arg('user_id')
ORDER BY c.issued_at DESC, c.id DESC;

-- === source: database/dml/repository/coupon/update_coupon_used.sql ===
-- name: UpdateCouponUsed :execrows
-- クーポンを使用済みにする。更新件数を返す。
-- WHERE の used_at IS NULL は、行ロックを取らずに呼ばれた場合に備える二重防御
-- （該当行なしは呼び出し側が競合として扱う）。詳細は docs/spec/domain/coupon.md の
-- Repository Methods > UpdateUsed を参照。
UPDATE coupons
SET
    used_at = sqlc.arg('used_at'),
    updated_at = NOW()
WHERE coupons.id = sqlc.arg('id')
    AND coupons.used_at IS NULL;
