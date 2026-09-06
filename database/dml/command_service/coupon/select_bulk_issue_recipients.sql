-- name: SelectBulkIssueRecipients :many
-- 退会していないすべての利用者を受給者として返す。
-- 述語が退会の除外だけなのは、この操作が不特定多数への配布であるため。絞り込みの表現力は持たない
-- （docs/spec/usecase/coupon.md の Workflow — IssuePromotionalCoupons を参照）。
-- 退会の除外は廃番の一括発行と同一の述語で、受給者の母集団を 2 つの発行経路で揃える。
SELECT u.id::UUID AS user_id
FROM users AS u
WHERE u.deleted_at IS NULL;
