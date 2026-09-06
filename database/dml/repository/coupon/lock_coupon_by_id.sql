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
