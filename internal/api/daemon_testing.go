package api

import (
	"database/sql"
	"testing"
)

// GlobalDBTest returns the global database of the started daemon, for tests to seed it.
func (d *Daemon) GlobalDBTest(t *testing.T) *sql.DB {
	t.Helper()

	return d.globalRepo.DB()
}
