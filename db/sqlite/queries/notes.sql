-- name: CreateNote :one
INSERT INTO notes (id, title, body, created_at)
VALUES (?, ?, ?, ?)
RETURNING id, title, body, created_at;

-- name: GetNoteByID :one
SELECT id, title, body, created_at
FROM notes
WHERE id = ?;

-- name: ListNotes :many
SELECT id, title, body, created_at
FROM notes
ORDER BY created_at DESC
LIMIT ? OFFSET ?;
