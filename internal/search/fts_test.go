package search

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFTSService_BasicSearch(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)

	// Test data
	sessionID := "test-session"
	testCode := `
func authenticateUser(username, password string) (*User, error) {
    // Validate user credentials
    if username == "" || password == "" {
        return nil, errors.New("username and password required")
    }
    
    user := &User{Username: username}
    return user, nil
}`

	// Index test content
	err := service.IndexCodeContent(
		context.Background(),
		1,
		sessionID,
		"/auth/user.go",
		"go",
		"function",
		"authenticateUser",
		testCode,
		DefaultIndexingOptions(),
	)
	require.NoError(t, err)

	// Search for the content
	options := &SearchOptions{
		Query:       "authenticate user",
		SessionID:   sessionID,
		Limit:       10,
		IncludeCode: true,
	}

	results, err := service.Search(context.Background(), options)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Verify result content
	found := false
	for _, result := range results {
		if result.Type == "code" && result.Symbol != nil && *result.Symbol == "authenticateUser" {
			found = true
			assert.Contains(t, result.Content, "authenticateUser")
			assert.Equal(t, "go", *result.Language)
			assert.Equal(t, "function", *result.Kind)
			break
		}
	}
	assert.True(t, found, "Should find the indexed function")
}

func TestFTSService_MultiLanguageSearch(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)
	ctx := context.Background()
	sessionID := "multi-lang-session"

	// Index content in different languages
	testCases := []struct {
		nodeID   int64
		language string
		kind     string
		symbol   string
		content  string
	}{
		{
			nodeID:   1,
			language: "go",
			kind:     "function",
			symbol:   "ProcessPayment",
			content:  "func ProcessPayment(amount float64) error { /* payment logic */ }",
		},
		{
			nodeID:   2,
			language: "python",
			kind:     "function",
			symbol:   "process_payment",
			content:  "def process_payment(amount): # Handle payment processing\n    pass",
		},
		{
			nodeID:   3,
			language: "javascript",
			kind:     "function",
			symbol:   "processPayment",
			content:  "function processPayment(amount) { // JavaScript payment handler }",
		},
	}

	for _, tc := range testCases {
		err := service.IndexCodeContent(
			ctx, tc.nodeID, sessionID,
			fmt.Sprintf("/src/%s/payment.%s", tc.language, getFileExtension(tc.language)),
			tc.language, tc.kind, tc.symbol, tc.content,
			DefaultIndexingOptions(),
		)
		require.NoError(t, err)
	}

	// Search across all languages
	options := &SearchOptions{
		Query:       "payment processing",
		SessionID:   sessionID,
		Limit:       10,
		IncludeCode: true,
	}

	results, err := service.Search(ctx, options)
	require.NoError(t, err)
	assert.Len(t, results, 3, "Should find results in all languages")

	// Verify language distribution
	languages := make(map[string]int)
	for _, result := range results {
		if result.Language != nil {
			languages[*result.Language]++
		}
	}
	assert.Equal(t, 1, languages["go"])
	assert.Equal(t, 1, languages["python"])
	assert.Equal(t, 1, languages["javascript"])

	// Test language-specific search
	options.Language = stringPtr("python")
	results, err = service.Search(ctx, options)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "python", *results[0].Language)
}

func TestFTSService_CommentExtraction(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)

	testCases := []struct {
		name     string
		language string
		content  string
		expected string
	}{
		{
			name:     "Go single line comments",
			language: "go",
			content: `
				// This is a comment
				func test() {
					// Another comment
					return nil
				}`,
			expected: "comment",
		},
		{
			name:     "Go block comments",
			language: "go",
			content: `
				/*
				 * Multi-line comment
				 * with multiple lines
				 */
				func test() {}`,
			expected: "Multi-line comment",
		},
		{
			name:     "Python comments",
			language: "python",
			content: `
				# Python comment
				def test():
					"""Docstring comment"""
					pass`,
			expected: "Python comment",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			comments := service.extractComments(tc.content, tc.language)
			assert.Contains(t, comments, tc.expected)
		})
	}
}

func TestFTSService_IdentifierExtraction(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)

	content := `
func ProcessUserData(userData *UserModel) error {
	validator := NewDataValidator()
	if err := validator.ValidateUser(userData); err != nil {
		return err
	}
	return dataProcessor.Process(userData)
}`

	identifiers := service.extractIdentifiers(content, "go")

	// Should extract function and variable names
	assert.Contains(t, identifiers, "ProcessUserData")
	assert.Contains(t, identifiers, "UserModel")
	assert.Contains(t, identifiers, "validator")
	assert.Contains(t, identifiers, "NewDataValidator")
	assert.Contains(t, identifiers, "ValidateUser")
	assert.Contains(t, identifiers, "userData")
	assert.Contains(t, identifiers, "dataProcessor")

	// Should not extract keywords
	assert.NotContains(t, identifiers, "func")
	assert.NotContains(t, identifiers, "if")
	assert.NotContains(t, identifiers, "return")
}

func TestFTSService_FileContentSearch(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)
	ctx := context.Background()
	sessionID := "file-search-session"

	// Index file content
	fileContent := `
package main

import (
	"fmt"
	"log"
)

// Main application entry point
func main() {
	fmt.Println("Hello, World!")
	log.Println("Application starting...")
}`

	err := service.IndexFileContent(
		ctx,
		sessionID,
		"/cmd/main.go",
		"go",
		fileContent,
		int64(len(fileContent)),
	)
	require.NoError(t, err)

	// Search file content
	options := &SearchOptions{
		Query:       "application entry point",
		SessionID:   sessionID,
		Limit:       10,
		IncludeFile: true,
		IncludeCode: false,
		IncludeDocs: false,
	}

	results, err := service.Search(ctx, options)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Verify file result
	found := false
	for _, result := range results {
		if result.Type == "file" && result.Path == "/cmd/main.go" {
			found = true
			assert.Equal(t, "main.go", *result.Filename)
			assert.Equal(t, "go", *result.Language)
			assert.NotNil(t, result.FileSize)
			break
		}
	}
	assert.True(t, found, "Should find the indexed file")
}

func TestFTSService_SearchQueryPreparation(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)

	testCases := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "Simple query",
			input:    "authentication",
			expected: `"authentication" OR authentication*`,
		},
		{
			name:     "Multi-word query",
			input:    "user authentication system",
			expected: "user authentication system",
		},
		{
			name:     "Query with quotes",
			input:    `user "admin" access`,
			expected: `user ""admin"" access`,
		},
		{
			name:     "Empty query",
			input:    "",
			hasError: true,
		},
		{
			name:     "Whitespace only",
			input:    "   ",
			hasError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.prepareSearchQuery(tc.input)

			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestFTSService_IndexManagement(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)
	ctx := context.Background()

	// Test index statistics
	stats, err := service.GetIndexStats(ctx)
	require.NoError(t, err)
	assert.Contains(t, stats, "tables")
	assert.Contains(t, stats, "total_tables")

	tables := stats["tables"].([]map[string]interface{})
	assert.Len(t, tables, 3) // code_content_fts, file_content_fts, documentation_fts

	// Test index rebuild
	err = service.RebuildIndex(ctx, "code_content_fts")
	assert.NoError(t, err)

	// Verify rebuild timestamp was updated
	stats, err = service.GetIndexStats(ctx)
	require.NoError(t, err)

	tables = stats["tables"].([]map[string]interface{})
	for _, table := range tables {
		if table["table_name"] == "code_content_fts" {
			assert.NotNil(t, table["last_rebuild"])
			break
		}
	}
}

func TestFTSService_RemoveFromIndex(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)
	ctx := context.Background()
	sessionID := "remove-test-session"

	// Index some content
	err := service.IndexCodeContent(
		ctx, 1, sessionID, "/test.go", "go", "function", "testFunc",
		"func testFunc() { /* test */ }", DefaultIndexingOptions(),
	)
	require.NoError(t, err)

	// Verify it's indexed
	options := &SearchOptions{
		Query:       "testFunc",
		SessionID:   sessionID,
		Limit:       10,
		IncludeCode: true,
	}

	results, err := service.Search(ctx, options)
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Remove from index
	err = service.RemoveFromIndex(ctx, 1)
	require.NoError(t, err)

	// Verify it's no longer found
	results, err = service.Search(ctx, options)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestFTSService_HealthCheck(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)

	err := service.HealthCheck(context.Background())
	assert.NoError(t, err, "FTS5 health check should pass")
}

func TestFTSService_ConcurrentOperations(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)
	ctx := context.Background()
	sessionID := "concurrent-test-session"

	// Perform concurrent indexing operations
	concurrency := 10
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			err := service.IndexCodeContent(
				ctx,
				int64(id),
				sessionID,
				fmt.Sprintf("/test%d.go", id),
				"go",
				"function",
				fmt.Sprintf("testFunc%d", id),
				fmt.Sprintf("func testFunc%d() { /* test %d */ }", id, id),
				DefaultIndexingOptions(),
			)
			done <- err
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < concurrency; i++ {
		err := <-done
		assert.NoError(t, err)
	}

	// Verify all content was indexed
	options := &SearchOptions{
		Query:       "testFunc",
		SessionID:   sessionID,
		Limit:       concurrency + 5,
		IncludeCode: true,
	}

	results, err := service.Search(ctx, options)
	require.NoError(t, err)
	assert.Len(t, results, concurrency)
}

func TestFTSService_SearchPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	testDB := setupTestDB(t)
	defer testDB.Close()

	service := NewFTSService(testDB)
	ctx := context.Background()
	sessionID := "perf-test-session"

	// Index a large number of documents
	numDocs := 1000
	for i := 0; i < numDocs; i++ {
		content := fmt.Sprintf(`
func processData%d(input []byte) error {
	// Process data %d
	validator := NewValidator()
	if err := validator.Validate(input); err != nil {
		return fmt.Errorf("validation failed for data %d: %%w", err)
	}
	return processHandler.Handle(input)
}`, i, i, i)

		err := service.IndexCodeContent(
			ctx,
			int64(i),
			sessionID,
			fmt.Sprintf("/src/data%d.go", i),
			"go",
			"function",
			fmt.Sprintf("processData%d", i),
			content,
			DefaultIndexingOptions(),
		)
		require.NoError(t, err)
	}

	// Measure search performance
	start := time.Now()

	options := &SearchOptions{
		Query:       "validation failed",
		SessionID:   sessionID,
		Limit:       50,
		IncludeCode: true,
	}

	results, err := service.Search(ctx, options)
	searchTime := time.Since(start)

	require.NoError(t, err)
	assert.NotEmpty(t, results)

	// Search should complete within reasonable time
	assert.Less(t, searchTime, 500*time.Millisecond,
		"Search should complete within 500ms for %d documents", numDocs)

	t.Logf("Search completed in %v for %d documents, found %d results",
		searchTime, numDocs, len(results))
}

// Helper functions for tests

func setupTestDB(t *testing.T) *sql.DB {
	// Create in-memory test database
	database, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// Enable FTS5
	_, err = database.Exec("PRAGMA table_info=fts5")
	require.NoError(t, err)

	// Create FTS5 tables (simplified for testing)
	_, err = database.Exec(`
		CREATE VIRTUAL TABLE code_content_fts USING fts5(
			content, symbol, comments, identifiers,
			node_id UNINDEXED, session_id UNINDEXED, path UNINDEXED,
			language UNINDEXED, kind UNINDEXED
		)`)
	require.NoError(t, err)

	_, err = database.Exec(`
		CREATE VIRTUAL TABLE file_content_fts USING fts5(
			content, filename,
			session_id UNINDEXED, path UNINDEXED, language UNINDEXED, file_size UNINDEXED
		)`)
	require.NoError(t, err)

	_, err = database.Exec(`
		CREATE VIRTUAL TABLE documentation_fts USING fts5(
			title, content, tags,
			node_id UNINDEXED, session_id UNINDEXED, doc_type UNINDEXED, language UNINDEXED
		)`)
	require.NoError(t, err)

	// Create FTS config table
	_, err = database.Exec(`
		CREATE TABLE fts_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			table_name TEXT NOT NULL UNIQUE,
			enabled BOOLEAN DEFAULT TRUE,
			last_rebuild DATETIME,
			document_count INTEGER DEFAULT 0,
			index_size_kb INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
	require.NoError(t, err)

	// Initialize config
	_, err = database.Exec(`
		INSERT INTO fts_config (table_name, enabled) VALUES 
		('code_content_fts', TRUE),
		('file_content_fts', TRUE),
		('documentation_fts', TRUE)`)
	require.NoError(t, err)

	// Create code_nodes table for joins
	_, err = database.Exec(`
		CREATE TABLE code_nodes (
			id INTEGER PRIMARY KEY,
			session_id TEXT NOT NULL,
			path TEXT NOT NULL,
			start_line INTEGER,
			end_line INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
	require.NoError(t, err)

	return database
}

func getFileExtension(language string) string {
	switch language {
	case "go":
		return "go"
	case "python":
		return "py"
	case "javascript":
		return "js"
	case "typescript":
		return "ts"
	default:
		return "txt"
	}
}

func stringPtr(s string) *string {
	return &s
}
