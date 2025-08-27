-- name: CreateAnalysisMetadata :one
INSERT INTO analysis_metadata (
    node_id,
    session_id,
    tier,
    result,
    status,
    started_at,
    completed_at,
    created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, strftime('%s', 'now')
)
RETURNING *;

-- name: ListAnalysisByNode :many
SELECT *
FROM analysis_metadata
WHERE node_id = ?
ORDER BY created_at DESC;

-- name: ListAnalysisBySession :many
SELECT *
FROM analysis_metadata
WHERE session_id = ?
ORDER BY created_at DESC;

-- name: UpdateAnalysisStatus :one
UPDATE analysis_metadata
SET status = ?, result = ?, completed_at = ?, updated_at = strftime('%s', 'now')
WHERE id = ?
RETURNING *;

-- name: ListAnalysisByTier :many
SELECT *
FROM analysis_metadata
WHERE tier = ?
ORDER BY created_at DESC;

-- name: ListAnalysisByStatus :many
SELECT *
FROM analysis_metadata
WHERE status = ?
ORDER BY created_at DESC;

-- name: GetLatestAnalysisByNode :one
SELECT *
FROM analysis_metadata
WHERE node_id = ? AND tier = ?
ORDER BY created_at DESC
LIMIT 1;

-- name: CountAnalysisByTier :one
SELECT COUNT(*)
FROM analysis_metadata
WHERE tier = ? AND session_id = ?;

-- name: CountAnalysisByStatus :one
SELECT COUNT(*)
FROM analysis_metadata
WHERE status = ? AND session_id = ?;

-- name: ListPendingAnalysis :many
SELECT *
FROM analysis_metadata
WHERE status = 'pending' OR status = 'running'
ORDER BY created_at ASC;

-- name: DeleteAnalysisMetadata :exec
DELETE FROM analysis_metadata
WHERE id = ?;

-- name: DeleteAnalysisByNode :exec
DELETE FROM analysis_metadata
WHERE node_id = ?;

-- name: DeleteAnalysisBySession :exec
DELETE FROM analysis_metadata
WHERE session_id = ?;
