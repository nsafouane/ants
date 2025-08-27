package vector

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
)

func TestDefaultScoringWeights(t *testing.T) {
	t.Parallel()

	weights := DefaultScoringWeights()

	require.NotNil(t, weights)
	assert.Equal(t, 1.0, weights.VectorSimilarity)
	assert.Equal(t, 0.3, weights.AnalysisTier)
	assert.Equal(t, 0.2, weights.CodeQuality)
	assert.Equal(t, 0.1, weights.Recency)
	assert.Equal(t, -0.1, weights.Complexity)
	assert.Equal(t, -0.2, weights.RiskScore)
	assert.Equal(t, 0.15, weights.PopularityBoost)
	assert.Equal(t, 0.25, weights.SymbolMatch)
}

func TestDefaultFilterOptimizer(t *testing.T) {
	t.Parallel()

	optimizer := DefaultFilterOptimizer()

	require.NotNil(t, optimizer)
	assert.Contains(t, optimizer.sessionFilters, "session_id")
	assert.Contains(t, optimizer.booleanFilters, "has_tests")
	assert.Contains(t, optimizer.booleanFilters, "has_docs")
	assert.Contains(t, optimizer.rangeFilters, "complexity")
	assert.Contains(t, optimizer.rangeFilters, "risk_score")
	assert.Contains(t, optimizer.textFilters, "symbol")
	assert.Contains(t, optimizer.textFilters, "path")
}

func TestNewSearchEngine(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)

	engine := NewSearchEngine(mockEmbedding, mockBridge, mockQueries)

	require.NotNil(t, engine)
	assert.Equal(t, mockEmbedding, engine.embeddingService)
	assert.Equal(t, mockBridge, engine.vectorBridge)
	assert.Equal(t, mockQueries, engine.queries)
	assert.Equal(t, 10, engine.defaultLimit)
	assert.Equal(t, 100, engine.maxLimit)
	assert.NotNil(t, engine.scoringWeights)
	assert.NotNil(t, engine.filterOptimizer)
}

func TestSearchEngine_PrepareQueryEmbedding(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	engine := NewSearchEngine(mockEmbedding, mockBridge, mockQueries)

	t.Run("use provided embedding", func(t *testing.T) {
		t.Parallel()

		options := &SmartSearchOptions{
			QueryEmbedding: []float32{0.1, 0.2, 0.3},
		}

		embedding, err := engine.prepareQueryEmbedding(context.Background(), options)

		require.NoError(t, err)
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, embedding)
	})

	t.Run("generate from query text", func(t *testing.T) {
		t.Parallel()

		options := &SmartSearchOptions{
			Query: "test function",
		}

		expectedEmbedding := []float32{0.4, 0.5, 0.6}
		mockEmbedding.On("GenerateEmbedding", mock.Anything, "test function").Return(expectedEmbedding, nil)

		embedding, err := engine.prepareQueryEmbedding(context.Background(), options)

		require.NoError(t, err)
		assert.Equal(t, expectedEmbedding, embedding)
		mockEmbedding.AssertExpectations(t)
	})

	t.Run("no query or embedding", func(t *testing.T) {
		t.Parallel()

		options := &SmartSearchOptions{}

		_, err := engine.prepareQueryEmbedding(context.Background(), options)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "either query text or query embedding must be provided")
	})
}

func TestSearchEngine_BuildSearchConfig(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{defaultLimit: 10, maxLimit: 100}

	t.Run("basic config", func(t *testing.T) {
		t.Parallel()

		sessionID := "test-session"
		options := &SmartSearchOptions{
			Limit:     20,
			SessionID: &sessionID,
			Languages: []string{"go", "python"},
			Kinds:     []string{"function", "method"},
		}

		config := engine.buildSearchConfig(options)

		require.NotNil(t, config)
		assert.Equal(t, 60, config.VectorLimit) // 20 * 3
		assert.Equal(t, 40, config.FinalLimit)  // 20 * 2
		assert.Equal(t, &sessionID, config.SessionID)
		assert.Equal(t, []string{"go", "python"}, config.Languages)
		assert.Equal(t, []string{"function", "method"}, config.Kinds)
	})

	t.Run("with complexity filter", func(t *testing.T) {
		t.Parallel()

		maxComplexity := 10
		options := &SmartSearchOptions{
			Limit:         5,
			MaxComplexity: &maxComplexity,
		}

		config := engine.buildSearchConfig(options)

		require.NotNil(t, config)
		assert.Equal(t, &maxComplexity, config.MaxComplexity)
	})

	t.Run("vector limit bounds", func(t *testing.T) {
		t.Parallel()

		// Test minimum bound
		options1 := &SmartSearchOptions{Limit: 1}
		config1 := engine.buildSearchConfig(options1)
		assert.Equal(t, 50, config1.VectorLimit) // Minimum 50

		// Test maximum bound
		options2 := &SmartSearchOptions{Limit: 100}
		config2 := engine.buildSearchConfig(options2)
		assert.Equal(t, 200, config2.VectorLimit) // Maximum 200
	})
}

func TestSearchEngine_CalculateSmartScore(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{scoringWeights: DefaultScoringWeights()}

	result := &HybridSearchResult{
		Score: 0.8,
		VectorMetadata: map[string]interface{}{
			"complexity": 5,
			"risk_score": 0.3,
		},
		CodeNode: &db.CodeNode{
			Symbol: sql.NullString{String: "TestFunction", Valid: true},
		},
	}

	quality := &QualitySignals{
		HasTests:     true,
		HasDocs:      true,
		IsPublic:     true,
		AnalysisTier: 3,
	}

	options := &SmartSearchOptions{
		Query:        "TestFunction",
		BoostRecent:  true,
		BoostQuality: true,
	}

	finalScore, breakdown := engine.calculateSmartScore(result, quality, options)

	require.NotNil(t, breakdown)

	// Check that base vector score is preserved
	assert.Equal(t, 0.8, breakdown.VectorScore)

	// Check that bonuses are applied
	assert.True(t, breakdown.TierBonus > 0)       // Analysis tier 3
	assert.True(t, breakdown.QualityBonus > 0)    // Has tests and docs
	assert.True(t, breakdown.PopularityBonus > 0) // Is public
	assert.True(t, breakdown.SymbolBonus > 0)     // Symbol match

	// Check that penalties are applied
	assert.True(t, breakdown.ComplexityScore < 0) // Complexity penalty
	assert.True(t, breakdown.RiskPenalty < 0)     // Risk penalty

	// Final score should be higher than base vector score due to bonuses
	assert.True(t, finalScore > 0.8)
	assert.True(t, finalScore >= 0) // Score should never be negative
}

func TestSearchEngine_BuildQualitySignals(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	result := &HybridSearchResult{
		VectorMetadata: map[string]interface{}{
			"has_tests":      true,
			"has_docs":       false,
			"is_public":      true,
			"is_entry":       false,
			"tier_completed": 2,
			"complexity":     8,
			"risk_score":     0.4,
			"tags":           []string{"controller", "api"},
		},
	}

	signals := engine.buildQualitySignals(result)

	require.NotNil(t, signals)
	assert.True(t, signals.HasTests)
	assert.False(t, signals.HasDocs)
	assert.True(t, signals.IsPublic)
	assert.False(t, signals.IsEntry)
	assert.Equal(t, 2, signals.AnalysisTier)
	assert.Equal(t, "medium", signals.ComplexityLevel) // 8 is medium complexity
	assert.Equal(t, "medium", signals.RiskLevel)       // 0.4 is medium risk
	assert.Equal(t, []string{"controller", "api"}, signals.Tags)
}

func TestSearchEngine_BuildMatchReasons(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	result := &HybridSearchResult{
		Score: 0.85,
		CodeNode: &db.CodeNode{
			Symbol:   sql.NullString{String: "getUserById", Valid: true},
			Language: sql.NullString{String: "go", Valid: true},
		},
	}

	quality := &QualitySignals{
		HasTests:     true,
		HasDocs:      true,
		AnalysisTier: 3,
	}

	options := &SmartSearchOptions{
		Query:     "getUserById",
		Languages: []string{"go", "python"},
	}

	reasons := engine.buildMatchReasons(result, quality, options)

	require.NotEmpty(t, reasons)
	assert.Contains(t, reasons, "High semantic similarity")
	assert.Contains(t, reasons, "High-quality code with tests and documentation")
	assert.Contains(t, reasons, "Fully analyzed code (Tier 3)")
	assert.Contains(t, reasons, "Symbol name matches query")
	assert.Contains(t, reasons, "Written in go")
}

func TestSearchEngine_CategorizeComplexity(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	tests := []struct {
		complexity int
		expected   string
	}{
		{1, "low"},
		{5, "low"},
		{8, "medium"},
		{15, "medium"},
		{20, "high"},
		{50, "high"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("complexity_%d", tt.complexity), func(t *testing.T) {
			result := engine.categorizeComplexity(tt.complexity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSearchEngine_CategorizeRisk(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	tests := []struct {
		riskScore float32
		expected  string
	}{
		{0.1, "low"},
		{0.3, "low"},
		{0.5, "medium"},
		{0.7, "medium"},
		{0.9, "high"},
		{1.0, "high"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("risk_%.1f", tt.riskScore), func(t *testing.T) {
			result := engine.categorizeRisk(tt.riskScore)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSearchEngine_IsSymbolMatch(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	tests := []struct {
		query    string
		symbol   string
		expected bool
	}{
		{"getUserById", "getUserById", true},
		{"getUserById", "getUserById()", true},
		{"getUser", "getUserById", true},  // Prefix match
		{"user", "getUserById", false},    // Too short
		{"User", "getUserById", false},    // Case sensitive
		{"getData", "getUserById", false}, // No match
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.query, tt.symbol), func(t *testing.T) {
			result := engine.isSymbolMatch(tt.query, tt.symbol)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSearchEngine_CalculateDiversityScore(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	resultA := &SmartSearchResult{
		HybridSearchResult: &HybridSearchResult{
			CodeNode: &db.CodeNode{
				Path:     "/src/user/service.go",
				Language: sql.NullString{String: "go", Valid: true},
				Kind:     sql.NullString{String: "function", Valid: true},
			},
		},
	}

	resultB := &SmartSearchResult{
		HybridSearchResult: &HybridSearchResult{
			CodeNode: &db.CodeNode{
				Path:     "/src/user/repository.go",
				Language: sql.NullString{String: "go", Valid: true},
				Kind:     sql.NullString{String: "method", Valid: true},
			},
		},
	}

	resultC := &SmartSearchResult{
		HybridSearchResult: &HybridSearchResult{
			CodeNode: &db.CodeNode{
				Path:     "/frontend/components/User.tsx",
				Language: sql.NullString{String: "typescript", Valid: true},
				Kind:     sql.NullString{String: "function", Valid: true},
			},
		},
	}

	// Same language, different kind and path
	scoreAB := engine.calculateDiversityScore(resultA, resultB)
	assert.True(t, scoreAB > 0.5) // Should be fairly diverse

	// Different language, path, same kind
	scoreAC := engine.calculateDiversityScore(resultA, resultC)
	assert.True(t, scoreAC > 0.8) // Should be very diverse

	// Same result should have 0 diversity
	scoreAA := engine.calculateDiversityScore(resultA, resultA)
	assert.True(t, scoreAA < 0.5) // Should be low diversity
}

func TestSearchEngine_ApplyDiversityFiltering(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	results := []*SmartSearchResult{
		{
			HybridSearchResult: &HybridSearchResult{
				CodeNode: &db.CodeNode{
					Path:     "/src/user.go",
					Language: sql.NullString{String: "go", Valid: true},
				},
			},
			FinalScore: 1.0,
		},
		{
			HybridSearchResult: &HybridSearchResult{
				CodeNode: &db.CodeNode{
					Path:     "/src/user.go", // Same file
					Language: sql.NullString{String: "go", Valid: true},
				},
			},
			FinalScore: 0.9,
		},
		{
			HybridSearchResult: &HybridSearchResult{
				CodeNode: &db.CodeNode{
					Path:     "/frontend/user.ts", // Different file and language
					Language: sql.NullString{String: "typescript", Valid: true},
				},
			},
			FinalScore: 0.8,
		},
	}

	t.Run("no diversity filtering", func(t *testing.T) {
		t.Parallel()

		filtered := engine.applyDiversityFiltering(results, 0.0)
		assert.Len(t, filtered, 3) // All results should remain
	})

	t.Run("high diversity filtering", func(t *testing.T) {
		t.Parallel()

		filtered := engine.applyDiversityFiltering(results, 0.8)
		assert.Len(t, filtered, 2)                   // Should filter out the duplicate
		assert.Equal(t, 1.0, filtered[0].FinalScore) // Top result always included
		assert.Equal(t, 0.8, filtered[1].FinalScore) // Diverse result included
	})
}

func TestSearchEngine_HealthCheck(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	engine := NewSearchEngine(mockEmbedding, mockBridge, mockQueries)

	t.Run("healthy", func(t *testing.T) {
		t.Parallel()

		mockEmbedding.On("HealthCheck", mock.Anything).Return(nil)
		mockVector.On("Ping", mock.Anything).Return(nil)

		err := engine.HealthCheck(context.Background())

		require.NoError(t, err)
		mockEmbedding.AssertExpectations(t)
		mockVector.AssertExpectations(t)
	})

	t.Run("embedding service unhealthy", func(t *testing.T) {
		t.Parallel()

		mockEmbedding.On("HealthCheck", mock.Anything).Return(fmt.Errorf("service down"))

		err := engine.HealthCheck(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "embedding service health check failed")
		mockEmbedding.AssertExpectations(t)
	})

	t.Run("vector client unhealthy", func(t *testing.T) {
		t.Parallel()

		mockEmbedding.On("HealthCheck", mock.Anything).Return(nil)
		mockVector.On("Ping", mock.Anything).Return(fmt.Errorf("connection failed"))

		err := engine.HealthCheck(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "vector client health check failed")
		mockEmbedding.AssertExpectations(t)
		mockVector.AssertExpectations(t)
	})
}

// BenchmarkSmartScoring benchmarks the scoring calculation performance.
func BenchmarkSmartScoring(b *testing.B) {
	engine := &SearchEngine{scoringWeights: DefaultScoringWeights()}

	result := &HybridSearchResult{
		Score: 0.8,
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
		Query: "TestFunction",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = engine.calculateSmartScore(result, quality, options)
	}
}

func TestSearchEngine_ExtractMetadata(t *testing.T) {
	t.Parallel()

	engine := &SearchEngine{}

	result := &HybridSearchResult{
		VectorMetadata: map[string]interface{}{
			"complexity": float64(10),
			"risk_score": float32(0.5),
		},
	}

	// Test complexity extraction
	complexity := engine.extractComplexity(result)
	assert.Equal(t, 10, complexity)

	// Test risk score extraction
	riskScore := engine.extractRiskScore(result)
	assert.Equal(t, float32(0.5), riskScore)

	// Test with missing metadata
	emptyResult := &HybridSearchResult{VectorMetadata: map[string]interface{}{}}
	assert.Equal(t, 0, engine.extractComplexity(emptyResult))
	assert.Equal(t, float32(0.0), engine.extractRiskScore(emptyResult))
}
