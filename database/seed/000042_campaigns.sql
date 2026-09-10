-- 配布中 / 配布前 / 停止済み / 上限到達 の 4 状態を 1 件ずつ置く。ランタイム検証と手元の確認が、
-- 拒否理由ごとの応答を作らずに辿れるようにするため。いずれも 4 状態とも同じ 422 に畳まれる
-- （docs/spec/usecase/campaign.md の「受け取れない理由を区別しない」）。
-- discount_kind / scope_kind は種別の名前。閉じた集合を所有するのはクーポン側で、
-- キャンペーンは名前を保つ（docs/spec/domain/campaign.md の Overview）。
INSERT INTO campaigns (
    id, code, discount_kind, discount_value, discount_max_amount, scope_kind, scope_target_id,
    coupon_min_purchase_amount, coupon_usable_from, coupon_expires_at,
    starts_at, ends_at, total_limit, per_user_limit, issued_count, suspended_at
) VALUES
-- 配布中。10% off・上限 $20・$50 以上の購入で使える。
('0193a1c0-0002-7000-8000-000000000001', 'WELCOME-2026', 'rate', '0.10', 2000, 'all', NULL,
 5000, NULL, NOW() + INTERVAL '90 days',
 NOW() - INTERVAL '7 days', NOW() + INTERVAL '30 days', 1000, 1, 0, NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO campaigns (
    id, code, discount_kind, discount_value, discount_max_amount, scope_kind, scope_target_id,
    coupon_min_purchase_amount, coupon_usable_from, coupon_expires_at,
    starts_at, ends_at, total_limit, per_user_limit, issued_count, suspended_at
) VALUES
-- 配布前。開始日時ちょうどから受け取れる。
('0193a1c0-0002-7000-8000-000000000002', 'UPCOMING-2026', 'flat', '3.00', NULL, 'all', NULL,
 NULL, NULL, NOW() + INTERVAL '120 days',
 NOW() + INTERVAL '7 days', NOW() + INTERVAL '60 days', 500, 2, 0, NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO campaigns (
    id, code, discount_kind, discount_value, discount_max_amount, scope_kind, scope_target_id,
    coupon_min_purchase_amount, coupon_usable_from, coupon_expires_at,
    starts_at, ends_at, total_limit, per_user_limit, issued_count, suspended_at
) VALUES
-- 停止済み。配布期間は残っているが、以後は受け取れない。
('0193a1c0-0002-7000-8000-000000000003', 'SUSPENDED-2026', 'rate', '0.20', NULL, 'category',
 '3a60c501-7049-4a63-bfd3-bf34555f3aec',
 NULL, NULL, NOW() + INTERVAL '90 days',
 NOW() - INTERVAL '14 days', NOW() + INTERVAL '30 days', 100, 1, 3, NOW() - INTERVAL '1 days')
ON CONFLICT (id) DO NOTHING;

INSERT INTO campaigns (
    id, code, discount_kind, discount_value, discount_max_amount, scope_kind, scope_target_id,
    coupon_min_purchase_amount, coupon_usable_from, coupon_expires_at,
    starts_at, ends_at, total_limit, per_user_limit, issued_count, suspended_at
) VALUES
-- 総枚数上限に到達済み。配布期間内で停止もしていないが、もう配れない。
('0193a1c0-0002-7000-8000-000000000004', 'SOLDOUT-2026', 'flat', '10.00', NULL, 'all', NULL,
 NULL, NULL, NOW() + INTERVAL '90 days',
 NOW() - INTERVAL '14 days', NOW() + INTERVAL '30 days', 5, 1, 5, NULL)
ON CONFLICT (id) DO NOTHING;
