package api

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func Test_dbtxWrapper_concurrent(t *testing.T) {
	t.Parallel()

	db, err := sqlite.Open(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	wrapper := &dbtxWrapper{}
	require.Nil(t, wrapper.get(), "the database is not registered yet")

	done := make(chan struct{})
	go func() {
		defer close(done)

		// The daemon registers the global database while requests already check for it.
		wrapper.Register(db)
	}()

	for wrapper.get() == nil {
		runtime.Gosched()
	}

	var one int
	require.NoError(t, wrapper.QueryRowContext(t.Context(), "SELECT 1").Scan(&one))
	require.Equal(t, 1, one)

	<-done
}
