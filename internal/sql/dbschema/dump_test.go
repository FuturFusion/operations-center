package dbschema_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func TestDumpSchema(t *testing.T) {
	const insertToken = `INSERT INTO tokens (uuid, uses_remaining, expire_at, description, channel_id, auto_remove)
VALUES ('b32d0079-c48b-4957-b1cb-bef54125c861', 1, '2026-01-01 00:00:00+00:00', '', 1, 1)`

	tests := []struct {
		name  string
		setup func(t *testing.T, db dbdriver.DBTX)

		wantContains string
	}{
		{
			name: "boolean true",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), insertToken)
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO tokens VALUES(1,'b32d0079-c48b-4957-b1cb-bef54125c861',1,'2026-01-01 00:00:00+00:00','',1,1);`,
		},
		{
			name: "boolean false",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), insertToken)
				require.NoError(t, err)

				_, err = db.ExecContext(t.Context(), `INSERT INTO tokens_seeds (token_id, name, description, public, seeds, last_updated)
VALUES (1, 'seed', '', 0, '{}', '2026-01-01 00:00:00+00:00')`)
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO tokens_seeds VALUES(1,1,'seed','',0,'{}','2026-01-01 00:00:00+00:00');`,
		},
		{
			name: "real",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE reals (value REAL NOT NULL); INSERT INTO reals VALUES (1.5)`)
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO reals VALUES(1.5);`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			ctx := context.Background()
			tmpDir := t.TempDir()

			db, err := dbdriver.Open(tmpDir)
			require.NoError(t, err)

			t.Cleanup(func() {
				err := db.Close()
				require.NoError(t, err)
			})

			_, err = dbschema.EnsureLocal(ctx, db.DB, tmpDir, dbschema.Updates(), dbschema.FreshSchema())
			require.NoError(t, err)

			tc.setup(t, db)

			// Run test
			got, err := dbschema.DumpSchema(ctx, db, false)

			// Assert
			require.NoError(t, err)
			require.Contains(t, got, tc.wantContains, "dump should contain the inserted row")
		})
	}
}
