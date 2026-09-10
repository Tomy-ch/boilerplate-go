-- name: CreateCampaign :exec
INSERT INTO campaigns (
    id,
    code,
    discount_kind,
    discount_value,
    discount_max_amount,
    scope_kind,
    scope_target_id,
    coupon_min_purchase_amount,
    coupon_usable_from,
    coupon_expires_at,
    starts_at,
    ends_at,
    total_limit,
    per_user_limit,
    issued_count
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('code'),
    sqlc.arg('discount_kind'),
    sqlc.arg('discount_value'),
    sqlc.narg('discount_max_amount'),
    sqlc.arg('scope_kind'),
    sqlc.arg('scope_target_id'),
    sqlc.narg('coupon_min_purchase_amount'),
    sqlc.narg('coupon_usable_from'),
    sqlc.arg('coupon_expires_at'),
    sqlc.arg('starts_at'),
    sqlc.arg('ends_at'),
    sqlc.arg('total_limit'),
    sqlc.arg('per_user_limit'),
    sqlc.arg('issued_count')
);
