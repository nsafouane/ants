package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*Queries, func()) {
	t.Helper()
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "crush-db-test-")
	require.NoError(t, err)

	dataDir := filepath.Join(tmpDir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))

	db, err := Connect(ctx, dataDir)
	require.NoError(t, err)

	q := New(db)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
	return q, cleanup
}

func TestCreateAndGetCodeNode(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	created, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "session-1",
		Path:      "/tmp/example.go",
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	fetched, err := q.GetCodeNode(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Path, fetched.Path)
}

func TestCreateAndGetEmbedding(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	node, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "s-vec",
		Path:      "/tmp/vec.go",
	})
	require.NoError(t, err)

	emb, err := q.CreateEmbedding(ctx, CreateEmbeddingParams{
		NodeID:    node.ID,
		SessionID: sql.NullString{String: "s-vec", Valid: true},
		VectorID:  sql.NullString{String: "vec-1", Valid: true},
		Vector:    []byte{1, 2},
		Dims:      sql.NullInt64{},
		Metadata:  "{}",
	})
	require.NoError(t, err)
	require.NotZero(t, emb.ID)

	fetched, err := q.GetEmbeddingByVectorID(ctx, sqlNullString("vec-1"))
	require.NoError(t, err)
	require.Equal(t, emb.ID, fetched.ID)
}

func TestCreateAndListDependencies(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	n1, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{Path: "/a.go"})
	require.NoError(t, err)
	n2, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{Path: "/b.go"})
	require.NoError(t, err)

	dep, err := q.CreateDependency(ctx, CreateDependencyParams{FromNode: n1.ID, ToNode: n2.ID, Relation: sql.NullString{String: "import", Valid: true}})
	require.NoError(t, err)
	require.NotZero(t, dep.ID)

	fromList, err := q.ListDependenciesFrom(ctx, n1.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(fromList), 1)
}

// helpers
func sqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func sqlNullInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: i, Valid: true}
}

// Extended tests for Context Engine database operations

func TestCodeNodeOperations(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Test creating multiple code nodes
	node1, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "test-session",
		Path:      "/src/main.go",
		Language:  sqlNullString("go"),
		Symbol:    sqlNullString("main"),
		Kind:      sqlNullString("function"),
		StartLine: sqlNullInt64(10),
		EndLine:   sqlNullInt64(20),
		Metadata:  `{"complexity": 5}`,
	})
	require.NoError(t, err)
	require.NotZero(t, node1.ID)
	require.Equal(t, "test-session", node1.SessionID)

	node2, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "test-session",
		Path:      "/src/utils.go",
		Language:  sqlNullString("go"),
		Symbol:    sqlNullString("Helper"),
		Kind:      sqlNullString("function"),
		StartLine: sqlNullInt64(5),
		EndLine:   sqlNullInt64(15),
		Metadata:  `{"complexity": 2}`,
	})
	require.NoError(t, err)

	// Test list by session
	nodes, err := q.ListCodeNodesBySession(ctx, "test-session")
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	// Test list by path
	pathNodes, err := q.ListCodeNodesByPath(ctx, "/src/main.go")
	require.NoError(t, err)
	require.Len(t, pathNodes, 1)
	require.Equal(t, "main", pathNodes[0].Symbol.String)

	// Test list by kind
	kindNodes, err := q.ListCodeNodesByKind(ctx, sqlNullString("function"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(kindNodes), 2)

	// Test list by language
	langNodes, err := q.ListCodeNodesByLanguage(ctx, sqlNullString("go"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(langNodes), 2)

	// Test search by symbol (search for nodes with symbol starting with "main")
	symbolNodes, err := q.SearchCodeNodesBySymbol(ctx, sqlNullString("main"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(symbolNodes), 1)

	// Test count operations
	count, err := q.CountCodeNodesBySession(ctx, "test-session")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	kindCount, err := q.CountCodeNodesByKind(ctx, CountCodeNodesByKindParams{
		Kind:      sqlNullString("function"),
		SessionID: "test-session",
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), kindCount)

	// Test get by path and symbol
	node, err := q.GetCodeNodeByPathAndSymbol(ctx, GetCodeNodeByPathAndSymbolParams{
		Path:   "/src/main.go",
		Symbol: sqlNullString("main"),
	})
	require.NoError(t, err)
	require.Equal(t, node1.ID, node.ID)

	// Test list nodes in range (find nodes that overlap with range 8-12)
	rangeNodes, err := q.ListCodeNodesInRange(ctx, ListCodeNodesInRangeParams{
		Path:      "/src/main.go",
		StartLine: sqlNullInt64(12), // query end - for start_line <= ?
		EndLine:   sqlNullInt64(8),  // query start - for end_line >= ?
	})
	require.NoError(t, err)
	require.Len(t, rangeNodes, 1)

	// Test update
	updated, err := q.UpdateCodeNode(ctx, UpdateCodeNodeParams{
		ID:        node1.ID,
		Language:  sqlNullString("go"),
		Symbol:    sqlNullString("mainUpdated"),
		Kind:      sqlNullString("function"),
		StartLine: sqlNullInt64(10),
		EndLine:   sqlNullInt64(25),
		Metadata:  `{"complexity": 7}`,
	})
	require.NoError(t, err)
	require.Equal(t, "mainUpdated", updated.Symbol.String)

	// Test delete operations
	err = q.DeleteCodeNode(ctx, node2.ID)
	require.NoError(t, err)

	// Verify deletion
	nodes, err = q.ListCodeNodesBySession(ctx, "test-session")
	require.NoError(t, err)
	require.Len(t, nodes, 1)
}

func TestAnalysisMetadataOperations(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a test code node
	node, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "analysis-session",
		Path:      "/src/test.go",
		Language:  sqlNullString("go"),
		Symbol:    sqlNullString("TestFunc"),
		Kind:      sqlNullString("function"),
	})
	require.NoError(t, err)

	// Create analysis metadata for different tiers
	tier1, err := q.CreateAnalysisMetadata(ctx, CreateAnalysisMetadataParams{
		NodeID:    sqlNullInt64(node.ID),
		SessionID: sqlNullString("analysis-session"),
		Tier:      1,
		Result:    `{"structural": true}`,
		Status:    sqlNullString("pending"),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), tier1.Tier)

	tier2, err := q.CreateAnalysisMetadata(ctx, CreateAnalysisMetadataParams{
		NodeID:    sqlNullInt64(node.ID),
		SessionID: sqlNullString("analysis-session"),
		Tier:      2,
		Result:    `{"semantic": true}`,
		Status:    sqlNullString("running"),
	})
	require.NoError(t, err)

	// Test list by node
	nodeAnalysis, err := q.ListAnalysisByNode(ctx, sqlNullInt64(node.ID))
	require.NoError(t, err)
	require.Len(t, nodeAnalysis, 2)

	// Test list by session
	sessionAnalysis, err := q.ListAnalysisBySession(ctx, sqlNullString("analysis-session"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(sessionAnalysis), 2)

	// Test list by tier
	tierAnalysis, err := q.ListAnalysisByTier(ctx, 1)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tierAnalysis), 1)

	// Test list by status
	statusAnalysis, err := q.ListAnalysisByStatus(ctx, sqlNullString("pending"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(statusAnalysis), 1)

	// Test get latest analysis by node
	latest, err := q.GetLatestAnalysisByNode(ctx, GetLatestAnalysisByNodeParams{
		NodeID: sqlNullInt64(node.ID),
		Tier:   1,
	})
	require.NoError(t, err)
	require.Equal(t, tier1.ID, latest.ID)

	// Test count operations
	tierCount, err := q.CountAnalysisByTier(ctx, CountAnalysisByTierParams{
		Tier:      1,
		SessionID: sqlNullString("analysis-session"),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), tierCount)

	statusCount, err := q.CountAnalysisByStatus(ctx, CountAnalysisByStatusParams{
		Status:    sqlNullString("pending"),
		SessionID: sqlNullString("analysis-session"),
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), statusCount)

	// Test list pending analysis
	pending, err := q.ListPendingAnalysis(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(pending), 2)

	// Test update status
	updated, err := q.UpdateAnalysisStatus(ctx, UpdateAnalysisStatusParams{
		ID:     tier1.ID,
		Status: sqlNullString("completed"),
		Result: `{"structural": true, "completed": true}`,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", updated.Status.String)

	// Test delete operations
	err = q.DeleteAnalysisMetadata(ctx, tier2.ID)
	require.NoError(t, err)

	// Verify deletion
	nodeAnalysis, err = q.ListAnalysisByNode(ctx, sqlNullInt64(node.ID))
	require.NoError(t, err)
	require.Len(t, nodeAnalysis, 1)
}

func TestDependencyOperations(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create test nodes
	node1, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "dep-session",
		Path:      "/src/service.go",
		Symbol:    sqlNullString("Service"),
		Kind:      sqlNullString("class"),
	})
	require.NoError(t, err)

	node2, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "dep-session",
		Path:      "/src/repository.go",
		Symbol:    sqlNullString("Repository"),
		Kind:      sqlNullString("class"),
	})
	require.NoError(t, err)

	node3, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "dep-session",
		Path:      "/src/model.go",
		Symbol:    sqlNullString("Model"),
		Kind:      sqlNullString("struct"),
	})
	require.NoError(t, err)

	// Create dependencies
	dep1, err := q.CreateDependency(ctx, CreateDependencyParams{
		FromNode: node1.ID,
		ToNode:   node2.ID,
		Relation: sqlNullString("imports"),
		Metadata: `{"strength": 1.0}`,
	})
	require.NoError(t, err)

	dep2, err := q.CreateDependency(ctx, CreateDependencyParams{
		FromNode: node2.ID,
		ToNode:   node3.ID,
		Relation: sqlNullString("uses"),
		Metadata: `{"strength": 0.8}`,
	})
	require.NoError(t, err)

	// Test list dependencies from
	fromDeps, err := q.ListDependenciesFrom(ctx, node1.ID)
	require.NoError(t, err)
	require.Len(t, fromDeps, 1)
	require.Equal(t, dep1.ID, fromDeps[0].ID)

	// Test list dependencies to
	toDeps, err := q.ListDependenciesTo(ctx, node2.ID)
	require.NoError(t, err)
	require.Len(t, toDeps, 1)
	require.Equal(t, dep1.ID, toDeps[0].ID)

	// Test list by relation
	importDeps, err := q.ListDependenciesByRelation(ctx, sqlNullString("imports"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(importDeps), 1)

	// Test get specific dependency
	dep, err := q.GetDependency(ctx, GetDependencyParams{
		FromNode: node1.ID,
		ToNode:   node2.ID,
		Relation: sqlNullString("imports"),
	})
	require.NoError(t, err)
	require.Equal(t, dep1.ID, dep.ID)

	// Test count operations
	fromCount, err := q.CountDependenciesFrom(ctx, node1.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), fromCount)

	toCount, err := q.CountDependenciesTo(ctx, node2.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), toCount)

	// Test list all dependencies with joins
	allDeps, err := q.ListAllDependencies(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allDeps), 2)

	// Test delete operations
	err = q.DeleteDependency(ctx, dep2.ID)
	require.NoError(t, err)

	// Test delete by node
	err = q.DeleteDependenciesByNode(ctx, DeleteDependenciesByNodeParams{
		FromNode: node1.ID,
		ToNode:   node1.ID,
	})
	require.NoError(t, err)

	// Verify deletion
	fromDeps, err = q.ListDependenciesFrom(ctx, node1.ID)
	require.NoError(t, err)
	require.Len(t, fromDeps, 0)
}

func TestEmbeddingOperations(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create test node
	node, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{
		SessionID: "embedding-session",
		Path:      "/src/embedding.go",
		Symbol:    sqlNullString("EmbeddingFunc"),
		Kind:      sqlNullString("function"),
	})
	require.NoError(t, err)

	// Create embedding
	emb, err := q.CreateEmbedding(ctx, CreateEmbeddingParams{
		NodeID:    node.ID,
		SessionID: sqlNullString("embedding-session"),
		VectorID:  sqlNullString("vec-123"),
		Vector:    []byte{1, 2, 3, 4},
		Dims:      sqlNullInt64(4),
		Metadata:  `{"model": "text-embedding-ada-002"}`,
	})
	require.NoError(t, err)
	require.NotZero(t, emb.ID)

	// Test get embedding
	retrieved, err := q.GetEmbedding(ctx, emb.ID)
	require.NoError(t, err)
	require.Equal(t, emb.ID, retrieved.ID)

	// Test get by node
	byNode, err := q.GetEmbeddingByNode(ctx, node.ID)
	require.NoError(t, err)
	require.Equal(t, emb.ID, byNode.ID)

	// Test get by vector ID
	byVectorID, err := q.GetEmbeddingByVectorID(ctx, sqlNullString("vec-123"))
	require.NoError(t, err)
	require.Equal(t, emb.ID, byVectorID.ID)

	// Test list by session
	sessionEmbs, err := q.ListEmbeddingsBySession(ctx, sqlNullString("embedding-session"))
	require.NoError(t, err)
	require.Len(t, sessionEmbs, 1)

	// Test list by dims
	dimsEmbs, err := q.ListEmbeddingsByDims(ctx, sqlNullInt64(4))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(dimsEmbs), 1)

	// Test count by session
	count, err := q.CountEmbeddingsBySession(ctx, sqlNullString("embedding-session"))
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	// Test update embedding
	updated, err := q.UpdateEmbedding(ctx, UpdateEmbeddingParams{
		ID:       emb.ID,
		VectorID: sqlNullString("vec-456"),
		Vector:   []byte{5, 6, 7, 8},
		Dims:     sqlNullInt64(4),
		Metadata: `{"model": "text-embedding-3-large"}`,
	})
	require.NoError(t, err)
	require.Equal(t, "vec-456", updated.VectorID.String)

	// Test delete operations
	err = q.DeleteEmbedding(ctx, emb.ID)
	require.NoError(t, err)

	// Verify deletion
	count, err = q.CountEmbeddingsBySession(ctx, sqlNullString("embedding-session"))
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}
