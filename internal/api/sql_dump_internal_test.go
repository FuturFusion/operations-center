package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func TestDumpSchema(t *testing.T) {
	t.Parallel()

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
				require.NoError(t, err, "inserting the token should succeed")
			},

			wantContains: `INSERT INTO "tokens" VALUES(1,'b32d0079-c48b-4957-b1cb-bef54125c861',1,'2026-01-01 00:00:00+00:00','',1,1);`,
		},
		{
			name: "boolean false",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), insertToken)
				require.NoError(t, err, "inserting the token should succeed")

				_, err = db.ExecContext(t.Context(), `INSERT INTO tokens_seeds (token_id, name, description, public, seeds, last_updated)
VALUES (1, 'seed', '', 0, '{}', '2026-01-01 00:00:00+00:00')`)
				require.NoError(t, err, "inserting the token seed should succeed")
			},

			wantContains: `INSERT INTO "tokens_seeds" VALUES(1,1,'seed','',0,'{}','2026-01-01 00:00:00+00:00');`,
		},
		{
			name: "real",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE reals (value REAL NOT NULL); INSERT INTO reals VALUES (1.5)`)
				require.NoError(t, err, "inserting the real should succeed")
			},

			wantContains: `INSERT INTO "reals" VALUES(1.5);`,
		},
		{
			name: "blob",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE blobs (value BLOB); INSERT INTO blobs VALUES (X'00ff27')`)
				require.NoError(t, err, "inserting the blob should succeed")
			},

			wantContains: `INSERT INTO "blobs" VALUES(X'00FF27');`,
		},
		{
			name: "blob with single quote in text column",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE objects (object TEXT NOT NULL)`)
				require.NoError(t, err, "creating the table should succeed")

				_, err = db.ExecContext(t.Context(), `INSERT INTO objects VALUES (?)`, []byte(`x');--`))
				require.NoError(t, err, "inserting the blob should succeed")
			},

			wantContains: `INSERT INTO "objects" VALUES(X'7827293B2D2D');`,
		},
		{
			name: "table name with double quote",
			setup: func(t *testing.T, db dbdriver.DBTX) {
				t.Helper()

				_, err := db.ExecContext(t.Context(), `CREATE TABLE "we""ird" (value INTEGER NOT NULL); INSERT INTO "we""ird" VALUES (1)`)
				require.NoError(t, err, "inserting into the table with a quoted name should succeed")
			},

			wantContains: `INSERT INTO "we""ird" VALUES(1);`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()

			db, err := dbdriver.Open(tmpDir)
			require.NoError(t, err, "opening the database should succeed")

			t.Cleanup(func() {
				err := db.Close()
				require.NoError(t, err, "closing the database should succeed")
			})

			_, err = dbschema.Ensure(t.Context(), db, tmpDir)
			require.NoError(t, err, "creating the schema should succeed")

			tc.setup(t, db)

			// Run test
			got, err := dumpSchema(t.Context(), db, false)

			// Assert
			require.NoError(t, err, "dumping the database should succeed")
			require.Contains(t, got, tc.wantContains, "dump should contain the inserted row")
		})
	}
}

func TestDumpSchemaRoundTrip(t *testing.T) {
	t.Parallel()

	// JSON stored through a driver.Value of type []byte has the BLOB storage class.
	jsonObject := []byte(`{"description":"x'); UPDATE tokens SET uses_remaining=1000;--"}`)

	restoreMethods := []struct {
		name    string
		restore func(t *testing.T, dump string, dir string)
	}{
		{
			name: "go driver",
			restore: func(t *testing.T, dump string, dir string) {
				t.Helper()

				db, err := dbdriver.Open(dir)
				require.NoError(t, err, "opening the target database should succeed")

				defer func() {
					err := db.Close()
					require.NoError(t, err, "closing the target database should succeed")
				}()

				_, err = db.ExecContext(t.Context(), dump)
				require.NoError(t, err, "replaying the dump with the Go driver should succeed")
			},
		},
		{
			name: "sqlite3 command line tool",
			restore: func(t *testing.T, dump string, dir string) {
				t.Helper()

				requireSQLite3(t)

				// Stop at the first error. Without -bail, sqlite3 runs the remaining statements.
				cmd := exec.CommandContext(t.Context(), "sqlite3", "-bail", filepath.Join(dir, "local.db"))
				cmd.Stdin = strings.NewReader(dump)
				output, err := cmd.CombinedOutput()
				require.NoError(t, err, "replaying the dump with sqlite3 should succeed: %s", output)
			},
		},
	}

	tests := []struct {
		name  string
		value any

		wantValue any
		wantType  string
	}{
		{
			name:  "json object with sql in blob",
			value: jsonObject,

			wantValue: jsonObject,
			wantType:  "blob",
		},
		{
			name:  "empty blob",
			value: []byte{},

			wantValue: []byte{},
			wantType:  "blob",
		},
		{
			name:  "null",
			value: nil,

			wantValue: nil,
			wantType:  "null",
		},
		{
			name:  "empty text",
			value: "",

			wantValue: "",
			wantType:  "text",
		},
		{
			name:  "line feed",
			value: "line feed\nend",

			wantValue: "line feed\nend",
			wantType:  "text",
		},
		{
			name:  "carriage return",
			value: "carriage return\rend",

			wantValue: "carriage return\rend",
			wantType:  "text",
		},
		{
			name:  "carriage return and line feed",
			value: "both\r\nend",

			wantValue: "both\r\nend",
			wantType:  "text",
		},
		{
			name:  "literal markers",
			value: `literal markers \n \012 (\n0) \r \015 (\r0)` + "\n\r",

			wantValue: `literal markers \n \012 (\n0) \r \015 (\r0)` + "\n\r",
			wantType:  "text",
		},
		{
			name:  "single quote and semicolon",
			value: "single quote ' and semicolon ;\n",

			wantValue: "single quote ' and semicolon ;\n",
			wantType:  "text",
		},
		{
			name:  "nul byte",
			value: "nul byte \x00 with ' and \n after",

			wantValue: "nul byte \x00 with ' and \n after",
			wantType:  "text",
		},
	}

	for _, method := range restoreMethods {
		t.Run(method.name, func(t *testing.T) {
			t.Parallel()

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					// Setup
					sourceDir := t.TempDir()
					targetDir := t.TempDir()

					source, err := dbdriver.Open(sourceDir)
					require.NoError(t, err, "opening the source database should succeed")

					t.Cleanup(func() {
						err := source.Close()
						require.NoError(t, err, "closing the source database should succeed")
					})

					_, err = dbschema.Ensure(t.Context(), source, sourceDir)
					require.NoError(t, err, "creating the schema should succeed")

					_, err = source.ExecContext(t.Context(), `INSERT INTO tokens (uuid, uses_remaining, expire_at, description, channel_id, auto_remove)
VALUES ('b32d0079-c48b-4957-b1cb-bef54125c861', 1, '2026-01-01 00:00:00+00:00', '', 1, 1)`)
					require.NoError(t, err, "inserting the token should succeed")

					// The column has no type, so it has no affinity. SQLite keeps the storage class of the value.
					_, err = source.ExecContext(t.Context(), `CREATE TABLE "we""ird" (value)`)
					require.NoError(t, err, "creating the table with a quoted name should succeed")

					_, err = source.ExecContext(t.Context(), `INSERT INTO "we""ird" VALUES (?)`, tc.value)
					require.NoError(t, err, "inserting the value should succeed")

					dump, err := dumpSchema(t.Context(), source, false)
					require.NoError(t, err, "dumping the source database should succeed")

					// Run test
					method.restore(t, dump, targetDir)

					// Assert
					target, err := dbdriver.Open(targetDir)
					require.NoError(t, err, "opening the target database should succeed")

					t.Cleanup(func() {
						err := target.Close()
						require.NoError(t, err, "closing the target database should succeed")
					})

					var gotValue any
					var gotType string
					err = target.QueryRowContext(t.Context(), `SELECT value, typeof(value) FROM "we""ird"`).Scan(&gotValue, &gotType)
					require.NoError(t, err, "the table with a quoted name should be restored")
					require.Equal(t, tc.wantValue, gotValue, "value should be restored byte by byte")
					require.Equal(t, tc.wantType, gotType, "value should keep its storage class")

					var gotUsesRemaining int
					err = target.QueryRowContext(t.Context(), `SELECT uses_remaining FROM tokens`).Scan(&gotUsesRemaining)
					require.NoError(t, err, "the token should be restored")
					require.Equal(t, 1, gotUsesRemaining, "the value must not run as SQL")
				})
			}
		})
	}
}

// requireSQLite3 skips the test outside of CI if sqlite3 is missing. In CI, a missing sqlite3 fails the test.
func requireSQLite3(t *testing.T) {
	t.Helper()

	_, err := exec.LookPath("sqlite3")
	if err != nil && os.Getenv("CI") == "" {
		t.Skip("sqlite3 command line tool is not installed")
	}

	require.NoError(t, err, "sqlite3 command line tool must be installed in CI")
}

func TestQuoteString(t *testing.T) {
	t.Parallel()

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
		{
			name:  "nul byte",
			input: "a\x00b",

			want: `CAST(X'610062' AS TEXT)`,
		},
		{
			name:  "nul byte with single quote and newline",
			input: "a\x00'\n",

			want: `CAST(X'6100270A' AS TEXT)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			db, err := dbdriver.Open(t.TempDir())
			require.NoError(t, err, "opening the database should succeed")

			t.Cleanup(func() {
				err := db.Close()
				require.NoError(t, err, "closing the database should succeed")
			})

			// Run test
			got := quoteString(tc.input)

			// Assert
			require.Equal(t, tc.want, got, "literal should match the expected SQL")

			var restored string
			err = db.QueryRowContext(t.Context(), "SELECT "+got).Scan(&restored)
			require.NoError(t, err, "SQLite should accept the literal")
			require.Equal(t, tc.input, restored, "SQLite should evaluate the literal to the input")
		})
	}
}
