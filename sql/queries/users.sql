-- name: CreateUser :one
INSERT INTO users (email)
VALUES ($1)
RETURNING *;

-- name: DeleteUsers :exec
DELETE FROM users;