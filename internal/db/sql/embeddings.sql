-- name: CreateEmbedding :one
INSERT INTO embeddings (
    node_id,
    session_id,
    vector_id,
    vector,
    dims,
    metadata,
    created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, strftime('%s', 'now')
)
RETURNING *;

-- name: GetEmbeddingsByNode :many
SELECT *
FROM embeddings
WHERE node_id = ?
ORDER BY created_at DESC;

-- name: GetEmbeddingByVectorID :one
SELECT *
FROM embeddings
WHERE vector_id = ?
LIMIT 1;

-- name: DeleteEmbedding :exec
DELETE FROM embeddings
WHERE id = ?;

-- name: GetEmbedding :one
SELECT *
FROM embeddings
WHERE id = ?
LIMIT 1;

-- name: GetEmbeddingByNode :one
SELECT *
FROM embeddings
WHERE node_id = ?
LIMIT 1;

-- name: ListEmbeddingsBySession :many
SELECT *
FROM embeddings
WHERE session_id = ?
ORDER BY created_at DESC;

-- name: ListEmbeddingsByDims :many
SELECT *
FROM embeddings
WHERE dims = ?
ORDER BY created_at DESC;

-- name: UpdateEmbedding :one
UPDATE embeddings
SET vector_id = ?, vector = ?, dims = ?, metadata = ?
WHERE id = ?
RETURNING *;

-- name: DeleteEmbeddingByNode :exec
DELETE FROM embeddings
WHERE node_id = ?;

-- name: DeleteEmbeddingsBySession :exec
DELETE FROM embeddings
WHERE session_id = ?;

-- name: CountEmbeddingsBySession :one
SELECT COUNT(*)
FROM embeddings
WHERE session_id = ?;
