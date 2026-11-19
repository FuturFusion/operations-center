package cowsql_test

import (
	"testing"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities/cowsql"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

func TestDetermineRaftNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		nodes   []cowsqldb.RaftNode
		address string

		wantNode *cowsqldb.RaftNode
	}{
		{
			name: "no cluster address",
			nodes: []cowsqldb.RaftNode{
				{ID: 2, Address: "10.0.0.2:8443", Name: "m2"},
			},

			wantNode: cowsqldb.DefaultRaftNode(),
		},
		{
			name:    "joining without raft nodes",
			address: "10.0.0.1:8443",

			wantNode: cowsqldb.DefaultRaftNode(),
		},
		{
			name: "matching raft node",
			nodes: []cowsqldb.RaftNode{
				{ID: 1, Address: "10.0.0.1:8443", Name: "m1"},
				{ID: 2, Address: "10.0.0.2:8443", Name: "m2"},
			},
			address: "10.0.0.2:8443",

			wantNode: &cowsqldb.RaftNode{ID: 2, Address: "10.0.0.2:8443", Name: "m2"},
		},
		{
			name: "no matching raft node",
			nodes: []cowsqldb.RaftNode{
				{ID: 1, Address: "10.0.0.1:8443", Name: "m1"},
			},
			address: "10.0.0.3:8443",

			wantNode: cowsqldb.DefaultRaftNode(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			db, err := dbdriver.Open(dir)
			require.NoError(t, err)

			t.Cleanup(func() { _ = db.Close() })

			_, err = dbschema.EnsureLocal(t.Context(), db.DB, dir, dbschema.LocalUpdates(), dbschema.LocalFreshSchema())
			require.NoError(t, err)

			tx, err := db.DB.BeginTx(t.Context(), nil)
			require.NoError(t, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			require.NoError(t, cowsql.ReplaceRaftNodes(t.Context(), tx, tc.nodes))

			node, err := cowsql.DetermineRaftNode(t.Context(), tx, tc.address)
			require.NoError(t, err)
			require.Equal(t, tc.wantNode, node)
		})
	}
}
