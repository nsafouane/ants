-- name: CreateCodeNode :one
INSERT INTO code_nodes (
    session_id,
    path,
    language,
    symbol,
    kind,
    start_line,
    end_line,
    metadata,
    created_at,
    updated_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now'), strftime('%s', 'now')
)
RETURNING *;

-- name: GetCodeNode :one
SELECT *
FROM code_nodes
WHERE id = ?
LIMIT 1;

-- name: ListCodeNodesBySession :many
SELECT *
FROM code_nodes
WHERE session_id = ?
ORDER BY path, start_line;

-- name: ListCodeNodesByPath :many
SELECT *
FROM code_nodes
WHERE path = ?
ORDER BY start_line;

-- name: UpdateCodeNode :one
UPDATE code_nodes
SET language = ?, symbol = ?, kind = ?, start_line = ?, end_line = ?, metadata = ?, updated_at = strftime('%s', 'now')
WHERE id = ?
RETURNING *;

-- name: DeleteCodeNode :exec
DELETE FROM code_nodes
WHERE id = ?;

-- name: DeleteCodeNodesByPath :exec
DELETE FROM code_nodes
WHERE path = ?;

-- name: DeleteCodeNodesBySession :exec
DELETE FROM code_nodes
WHERE session_id = ?;

-- name: ListCodeNodesByKind :many
SELECT *
FROM code_nodes
WHERE kind = ?
ORDER BY path, start_line;

-- name: ListCodeNodesByLanguage :many
SELECT *
FROM code_nodes
WHERE language = ?
ORDER BY path, start_line;

-- name: SearchCodeNodesBySymbol :many
SELECT *
FROM code_nodes
WHERE symbol LIKE ? || '%'
ORDER BY path, start_line;

-- name: CountCodeNodesBySession :one
SELECT COUNT(*)
FROM code_nodes
WHERE session_id = ?;

-- name: CountCodeNodesByKind :one
SELECT COUNT(*)
FROM code_nodes
WHERE kind = ? AND session_id = ?;

-- name: GetCodeNodeByPathAndSymbol :one
SELECT *
FROM code_nodes
WHERE path = ? AND symbol = ?
LIMIT 1;

-- name: ListCodeNodesInRange :many
SELECT *
FROM code_nodes
WHERE path = ? AND start_line <= ? AND end_line >= ?
ORDER BY start_line;
