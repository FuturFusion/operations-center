package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/cowsql/go-cowsql/cluster/db/transaction"
	"github.com/mattn/go-sqlite3"
)

// TODO: make this command-line configurable?
const busyTimeoutMS = 5000

func init() {
	sql.Register("sqlite3_with_fk", &sqlite3.SQLiteDriver{ConnectHook: sqliteEnableForeignKeys})
}

type DB struct {
	*sql.DB
}

// Open the local database object.
func Open(dir string) (*DB, error) {
	path := filepath.Join(dir, "local.db")

	// These are used to tune the transaction BEGIN behavior instead of using the
	// similar "locking_mode" pragma (locking for the whole database connection).
	openPath := fmt.Sprintf("%s?_busy_timeout=%d&_txlock=exclusive", path, busyTimeoutMS)

	// Open the database. If the file doesn't exist it is created.
	db, err := sql.Open("sqlite3_with_fk", openPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open node database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return &DB{DB: db}, nil
}

func (d *DB) BeginTx(ctx context.Context) (transaction.TX, error) {
	return d.DB.BeginTx(ctx, nil)
}

func sqliteEnableForeignKeys(conn *sqlite3.SQLiteConn) error {
	_, err := conn.Exec("PRAGMA foreign_keys=ON;", nil)
	return err
}
