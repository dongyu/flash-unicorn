-- name: CreateNote :one
INSERT INTO notes (id, title, body, created_at)
VALUES ($1, $2, $3, $4)
RETURNING id, title, body, created_at;

-- name: GetNoteByID :one
SELECT id, title, body, created_at
FROM notes
WHERE id = $1;

-- name: ListNotes :many
SELECT id, title, body, created_at
FROM notes
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
