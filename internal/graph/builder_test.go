package graph

import (
	"context"
	"database/sql"
	"testing"

	"github.com/charmbracelet/crush/internal/ast"
	"github.com/charmbracelet/crush/internal/complexity"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/git"
	"github.com/charmbracelet/crush/internal/linking"
	"github.com/charmbracelet/crush/internal/pubsub"
	"github.com/stretchr/testify/require"
)

func TestBuilder_New(t *testing.T) {
	t.Parallel()

	mockDB := &MockQuerier{}
	broker := pubsub.NewBroker[map[string]interface{}]()
	defer broker.Shutdown()

	cfg := &BuilderConfig{
		SessionID: "test-session",
		MaxNodes:  1000,
		BatchSize: 50,
	}

	builder := New(cfg, mockDB, broker)

	require.NotNil(t, builder)
	require.Equal(t, cfg, builder.config)
}

// MockQuerier implements minimal db.Querier interface for testing
type MockQuerier struct{}

func (m *MockQuerier) CreateCodeNode(ctx context.Context, arg db.CreateCodeNodeParams) (db.CodeNode, error) {
	return db.CodeNode{ID: 1}, nil
}

func (m *MockQuerier) CreateDependency(ctx context.Context, arg db.CreateDependencyParams) (db.Dependency, error) {
	return db.Dependency{ID: 1}, nil
}

func (m *MockQuerier) ListCodeNodesByPath(ctx context.Context, path string) ([]db.CodeNode, error) {
	return []db.CodeNode{}, nil
}

// Minimal interface compliance stubs
func (m *MockQuerier) AcknowledgeAlert(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) CountDependenciesFrom(ctx context.Context, fromNode int64) (int64, error) { return 0, nil }
func (m *MockQuerier) CountDependenciesTo(ctx context.Context, toNode int64) (int64, error) { return 0, nil }
func (m *MockQuerier) CreateAnalysisMetadata(ctx context.Context, arg db.CreateAnalysisMetadataParams) (db.AnalysisMetadatum, error) { return db.AnalysisMetadatum{}, nil }
func (m *MockQuerier) CreateEmbedding(ctx context.Context, arg db.CreateEmbeddingParams) (db.Embedding, error) { return db.Embedding{}, nil }
func (m *MockQuerier) CreateFile(ctx context.Context, arg db.CreateFileParams) (db.File, error) { return db.File{}, nil }
func (m *MockQuerier) CreateMessage(ctx context.Context, arg db.CreateMessageParams) (db.Message, error) { return db.Message{}, nil }
func (m *MockQuerier) CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error) { return db.Session{}, nil }
func (m *MockQuerier) DeleteAnalysisMetadata(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteCodeNode(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteCodeNodesByPath(ctx context.Context, path string) error { return nil }
func (m *MockQuerier) DeleteCodeNodesBySession(ctx context.Context, sessionID string) error { return nil }
func (m *MockQuerier) DeleteDependenciesByNode(ctx context.Context, arg db.DeleteDependenciesByNodeParams) error { return nil }
func (m *MockQuerier) DeleteDependency(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteEmbedding(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteEmbeddingByNode(ctx context.Context, nodeID int64) error { return nil }
func (m *MockQuerier) DeleteEmbeddingsBySession(ctx context.Context, sessionID sql.NullString) error { return nil }
func (m *MockQuerier) DeleteFile(ctx context.Context, id int64) error { return nil }
func (m *MockQuerier) DeleteMessage(ctx context.Context, id string) error { return nil }
func (m *MockQuerier) DeleteSession(ctx context.Context, id string) error { return nil }
func (m *MockQuerier) GetAnalysisMetadata(ctx context.Context, id int64) (db.AnalysisMetadatum, error) { return db.AnalysisMetadatum{}, nil }
func (m *MockQuerier) GetCodeNode(ctx context.Context, id int64) (db.CodeNode, error) { return db.CodeNode{}, nil }
func (m *MockQuerier) GetDependency(ctx context.Context, arg db.GetDependencyParams) (db.Dependency, error) { return db.Dependency{}, nil }
func (m *MockQuerier) GetEmbedding(ctx context.Context, id int64) (db.Embedding, error) { return db.Embedding{}, nil }
func (m *MockQuerier) GetEmbeddingByNode(ctx context.Context, nodeID int64) (db.Embedding, error) { return db.Embedding{}, nil }
func (m *MockQuerier) GetEmbeddingByVectorID(ctx context.Context, vectorID sql.NullString) (db.Embedding, error) { return db.Embedding{}, nil }
func (m *MockQuerier) GetEmbeddingsByNode(ctx context.Context, nodeID int64) ([]db.Embedding, error) { return []db.Embedding{}, nil }
func (m *MockQuerier) GetFile(ctx context.Context, id int64) (db.File, error) { return db.File{}, nil }
func (m *MockQuerier) GetFileByPathAndSession(ctx context.Context, arg db.GetFileByPathAndSessionParams) (db.File, error) { return db.File{}, nil }
func (m *MockQuerier) GetLatestAnalysisByNode(ctx context.Context, nodeID sql.NullInt64) (db.AnalysisMetadatum, error) { return db.AnalysisMetadatum{}, nil }
func (m *MockQuerier) GetMessage(ctx context.Context, id string) (db.Message, error) { return db.Message{}, nil }
func (m *MockQuerier) GetSessionByID(ctx context.Context, id string) (db.Session, error) { return db.Session{}, nil }
func (m *MockQuerier) ListAllCodeNodes(ctx context.Context) ([]db.CodeNode, error) { return []db.CodeNode{}, nil }
func (m *MockQuerier) ListAllDependencies(ctx context.Context) ([]db.ListAllDependenciesRow, error) { return []db.ListAllDependenciesRow{}, nil }
func (m *MockQuerier) ListAnalysisByNode(ctx context.Context, nodeID sql.NullInt64) ([]db.AnalysisMetadatum, error) { return []db.AnalysisMetadatum{}, nil }
func (m *MockQuerier) ListAnalysisBySession(ctx context.Context, sessionID sql.NullString) ([]db.AnalysisMetadatum, error) { return []db.AnalysisMetadatum{}, nil }
func (m *MockQuerier) ListCodeNodesBySession(ctx context.Context, sessionID string) ([]db.CodeNode, error) { return []db.CodeNode{}, nil }
func (m *MockQuerier) ListDependenciesByRelation(ctx context.Context, relation sql.NullString) ([]db.Dependency, error) { return []db.Dependency{}, nil }
func (m *MockQuerier) ListDependenciesFrom(ctx context.Context, fromNode int64) ([]db.Dependency, error) { return []db.Dependency{}, nil }
func (m *MockQuerier) ListDependenciesTo(ctx context.Context, toNode int64) ([]db.Dependency, error) { return []db.Dependency{}, nil }
func (m *MockQuerier) ListFilesByPath(ctx context.Context, path string) ([]db.File, error) { return []db.File{}, nil }
func (m *MockQuerier) ListFilesBySession(ctx context.Context, sessionID string) ([]db.File, error) { return []db.File{}, nil }
func (m *MockQuerier) ListMessagesByConversation(ctx context.Context, conversationID string) ([]db.Message, error) { return []db.Message{}, nil }
func (m *MockQuerier) ListMessagesBySession(ctx context.Context, sessionID string) ([]db.Message, error) { return []db.Message{}, nil }
func (m *MockQuerier) ListSessionsByPath(ctx context.Context, path string) ([]db.Session, error) { return []db.Session{}, nil }
func (m *MockQuerier) UpdateCodeNode(ctx context.Context, arg db.UpdateCodeNodeParams) (db.CodeNode, error) { return db.CodeNode{}, nil }
func (m *MockQuerier) UpdateEmbedding(ctx context.Context, arg db.UpdateEmbeddingParams) (db.Embedding, error) { return db.Embedding{}, nil }
func (m *MockQuerier) UpdateFile(ctx context.Context, arg db.UpdateFileParams) (db.File, error) { return db.File{}, nil }
func (m *MockQuerier) UpdateMessage(ctx context.Context, arg db.UpdateMessageParams) (db.Message, error) { return db.Message{}, nil }
func (m *MockQuerier) UpdateSession(ctx context.Context, arg db.UpdateSessionParams) (db.Session, error) { return db.Session{}, nil }