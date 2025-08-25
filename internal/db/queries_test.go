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
        NodeID:   node.ID,
        SessionID: sql.NullString{String: "s-vec", Valid: true},
        VectorID: sql.NullString{String: "vec-1", Valid: true},
        Vector:   []byte{1, 2},
        Dims:     sql.NullInt64{},
        Metadata: "{}",
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
