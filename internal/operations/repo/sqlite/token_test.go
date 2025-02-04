package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/operations"
	"github.com/FuturFusion/operations-center/internal/operations/repo/sqlite"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func TestTokenDatabaseActions(t *testing.T) {
	tokenA := operations.Token{
		UsesRemaining: 1,
		ExpireAt:      time.Now().Add(1 * time.Minute),
		Description:   "foobar",
	}

	ctx := context.Background()

	// Create a new temporary database.
	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	token := sqlite.NewToken(db)

	// Add token
	_, err = token.Create(ctx, tokenA)
	require.NoError(t, err)
}
