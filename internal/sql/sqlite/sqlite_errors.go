package sqlite

import (
	"database/sql"
	"errors"

	"github.com/mattn/go-sqlite3"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func MapErr(err error) error {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return domain.NewErrorf(domain.ErrConstraintViolation, "", "The database rejected the change, because it violates a constraint").WithCause(err)
			}
		}

		return err
	}

	return nil
}
