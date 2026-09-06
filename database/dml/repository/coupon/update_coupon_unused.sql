-- name: UpdateCouponUnused :execrows
-- 使用済みのクーポンを未使用へ戻す。更新件数を返す。
-- WHERE の used_at IS NOT NULL は、行ロックを取らずに呼ばれた場合に備える二重防御
-- （該当行なしは呼び出し側が未使用として扱う）。詳細は docs/spec/domain/coupon.md の
-- Repository Methods > UpdateUnused を参照。
UPDATE coupons
SET
    used_at = NULL,
    updated_at = NOW()
WHERE coupons.id = sqlc.arg('id')
    AND coupons.used_at IS NOT NULL;
