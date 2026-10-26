package cowsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/cowsql/go-cowsql"
	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	"github.com/cowsql/go-cowsql/driver"

	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// SQLite is used through cowsql and directly for the local database, so it needs its locks.
func init() {
	err := cowsql.ConfigMultiThread()
	if err != nil {
		panic(fmt.Sprintf("Failed to enable the multi-thread mode of SQLite: %v", err))
	}
}

type GlobalDB struct {
	*sql.DB
	nodeID        int64
	schemaVersion int
}

func (g *GlobalDB) BeginTx(ctx context.Context) (transaction.TX, error) {
	return g.DB.BeginTx(ctx, nil)
}

func (g *GlobalDB) SchemaVersion() int {
	return g.schemaVersion
}

func (g *GlobalDB) NodeID() int64 {
	return g.nodeID
}

func (g *GlobalDB) SetNodeID(id int64) {
	g.nodeID = id
}

// Monotonic serial number for registering new instances of cowsql.Driver
// using the database/sql stdlib package. This is needed since there's no way
// to unregister drivers, and in unit tests more than one driver gets
// registered.
var cowsqlDriverSerial uint64

// Generate a new name for the cowsql driver registration. We need it to be
// unique for testing, see below.
func cowsqlDriverName() string {
	return fmt.Sprintf("cowsql-%d", atomic.AddUint64(&cowsqlDriverSerial, 1))
}

// Open the cluster database object.
//
// The name argument is the name of the cluster database. It defaults to
// 'db.bin', but can be overwritten for testing.
//
// The dialer argument is a function that returns a gRPC dialer that can be
// used to connect to a database node using the gRPC SQL package.
func open(name string, store driver.NodeStore, options ...driver.Option) (*sql.DB, error) {
	drv, err := driver.New(store, options...)
	if err != nil {
		return nil, fmt.Errorf("Failed to create cowsql driver: %w", err)
	}

	driverName := cowsqlDriverName()
	sql.Register(driverName, drv)

	// Create the cluster db. This won't immediately establish any network
	// connection, that will happen only when a db transaction is started
	// (see the database/sql connection pooling code for more details).
	if name == "" {
		name = "db.bin"
	}

	db, err := sql.Open(driverName, name)
	if err != nil {
		return nil, fmt.Errorf("cannot open cluster database: %w", err)
	}

	return db, nil
}

// ErrSomeNodesAreBehind is returned by OpenCluster if some of the nodes in the
// cluster have a schema or API version that is less recent than this node.
var ErrSomeNodesAreBehind = errors.New("some nodes are behind this node's version")

func Open(closingCtx context.Context, name string, store driver.NodeStore, address, dir string, timeout time.Duration, options ...driver.Option) (*GlobalDB, error) {
	db, err := open(name, store, options...)
	if err != nil {
		return nil, fmt.Errorf("Failed to open global database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Test that the cluster database is operational. We wait up to the
	// given timeout , in case there's no quorum of nodes online yet.
	connectCtx, connectCancel := context.WithTimeout(closingCtx, timeout)
	defer connectCancel()
	for i := 0; ; i++ {
		// Log initial attempts at debug level, but use warn
		// level after the 5'th attempt (about 10 seconds).
		// After the 15'th attempt (about 30 seconds), log
		// only one attempt every 5.
		logPriority := 1 // 0 is discard, 1 is Debug, 2 is Error
		if i > 5 {
			logPriority = 2
			if i > 15 && i%5 != 0 {
				logPriority = 0
			}
		}

		slog.InfoContext(connectCtx, "Connecting to global database")
		pingCtx, pingCancel := context.WithTimeout(connectCtx, time.Second*5)
		err = db.PingContext(pingCtx)
		pingCancel()

		if err != nil && !errors.Is(err, driver.ErrNoAvailableLeader) {
			return nil, err
		} else if err == nil {
			slog.InfoContext(connectCtx, "Connected to global database")
			break
		}

		switch logPriority {
		case 1:
			slog.DebugContext(connectCtx, "Failed connecting to global database", logger.Err(err), slog.Int("attempt", i))

		case 2:
			slog.ErrorContext(connectCtx, "Failed connecting to global database", logger.Err(err), slog.Int("attempt", i))
		}

		select {
		case <-connectCtx.Done():
			return nil, connectCtx.Err()

		default:
			time.Sleep(2 * time.Second)
		}
	}

	_, err = db.Exec("PRAGMA cache_size=-50000")
	if err != nil {
		return nil, fmt.Errorf("Failed to set page cache size: %w", err)
	}

	nodesVersionsMatch, schemaVersion, err := dbschema.EnsureGlobal(context.TODO(), db, address, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure schema: %w", err)
	}

	gdb := &GlobalDB{DB: db, schemaVersion: schemaVersion}
	if !nodesVersionsMatch {
		return gdb, ErrSomeNodesAreBehind
	}

	return gdb, nil
}
