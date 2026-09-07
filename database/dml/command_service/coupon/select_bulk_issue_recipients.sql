-- name: SelectBulkIssueRecipients :many
-- 退会していないすべての利用者を受給者として返す。
-- 述語が退会の除外だけなのは、この操作が不特定多数への配布であるため。絞り込みの表現力は持たない
-- （述語と母集団は docs/spec/usecase/coupon.md の IssuePromotionalCoupons invariants を参照）。
SELECT u.id::UUID AS user_id
FROM users AS u
WHERE u.deleted_at IS NULL;
