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
