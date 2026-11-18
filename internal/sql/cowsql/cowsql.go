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
	"github.com/lxc/incus/v7/shared/revert"

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

// cowsqlDriverName returns a new name for the registration of the cowsql driver. database/sql registers a name only
// once, while the global database is opened again, e.g. after waiting for upgraded members, and in tests.
func cowsqlDriverName() string {
	return fmt.Sprintf("cowsql-%d", atomic.AddUint64(&cowsqlDriverSerial, 1))
}

// open returns the global database named name, 'db.bin' if empty, through a new cowsql driver.
//
// The options of the driver set how it connects to the members, e.g. with the dial function of the gateway.
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

// ErrSomeNodesAreBehind is returned by Open if some cluster members have a schema version or a number of API
// extensions that is less recent than the one of this member.
var ErrSomeNodesAreBehind = errors.New("some nodes are behind this node's version")

func Open(closingCtx context.Context, name string, store driver.NodeStore, address, dir string, timeout time.Duration, options ...driver.Option) (*GlobalDB, error) {
	db, err := open(name, store, options...)
	if err != nil {
		return nil, fmt.Errorf("Failed to open global database: %w", err)
	}

	// The database stays open only when it is returned.
	reverter := revert.New()
	defer reverter.Fail()

	reverter.Add(func() { _ = db.Close() })

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

	nodesVersionsMatch, schemaVersion, err := dbschema.EnsureGlobal(closingCtx, db, address, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure schema: %w", err)
	}

	reverter.Success()

	gdb := &GlobalDB{DB: db, schemaVersion: schemaVersion}
	if !nodesVersionsMatch {
		return gdb, ErrSomeNodesAreBehind
	}

	return gdb, nil
}
