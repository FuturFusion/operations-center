package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGlobalDB_EnterExclusive_timeout(t *testing.T) {
	t.Parallel()

	g := &globalDB{exclusiveTimeout: 50 * time.Millisecond}

	release := g.OnTxStart(false)
	require.Error(t, g.EnterExclusive())
	release()

	// An abandoned exclusive lock must not block later transactions.
	done := make(chan struct{})
	go func() {
		g.OnTxStart(false)()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Transaction blocked by an abandoned exclusive lock")
	}

	require.NoError(t, g.EnterExclusive())
	g.OnTxStart(true)()
}
