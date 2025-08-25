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
