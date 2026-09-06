DROP INDEX IF EXISTS purchases_coupon_id_idx;

ALTER TABLE purchases
DROP CONSTRAINT IF EXISTS purchases_coupon_id_foreign;

ALTER TABLE purchases
DROP COLUMN IF EXISTS discount_amount,
DROP COLUMN IF EXISTS coupon_id;
