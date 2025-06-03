package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	incusapi "github.com/lxc/incus/v6/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/ptr"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerDatabaseActions(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC).Truncate(0) // Truncate to remove the monotonic clock.

	serverA := provisioning.Server{
		Name:          "one",
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		Certificate: `-----BEGIN CERTIFICATE-----
server A
-----END CERTIFICATE-----
`,
		HardwareData: incusapi.Resources{},
		VersionData:  json.RawMessage(nil),
		Status:       api.ServerStatusReady,
		LastUpdated:  fixedDate,
	}

	serverB := provisioning.Server{
		Name:          "two",
		Type:          api.ServerTypeMigrationManager,
		ConnectionURL: "https://two/",
		Certificate: `-----BEGIN CERTIFICATE-----
server B
-----END CERTIFICATE-----
`,
		HardwareData: incusapi.Resources{},
		VersionData:  json.RawMessage(nil),
		Status:       api.ServerStatusReady,
		LastUpdated:  fixedDate,
	}

	ctx := context.Background()

	// Create a new temporary database.
	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	server := sqlite.NewServer(tx)
	serverSvc := provisioning.NewServerService(server, nil, nil,
		provisioning.ServerServiceWithNow(func() time.Time { return fixedDate }),
	)

	clusterSvc := provisioning.NewClusterService(sqlite.NewCluster(db), serverSvc, nil, provisioning.ClusterServiceWithNow(func() time.Time { return fixedDate }))

	// Add server
	_, err = server.Create(ctx, serverA)
	require.NoError(t, err)
	_, err = server.Create(ctx, serverB)
	require.NoError(t, err)

	// Ensure we have two entries
	servers, err := server.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, servers, 2)

	serverIDs, err := server.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, serverIDs, 2)
	require.ElementsMatch(t, []string{"one", "two"}, serverIDs)

	// Should get back serverA unchanged.
	dbServerA, err := server.GetByName(ctx, serverA.Name)
	require.NoError(t, err)
	serverA.ID = dbServerA.ID
	require.Equal(t, serverA, *dbServerA)

	dbServerB, err := server.GetByName(ctx, serverB.Name)
	require.NoError(t, err)
	serverB.ID = dbServerB.ID
	require.Equal(t, serverB, *dbServerB)

	// Test updating a server.
	serverB.ConnectionURL = "https://two-new/"
	err = server.Update(ctx, serverB)
	require.NoError(t, err)
	serverB.Name = "two-new"
	err = server.Rename(ctx, "two", serverB.Name)
	require.NoError(t, err)
	dbServerB, err = server.GetByName(ctx, serverB.Name)
	require.NoError(t, err)
	serverB.ID = dbServerB.ID
	require.Equal(t, serverB, *dbServerB)

	// Delete a server.
	err = server.DeleteByName(ctx, serverA.Name)
	require.NoError(t, err)
	_, err = server.GetByName(ctx, serverA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have two servers remaining.
	servers, err = server.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, servers, 1)

	// Can't delete a server that doesn't exist.
	err = server.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a server that doesn't exist.
	err = server.Update(ctx, serverA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate a server.
	_, err = server.Create(ctx, serverB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Add server one to a cluster
	_, err = clusterSvc.Create(ctx, provisioning.Cluster{
		Name:          "one",
		ConnectionURL: "https://one/",
		ServerNames:   []string{"two-new"},
	})
	require.NoError(t, err)

	// Get all with filter
	servers, err = server.GetAllWithFilter(ctx, provisioning.ServerFilter{
		Cluster: ptr.To("one"),
	})
	require.NoError(t, err)
	require.Len(t, servers, 1)

	// Get all names with filter
	serverIDs, err = server.GetAllNamesWithFilter(ctx, provisioning.ServerFilter{
		Cluster: ptr.To("one"),
	})
	require.NoError(t, err)
	require.Len(t, serverIDs, 1)
	require.ElementsMatch(t, []string{"two-new"}, serverIDs)

	// Ensure deletion of cluster fails if a linked server is present.
	err = clusterSvc.DeleteByName(ctx, "one")
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
