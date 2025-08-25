-- name: CreateDependency :one
INSERT INTO dependencies (
    from_node,
    to_node,
    relation,
    metadata,
    created_at
) VALUES (
    ?, ?, ?, ?, strftime('%s', 'now')
)
RETURNING *;

-- name: ListDependenciesFrom :many
SELECT *
FROM dependencies
WHERE from_node = ?
ORDER BY id;

-- name: ListDependenciesTo :many
SELECT *
FROM dependencies
WHERE to_node = ?
ORDER BY id;

-- name: DeleteDependency :exec
DELETE FROM dependencies
WHERE id = ?;
