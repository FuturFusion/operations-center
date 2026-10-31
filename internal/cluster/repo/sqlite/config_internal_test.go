package sqlite

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
)

func TestConfigRepo_CreateSharedConfig_once(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	stmts, err := entities.PrepareStmts(db, false)
	require.NoError(t, err)

	ctx := entities.WithPreparedStmts(t.Context(), stmts)
	repo := NewConfigRepo(NewGlobalRaftRepo(db))

	require.NoError(t, repo.CreateSharedConfig(ctx, config.SharedConfig{}))

	// A second shared row would make every later read and update fail.
	err = repo.CreateSharedConfig(ctx, config.SharedConfig{})
	require.ErrorIs(t, err, entities.ErrConflict)

	_, err = repo.GetSharedConfig(ctx)
	require.NoError(t, err)
}

// The shared config stored by v44 reads and writes back unchanged, so a changed json tag of its types fails here.
func TestConfigRepo_storedFormat(t *testing.T) {
	t.Parallel()

	stored, err := os.ReadFile("testdata/shared_config_v44.json")
	require.NoError(t, err)

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	stmts, err := entities.PrepareStmts(db, false)
	require.NoError(t, err)

	ctx := entities.WithPreparedStmts(t.Context(), stmts)
	repo := NewConfigRepo(NewGlobalRaftRepo(db))

	_, err = db.ExecContext(ctx, `INSERT INTO config (node_id, data) VALUES (NULL, ?)`, string(stored))
	require.NoError(t, err)

	cfg, err := repo.GetSharedConfig(ctx)
	require.NoError(t, err)

	err = repo.UpdateSharedConfig(ctx, *cfg)
	require.NoError(t, err)

	var data string
	err = db.QueryRowContext(ctx, `SELECT data FROM config WHERE node_id IS NULL`).Scan(&data)
	require.NoError(t, err)
	require.JSONEq(t, string(stored), data)
}
