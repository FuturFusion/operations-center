package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/sql/dump"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_internalHandler_sqlPost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string

		wantStatusCode int
		wantBatch      dump.SQLBatch
	}{
		{
			name:  "success - exec and select",
			query: `CREATE TABLE items (id INTEGER, name TEXT); INSERT INTO items VALUES (1, 'one'), (2, 'two'); SELECT id, name FROM items ORDER BY id`,

			wantStatusCode: http.StatusOK,
			wantBatch: dump.SQLBatch{
				Results: []dump.SQLResult{
					{Type: "exec", Columns: nil, Rows: nil, RowsAffected: 0},
					{Type: "exec", Columns: nil, Rows: nil, RowsAffected: 2},
					{Type: "select", Columns: []string{"id", "name"}, Rows: [][]any{{float64(1), "one"}, {float64(2), "two"}}, RowsAffected: 0},
				},
			},
		},
		{
			name:  "error - invalid query",
			query: `SELECT * FROM missing`,

			wantStatusCode: http.StatusInternalServerError,
			wantBatch:      dump.SQLBatch{Results: nil},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			db, err := sqlite.Open(t.TempDir())
			require.NoError(t, err)

			t.Cleanup(func() { _ = db.Close() })

			handler := &internalHandler{db: db}

			body, err := json.Marshal(dump.SQLQuery{Query: tc.query})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/internal/sql", strings.NewReader(string(body)))
			rec := httptest.NewRecorder()

			// Run test
			err = handler.sqlPost(req).Render(rec)

			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.wantStatusCode, rec.Code)

			var apiResponse api.Response
			err = json.Unmarshal(rec.Body.Bytes(), &apiResponse)
			require.NoError(t, err)

			var gotBatch dump.SQLBatch
			err = apiResponse.MetadataAsStruct(&gotBatch)
			require.NoError(t, err)

			require.Equal(t, tc.wantBatch, gotBatch, "expect one result per query")
		})
	}
}
