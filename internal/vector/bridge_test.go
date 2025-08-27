package vector

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
)

// MockVectorClient implements VectorClient for testing.
type MockVectorClient struct {
	mock.Mock
}

func (m *MockVectorClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockVectorClient) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockVectorClient) CollectionExists(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func (m *MockVectorClient) CreateCollection(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockVectorClient) EnsureCollection(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockVectorClient) UpsertEmbedding(ctx context.Context, embedding *CodeEmbedding) error {
	args := m.Called(ctx, embedding)
	return args.Error(0)
}

func (m *MockVectorClient) UpsertEmbeddings(ctx context.Context, embeddings []*CodeEmbedding) error {
	args := m.Called(ctx, embeddings)
	return args.Error(0)
}

func (m *MockVectorClient) DeleteEmbedding(ctx context.Context, vectorID string) error {
	args := m.Called(ctx, vectorID)
	return args.Error(0)
}

func (m *MockVectorClient) DeleteEmbeddingsByNodeID(ctx context.Context, nodeID int64) error {
	args := m.Called(ctx, nodeID)
	return args.Error(0)
}

func (m *MockVectorClient) DeleteEmbeddingsBySessionID(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *MockVectorClient) SearchSimilar(ctx context.Context, vector []float32, limit int, filter map[string]interface{}) ([]*SearchResult, error) {
	args := m.Called(ctx, vector, limit, filter)
	return args.Get(0).([]*SearchResult), args.Error(1)
}

func (m *MockVectorClient) CountEmbeddings(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockVectorClient) CountEmbeddingsBySessionID(ctx context.Context, sessionID string) (int64, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).(int64), args.Error(1)
}

// MockQueries implements QueryInterface for testing.
type MockQueries struct {
	mock.Mock
}

// Ensure MockQueries implements QueryInterface
var _ QueryInterface = (*MockQueries)(nil)

func (m *MockQueries) GetCodeNode(ctx context.Context, id int64) (db.CodeNode, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(db.CodeNode), args.Error(1)
}

func (m *MockQueries) ListCodeNodesBySession(ctx context.Context, sessionID string) ([]db.CodeNode, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).([]db.CodeNode), args.Error(1)
}

func (m *MockQueries) CountCodeNodesBySession(ctx context.Context, sessionID string) (int64, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockQueries) GetEmbeddingByNode(ctx context.Context, nodeID int64) (db.Embedding, error) {
	args := m.Called(ctx, nodeID)
	return args.Get(0).(db.Embedding), args.Error(1)
}

func (m *MockQueries) UpdateEmbedding(ctx context.Context, params db.UpdateEmbeddingParams) (db.Embedding, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(db.Embedding), args.Error(1)
}

func (m *MockQueries) CreateEmbedding(ctx context.Context, params db.CreateEmbeddingParams) (db.Embedding, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(db.Embedding), args.Error(1)
}

func (m *MockQueries) DeleteEmbeddingByNode(ctx context.Context, nodeID int64) error {
	args := m.Called(ctx, nodeID)
	return args.Error(0)
}

func (m *MockQueries) DeleteEmbeddingsBySession(ctx context.Context, sessionID sql.NullString) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func TestNewBridge(t *testing.T) {
	t.Parallel()

	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}

	bridge := NewBridge(mockVector, mockQueries)

	require.NotNil(t, bridge)
	require.Equal(t, mockVector, bridge.vectorClient)
	require.Equal(t, mockQueries, bridge.queries)
}

func TestBridge_Search(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options *HybridSearchOptions
		wantErr bool
	}{
		{
			name:    "nil options",
			options: nil,
			wantErr: true,
		},
		{
			name: "empty vector",
			options: &HybridSearchOptions{
				Vector: []float32{},
			},
			wantErr: true,
		},
		{
			name: "valid search",
			options: &HybridSearchOptions{
				Vector:      []float32{0.1, 0.2, 0.3},
				VectorLimit: 10,
				FinalLimit:  5,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockVector := &MockVectorClient{}
			mockQueries := &MockQueries{}
			bridge := NewBridge(mockVector, mockQueries)

			if !tt.wantErr {
				// Setup mock expectations for successful search
				mockVector.On("SearchSimilar", mock.Anything, tt.options.Vector, tt.options.VectorLimit, mock.Anything).
					Return([]*SearchResult{
						{
							NodeID:   123,
							VectorID: "123",
							Score:    0.95,
							Metadata: map[string]interface{}{"test": "data"},
						},
					}, nil)

				mockQueries.On("GetCodeNode", mock.Anything, int64(123)).
					Return(db.CodeNode{
						ID:        123,
						SessionID: "test-session",
						Path:      "/test/file.go",
						Language:  sql.NullString{String: "go", Valid: true},
						Kind:      sql.NullString{String: "function", Valid: true},
					}, nil)

				mockQueries.On("GetEmbeddingByNode", mock.Anything, int64(123)).
					Return(db.Embedding{
						ID:       1,
						NodeID:   123,
						VectorID: sql.NullString{String: "123", Valid: true},
						Vector:   []byte(`[0.1,0.2,0.3]`),
						Dims:     sql.NullInt64{Int64: 3, Valid: true},
					}, nil)
			}

			results, err := bridge.Search(context.Background(), tt.options)

			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, results)
			} else {
				require.NoError(t, err)
				require.NotNil(t, results)
				require.Len(t, results, 1)
				require.Equal(t, float32(0.95), results[0].Score)
				require.Equal(t, int64(123), results[0].CodeNode.ID)
			}

			mockVector.AssertExpectations(t)
			mockQueries.AssertExpectations(t)
		})
	}
}

func TestBridge_SyncEmbedding(t *testing.T) {
	t.Parallel()

	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	bridge := NewBridge(mockVector, mockQueries)

	nodeID := int64(123)
	vector := []float32{0.1, 0.2, 0.3}
	metadata := &EmbeddingMetadata{
		NodeID:    nodeID,
		SessionID: "test-session",
		Path:      "/test/file.go",
		Language:  "go",
		Kind:      "function",
	}

	// Setup mock expectations
	mockVector.On("UpsertEmbedding", mock.Anything, mock.MatchedBy(func(embedding *CodeEmbedding) bool {
		return embedding.NodeID == nodeID && len(embedding.Vector) == 3
	})).Return(nil)

	// Mock update failing (embedding doesn't exist)
	mockQueries.On("UpdateEmbedding", mock.Anything, mock.AnythingOfType("db.UpdateEmbeddingParams")).
		Return(db.Embedding{}, sql.ErrNoRows)

	// Mock create succeeding
	mockQueries.On("CreateEmbedding", mock.Anything, mock.MatchedBy(func(params db.CreateEmbeddingParams) bool {
		return params.NodeID == nodeID
	})).Return(db.Embedding{
		ID:     1,
		NodeID: nodeID,
	}, nil)

	err := bridge.SyncEmbedding(context.Background(), nodeID, vector, metadata)

	require.NoError(t, err)
	mockVector.AssertExpectations(t)
	mockQueries.AssertExpectations(t)
}

func TestBridge_DeleteEmbedding(t *testing.T) {
	t.Parallel()

	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	bridge := NewBridge(mockVector, mockQueries)

	nodeID := int64(123)

	// Setup mock expectations
	mockVector.On("DeleteEmbeddingsByNodeID", mock.Anything, nodeID).Return(nil)
	mockQueries.On("DeleteEmbeddingByNode", mock.Anything, nodeID).Return(nil)

	err := bridge.DeleteEmbedding(context.Background(), nodeID)

	require.NoError(t, err)
	mockVector.AssertExpectations(t)
	mockQueries.AssertExpectations(t)
}

func TestBridge_DeleteEmbeddingsBySession(t *testing.T) {
	t.Parallel()

	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	bridge := NewBridge(mockVector, mockQueries)

	sessionID := "test-session"

	// Setup mock expectations
	mockVector.On("DeleteEmbeddingsBySessionID", mock.Anything, sessionID).Return(nil)
	mockQueries.On("DeleteEmbeddingsBySession", mock.Anything, mock.MatchedBy(func(nullString sql.NullString) bool {
		return nullString.Valid && nullString.String == sessionID
	})).Return(nil)

	err := bridge.DeleteEmbeddingsBySession(context.Background(), sessionID)

	require.NoError(t, err)
	mockVector.AssertExpectations(t)
	mockQueries.AssertExpectations(t)
}

func TestBridge_matchesFilters(t *testing.T) {
	t.Parallel()

	bridge := &Bridge{}

	node := &db.CodeNode{
		ID:        123,
		SessionID: "test-session",
		Language:  sql.NullString{String: "go", Valid: true},
		Kind:      sql.NullString{String: "function", Valid: true},
		Metadata:  map[string]interface{}{"complexity": float64(5)},
	}

	tests := []struct {
		name    string
		options *HybridSearchOptions
		want    bool
	}{
		{
			name:    "no filters",
			options: &HybridSearchOptions{},
			want:    true,
		},
		{
			name: "matching session",
			options: &HybridSearchOptions{
				SessionID: &[]string{"test-session"}[0],
			},
			want: true,
		},
		{
			name: "non-matching session",
			options: &HybridSearchOptions{
				SessionID: &[]string{"other-session"}[0],
			},
			want: false,
		},
		{
			name: "matching language",
			options: &HybridSearchOptions{
				Languages: []string{"go", "python"},
			},
			want: true,
		},
		{
			name: "non-matching language",
			options: &HybridSearchOptions{
				Languages: []string{"python", "javascript"},
			},
			want: false,
		},
		{
			name: "complexity in range",
			options: &HybridSearchOptions{
				MinComplexity: &[]int{3}[0],
				MaxComplexity: &[]int{10}[0],
			},
			want: true,
		},
		{
			name: "complexity below range",
			options: &HybridSearchOptions{
				MinComplexity: &[]int{10}[0],
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := bridge.matchesFilters(node, tt.options)
			require.Equal(t, tt.want, result)
		})
	}
}

func TestBridge_extractComplexityFromMetadata(t *testing.T) {
	t.Parallel()

	bridge := &Bridge{}

	tests := []struct {
		name     string
		metadata interface{}
		want     int
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			want:     0,
		},
		{
			name:     "valid complexity",
			metadata: map[string]interface{}{"complexity": float64(5)},
			want:     5,
		},
		{
			name:     "missing complexity",
			metadata: map[string]interface{}{"other": "value"},
			want:     0,
		},
		{
			name:     "invalid complexity type",
			metadata: map[string]interface{}{"complexity": "invalid"},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := bridge.extractComplexityFromMetadata(tt.metadata)
			require.Equal(t, tt.want, result)
		})
	}
}
