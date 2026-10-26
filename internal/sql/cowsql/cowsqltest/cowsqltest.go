package cowsqltest

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	cowsqlnode "github.com/cowsql/go-cowsql"
	"github.com/cowsql/go-cowsql/driver"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/cowsql"
)

// cowsql names the VFS of a node from a counter, which isn't thread safe.
var nodeCreateMu sync.Mutex

// Open starts a single cowsql node on an abstract unix socket and returns its
// global database with the schema applied. The node stops when the test ends.
func Open(t *testing.T) (*cowsql.GlobalDB, error) {
	t.Helper()

	// The kernel picks an unused abstract address.
	listener, err := net.Listen("unix", "")
	require.NoError(t, err)

	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	dir := filepath.Join(t.TempDir(), "global")
	require.NoError(t, os.Mkdir(dir, 0o700))

	nodeCreateMu.Lock()
	node, err := cowsqlnode.New(1, address, dir, cowsqlnode.WithBindAddress(address))
	nodeCreateMu.Unlock()
	require.NoError(t, err)
	require.NoError(t, node.Start())

	t.Cleanup(func() {
		require.NoError(t, node.Close())
	})

	store, err := driver.DefaultNodeStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), []driver.NodeInfo{{ID: 1, Address: address}}))

	dial := func(ctx context.Context, address string) (net.Conn, error) {
		return net.Dial("unix", address)
	}

	return cowsql.Open(context.Background(), "test.db", store, address, dir, 5*time.Second, driver.WithDialFunc(dial))
}
