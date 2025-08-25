-- +goose Up
-- +goose StatementBegin
-- Migration: create code_nodes table
-- Generated: 2025-08-25

CREATE TABLE IF NOT EXISTS code_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    path TEXT NOT NULL,
    language TEXT,
    symbol TEXT,
    kind TEXT,
    start_line INTEGER,
    end_line INTEGER,
    metadata JSON DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_code_nodes_session ON code_nodes(session_id);
CREATE INDEX IF NOT EXISTS idx_code_nodes_path ON code_nodes(path);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_code_nodes_session;
DROP INDEX IF EXISTS idx_code_nodes_path;
DROP TABLE IF EXISTS code_nodes;
-- +goose StatementEnd
