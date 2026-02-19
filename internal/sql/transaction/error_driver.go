package transaction

import (
	"database/sql"
	"database/sql/driver"
	"errors"
)

type errorDriver struct{}

var errDriver *errorDriver

func init() {
	errDriver = &errorDriver{}
	sql.Register("sqlerrordriver", errDriver)
}

func (e *errorDriver) Open(dsn string) (driver.Conn, error) {
	return e, nil
}

// Begin meets https://pkg.go.dev/database/sql/driver#Conn interface.
func (e *errorDriver) Begin() (driver.Tx, error) {
	return e, nil
}

// Close meets https://pkg.go.dev/database/sql/driver#Conn interface.
func (e *errorDriver) Close() error {
	return nil
}

// Prepare meets https://pkg.go.dev/database/sql/driver#Conn interface.
func (e *errorDriver) Prepare(query string) (driver.Stmt, error) {
	return &errorStatement{
		err: errors.New(query),
	}, nil
}

func (e *errorDriver) Commit() error {
	return nil
}

func (e *errorDriver) Rollback() error {
	return nil
}

type errorStatement struct {
	err error
}

// Close meets https://pkg.go.dev/database/sql/driver#Stmt interface.
func (e *errorStatement) Close() error {
	return e.err
}

// NumInput meets https://pkg.go.dev/database/sql/driver#Stmt interface.
func (e *errorStatement) NumInput() int {
	return -1
}

// Exec meets https://pkg.go.dev/database/sql/driver#Stmt interface.
func (e *errorStatement) Exec(args []driver.Value) (driver.Result, error) {
	return nil, e.err
}

// Query meets https://pkg.go.dev/database/sql/driver#Stmt interface.
func (e *errorStatement) Query(args []driver.Value) (driver.Rows, error) {
	return nil, e.err
}
