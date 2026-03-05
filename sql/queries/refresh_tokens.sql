-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens(token, user_id, expires_at)
VALUES($1, $2, $3);

-- name: GetUserFromRefreshToken :one
SELECT refresh_tokens.* FROM refresh_tokens
WHERE token = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = now(), updated_at = now()
WHERE token = $1;