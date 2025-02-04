package dbschema

import (
	"context"
	"database/sql"
)

func SetupVersion(db *sql.DB, version int) error {
	selectedUpdates := make(map[int]update, version)
	for k, v := range updates {
		if k <= version {
			selectedUpdates[k] = v
		}
	}

	schema := newFromMap(selectedUpdates)
	_, err := schema.ensure(context.Background(), db)
	return err
}

func MaxVersion() int {
	var maxVersion int
	for k := range updates {
		if k > maxVersion {
			maxVersion = k
		}
	}

	return maxVersion
}
