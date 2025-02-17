package entities

import (
	"errors"
	"net/http"

	"github.com/lxc/incus/v6/shared/api"
)

var (
	ErrNotFound = errors.New("Not found")
	ErrConflict = errors.New("Conflict")
)

func mapErr(err error, entity string) error {
	if errors.Is(err, ErrNotFound) {
		return api.StatusErrorf(http.StatusNotFound, "%s not found", entity)
	}

	if errors.Is(err, ErrConflict) {
		// TODO: This is not exactly the same error as before:
		// api.StatusErrorf(http.StatusConflict, "This \"%s\" entry already exists")`, entityTable(m.entity, m.config["table"])
		return api.StatusErrorf(http.StatusConflict, "This entry already exists for %s", entity)
	}

	return err
}
