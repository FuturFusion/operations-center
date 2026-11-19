package sqlite

import (
	"encoding/json"
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

// Every field of the shared config stored by v44 reads and writes back unchanged, so a changed json tag of its types fails here.
// Fields added later are written back in addition.
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

	var storedFields, writtenFields map[string]any
	require.NoError(t, json.Unmarshal(stored, &storedFields))
	require.NoError(t, json.Unmarshal([]byte(data), &writtenFields))
	requireFieldsKept(t, storedFields, writtenFields, "")
}

// requireFieldsKept fails if a field of stored is missing in written or holds another value.
func requireFieldsKept(t *testing.T, stored map[string]any, written map[string]any, path string) {
	t.Helper()

	for key, value := range stored {
		writtenValue, ok := written[key]
		require.True(t, ok, "Field %s%s is not written back", path, key)

		storedObject, isObject := value.(map[string]any)
		if !isObject {
			require.Equal(t, value, writtenValue, "Field %s%s changed", path, key)
			continue
		}

		writtenObject, isObject := writtenValue.(map[string]any)
		require.True(t, isObject, "Field %s%s is not an object anymore", path, key)
		requireFieldsKept(t, storedObject, writtenObject, path+key+".")
	}
}
