package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateAndUpdateAnalysisMetadata(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// create a node to attach analysis to
	node, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{Path: "/analyze.go"})
	require.NoError(t, err)

	created, err := q.CreateAnalysisMetadata(ctx, CreateAnalysisMetadataParams{
		NodeID:    sql.NullInt64{Int64: node.ID, Valid: true},
		SessionID: sql.NullString{String: "sess-a", Valid: true},
		Tier:      1,
		Result:    "{}",
		Status:    sql.NullString{String: "pending", Valid: true},
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	// Update analysis status to completed
	now := time.Now()
	updated, err := q.UpdateAnalysisStatus(ctx, UpdateAnalysisStatusParams{
		Status:      sql.NullString{String: "completed", Valid: true},
		Result:      "{\"ok\":true}",
		CompletedAt: sql.NullTime{Time: now, Valid: true},
		ID:          created.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "completed", updated.Status.String)

	// List by node
	list, err := q.ListAnalysisByNode(ctx, sql.NullInt64{Int64: node.ID, Valid: true})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(list), 1)
}

func TestDuplicateVectorIDAndFKFailures(t *testing.T) {
	t.Parallel()
	q, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// create node and embedding with vector_id
	n, err := q.CreateCodeNode(ctx, CreateCodeNodeParams{Path: "/v.go"})
	require.NoError(t, err)

	_, err = q.CreateEmbedding(ctx, CreateEmbeddingParams{
		NodeID:   n.ID,
		VectorID: sql.NullString{String: "dup-vec", Valid: true},
		Vector:   []byte{1},
		Metadata: "{}",
	})
	require.NoError(t, err)

	// inserting another embedding with same vector_id should succeed (no uniqueness constraint)
	_, err = q.CreateEmbedding(ctx, CreateEmbeddingParams{
		NodeID:   n.ID,
		VectorID: sql.NullString{String: "dup-vec", Valid: true},
		Vector:   []byte{2},
		Metadata: "{}",
	})
	require.NoError(t, err)

	// attempt to create a dependency referencing non-existent node should fail due to FK
	_, err = q.CreateDependency(ctx, CreateDependencyParams{FromNode: 9999999, ToNode: 8888888, Relation: sql.NullString{String: "import", Valid: true}})
	require.Error(t, err)
}
