package dbschema

import (
	"net/http"
	"strings"

	"github.com/FuturFusion/operations-center/shared/api"
)

func MapDBError(err error) error {
	if err == nil {
		return nil
	}

	if strings.HasPrefix(err.Error(), "UNIQUE constraint failed") {
		return api.StatusErrorf(http.StatusBadRequest, "Database operation failed: %v", err)
	}

	if strings.HasPrefix(err.Error(), "FOREIGN KEY constraint failed") {
		return api.StatusErrorf(http.StatusBadRequest, "Database operation failed: %v", err)
	}

	return err
}
