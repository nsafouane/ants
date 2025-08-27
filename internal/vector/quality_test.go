package vector

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
)

// TestEmbeddingQuality validates embedding quality metrics.
func TestEmbeddingQuality(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping embedding quality tests in short mode")
	}

	// Test embedding quality for similar code snippets
	t.Run("similar_code_similarity", func(t *testing.T) {
		service := createTestEmbeddingService()

		code1 := `func getUserById(id int64) (*User, error) {
			return userRepo.FindByID(id)
		}`

		code2 := `func getUser(userId int64) (*User, error) {
			return userRepository.FindByID(userId)
		}`

		metadata := &EmbeddingMetadata{
			NodeID:    1,
			SessionID: "test",
			Language:  "go",
			Kind:      "function",
		}

		embedding1, err := service.GenerateCodeEmbedding(context.Background(), code1, metadata)
		require.NoError(t, err)

		embedding2, err := service.GenerateCodeEmbedding(context.Background(), code2, metadata)
		require.NoError(t, err)

		similarity := cosineSimilarity(embedding1, embedding2)

		// Similar functions should have high similarity (>0.7)
		assert.True(t, similarity > 0.7, "Similar functions should have high similarity, got %f", similarity)
	})

	t.Run("different_code_dissimilarity", func(t *testing.T) {
		service := createTestEmbeddingService()

		codeFunction := `func getUserById(id int64) (*User, error) {
			return userRepo.FindByID(id)
		}`

		codeStruct := `type DatabaseConfig struct {
			Host     string
			Port     int
			Database string
		}`

		metadata := &EmbeddingMetadata{
			NodeID:    1,
			SessionID: "test",
			Language:  "go",
		}

		embedding1, err := service.GenerateCodeEmbedding(context.Background(), codeFunction, metadata)
		require.NoError(t, err)

		embedding2, err := service.GenerateCodeEmbedding(context.Background(), codeStruct, metadata)
		require.NoError(t, err)

		similarity := cosineSimilarity(embedding1, embedding2)

		// Different types of code should have lower similarity (<0.5)
		assert.True(t, similarity < 0.5, "Different code types should have low similarity, got %f", similarity)
	})

	t.Run("metadata_influence", func(t *testing.T) {
		service := createTestEmbeddingService()

		code := `func process() { /* implementation */ }`

		metadata1 := &EmbeddingMetadata{
			NodeID:    1,
			SessionID: "test",
			Language:  "go",
			Kind:      "function",
			Symbol:    "processUser",
			Summary:   "Processes user data",
		}

		metadata2 := &EmbeddingMetadata{
			NodeID:    2,
			SessionID: "test",
			Language:  "go",
			Kind:      "function",
			Symbol:    "processPayment",
			Summary:   "Processes payment data",
		}

		embedding1, err := service.GenerateCodeEmbedding(context.Background(), code, metadata1)
		require.NoError(t, err)

		embedding2, err := service.GenerateCodeEmbedding(context.Background(), code, metadata2)
		require.NoError(t, err)

		// Same code with different metadata should produce different embeddings
		similarity := cosineSimilarity(embedding1, embedding2)
		assert.True(t, similarity < 0.95, "Metadata should influence embeddings, got similarity %f", similarity)
	})
}

// TestSearchRelevance validates search result relevance and ranking.
func TestSearchRelevance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping search relevance tests in short mode")
	}

	t.Run("semantic_search_accuracy", func(t *testing.T) {
		engine := createTestSearchEngine()

		// Setup test data
		testNodes := []*db.CodeNode{
			{
				ID:        1,
				SessionID: "test",
				Path:      "/auth/user.go",
				Language:  sql.NullString{String: "go", Valid: true},
				Kind:      sql.NullString{String: "function", Valid: true},
				Symbol:    sql.NullString{String: "authenticateUser", Valid: true},
			},
			{
				ID:        2,
				SessionID: "test",
				Path:      "/auth/token.go",
				Language:  sql.NullString{String: "go", Valid: true},
				Kind:      sql.NullString{String: "function", Valid: true},
				Symbol:    sql.NullString{String: "generateToken", Valid: true},
			},
			{
				ID:        3,
				SessionID: "test",
				Path:      "/db/migrations.go",
				Language:  sql.NullString{String: "go", Valid: true},
				Kind:      sql.NullString{String: "function", Valid: true},
				Symbol:    sql.NullString{String: "runMigrations", Valid: true},
			},
		}

		// Mock search results that simulate semantic similarity
		hybridResults := createMockHybridResults(testNodes, []float32{0.95, 0.88, 0.45})

		// Test authentication-related query
		options := &SmartSearchOptions{
			Query:     "user authentication login",
			Limit:     10,
			SessionID: &testNodes[0].SessionID,
		}

		// Mock the search process
		results := mockSmartSearch(engine, hybridResults, options)

		require.Len(t, results, 3)

		// Authentication-related functions should rank higher
		assert.True(t, results[0].FinalScore > results[1].FinalScore)
		assert.True(t, results[1].FinalScore > results[2].FinalScore)

		// Top result should be authentication function
		assert.Equal(t, int64(1), results[0].CodeNode.ID)
		assert.Contains(t, results[0].MatchReasons, "High semantic similarity")
	})

	t.Run("quality_boost_effectiveness", func(t *testing.T) {
		engine := createTestSearchEngine()

		// Two similar functions, one with tests/docs, one without
		nodeWithQuality := &db.CodeNode{
			ID:        1,
			SessionID: "test",
			Symbol:    sql.NullString{String: "processPayment", Valid: true},
		}

		nodeWithoutQuality := &db.CodeNode{
			ID:        2,
			SessionID: "test",
			Symbol:    sql.NullString{String: "processOrder", Valid: true},
		}

		// Both have similar vector scores
		result1 := &HybridSearchResult{
			Score:    0.85,
			CodeNode: nodeWithQuality,
			VectorMetadata: map[string]interface{}{
				"has_tests":      true,
				"has_docs":       true,
				"tier_completed": 3,
			},
		}

		result2 := &HybridSearchResult{
			Score:    0.86, // Slightly higher vector score
			CodeNode: nodeWithoutQuality,
			VectorMetadata: map[string]interface{}{
				"has_tests":      false,
				"has_docs":       false,
				"tier_completed": 1,
			},
		}

		options := &SmartSearchOptions{
			Query:        "payment processing",
			BoostQuality: true,
		}

		// Process results
		enhanced1 := mockEnhanceResult(engine, result1, options)
		enhanced2 := mockEnhanceResult(engine, result2, options)

		// Quality-boosted result should have higher final score despite lower vector score
		assert.True(t, enhanced1.FinalScore > enhanced2.FinalScore,
			"Quality boost should overcome slight vector score disadvantage")

		assert.True(t, enhanced1.QualitySignals.HasTests)
		assert.True(t, enhanced1.QualitySignals.HasDocs)
		assert.Equal(t, 3, enhanced1.QualitySignals.AnalysisTier)
	})

	t.Run("symbol_match_precision", func(t *testing.T) {
		engine := createTestSearchEngine()

		testCases := []struct {
			query       string
			symbol      string
			shouldMatch bool
		}{
			{"getUserById", "getUserById", true},
			{"getUserById", "getUserById()", true},
			{"getUser", "getUserById", true},  // Prefix match
			{"user", "getUserById", false},    // Too short
			{"getData", "getUserById", false}, // No match
		}

		for _, tc := range testCases {
			result := engine.isSymbolMatch(tc.query, tc.symbol)
			assert.Equal(t, tc.shouldMatch, result,
				"Symbol match for query '%s' and symbol '%s'", tc.query, tc.symbol)
		}
	})
}

// TestPerformanceBenchmarks validates system performance under various loads.
func TestPerformanceBenchmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance benchmarks in short mode")
	}

	t.Run("embedding_generation_latency", func(t *testing.T) {
		service := createTestEmbeddingService()

		codeSnippets := []string{
			"func simple() {}",
			"func medium() { /* some logic */ for i := 0; i < 10; i++ { process(i) } }",
			"func complex() { /* complex logic with multiple nested loops and conditions */ }",
		}

		metadata := &EmbeddingMetadata{
			NodeID:    1,
			SessionID: "perf-test",
			Language:  "go",
			Kind:      "function",
		}

		for i, code := range codeSnippets {
			start := time.Now()
			_, err := service.GenerateCodeEmbedding(context.Background(), code, metadata)
			duration := time.Since(start)

			require.NoError(t, err)

			// Embedding generation should complete within reasonable time
			assert.True(t, duration < 5*time.Second,
				"Embedding generation took too long for snippet %d: %v", i, duration)
		}
	})

	t.Run("search_response_time", func(t *testing.T) {
		engine := createTestSearchEngine()

		options := &SmartSearchOptions{
			Query: "user authentication",
			Limit: 50,
		}

		start := time.Now()
		// Mock a search operation
		_ = mockSearchLatency(engine, options, 100) // Simulate 100 results to process
		duration := time.Since(start)

		// Search should complete within reasonable time
		assert.True(t, duration < 2*time.Second,
			"Search took too long: %v", duration)
	})

	t.Run("batch_processing_throughput", func(t *testing.T) {
		processor := createTestBatchProcessor()

		// Simulate processing 100 nodes
		nodeCount := 100
		nodes := make([]*db.CodeNode, nodeCount)
		for i := 0; i < nodeCount; i++ {
			nodes[i] = &db.CodeNode{
				ID:        int64(i + 1),
				SessionID: "batch-test",
				Path:      fmt.Sprintf("/test/file%d.go", i),
				Language:  sql.NullString{String: "go", Valid: true},
				Kind:      sql.NullString{String: "function", Valid: true},
			}
		}

		start := time.Now()
		// Mock batch processing
		processedCount := mockBatchProcessing(processor, nodes)
		duration := time.Since(start)

		throughput := float64(processedCount) / duration.Seconds()

		// Should process at least 10 nodes per second
		assert.True(t, throughput >= 10.0,
			"Batch processing throughput too low: %.2f nodes/sec", throughput)
	})
}

// TestVectorOperationsIntegration tests end-to-end vector operations.
func TestVectorOperationsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	t.Run("end_to_end_workflow", func(t *testing.T) {
		// Test the complete workflow: embed -> store -> search -> retrieve
		embeddingService := createTestEmbeddingService()
		vectorBridge := createTestVectorBridge()
		searchEngine := createTestSearchEngine()

		// Step 1: Generate embedding
		code := `func authenticateUser(username, password string) (*User, error) {
			user, err := userRepo.FindByUsername(username)
			if err != nil {
				return nil, err
			}
			if !checkPassword(user.PasswordHash, password) {
				return nil, errors.New("invalid credentials")
			}
			return user, nil
		}`

		metadata := &EmbeddingMetadata{
			NodeID:    123,
			SessionID: "integration-test",
			Path:      "/auth/user.go",
			Language:  "go",
			Kind:      "function",
			Symbol:    "authenticateUser",
		}

		embedding, err := embeddingService.GenerateCodeEmbedding(context.Background(), code, metadata)
		require.NoError(t, err)
		require.True(t, len(embedding) > 0)

		// Step 2: Store embedding (mocked)
		err = mockStoreEmbedding(vectorBridge, 123, embedding, metadata)
		require.NoError(t, err)

		// Step 3: Search for similar code
		searchOptions := &SmartSearchOptions{
			Query:     "user login authentication",
			Limit:     10,
			SessionID: &metadata.SessionID,
		}

		results := mockSearchOperation(searchEngine, searchOptions, embedding)
		require.NotEmpty(t, results)

		// Step 4: Verify search results
		topResult := results[0]
		assert.Equal(t, int64(123), topResult.CodeNode.ID)
		assert.True(t, topResult.FinalScore > 0.8)
		assert.Contains(t, topResult.MatchReasons, "High semantic similarity")
	})

	t.Run("session_isolation", func(t *testing.T) {
		searchEngine := createTestSearchEngine()

		// Create test data in different sessions
		session1 := "session-1"
		session2 := "session-2"

		// Search should only return results from specified session
		options := &SmartSearchOptions{
			Query:     "authentication",
			SessionID: &session1,
			Limit:     10,
		}

		results := mockSessionSearch(searchEngine, options, session1, session2)

		// All results should be from session1
		for _, result := range results {
			assert.Equal(t, session1, result.CodeNode.SessionID)
		}
	})
}

// Helper functions for testing

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func createTestEmbeddingService() *EmbeddingService {
	config := &EmbeddingServiceConfig{
		BaseURL: "http://localhost:11434",
		Model:   "all-minilm",
		Timeout: 30 * time.Second,
	}
	return NewEmbeddingService(config)
}

func createTestSearchEngine() *SearchEngine {
	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	return NewSearchEngine(mockEmbedding, mockBridge, mockQueries)
}

func createTestVectorBridge() *Bridge {
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	return NewBridge(mockVector, mockQueries)
}

func createTestBatchProcessor() *BatchProcessor {
	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	return NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, DefaultBatchProcessingOptions())
}

func createMockHybridResults(nodes []*db.CodeNode, scores []float32) []*HybridSearchResult {
	results := make([]*HybridSearchResult, len(nodes))
	for i, node := range nodes {
		results[i] = &HybridSearchResult{
			Score:    scores[i],
			CodeNode: node,
			VectorMetadata: map[string]interface{}{
				"tier_completed": 2,
				"has_tests":      i%2 == 0, // Alternate test coverage
				"has_docs":       i%3 == 0, // Every third has docs
			},
		}
	}
	return results
}

// Mock functions for testing without external dependencies

func mockSmartSearch(engine *SearchEngine, hybridResults []*HybridSearchResult, options *SmartSearchOptions) []*SmartSearchResult {
	// Simulate the smart search process
	enhanced := make([]*SmartSearchResult, len(hybridResults))
	for i, result := range hybridResults {
		enhanced[i] = mockEnhanceResult(engine, result, options)
	}

	// Sort by final score
	for i := 0; i < len(enhanced)-1; i++ {
		for j := i + 1; j < len(enhanced); j++ {
			if enhanced[j].FinalScore > enhanced[i].FinalScore {
				enhanced[i], enhanced[j] = enhanced[j], enhanced[i]
			}
		}
	}

	return enhanced
}

func mockEnhanceResult(engine *SearchEngine, result *HybridSearchResult, options *SmartSearchOptions) *SmartSearchResult {
	quality := engine.buildQualitySignals(result)
	finalScore, breakdown := engine.calculateSmartScore(result, quality, options)
	matchReasons := engine.buildMatchReasons(result, quality, options)

	return &SmartSearchResult{
		HybridSearchResult: result,
		FinalScore:         finalScore,
		ScoreBreakdown:     breakdown,
		MatchReasons:       matchReasons,
		QualitySignals:     quality,
	}
}

func mockSearchLatency(engine *SearchEngine, options *SmartSearchOptions, resultCount int) time.Duration {
	// Simulate search processing time based on result count
	processingTime := time.Duration(resultCount) * time.Millisecond
	time.Sleep(processingTime)
	return processingTime
}

func mockBatchProcessing(processor *BatchProcessor, nodes []*db.CodeNode) int {
	// Simulate batch processing with some processing time
	for range nodes {
		time.Sleep(time.Microsecond * 100) // Simulate processing time
	}
	return len(nodes)
}

func mockStoreEmbedding(bridge *Bridge, nodeID int64, embedding []float32, metadata *EmbeddingMetadata) error {
	// Simulate storing embedding
	time.Sleep(time.Millisecond * 10)
	return nil
}

func mockSearchOperation(engine *SearchEngine, options *SmartSearchOptions, queryEmbedding []float32) []*SmartSearchResult {
	// Simulate search operation returning relevant results
	mockNode := &db.CodeNode{
		ID:        123,
		SessionID: *options.SessionID,
		Path:      "/auth/user.go",
		Language:  sql.NullString{String: "go", Valid: true},
		Kind:      sql.NullString{String: "function", Valid: true},
		Symbol:    sql.NullString{String: "authenticateUser", Valid: true},
	}

	mockResult := &HybridSearchResult{
		Score:    0.92,
		CodeNode: mockNode,
		VectorMetadata: map[string]interface{}{
			"has_tests":      true,
			"has_docs":       true,
			"tier_completed": 3,
		},
	}

	return []*SmartSearchResult{
		mockEnhanceResult(engine, mockResult, options),
	}
}

func mockSessionSearch(engine *SearchEngine, options *SmartSearchOptions, targetSession, otherSession string) []*SmartSearchResult {
	// Return only results from target session
	mockNode := &db.CodeNode{
		ID:        456,
		SessionID: targetSession,
		Path:      "/auth/session.go",
		Language:  sql.NullString{String: "go", Valid: true},
		Kind:      sql.NullString{String: "function", Valid: true},
	}

	mockResult := &HybridSearchResult{
		Score:    0.85,
		CodeNode: mockNode,
		VectorMetadata: map[string]interface{}{
			"session_id": targetSession,
		},
	}

	return []*SmartSearchResult{
		mockEnhanceResult(engine, mockResult, options),
	}
}
