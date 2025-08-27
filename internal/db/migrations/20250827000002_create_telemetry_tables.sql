-- +goose Up
-- +goose StatementBegin
-- Migration: Create telemetry and metrics tables for Context Engine monitoring
-- Generated: 2025-08-27

-- Create metrics data table for storing time-series metrics
CREATE TABLE IF NOT EXISTS metrics_data (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_name TEXT NOT NULL,
    metric_type TEXT NOT NULL CHECK (metric_type IN ('timing', 'counter', 'gauge', 'rate', 'status')),
    value REAL NOT NULL,
    unit TEXT,
    session_id TEXT,
    component TEXT, -- Which component generated the metric
    tags TEXT, -- JSON string of additional tags
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create metrics metadata table for metric definitions and configuration
CREATE TABLE IF NOT EXISTS metrics_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_name TEXT NOT NULL UNIQUE,
    metric_type TEXT NOT NULL,
    description TEXT,
    unit TEXT,
    component TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    retention_days INTEGER DEFAULT 30,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create performance benchmarks table for tracking performance targets
CREATE TABLE IF NOT EXISTS performance_benchmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    metric_name TEXT NOT NULL,
    target_value REAL NOT NULL,
    threshold_warning REAL,
    threshold_critical REAL,
    unit TEXT,
    description TEXT,
    active BOOLEAN DEFAULT TRUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create system health status table
CREATE TABLE IF NOT EXISTS system_health (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    component TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('healthy', 'degraded', 'unhealthy', 'unknown')),
    details TEXT, -- JSON string with detailed status information
    last_check DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create alerts table for tracking performance alerts and notifications
CREATE TABLE IF NOT EXISTS performance_alerts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alert_type TEXT NOT NULL, -- 'threshold_exceeded', 'performance_degraded', 'system_error'
    metric_name TEXT NOT NULL,
    current_value REAL,
    threshold_value REAL,
    severity TEXT NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    message TEXT NOT NULL,
    component TEXT,
    session_id TEXT,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_by TEXT,
    acknowledged_at DATETIME,
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient querying

-- Metrics data indexes
CREATE INDEX IF NOT EXISTS idx_metrics_data_name_timestamp ON metrics_data(metric_name, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_data_type_timestamp ON metrics_data(metric_type, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_data_session ON metrics_data(session_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_data_component ON metrics_data(component, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_data_timestamp ON metrics_data(timestamp DESC);

-- System health indexes
CREATE INDEX IF NOT EXISTS idx_system_health_component ON system_health(component, last_check DESC);
CREATE INDEX IF NOT EXISTS idx_system_health_status ON system_health(status, last_check DESC);

-- Performance alerts indexes
CREATE INDEX IF NOT EXISTS idx_alerts_severity_created ON performance_alerts(severity, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_metric_created ON performance_alerts(metric_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_unresolved ON performance_alerts(resolved, created_at DESC) WHERE resolved = FALSE;
CREATE INDEX IF NOT EXISTS idx_alerts_session ON performance_alerts(session_id, created_at DESC);

-- Insert default metrics metadata for Context Engine KPIs
INSERT OR IGNORE INTO metrics_metadata (metric_name, metric_type, description, unit, component) VALUES
-- Tier 1 Analysis Metrics
('tier_1_analysis_time_seconds', 'timing', 'Time taken for Tier 1 analysis operations', 'seconds', 'analysis_engine'),
('tier_1_success_count', 'counter', 'Number of successful Tier 1 analysis operations', 'count', 'analysis_engine'),
('tier_1_failure_count', 'counter', 'Number of failed Tier 1 analysis operations', 'count', 'analysis_engine'),

-- AI Worker Query Metrics  
('ai_worker_query_latency_ms', 'timing', 'AI worker query response latency', 'milliseconds', 'ai_worker'),
('ai_worker_requests_total', 'counter', 'Total AI worker requests', 'count', 'ai_worker'),
('ai_worker_errors_total', 'counter', 'Total AI worker errors', 'count', 'ai_worker'),

-- Knowledge Graph Metrics
('knowledge_graph_size_mb', 'gauge', 'Knowledge graph database size', 'megabytes', 'knowledge_graph'),
('knowledge_graph_nodes_count', 'gauge', 'Number of nodes in knowledge graph', 'count', 'knowledge_graph'),
('knowledge_graph_dependencies_count', 'gauge', 'Number of dependencies in knowledge graph', 'count', 'knowledge_graph'),

-- Vector Database Metrics
('vector_db_index_status', 'status', 'Vector database index health status', 'status', 'vector_db'),
('vector_db_query_latency_ms', 'timing', 'Vector database query latency', 'milliseconds', 'vector_db'),
('vector_db_embeddings_count', 'gauge', 'Number of embeddings in vector database', 'count', 'vector_db'),

-- Cache Metrics
('cache_hit_rate_percent', 'rate', 'Cache hit rate percentage', 'percent', 'cache'),
('cache_size_mb', 'gauge', 'Total cache size', 'megabytes', 'cache'),
('cache_evictions_total', 'counter', 'Total cache evictions', 'count', 'cache'),

-- System Resource Metrics
('memory_usage_mb', 'gauge', 'System memory usage', 'megabytes', 'system'),
('cpu_usage_percent', 'gauge', 'System CPU usage percentage', 'percent', 'system'),
('disk_usage_mb', 'gauge', 'Disk space usage', 'megabytes', 'system'),
('active_workers_count', 'gauge', 'Number of active worker threads', 'count', 'system'),

-- Search Performance Metrics
('search_latency_ms', 'timing', 'Search operation latency', 'milliseconds', 'search'),
('search_accuracy_percent', 'rate', 'Search result accuracy percentage', 'percent', 'search'),
('indexing_throughput_docs_per_sec', 'rate', 'Document indexing throughput', 'docs_per_second', 'search');

-- Insert default performance benchmarks
INSERT OR IGNORE INTO performance_benchmarks (metric_name, target_value, threshold_warning, threshold_critical, unit, description) VALUES
-- Analysis Performance Targets
('tier_1_analysis_time_seconds', 5.0, 8.0, 15.0, 'seconds', 'Tier 1 analysis should complete within 5 seconds'),
('ai_worker_query_latency_ms', 2000.0, 5000.0, 10000.0, 'milliseconds', 'AI worker queries should respond within 2 seconds'),

-- Search Performance Targets  
('search_latency_ms', 200.0, 500.0, 1000.0, 'milliseconds', 'Search operations should complete within 200ms'),
('vector_db_query_latency_ms', 100.0, 300.0, 1000.0, 'milliseconds', 'Vector database queries should complete within 100ms'),

-- Cache Performance Targets
('cache_hit_rate_percent', 80.0, 60.0, 40.0, 'percent', 'Cache hit rate should be above 80%'),

-- System Resource Targets
('memory_usage_mb', 512.0, 768.0, 1024.0, 'megabytes', 'Memory usage should stay below 512MB in performance mode'),
('cpu_usage_percent', 70.0, 85.0, 95.0, 'percent', 'CPU usage should stay below 70%'),

-- Quality Targets
('search_accuracy_percent', 85.0, 70.0, 50.0, 'percent', 'Search accuracy should be above 85%'),
('indexing_throughput_docs_per_sec', 25.0, 15.0, 5.0, 'docs_per_second', 'Indexing throughput should be above 25 docs/second');

-- Create triggers for maintaining metrics metadata
CREATE TRIGGER IF NOT EXISTS update_metrics_metadata_timestamp
AFTER UPDATE ON metrics_metadata
BEGIN
    UPDATE metrics_metadata 
    SET updated_at = CURRENT_TIMESTAMP 
    WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_performance_benchmarks_timestamp
AFTER UPDATE ON performance_benchmarks
BEGIN
    UPDATE performance_benchmarks 
    SET updated_at = CURRENT_TIMESTAMP 
    WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_system_health_timestamp
AFTER UPDATE ON system_health
BEGIN
    UPDATE system_health 
    SET updated_at = CURRENT_TIMESTAMP 
    WHERE id = NEW.id;
END;

-- Create view for current system health overview
CREATE VIEW IF NOT EXISTS system_health_overview AS
SELECT 
    component,
    status,
    details,
    last_check,
    CASE 
        WHEN status = 'healthy' THEN 1
        WHEN status = 'degraded' THEN 2
        WHEN status = 'unhealthy' THEN 3
        ELSE 4
    END as status_priority
FROM system_health
WHERE last_check > datetime('now', '-5 minutes')
ORDER BY status_priority DESC, last_check DESC;

-- Create view for active performance alerts
CREATE VIEW IF NOT EXISTS active_alerts AS
SELECT 
    alert_type,
    metric_name,
    current_value,
    threshold_value,
    severity,
    message,
    component,
    session_id,
    created_at,
    CASE severity
        WHEN 'critical' THEN 1
        WHEN 'high' THEN 2  
        WHEN 'medium' THEN 3
        WHEN 'low' THEN 4
    END as severity_priority
FROM performance_alerts
WHERE resolved = FALSE
ORDER BY severity_priority ASC, created_at DESC;

-- Create view for recent metrics summary
CREATE VIEW IF NOT EXISTS recent_metrics_summary AS
SELECT 
    m.metric_name,
    m.metric_type,
    m.unit,
    m.component,
    COUNT(d.id) as data_points,
    AVG(d.value) as avg_value,
    MIN(d.value) as min_value,
    MAX(d.value) as max_value,
    MAX(d.timestamp) as last_updated
FROM metrics_metadata m
LEFT JOIN metrics_data d ON m.metric_name = d.metric_name 
    AND d.timestamp > datetime('now', '-1 hour')
WHERE m.enabled = TRUE
GROUP BY m.metric_name, m.metric_type, m.unit, m.component
ORDER BY m.component, m.metric_name;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop telemetry tables and related structures

-- Drop views first (depend on tables)
DROP VIEW IF EXISTS recent_metrics_summary;
DROP VIEW IF EXISTS active_alerts;
DROP VIEW IF EXISTS system_health_overview;

-- Drop triggers
DROP TRIGGER IF EXISTS update_system_health_timestamp;
DROP TRIGGER IF EXISTS update_performance_benchmarks_timestamp;
DROP TRIGGER IF EXISTS update_metrics_metadata_timestamp;

-- Drop indexes
DROP INDEX IF EXISTS idx_alerts_session;
DROP INDEX IF EXISTS idx_alerts_unresolved;
DROP INDEX IF EXISTS idx_alerts_metric_created;
DROP INDEX IF EXISTS idx_alerts_severity_created;
DROP INDEX IF EXISTS idx_system_health_status;
DROP INDEX IF EXISTS idx_system_health_component;
DROP INDEX IF EXISTS idx_metrics_data_timestamp;
DROP INDEX IF EXISTS idx_metrics_data_component;
DROP INDEX IF EXISTS idx_metrics_data_session;
DROP INDEX IF EXISTS idx_metrics_data_type_timestamp;
DROP INDEX IF EXISTS idx_metrics_data_name_timestamp;

-- Drop tables
DROP TABLE IF EXISTS performance_alerts;
DROP TABLE IF EXISTS system_health;
DROP TABLE IF EXISTS performance_benchmarks;
DROP TABLE IF EXISTS metrics_metadata;
DROP TABLE IF EXISTS metrics_data;

-- +goose StatementEnd