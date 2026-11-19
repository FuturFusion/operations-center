package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
)

func TestConfigRepo_GetSharedConfig(t *testing.T) {
	t.Parallel()

	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	stmts, err := entities.PrepareStmts(db, false)
	require.NoError(t, err)

	ctx := entities.WithPreparedStmts(t.Context(), stmts)
	repo := sqlite.NewConfigRepo(sqlite.NewGlobalRaftRepo(db))

	// A database without a shared config gets the provisional one.
	cfg, err := repo.GetSharedConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, config.ProvisionalShared(), *cfg)

	updated := *cfg
	updated.Settings.ServerRegistrationScriptlet = "updated"
	require.NoError(t, repo.UpdateSharedConfig(ctx, updated))

	cfg, err = repo.GetSharedConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, updated, *cfg)

	// A shared config that isn't valid JSON fails.
	_, err = db.ExecContext(ctx, `UPDATE config SET data = '{' WHERE node_id IS NULL`)
	require.NoError(t, err)

	_, err = repo.GetSharedConfig(ctx)
	require.Error(t, err)
}
