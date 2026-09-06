-- name: InsertPurchase :exec
-- 購入を 1 行 INSERT する。status_id は code から解決する（理由は docs/spec/domain/purchase.md の Notes）。
-- ordered_at / created_at / updated_at は DB 既定（NOW()）に委ねる。
-- coupon_id は未適用なら NULL。discount_amount との対応（NULL ⇔ 0）はドメインが課す。
INSERT INTO purchases (
    id,
    code,
    user_id,
    status_id,
    subtotal_amount,
    tax_amount,
    shipping_fee,
    total_amount,
    coupon_id,
    discount_amount
) VALUES (
    @id,
    @code,
    @user_id,
    (
        SELECT ps.id FROM purchase_statuses AS ps
        WHERE ps.code = @status_code
    ),
    @subtotal_amount,
    @tax_amount,
    @shipping_fee,
    @total_amount,
    sqlc.narg('coupon_id'),
    @discount_amount
);
