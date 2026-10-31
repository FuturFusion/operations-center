package patches

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func openPatchesDB(t *testing.T) dbdriver.DBTX {
	t.Helper()

	db, err := dbdriver.Open(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(t.Context(), `CREATE TABLE patches (id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, name VARCHAR(255) NOT NULL, applied_at DATETIME NOT NULL, UNIQUE (name))`)
	require.NoError(t, err)

	return db
}

// setPatches replaces the patches for the test.
func setPatches(t *testing.T, ps []patch) {
	t.Helper()

	previous := patches
	patches = ps
	t.Cleanup(func() { patches = previous })
}

func TestApply_clusterPatchOnce(t *testing.T) {
	var runs atomic.Int32
	running := make(chan struct{})
	release := make(chan struct{})

	setPatches(t, []patch{{name: "cluster", stage: PreSecurityInfrastructure, scope: scopeCluster, run: func(ctx context.Context, name string, _ *os.Root) error {
		if runs.Add(1) == 1 {
			close(running)
			<-release
		}

		return nil
	}}})

	global := openPatchesDB(t)
	firstLocal := openPatchesDB(t)

	first := make(chan error)
	go func() {
		first <- Apply(t.Context(), global, firstLocal, nil, PreSecurityInfrastructure)
	}()

	// The second member starts while the first one still runs the patch.
	<-running
	require.NoError(t, Apply(t.Context(), global, openPatchesDB(t), nil, PreSecurityInfrastructure))

	close(release)
	require.NoError(t, <-first)
	require.Equal(t, int32(1), runs.Load(), "a cluster patch runs once per cluster")
}

func TestApply_memberPatchOnEveryMember(t *testing.T) {
	var runs atomic.Int32
	setPatches(t, []patch{{name: "member", stage: PreSecurityInfrastructure, scope: scopeMember, run: func(ctx context.Context, name string, _ *os.Root) error {
		runs.Add(1)
		return nil
	}}})

	global := openPatchesDB(t)
	local := openPatchesDB(t)

	require.NoError(t, Apply(t.Context(), global, local, nil, PreSecurityInfrastructure))
	require.NoError(t, Apply(t.Context(), global, openPatchesDB(t), nil, PreSecurityInfrastructure))

	// A restart of the first member doesn't run it again.
	require.NoError(t, Apply(t.Context(), global, local, nil, PreSecurityInfrastructure))

	require.Equal(t, int32(2), runs.Load(), "a member patch runs once on every member")
}

func TestApply_failedClusterPatchRunsAgain(t *testing.T) {
	boom := errors.New("boom")
	fail := true

	var runs atomic.Int32
	setPatches(t, []patch{{name: "cluster", stage: PreSecurityInfrastructure, scope: scopeCluster, run: func(ctx context.Context, name string, _ *os.Root) error {
		runs.Add(1)
		if fail {
			return boom
		}

		return nil
	}}})

	global := openPatchesDB(t)

	err := Apply(t.Context(), global, openPatchesDB(t), nil, PreSecurityInfrastructure)
	require.ErrorIs(t, err, boom)

	fail = false
	require.NoError(t, Apply(t.Context(), global, openPatchesDB(t), nil, PreSecurityInfrastructure))
	require.Equal(t, int32(2), runs.Load(), "a failed cluster patch runs again on the next start")
}
