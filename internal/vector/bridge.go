package vector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/crush/internal/db"
)

// QueryInterface defines the database queries needed by the bridge.
type QueryInterface interface {
	GetCodeNode(ctx context.Context, id int64) (db.CodeNode, error)
	ListCodeNodesBySession(ctx context.Context, sessionID string) ([]db.CodeNode, error)
	CountCodeNodesBySession(ctx context.Context, sessionID string) (int64, error)
	GetEmbeddingByNode(ctx context.Context, nodeID int64) (db.Embedding, error)
	UpdateEmbedding(ctx context.Context, params db.UpdateEmbeddingParams) (db.Embedding, error)
	CreateEmbedding(ctx context.Context, params db.CreateEmbeddingParams) (db.Embedding, error)
	DeleteEmbeddingByNode(ctx context.Context, nodeID int64) error
	DeleteEmbeddingsBySession(ctx context.Context, sessionID sql.NullString) error
}

// Ensure db.Queries implements QueryInterface
var _ QueryInterface = (*db.Queries)(nil)

// Bridge provides hybrid search capabilities combining vector similarity with SQL queries.
type Bridge struct {
	vectorClient VectorClient
	queries      QueryInterface
}

// NewBridge creates a new vector-to-SQL bridge for hybrid search operations.
func NewBridge(vectorClient VectorClient, queries QueryInterface) *Bridge {
	return &Bridge{
		vectorClient: vectorClient,
		queries:      queries,
	}
}

// HybridSearchOptions configures hybrid search behavior.
type HybridSearchOptions struct {
	// Vector search parameters
	Vector      []float32     `json:"vector"`
	VectorLimit int           `json:"vector_limit"` // Initial vector search limit
	Filter      *SearchFilter `json:"filter"`       // Vector metadata filtering

	// SQL filtering parameters
	SessionID     *string  `json:"session_id,omitempty"`
	Languages     []string `json:"languages,omitempty"`
	Kinds         []string `json:"kinds,omitempty"`
	MinComplexity *int     `json:"min_complexity,omitempty"`
	MaxComplexity *int     `json:"max_complexity,omitempty"`

	// Result parameters
	FinalLimit int `json:"final_limit"` // Final result limit after SQL filtering
}

// HybridSearchResult combines vector similarity scores with full SQL data.
type HybridSearchResult struct {
	// Vector data
	Score    float32 `json:"score"`
	VectorID string  `json:"vector_id"`

	// SQL data
	CodeNode  *db.CodeNode  `json:"code_node"`
	Embedding *db.Embedding `json:"embedding"`

	// Combined metadata
	VectorMetadata map[string]interface{} `json:"vector_metadata"`
}

// Search performs hybrid vector + SQL search.
func (b *Bridge) Search(ctx context.Context, options *HybridSearchOptions) ([]*HybridSearchResult, error) {
	if options == nil {
		return nil, fmt.Errorf("search options cannot be nil")
	}

	if len(options.Vector) == 0 {
		return nil, fmt.Errorf("search vector cannot be empty")
	}

	// Set defaults
	if options.VectorLimit == 0 {
		options.VectorLimit = 50 // Get more vector results for SQL filtering
	}
	if options.FinalLimit == 0 {
		options.FinalLimit = 10 // Default final result limit
	}

	// Step 1: Perform vector similarity search
	vectorFilter := make(map[string]interface{})
	if options.Filter != nil {
		vectorFilter = options.Filter.ToQdrantFilter()
	}

	vectorResults, err := b.vectorClient.SearchSimilar(ctx, options.Vector, options.VectorLimit, vectorFilter)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	if len(vectorResults) == 0 {
		return nil, nil // No vector results
	}

	// Step 2: Extract node IDs for SQL queries
	nodeIDs := make([]int64, len(vectorResults))
	vectorResultMap := make(map[int64]*SearchResult)

	for i, result := range vectorResults {
		nodeIDs[i] = result.NodeID
		vectorResultMap[result.NodeID] = result
	}

	// Step 3: Fetch corresponding code nodes with SQL filtering
	filteredNodes, err := b.getFilteredCodeNodes(ctx, nodeIDs, options)
	if err != nil {
		return nil, fmt.Errorf("SQL filtering failed: %w", err)
	}

	// Step 4: Fetch embeddings for the filtered nodes
	embeddings, err := b.getEmbeddingsByNodeIDs(ctx, getNodeIDs(filteredNodes))
	if err != nil {
		return nil, fmt.Errorf("embedding fetch failed: %w", err)
	}

	embeddingMap := make(map[int64]*db.Embedding)
	for _, embedding := range embeddings {
		embeddingMap[embedding.NodeID] = embedding
	}

	// Step 5: Combine results and sort by vector score
	results := make([]*HybridSearchResult, 0, len(filteredNodes))

	for _, node := range filteredNodes {
		vectorResult := vectorResultMap[node.ID]
		if vectorResult == nil {
			continue // This shouldn't happen, but safety check
		}

		embedding := embeddingMap[node.ID]

		result := &HybridSearchResult{
			Score:          vectorResult.Score,
			VectorID:       vectorResult.VectorID,
			CodeNode:       node,
			Embedding:      embedding,
			VectorMetadata: vectorResult.Metadata,
		}

		results = append(results, result)
	}

	// Results are already sorted by vector score (from vector search)
	// Apply final limit
	if len(results) > options.FinalLimit {
		results = results[:options.FinalLimit]
	}

	return results, nil
}

// getFilteredCodeNodes fetches code nodes with SQL filtering applied.
func (b *Bridge) getFilteredCodeNodes(ctx context.Context, nodeIDs []int64, options *HybridSearchOptions) ([]*db.CodeNode, error) {
	// For now, implement a simple approach - fetch all nodes and filter in memory
	// In a production system, you'd want to build dynamic SQL queries

	allNodes := make([]*db.CodeNode, 0, len(nodeIDs))

	for _, nodeID := range nodeIDs {
		node, err := b.queries.GetCodeNode(ctx, nodeID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue // Node doesn't exist, skip
			}
			return nil, fmt.Errorf("failed to get code node %d: %w", nodeID, err)
		}

		// Apply SQL filters
		if b.matchesFilters(&node, options) {
			allNodes = append(allNodes, &node)
		}
	}

	return allNodes, nil
}

// matchesFilters checks if a code node matches the SQL filtering criteria.
func (b *Bridge) matchesFilters(node *db.CodeNode, options *HybridSearchOptions) bool {
	// Session ID filter
	if options.SessionID != nil && node.SessionID != *options.SessionID {
		return false
	}

	// Language filter
	if len(options.Languages) > 0 {
		nodeLanguage := ""
		if node.Language.Valid {
			nodeLanguage = node.Language.String
		}

		found := false
		for _, lang := range options.Languages {
			if nodeLanguage == lang {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Kind filter
	if len(options.Kinds) > 0 {
		nodeKind := ""
		if node.Kind.Valid {
			nodeKind = node.Kind.String
		}

		found := false
		for _, kind := range options.Kinds {
			if nodeKind == kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Complexity filters (extract from metadata)
	if options.MinComplexity != nil || options.MaxComplexity != nil {
		complexity := b.extractComplexityFromMetadata(node.Metadata)

		if options.MinComplexity != nil && complexity < *options.MinComplexity {
			return false
		}

		if options.MaxComplexity != nil && complexity > *options.MaxComplexity {
			return false
		}
	}

	return true
}

// extractComplexityFromMetadata extracts complexity value from node metadata.
func (b *Bridge) extractComplexityFromMetadata(metadata interface{}) int {
	if metadata == nil {
		return 0
	}

	// Try to extract complexity from JSON metadata
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return 0
	}

	var metadataMap map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadataMap); err != nil {
		return 0
	}

	if complexity, ok := metadataMap["complexity"]; ok {
		if complexityFloat, ok := complexity.(float64); ok {
			return int(complexityFloat)
		}
	}

	return 0
}

// getEmbeddingsByNodeIDs fetches embeddings for multiple node IDs.
func (b *Bridge) getEmbeddingsByNodeIDs(ctx context.Context, nodeIDs []int64) ([]*db.Embedding, error) {
	embeddings := make([]*db.Embedding, 0, len(nodeIDs))

	for _, nodeID := range nodeIDs {
		embedding, err := b.queries.GetEmbeddingByNode(ctx, nodeID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue // No embedding for this node, skip
			}
			return nil, fmt.Errorf("failed to get embedding for node %d: %w", nodeID, err)
		}

		embeddings = append(embeddings, &embedding)
	}

	return embeddings, nil
}

// getNodeIDs extracts node IDs from a slice of code nodes.
func getNodeIDs(nodes []*db.CodeNode) []int64 {
	ids := make([]int64, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	return ids
}

// SyncEmbedding synchronizes an embedding between vector database and SQL database.
func (b *Bridge) SyncEmbedding(ctx context.Context, nodeID int64, vector []float32, metadata *EmbeddingMetadata) error {
	// Step 1: Upsert to vector database
	vectorID := fmt.Sprintf("%d", nodeID)
	codeEmbedding := &CodeEmbedding{
		NodeID:   nodeID,
		VectorID: vectorID,
		Vector:   vector,
		Metadata: metadata.ToMap(),
	}

	if err := b.vectorClient.UpsertEmbedding(ctx, codeEmbedding); err != nil {
		return fmt.Errorf("failed to upsert to vector database: %w", err)
	}

	// Step 2: Upsert to SQL database
	vectorBytes, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("failed to marshal vector: %w", err)
	}

	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Try to update existing embedding first
	// Note: We need to get the embedding ID first to update it
	existingEmbedding, err := b.queries.GetEmbeddingByNode(ctx, nodeID)
	if err == nil {
		// Update existing embedding
		_, err = b.queries.UpdateEmbedding(ctx, db.UpdateEmbeddingParams{
			ID:       existingEmbedding.ID,
			VectorID: sql.NullString{String: vectorID, Valid: true},
			Vector:   vectorBytes,
			Dims:     sql.NullInt64{Int64: int64(len(vector)), Valid: true},
			Metadata: metadataBytes,
		})
		if err != nil {
			return fmt.Errorf("failed to update embedding in SQL: %w", err)
		}
	} else {
		// Create new embedding
		_, err = b.queries.CreateEmbedding(ctx, db.CreateEmbeddingParams{
			NodeID:    nodeID,
			SessionID: sql.NullString{String: metadata.SessionID, Valid: metadata.SessionID != ""},
			VectorID:  sql.NullString{String: vectorID, Valid: true},
			Vector:    vectorBytes,
			Dims:      sql.NullInt64{Int64: int64(len(vector)), Valid: true},
			Metadata:  metadataBytes,
		})
		if err != nil {
			return fmt.Errorf("failed to create embedding in SQL: %w", err)
		}
	}

	return nil
}

// DeleteEmbedding removes an embedding from both vector and SQL databases.
func (b *Bridge) DeleteEmbedding(ctx context.Context, nodeID int64) error {
	// Step 1: Delete from vector database
	if err := b.vectorClient.DeleteEmbeddingsByNodeID(ctx, nodeID); err != nil {
		return fmt.Errorf("failed to delete from vector database: %w", err)
	}

	// Step 2: Delete from SQL database
	if err := b.queries.DeleteEmbeddingByNode(ctx, nodeID); err != nil {
		return fmt.Errorf("failed to delete from SQL database: %w", err)
	}

	return nil
}

// DeleteEmbeddingsBySession removes all embeddings for a session from both databases.
func (b *Bridge) DeleteEmbeddingsBySession(ctx context.Context, sessionID string) error {
	// Step 1: Delete from vector database
	if err := b.vectorClient.DeleteEmbeddingsBySessionID(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session embeddings from vector database: %w", err)
	}

	// Step 2: Delete from SQL database
	sessionIDParam := sql.NullString{String: sessionID, Valid: true}
	if err := b.queries.DeleteEmbeddingsBySession(ctx, sessionIDParam); err != nil {
		return fmt.Errorf("failed to delete session embeddings from SQL database: %w", err)
	}

	return nil
}
