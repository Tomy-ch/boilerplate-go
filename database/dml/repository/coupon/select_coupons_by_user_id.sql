-- name: ListCouponsByUserID :many
-- 指定利用者が保有するクーポンを発行日時の新しい順（同時刻は ID 降順）で返す。
-- 使用済み・失効済みで絞らない理由は docs/spec/domain/coupon.md の
-- Repository Methods > FindByUserID を参照。
SELECT sqlc.embed(c)
FROM coupons AS c
WHERE c.user_id = sqlc.arg('user_id')
ORDER BY c.issued_at DESC, c.id DESC;
