-- +goose Up
-- +goose StatementBegin
-- Migration: create dependencies table
-- Generated: 2025-08-25

CREATE TABLE IF NOT EXISTS dependencies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_node INTEGER NOT NULL,
    to_node INTEGER NOT NULL,
    relation TEXT,
    metadata JSON DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_from_node FOREIGN KEY(from_node) REFERENCES code_nodes(id) ON DELETE CASCADE,
    CONSTRAINT fk_to_node FOREIGN KEY(to_node) REFERENCES code_nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_dependencies_from ON dependencies(from_node);
CREATE INDEX IF NOT EXISTS idx_dependencies_to ON dependencies(to_node);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_dependencies_from;
DROP INDEX IF EXISTS idx_dependencies_to;
DROP TABLE IF EXISTS dependencies;
-- +goose StatementEnd
