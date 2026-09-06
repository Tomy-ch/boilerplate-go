-- name: GetPurchaseByID :one
-- ID から購入を 1 件取得する。現在状態は購入ステータスマスタとの結合で code を解決する
-- （code が状態機械の業務キーである根拠は Purchase 集約の定義。docs/spec/domain/purchase.md 参照）。
-- 存在しない場合は 0 行（NotFound）。
SELECT
    ps.code AS status_code,
    sqlc.embed(p)
FROM purchases AS p
INNER JOIN purchase_statuses AS ps ON p.status_id = ps.id
WHERE p.id = @id;

-- name: GetPurchaseDetailByID :one
-- ID から購入詳細（読み取りモデル）を 1 件取得する。ステータス名は購入ステータスマスタとの結合で
-- 解決済み（JOIN の許容範囲は internal/infrastructure/rdb/repository/README.md の
-- Reference-master exception）。
-- NULL 列の意味は docs/spec/domain/purchase.md の FindDetailByID を参照。
-- 存在しない場合は 0 行（NotFound）。
SELECT
    p.id,
    p.code,
    p.user_id,
    ps.id AS status_id,
    ps.code AS status_code,
    ps.name AS status_name,
    p.subtotal_amount,
    p.tax_amount,
    p.shipping_fee,
    p.total_amount,
    p.ordered_at,
    p.paid_at,
    p.canceled_at,
    p.shipped_at,
    p.delivered_at,
    p.coupon_id,
    p.discount_amount
FROM purchases AS p
INNER JOIN purchase_statuses AS ps ON p.status_id = ps.id
WHERE p.id = @id;

-- name: ListPurchaseDetailsByPurchaseID :many
-- 購入 ID から明細を id 昇順で取得する。
SELECT sqlc.embed(d)
FROM purchase_details AS d
WHERE d.purchase_id = @purchase_id_param
ORDER BY d.id;
