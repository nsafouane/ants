package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConnectAndMigrations ensures Connect applies migrations and DB is usable.
func TestConnectAndMigrations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "crush-db-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Ensure dataDir exists
	dataDir := filepath.Join(tmpDir, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))

	db, err := Connect(ctx, dataDir)
	require.NoError(t, err)
	defer db.Close()

	// Ensure queries can be constructed
	q := New(db)
	_, err = q.ListSessions(ctx)
	require.NoError(t, err)
}
