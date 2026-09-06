
-- === source: database/dml/repository/purchase/insert_purchase.sql ===
-- name: InsertPurchase :exec
-- 購入を 1 行 INSERT する。status_id は code から解決する（理由は docs/spec/domain/purchase.md の Notes）。
-- ordered_at / created_at / updated_at は DB 既定（NOW()）に委ねる。
-- coupon_id は未適用なら NULL。discount_amount との対応（NULL ⇔ 0）はドメインが課す。
INSERT INTO purchases (
    id,
    code,
    user_id,
    status_id,
    subtotal_amount,
    tax_amount,
    shipping_fee,
    total_amount,
    coupon_id,
    discount_amount
) VALUES (
    @id,
    @code,
    @user_id,
    (
        SELECT ps.id FROM purchase_statuses AS ps
        WHERE ps.code = @status_code
    ),
    @subtotal_amount,
    @tax_amount,
    @shipping_fee,
    @total_amount,
    sqlc.narg('coupon_id'),
    @discount_amount
);

-- === source: database/dml/repository/purchase/insert_purchase_detail.sql ===
-- name: InsertPurchaseDetail :exec
-- 購入明細を 1 行 INSERT する。unit_price は購入時点の単価スナップショット（USD セント整数）。
INSERT INTO purchase_details (
    id,
    purchase_id,
    product_id,
    quantity,
    unit_price
) VALUES (
    @id,
    @purchase_id,
    @product_id,
    @quantity,
    @unit_price
);

-- === source: database/dml/repository/purchase/lock_purchase_by_code.sql ===
-- name: LockPurchaseByCode :one
-- 購入コードから購入を 1 件、購入行のみ悲観ロック（FOR UPDATE OF p）して取得する。支払いの状態遷移の
-- 競合（同一購入への並行支払い）を購入行ロックで直列化する（結合先の固定参照マスタはロックしない）。
-- 現在状態は購入ステータスマスタとの結合で code を解決する。存在しない場合は 0 行（NotFound）。
SELECT
    ps.code AS status_code,
    sqlc.embed(p)
FROM purchases AS p
INNER JOIN purchase_statuses AS ps ON p.status_id = ps.id
WHERE p.code = @code
FOR UPDATE OF p;

-- === source: database/dml/repository/purchase/select_purchase_by_id.sql ===
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

-- === source: database/dml/repository/purchase/select_purchase_status_codes_by_product_id.sql ===
-- name: SelectPurchaseStatusCodesByProductID :many
-- 指定商品を明細に持つ購入が取っているステータスの code を重複なく返す。
-- 進行中かどうかで絞らないのは、その判定を購入集約（Status.IsTerminal）が持つためで、
-- SQL 側に同じ規則を書き写さない（理由は docs/spec/domain/purchase.md の FindStatusesByProductID）。
SELECT DISTINCT ps.code
FROM purchases AS p
INNER JOIN purchase_details AS pd ON p.id = pd.purchase_id
INNER JOIN purchase_statuses AS ps ON p.status_id = ps.id
WHERE pd.product_id = sqlc.arg('product_id');

-- === source: database/dml/repository/purchase/select_purchase_status_codes_by_user_id.sql ===
-- name: SelectPurchaseStatusCodesByUserID :many
-- 指定ユーザーの購入が取っているステータス code を重複なく返す。
-- 進行中かどうかの判定はドメイン（Status.IsTerminal の否定）が行うため、ここでは業務条件で絞り込まない。
-- 重複を除くため行数はステータスの種類数で頭打ちになり、購入件数には比例しない。
-- ステータスは購入ステータスマスタとの結合で解決する（JOIN の許容範囲は
-- internal/infrastructure/rdb/repository/README.md の Reference-master exception）。
SELECT DISTINCT ps.code
FROM purchases AS p
INNER JOIN purchase_statuses AS ps ON p.status_id = ps.id
WHERE p.user_id = sqlc.arg('user_id');

-- === source: database/dml/repository/purchase/select_shippable_purchases.sql ===
-- name: ListShippablePurchases :many
-- 発送可能な購入を、注文日時の古い順（同時刻は ID 昇順）で最大 limit 件取得する。
-- 現在状態は購入ステータスマスタとの結合で code を解決する（status_id は SoT、code は集約が
-- 状態機械の判定に用いる業務キー。JOIN の許容範囲は
-- internal/infrastructure/rdb/repository/README.md の Reference-master exception）。
-- 「発送可能」を定義するのは Purchase.IsShippable で、以下の条件はその実行形です。片方だけ変更しないこと。
-- 支払い済みを表す code は呼び出し側がドメイン定数から渡す。
SELECT
    ps.code AS status_code,
    sqlc.embed(p)
FROM purchases AS p
INNER JOIN purchase_statuses AS ps ON p.status_id = ps.id
WHERE ps.code = sqlc.arg('status_code')
ORDER BY p.ordered_at ASC, p.id ASC
LIMIT sqlc.arg('limit_param');

-- name: ListPurchaseDetailsByPurchaseIDs :many
-- 複数の購入 ID から明細をまとめて取得する。購入 1 件ずつの取得を件数分繰り返さないための一括版で、
-- 並びは購入 ID 昇順・同一購入内は明細 ID 昇順。purchase_ids が空の場合は 0 行。
SELECT sqlc.embed(d)
FROM purchase_details AS d
WHERE d.purchase_id = ANY(sqlc.arg('purchase_ids')::UUID[])
ORDER BY d.purchase_id, d.id;

-- === source: database/dml/repository/purchase/select_user_ids_with_purchases.sql ===
-- name: ListUserIDsWithPurchases :many
-- 与えたユーザー ID のうち、購入を 1 件以上持つものを返す。購入は独立集約のため、
-- ユーザー側の絞り込みと結合せず ID 群の照会として切り出す（docs/rules.md の Repository / QueryService Rules）。
SELECT DISTINCT user_id
FROM purchases
WHERE user_id = ANY(sqlc.arg('user_ids')::UUID[]);

-- === source: database/dml/repository/purchase/update_purchase_canceled.sql ===
-- name: UpdatePurchaseCanceled :exec
-- 購入をキャンセル状態へ更新する。status_id は code から解決する。canceled_at はドメインが決定した
-- 時刻（引数）を書き込み、イベント payload・レスポンスと同一時刻に揃える。
-- 遷移可否ガードは付けない（理由は docs/spec/domain/purchase.md の Repository Methods）。
UPDATE purchases
SET
    status_id = (
        SELECT ps.id FROM purchase_statuses AS ps
        WHERE ps.code = @status_code
    ),
    canceled_at = @canceled_at,
    updated_at = NOW()
WHERE purchases.id = @id;

-- === source: database/dml/repository/purchase/update_purchase_delivered.sql ===
-- name: UpdatePurchaseDelivered :exec
-- 購入を配達済み状態へ更新する。status_id は code から解決する。delivered_at はドメインが決定した時刻（引数）を
-- 書き込み、イベント payload・レスポンスと同一時刻に揃える。
-- 遷移可否ガードは付けない（理由は docs/spec/domain/purchase.md の Repository Methods）。
UPDATE purchases
SET
    status_id = (
        SELECT ps.id FROM purchase_statuses AS ps
        WHERE ps.code = @status_code
    ),
    delivered_at = @delivered_at,
    updated_at = NOW()
WHERE purchases.id = @id;

-- === source: database/dml/repository/purchase/update_purchase_paid.sql ===
-- name: UpdatePurchasePaid :exec
-- 購入を支払い済み状態へ更新する。status_id は code から解決する。paid_at はドメインが決定した時刻（引数）を
-- 書き込み、イベント payload・レスポンスと同一時刻に揃える。
-- 遷移可否ガードは付けない（理由は docs/spec/domain/purchase.md の Repository Methods）。
UPDATE purchases
SET
    status_id = (
        SELECT ps.id FROM purchase_statuses AS ps
        WHERE ps.code = @status_code
    ),
    paid_at = @paid_at,
    updated_at = NOW()
WHERE purchases.id = @id;

-- === source: database/dml/repository/purchase/update_purchase_shipped.sql ===
-- name: UpdatePurchaseShipped :exec
-- 購入を発送済み状態へ更新する。status_id は code から解決する。shipped_at はドメインが決定した時刻（引数）を
-- 書き込み、イベント payload・レスポンスと同一時刻に揃える。
-- 遷移可否ガードは付けない（理由は docs/spec/domain/purchase.md の Repository Methods）。
UPDATE purchases
SET
    status_id = (
        SELECT ps.id FROM purchase_statuses AS ps
        WHERE ps.code = @status_code
    ),
    shipped_at = @shipped_at,
    updated_at = NOW()
WHERE purchases.id = @id;
