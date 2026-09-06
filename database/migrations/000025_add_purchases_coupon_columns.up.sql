ALTER TABLE purchases
ADD COLUMN IF NOT EXISTS coupon_id UUID,
ADD COLUMN IF NOT EXISTS discount_amount BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN purchases.coupon_id IS '適用したクーポンのID（未使用のときNULL）';
COMMENT ON COLUMN purchases.discount_amount IS '値引き額';

-- 参照先を消せないことを構造で守る。控えは値引きの理由をこの結合で解決するため、
-- 使用済みクーポンの行が消えると金額の説明が付かなくなる。
ALTER TABLE purchases
ADD CONSTRAINT purchases_coupon_id_foreign FOREIGN KEY (coupon_id) REFERENCES coupons (id);

-- クーポン行を消そうとしたときの参照検査が引く（利用者の物理削除が coupons を消す経路を持つ）。
-- 適用していない購入は対象外なので、部分索引で NULL の行を載せない。
CREATE INDEX IF NOT EXISTS purchases_coupon_id_idx ON purchases (coupon_id)
WHERE coupon_id IS NOT NULL;
