CREATE TABLE IF NOT EXISTS campaigns (
    id UUID NOT NULL,
    code TEXT NOT NULL,
    discount_kind TEXT NOT NULL,
    discount_value NUMERIC NOT NULL,
    discount_max_amount BIGINT,
    scope_kind TEXT NOT NULL,
    scope_target_id UUID,
    coupon_min_purchase_amount BIGINT,
    coupon_usable_from TIMESTAMPTZ,
    coupon_expires_at TIMESTAMPTZ NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    total_limit INTEGER NOT NULL,
    per_user_limit INTEGER NOT NULL,
    issued_count INTEGER NOT NULL DEFAULT 0,
    suspended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT campaigns_id_primary PRIMARY KEY (id),
    CONSTRAINT campaigns_code_unique UNIQUE (code)
);

COMMENT ON TABLE campaigns IS 'キャンペーン';
COMMENT ON COLUMN campaigns.id IS 'ID';
COMMENT ON COLUMN campaigns.code IS 'キャンペーンコード（正規化済み。大文字・前後空白なし）';
COMMENT ON COLUMN campaigns.discount_kind IS '配るクーポンの値引き種別の名前。閉じた集合を所有するのはクーポン側で、キャンペーンは名前を保つ';
COMMENT ON COLUMN campaigns.discount_value IS '配るクーポンの値引きの値（定額なら金額、定率なら率）';
COMMENT ON COLUMN campaigns.discount_max_amount IS '配るクーポンの値引き上限（上限なしまたは定額のときNULL）';
COMMENT ON COLUMN campaigns.scope_kind IS '配るクーポンの適用範囲種別の名前。閉じた集合を所有するのはクーポン側で、キャンペーンは名前を保つ';
COMMENT ON COLUMN campaigns.scope_target_id IS '配るクーポンの適用範囲の対象ID（全体のときNULL）';
COMMENT ON COLUMN campaigns.coupon_min_purchase_amount IS '配るクーポンの最低購入金額（条件なしのときNULL）';
COMMENT ON COLUMN campaigns.coupon_usable_from IS '配るクーポンの利用開始日時（発行時点から使えるときNULL）';
COMMENT ON COLUMN campaigns.coupon_expires_at IS '配るクーポンの有効期限';
COMMENT ON COLUMN campaigns.starts_at IS '配布期間の開始日時';
COMMENT ON COLUMN campaigns.ends_at IS '配布期間の終了日時';
COMMENT ON COLUMN campaigns.total_limit IS '総枚数上限';
COMMENT ON COLUMN campaigns.per_user_limit IS '1人あたり上限';
COMMENT ON COLUMN campaigns.issued_count IS '発行済み枚数。返却では減らない';
COMMENT ON COLUMN campaigns.suspended_at IS '停止日時（停止していないときNULL）';
COMMENT ON COLUMN campaigns.created_at IS '作成日時';
COMMENT ON COLUMN campaigns.updated_at IS '更新日時';
