package entities

import (
	"errors"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/sqlite"
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

	return sqlite.MapErr(err)
}
