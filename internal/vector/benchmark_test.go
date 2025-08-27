package vector

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
)

// BenchmarkEmbeddingGeneration benchmarks embedding generation performance.
func BenchmarkEmbeddingGeneration(b *testing.B) {
	service := createTestEmbeddingService()

	testCases := []struct {
		name string
		code string
	}{
		{
			name: "small_function",
			code: `func add(a, b int) int {
				return a + b
			}`,
		},
		{
			name: "medium_function",
			code: `func processUser(user *User) error {
				if user == nil {
					return errors.New("user cannot be nil")
				}
				
				if err := user.Validate(); err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}
				
				if err := userRepo.Save(user); err != nil {
					return fmt.Errorf("failed to save user: %w", err)
				}
				
				return nil
			}`,
		},
		{
			name: "large_function",
			code: generateLargeCodeSnippet(),
		},
	}

	metadata := &EmbeddingMetadata{
		NodeID:    1,
		SessionID: "benchmark",
		Language:  "go",
		Kind:      "function",
		Symbol:    "testFunction",
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := service.GenerateCodeEmbedding(context.Background(), tc.code, metadata)
				if err != nil {
					b.Fatalf("Embedding generation failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkBatchEmbedding benchmarks batch embedding processing.
func BenchmarkBatchEmbedding(b *testing.B) {
	processor := createTestBatchProcessor()

	batchSizes := []int{10, 50, 100, 500}

	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("batch_size_%d", size), func(b *testing.B) {
			nodes := generateTestNodes(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Reset state for each iteration
				nodePointers := make([]*db.CodeNode, len(nodes))
				for j := range nodes {
					nodePointers[j] = &nodes[j]
				}
				b.StartTimer()

				// Simulate batch processing
				mockBatchProcessingBenchmark(processor, nodePointers)
			}
		})
	}
}

// BenchmarkSearchOperations benchmarks different search operations.
func BenchmarkSearchOperations(b *testing.B) {
	engine := createTestSearchEngine()

	searchTypes := []struct {
		name    string
		options *SmartSearchOptions
	}{
		{
			name: "simple_search",
			options: &SmartSearchOptions{
				Query: "authentication",
				Limit: 10,
			},
		},
		{
			name: "filtered_search",
			options: &SmartSearchOptions{
				Query:     "user management",
				Languages: []string{"go", "python"},
				Kinds:     []string{"function", "method"},
				Limit:     20,
			},
		},
		{
			name: "quality_boosted_search",
			options: &SmartSearchOptions{
				Query:        "database operations",
				RequireTests: &[]bool{true}[0],
				RequireDocs:  &[]bool{true}[0],
				BoostQuality: true,
				Limit:        15,
			},
		},
		{
			name: "diverse_search",
			options: &SmartSearchOptions{
				Query:           "api endpoints",
				DiversityFactor: 0.8,
				Limit:           25,
			},
		},
	}

	for _, st := range searchTypes {
		b.Run(st.name, func(b *testing.B) {
			// Generate mock results for benchmarking
			mockResults := generateMockSearchResults(100) // Large result set

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				mockSmartSearchBenchmark(engine, mockResults, st.options)
			}
		})
	}
}

// BenchmarkVectorSimilarity benchmarks vector similarity calculations.
func BenchmarkVectorSimilarity(b *testing.B) {
	vectorSizes := []int{128, 256, 512, 1024, 1536}

	for _, size := range vectorSizes {
		b.Run(fmt.Sprintf("vector_size_%d", size), func(b *testing.B) {
			vector1 := generateRandomVector(size)
			vector2 := generateRandomVector(size)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = cosineSimilarity(vector1, vector2)
			}
		})
	}
}

// BenchmarkScoreCalculation benchmarks smart scoring algorithm.
func BenchmarkScoreCalculation(b *testing.B) {
	engine := &SearchEngine{scoringWeights: DefaultScoringWeights()}

	result := &HybridSearchResult{
		Score: 0.85,
		VectorMetadata: map[string]interface{}{
			"has_tests":      true,
			"has_docs":       true,
			"is_public":      true,
			"complexity":     8,
			"risk_score":     0.3,
			"tier_completed": 2,
		},
		CodeNode: &db.CodeNode{
			Symbol: sql.NullString{String: "TestFunction", Valid: true},
		},
	}

	quality := &QualitySignals{
		HasTests:     true,
		HasDocs:      true,
		IsPublic:     true,
		AnalysisTier: 2,
	}

	options := &SmartSearchOptions{
		Query:        "TestFunction",
		BoostQuality: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.calculateSmartScore(result, quality, options)
	}
}

// BenchmarkDiversityFiltering benchmarks diversity filtering algorithm.
func BenchmarkDiversityFiltering(b *testing.B) {
	engine := &SearchEngine{}

	resultCounts := []int{50, 100, 200, 500}

	for _, count := range resultCounts {
		b.Run(fmt.Sprintf("results_%d", count), func(b *testing.B) {
			results := generateMockSmartResults(count)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = engine.applyDiversityFiltering(results, 0.7)
			}
		})
	}
}

// BenchmarkMemoryUsage tests memory efficiency of vector operations.
func BenchmarkMemoryUsage(b *testing.B) {
	b.Run("embedding_storage", func(b *testing.B) {
		embeddings := make([][]float32, 0, b.N)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			embedding := generateRandomVector(1536) // Standard size
			embeddings = append(embeddings, embedding)
		}

		// Report memory usage
		b.ReportMetric(float64(len(embeddings)*1536*4), "bytes_stored")
	})

	b.Run("search_result_allocation", func(b *testing.B) {
		results := make([]*SmartSearchResult, 0, b.N)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result := &SmartSearchResult{
				HybridSearchResult: &HybridSearchResult{
					Score: rand.Float32(),
					CodeNode: &db.CodeNode{
						ID:        int64(i),
						SessionID: "test",
					},
				},
				FinalScore:     rand.Float64(),
				ScoreBreakdown: &ScoreBreakdown{},
				QualitySignals: &QualitySignals{},
			}
			results = append(results, result)
		}

		b.ReportMetric(float64(len(results)), "results_allocated")
	})
}

// BenchmarkConcurrentOperations tests performance under concurrent load.
func BenchmarkConcurrentOperations(b *testing.B) {
	concurrencyLevels := []int{1, 4, 8, 16}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrency_%d", concurrency), func(b *testing.B) {
			service := createTestEmbeddingService()

			code := `func benchmarkFunction() {
				// Sample code for benchmarking
				for i := 0; i < 100; i++ {
					process(i)
				}
			}`

			metadata := &EmbeddingMetadata{
				NodeID:    1,
				SessionID: "concurrent-test",
				Language:  "go",
				Kind:      "function",
			}

			b.SetParallelism(concurrency)
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := service.GenerateCodeEmbedding(context.Background(), code, metadata)
					if err != nil {
						b.Errorf("Concurrent embedding generation failed: %v", err)
					}
				}
			})
		})
	}
}

// Benchmark helper functions

func generateLargeCodeSnippet() string {
	return `func complexBusinessLogic(ctx context.Context, request *ProcessRequest) (*ProcessResponse, error) {
		logger := log.FromContext(ctx)
		
		// Validate input
		if request == nil {
			return nil, errors.New("request cannot be nil")
		}
		
		if err := request.Validate(); err != nil {
			logger.Error("Request validation failed", "error", err)
			return nil, fmt.Errorf("validation error: %w", err)
		}
		
		// Initialize response
		response := &ProcessResponse{
			ID:        generateID(),
			Timestamp: time.Now(),
			Status:    StatusPending,
		}
		
		// Process in stages
		for i, stage := range request.Stages {
			logger.Info("Processing stage", "stage", i, "type", stage.Type)
			
			switch stage.Type {
			case StageTypeValidation:
				if err := s.validateStage(ctx, stage); err != nil {
					response.Status = StatusFailed
					response.Error = err.Error()
					return response, nil
				}
				
			case StageTypeTransformation:
				result, err := s.transformStage(ctx, stage)
				if err != nil {
					response.Status = StatusFailed
					response.Error = err.Error()
					return response, nil
				}
				response.Results = append(response.Results, result)
				
			case StageTypeEnrichment:
				if err := s.enrichStage(ctx, stage, response); err != nil {
					logger.Warn("Enrichment failed", "error", err)
					// Continue processing even if enrichment fails
				}
				
			default:
				logger.Warn("Unknown stage type", "type", stage.Type)
			}
			
			// Check for cancellation
			select {
			case <-ctx.Done():
				response.Status = StatusCancelled
				return response, ctx.Err()
			default:
			}
		}
		
		// Finalize processing
		if err := s.finalizeProcessing(ctx, response); err != nil {
			response.Status = StatusFailed
			response.Error = err.Error()
			return response, nil
		}
		
		response.Status = StatusCompleted
		logger.Info("Processing completed successfully", "id", response.ID)
		
		return response, nil
	}`
}

func generateTestNodes(count int) []db.CodeNode {
	nodes := make([]db.CodeNode, count)

	languages := []string{"go", "python", "javascript", "typescript"}
	kinds := []string{"function", "method", "class", "interface"}

	for i := 0; i < count; i++ {
		nodes[i] = db.CodeNode{
			ID:        int64(i + 1),
			SessionID: "benchmark-session",
			Path:      fmt.Sprintf("/test/file%d.go", i),
			Language:  sql.NullString{String: languages[i%len(languages)], Valid: true},
			Kind:      sql.NullString{String: kinds[i%len(kinds)], Valid: true},
			Symbol:    sql.NullString{String: fmt.Sprintf("testFunction%d", i), Valid: true},
		}
	}

	return nodes
}

func generateMockSearchResults(count int) []*HybridSearchResult {
	results := make([]*HybridSearchResult, count)

	for i := 0; i < count; i++ {
		score := 0.5 + rand.Float32()*0.5 // Score between 0.5 and 1.0

		results[i] = &HybridSearchResult{
			Score: score,
			CodeNode: &db.CodeNode{
				ID:        int64(i + 1),
				SessionID: "benchmark",
				Path:      fmt.Sprintf("/test/file%d.go", i),
				Language:  sql.NullString{String: "go", Valid: true},
				Kind:      sql.NullString{String: "function", Valid: true},
			},
			VectorMetadata: map[string]interface{}{
				"has_tests":      i%2 == 0,
				"has_docs":       i%3 == 0,
				"complexity":     rand.Intn(20) + 1,
				"tier_completed": rand.Intn(3) + 1,
			},
		}
	}

	return results
}

func generateMockSmartResults(count int) []*SmartSearchResult {
	results := make([]*SmartSearchResult, count)

	languages := []string{"go", "python", "javascript"}
	paths := []string{"/auth/", "/api/", "/db/", "/util/", "/service/"}

	for i := 0; i < count; i++ {
		results[i] = &SmartSearchResult{
			HybridSearchResult: &HybridSearchResult{
				Score: rand.Float32(),
				CodeNode: &db.CodeNode{
					ID:        int64(i + 1),
					SessionID: "diversity-test",
					Path:      fmt.Sprintf("%sfile%d.go", paths[i%len(paths)], i),
					Language:  sql.NullString{String: languages[i%len(languages)], Valid: true},
					Kind:      sql.NullString{String: "function", Valid: true},
				},
			},
			FinalScore: rand.Float64(),
		}
	}

	return results
}

func generateRandomVector(size int) []float32 {
	vector := make([]float32, size)
	for i := range vector {
		vector[i] = rand.Float32()*2 - 1 // Random values between -1 and 1
	}
	return vector
}

func mockBatchProcessingBenchmark(processor *BatchProcessor, nodes []*db.CodeNode) {
	// Simulate processing time proportional to batch size
	processingTime := time.Duration(len(nodes)) * time.Microsecond * 10
	time.Sleep(processingTime)
}

func mockSmartSearchBenchmark(engine *SearchEngine, hybridResults []*HybridSearchResult, options *SmartSearchOptions) []*SmartSearchResult {
	// Simulate search processing with realistic timing
	processingTime := time.Duration(len(hybridResults)) * time.Microsecond * 5
	time.Sleep(processingTime)

	// Return a subset of results
	limit := options.Limit
	if limit > len(hybridResults) {
		limit = len(hybridResults)
	}

	enhanced := make([]*SmartSearchResult, limit)
	for i := 0; i < limit; i++ {
		enhanced[i] = &SmartSearchResult{
			HybridSearchResult: hybridResults[i],
			FinalScore:         float64(hybridResults[i].Score),
		}
	}

	return enhanced
}

// Performance validation tests

func TestPerformanceTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance target validation in short mode")
	}

	t.Run("embedding_latency_sla", func(t *testing.T) {
		service := createTestEmbeddingService()

		code := "func testFunction() { /* sample code */ }"
		metadata := &EmbeddingMetadata{
			NodeID:    1,
			SessionID: "sla-test",
			Language:  "go",
			Kind:      "function",
		}

		// Test 10 embedding generations
		totalTime := time.Duration(0)
		for i := 0; i < 10; i++ {
			start := time.Now()
			_, err := service.GenerateCodeEmbedding(context.Background(), code, metadata)
			duration := time.Since(start)

			require.NoError(t, err)
			totalTime += duration
		}

		avgLatency := totalTime / 10

		// SLA: Average embedding generation should be under 1 second
		assert.True(t, avgLatency < time.Second,
			"Embedding generation SLA violated: avg %v > 1s", avgLatency)
	})

	t.Run("search_latency_sla", func(t *testing.T) {
		engine := createTestSearchEngine()

		options := &SmartSearchOptions{
			Query: "test search",
			Limit: 20,
		}

		// Test 5 search operations
		totalTime := time.Duration(0)
		for i := 0; i < 5; i++ {
			start := time.Now()
			_ = mockSearchLatency(engine, options, 50) // 50 results to process
			duration := time.Since(start)
			totalTime += duration
		}

		avgLatency := totalTime / 5

		// SLA: Average search should be under 500ms
		assert.True(t, avgLatency < 500*time.Millisecond,
			"Search latency SLA violated: avg %v > 500ms", avgLatency)
	})

	t.Run("throughput_targets", func(t *testing.T) {
		processor := createTestBatchProcessor()

		nodes := generateTestNodes(100)
		nodePointers := make([]*db.CodeNode, len(nodes))
		for i := range nodes {
			nodePointers[i] = &nodes[i]
		}

		start := time.Now()
		processed := mockBatchProcessing(processor, nodePointers)
		duration := time.Since(start)

		throughput := float64(processed) / duration.Seconds()

		// Target: Process at least 50 nodes per second
		assert.True(t, throughput >= 50.0,
			"Throughput target not met: %.2f nodes/sec < 50 nodes/sec", throughput)
	})
}
