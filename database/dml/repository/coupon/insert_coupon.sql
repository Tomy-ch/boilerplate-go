-- name: CreateCoupon :exec
INSERT INTO coupons (
    id,
    user_id,
    discount_kind,
    discount_value,
    discount_max_amount,
    scope_kind,
    scope_target_id,
    min_purchase_amount,
    usable_from,
    expires_at,
    issued_at
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('user_id'),
    sqlc.arg('discount_kind'),
    sqlc.arg('discount_value'),
    sqlc.narg('discount_max_amount'),
    sqlc.arg('scope_kind'),
    sqlc.arg('scope_target_id'),
    sqlc.narg('min_purchase_amount'),
    sqlc.narg('usable_from'),
    sqlc.arg('expires_at'),
    sqlc.arg('issued_at')
);
