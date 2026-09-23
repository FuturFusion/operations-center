package entities

import (
	"errors"
	"strings"

	"github.com/mattn/go-sqlite3"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
)

func init() {
	mapErr = warningMapErr
}

func warningMapErr(err error, entity string) error {
	entityName := strings.ReplaceAll(entity, "_", " ")

	if errors.Is(err, ErrNotFound) {
		return domain.NewErrorf(domain.ErrNotFound, "", "%s not found", entityName).WithCause(err)
	}

	if errors.Is(err, ErrConflict) {
		return domain.NewErrorf(domain.ErrConstraintViolation, "", "%s already exists", entityName).WithCause(err)
	}

	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
		switch sqliteErr.ExtendedCode {
		case sqlite3.ErrConstraintUnique, sqlite3.ErrConstraintPrimaryKey:
			return domain.NewErrorf(domain.ErrConstraintViolation, "", "%s already exists", entityName).WithCause(err)

		case sqlite3.ErrConstraintForeignKey:
			return domain.NewErrorf(domain.ErrConstraintViolation, "", "%s references an entity, which does not exist", entityName).WithCause(err)

		default:
			return domain.NewErrorf(domain.ErrConstraintViolation, "", "%s violates a constraint of the database", entityName).WithCause(err)
		}
	}

	return dbschema.MapDBError(err)
}
