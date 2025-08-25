-- +goose Up
-- +goose StatementBegin
-- Migration: create embeddings table
-- Generated: 2025-08-25

CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id INTEGER NOT NULL,
    session_id TEXT,
    vector_id TEXT,
    vector BLOB,
    dims INTEGER,
    metadata JSON DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_node FOREIGN KEY(node_id) REFERENCES code_nodes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_embeddings_node ON embeddings(node_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_vector_id ON embeddings(vector_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_embeddings_node;
DROP INDEX IF EXISTS idx_embeddings_vector_id;
DROP TABLE IF EXISTS embeddings;
-- +goose StatementEnd
