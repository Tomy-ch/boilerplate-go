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
