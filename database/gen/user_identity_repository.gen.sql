
-- === source: database/dml/repository/user_identity/insert_user_identity.sql ===
-- name: CreateUserIdentity :exec
INSERT INTO user_identities (
    id,
    user_id,
    issuer,
    subject
) VALUES
(
    sqlc.arg('id'),
    sqlc.arg('user_id'),
    sqlc.arg('issuer'),
    sqlc.arg('subject')
);

-- === source: database/dml/repository/user_identity/resolve_user_by_identity.sql ===
-- name: ResolveUserByIdentity :one
SELECT
    u.id,
    u.deleted_at
FROM user_identities AS ui
INNER JOIN users AS u ON ui.user_id = u.id
WHERE ui.issuer = sqlc.arg('issuer_param')
    AND ui.subject = sqlc.arg('subject_param');
