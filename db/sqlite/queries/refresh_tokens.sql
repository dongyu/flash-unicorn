-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, revoked_at, rotated_from_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = ?;

-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokensForUser :execrows
UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL;
