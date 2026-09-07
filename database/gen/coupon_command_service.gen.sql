
-- === source: database/dml/command_service/coupon/insert_coupons.sql ===
-- name: InsertCoupons :execrows
-- 採番済みの id と受給者 user_id を 1 対 1 で zip し、同じ条件のクーポンを一括発行する
-- （2 文に分かれる理由と往復コストは ADR-0114 と ADR-0034 の Worked instances を参照）。
-- 発行経路ごとに複製せずこの 1 文を再利用する
-- （internal/infrastructure/rdb/README.md の command_service を参照）。
-- 2 つの配列は WITH ORDINALITY の行番号で突き合わせる（sqlc が 2 引数形の unnest を解決できない）。
-- 長さが食い違うと内部結合で余った側が落ちるため、呼び出し側が必ず同じ長さで渡す。
INSERT INTO coupons (
    id,
    user_id,
    discount_kind,
    discount_value,
    scope_kind,
    scope_target_id,
    expires_at,
    issued_at
)
SELECT
    ids.id,
    ids.user_id,
    sqlc.arg('discount_kind'),
    sqlc.arg('discount_value'),
    sqlc.arg('scope_kind'),
    sqlc.arg('scope_target_id'),
    sqlc.arg('expires_at'),
    sqlc.arg('issued_at')
FROM (
    SELECT
        i.id,
        u.user_id
    FROM UNNEST(sqlc.arg('ids')::UUID[]) WITH ORDINALITY AS i (id, ord)
    INNER JOIN UNNEST(sqlc.arg('user_ids')::UUID[]) WITH ORDINALITY AS u (user_id, ord)
        ON i.ord = u.ord
) AS ids;

-- === source: database/dml/command_service/coupon/select_bulk_issue_recipients.sql ===
-- name: SelectBulkIssueRecipients :many
-- 退会していないすべての利用者を受給者として返す。
-- 述語が退会の除外だけなのは、この操作が不特定多数への配布であるため。絞り込みの表現力は持たない
-- （述語と母集団は docs/spec/usecase/coupon.md の Workflow — IssuePromotionalCoupons の invariants を参照）。
SELECT u.id::UUID AS user_id
FROM users AS u
WHERE u.deleted_at IS NULL;
