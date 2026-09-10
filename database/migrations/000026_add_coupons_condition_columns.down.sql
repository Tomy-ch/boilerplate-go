ALTER TABLE coupons
DROP COLUMN IF EXISTS usable_from,
DROP COLUMN IF EXISTS discount_max_amount,
DROP COLUMN IF EXISTS min_purchase_amount;
