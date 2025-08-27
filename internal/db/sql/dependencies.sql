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

-- name: DeleteDependenciesByNode :exec
DELETE FROM dependencies
WHERE from_node = ? OR to_node = ?;

-- name: ListDependenciesByRelation :many
SELECT *
FROM dependencies
WHERE relation = ?
ORDER BY id;

-- name: GetDependency :one
SELECT *
FROM dependencies
WHERE from_node = ? AND to_node = ? AND relation = ?
LIMIT 1;

-- name: CountDependenciesFrom :one
SELECT COUNT(*)
FROM dependencies
WHERE from_node = ?;

-- name: CountDependenciesTo :one
SELECT COUNT(*)
FROM dependencies
WHERE to_node = ?;

-- name: ListAllDependencies :many
SELECT d.*, 
       cn1.path as from_path, cn1.symbol as from_symbol,
       cn2.path as to_path, cn2.symbol as to_symbol
FROM dependencies d
JOIN code_nodes cn1 ON d.from_node = cn1.id
JOIN code_nodes cn2 ON d.to_node = cn2.id
ORDER BY d.id;
