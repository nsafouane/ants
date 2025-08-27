package search

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/db"
)

// FTSService provides full-text search capabilities using SQLite FTS5.
type FTSService struct {
	queries *db.Queries
	db      *sql.DB
}

// NewFTSService creates a new FTS5 search service.
func NewFTSService(database *sql.DB) *FTSService {
	return &FTSService{
		queries: db.New(database),
		db:      database,
	}
}

// SearchOptions configures FTS5 search behavior.
type SearchOptions struct {
	// Query parameters
	Query     string `json:"query"`      // Search query
	SessionID string `json:"session_id"` // Limit to specific session

	// Filters
	Language *string `json:"language,omitempty"` // Programming language filter
	Kind     *string `json:"kind,omitempty"`     // Code element type filter
	DocType  *string `json:"doc_type,omitempty"` // Documentation type filter

	// Result configuration
	Limit       int  `json:"limit"`        // Maximum results
	IncludeCode bool `json:"include_code"` // Include code content in results
	IncludeFile bool `json:"include_file"` // Include file content in results
	IncludeDocs bool `json:"include_docs"` // Include documentation in results
}

// SearchResult represents a unified search result from FTS5.
type SearchResult struct {
	// Common fields
	Type      string  `json:"type"`       // "code", "file", or "docs"
	Score     float64 `json:"score"`      // FTS5 ranking score
	SessionID string  `json:"session_id"` // Session identifier
	Language  *string `json:"language"`   // Programming language

	// Code-specific fields
	NodeID      *int64  `json:"node_id,omitempty"`     // Code node ID
	Symbol      *string `json:"symbol,omitempty"`      // Function/class name
	Kind        *string `json:"kind,omitempty"`        // Code element type
	StartLine   *int32  `json:"start_line,omitempty"`  // Start line number
	EndLine     *int32  `json:"end_line,omitempty"`    // End line number
	Comments    *string `json:"comments,omitempty"`    // Extracted comments
	Identifiers *string `json:"identifiers,omitempty"` // Code identifiers

	// File-specific fields
	Filename  *string `json:"filename,omitempty"`   // File name only
	FileSize  *int64  `json:"file_size,omitempty"`  // File size in bytes
	NodeCount *int    `json:"node_count,omitempty"` // Related code nodes

	// Documentation-specific fields
	Title   *string `json:"title,omitempty"`    // Documentation title
	DocType *string `json:"doc_type,omitempty"` // Documentation type
	Tags    *string `json:"tags,omitempty"`     // Documentation tags

	// Common content fields
	Path    string `json:"path"`    // File path
	Content string `json:"content"` // Content snippet

	// Metadata
	CreatedAt *time.Time `json:"created_at,omitempty"` // Creation timestamp
	UpdatedAt *time.Time `json:"updated_at,omitempty"` // Last update
}

// IndexingOptions configures content indexing behavior.
type IndexingOptions struct {
	// Content processing
	ExtractComments    bool `json:"extract_comments"`    // Extract and index comments
	ExtractIdentifiers bool `json:"extract_identifiers"` // Extract variable/function names
	ProcessDocstrings  bool `json:"process_docstrings"`  // Process documentation strings

	// Performance options
	BatchSize      int  `json:"batch_size"`      // Batch size for bulk operations
	UpdateExisting bool `json:"update_existing"` // Update existing entries
}

// DefaultSearchOptions returns sensible defaults for search.
func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		Limit:       20,
		IncludeCode: true,
		IncludeFile: true,
		IncludeDocs: true,
	}
}

// DefaultIndexingOptions returns sensible defaults for indexing.
func DefaultIndexingOptions() *IndexingOptions {
	return &IndexingOptions{
		ExtractComments:    true,
		ExtractIdentifiers: true,
		ProcessDocstrings:  true,
		BatchSize:          100,
		UpdateExisting:     true,
	}
}

// Search performs unified full-text search across code, files, and documentation.
func (fts *FTSService) Search(ctx context.Context, options *SearchOptions) ([]*SearchResult, error) {
	if options == nil {
		options = DefaultSearchOptions()
	}

	if options.Query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}

	// Validate and prepare query
	query, err := fts.prepareSearchQuery(options.Query)
	if err != nil {
		return nil, fmt.Errorf("invalid search query: %w", err)
	}

	slog.Debug("Performing FTS5 search",
		"query", query,
		"session_id", options.SessionID,
		"language", options.Language,
		"limit", options.Limit)

	var results []*SearchResult

	// Search code content
	if options.IncludeCode {
		codeResults, err := fts.searchCodeContent(ctx, query, options)
		if err != nil {
			slog.Warn("Code search failed", "error", err)
		} else {
			results = append(results, codeResults...)
		}
	}

	// Search file content
	if options.IncludeFile {
		fileResults, err := fts.searchFileContent(ctx, query, options)
		if err != nil {
			slog.Warn("File search failed", "error", err)
		} else {
			results = append(results, fileResults...)
		}
	}

	// Search documentation
	if options.IncludeDocs {
		docResults, err := fts.searchDocumentation(ctx, query, options)
		if err != nil {
			slog.Warn("Documentation search failed", "error", err)
		} else {
			results = append(results, docResults...)
		}
	}

	// Sort by relevance score
	fts.sortResultsByRelevance(results)

	// Apply final limit
	if len(results) > options.Limit {
		results = results[:options.Limit]
	}

	slog.Info("FTS5 search completed",
		"query", options.Query,
		"results_found", len(results),
		"session_id", options.SessionID)

	return results, nil
}

// searchCodeContent searches in code content using FTS5.
func (fts *FTSService) searchCodeContent(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error) {
	// Note: This would need to be implemented with proper SQLC generated methods
	// For now, using raw SQL as an example
	sqlQuery := `
		SELECT 
			f.rowid,
			f.node_id,
			f.session_id,
			f.path,
			f.language,
			f.kind,
			f.symbol,
			f.content,
			f.comments,
			f.identifiers,
			c.start_line,
			c.end_line,
			c.created_at,
			c.updated_at,
			rank as fts_rank
		FROM code_content_fts(?) f
		LEFT JOIN code_nodes c ON f.node_id = c.id
		WHERE f.session_id = ?
		  AND (? IS NULL OR f.language = ?)
		  AND (? IS NULL OR f.kind = ?)
		ORDER BY rank
		LIMIT ?`

	rows, err := fts.db.QueryContext(ctx, sqlQuery,
		query,
		options.SessionID,
		options.Language, options.Language,
		options.Kind, options.Kind,
		options.Limit)
	if err != nil {
		return nil, fmt.Errorf("code content search failed: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var result SearchResult
		var nodeID sql.NullInt64
		var symbol, comments, identifiers sql.NullString
		var language, kind sql.NullString
		var startLine, endLine sql.NullInt32
		var createdAt, updatedAt sql.NullTime
		var ftsRank sql.NullFloat64

		err := rows.Scan(
			&result.Type, // Will be set to "code"
			&nodeID,
			&result.SessionID,
			&result.Path,
			&language,
			&kind,
			&symbol,
			&result.Content,
			&comments,
			&identifiers,
			&startLine,
			&endLine,
			&createdAt,
			&updatedAt,
			&ftsRank,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		// Set type and convert nullable fields
		result.Type = "code"
		if nodeID.Valid {
			result.NodeID = &nodeID.Int64
		}
		if symbol.Valid {
			result.Symbol = &symbol.String
		}
		if language.Valid {
			result.Language = &language.String
		}
		if kind.Valid {
			result.Kind = &kind.String
		}
		if comments.Valid {
			result.Comments = &comments.String
		}
		if identifiers.Valid {
			result.Identifiers = &identifiers.String
		}
		if startLine.Valid {
			result.StartLine = &startLine.Int32
		}
		if endLine.Valid {
			result.EndLine = &endLine.Int32
		}
		if createdAt.Valid {
			result.CreatedAt = &createdAt.Time
		}
		if updatedAt.Valid {
			result.UpdatedAt = &updatedAt.Time
		}
		if ftsRank.Valid {
			result.Score = ftsRank.Float64
		}

		results = append(results, &result)
	}

	return results, nil
}

// searchFileContent searches in file content using FTS5.
func (fts *FTSService) searchFileContent(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error) {
	sqlQuery := `
		SELECT 
			f.rowid,
			f.session_id,
			f.path,
			f.language,
			f.filename,
			f.content,
			f.file_size,
			(SELECT COUNT(*) FROM code_nodes c WHERE c.path = f.path AND c.session_id = f.session_id) as node_count,
			rank as fts_rank
		FROM file_content_fts(?) f
		WHERE f.session_id = ?
		  AND (? IS NULL OR f.language = ?)
		ORDER BY rank
		LIMIT ?`

	rows, err := fts.db.QueryContext(ctx, sqlQuery,
		query,
		options.SessionID,
		options.Language, options.Language,
		options.Limit)
	if err != nil {
		return nil, fmt.Errorf("file content search failed: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var result SearchResult
		var language sql.NullString
		var filename sql.NullString
		var fileSize sql.NullInt64
		var nodeCount sql.NullInt32
		var ftsRank sql.NullFloat64

		err := rows.Scan(
			&result.Type, // Will be set to "file"
			&result.SessionID,
			&result.Path,
			&language,
			&filename,
			&result.Content,
			&fileSize,
			&nodeCount,
			&ftsRank,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		// Set type and convert nullable fields
		result.Type = "file"
		if language.Valid {
			result.Language = &language.String
		}
		if filename.Valid {
			result.Filename = &filename.String
		}
		if fileSize.Valid {
			result.FileSize = &fileSize.Int64
		}
		if nodeCount.Valid {
			count := int(nodeCount.Int32)
			result.NodeCount = &count
		}
		if ftsRank.Valid {
			result.Score = ftsRank.Float64
		}

		results = append(results, &result)
	}

	return results, nil
}

// searchDocumentation searches in documentation using FTS5.
func (fts *FTSService) searchDocumentation(ctx context.Context, query string, options *SearchOptions) ([]*SearchResult, error) {
	sqlQuery := `
		SELECT 
			d.rowid,
			d.node_id,
			d.session_id,
			d.doc_type,
			d.language,
			d.title,
			d.content,
			d.tags,
			c.path,
			c.symbol,
			c.kind,
			rank as fts_rank
		FROM documentation_fts(?) d
		LEFT JOIN code_nodes c ON d.node_id = c.id
		WHERE d.session_id = ?
		  AND (? IS NULL OR d.language = ?)
		  AND (? IS NULL OR d.doc_type = ?)
		ORDER BY rank
		LIMIT ?`

	rows, err := fts.db.QueryContext(ctx, sqlQuery,
		query,
		options.SessionID,
		options.Language, options.Language,
		options.DocType, options.DocType,
		options.Limit)
	if err != nil {
		return nil, fmt.Errorf("documentation search failed: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		var result SearchResult
		var nodeID sql.NullInt64
		var docType, language, title, tags sql.NullString
		var path, symbol, kind sql.NullString
		var ftsRank sql.NullFloat64

		err := rows.Scan(
			&result.Type, // Will be set to "docs"
			&nodeID,
			&result.SessionID,
			&docType,
			&language,
			&title,
			&result.Content,
			&tags,
			&path,
			&symbol,
			&kind,
			&ftsRank,
		)
		if err != nil {
			continue // Skip invalid rows
		}

		// Set type and convert nullable fields
		result.Type = "docs"
		if nodeID.Valid {
			result.NodeID = &nodeID.Int64
		}
		if docType.Valid {
			result.DocType = &docType.String
		}
		if language.Valid {
			result.Language = &language.String
		}
		if title.Valid {
			result.Title = &title.String
		}
		if tags.Valid {
			result.Tags = &tags.String
		}
		if path.Valid {
			result.Path = path.String
		}
		if symbol.Valid {
			result.Symbol = &symbol.String
		}
		if kind.Valid {
			result.Kind = &kind.String
		}
		if ftsRank.Valid {
			result.Score = ftsRank.Float64
		}

		results = append(results, &result)
	}

	return results, nil
}

// IndexCodeContent indexes code content for full-text search.
func (fts *FTSService) IndexCodeContent(ctx context.Context, nodeID int64, sessionID, path, language, kind, symbol, content string, options *IndexingOptions) error {
	if options == nil {
		options = DefaultIndexingOptions()
	}

	// Extract search-relevant content
	comments := ""
	identifiers := ""

	if options.ExtractComments {
		comments = fts.extractComments(content, language)
	}

	if options.ExtractIdentifiers {
		identifiers = fts.extractIdentifiers(content, language)
	}

	// Index the content
	// Note: Using placeholder query - would use SQLC generated method in real implementation
	sqlQuery := `
		INSERT INTO code_content_fts(
			content, symbol, comments, identifiers,
			node_id, session_id, path, language, kind
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET
			content = excluded.content,
			symbol = excluded.symbol,
			comments = excluded.comments,
			identifiers = excluded.identifiers,
			path = excluded.path,
			language = excluded.language,
			kind = excluded.kind`

	_, err := fts.db.ExecContext(ctx, sqlQuery,
		content, symbol, comments, identifiers,
		nodeID, sessionID, path, language, kind)
	if err != nil {
		return fmt.Errorf("failed to index code content: %w", err)
	}

	// Update FTS statistics
	err = fts.updateFTSStats(ctx, "code_content_fts")
	if err != nil {
		slog.Warn("Failed to update FTS statistics", "error", err)
	}

	return nil
}

// IndexFileContent indexes file content for full-text search.
func (fts *FTSService) IndexFileContent(ctx context.Context, sessionID, path, language, content string, fileSize int64) error {
	filename := filepath.Base(path)

	sqlQuery := `
		INSERT INTO file_content_fts(
			content, filename, session_id, path, language, file_size
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path, session_id) DO UPDATE SET
			content = excluded.content,
			filename = excluded.filename,
			language = excluded.language,
			file_size = excluded.file_size`

	_, err := fts.db.ExecContext(ctx, sqlQuery,
		content, filename, sessionID, path, language, fileSize)
	if err != nil {
		return fmt.Errorf("failed to index file content: %w", err)
	}

	err = fts.updateFTSStats(ctx, "file_content_fts")
	if err != nil {
		slog.Warn("Failed to update FTS statistics", "error", err)
	}

	return nil
}

// RemoveFromIndex removes content from FTS5 indexes.
func (fts *FTSService) RemoveFromIndex(ctx context.Context, nodeID int64) error {
	// Remove from code content index
	_, err := fts.db.ExecContext(ctx, "DELETE FROM code_content_fts WHERE node_id = ?", nodeID)
	if err != nil {
		return fmt.Errorf("failed to remove from code content index: %w", err)
	}

	// Remove from documentation index
	_, err = fts.db.ExecContext(ctx, "DELETE FROM documentation_fts WHERE node_id = ?", nodeID)
	if err != nil {
		return fmt.Errorf("failed to remove from documentation index: %w", err)
	}

	return nil
}

// RebuildIndex rebuilds a specific FTS5 index.
func (fts *FTSService) RebuildIndex(ctx context.Context, tableName string) error {
	slog.Info("Rebuilding FTS5 index", "table", tableName)

	// Rebuild the index
	_, err := fts.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild')", tableName, tableName))
	if err != nil {
		return fmt.Errorf("failed to rebuild FTS5 index %s: %w", tableName, err)
	}

	// Update configuration
	_, err = fts.db.ExecContext(ctx,
		"UPDATE fts_config SET last_rebuild = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE table_name = ?",
		tableName)
	if err != nil {
		slog.Warn("Failed to update rebuild timestamp", "table", tableName, "error", err)
	}

	slog.Info("FTS5 index rebuilt successfully", "table", tableName)
	return nil
}

// GetIndexStats returns statistics about FTS5 indexes.
func (fts *FTSService) GetIndexStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get basic statistics
	rows, err := fts.db.QueryContext(ctx, `
		SELECT 
			table_name,
			enabled,
			document_count,
			index_size_kb,
			last_rebuild,
			updated_at
		FROM fts_config
		ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("failed to get FTS statistics: %w", err)
	}
	defer rows.Close()

	var tables []map[string]interface{}
	for rows.Next() {
		var tableName string
		var enabled bool
		var docCount, indexSize sql.NullInt64
		var lastRebuild, updatedAt sql.NullTime

		err := rows.Scan(&tableName, &enabled, &docCount, &indexSize, &lastRebuild, &updatedAt)
		if err != nil {
			continue
		}

		table := map[string]interface{}{
			"table_name": tableName,
			"enabled":    enabled,
		}

		if docCount.Valid {
			table["document_count"] = docCount.Int64
		}
		if indexSize.Valid {
			table["index_size_kb"] = indexSize.Int64
		}
		if lastRebuild.Valid {
			table["last_rebuild"] = lastRebuild.Time
		}
		if updatedAt.Valid {
			table["updated_at"] = updatedAt.Time
		}

		tables = append(tables, table)
	}

	stats["tables"] = tables
	stats["total_tables"] = len(tables)

	return stats, nil
}

// Helper methods

func (fts *FTSService) prepareSearchQuery(query string) (string, error) {
	// Clean and validate the query
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("empty query")
	}

	// Escape special FTS5 characters if needed
	// This is a simplified version - a full implementation would handle all FTS5 syntax
	query = strings.ReplaceAll(query, `"`, `""`)

	// Add fuzzy search for single terms
	terms := strings.Fields(query)
	if len(terms) == 1 && len(terms[0]) > 3 {
		return fmt.Sprintf(`"%s" OR %s*`, terms[0], terms[0]), nil
	}

	return query, nil
}

func (fts *FTSService) extractComments(content, language string) string {
	var comments []string

	switch language {
	case "go", "javascript", "typescript", "java", "c", "cpp":
		// Extract // and /* */ comments
		lineComments := regexp.MustCompile(`//.*`)
		blockComments := regexp.MustCompile(`/\*[\s\S]*?\*/`)

		comments = append(comments, lineComments.FindAllString(content, -1)...)
		comments = append(comments, blockComments.FindAllString(content, -1)...)

	case "python":
		// Extract # comments and docstrings
		lineComments := regexp.MustCompile(`#.*`)
		docstrings := regexp.MustCompile(`"""[\s\S]*?"""|'''[\s\S]*?'''`)

		comments = append(comments, lineComments.FindAllString(content, -1)...)
		comments = append(comments, docstrings.FindAllString(content, -1)...)

	case "sql":
		// Extract -- comments
		lineComments := regexp.MustCompile(`--.*`)
		comments = append(comments, lineComments.FindAllString(content, -1)...)
	}

	return strings.Join(comments, " ")
}

func (fts *FTSService) extractIdentifiers(content, language string) string {
	// This is a simplified identifier extraction
	// A more sophisticated implementation would use language-specific parsing
	identifierRegex := regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\b`)
	identifiers := identifierRegex.FindAllString(content, -1)

	// Remove duplicates and common keywords
	uniqueIdentifiers := make(map[string]bool)
	keywords := map[string]bool{
		"if": true, "else": true, "for": true, "while": true, "return": true,
		"func": true, "function": true, "var": true, "let": true, "const": true,
		"class": true, "struct": true, "interface": true, "type": true,
	}

	var filtered []string
	for _, id := range identifiers {
		if !keywords[strings.ToLower(id)] && !uniqueIdentifiers[id] && len(id) > 2 {
			uniqueIdentifiers[id] = true
			filtered = append(filtered, id)
		}
	}

	return strings.Join(filtered, " ")
}

func (fts *FTSService) updateFTSStats(ctx context.Context, tableName string) error {
	// Count documents in the table
	var count int
	err := fts.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
	if err != nil {
		return err
	}

	// Update statistics
	_, err = fts.db.ExecContext(ctx, `
		UPDATE fts_config 
		SET document_count = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE table_name = ?`, count, tableName)

	return err
}

func (fts *FTSService) sortResultsByRelevance(results []*SearchResult) {
	// Simple sorting by FTS5 rank score
	// More sophisticated implementations could combine multiple factors
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// HealthCheck verifies FTS5 functionality.
func (fts *FTSService) HealthCheck(ctx context.Context) error {
	// Check if FTS5 is available
	var ftsVersion string
	err := fts.db.QueryRowContext(ctx, "SELECT fts5_version()").Scan(&ftsVersion)
	if err != nil {
		return fmt.Errorf("FTS5 not available: %w", err)
	}

	slog.Debug("FTS5 health check passed", "version", ftsVersion)
	return nil
}
