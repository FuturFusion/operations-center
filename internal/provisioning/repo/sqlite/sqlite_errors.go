package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/mattn/go-sqlite3"

	"github.com/FuturFusion/operations-center/internal/domain"
)

func mapErr(err error) error {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) {
			if sqliteErr.Code == sqlite3.ErrConstraint {
				return fmt.Errorf("%w: %v", domain.ErrConstraintViolation, err)
			}
		}

		return err
	}

	return nil
}
