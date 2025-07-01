package sqlite_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/dbschema"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/ptr"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestServerDatabaseActions(t *testing.T) {
	serverA := provisioning.Server{
		Name:          "one",
		Type:          api.ServerTypeIncus,
		ConnectionURL: "https://one/",
		Certificate: `-----BEGIN CERTIFICATE-----
server A
-----END CERTIFICATE-----
`,
		HardwareData: api.HardwareData{},
		VersionData:  json.RawMessage(nil),
		Status:       api.ServerStatusReady,
	}

	serverB := provisioning.Server{
		Name:          "two",
		Type:          api.ServerTypeMigrationManager,
		ConnectionURL: "https://two/",
		Certificate: `-----BEGIN CERTIFICATE-----
server B
-----END CERTIFICATE-----
`,
		HardwareData: api.HardwareData{},
		VersionData:  json.RawMessage(nil),
		Status:       api.ServerStatusReady,
	}

	client := &adapterMock.ClusterClientPortMock{
		PingFunc: func(ctx context.Context, server provisioning.Server) error {
			return nil
		},
		EnableOSServiceLVMFunc: func(ctx context.Context, server provisioning.Server) error {
			return nil
		},
		SetServerConfigFunc: func(ctx context.Context, server provisioning.Server, config map[string]string) error {
			return nil
		},
		GetClusterNodeNamesFunc: func(ctx context.Context, server provisioning.Server) ([]string, error) {
			return []string{server.Name}, nil
		},
		GetClusterJoinTokenFunc: func(ctx context.Context, server provisioning.Server, memberName string) (string, error) {
			return "token", nil
		},
		EnableClusterFunc: func(ctx context.Context, server provisioning.Server) (string, error) {
			return "certificate", nil
		},
		JoinClusterFunc: func(ctx context.Context, server provisioning.Server, joinToken string, cluster provisioning.Server) error {
			return nil
		},
		CreateProjectFunc: func(ctx context.Context, server provisioning.Server, name string) error {
			return nil
		},
		InitializeDefaultStorageFunc: func(ctx context.Context, servers []provisioning.Server) error {
			return nil
		},
		GetOSDataFunc: func(ctx context.Context, server provisioning.Server) (api.OSData, error) {
			return api.OSData{}, nil
		},
		InitializeDefaultNetworkingFunc: func(ctx context.Context, servers []provisioning.Server, primaryNic string) error {
			return nil
		},
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
	serverSvc := provisioning.NewServerService(server, nil, nil)

	clusterSvc := provisioning.NewClusterService(sqlite.NewCluster(db), client, serverSvc, nil)

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
	serverA.LastUpdated = dbServerA.LastUpdated
	require.Equal(t, serverA, *dbServerA)

	dbServerB, err := server.GetByName(ctx, serverB.Name)
	require.NoError(t, err)
	serverB.ID = dbServerB.ID
	serverB.LastUpdated = dbServerB.LastUpdated
	require.Equal(t, serverB, *dbServerB)

	// GetByCertificate
	dbServerA, err = server.GetByCertificate(ctx, serverA.Certificate)
	require.NoError(t, err)
	require.Equal(t, serverA, *dbServerA)

	_, err = server.GetByCertificate(ctx, ``)
	require.ErrorIs(t, err, domain.ErrNotFound)

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
	serverB.LastUpdated = dbServerB.LastUpdated
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
		Name:        "one",
		ServerNames: []string{"two-new"},
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
