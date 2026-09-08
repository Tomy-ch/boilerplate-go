-- name: CreateCoupon :exec
INSERT INTO coupons (
    id,
    user_id,
    discount_kind,
    discount_value,
    scope_kind,
    scope_target_id,
    expires_at,
    issued_at
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('user_id'),
    sqlc.arg('discount_kind'),
    sqlc.arg('discount_value'),
    sqlc.arg('scope_kind'),
    sqlc.arg('scope_target_id'),
    sqlc.arg('expires_at'),
    sqlc.arg('issued_at')
);
