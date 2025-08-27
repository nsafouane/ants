-- +goose Up
-- +goose StatementBegin
-- Migration: Add performance indexes for Context Engine
-- Generated: 2025-08-27

-- Code nodes performance indexes
CREATE INDEX IF NOT EXISTS idx_code_nodes_kind_symbol ON code_nodes(kind, symbol);
CREATE INDEX IF NOT EXISTS idx_code_nodes_language ON code_nodes(language);
CREATE INDEX IF NOT EXISTS idx_code_nodes_path_session ON code_nodes(path, session_id);

-- Dependencies performance indexes
CREATE INDEX IF NOT EXISTS idx_dependencies_relation ON dependencies(relation);
CREATE INDEX IF NOT EXISTS idx_dependencies_from_relation ON dependencies(from_node, relation);
CREATE INDEX IF NOT EXISTS idx_dependencies_to_relation ON dependencies(to_node, relation);

-- Analysis metadata performance indexes
CREATE INDEX IF NOT EXISTS idx_analysis_tier_status ON analysis_metadata(tier, status);
CREATE INDEX IF NOT EXISTS idx_analysis_session_tier ON analysis_metadata(session_id, tier);
CREATE INDEX IF NOT EXISTS idx_analysis_started_at ON analysis_metadata(started_at);
CREATE INDEX IF NOT EXISTS idx_analysis_completed_at ON analysis_metadata(completed_at);

-- Embeddings performance indexes
CREATE INDEX IF NOT EXISTS idx_embeddings_session ON embeddings(session_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_dims ON embeddings(dims);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop performance indexes
DROP INDEX IF EXISTS idx_code_nodes_kind_symbol;
DROP INDEX IF EXISTS idx_code_nodes_language;
DROP INDEX IF EXISTS idx_code_nodes_path_session;
DROP INDEX IF EXISTS idx_dependencies_relation;
DROP INDEX IF EXISTS idx_dependencies_from_relation;
DROP INDEX IF EXISTS idx_dependencies_to_relation;
DROP INDEX IF EXISTS idx_analysis_tier_status;
DROP INDEX IF EXISTS idx_analysis_session_tier;
DROP INDEX IF EXISTS idx_analysis_started_at;
DROP INDEX IF EXISTS idx_analysis_completed_at;
DROP INDEX IF EXISTS idx_embeddings_session;
DROP INDEX IF EXISTS idx_embeddings_dims;
-- +goose StatementEnd