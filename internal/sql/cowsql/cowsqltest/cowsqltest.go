package cowsqltest

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	cowsqlnode "github.com/cowsql/go-cowsql"
	"github.com/cowsql/go-cowsql/client"
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

	dir := filepath.Join(t.TempDir(), "global")
	require.NoError(t, os.Mkdir(dir, 0o700))

	address, store, dial := startNode(t, dir)

	return cowsql.Open(context.Background(), "test.db", store, address, dir, 5*time.Second, driver.WithDialFunc(dial))
}

// OpenEmpty starts a single cowsql node in dir and returns its global database
// without any schema. The node stops when the test ends.
func OpenEmpty(t *testing.T, dir string) *sql.DB {
	t.Helper()

	_, store, dial := startNode(t, dir)

	drv, err := driver.New(store, driver.WithDialFunc(dial))
	require.NoError(t, err)

	connector, err := drv.OpenConnector("test.db")
	require.NoError(t, err)

	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Like the global database of the daemon.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return db
}

func startNode(t *testing.T, dir string) (string, client.NodeStore, func(context.Context, string) (net.Conn, error)) {
	t.Helper()

	// The kernel picks an unused abstract address.
	listener, err := net.Listen("unix", "")
	require.NoError(t, err)

	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	nodeCreateMu.Lock()
	node, err := cowsqlnode.New(1, address, dir, cowsqlnode.WithBindAddress(address))
	nodeCreateMu.Unlock()
	require.NoError(t, err)
	require.NoError(t, node.Start())

	t.Cleanup(func() {
		require.NoError(t, node.Close())
	})

	// The store holds no database, so the test leaves no goroutine of it behind.
	store := client.NewInmemNodeStore()
	require.NoError(t, store.Set(context.Background(), []driver.NodeInfo{{ID: 1, Address: address}}))

	dial := func(ctx context.Context, address string) (net.Conn, error) {
		return net.Dial("unix", address)
	}

	return address, store, dial
}
