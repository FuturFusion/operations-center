package dbschema_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func TestSchemaEnsure(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*sql.DB) error

		wantSchemaVersion int
	}{
		{
			name:    "ensure from fresh",
			prepare: func(_ *sql.DB) error { return nil },

			wantSchemaVersion: 0,
		},
		{
			name: "update v1 to latest",
			prepare: func(db *sql.DB) error {
				return dbschema.SetupVersion(db, 1)
			},

			wantSchemaVersion: 1,
		},
		{
			name: "update latest to latest",
			prepare: func(db *sql.DB) error {
				return dbschema.SetupVersion(db, dbschema.MaxVersion())
			},

			wantSchemaVersion: dbschema.MaxVersion(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			db, err := sqlite.Open(tmpDir)
			require.NoError(t, err)

			err = tc.prepare(db)
			require.NoError(t, err)

			schemaVersion, err := dbschema.Ensure(context.Background(), db, tmpDir)
			require.NoError(t, err)
			require.Equal(t, tc.wantSchemaVersion, schemaVersion)
		})
	}
}
