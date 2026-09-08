-- 値引き 2 種（定額 / 定率）× 適用範囲 3 種（全体 / カテゴリ限定 / 商品限定）の 6 組み合わせを 1 件ずつ置く。
-- 廃番ジャーニーが作らない組み合わせは発行 API 経由でのみ生成される
-- （docs/spec/usecase/product.md の IssueDiscontinuationCoupons）。読み・引き換え側のテストが
-- 6 組み合わせを固定で参照できるよう、ここに揃えて置く。
-- discount_kind / scope_kind の値はドメインが持つ業務キー（coupon.DiscountKind / coupon.ScopeKind）。
INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) VALUES
('0193a1c0-0001-7000-8000-000000000001', '090f5b51-37ac-4413-b326-1709ae4661f4', 1, '5.00', 1, NULL, NOW() + INTERVAL '30 days', NOW() - INTERVAL '1 days') ON CONFLICT (id) DO NOTHING;
INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) VALUES
('0193a1c0-0001-7000-8000-000000000002', '090f5b51-37ac-4413-b326-1709ae4661f4', 2, '0.10', 1, NULL, NOW() + INTERVAL '30 days', NOW() - INTERVAL '1 days') ON CONFLICT (id) DO NOTHING;
INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) VALUES
('0193a1c0-0001-7000-8000-000000000003', '099bfa4f-4e6f-586d-af12-9bfce82cc552', 1, '3.00', 2, '3a60c501-7049-4a63-bfd3-bf34555f3aec', NOW() + INTERVAL '30 days', NOW() - INTERVAL '2 days') ON CONFLICT (id) DO NOTHING;
INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) VALUES
('0193a1c0-0001-7000-8000-000000000004', '099bfa4f-4e6f-586d-af12-9bfce82cc552', 2, '0.15', 2, '3a60c501-7049-4a63-bfd3-bf34555f3aec', NOW() + INTERVAL '30 days', NOW() - INTERVAL '2 days') ON CONFLICT (id) DO NOTHING;
INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) VALUES
('0193a1c0-0001-7000-8000-000000000005', '0b393ac1-b8a2-4f69-8972-de680aeb0a95', 1, '1.50', 3, '00a4c85e-6d90-5567-9f4d-e7ab46d51a74', NOW() + INTERVAL '30 days', NOW() - INTERVAL '3 days') ON CONFLICT (id) DO NOTHING;
INSERT INTO coupons (id, user_id, discount_kind, discount_value, scope_kind, scope_target_id, expires_at, issued_at) VALUES
('0193a1c0-0001-7000-8000-000000000006', '0b393ac1-b8a2-4f69-8972-de680aeb0a95', 2, '0.05', 3, '00a4c85e-6d90-5567-9f4d-e7ab46d51a74', NOW() + INTERVAL '30 days', NOW() - INTERVAL '3 days') ON CONFLICT (id) DO NOTHING;
