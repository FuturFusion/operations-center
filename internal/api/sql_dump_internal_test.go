package api

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

			wantContains: `INSERT INTO "tokens" VALUES(1,'b32d0079-c48b-4957-b1cb-bef54125c861',1,'2026-01-01 00:00:00+00:00','',1,1);`,
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

			wantContains: `INSERT INTO "tokens_seeds" VALUES(1,1,'seed','',0,'{}','2026-01-01 00:00:00+00:00');`,
		},
		{
			name: "real",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE reals (value REAL NOT NULL); INSERT INTO reals VALUES (1.5)`)
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO "reals" VALUES(1.5);`,
		},
		{
			name: "blob",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE blobs (value BLOB); INSERT INTO blobs VALUES (X'00ff27')`)
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO "blobs" VALUES(X'00FF27');`,
		},
		{
			name: "blob with single quote in text column",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE objects (object TEXT NOT NULL)`)
				require.NoError(t, err)

				_, err = db.ExecContext(t.Context(), `INSERT INTO objects VALUES (?)`, []byte(`x');--`))
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO "objects" VALUES(X'7827293B2D2D');`,
		},
		{
			name: "table name with double quote",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE "we""ird" (value INTEGER NOT NULL); INSERT INTO "we""ird" VALUES (1)`)
				require.NoError(t, err)
			},

			wantContains: `INSERT INTO "we""ird" VALUES(1);`,
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

			_, err = dbschema.Ensure(ctx, db, tmpDir)
			require.NoError(t, err)

			tc.setup(t, db)

			// Run test
			got, err := dumpSchema(ctx, db, false)

			// Assert
			require.NoError(t, err)
			require.Contains(t, got, tc.wantContains, "dump should contain the inserted row")
		})
	}
}

func TestDumpSchemaRoundTrip(t *testing.T) {
	t.Parallel()

	// Setup
	ctx := context.Background()
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	source, err := dbdriver.Open(sourceDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := source.Close()
		require.NoError(t, err)
	})

	target, err := dbdriver.Open(targetDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err := target.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, source, sourceDir)
	require.NoError(t, err)

	_, err = source.ExecContext(ctx, `INSERT INTO tokens (uuid, uses_remaining, expire_at, description, channel_id, auto_remove)
VALUES ('b32d0079-c48b-4957-b1cb-bef54125c861', 1, '2026-01-01 00:00:00+00:00', '', 1, 1)`)
	require.NoError(t, err)

	// JSON stored through a driver.Value of type []byte has the BLOB storage class.
	payload := []byte(`{"description":"x'); UPDATE tokens SET uses_remaining=1000;--"}`)

	_, err = source.ExecContext(ctx, `CREATE TABLE objects (object TEXT NOT NULL)`)
	require.NoError(t, err)

	_, err = source.ExecContext(ctx, `INSERT INTO objects VALUES (?)`, payload)
	require.NoError(t, err)

	dump, err := dumpSchema(ctx, source, false)
	require.NoError(t, err)

	// Run test
	_, err = target.ExecContext(ctx, dump)

	// Assert
	require.NoError(t, err, "replaying the dump should succeed")

	var gotObject []byte
	var gotType string
	err = target.QueryRowContext(ctx, `SELECT object, typeof(object) FROM objects`).Scan(&gotObject, &gotType)
	require.NoError(t, err)
	require.Equal(t, payload, gotObject, "object should be restored byte by byte")
	require.Equal(t, "blob", gotType, "object should keep the BLOB storage class")

	var gotUsesRemaining int
	err = target.QueryRowContext(ctx, `SELECT uses_remaining FROM tokens`).Scan(&gotUsesRemaining)
	require.NoError(t, err)
	require.Equal(t, 1, gotUsesRemaining, "payload in the object must not run as SQL")
}

func TestQuoteString(t *testing.T) {
	tests := []struct {
		name  string
		input string

		want string
	}{
		{
			name:  "plain",
			input: "plain",

			want: `'plain'`,
		},
		{
			name:  "single quote",
			input: "it's",

			want: `'it''s'`,
		},
		{
			name:  "newline",
			input: "a\nb",

			want: `replace('a\nb','\n',char(10))`,
		},
		{
			name:  "carriage return",
			input: "a\rb",

			want: `replace('a\rb','\r',char(13))`,
		},
		{
			name:  "newline and carriage return",
			input: "a\r\nb",

			want: `replace(replace('a\r\nb','\n',char(10)),'\r',char(13))`,
		},
		{
			name:  "newline and literal newline marker",
			input: `a\n` + "\n",

			want: `replace('a\n\012','\012',char(10))`,
		},
		{
			name:  "newline and both literal newline markers",
			input: `a\n\012` + "\n",

			want: `replace('a\n\012(\n0)','(\n0)',char(10))`,
		},
		{
			name:  "carriage return and literal carriage return marker",
			input: `a\r` + "\r",

			want: `replace('a\r\015','\015',char(13))`,
		},
		{
			name:  "carriage return and all literal carriage return markers",
			input: `a\r\015(\r0)` + "\r",

			want: `replace('a\r\015(\r0)(\r1)','(\r1)',char(13))`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			db, err := dbdriver.Open(t.TempDir())
			require.NoError(t, err)

			t.Cleanup(func() {
				err := db.Close()
				require.NoError(t, err)
			})

			// Run test
			got := quoteString(tc.input)

			// Assert
			require.Equal(t, tc.want, got, "literal should match the sqlite3 .dump format")

			var restored string
			err = db.QueryRowContext(t.Context(), "SELECT "+got).Scan(&restored)
			require.NoError(t, err)
			require.Equal(t, tc.input, restored, "SQLite should evaluate the literal to the input")
		})
	}
}
