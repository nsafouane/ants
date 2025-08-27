-- +goose Up
-- +goose StatementBegin
-- Migration: Enable FTS5 extension and create virtual tables for code search
-- Generated: 2025-08-27

-- Enable FTS5 extension (if not already enabled)
-- Note: FTS5 is typically built into modern SQLite distributions

-- Create FTS5 virtual table for code content search
CREATE VIRTUAL TABLE IF NOT EXISTS code_content_fts USING fts5(
    -- Content columns
    content,              -- Full code content
    symbol,               -- Function/class/variable names
    comments,             -- Extracted comments and documentation
    identifiers,          -- All identifiers (variables, function names, etc.)
    
    -- Metadata columns (not indexed for FTS, but stored)
    node_id UNINDEXED,    -- Reference to code_nodes.id
    session_id UNINDEXED, -- Reference to session
    path UNINDEXED,       -- File path
    language UNINDEXED,   -- Programming language
    kind UNINDEXED,       -- Code element type (function, class, etc.)
    
    -- FTS5 configuration options
    tokenize='porter unicode61 remove_diacritics 1 tokenchars "-_"',
    content='', -- External content table (we'll populate manually)
    contentless_delete=1 -- Enable deletion by rowid
);

-- Create FTS5 virtual table for file-level search
CREATE VIRTUAL TABLE IF NOT EXISTS file_content_fts USING fts5(
    -- Content columns  
    content,              -- Full file content
    filename,             -- File name only (not full path)
    
    -- Metadata columns
    session_id UNINDEXED, -- Reference to session
    path UNINDEXED,       -- Full file path
    language UNINDEXED,   -- Programming language
    file_size UNINDEXED,  -- File size in bytes
    
    -- FTS5 configuration
    tokenize='porter unicode61 remove_diacritics 1 tokenchars "-_."',
    content='',
    contentless_delete=1
);

-- Create FTS5 virtual table for documentation and comments
CREATE VIRTUAL TABLE IF NOT EXISTS documentation_fts USING fts5(
    -- Content columns
    title,                -- Documentation title/heading
    content,              -- Documentation content
    tags,                 -- Documentation tags/keywords
    
    -- Metadata columns
    node_id UNINDEXED,    -- Reference to code_nodes.id (if applicable)
    session_id UNINDEXED, -- Reference to session
    doc_type UNINDEXED,   -- Type: comment, docstring, readme, etc.
    language UNINDEXED,   -- Programming language context
    
    -- FTS5 configuration for natural language
    tokenize='porter unicode61 remove_diacritics 1',
    content='',
    contentless_delete=1
);

-- Create helper table for FTS search configuration and statistics
CREATE TABLE IF NOT EXISTS fts_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL UNIQUE, -- FTS table name
    enabled BOOLEAN DEFAULT TRUE,     -- Enable/disable indexing
    last_rebuild DATETIME,            -- Last full rebuild timestamp
    document_count INTEGER DEFAULT 0, -- Number of indexed documents
    index_size_kb INTEGER DEFAULT 0,  -- Approximate index size
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Initialize FTS configuration
INSERT OR IGNORE INTO fts_config (table_name, enabled) VALUES 
    ('code_content_fts', TRUE),
    ('file_content_fts', TRUE),
    ('documentation_fts', TRUE);

-- Create indexes for FTS metadata columns
CREATE INDEX IF NOT EXISTS idx_fts_config_table_name ON fts_config(table_name);
CREATE INDEX IF NOT EXISTS idx_fts_config_enabled ON fts_config(enabled);

-- Create triggers to maintain FTS indexes when code_nodes change
CREATE TRIGGER IF NOT EXISTS update_code_fts_on_insert
AFTER INSERT ON code_nodes
BEGIN
    -- Trigger will be implemented by application code
    -- This placeholder ensures the trigger exists for future use
    UPDATE fts_config 
    SET updated_at = CURRENT_TIMESTAMP 
    WHERE table_name = 'code_content_fts';
END;

CREATE TRIGGER IF NOT EXISTS update_code_fts_on_update
AFTER UPDATE ON code_nodes
BEGIN
    -- Trigger will be implemented by application code
    UPDATE fts_config 
    SET updated_at = CURRENT_TIMESTAMP 
    WHERE table_name = 'code_content_fts';
END;

CREATE TRIGGER IF NOT EXISTS update_code_fts_on_delete
AFTER DELETE ON code_nodes
BEGIN
    -- Remove from FTS index when code node is deleted
    DELETE FROM code_content_fts WHERE node_id = OLD.id;
    UPDATE fts_config 
    SET document_count = document_count - 1,
        updated_at = CURRENT_TIMESTAMP 
    WHERE table_name = 'code_content_fts';
END;

-- Create view for FTS search results with enriched metadata
CREATE VIEW IF NOT EXISTS code_search_results AS
SELECT 
    f.node_id,
    f.session_id,
    f.path,
    f.language,
    f.kind,
    f.symbol,
    f.content,
    f.comments,
    f.identifiers,
    -- Join with code_nodes for additional metadata
    c.start_line,
    c.end_line,
    c.metadata as node_metadata,
    c.created_at as node_created_at,
    c.updated_at as node_updated_at
FROM code_content_fts f
LEFT JOIN code_nodes c ON f.node_id = c.id;

-- Create view for file search results
CREATE VIEW IF NOT EXISTS file_search_results AS
SELECT 
    f.session_id,
    f.path,
    f.language,
    f.filename,
    f.content,
    f.file_size,
    -- Count related code nodes
    (SELECT COUNT(*) FROM code_nodes c WHERE c.path = f.path AND c.session_id = f.session_id) as node_count
FROM file_content_fts f;

-- Create view for documentation search results  
CREATE VIEW IF NOT EXISTS documentation_search_results AS
SELECT 
    d.node_id,
    d.session_id,
    d.doc_type,
    d.language,
    d.title,
    d.content,
    d.tags,
    -- Join with code_nodes if applicable
    c.path,
    c.symbol,
    c.kind
FROM documentation_fts d
LEFT JOIN code_nodes c ON d.node_id = c.id;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop FTS5 virtual tables and related structures

-- Drop views first (depend on tables)
DROP VIEW IF EXISTS documentation_search_results;
DROP VIEW IF EXISTS file_search_results;
DROP VIEW IF EXISTS code_search_results;

-- Drop triggers
DROP TRIGGER IF EXISTS update_code_fts_on_delete;
DROP TRIGGER IF EXISTS update_code_fts_on_update;
DROP TRIGGER IF EXISTS update_code_fts_on_insert;

-- Drop indexes
DROP INDEX IF EXISTS idx_fts_config_enabled;
DROP INDEX IF EXISTS idx_fts_config_table_name;

-- Drop helper table
DROP TABLE IF EXISTS fts_config;

-- Drop FTS5 virtual tables
DROP TABLE IF EXISTS documentation_fts;
DROP TABLE IF EXISTS file_content_fts;
DROP TABLE IF EXISTS code_content_fts;

-- +goose StatementEnd