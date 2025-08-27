package vector

import (
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/config"
)

func TestDefaultCollectionStrategy(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	require.NotNil(t, strategy)
	assert.Equal(t, uint64(1536), strategy.VectorSize)
	assert.Equal(t, qdrant.Distance_Cosine, strategy.DistanceMetric)
	assert.True(t, strategy.EnableHNSW)
	assert.True(t, strategy.EnableQuantization)
	assert.NotEmpty(t, strategy.PayloadIndexes)

	// Check critical payload indexes are present
	indexNames := make(map[string]PayloadFieldType)
	for _, idx := range strategy.PayloadIndexes {
		indexNames[idx.FieldName] = idx.FieldType
	}

	// Verify essential indexes for Context Engine
	assert.Equal(t, PayloadTypeKeyword, indexNames["session_id"])
	assert.Equal(t, PayloadTypeKeyword, indexNames["language"])
	assert.Equal(t, PayloadTypeKeyword, indexNames["kind"])
	assert.Equal(t, PayloadTypeInteger, indexNames["complexity"])
	assert.Equal(t, PayloadTypeInteger, indexNames["tier_completed"])
	assert.Equal(t, PayloadTypeBool, indexNames["has_tests"])
	assert.Equal(t, PayloadTypeBool, indexNames["has_docs"])
	assert.Equal(t, PayloadTypeFloat, indexNames["risk_score"])
	assert.Equal(t, PayloadTypeText, indexNames["path"])
	assert.Equal(t, PayloadTypeText, indexNames["symbol"])
}

func TestHNSWIndexConfig(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	require.NotNil(t, strategy.HNSWConfig)

	// Verify optimal HNSW parameters for code search
	assert.Equal(t, uint64(16), strategy.HNSWConfig.M)
	assert.Equal(t, uint64(100), strategy.HNSWConfig.EfConstruct)
	assert.Equal(t, uint64(10000), strategy.HNSWConfig.FullScanThreshold)
	assert.Equal(t, uint64(4), strategy.HNSWConfig.MaxIndexingThreads)
	assert.NotNil(t, strategy.HNSWConfig.OnDisk)
	assert.False(t, *strategy.HNSWConfig.OnDisk) // Keep in memory for speed
}

func TestQuantizationConfig(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	require.NotNil(t, strategy.QuantizationConfig)
	require.NotNil(t, strategy.QuantizationConfig.Scalar)

	// Verify scalar quantization settings
	assert.Equal(t, "int8", strategy.QuantizationConfig.Scalar.Type)
	assert.NotNil(t, strategy.QuantizationConfig.Scalar.Quantile)
	assert.Equal(t, float32(0.99), *strategy.QuantizationConfig.Scalar.Quantile)
	assert.NotNil(t, strategy.QuantizationConfig.Scalar.AlwaysRam)
	assert.True(t, *strategy.QuantizationConfig.Scalar.AlwaysRam)
}

func TestOptimizersConfig(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	require.NotNil(t, strategy.OptimizersConfig)

	// Verify optimizer settings for code analysis workloads
	assert.NotNil(t, strategy.OptimizersConfig.DeletedThreshold)
	assert.Equal(t, 0.2, *strategy.OptimizersConfig.DeletedThreshold)

	assert.NotNil(t, strategy.OptimizersConfig.VacuumMinVectorNumber)
	assert.Equal(t, uint64(1000), *strategy.OptimizersConfig.VacuumMinVectorNumber)

	assert.NotNil(t, strategy.OptimizersConfig.DefaultSegmentNumber)
	assert.Equal(t, uint64(4), *strategy.OptimizersConfig.DefaultSegmentNumber)

	assert.NotNil(t, strategy.OptimizersConfig.FlushIntervalSec)
	assert.Equal(t, uint64(5), *strategy.OptimizersConfig.FlushIntervalSec)
}

func TestNewCollectionManager(t *testing.T) {
	t.Parallel()

	cfg := &config.VectorDBConfig{
		Collection: "test_collection",
	}

	client := &Client{config: cfg}
	manager := NewCollectionManager(client, cfg)

	require.NotNil(t, manager)
	assert.Equal(t, client, manager.client)
	assert.Equal(t, cfg, manager.config)
}

func TestPayloadFieldTypeConversion(t *testing.T) {
	t.Parallel()

	cfg := &config.VectorDBConfig{Collection: "test"}
	client := &Client{config: cfg}
	manager := NewCollectionManager(client, cfg)

	tests := []struct {
		input    PayloadFieldType
		expected qdrant.FieldType
	}{
		{PayloadTypeKeyword, qdrant.FieldType_FieldTypeKeyword},
		{PayloadTypeInteger, qdrant.FieldType_FieldTypeInteger},
		{PayloadTypeFloat, qdrant.FieldType_FieldTypeFloat},
		{PayloadTypeBool, qdrant.FieldType_FieldTypeBool},
		{PayloadTypeGeo, qdrant.FieldType_FieldTypeGeo},
		{PayloadTypeText, qdrant.FieldType_FieldTypeText},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := manager.convertPayloadFieldType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildQuantizationConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.VectorDBConfig{Collection: "test"}
	client := &Client{config: cfg}
	manager := NewCollectionManager(client, cfg)

	t.Run("scalar quantization", func(t *testing.T) {
		t.Parallel()

		config := &QuantizationConfig{
			Scalar: &ScalarQuantization{
				Type:      "int8",
				Quantile:  &[]float32{0.95}[0],
				AlwaysRam: &[]bool{true}[0],
			},
		}

		result := manager.buildQuantizationConfig(config)
		require.NotNil(t, result)

		scalarConfig := result.GetScalar()
		require.NotNil(t, scalarConfig)
		assert.Equal(t, qdrant.QuantizationType_Int8, scalarConfig.Type)
		assert.NotNil(t, scalarConfig.Quantile)
		assert.Equal(t, float32(0.95), *scalarConfig.Quantile)
		assert.NotNil(t, scalarConfig.AlwaysRam)
		assert.True(t, *scalarConfig.AlwaysRam)
	})

	t.Run("binary quantization", func(t *testing.T) {
		t.Parallel()

		config := &QuantizationConfig{
			Binary: &BinaryQuantization{
				AlwaysRam: &[]bool{false}[0],
			},
		}

		result := manager.buildQuantizationConfig(config)
		require.NotNil(t, result)

		binaryConfig := result.GetBinary()
		require.NotNil(t, binaryConfig)
		assert.NotNil(t, binaryConfig.AlwaysRam)
		assert.False(t, *binaryConfig.AlwaysRam)
	})

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		result := manager.buildQuantizationConfig(nil)
		assert.Nil(t, result)
	})
}

func TestPayloadIndexValidation(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	// Verify all critical Context Engine fields are indexed
	requiredFields := map[string]PayloadFieldType{
		"session_id":     PayloadTypeKeyword,
		"language":       PayloadTypeKeyword,
		"kind":           PayloadTypeKeyword,
		"complexity":     PayloadTypeInteger,
		"tier_completed": PayloadTypeInteger,
		"has_tests":      PayloadTypeBool,
		"has_docs":       PayloadTypeBool,
		"is_public":      PayloadTypeBool,
		"is_entry":       PayloadTypeBool,
		"risk_score":     PayloadTypeFloat,
		"change_freq":    PayloadTypeKeyword,
		"path":           PayloadTypeText,
		"symbol":         PayloadTypeText,
	}

	indexedFields := make(map[string]PayloadFieldType)
	for _, index := range strategy.PayloadIndexes {
		indexedFields[index.FieldName] = index.FieldType
	}

	for field, expectedType := range requiredFields {
		actualType, exists := indexedFields[field]
		assert.True(t, exists, "Required field %s is not indexed", field)
		assert.Equal(t, expectedType, actualType, "Field %s has wrong index type", field)
	}
}

func TestTextFieldIndexConfiguration(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	// Find text field indexes
	var pathIndex, symbolIndex *PayloadIndex
	for i, index := range strategy.PayloadIndexes {
		if index.FieldName == "path" {
			pathIndex = &strategy.PayloadIndexes[i]
		}
		if index.FieldName == "symbol" {
			symbolIndex = &strategy.PayloadIndexes[i]
		}
	}

	// Verify path index configuration
	require.NotNil(t, pathIndex)
	assert.Equal(t, PayloadTypeText, pathIndex.FieldType)
	require.NotNil(t, pathIndex.FieldSchema)
	require.NotNil(t, pathIndex.FieldSchema.Tokenizer)
	assert.Equal(t, "prefix", *pathIndex.FieldSchema.Tokenizer)
	require.NotNil(t, pathIndex.FieldSchema.Lowercase)
	assert.True(t, *pathIndex.FieldSchema.Lowercase)

	// Verify symbol index configuration
	require.NotNil(t, symbolIndex)
	assert.Equal(t, PayloadTypeText, symbolIndex.FieldType)
	require.NotNil(t, symbolIndex.FieldSchema)
	require.NotNil(t, symbolIndex.FieldSchema.Tokenizer)
	assert.Equal(t, "prefix", *symbolIndex.FieldSchema.Tokenizer)
	require.NotNil(t, symbolIndex.FieldSchema.Lowercase)
	assert.False(t, *symbolIndex.FieldSchema.Lowercase) // Keep case for symbols
}

func TestCollectionStrategySharding(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	// Verify default sharding configuration
	require.NotNil(t, strategy.ShardNumber)
	assert.Equal(t, uint32(1), *strategy.ShardNumber)

	require.NotNil(t, strategy.ReplicationFactor)
	assert.Equal(t, uint32(1), *strategy.ReplicationFactor)
}

// BenchmarkCollectionStrategyCreation benchmarks strategy creation performance.
func BenchmarkCollectionStrategyCreation(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		strategy := DefaultCollectionStrategy()
		_ = strategy // Prevent optimization
	}
}

// BenchmarkPayloadIndexConversion benchmarks field type conversion.
func BenchmarkPayloadIndexConversion(b *testing.B) {
	cfg := &config.VectorDBConfig{Collection: "test"}
	client := &Client{config: cfg}
	manager := NewCollectionManager(client, cfg)

	fieldTypes := []PayloadFieldType{
		PayloadTypeKeyword,
		PayloadTypeInteger,
		PayloadTypeFloat,
		PayloadTypeBool,
		PayloadTypeText,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, fieldType := range fieldTypes {
			_ = manager.convertPayloadFieldType(fieldType)
		}
	}
}

// TestCollectionStrategyMemoryUsage verifies memory-efficient configuration.
func TestCollectionStrategyMemoryUsage(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	// Verify quantization is enabled for memory efficiency
	assert.True(t, strategy.EnableQuantization)
	require.NotNil(t, strategy.QuantizationConfig)
	require.NotNil(t, strategy.QuantizationConfig.Scalar)
	assert.Equal(t, "int8", strategy.QuantizationConfig.Scalar.Type)

	// Verify HNSW is configured for memory efficiency
	require.NotNil(t, strategy.HNSWConfig)
	assert.NotNil(t, strategy.HNSWConfig.OnDisk)
	assert.False(t, *strategy.HNSWConfig.OnDisk) // In-memory for speed

	// Verify optimizer thresholds are reasonable
	require.NotNil(t, strategy.OptimizersConfig)
	require.NotNil(t, strategy.OptimizersConfig.MemmapThreshold)
	assert.Equal(t, uint64(50000), *strategy.OptimizersConfig.MemmapThreshold)
}

// TestCollectionStrategyPerformance verifies performance-oriented configuration.
func TestCollectionStrategyPerformance(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	// Verify HNSW is enabled for fast search
	assert.True(t, strategy.EnableHNSW)
	require.NotNil(t, strategy.HNSWConfig)

	// Verify HNSW parameters are tuned for performance
	assert.Equal(t, uint64(16), strategy.HNSWConfig.M)                 // Good connectivity
	assert.Equal(t, uint64(100), strategy.HNSWConfig.EfConstruct)      // High accuracy
	assert.Equal(t, uint64(4), strategy.HNSWConfig.MaxIndexingThreads) // Parallel indexing

	// Verify optimizer settings support fast operations
	require.NotNil(t, strategy.OptimizersConfig)
	require.NotNil(t, strategy.OptimizersConfig.FlushIntervalSec)
	assert.Equal(t, uint64(5), *strategy.OptimizersConfig.FlushIntervalSec) // Fast commits

	require.NotNil(t, strategy.OptimizersConfig.MaxOptimizationThreads)
	assert.Equal(t, uint64(2), *strategy.OptimizersConfig.MaxOptimizationThreads)
}

// TestContextEngineSpecificIndexes verifies indexes are tailored for Context Engine queries.
func TestContextEngineSpecificIndexes(t *testing.T) {
	t.Parallel()

	strategy := DefaultCollectionStrategy()

	// Build index map for easier testing
	indexes := make(map[string]PayloadIndex)
	for _, idx := range strategy.PayloadIndexes {
		indexes[idx.FieldName] = idx
	}

	// Verify session-scoped queries are optimized
	sessionIdx, exists := indexes["session_id"]
	require.True(t, exists)
	assert.Equal(t, PayloadTypeKeyword, sessionIdx.FieldType)

	// Verify language-based filtering is optimized
	langIdx, exists := indexes["language"]
	require.True(t, exists)
	assert.Equal(t, PayloadTypeKeyword, langIdx.FieldType)

	// Verify analysis tier filtering is optimized
	tierIdx, exists := indexes["tier_completed"]
	require.True(t, exists)
	assert.Equal(t, PayloadTypeInteger, tierIdx.FieldType)

	// Verify code quality filtering is optimized
	testsIdx, exists := indexes["has_tests"]
	require.True(t, exists)
	assert.Equal(t, PayloadTypeBool, testsIdx.FieldType)

	docsIdx, exists := indexes["has_docs"]
	require.True(t, exists)
	assert.Equal(t, PayloadTypeBool, docsIdx.FieldType)

	// Verify risk-based filtering is optimized
	riskIdx, exists := indexes["risk_score"]
	require.True(t, exists)
	assert.Equal(t, PayloadTypeFloat, riskIdx.FieldType)
}
