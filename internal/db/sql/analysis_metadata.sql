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
