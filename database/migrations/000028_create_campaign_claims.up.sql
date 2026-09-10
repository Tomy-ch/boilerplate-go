CREATE TABLE IF NOT EXISTS campaign_claims (
    id UUID NOT NULL,
    campaign_id UUID NOT NULL,
    user_id UUID NOT NULL,
    coupon_id UUID NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT campaign_claims_id_primary PRIMARY KEY (id),
    CONSTRAINT campaign_claims_campaign_id_foreign FOREIGN KEY (campaign_id) REFERENCES campaigns (id),
    CONSTRAINT campaign_claims_user_id_foreign FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT campaign_claims_coupon_id_foreign FOREIGN KEY (coupon_id) REFERENCES coupons (id)
);

COMMENT ON TABLE campaign_claims IS 'キャンペーンの受け取り記録';
COMMENT ON COLUMN campaign_claims.id IS 'ID';
COMMENT ON COLUMN campaign_claims.campaign_id IS '受け取り元のキャンペーンID';
COMMENT ON COLUMN campaign_claims.user_id IS '受け取った利用者のユーザーID';
COMMENT ON COLUMN campaign_claims.coupon_id IS '受け取りで発行されたクーポンのID';
COMMENT ON COLUMN campaign_claims.claimed_at IS '受け取り日時';
COMMENT ON COLUMN campaign_claims.created_at IS '作成日時';
COMMENT ON COLUMN campaign_claims.updated_at IS '更新日時';

-- 1 人あたり上限の判定が引く。キャンペーン行の排他ロック下で、この索引を使って枚数を数える。
CREATE INDEX IF NOT EXISTS campaign_claims_campaign_id_user_id_idx
ON campaign_claims (campaign_id, user_id);

-- 利用者の物理削除とクーポン行の削除に伴う参照検査が引く。
CREATE INDEX IF NOT EXISTS campaign_claims_user_id_idx ON campaign_claims (user_id);
CREATE INDEX IF NOT EXISTS campaign_claims_coupon_id_idx ON campaign_claims (coupon_id);
