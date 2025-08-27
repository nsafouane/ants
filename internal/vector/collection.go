package vector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/qdrant/go-client/qdrant"

	"github.com/charmbracelet/crush/internal/config"
)

// CollectionManager handles advanced collection setup, indexing, and optimization.
type CollectionManager struct {
	client *Client
	config *config.VectorDBConfig
}

// NewCollectionManager creates a new collection manager with the given client.
func NewCollectionManager(client *Client, config *config.VectorDBConfig) *CollectionManager {
	return &CollectionManager{
		client: client,
		config: config,
	}
}

// CollectionStrategy defines collection optimization strategies.
type CollectionStrategy struct {
	// Vector configuration
	VectorSize     uint64          `json:"vector_size"`     // Embedding dimensions
	DistanceMetric qdrant.Distance `json:"distance_metric"` // Similarity metric

	// Index optimization
	EnableHNSW bool             `json:"enable_hnsw"` // Enable HNSW index
	HNSWConfig *HNSWIndexConfig `json:"hnsw_config"` // HNSW parameters

	// Quantization for memory efficiency
	EnableQuantization bool                `json:"enable_quantization"` // Enable vector quantization
	QuantizationConfig *QuantizationConfig `json:"quantization_config"` // Quantization parameters

	// Payload indexing for fast filtering
	PayloadIndexes []PayloadIndex `json:"payload_indexes"` // Field indexes

	// Performance tuning
	OptimizersConfig *OptimizersConfig `json:"optimizers_config"` // Optimizer settings

	// Sharding strategy (for large collections)
	ShardNumber       *uint32 `json:"shard_number"`       // Number of shards
	ReplicationFactor *uint32 `json:"replication_factor"` // Replication factor
}

// HNSWIndexConfig configures the HNSW index parameters.
type HNSWIndexConfig struct {
	M                  uint64 `json:"m"`                    // Number of bi-directional links for every new element
	EfConstruct        uint64 `json:"ef_construct"`         // Size of the dynamic candidate list
	FullScanThreshold  uint64 `json:"full_scan_threshold"`  // Minimal size for switching to full scan
	MaxIndexingThreads uint64 `json:"max_indexing_threads"` // Number of parallel threads for indexing
	OnDisk             *bool  `json:"on_disk"`              // Store HNSW index on disk
}

// QuantizationConfig configures vector quantization for memory efficiency.
type QuantizationConfig struct {
	Scalar  *ScalarQuantization  `json:"scalar,omitempty"`  // Scalar quantization
	Product *ProductQuantization `json:"product,omitempty"` // Product quantization
	Binary  *BinaryQuantization  `json:"binary,omitempty"`  // Binary quantization
}

// ScalarQuantization reduces precision of vector components.
type ScalarQuantization struct {
	Type      string   `json:"type"`       // "int8"
	Quantile  *float32 `json:"quantile"`   // Quantile for clipping outliers
	AlwaysRam *bool    `json:"always_ram"` // Keep quantized vectors in RAM
}

// ProductQuantization reduces vector dimensions.
type ProductQuantization struct {
	Compression string `json:"compression"` // "x4", "x8", "x16", "x32", "x64"
	AlwaysRam   *bool  `json:"always_ram"`  // Keep quantized vectors in RAM
}

// BinaryQuantization converts vectors to binary representation.
type BinaryQuantization struct {
	AlwaysRam *bool `json:"always_ram"` // Keep quantized vectors in RAM
}

// PayloadIndex defines indexing strategy for metadata fields.
type PayloadIndex struct {
	FieldName   string              `json:"field_name"`   // Field to index
	FieldType   PayloadFieldType    `json:"field_type"`   // Field data type
	FieldSchema *PayloadFieldSchema `json:"field_schema"` // Field schema options
}

// PayloadFieldType represents the type of payload field.
type PayloadFieldType string

const (
	PayloadTypeKeyword PayloadFieldType = "keyword" // Exact match
	PayloadTypeInteger PayloadFieldType = "integer" // Numeric range
	PayloadTypeFloat   PayloadFieldType = "float"   // Numeric range
	PayloadTypeBool    PayloadFieldType = "bool"    // Boolean
	PayloadTypeGeo     PayloadFieldType = "geo"     // Geographic point
	PayloadTypeText    PayloadFieldType = "text"    // Full-text search
)

// PayloadFieldSchema defines additional options for payload fields.
type PayloadFieldSchema struct {
	// For keyword fields
	Tokenizer *string `json:"tokenizer,omitempty"` // "word", "whitespace", "prefix"

	// For text fields
	MinTokenLen *uint64 `json:"min_token_len,omitempty"` // Minimum token length
	MaxTokenLen *uint64 `json:"max_token_len,omitempty"` // Maximum token length
	Lowercase   *bool   `json:"lowercase,omitempty"`     // Convert to lowercase
}

// OptimizersConfig defines collection optimization parameters.
type OptimizersConfig struct {
	DeletedThreshold       *float64 `json:"deleted_threshold"`        // Trigger optimization when ratio of deleted > threshold
	VacuumMinVectorNumber  *uint64  `json:"vacuum_min_vector_number"` // Minimum vectors for vacuum
	DefaultSegmentNumber   *uint64  `json:"default_segment_number"`   // Default number of segments
	MaxSegmentSize         *uint64  `json:"max_segment_size"`         // Maximum segment size in KB
	MemmapThreshold        *uint64  `json:"memmap_threshold"`         // Size threshold for using mmap
	IndexingThreshold      *uint64  `json:"indexing_threshold"`       // Size threshold for indexing
	FlushIntervalSec       *uint64  `json:"flush_interval_sec"`       // Flush interval in seconds
	MaxOptimizationThreads *uint64  `json:"max_optimization_threads"` // Max threads for optimization
}

// DefaultCollectionStrategy returns optimized defaults for the Context Engine.
func DefaultCollectionStrategy() *CollectionStrategy {
	return &CollectionStrategy{
		VectorSize:     1536, // Ollama all-minilm default
		DistanceMetric: qdrant.Distance_Cosine,

		// Enable HNSW for fast approximate search
		EnableHNSW: true,
		HNSWConfig: &HNSWIndexConfig{
			M:                  16,                // Good balance between speed and accuracy
			EfConstruct:        100,               // Higher for better accuracy
			FullScanThreshold:  10000,             // Switch to full scan for small collections
			MaxIndexingThreads: 4,                 // Parallel indexing
			OnDisk:             &[]bool{false}[0], // Keep in memory for speed
		},

		// Enable scalar quantization for memory efficiency
		EnableQuantization: true,
		QuantizationConfig: &QuantizationConfig{
			Scalar: &ScalarQuantization{
				Type:      "int8",
				Quantile:  &[]float32{0.99}[0], // Clip 1% outliers
				AlwaysRam: &[]bool{true}[0],    // Keep in RAM
			},
		},

		// Index critical fields for fast filtering
		PayloadIndexes: []PayloadIndex{
			{
				FieldName: "session_id",
				FieldType: PayloadTypeKeyword,
				FieldSchema: &PayloadFieldSchema{
					Tokenizer: &[]string{"keyword"}[0],
				},
			},
			{
				FieldName: "language",
				FieldType: PayloadTypeKeyword,
				FieldSchema: &PayloadFieldSchema{
					Tokenizer: &[]string{"keyword"}[0],
				},
			},
			{
				FieldName: "kind",
				FieldType: PayloadTypeKeyword,
				FieldSchema: &PayloadFieldSchema{
					Tokenizer: &[]string{"keyword"}[0],
				},
			},
			{
				FieldName: "complexity",
				FieldType: PayloadTypeInteger,
			},
			{
				FieldName: "tier_completed",
				FieldType: PayloadTypeInteger,
			},
			{
				FieldName: "has_tests",
				FieldType: PayloadTypeBool,
			},
			{
				FieldName: "has_docs",
				FieldType: PayloadTypeBool,
			},
			{
				FieldName: "is_public",
				FieldType: PayloadTypeBool,
			},
			{
				FieldName: "is_entry",
				FieldType: PayloadTypeBool,
			},
			{
				FieldName: "risk_score",
				FieldType: PayloadTypeFloat,
			},
			{
				FieldName: "change_freq",
				FieldType: PayloadTypeKeyword,
				FieldSchema: &PayloadFieldSchema{
					Tokenizer: &[]string{"keyword"}[0],
				},
			},
			{
				FieldName: "path",
				FieldType: PayloadTypeText,
				FieldSchema: &PayloadFieldSchema{
					Tokenizer:   &[]string{"prefix"}[0], // Enable prefix search
					MinTokenLen: &[]uint64{1}[0],
					MaxTokenLen: &[]uint64{256}[0],
					Lowercase:   &[]bool{true}[0],
				},
			},
			{
				FieldName: "symbol",
				FieldType: PayloadTypeText,
				FieldSchema: &PayloadFieldSchema{
					Tokenizer:   &[]string{"prefix"}[0], // Enable prefix search
					MinTokenLen: &[]uint64{1}[0],
					MaxTokenLen: &[]uint64{100}[0],
					Lowercase:   &[]bool{false}[0], // Keep case for symbols
				},
			},
		},

		// Optimize for code analysis workloads
		OptimizersConfig: &OptimizersConfig{
			DeletedThreshold:       &[]float64{0.2}[0],   // Optimize when 20% deleted
			VacuumMinVectorNumber:  &[]uint64{1000}[0],   // Minimum vectors for vacuum
			DefaultSegmentNumber:   &[]uint64{4}[0],      // 4 segments for parallelism
			MaxSegmentSize:         &[]uint64{100000}[0], // 100MB max segment size
			MemmapThreshold:        &[]uint64{50000}[0],  // Use mmap for large segments
			IndexingThreshold:      &[]uint64{20000}[0],  // Index after 20k vectors
			FlushIntervalSec:       &[]uint64{5}[0],      // Flush every 5 seconds
			MaxOptimizationThreads: &[]uint64{2}[0],      // 2 threads for optimization
		},

		// Single shard for most deployments (can be overridden)
		ShardNumber:       &[]uint32{1}[0],
		ReplicationFactor: &[]uint32{1}[0],
	}
}

// CreateOptimizedCollection creates a collection with advanced indexing and optimization.
func (cm *CollectionManager) CreateOptimizedCollection(ctx context.Context, strategy *CollectionStrategy) error {
	if strategy == nil {
		strategy = DefaultCollectionStrategy()
	}

	slog.Info("Creating optimized vector collection",
		"collection", cm.config.Collection,
		"vector_size", strategy.VectorSize,
		"distance_metric", strategy.DistanceMetric,
		"quantization", strategy.EnableQuantization,
		"payload_indexes", len(strategy.PayloadIndexes))

	// Build vectors configuration
	vectorsConfig := qdrant.NewVectorsConfig(&qdrant.VectorParams{
		Size:     strategy.VectorSize,
		Distance: strategy.DistanceMetric,
	})

	// Build HNSW configuration if enabled
	var hnswConfig *qdrant.HnswConfigDiff
	if strategy.EnableHNSW && strategy.HNSWConfig != nil {
		hnswConfig = &qdrant.HnswConfigDiff{
			M:                  &strategy.HNSWConfig.M,
			EfConstruct:        &strategy.HNSWConfig.EfConstruct,
			FullScanThreshold:  &strategy.HNSWConfig.FullScanThreshold,
			MaxIndexingThreads: &strategy.HNSWConfig.MaxIndexingThreads,
			OnDisk:             strategy.HNSWConfig.OnDisk,
		}
	}

	// Build quantization configuration if enabled
	var quantizationConfig *qdrant.QuantizationConfig
	if strategy.EnableQuantization && strategy.QuantizationConfig != nil {
		quantizationConfig = cm.buildQuantizationConfig(strategy.QuantizationConfig)
	}

	// Build optimizers configuration
	var optimizersConfig *qdrant.OptimizersConfigDiff
	if strategy.OptimizersConfig != nil {
		optimizersConfig = &qdrant.OptimizersConfigDiff{
			DeletedThreshold:      strategy.OptimizersConfig.DeletedThreshold,
			VacuumMinVectorNumber: strategy.OptimizersConfig.VacuumMinVectorNumber,
			DefaultSegmentNumber:  strategy.OptimizersConfig.DefaultSegmentNumber,
			MaxSegmentSize:        strategy.OptimizersConfig.MaxSegmentSize,
			MemmapThreshold:       strategy.OptimizersConfig.MemmapThreshold,
			IndexingThreshold:     strategy.OptimizersConfig.IndexingThreshold,
			FlushIntervalSec:      strategy.OptimizersConfig.FlushIntervalSec,
			// Note: Skipping MaxOptimizationThreads due to API compatibility
		}
	}

	// Create collection request
	createReq := &qdrant.CreateCollection{
		CollectionName:     cm.config.Collection,
		VectorsConfig:      vectorsConfig,
		HnswConfig:         hnswConfig,
		QuantizationConfig: quantizationConfig,
		OptimizersConfig:   optimizersConfig,
		ShardNumber:        strategy.ShardNumber,
		ReplicationFactor:  strategy.ReplicationFactor,
	}

	// Create the collection
	if err := cm.client.client.CreateCollection(ctx, createReq); err != nil {
		return fmt.Errorf("failed to create optimized collection: %w", err)
	}

	// Create payload indexes for fast filtering
	if err := cm.createPayloadIndexes(ctx, strategy.PayloadIndexes); err != nil {
		return fmt.Errorf("failed to create payload indexes: %w", err)
	}

	slog.Info("Successfully created optimized vector collection",
		"collection", cm.config.Collection,
		"indexes_created", len(strategy.PayloadIndexes))

	return nil
}

// createPayloadIndexes creates indexes on payload fields for fast filtering.
func (cm *CollectionManager) createPayloadIndexes(ctx context.Context, indexes []PayloadIndex) error {
	for _, index := range indexes {
		slog.Debug("Creating payload index",
			"field", index.FieldName,
			"type", index.FieldType)

		// Convert our types to Qdrant types
		fieldType := cm.convertPayloadFieldType(index.FieldType)

		// Build field index request
		indexReq := &qdrant.CreateFieldIndexCollection{
			CollectionName: cm.config.Collection,
			FieldName:      index.FieldName,
			FieldType:      &fieldType,
		}

		// Add field schema if provided
		if index.FieldSchema != nil {
			indexReq.FieldIndexParams = cm.buildFieldIndexParams(index.FieldSchema)
		}

		// Create the index - handle return values properly
		_, err := cm.client.client.CreateFieldIndex(ctx, indexReq)
		if err != nil {
			return fmt.Errorf("failed to create index for field %s: %w", index.FieldName, err)
		}
	}

	return nil
}

// convertPayloadFieldType converts our field type to Qdrant field type.
func (cm *CollectionManager) convertPayloadFieldType(fieldType PayloadFieldType) qdrant.FieldType {
	switch fieldType {
	case PayloadTypeKeyword:
		return qdrant.FieldType_FieldTypeKeyword
	case PayloadTypeInteger:
		return qdrant.FieldType_FieldTypeInteger
	case PayloadTypeFloat:
		return qdrant.FieldType_FieldTypeFloat
	case PayloadTypeBool:
		return qdrant.FieldType_FieldTypeBool
	case PayloadTypeGeo:
		return qdrant.FieldType_FieldTypeGeo
	case PayloadTypeText:
		return qdrant.FieldType_FieldTypeText
	default:
		return qdrant.FieldType_FieldTypeKeyword // Default fallback
	}
}

// buildFieldIndexParams converts field schema to Qdrant parameters.
func (cm *CollectionManager) buildFieldIndexParams(schema *PayloadFieldSchema) *qdrant.PayloadIndexParams {
	params := &qdrant.PayloadIndexParams{}

	// Text field parameters
	if schema.Tokenizer != nil {
		// Note: This depends on Qdrant Go client supporting text field parameters
		// May need to be adjusted based on the actual API
	}

	return params
}

// buildQuantizationConfig converts our quantization config to Qdrant config.
func (cm *CollectionManager) buildQuantizationConfig(config *QuantizationConfig) *qdrant.QuantizationConfig {
	if config == nil {
		return nil
	}

	if config.Scalar != nil {
		return &qdrant.QuantizationConfig{
			Quantization: &qdrant.QuantizationConfig_Scalar{
				Scalar: &qdrant.ScalarQuantization{
					Type:      qdrant.QuantizationType_Int8, // Assuming int8
					Quantile:  config.Scalar.Quantile,
					AlwaysRam: config.Scalar.AlwaysRam,
				},
			},
		}
	}

	if config.Product != nil {
		// Note: Product quantization implementation depends on Qdrant client
		// This is a placeholder and needs actual implementation
		return &qdrant.QuantizationConfig{}
	}

	if config.Binary != nil {
		return &qdrant.QuantizationConfig{
			Quantization: &qdrant.QuantizationConfig_Binary{
				Binary: &qdrant.BinaryQuantization{
					AlwaysRam: config.Binary.AlwaysRam,
				},
			},
		}
	}

	return nil
}

// OptimizeCollection manually triggers collection optimization.
func (cm *CollectionManager) OptimizeCollection(ctx context.Context) error {
	slog.Info("Optimizing vector collection", "collection", cm.config.Collection)

	// Trigger collection optimization
	err := cm.client.client.UpdateCollection(ctx, &qdrant.UpdateCollection{
		CollectionName: cm.config.Collection,
		OptimizersConfig: &qdrant.OptimizersConfigDiff{
			// Force optimization by setting deleted threshold to 0
			DeletedThreshold: &[]float64{0.0}[0],
		},
	})

	if err != nil {
		return fmt.Errorf("failed to optimize collection: %w", err)
	}

	slog.Info("Collection optimization triggered", "collection", cm.config.Collection)
	return nil
}

// GetCollectionInfo returns detailed information about the collection.
func (cm *CollectionManager) GetCollectionInfo(ctx context.Context) (*qdrant.CollectionInfo, error) {
	// Use DescribeCollection instead of GetCollection
	return cm.client.client.GetCollectionInfo(ctx, cm.config.Collection)
}

// UpdateCollectionStrategy updates collection configuration with new strategy.
func (cm *CollectionManager) UpdateCollectionStrategy(ctx context.Context, strategy *CollectionStrategy) error {
	slog.Info("Updating collection strategy", "collection", cm.config.Collection)

	// Build update request with new configuration
	updateReq := &qdrant.UpdateCollection{
		CollectionName: cm.config.Collection,
	}

	// Update optimizers if provided
	if strategy.OptimizersConfig != nil {
		updateReq.OptimizersConfig = &qdrant.OptimizersConfigDiff{
			DeletedThreshold:      strategy.OptimizersConfig.DeletedThreshold,
			VacuumMinVectorNumber: strategy.OptimizersConfig.VacuumMinVectorNumber,
			DefaultSegmentNumber:  strategy.OptimizersConfig.DefaultSegmentNumber,
			MaxSegmentSize:        strategy.OptimizersConfig.MaxSegmentSize,
			MemmapThreshold:       strategy.OptimizersConfig.MemmapThreshold,
			IndexingThreshold:     strategy.OptimizersConfig.IndexingThreshold,
			FlushIntervalSec:      strategy.OptimizersConfig.FlushIntervalSec,
			// Note: Skipping MaxOptimizationThreads due to API compatibility
		}
	}

	// Update HNSW configuration if provided
	if strategy.EnableHNSW && strategy.HNSWConfig != nil {
		updateReq.HnswConfig = &qdrant.HnswConfigDiff{
			M:                  &strategy.HNSWConfig.M,
			EfConstruct:        &strategy.HNSWConfig.EfConstruct,
			FullScanThreshold:  &strategy.HNSWConfig.FullScanThreshold,
			MaxIndexingThreads: &strategy.HNSWConfig.MaxIndexingThreads,
			OnDisk:             strategy.HNSWConfig.OnDisk,
		}
	}

	// Apply the update
	err := cm.client.client.UpdateCollection(ctx, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update collection strategy: %w", err)
	}

	slog.Info("Successfully updated collection strategy", "collection", cm.config.Collection)
	return nil
}
