package entities

import (
	"errors"
	"fmt"

	"github.com/mattn/go-sqlite3"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
)

func init() {
	mapErr = clusterMapErr
}

func clusterMapErr(err error, entity string) error {
	if errors.Is(err, ErrNotFound) {
		return domain.ErrNotFound
	}

	if errors.Is(err, ErrConflict) {
		return domain.ErrConstraintViolation
	}

	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		if sqliteErr.Code == sqlite3.ErrConstraint {
			return fmt.Errorf("%w: %v", domain.ErrConstraintViolation, err)
		}
	}

	return dbschema.MapDBError(err)
}
