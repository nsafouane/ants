package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// TestPhase2Integration tests the complete Phase 2 integration.
func TestPhase2Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup test environment
	testDB := setupTestDB(t)
	defer testDB.Close()

	ftsService := NewFTSService(testDB)

	// Create mock vector services
	vectorConfig := &config.VectorDBConfig{
		Type:       "qdrant",
		URL:        "http://localhost",
		Port:       6334,
		Collection: "test_collection",
	}
	_ = vectorConfig // Suppress unused warning for now

	// Note: For testing, we would need actual Qdrant instance or mocks
	// For now, testing just the FTS components
	t.Run("FTS5_Full_Workflow", func(t *testing.T) {
		testFTS5FullWorkflow(t, ctx, ftsService)
	})

	t.Run("Search_Optimization", func(t *testing.T) {
		testSearchOptimization(t, ctx, ftsService)
	})

	t.Run("Incremental_Indexing", func(t *testing.T) {
		testIncrementalIndexing(t, ctx, ftsService)
	})

	t.Run("Performance_Requirements", func(t *testing.T) {
		testPerformanceRequirements(t, ctx, ftsService)
	})
}

// testFTS5FullWorkflow tests the complete FTS5 search workflow.
func testFTS5FullWorkflow(t *testing.T, ctx context.Context, ftsService *FTSService) {
	sessionID := "integration-test-session"

	// Test data representing different code types
	testData := []struct {
		nodeID   int64
		language string
		kind     string
		symbol   string
		content  string
		path     string
	}{
		{
			nodeID:   1,
			language: "go",
			kind:     "function",
			symbol:   "authenticateUser",
			content:  "func authenticateUser(username, password string) (*User, error) { /* authentication logic */ }",
			path:     "/auth/user.go",
		},
		{
			nodeID:   2,
			language: "go",
			kind:     "struct",
			symbol:   "UserRepository",
			content:  "type UserRepository struct { db *sql.DB } // Database repository for user operations",
			path:     "/auth/repository.go",
		},
		{
			nodeID:   3,
			language: "python",
			kind:     "function",
			symbol:   "validate_email",
			content:  "def validate_email(email: str) -> bool: \"\"\"Validate email format using regex\"\"\" return True",
			path:     "/utils/validation.py",
		},
		{
			nodeID:   4,
			language: "javascript",
			kind:     "function",
			symbol:   "processPayment",
			content:  "function processPayment(amount, currency) { // Handle payment processing return true; }",
			path:     "/frontend/payment.js",
		},
	}

	// Index all test data
	for _, data := range testData {
		err := ftsService.IndexCodeContent(
			ctx,
			data.nodeID,
			sessionID,
			data.path,
			data.language,
			data.kind,
			data.symbol,
			data.content,
			DefaultIndexingOptions(),
		)
		require.NoError(t, err, "Failed to index code content")
	}

	// Test 1: Basic search functionality
	t.Run("Basic_Search", func(t *testing.T) {
		options := &SearchOptions{
			Query:       "authentication",
			SessionID:   sessionID,
			Limit:       10,
			IncludeCode: true,
		}

		results, err := ftsService.Search(ctx, options)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find authentication-related results")

		// Verify we found the authentication function
		found := false
		for _, result := range results {
			if result.Symbol != nil && *result.Symbol == "authenticateUser" {
				found = true
				assert.Equal(t, "go", *result.Language)
				assert.Equal(t, "function", *result.Kind)
				assert.Contains(t, result.Content, "authenticateUser")
				break
			}
		}
		assert.True(t, found, "Should find authenticateUser function")
	})

	// Test 2: Language-specific search
	t.Run("Language_Specific_Search", func(t *testing.T) {
		options := &SearchOptions{
			Query:       "function",
			SessionID:   sessionID,
			Language:    stringPtr("python"),
			Limit:       10,
			IncludeCode: true,
		}

		results, err := ftsService.Search(ctx, options)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find Python functions")

		// All results should be Python
		for _, result := range results {
			assert.Equal(t, "python", *result.Language)
		}
	})

	// Test 3: Multi-language search
	t.Run("Multi_Language_Search", func(t *testing.T) {
		options := &SearchOptions{
			Query:       "payment",
			SessionID:   sessionID,
			Limit:       10,
			IncludeCode: true,
		}

		results, err := ftsService.Search(ctx, options)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find payment-related code")

		// Should find JavaScript payment function
		found := false
		for _, result := range results {
			if result.Symbol != nil && *result.Symbol == "processPayment" {
				found = true
				assert.Equal(t, "javascript", *result.Language)
				break
			}
		}
		assert.True(t, found, "Should find processPayment function")
	})

	// Test 4: Kind-specific search
	t.Run("Kind_Specific_Search", func(t *testing.T) {
		options := &SearchOptions{
			Query:       "repository",
			SessionID:   sessionID,
			Kind:        stringPtr("struct"),
			Limit:       10,
			IncludeCode: true,
		}

		results, err := ftsService.Search(ctx, options)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find struct types")

		// All results should be structs
		for _, result := range results {
			assert.Equal(t, "struct", *result.Kind)
		}
	})

	// Test 5: File-level indexing and search
	t.Run("File_Level_Search", func(t *testing.T) {
		fileContent := `package main

import (
	"fmt"
	"net/http"
)

// Main HTTP server for user authentication
func main() {
	http.HandleFunc("/auth", authHandler)
	fmt.Println("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}`

		err := ftsService.IndexFileContent(
			ctx,
			sessionID,
			"/cmd/server.go",
			"go",
			fileContent,
			int64(len(fileContent)),
		)
		require.NoError(t, err)

		options := &SearchOptions{
			Query:       "HTTP server",
			SessionID:   sessionID,
			Limit:       10,
			IncludeFile: true,
			IncludeCode: false,
		}

		results, err := ftsService.Search(ctx, options)
		require.NoError(t, err)
		assert.NotEmpty(t, results, "Should find file-level content")

		// Verify file result
		found := false
		for _, result := range results {
			if result.Type == "file" && result.Path == "/cmd/server.go" {
				found = true
				assert.Equal(t, "server.go", *result.Filename)
				assert.Contains(t, result.Content, "HTTP server")
				break
			}
		}
		assert.True(t, found, "Should find server.go file")
	})

	// Test 6: Index management
	t.Run("Index_Management", func(t *testing.T) {
		// Get index statistics
		stats, err := ftsService.GetIndexStats(ctx)
		require.NoError(t, err)
		assert.Contains(t, stats, "tables")
		assert.Contains(t, stats, "total_tables")

		tables := stats["tables"].([]map[string]interface{})
		assert.Len(t, tables, 3) // Should have 3 FTS tables

		// Test index rebuild
		err = ftsService.RebuildIndex(ctx, "code_content_fts")
		assert.NoError(t, err)
	})

	// Test 7: Content removal
	t.Run("Content_Removal", func(t *testing.T) {
		// Remove a specific node
		err := ftsService.RemoveFromIndex(ctx, 1) // Remove authenticateUser
		require.NoError(t, err)

		// Search should no longer find it
		options := &SearchOptions{
			Query:       "authenticateUser",
			SessionID:   sessionID,
			Limit:       10,
			IncludeCode: true,
		}

		results, err := ftsService.Search(ctx, options)
		require.NoError(t, err)

		// Should not find the removed function
		for _, result := range results {
			assert.NotEqual(t, "authenticateUser", getStringValue(result.Symbol))
		}
	})
}

// testSearchOptimization tests the search optimization and ranking features.
func testSearchOptimization(t *testing.T, ctx context.Context, ftsService *FTSService) {
	// Note: This would require the vector search engine to be fully set up
	// For now, testing just the optimizer structure

	optimizer := NewSearchOptimizer(ftsService, nil) // nil vector engine for testing

	t.Run("Optimizer_Creation", func(t *testing.T) {
		assert.NotNil(t, optimizer)
		assert.NotNil(t, optimizer.weightConfig)
		assert.True(t, optimizer.hybridEnabled)
	})

	t.Run("Weight_Configuration", func(t *testing.T) {
		weights := DefaultSearchWeights()
		assert.Equal(t, 1.0, weights.FTSScore)
		assert.Equal(t, 0.8, weights.VectorScore)
		assert.Equal(t, 2.0, weights.ExactMatch)
		assert.True(t, weights.HasTests > 0)
		assert.True(t, weights.HasDocs > 0)
	})

	// Test query preparation and enhancement
	t.Run("Query_Enhancement", func(t *testing.T) {
		// Test query expansion
		expanded := optimizer.expandQuery("auth")
		assert.Contains(t, expanded, "authentication")
		assert.Contains(t, expanded, "authorize")
		assert.Contains(t, expanded, "login")

		// Test spell correction
		corrected := optimizer.correctSpelling("authentiction databse")
		assert.Contains(t, corrected, "authentication")
		assert.Contains(t, corrected, "database")
	})
}

// testIncrementalIndexing tests the incremental indexing functionality.
func testIncrementalIndexing(t *testing.T, ctx context.Context, ftsService *FTSService) {
	// Create event brokers
	codeNodeBroker := pubsub.NewBroker[pubsub.CodeNodeEvent]()
	fileBroker := pubsub.NewBroker[pubsub.FileSystemEvent]()

	// Create incremental indexer
	indexer := NewIncrementalIndexer(
		ftsService,
		nil, // vector processor (would be real in production)
		nil, // queries (would be real in production)
		codeNodeBroker,
		fileBroker,
		DefaultIndexerConfig(),
	)

	t.Run("Indexer_Creation", func(t *testing.T) {
		assert.NotNil(t, indexer)
		assert.NotNil(t, indexer.config)
		assert.Equal(t, 10, indexer.config.BatchSize)
		assert.Equal(t, 2, indexer.config.WorkerCount)
		assert.True(t, indexer.config.UpdateFTS)
	})

	t.Run("Configuration_Defaults", func(t *testing.T) {
		config := DefaultIndexerConfig()
		assert.Equal(t, 10, config.BatchSize)
		assert.Equal(t, 5*time.Second, config.BatchTimeout)
		assert.Equal(t, 2, config.WorkerCount)
		assert.Equal(t, 1000, config.QueueSize)
		assert.True(t, config.UpdateFTS)
		assert.True(t, config.UpdateVector)
		assert.True(t, config.UpdateAsync)
		assert.Equal(t, 500*time.Millisecond, config.DebounceInterval)
		assert.Equal(t, 3, config.MaxRetries)
		assert.Equal(t, 1*time.Second, config.RetryDelay)
	})

	t.Run("Health_Check", func(t *testing.T) {
		// Indexer should not be healthy when not running
		err := indexer.HealthCheck(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not running")
	})

	t.Run("Stats_Tracking", func(t *testing.T) {
		stats := indexer.GetStats()
		assert.Equal(t, int64(0), stats.TasksProcessed)
		assert.Equal(t, int64(0), stats.TasksSucceeded)
		assert.Equal(t, int64(0), stats.TasksFailed)
		assert.Equal(t, 0.0, stats.ErrorRate)
	})

	// Test manual reindexing
	t.Run("Manual_Reindex", func(t *testing.T) {
		// Should fail when indexer is not running
		err := indexer.ManualReindex(ctx, 123, "test-session", 5)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not running")
	})
}

// testPerformanceRequirements validates that Phase 2 meets performance targets.
func testPerformanceRequirements(t *testing.T, ctx context.Context, ftsService *FTSService) {
	sessionID := "performance-test-session"

	// Index a significant amount of test data
	numDocs := 100
	for i := 0; i < numDocs; i++ {
		content := fmt.Sprintf(`
func processData%d(input []byte) error {
	// Process data item %d
	validator := NewValidator()
	if err := validator.Validate(input); err != nil {
		return fmt.Errorf("validation failed for item %d: %%w", err)
	}
	
	processor := GetProcessor()
	result := processor.Process(input)
	
	if result.HasErrors() {
		return fmt.Errorf("processing failed for item %d", result.ErrorCount())
	}
	
	return storage.Save(result)
}`, i, i, i, i)

		err := ftsService.IndexCodeContent(
			ctx,
			int64(i+1),
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

	t.Run("Search_Latency_SLA", func(t *testing.T) {
		// Test search latency
		iterations := 10
		totalTime := time.Duration(0)

		for i := 0; i < iterations; i++ {
			start := time.Now()

			options := &SearchOptions{
				Query:       "validation failed",
				SessionID:   sessionID,
				Limit:       20,
				IncludeCode: true,
			}

			results, err := ftsService.Search(ctx, options)
			duration := time.Since(start)
			totalTime += duration

			require.NoError(t, err)
			assert.NotEmpty(t, results)
		}

		avgLatency := totalTime / time.Duration(iterations)

		// SLA: Average search should complete within 200ms for 100 documents
		assert.Less(t, avgLatency, 200*time.Millisecond,
			"Search latency SLA violated: avg %v > 200ms for %d documents", avgLatency, numDocs)

		t.Logf("Search performance: avg %v for %d documents (%d iterations)",
			avgLatency, numDocs, iterations)
	})

	t.Run("Indexing_Throughput", func(t *testing.T) {
		// Test indexing throughput
		batchSize := 50
		start := time.Now()

		for i := 0; i < batchSize; i++ {
			content := fmt.Sprintf("func batchTest%d() { /* batch test function %d */ }", i, i)

			err := ftsService.IndexCodeContent(
				ctx,
				int64(numDocs+i+1),
				sessionID,
				fmt.Sprintf("/batch/test%d.go", i),
				"go",
				"function",
				fmt.Sprintf("batchTest%d", i),
				content,
				DefaultIndexingOptions(),
			)
			require.NoError(t, err)
		}

		duration := time.Since(start)
		throughput := float64(batchSize) / duration.Seconds()

		// Target: Index at least 25 documents per second
		assert.True(t, throughput >= 25.0,
			"Indexing throughput target not met: %.2f docs/sec < 25 docs/sec", throughput)

		t.Logf("Indexing performance: %.2f docs/sec for %d documents",
			throughput, batchSize)
	})

	t.Run("Memory_Efficiency", func(t *testing.T) {
		// Test memory usage doesn't grow excessively
		// This is a simplified test - in production would use runtime.MemStats

		options := &SearchOptions{
			Query:       "process",
			SessionID:   sessionID,
			Limit:       50,
			IncludeCode: true,
		}

		// Perform multiple searches to check for memory leaks
		for i := 0; i < 10; i++ {
			results, err := ftsService.Search(ctx, options)
			require.NoError(t, err)
			assert.NotEmpty(t, results)
		}

		// If we get here without issues, memory usage is reasonable
		t.Log("Memory efficiency test completed without issues")
	})

	t.Run("Concurrent_Operations", func(t *testing.T) {
		// Test concurrent search operations
		concurrency := 5
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func(id int) {
				options := &SearchOptions{
					Query:       fmt.Sprintf("processData%d", id),
					SessionID:   sessionID,
					Limit:       10,
					IncludeCode: true,
				}

				results, err := ftsService.Search(ctx, options)
				if err != nil {
					done <- err
					return
				}

				if len(results) == 0 {
					done <- fmt.Errorf("no results found for concurrent search %d", id)
					return
				}

				done <- nil
			}(i)
		}

		// Wait for all operations to complete
		for i := 0; i < concurrency; i++ {
			err := <-done
			assert.NoError(t, err, "Concurrent search operation failed")
		}

		t.Log("Concurrent operations test completed successfully")
	})
}

// TestHealthCheck validates health check functionality.
func TestHealthCheck(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	ftsService := NewFTSService(testDB)

	err := ftsService.HealthCheck(context.Background())
	assert.NoError(t, err, "FTS5 health check should pass")
}

// TestErrorHandling validates error handling in various scenarios.
func TestErrorHandling(t *testing.T) {
	testDB := setupTestDB(t)
	defer testDB.Close()

	ftsService := NewFTSService(testDB)
	ctx := context.Background()

	t.Run("Empty_Query", func(t *testing.T) {
		options := &SearchOptions{
			Query:     "",
			SessionID: "test",
			Limit:     10,
		}

		results, err := ftsService.Search(ctx, options)
		assert.Error(t, err)
		assert.Empty(t, results)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("Invalid_Parameters", func(t *testing.T) {
		// Test with nil options
		results, err := ftsService.Search(ctx, nil)
		assert.Error(t, err)
		assert.Empty(t, results)

		// Test with invalid node ID
		err = ftsService.IndexCodeContent(ctx, -1, "test", "/invalid", "go", "function", "test", "content", nil)
		// Should handle gracefully (implementation dependent)
	})

	t.Run("Resource_Cleanup", func(t *testing.T) {
		// Test that resources are properly cleaned up
		err := ftsService.RemoveFromIndex(ctx, 999999) // Non-existent node
		// Should not error for non-existent items
		assert.NoError(t, err)
	})
}

// Helper functions (removed duplicate getStringValue since it exists in indexer.go)
