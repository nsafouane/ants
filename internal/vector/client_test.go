package vector

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/config"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *config.VectorDBConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "minimal config",
			config: &config.VectorDBConfig{
				Type:       "qdrant",
				URL:        "http://localhost",
				Collection: "test_collection",
			},
			wantErr: false,
		},
		{
			name: "full config",
			config: &config.VectorDBConfig{
				Type:                "qdrant",
				URL:                 "http://localhost",
				Port:                6334,
				Collection:          "test_collection",
				UseTLS:              false,
				ConnectionTimeoutMs: 5000,
				RequestTimeoutMs:    30000,
				BatchSize:           100,
				VectorSize:          1536,
				DistanceMetric:      "Cosine",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				require.NotNil(t, client.client)
				require.Equal(t, tt.config, client.config)

				// Clean up
				require.NoError(t, client.Close())
			}
		})
	}
}

func TestEmbeddingMetadata_ToMap(t *testing.T) {
	t.Parallel()

	metadata := &EmbeddingMetadata{
		NodeID:        123,
		SessionID:     "test-session",
		Path:          "/src/main.go",
		Language:      "go",
		Kind:          "function",
		Symbol:        "main",
		StartLine:     10,
		EndLine:       20,
		Complexity:    5,
		LineCount:     11,
		LastModified:  1643723400,
		ChangeFreq:    "warm",
		Authors:       2,
		TierCompleted: 2,
		Tags:          []string{"entry", "main"},
		Summary:       "Application entry point",
		RiskScore:     0.3,
		HasTests:      true,
		HasDocs:       false,
		IsEntry:       true,
		IsPublic:      true,
		IsDeprecated:  false,
	}

	result := metadata.ToMap()

	require.Equal(t, int64(123), result["node_id"])
	require.Equal(t, "test-session", result["session_id"])
	require.Equal(t, "/src/main.go", result["path"])
	require.Equal(t, "go", result["language"])
	require.Equal(t, "function", result["kind"])
	require.Equal(t, "main", result["symbol"])
	require.Equal(t, 10, result["start_line"])
	require.Equal(t, 20, result["end_line"])
	require.Equal(t, 5, result["complexity"])
	require.Equal(t, 11, result["line_count"])
	require.Equal(t, int64(1643723400), result["last_modified"])
	require.Equal(t, "warm", result["change_freq"])
	require.Equal(t, 2, result["authors"])
	require.Equal(t, 2, result["tier_completed"])
	require.Equal(t, []string{"entry", "main"}, result["tags"])
	require.Equal(t, "Application entry point", result["summary"])
	require.Equal(t, float32(0.3), result["risk_score"])
	require.Equal(t, true, result["has_tests"])
	require.Equal(t, false, result["has_docs"])
	require.Equal(t, true, result["is_entry"])
	require.Equal(t, true, result["is_public"])
	require.Equal(t, false, result["is_deprecated"])
}

func TestSearchFilter_ToQdrantFilter(t *testing.T) {
	t.Parallel()

	sessionID := "test-session"
	hasTests := true
	minTier := 2

	filter := &SearchFilter{
		SessionID:        &sessionID,
		Languages:        []string{"go"},
		Kinds:            []string{"function"},
		MinTierCompleted: &minTier,
		HasTests:         &hasTests,
	}

	result := filter.ToQdrantFilter()

	require.Equal(t, "test-session", result["session_id"])
	require.Equal(t, "go", result["language"])
	require.Equal(t, "function", result["kind"])
	require.Equal(t, 2, result["tier_completed"])
	require.Equal(t, true, result["has_tests"])
}

func TestDefaultSearchOptions(t *testing.T) {
	t.Parallel()

	options := DefaultSearchOptions()

	require.Equal(t, 10, options.Limit)
	require.Equal(t, float32(0.7), options.Threshold)
	require.Nil(t, options.Filter)
	require.True(t, options.IncludeMetadata)
}

func TestCodeEmbedding(t *testing.T) {
	t.Parallel()

	embedding := &CodeEmbedding{
		NodeID:   123,
		VectorID: "test-vector-123",
		Vector:   []float32{0.1, 0.2, 0.3, 0.4},
		Metadata: map[string]interface{}{
			"test_key": "test_value",
		},
	}

	require.Equal(t, int64(123), embedding.NodeID)
	require.Equal(t, "test-vector-123", embedding.VectorID)
	require.Equal(t, []float32{0.1, 0.2, 0.3, 0.4}, embedding.Vector)
	require.Equal(t, "test_value", embedding.Metadata["test_key"])
}

func TestSearchResult(t *testing.T) {
	t.Parallel()

	result := &SearchResult{
		NodeID:   456,
		VectorID: "result-vector-456",
		Score:    0.95,
		Metadata: map[string]interface{}{
			"similarity": "high",
		},
	}

	require.Equal(t, int64(456), result.NodeID)
	require.Equal(t, "result-vector-456", result.VectorID)
	require.Equal(t, float32(0.95), result.Score)
	require.Equal(t, "high", result.Metadata["similarity"])
}
