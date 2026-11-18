package sqlite

import (
	"database/sql"
	"errors"

	"github.com/cowsql/go-cowsql/driver"
	"github.com/mattn/go-sqlite3"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func MapErr(err error) error {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		_, ok := ConstraintErr(err)
		if ok {
			return domain.NewErrorf(domain.ErrConstraintViolation, "", "The database rejected the change, because it violates a constraint").WithCause(err)
		}

		return err
	}

	return nil
}

// ConstraintErr returns the extended code of a violated constraint, reported by sqlite or cowsql.
func ConstraintErr(err error) (sqlite3.ErrNoExtended, bool) {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
		return sqliteErr.ExtendedCode, true
	}

	var cowsqlErr driver.Error
	if errors.As(err, &cowsqlErr) && sqlite3.ErrNo(cowsqlErr.Code&0xff) == sqlite3.ErrConstraint {
		return sqlite3.ErrNoExtended(cowsqlErr.Code), true
	}

	return 0, false
}
