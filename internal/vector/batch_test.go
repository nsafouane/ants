package vector

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/crush/internal/db"
)

// MockContentProvider implements ContentProvider for testing.
type MockContentProvider struct {
	mock.Mock
}

func (m *MockContentProvider) GetCodeContent(ctx context.Context, node *db.CodeNode) (string, error) {
	args := m.Called(ctx, node)
	return args.String(0), args.Error(1)
}

func (m *MockContentProvider) GetCodeContents(ctx context.Context, nodes []*db.CodeNode) ([]string, error) {
	args := m.Called(ctx, nodes)
	return args.Get(0).([]string), args.Error(1)
}

// MockEmbeddingService implements EmbeddingGenerator for testing.
type MockEmbeddingService struct {
	mock.Mock
}

func (m *MockEmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	args := m.Called(ctx, text)
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockEmbeddingService) GenerateEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	args := m.Called(ctx, texts)
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *MockEmbeddingService) GenerateCodeEmbedding(ctx context.Context, codeContent string, metadata *EmbeddingMetadata) ([]float32, error) {
	args := m.Called(ctx, codeContent, metadata)
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockEmbeddingService) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockEmbeddingService) GetModel() string {
	args := m.Called()
	return args.String(0)
}

func TestDefaultBatchProcessingOptions(t *testing.T) {
	t.Parallel()

	options := DefaultBatchProcessingOptions()

	require.NotNil(t, options)
	assert.Equal(t, 8, options.MaxConcurrency)
	assert.Equal(t, 50, options.BatchSize)
	assert.Equal(t, 3, options.RetryAttempts)
	assert.Equal(t, 2*time.Second, options.RetryDelay)
	assert.True(t, options.SkipExisting)
	assert.False(t, options.OverwriteExisting)
}

func TestNewBatchProcessor(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)

	t.Run("with options", func(t *testing.T) {
		t.Parallel()

		options := &BatchProcessingOptions{
			MaxConcurrency: 16,
			BatchSize:      100,
			RetryAttempts:  5,
			RetryDelay:     5 * time.Second,
		}

		processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, options)

		require.NotNil(t, processor)
		assert.Equal(t, mockEmbedding, processor.embeddingService)
		assert.Equal(t, mockBridge, processor.vectorBridge)
		assert.Equal(t, mockQueries, processor.queries)
		assert.Equal(t, 16, processor.maxConcurrency)
		assert.Equal(t, 100, processor.batchSize)
		assert.Equal(t, 5, processor.retryAttempts)
		assert.Equal(t, 5*time.Second, processor.retryDelay)
		assert.NotNil(t, processor.activeSessions)
	})

	t.Run("with nil options uses defaults", func(t *testing.T) {
		t.Parallel()

		processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, nil)

		require.NotNil(t, processor)
		assert.Equal(t, 8, processor.maxConcurrency)
		assert.Equal(t, 50, processor.batchSize)
		assert.Equal(t, 3, processor.retryAttempts)
		assert.Equal(t, 2*time.Second, processor.retryDelay)
	})
}

func TestBatchProcessor_ProcessSessionBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sessionID      string
		nodes          []db.CodeNode
		setupMocks     func(*MockEmbeddingService, *MockVectorClient, *MockQueries)
		expectedError  bool
		expectedStatus BatchStatus
	}{
		{
			name:          "empty session ID",
			sessionID:     "",
			expectedError: true,
		},
		{
			name:      "no nodes found",
			sessionID: "test-session",
			nodes:     []db.CodeNode{},
			setupMocks: func(embedSvc *MockEmbeddingService, vectorClient *MockVectorClient, queries *MockQueries) {
				queries.On("ListCodeNodesBySession", mock.Anything, "test-session").Return([]db.CodeNode{}, nil)
			},
			expectedError:  false,
			expectedStatus: BatchStatusCompleted,
		},
		{
			name:      "successful processing",
			sessionID: "test-session",
			nodes: []db.CodeNode{
				{
					ID:        1,
					SessionID: "test-session",
					Path:      "/test/file1.go",
					Language:  sql.NullString{String: "go", Valid: true},
					Kind:      sql.NullString{String: "function", Valid: true},
					Symbol:    sql.NullString{String: "TestFunc1", Valid: true},
				},
				{
					ID:        2,
					SessionID: "test-session",
					Path:      "/test/file2.go",
					Language:  sql.NullString{String: "go", Valid: true},
					Kind:      sql.NullString{String: "function", Valid: true},
					Symbol:    sql.NullString{String: "TestFunc2", Valid: true},
				},
			},
			setupMocks: func(embedSvc *MockEmbeddingService, vectorClient *MockVectorClient, queries *MockQueries) {
				// Mock ListCodeNodesBySession
				queries.On("ListCodeNodesBySession", mock.Anything, "test-session").Return([]db.CodeNode{
					{
						ID:        1,
						SessionID: "test-session",
						Path:      "/test/file1.go",
						Language:  sql.NullString{String: "go", Valid: true},
						Kind:      sql.NullString{String: "function", Valid: true},
						Symbol:    sql.NullString{String: "TestFunc1", Valid: true},
					},
					{
						ID:        2,
						SessionID: "test-session",
						Path:      "/test/file2.go",
						Language:  sql.NullString{String: "go", Valid: true},
						Kind:      sql.NullString{String: "function", Valid: true},
						Symbol:    sql.NullString{String: "TestFunc2", Valid: true},
					},
				}, nil)

				// Mock embedding generation
				embedSvc.On("GenerateCodeEmbedding", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("*vector.EmbeddingMetadata")).Return([]float32{0.1, 0.2, 0.3}, nil)

				// Mock vector client operations
				vectorClient.On("UpsertEmbedding", mock.Anything, mock.AnythingOfType("*vector.CodeEmbedding")).Return(nil)

				// Mock database operations
				queries.On("UpdateEmbedding", mock.Anything, mock.AnythingOfType("db.UpdateEmbeddingParams")).Return(db.Embedding{}, sql.ErrNoRows)
				queries.On("CreateEmbedding", mock.Anything, mock.AnythingOfType("db.CreateEmbeddingParams")).Return(db.Embedding{ID: 1}, nil)
			},
			expectedError:  false,
			expectedStatus: BatchStatusCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup mocks
			mockEmbedding := &MockEmbeddingService{}
			mockVector := &MockVectorClient{}
			mockQueries := &MockQueries{}

			if tt.setupMocks != nil {
				tt.setupMocks(mockEmbedding, mockVector, mockQueries)
			}

			mockBridge := NewBridge(mockVector, mockQueries)
			processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, DefaultBatchProcessingOptions())

			// Execute
			progress, err := processor.ProcessSessionBatch(context.Background(), tt.sessionID, nil)

			// Verify
			if tt.expectedError {
				require.Error(t, err)
				assert.Nil(t, progress)
			} else {
				require.NoError(t, err)
				require.NotNil(t, progress)
				assert.Equal(t, tt.expectedStatus, progress.Status)
				assert.Equal(t, tt.sessionID, progress.SessionID)
			}

			// Verify mocks
			mockEmbedding.AssertExpectations(t)
			mockVector.AssertExpectations(t)
			mockQueries.AssertExpectations(t)
		})
	}
}

func TestBatchProcessor_FilterExistingEmbeddings(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, nil)

	nodes := []db.CodeNode{
		{ID: 1, SessionID: "test", Path: "/test1.go"},
		{ID: 2, SessionID: "test", Path: "/test2.go"},
		{ID: 3, SessionID: "test", Path: "/test3.go"},
	}

	// Mock: node 1 has embedding, nodes 2 and 3 don't
	mockQueries.On("GetEmbeddingByNode", mock.Anything, int64(1)).Return(db.Embedding{ID: 1}, nil)
	mockQueries.On("GetEmbeddingByNode", mock.Anything, int64(2)).Return(db.Embedding{}, sql.ErrNoRows)
	mockQueries.On("GetEmbeddingByNode", mock.Anything, int64(3)).Return(db.Embedding{}, sql.ErrNoRows)

	filtered, err := processor.filterExistingEmbeddings(context.Background(), nodes)

	require.NoError(t, err)
	require.Len(t, filtered, 2)
	assert.Equal(t, int64(2), filtered[0].ID)
	assert.Equal(t, int64(3), filtered[1].ID)

	mockQueries.AssertExpectations(t)
}

func TestBatchProcessor_GetBatchProgress(t *testing.T) {
	t.Parallel()

	processor := &BatchProcessor{}
	startTime := time.Now().Add(-30 * time.Second)

	sessionBatch := &SessionBatch{
		SessionID:      "test-session",
		TotalNodes:     100,
		ProcessedNodes: 70,
		FailedNodes:    10,
		StartTime:      startTime,
		Status:         BatchStatusRunning,
	}

	progress := processor.getBatchProgress(sessionBatch)

	require.NotNil(t, progress)
	assert.Equal(t, "test-session", progress.SessionID)
	assert.Equal(t, 100, progress.TotalNodes)
	assert.Equal(t, 70, progress.ProcessedNodes)
	assert.Equal(t, 10, progress.FailedNodes)
	assert.InDelta(t, 87.5, progress.SuccessRate, 0.1) // 70/(70+10) * 100
	assert.True(t, progress.ElapsedTime > 25*time.Second)
	assert.True(t, progress.NodesPerSecond > 0)
	assert.Equal(t, BatchStatusRunning, progress.Status)
}

func TestBatchProcessor_SessionManagement(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, nil)

	// Test with no active sessions
	progress, err := processor.GetSessionProgress("non-existent")
	assert.Error(t, err)
	assert.Nil(t, progress)

	// Test cancel non-existent session
	err = processor.CancelSession("non-existent")
	assert.Error(t, err)

	// Test list active sessions (empty)
	activeSessions := processor.ListActiveSessions()
	assert.Empty(t, activeSessions)

	// TODO: Add tests for active session management
	// This would require running actual batch processing which is complex for unit tests
}

func TestBatchStatus_Values(t *testing.T) {
	t.Parallel()

	// Verify all status constants are properly defined
	assert.Equal(t, BatchStatus("pending"), BatchStatusPending)
	assert.Equal(t, BatchStatus("running"), BatchStatusRunning)
	assert.Equal(t, BatchStatus("paused"), BatchStatusPaused)
	assert.Equal(t, BatchStatus("completed"), BatchStatusCompleted)
	assert.Equal(t, BatchStatus("failed"), BatchStatusFailed)
	assert.Equal(t, BatchStatus("cancelled"), BatchStatusCancelled)
}

func TestBatchProcessor_BuildMetadataFromCodeNode(t *testing.T) {
	t.Parallel()

	processor := &BatchProcessor{}

	node := &db.CodeNode{
		ID:        123,
		SessionID: "test-session",
		Path:      "/test/file.go",
		Language:  sql.NullString{String: "go", Valid: true},
		Kind:      sql.NullString{String: "function", Valid: true},
		Symbol:    sql.NullString{String: "TestFunction", Valid: true},
		StartLine: sql.NullInt64{Int64: 10, Valid: true},
		EndLine:   sql.NullInt64{Int64: 20, Valid: true},
	}

	metadata := processor.buildMetadataFromCodeNode(node)

	require.NotNil(t, metadata)
	assert.Equal(t, int64(123), metadata.NodeID)
	assert.Equal(t, "test-session", metadata.SessionID)
	assert.Equal(t, "/test/file.go", metadata.Path)
	assert.Equal(t, "go", metadata.Language)
	assert.Equal(t, "function", metadata.Kind)
	assert.Equal(t, "TestFunction", metadata.Symbol)
	assert.Equal(t, 10, metadata.StartLine)
	assert.Equal(t, 20, metadata.EndLine)
	assert.Equal(t, 11, metadata.LineCount) // 20 - 10 + 1
}

func TestBatchProcessor_GetCodeContent(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, nil)

	node := &db.CodeNode{
		ID:       123,
		Path:     "/test/file.go",
		Language: sql.NullString{String: "go", Valid: true},
		Kind:     sql.NullString{String: "function", Valid: true},
	}

	t.Run("with content provider", func(t *testing.T) {
		t.Parallel()

		mockContentProvider := &MockContentProvider{}
		mockContentProvider.On("GetCodeContent", mock.Anything, node).Return("func TestCode() {}", nil)

		options := &BatchProcessingOptions{
			ContentProvider: mockContentProvider,
		}

		content, err := processor.getCodeContent(context.Background(), node, options)

		require.NoError(t, err)
		assert.Equal(t, "func TestCode() {}", content)
		mockContentProvider.AssertExpectations(t)
	})

	t.Run("without content provider uses fallback", func(t *testing.T) {
		t.Parallel()

		content, err := processor.getCodeContent(context.Background(), node, nil)

		require.NoError(t, err)
		assert.Contains(t, content, "// Code for node 123")
		assert.Contains(t, content, "/test/file.go")
		assert.Contains(t, content, "Language: go")
		assert.Contains(t, content, "Kind: function")
	})
}

func TestBatchProcessor_HealthCheck(t *testing.T) {
	t.Parallel()

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)
	processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, nil)

	t.Run("healthy", func(t *testing.T) {
		t.Parallel()

		mockEmbedding.On("HealthCheck", mock.Anything).Return(nil)
		mockVector.On("Ping", mock.Anything).Return(nil)

		err := processor.HealthCheck(context.Background())

		require.NoError(t, err)
		mockEmbedding.AssertExpectations(t)
		mockVector.AssertExpectations(t)
	})

	t.Run("embedding service unhealthy", func(t *testing.T) {
		t.Parallel()

		mockEmbedding.On("HealthCheck", mock.Anything).Return(fmt.Errorf("service down"))

		err := processor.HealthCheck(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "embedding service health check failed")
		mockEmbedding.AssertExpectations(t)
	})

	t.Run("vector client unhealthy", func(t *testing.T) {
		t.Parallel()

		mockEmbedding.On("HealthCheck", mock.Anything).Return(nil)
		mockVector.On("Ping", mock.Anything).Return(fmt.Errorf("connection failed"))

		err := processor.HealthCheck(context.Background())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "vector client health check failed")
		mockEmbedding.AssertExpectations(t)
		mockVector.AssertExpectations(t)
	})
}

// BenchmarkBatchProcessing benchmarks batch processing performance.
func BenchmarkBatchProcessing(b *testing.B) {
	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)

	// Setup mocks
	mockEmbedding.On("GenerateCodeEmbedding", mock.Anything, mock.Anything, mock.Anything).Return([]float32{0.1, 0.2, 0.3}, nil)
	mockVector.On("UpsertEmbedding", mock.Anything, mock.Anything).Return(nil)
	mockQueries.On("UpdateEmbedding", mock.Anything, mock.Anything).Return(db.Embedding{}, sql.ErrNoRows)
	mockQueries.On("CreateEmbedding", mock.Anything, mock.Anything).Return(db.Embedding{ID: 1}, nil)

	processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, &BatchProcessingOptions{
		MaxConcurrency: 4,
		BatchSize:      20,
		RetryAttempts:  1,
	})

	// Create test node
	node := &db.CodeNode{
		ID:        1,
		SessionID: "bench-session",
		Path:      "/bench/file.go",
		Language:  sql.NullString{String: "go", Valid: true},
		Kind:      sql.NullString{String: "function", Valid: true},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := processor.processNodeWithRetries(context.Background(), node, nil)
		if err != nil {
			b.Fatalf("Processing failed: %v", err)
		}
	}
}

// TestBatchProcessor_ConcurrencyLimits tests that concurrency limits are respected.
func TestBatchProcessor_ConcurrencyLimits(t *testing.T) {
	t.Parallel()

	// This test would require more complex setup to verify actual concurrency
	// For now, we verify the configuration is set correctly
	options := &BatchProcessingOptions{
		MaxConcurrency: 16,
		BatchSize:      100,
	}

	mockEmbedding := &MockEmbeddingService{}
	mockVector := &MockVectorClient{}
	mockQueries := &MockQueries{}
	mockBridge := NewBridge(mockVector, mockQueries)

	processor := NewBatchProcessor(mockEmbedding, mockBridge, mockQueries, options)

	assert.Equal(t, 16, processor.maxConcurrency)
	assert.Equal(t, 100, processor.batchSize)
}
