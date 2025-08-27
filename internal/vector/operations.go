package vector

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

// CodeEmbedding represents a code node's vector embedding with metadata.
type CodeEmbedding struct {
	// NodeID corresponds to the code_nodes.id in the database
	NodeID int64 `json:"node_id"`
	// VectorID is the unique identifier in the vector database
	VectorID string `json:"vector_id"`
	// Vector is the embedding vector
	Vector []float32 `json:"vector"`
	// Metadata contains additional information for filtering
	Metadata map[string]interface{} `json:"metadata"`
}

// SearchResult represents a vector similarity search result.
type SearchResult struct {
	NodeID   int64                  `json:"node_id"`
	VectorID string                 `json:"vector_id"`
	Score    float32                `json:"score"`
	Metadata map[string]interface{} `json:"metadata"`
}

// UpsertEmbedding inserts or updates a single embedding in the vector database.
func (c *Client) UpsertEmbedding(ctx context.Context, embedding *CodeEmbedding) error {
	if embedding == nil {
		return fmt.Errorf("embedding cannot be nil")
	}

	// Prepare the point - using numeric ID based on NodeID
	point := &qdrant.PointStruct{
		Id:      qdrant.NewIDNum(uint64(embedding.NodeID)),
		Vectors: qdrant.NewVectors(embedding.Vector...),
		Payload: qdrant.NewValueMap(embedding.Metadata),
	}

	// Upsert the point
	_, err := c.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: c.config.Collection,
		Points:         []*qdrant.PointStruct{point},
	})

	if err != nil {
		return fmt.Errorf("failed to upsert embedding %s: %w", embedding.VectorID, err)
	}

	return nil
}

// UpsertEmbeddings inserts or updates multiple embeddings in batches.
func (c *Client) UpsertEmbeddings(ctx context.Context, embeddings []*CodeEmbedding) error {
	if len(embeddings) == 0 {
		return nil
	}

	batchSize := c.config.BatchSize
	if batchSize == 0 {
		batchSize = 100 // Default batch size
	}

	// Process embeddings in batches
	for i := 0; i < len(embeddings); i += batchSize {
		end := i + batchSize
		if end > len(embeddings) {
			end = len(embeddings)
		}

		batch := embeddings[i:end]
		points := make([]*qdrant.PointStruct, len(batch))

		// Convert batch to Qdrant points
		for j, embedding := range batch {
			points[j] = &qdrant.PointStruct{
				Id:      qdrant.NewIDNum(uint64(embedding.NodeID)),
				Vectors: qdrant.NewVectors(embedding.Vector...),
				Payload: qdrant.NewValueMap(embedding.Metadata),
			}
		}

		// Upsert the batch
		_, err := c.client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: c.config.Collection,
			Points:         points,
		})

		if err != nil {
			return fmt.Errorf("failed to upsert batch %d-%d: %w", i, end-1, err)
		}
	}

	return nil
}

// SearchSimilar performs similarity search with optional metadata filtering.
func (c *Client) SearchSimilar(ctx context.Context, vector []float32, limit int, filter map[string]interface{}) ([]*SearchResult, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("search vector cannot be empty")
	}

	if limit <= 0 {
		limit = 10 // Default limit
	}

	// Build the search request
	searchReq := &qdrant.QueryPoints{
		CollectionName: c.config.Collection,
		Query:          qdrant.NewQuery(vector...),
		Limit:          &[]uint64{uint64(limit)}[0],
		WithPayload:    qdrant.NewWithPayload(true),
	}

	// Add filter if provided
	if len(filter) > 0 {
		conditions := make([]*qdrant.Condition, 0, len(filter))
		for key, value := range filter {
			// Convert all values to strings for qdrant.NewMatch
			conditions = append(conditions, qdrant.NewMatch(key, fmt.Sprintf("%v", value)))
		}
		searchReq.Filter = &qdrant.Filter{
			Must: conditions,
		}
	}

	// Execute the search
	response, err := c.client.Query(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search vectors: %w", err)
	}

	// Convert results - response is directly []*qdrant.ScoredPoint
	results := make([]*SearchResult, len(response))
	for i, point := range response {
		// Extract node_id from point ID
		nodeID := int64(point.Id.GetNum())

		// Convert payload to map
		metadata := make(map[string]interface{})
		if point.Payload != nil {
			for k, v := range point.Payload {
				metadata[k] = extractValue(v)
			}
		}

		results[i] = &SearchResult{
			NodeID:   nodeID,
			VectorID: fmt.Sprintf("%d", point.Id.GetNum()),
			Score:    point.Score,
			Metadata: metadata,
		}
	}

	return results, nil
}

// DeleteEmbedding removes an embedding by vector ID.
func (c *Client) DeleteEmbedding(ctx context.Context, vectorID string) error {
	if vectorID == "" {
		return fmt.Errorf("vector ID cannot be empty")
	}

	// Convert string vectorID to numeric ID for Qdrant
	var numericID uint64
	_, err := fmt.Sscanf(vectorID, "%d", &numericID)
	if err != nil {
		return fmt.Errorf("invalid vector ID format: %w", err)
	}

	_, err = c.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: c.config.Collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: []*qdrant.PointId{qdrant.NewIDNum(numericID)},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to delete embedding %s: %w", vectorID, err)
	}

	return nil
}

// DeleteEmbeddingsByNodeID removes all embeddings associated with a specific node ID.
func (c *Client) DeleteEmbeddingsByNodeID(ctx context.Context, nodeID int64) error {
	_, err := c.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: c.config.Collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					Must: []*qdrant.Condition{
						qdrant.NewMatch("node_id", fmt.Sprintf("%d", nodeID)),
					},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to delete embeddings for node %d: %w", nodeID, err)
	}

	return nil
}

// DeleteEmbeddingsBySessionID removes all embeddings associated with a specific session ID.
func (c *Client) DeleteEmbeddingsBySessionID(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	_, err := c.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: c.config.Collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					Must: []*qdrant.Condition{
						qdrant.NewMatch("session_id", sessionID),
					},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to delete embeddings for session %s: %w", sessionID, err)
	}

	return nil
}

// CountEmbeddings returns the total number of embeddings in the collection.
func (c *Client) CountEmbeddings(ctx context.Context) (int64, error) {
	response, err := c.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: c.config.Collection,
	})

	if err != nil {
		return 0, fmt.Errorf("failed to count embeddings: %w", err)
	}

	return int64(response), nil
}

// CountEmbeddingsBySessionID returns the number of embeddings for a specific session.
func (c *Client) CountEmbeddingsBySessionID(ctx context.Context, sessionID string) (int64, error) {
	if sessionID == "" {
		return 0, fmt.Errorf("session ID cannot be empty")
	}

	response, err := c.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: c.config.Collection,
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("session_id", sessionID),
			},
		},
	})

	if err != nil {
		return 0, fmt.Errorf("failed to count embeddings for session %s: %w", sessionID, err)
	}

	return int64(response), nil
}

// extractValue converts a Qdrant value to a Go interface{}.
func extractValue(value *qdrant.Value) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.Kind.(type) {
	case *qdrant.Value_IntegerValue:
		return v.IntegerValue
	case *qdrant.Value_DoubleValue:
		return v.DoubleValue
	case *qdrant.Value_StringValue:
		return v.StringValue
	case *qdrant.Value_BoolValue:
		return v.BoolValue
	case *qdrant.Value_NullValue:
		return nil
	default:
		// For complex types, return as string representation
		return value.String()
	}
}
