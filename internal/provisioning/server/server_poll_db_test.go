package server_test

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

// pollDBReadTimeout is how long a read of an unrelated server record may take
// while a poll is talking to a BMC.
const pollDBReadTimeout = 5 * time.Second

// TestServerService_PollServerDoesNotHoldTheDatabaseAcrossABMCCall asserts, that
// polling an offline server, whose BMC is slow to answer, leaves the database
// alone meanwhile.
//
// The daemon runs on a single SQLite connection, so a transaction, that spans a
// BMC call, blocks every other reader and writer, including the ones asking
// about a completely different server.
func TestServerService_PollServerDoesNotHoldTheDatabaseAcrossABMCCall(t *testing.T) {
	ctx := t.Context()

	tmpDir := t.TempDir()

	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)

	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	serverDB := sqlite.NewServer(tx)

	unresponsive := deploymentTestServer("unresponsive")
	unresponsive.Status = api.ServerStatusOffline
	unresponsive.StatusDetail = api.ServerStatusDetailOfflineUnresponsive
	unresponsive.BMCData = deploymentTestBMCData(deploymentTestOpticalMedia)

	unresponsive.ID, err = serverDB.Create(ctx, unresponsive)
	require.NoError(t, err)
	require.NoError(t, serverDB.Update(ctx, unresponsive))

	bystander := deploymentTestServer("bystander")

	_, err = serverDB.Create(ctx, bystander)
	require.NoError(t, err)

	entered := make(chan struct{})
	release := make(chan struct{})

	bmc := &adapterMock.BMCServerClientPortMock{
		GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
			close(entered)

			select {
			case <-release:
			case <-ctx.Done():
				return api.BMCData{}, ctx.Err()
			}

			return deploymentTestBMCData(deploymentTestOpticalMedia), nil
		},
	}

	serverClient := &adapterMock.ServerClientPortMock{
		PingFunc: func(ctx context.Context, endpoint provisioning.Endpoint) error {
			return domain.NewRetryableErr(boom.Error)
		},
	}

	serverSvc := provisioningServer.New(serverDB, serverClient, nil, nil, nil, nil, nil, tls.Certificate{},
		provisioningServer.WithNow(time.Now),
		provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmc),
	)

	// Run test
	polled := make(chan error, 1)

	go func() {
		polled <- serverSvc.PollServer(context.Background(), unresponsive, true)
	}()

	<-entered

	read := make(chan error, 1)

	go func() {
		_, err := serverDB.GetByName(context.Background(), bystander.Name)
		read <- err
	}()

	// Assert
	select {
	case err := <-read:
		require.NoError(t, err)

	case <-time.After(pollDBReadTimeout):
		require.FailNow(t, "reading an unrelated server blocked while the poll was talking to a BMC")
	}

	close(release)

	require.NoError(t, <-polled)
}
