package dbschema

import (
	"strings"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// MapDBError reports a violated constraint of the database as constraint
// violation. The message of the database, which names the affected table and
// column, is only kept as cause, it is of no use for the user.
func MapDBError(err error) error {
	if err == nil {
		return nil
	}

	if strings.HasPrefix(err.Error(), "UNIQUE constraint failed") {
		return domain.NewErrorf(domain.ErrConstraintViolation, "", "The entry already exists").WithCause(err)
	}

	if strings.HasPrefix(err.Error(), "FOREIGN KEY constraint failed") {
		return MapForeignKeyError(err, "The entry")
	}

	return err
}

// MapForeignKeyError reports a violated foreign key constraint of entityName
// as constraint violation. SQLite does not report which side of the reference
// failed. A failed delete means that another entity still references
// entityName. Any other failed operation means that entityName references an
// entity, which does not exist.
func MapForeignKeyError(err error, entityName string) error {
	// The generated delete functions wrap the database error with this prefix.
	if strings.HasPrefix(err.Error(), "Delete ") {
		return domain.NewErrorf(domain.ErrConstraintViolation, "", "%s is still referenced by another entity", entityName).WithCause(err)
	}

	return domain.NewErrorf(domain.ErrConstraintViolation, "", "%s references an entity, which does not exist", entityName).WithCause(err)
}
