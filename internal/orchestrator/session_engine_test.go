package orchestrator

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockQuerier implements db.Querier for testing
type MockQuerier struct {
	mock.Mock
}

// Only implement the methods we actually use in tests
func (m *MockQuerier) ListCodeNodesBySession(ctx context.Context, sessionID string) ([]db.CodeNode, error) {
	args := m.Called(ctx, sessionID)
	return args.Get(0).([]db.CodeNode), args.Error(1)
}

// Implement all other methods as no-ops to satisfy the interface
func (m *MockQuerier) CreateAnalysisMetadata(ctx context.Context, arg db.CreateAnalysisMetadataParams) (db.AnalysisMetadatum, error) {
	return db.AnalysisMetadatum{}, nil
}
func (m *MockQuerier) CreateCodeNode(ctx context.Context, arg db.CreateCodeNodeParams) (db.CodeNode, error) {
	return db.CodeNode{}, nil
}
func (m *MockQuerier) CreateDependency(ctx context.Context, arg db.CreateDependencyParams) (db.Dependency, error) {
	return db.Dependency{}, nil
}
func (m *MockQuerier) CreateEmbedding(ctx context.Context, arg db.CreateEmbeddingParams) (db.Embedding, error) {
	return db.Embedding{}, nil
}
func (m *MockQuerier) CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error) {
	return db.File{}, nil
}
func (m *MockQuerier) CreateMessage(ctx context.Context, arg db.CreateMessageParams) (db.Message, error) {
	return db.Message{}, nil
}
func (m *MockQuerier) CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error) {
	return db.Session{}, nil
}
func (m *MockQuerier) DeleteCodeNode(ctx context.Context, id int64) error                { return nil }
func (m *MockQuerier) DeleteDependency(ctx context.Context, id int64) error              { return nil }
func (m *MockQuerier) DeleteEmbedding(ctx context.Context, id int64) error               { return nil }
func (m *MockQuerier) DeleteFile(ctx context.Context, id string) error                   { return nil }
func (m *MockQuerier) DeleteMessage(ctx context.Context, id string) error                { return nil }
func (m *MockQuerier) DeleteSession(ctx context.Context, id string) error                { return nil }
func (m *MockQuerier) DeleteSessionFiles(ctx context.Context, sessionID string) error    { return nil }
func (m *MockQuerier) DeleteSessionMessages(ctx context.Context, sessionID string) error { return nil }
func (m *MockQuerier) GetCodeNode(ctx context.Context, id int64) (db.CodeNode, error) {
	return db.CodeNode{}, nil
}
func (m *MockQuerier) GetEmbeddingByVectorID(ctx context.Context, vectorID sql.NullString) (db.Embedding, error) {
	return db.Embedding{}, nil
}
func (m *MockQuerier) GetEmbeddingsByNode(ctx context.Context, nodeID int64) ([]db.Embedding, error) {
	return []db.Embedding{}, nil
}
func (m *MockQuerier) GetFile(ctx context.Context, id string) (db.File, error) { return db.File{}, nil }
func (m *MockQuerier) GetFileByPathAndSession(ctx context.Context, arg db.GetFileByPathAndSessionParams) (db.File, error) {
	return db.File{}, nil
}
func (m *MockQuerier) GetMessage(ctx context.Context, id string) (db.Message, error) {
	return db.Message{}, nil
}
func (m *MockQuerier) GetSessionByID(ctx context.Context, id string) (db.Session, error) {
	return db.Session{}, nil
}
func (m *MockQuerier) ListAnalysisByNode(ctx context.Context, nodeID sql.NullInt64) ([]db.AnalysisMetadatum, error) {
	return []db.AnalysisMetadatum{}, nil
}
func (m *MockQuerier) ListAnalysisBySession(ctx context.Context, sessionID sql.NullString) ([]db.AnalysisMetadatum, error) {
	return []db.AnalysisMetadatum{}, nil
}
func (m *MockQuerier) ListCodeNodesByPath(ctx context.Context, path string) ([]db.CodeNode, error) {
	return []db.CodeNode{}, nil
}
func (m *MockQuerier) ListDependenciesFrom(ctx context.Context, fromNode int64) ([]db.Dependency, error) {
	return []db.Dependency{}, nil
}
func (m *MockQuerier) ListDependenciesTo(ctx context.Context, toNode int64) ([]db.Dependency, error) {
	return []db.Dependency{}, nil
}
func (m *MockQuerier) ListFilesByPath(ctx context.Context, path string) ([]db.File, error) {
	return []db.File{}, nil
}
func (m *MockQuerier) ListFilesBySession(ctx context.Context, sessionID string) ([]db.File, error) {
	return []db.File{}, nil
}
func (m *MockQuerier) ListLatestSessionFiles(ctx context.Context, sessionID string) ([]db.File, error) {
	return []db.File{}, nil
}
func (m *MockQuerier) ListMessagesBySession(ctx context.Context, sessionID string) ([]db.Message, error) {
	return []db.Message{}, nil
}
func (m *MockQuerier) ListNewFiles(ctx context.Context) ([]db.File, error) { return []db.File{}, nil }
func (m *MockQuerier) ListSessions(ctx context.Context) ([]db.Session, error) {
	return []db.Session{}, nil
}
func (m *MockQuerier) UpdateAnalysisStatus(ctx context.Context, arg db.UpdateAnalysisStatusParams) (db.AnalysisMetadatum, error) {
	return db.AnalysisMetadatum{}, nil
}
func (m *MockQuerier) UpdateCodeNode(ctx context.Context, arg db.UpdateCodeNodeParams) (db.CodeNode, error) {
	return db.CodeNode{}, nil
}
func (m *MockQuerier) UpdateMessage(ctx context.Context, arg db.UpdateMessageParams) error {
	return nil
}
func (m *MockQuerier) UpdateSession(ctx context.Context, arg db.UpdateSessionParams) (db.Session, error) {
	return db.Session{}, nil
}

func TestNewSessionContextEngine(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  4,
		MemoryBudgetMB: 256,
	}

	engine := NewSessionContextEngine("test-session", mockDB, config)

	require.NotNil(t, engine)
	assert.Equal(t, "test-session", engine.sessionID)
	assert.Equal(t, mockDB, engine.db)
	assert.Equal(t, config, engine.config)
	assert.NotNil(t, engine.eventBus)
	assert.NotNil(t, engine.ctx)
	assert.NotNil(t, engine.cancel)
}

func TestSessionContextEngine_StartStop(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{
		Mode:           config.ContextEngineModePerformance,
		MaxGoroutines:  2,
		MemoryBudgetMB: 128,
	}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	// Test start
	engine.Start()

	// Wait a bit for goroutine to start
	time.Sleep(50 * time.Millisecond)

	// Test stop
	engine.Stop()

	// Verify context is cancelled
	select {
	case <-engine.ctx.Done():
		// Expected - context should be cancelled
	default:
		t.Fatal("Context should be cancelled after Stop()")
	}
}

func TestSessionContextEngine_RequestAnalysisLock(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	files := []string{"test1.go", "test2.go"}
	commitHash := "abc123"

	// Test first lock request
	lock1, err := engine.RequestAnalysisLock(files, commitHash)
	require.NoError(t, err)
	require.NotNil(t, lock1)

	assert.Equal(t, files, lock1.Files)
	assert.Equal(t, commitHash, lock1.CommitHash)
	assert.Equal(t, "test-session", lock1.LockedBy)
	assert.WithinDuration(t, time.Now(), lock1.LockedAt, time.Second)

	// Test second lock request for same files/commit (should return existing lock)
	lock2, err := engine.RequestAnalysisLock(files, commitHash)
	require.NoError(t, err)
	require.NotNil(t, lock2)

	// Should be the same lock
	assert.Equal(t, lock1.LockedAt, lock2.LockedAt)
	assert.Equal(t, lock1.LockedBy, lock2.LockedBy)
}

func TestSessionContextEngine_ReleaseLock(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	files := []string{"test.go"}
	commitHash := "def456"

	// Get a lock
	lock, err := engine.RequestAnalysisLock(files, commitHash)
	require.NoError(t, err)
	require.NotNil(t, lock)

	// Verify lock exists
	lockKey := engine.generateLockKey(files, commitHash)
	_, exists := engine.analysisLocks.Load(lockKey)
	assert.True(t, exists, "Lock should exist after creation")

	// Release the lock
	engine.ReleaseLock(lock)

	// Verify lock is removed
	_, exists = engine.analysisLocks.Load(lockKey)
	assert.False(t, exists, "Lock should be removed after release")

	// Small delay to ensure different timestamp
	time.Sleep(1 * time.Millisecond)

	// Try to get lock again - should get a new one
	newLock, err := engine.RequestAnalysisLock(files, commitHash)
	require.NoError(t, err)
	require.NotNil(t, newLock)

	// Should be a different lock (different timestamp)
	assert.True(t, newLock.LockedAt.After(lock.LockedAt), "New lock should have later timestamp")
}

func TestSessionContextEngine_GetSessionNodes(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	// Setup mock expectations
	expectedNodes := []db.CodeNode{
		{
			ID:        1,
			SessionID: "test-session",
			Path:      "test1.go",
			Kind:      sql.NullString{String: "function", Valid: true},
		},
		{
			ID:        2,
			SessionID: "test-session",
			Path:      "test2.go",
			Kind:      sql.NullString{String: "class", Valid: true},
		},
	}

	mockDB.On("ListCodeNodesBySession", mock.Anything, "test-session").Return(expectedNodes, nil)

	// Test GetSessionNodes
	nodes, err := engine.GetSessionNodes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedNodes, nodes)

	// Verify mock expectations
	mockDB.AssertExpectations(t)
}

func TestSessionContextEngine_GetKnowledgeStats(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	// Setup mock expectations
	nodes := []db.CodeNode{
		{
			ID:        1,
			SessionID: "test-session",
			Path:      "test1.go",
			Kind:      sql.NullString{String: "function", Valid: true},
		},
		{
			ID:        2,
			SessionID: "test-session",
			Path:      "test2.go",
			Kind:      sql.NullString{String: "function", Valid: true},
		},
		{
			ID:        3,
			SessionID: "test-session",
			Path:      "test3.go",
			Kind:      sql.NullString{String: "class", Valid: true},
		},
	}

	mockDB.On("ListCodeNodesBySession", mock.Anything, "test-session").Return(nodes, nil)

	// Test GetKnowledgeStats
	stats, err := engine.GetKnowledgeStats(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stats)

	assert.Equal(t, "test-session", stats.SessionID)
	assert.Equal(t, 3, stats.TotalNodes)
	assert.Equal(t, 2, stats.ByType["function"])
	assert.Equal(t, 1, stats.ByType["class"])
	assert.WithinDuration(t, time.Now(), stats.LastUpdated, time.Second)

	// Verify mock expectations
	mockDB.AssertExpectations(t)
}

func TestSessionContextEngine_Subscribe(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test subscription
	eventChan := engine.Subscribe(ctx)
	require.NotNil(t, eventChan)

	// Test that we can receive events
	go func() {
		time.Sleep(10 * time.Millisecond)
		engine.eventBus.Publish(pubsub.EventType("test_event"), ContextEvent{
			Type:      AnalysisStarted,
			SessionID: "test-session",
			FilePath:  "test.go",
		})
	}()

	select {
	case event := <-eventChan:
		assert.Equal(t, pubsub.EventType("test_event"), event.Type)
		contextEvent := event.Payload
		assert.Equal(t, "test-session", contextEvent.SessionID)
		assert.Equal(t, "test.go", contextEvent.FilePath)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Should have received event")
	}
}

func TestSessionContextEngine_PerformMaintenance(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	// Add some locks
	files1 := []string{"old.go"}
	files2 := []string{"new.go"}

	_, err := engine.RequestAnalysisLock(files1, "old-commit")
	require.NoError(t, err)

	_, err = engine.RequestAnalysisLock(files2, "new-commit")
	require.NoError(t, err)

	// Manually set lock1 to be old (more than 1 hour ago)
	lockKey1 := engine.generateLockKey(files1, "old-commit")
	staleLock := &AnalysisLock{
		Files:      files1,
		CommitHash: "old-commit",
		LockedAt:   time.Now().Add(-2 * time.Hour), // 2 hours ago
		LockedBy:   "test-session",
	}
	engine.analysisLocks.Store(lockKey1, staleLock)

	// Perform maintenance
	engine.performMaintenance()

	// Check that stale lock is removed
	_, exists1 := engine.analysisLocks.Load(lockKey1)
	assert.False(t, exists1, "Stale lock should be removed")

	// Check that recent lock is still there
	lockKey2 := engine.generateLockKey(files2, "new-commit")
	_, exists2 := engine.analysisLocks.Load(lockKey2)
	assert.True(t, exists2, "Recent lock should remain")
}

func TestSessionContextEngine_GenerateLockKey(t *testing.T) {
	mockDB := &MockQuerier{}
	config := &config.ContextEngineConfig{Mode: config.ContextEngineModePerformance}

	engine := NewSessionContextEngine("test-session", mockDB, config)
	require.NotNil(t, engine)

	files := []string{"file1.go", "file2.go"}
	commitHash := "abc123"

	key := engine.generateLockKey(files, commitHash)

	expected := "test-session:abc123:file1.go:file2.go"
	assert.Equal(t, expected, key)

	// Test with different order - should produce different key
	files2 := []string{"file2.go", "file1.go"}
	key2 := engine.generateLockKey(files2, commitHash)
	expected2 := "test-session:abc123:file2.go:file1.go"
	assert.Equal(t, expected2, key2)
	assert.NotEqual(t, key, key2)
}
