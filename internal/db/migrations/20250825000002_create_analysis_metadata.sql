-- +goose Up
-- +goose StatementBegin
-- Migration: create analysis_metadata table
-- Generated: 2025-08-25

CREATE TABLE IF NOT EXISTS analysis_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id INTEGER,
    session_id TEXT,
    tier INTEGER NOT NULL,
    result JSON DEFAULT '{}',
    status TEXT,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_node FOREIGN KEY(node_id) REFERENCES code_nodes(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_analysis_metadata_node ON analysis_metadata(node_id);
CREATE INDEX IF NOT EXISTS idx_analysis_metadata_session ON analysis_metadata(session_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_analysis_metadata_node;
DROP INDEX IF EXISTS idx_analysis_metadata_session;
DROP TABLE IF EXISTS analysis_metadata;
-- +goose StatementEnd
