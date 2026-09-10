-- 3 列とも NULL 許容で足す。既存行は「条件なし」であって、既定値を入れると
-- 無かった条件をデータとして捏造することになる。
ALTER TABLE coupons
ADD COLUMN IF NOT EXISTS min_purchase_amount BIGINT,
ADD COLUMN IF NOT EXISTS discount_max_amount BIGINT,
ADD COLUMN IF NOT EXISTS usable_from TIMESTAMPTZ;

COMMENT ON COLUMN coupons.min_purchase_amount IS '使うために必要な購入額の下限（条件なしのときNULL）';
COMMENT ON COLUMN coupons.discount_max_amount IS '定率の値引きが1回に引ける額の上限（上限なしまたは定額のときNULL）';
COMMENT ON COLUMN coupons.usable_from IS '使えるようになる日時（発行時点から使えるときNULL）';
