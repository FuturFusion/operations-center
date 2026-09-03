package server_test

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMiddleware "github.com/FuturFusion/operations-center/internal/provisioning/adapter/middleware"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

// deploymentControlLoopTimeout keeps a test, whose control loop does not return,
// from running into the timeout of the whole package.
const deploymentControlLoopTimeout = 10 * time.Second

// blockingBMCWorld serves the BMC data of every server, blocking the ones it has
// been told to block until their context is done, the way a BMC, that accepts
// the connection and then stops answering, does.
type blockingBMCWorld struct {
	blocked  map[string]struct{}
	entered  chan string
	released chan struct{}
}

func newBlockingBMCWorld(blocked ...string) *blockingBMCWorld {
	world := &blockingBMCWorld{
		blocked:  map[string]struct{}{},
		entered:  make(chan string, 16),
		released: make(chan struct{}),
	}

	for _, name := range blocked {
		world.blocked[name] = struct{}{}
	}

	return world
}

// client returns the BMC client of the world, wrapped the way the daemon wraps
// the Redfish client, so a deadline is classified as retryable here as well.
func (w *blockingBMCWorld) client() provisioning.BMCServerClientPort {
	mock := &adapterMock.BMCServerClientPortMock{
		GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
			w.entered <- server.Name

			_, blocked := w.blocked[server.Name]
			if !blocked {
				return deploymentTestBMCData(deploymentTestOpticalMedia), nil
			}

			select {
			case <-ctx.Done():
				return api.BMCData{}, ctx.Err()

			case <-w.released:
				return deploymentTestBMCData(deploymentTestOpticalMedia), nil
			}
		},

		// The BIOS check is the step following the refresh, kept from succeeding,
		// so a deployment, that got past the refresh, comes to rest right after
		// it instead of running the whole state machine.
		BIOSAttributesFunc: func(ctx context.Context, server provisioning.Server) ([]api.BIOSAttribute, error) {
			return nil, domain.NewRetryableErr(boom.Error)
		},
	}

	return adapterMiddleware.NewBMCServerClientPortWithErrorWrapper(mock, domain.RetryableWrapper())
}

func deploymentBlockedTestServer(name string) provisioning.Server {
	server := deploymentTestServer(name)
	server.Status = api.ServerStatusDeploying
	server.StatusDetail = api.ServerStatusDetailDeployingPreparing
	server.StatusInternal.Deployment = &provisioning.ServerDeployment{
		State:          api.ServerDeploymentStateRefreshBMCData,
		StartedAt:      deploymentTestDate,
		StateEnteredAt: deploymentTestDate,
	}

	return server
}

func deploymentBlockedTestRepo(store *deploymentServerStore) *repoMock.ServerRepoMock {
	return &repoMock.ServerRepoMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
			return store.get(name)
		},
		GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
			var matching provisioning.Servers

			for _, server := range store.all() {
				if ptr.From(filter.Status) != server.Status {
					continue
				}

				matching = append(matching, server)
			}

			return matching, nil
		},
		UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
			store.put(in)

			return nil
		},
	}
}

// TestServerService_DeploymentControlLoopAdvancesServersConcurrently asserts,
// that a BMC, which is not answering, only holds up the deployment of its own
// server. It is the regression test for two concurrent deployments freezing each
// other.
func TestServerService_DeploymentControlLoopAdvancesServersConcurrently(t *testing.T) {
	store := newDeploymentServerStore(
		deploymentBlockedTestServer("blocked"),
		deploymentBlockedTestServer("responsive"),
	)

	world := newBlockingBMCWorld("blocked")

	serverSvc := provisioningServer.New(deploymentBlockedTestRepo(store), nil, nil, nil, nil, nil, nil, tls.Certificate{},
		provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
		provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, world.client()),
	)

	// Run test
	done := make(chan error, 1)

	go func() {
		done <- serverSvc.DeploymentControlLoop(context.Background(), nil)
	}()

	// Assert
	require.Eventually(t, func() bool {
		responsive, err := store.get("responsive")
		require.NoError(t, err)

		return responsive.StatusInternal.Deployment.State != api.ServerDeploymentStateRefreshBMCData
	}, deploymentControlLoopTimeout, 10*time.Millisecond, "the responsive server advances while the BMC of the other one is not answering")

	blocked, err := store.get("blocked")
	require.NoError(t, err)
	require.Equal(t, api.ServerDeploymentStateRefreshBMCData, blocked.StatusInternal.Deployment.State, "the blocked server is still waiting for its BMC")

	close(world.released)

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(deploymentControlLoopTimeout):
		require.FailNow(t, "the control loop did not return")
	}
}

// TestServerService_DeploymentControlLoopBoundsAStepOfAnUnresponsiveBMC asserts,
// that a BMC, which never answers, ends the attempt of the step instead of
// parking the control loop.
func TestServerService_DeploymentControlLoopBoundsAStepOfAnUnresponsiveBMC(t *testing.T) {
	store := newDeploymentServerStore(deploymentBlockedTestServer("blocked"))

	world := newBlockingBMCWorld("blocked")

	serverSvc := provisioningServer.New(deploymentBlockedTestRepo(store), nil, nil, nil, nil, nil, nil, tls.Certificate{},
		provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
		provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, world.client()),
	)

	// Run test
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()

	err := serverSvc.DeploymentControlLoop(ctx, nil)

	// Assert
	require.NoError(t, err, "a step, that ran out of time, is recorded on the deployment rather than failing the loop")

	blocked, err := store.get("blocked")
	require.NoError(t, err)

	deployment := blocked.StatusInternal.Deployment
	require.Equal(t, api.ServerDeploymentStateRefreshBMCData, deployment.State, "the state is kept, so the action is issued again")
	require.Equal(t, 1, deployment.Retries, "the attempt, that ran out of time, spent a retry")
	require.Contains(t, deployment.LastError, context.DeadlineExceeded.Error())
}

// TestServerService_CancelDeploymentInterruptsTheStepInFlight asserts, that a
// cancellation does not have to wait out a BMC, which is not answering.
func TestServerService_CancelDeploymentInterruptsTheStepInFlight(t *testing.T) {
	store := newDeploymentServerStore(deploymentBlockedTestServer("blocked"))

	world := newBlockingBMCWorld("blocked")

	serverSvc := provisioningServer.New(deploymentBlockedTestRepo(store), nil, nil, nil, nil, nil, nil, tls.Certificate{},
		provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
		provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, world.client()),
	)

	done := make(chan error, 1)

	go func() {
		done <- serverSvc.DeploymentControlLoop(context.Background(), nil)
	}()

	<-world.entered

	// Run test
	err := serverSvc.CancelDeploymentByName(t.Context(), "blocked")

	// Assert
	require.NoError(t, err)

	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(deploymentControlLoopTimeout):
		require.FailNow(t, "the cancellation did not interrupt the step in flight")
	}

	blocked, err := store.get("blocked")
	require.NoError(t, err)
	require.True(t, blocked.StatusInternal.Deployment.CancelRequested)

	close(world.released)
}
