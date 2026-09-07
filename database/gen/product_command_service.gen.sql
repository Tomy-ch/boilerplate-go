
-- === source: database/dml/command_service/product/count_discontinue_affected_carts.sql ===
-- name: CountDiscontinueAffectedCarts :one
-- 廃番対象の商品を明細に持つカートの件数を返す。所有者が確定していないゲストのカートも数える
-- （受給者との母集団差は docs/spec/usecase/product.md の Workflow — DiscontinueProduct を参照）。
SELECT COUNT(*)
FROM cart_items AS ci
WHERE ci.product_id = sqlc.arg('product_id');

-- === source: database/dml/command_service/product/select_discontinue_coupon_recipients.sql ===
-- name: SelectDiscontinueCouponRecipients :many
-- 廃番対象の商品を明細に持つカートのうち、所有者が確定していて退会もしていないユーザーを重複なく返す。
-- 絞り込みの理由と母集団が確定する時点は docs/spec/usecase/product.md の
-- Workflow — DiscontinueProduct の invariants を参照。
SELECT DISTINCT c.user_id::UUID AS user_id
FROM cart_items AS ci
INNER JOIN carts AS c ON ci.cart_id = c.id
INNER JOIN users AS u ON c.user_id = u.id
WHERE ci.product_id = sqlc.arg('product_id')
    AND u.deleted_at IS NULL;
