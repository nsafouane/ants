-- Telemetry and Metrics SQL Queries

-- name: InsertMetricData :exec
-- Insert a new metric data point
INSERT INTO metrics_data (
    metric_name, metric_type, value, unit, session_id, component, tags, timestamp
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: GetMetricData :many
-- Get metric data points for a specific metric within time range
SELECT 
    id, metric_name, metric_type, value, unit, session_id, component, tags, timestamp, created_at
FROM metrics_data 
WHERE metric_name = ?
  AND timestamp >= ? 
  AND timestamp <= ?
ORDER BY timestamp DESC
LIMIT ?;

-- name: GetMetricDataByComponent :many
-- Get metric data points for a specific component within time range
SELECT 
    id, metric_name, metric_type, value, unit, session_id, component, tags, timestamp, created_at
FROM metrics_data 
WHERE component = ?
  AND timestamp >= ? 
  AND timestamp <= ?
ORDER BY timestamp DESC
LIMIT ?;

-- name: GetRecentMetricsForSession :many
-- Get recent metrics for a specific session
SELECT 
    id, metric_name, metric_type, value, unit, session_id, component, tags, timestamp, created_at
FROM metrics_data 
WHERE session_id = ?
  AND timestamp >= ?
ORDER BY timestamp DESC
LIMIT ?;

-- name: GetMetricsSummary :many
-- Get metrics summary from the view
SELECT 
    metric_name, metric_type, unit, component, data_points, 
    avg_value, min_value, max_value, last_updated
FROM recent_metrics_summary;

-- name: CleanOldMetrics :exec
-- Delete old metric data points based on retention period
DELETE FROM metrics_data 
WHERE timestamp < ?;

-- name: InsertMetricMetadata :exec
-- Insert or update metric metadata
INSERT INTO metrics_metadata (
    metric_name, metric_type, description, unit, component, enabled, retention_days
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
) ON CONFLICT(metric_name) DO UPDATE SET
    metric_type = excluded.metric_type,
    description = excluded.description,
    unit = excluded.unit,
    component = excluded.component,
    enabled = excluded.enabled,
    retention_days = excluded.retention_days,
    updated_at = CURRENT_TIMESTAMP;

-- name: GetMetricMetadata :one
-- Get metadata for a specific metric
SELECT 
    id, metric_name, metric_type, description, unit, component, 
    enabled, retention_days, created_at, updated_at
FROM metrics_metadata 
WHERE metric_name = ?;

-- name: GetAllMetricMetadata :many
-- Get all metric metadata
SELECT 
    id, metric_name, metric_type, description, unit, component, 
    enabled, retention_days, created_at, updated_at
FROM metrics_metadata 
WHERE enabled = TRUE
ORDER BY component, metric_name;

-- name: UpdateMetricEnabled :exec
-- Enable or disable a metric
UPDATE metrics_metadata 
SET enabled = ?, updated_at = CURRENT_TIMESTAMP 
WHERE metric_name = ?;

-- name: InsertPerformanceBenchmark :exec
-- Insert a performance benchmark
INSERT INTO performance_benchmarks (
    metric_name, target_value, threshold_warning, threshold_critical, 
    unit, description, active
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
) ON CONFLICT(metric_name) DO UPDATE SET
    target_value = excluded.target_value,
    threshold_warning = excluded.threshold_warning,
    threshold_critical = excluded.threshold_critical,
    unit = excluded.unit,
    description = excluded.description,
    active = excluded.active,
    updated_at = CURRENT_TIMESTAMP;

-- name: GetPerformanceBenchmark :one
-- Get performance benchmark for a metric
SELECT 
    id, metric_name, target_value, threshold_warning, threshold_critical,
    unit, description, active, created_at, updated_at
FROM performance_benchmarks 
WHERE metric_name = ? AND active = TRUE;

-- name: GetAllPerformanceBenchmarks :many
-- Get all active performance benchmarks
SELECT 
    id, metric_name, target_value, threshold_warning, threshold_critical,
    unit, description, active, created_at, updated_at
FROM performance_benchmarks 
WHERE active = TRUE
ORDER BY metric_name;

-- name: UpdateSystemHealthStatus :exec
-- Update system health status for a component
INSERT INTO system_health (
    component, status, details, last_check
) VALUES (
    ?, ?, ?, ?
) ON CONFLICT(component) DO UPDATE SET
    status = excluded.status,
    details = excluded.details,
    last_check = excluded.last_check,
    updated_at = CURRENT_TIMESTAMP;

-- name: GetSystemHealth :many
-- Get current system health for all components
SELECT 
    id, component, status, details, last_check, created_at, updated_at
FROM system_health 
WHERE last_check > ?
ORDER BY component;

-- name: GetSystemHealthOverview :many
-- Get system health overview from view
SELECT 
    component, status, details, last_check, status_priority
FROM system_health_overview;

-- name: GetComponentHealth :one
-- Get health status for a specific component
SELECT 
    id, component, status, details, last_check, created_at, updated_at
FROM system_health 
WHERE component = ?
ORDER BY last_check DESC 
LIMIT 1;

-- name: InsertPerformanceAlert :one
-- Insert a new performance alert
INSERT INTO performance_alerts (
    alert_type, metric_name, current_value, threshold_value, severity,
    message, component, session_id
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?
) RETURNING id;

-- name: GetActiveAlerts :many
-- Get all active (unresolved) alerts
SELECT 
    id, alert_type, metric_name, current_value, threshold_value, severity,
    message, component, session_id, acknowledged, acknowledged_by, acknowledged_at,
    resolved, resolved_at, created_at
FROM performance_alerts 
WHERE resolved = FALSE
ORDER BY severity, created_at DESC;

-- name: GetActiveAlertsByComponent :many
-- Get active alerts for a specific component
SELECT 
    id, alert_type, metric_name, current_value, threshold_value, severity,
    message, component, session_id, acknowledged, acknowledged_by, acknowledged_at,
    resolved, resolved_at, created_at
FROM performance_alerts 
WHERE component = ? AND resolved = FALSE
ORDER BY severity, created_at DESC;

-- name: GetAlertsFromView :many
-- Get active alerts using the view
SELECT 
    alert_type, metric_name, current_value, threshold_value, severity,
    message, component, session_id, created_at, severity_priority
FROM active_alerts;

-- name: AcknowledgeAlert :exec
-- Acknowledge an alert
UPDATE performance_alerts 
SET acknowledged = TRUE, acknowledged_by = ?, acknowledged_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: ResolveAlert :exec
-- Mark an alert as resolved
UPDATE performance_alerts 
SET resolved = TRUE, resolved_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: GetAlertHistory :many
-- Get alert history for a metric
SELECT 
    id, alert_type, metric_name, current_value, threshold_value, severity,
    message, component, session_id, acknowledged, acknowledged_by, acknowledged_at,
    resolved, resolved_at, created_at
FROM performance_alerts 
WHERE metric_name = ?
ORDER BY created_at DESC
LIMIT ?;

-- name: CountActiveAlertsBySeverity :many
-- Count active alerts by severity level
SELECT 
    severity, COUNT(*) as alert_count
FROM performance_alerts 
WHERE resolved = FALSE
GROUP BY severity
ORDER BY 
    CASE severity
        WHEN 'critical' THEN 1
        WHEN 'high' THEN 2
        WHEN 'medium' THEN 3
        WHEN 'low' THEN 4
    END;

-- name: GetMetricStatistics :one
-- Get statistics for a specific metric over time period
SELECT 
    COUNT(*) as data_points,
    AVG(value) as avg_value,
    MIN(value) as min_value,
    MAX(value) as max_value,
    MIN(timestamp) as first_recorded,
    MAX(timestamp) as last_recorded
FROM metrics_data 
WHERE metric_name = ?
  AND timestamp >= ? 
  AND timestamp <= ?;

-- name: GetTopMetricsByValue :many
-- Get top metrics by value for a specific metric type
SELECT 
    metric_name, component, value, timestamp
FROM metrics_data 
WHERE metric_type = ?
  AND timestamp >= ?
ORDER BY value DESC
LIMIT ?;

-- name: GetComponentMetricsSummary :many
-- Get metrics summary for a specific component
SELECT 
    metric_name, metric_type, unit,
    COUNT(*) as data_points,
    AVG(value) as avg_value,
    MIN(value) as min_value,
    MAX(value) as max_value,
    MAX(timestamp) as last_updated
FROM metrics_data 
WHERE component = ?
  AND timestamp >= ?
GROUP BY metric_name, metric_type, unit
ORDER BY metric_name;

-- name: GetDatabaseSize :one
-- Get current database size statistics
SELECT 
    (SELECT COUNT(*) FROM metrics_data) as total_metrics,
    (SELECT COUNT(*) FROM performance_alerts) as total_alerts,
    (SELECT COUNT(*) FROM system_health) as health_components,
    (SELECT COUNT(DISTINCT component) FROM metrics_data) as unique_components,
    (SELECT COUNT(DISTINCT metric_name) FROM metrics_data) as unique_metrics;