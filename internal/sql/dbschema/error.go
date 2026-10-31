package dbschema

import (
	"errors"
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
		return domain.NewErrorf(domain.ErrConstraintViolation, "", "The entry references an entity, which does not exist").WithCause(err)
	}

	return err
}

// ErrGracefulAbort is a special error that can be returned by a Check function
// to force Schema.Ensure to abort gracefully.
//
// Every change performed so by the Check will be committed, although
// ErrGracefulAbort will be returned.
var ErrGracefulAbort = errors.New("schema check gracefully aborted")
