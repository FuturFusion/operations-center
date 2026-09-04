package server_test

import (
	"context"
	"crypto/tls"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	provisioningServer "github.com/FuturFusion/operations-center/internal/provisioning/server"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/uuidgen"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

const deploymentTestOperationsCenterAddress = "https://192.168.1.200:8443"

var deploymentTestDate = time.Date(2026, 3, 12, 10, 57, 43, 0, time.UTC)

// deploymentServerStore is an in memory stand in for the server repository, so a
// read modify write cycle of the deployment observes what the cycle before it
// wrote. Errors are injected through the queues, not through the store.
type deploymentServerStore struct {
	mu      sync.Mutex
	servers map[string]provisioning.Server
}

func newDeploymentServerStore(servers ...provisioning.Server) *deploymentServerStore {
	store := &deploymentServerStore{servers: map[string]provisioning.Server{}}
	for _, server := range servers {
		store.servers[server.Name] = cloneDeploymentServer(server)
	}

	return store
}

// cloneDeploymentServer detaches the deployment record from the caller, the way
// a repository writing it out as JSON does. Server.Clone is not used, since it
// drops everything following a field, that does not survive its own text
// unmarshaler, which the fixtures of the error cases deliberately produce.
func cloneDeploymentServer(server provisioning.Server) provisioning.Server {
	deployment := server.StatusInternal.Deployment
	if deployment == nil {
		return server
	}

	clone := *deployment
	clone.BIOSAttributes = maps.Clone(deployment.BIOSAttributes)
	clone.BIOSDeferredAttributes = maps.Clone(deployment.BIOSDeferredAttributes)
	clone.History = slices.Clone(deployment.History)

	server.StatusInternal.Deployment = &clone

	return server
}

func (s *deploymentServerStore) get(name string) (*provisioning.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	server, ok := s.servers[name]
	if !ok {
		return nil, domain.ErrNotFound
	}

	return new(cloneDeploymentServer(server)), nil
}

func (s *deploymentServerStore) put(server provisioning.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.servers[server.Name] = cloneDeploymentServer(server)
}

func (s *deploymentServerStore) all() provisioning.Servers {
	s.mu.Lock()
	defer s.mu.Unlock()

	servers := make(provisioning.Servers, 0, len(s.servers))
	for _, name := range slices.Sorted(maps.Keys(s.servers)) {
		servers = append(servers, cloneDeploymentServer(s.servers[name]))
	}

	return servers
}

func deploymentTestServer(name string) provisioning.Server {
	return provisioning.Server{
		Name:         name,
		Status:       api.ServerStatusUnregistered,
		StatusDetail: api.ServerStatusDetailNone,
		Channel:      "stable",
		BMCConfig: api.BMCConfig{
			APIType:  api.BMCAPITypeRedfishV1Generic,
			Endpoint: "https://bmc.local:8443",
			Username: "admin",
			Password: "secret",
		},
	}
}

func deploymentTestBMCData(virtualMedia ...api.BMCVirtualMedia) api.BMCData {
	data := api.BMCData{
		ServerPowerState: "On",
		VirtualMedia:     map[string]api.BMCVirtualMedia{},
	}

	for _, media := range virtualMedia {
		data.VirtualMedia[media.ID] = media
	}

	return data
}

var (
	deploymentTestOpticalMedia = api.BMCVirtualMedia{ID: "system:1", MediaTypes: []string{"CD", "DVD"}}
	deploymentTestUSBMedia     = api.BMCVirtualMedia{ID: "manager:1", MediaTypes: []string{"USBStick"}}
)

func TestServerService_DeployByName(t *testing.T) {
	tokenUUID := uuidgen.FromPattern(t, "1")

	validRequest := provisioning.ServerDeploymentRequest{
		TokenUUID:    tokenUUID,
		Seed:         "default",
		ImageType:    api.ImageTypeISO,
		Architecture: images.UpdateFileArchitecture64BitX86,
	}

	validToken := &provisioning.Token{
		UUID:          tokenUUID,
		UsesRemaining: 1,
		ExpireAt:      deploymentTestDate.Add(24 * time.Hour),
	}

	validSeed := &provisioning.TokenSeed{
		Token:  tokenUUID,
		Name:   "default",
		Public: true,
		Seeds: provisioning.TokenImageSeedConfigs{
			Install: api.SeedInstall{ForceReboot: true},
		},
	}

	validResolution := &provisioning.BIOSProfileResolution{
		Profiles:           []string{"generic"},
		Attributes:         map[string]any{"BootMode": "Uefi"},
		DeferredAttributes: map[string]any{"SecureBoot": "Enabled"},
	}

	tests := []struct {
		name                    string
		nameArg                 string
		requestArg              provisioning.ServerDeploymentRequest
		operationsCenterAddress string
		server                  *provisioning.Server
		repoGetByNameErrs       queue.Errs
		repoUpdateErrs          queue.Errs
		bmcGetData              api.BMCData
		bmcGetDataErr           error
		tokenSvcGetByUUID       *provisioning.Token
		tokenSvcGetByUUIDErr    error
		tokenSvcGetSeed         *provisioning.TokenSeed
		tokenSvcGetSeedErr      error
		channelSvcGetByNameErr  error
		withBIOSProfilePort     bool
		biosProfileResolve      *provisioning.BIOSProfileResolution
		biosProfileResolveErr   error

		wantDeployment *provisioning.ServerDeployment
		wantStatus     api.ServerStatus
		assertErr      require.ErrorAssertionFunc
	}{
		{
			name:                    "success",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestUSBMedia, deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			withBIOSProfilePort:     true,
			biosProfileResolve:      validResolution,

			wantDeployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateRefreshBMCData,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID:      tokenUUID,
					Seed:           "default",
					ImageType:      api.ImageTypeISO,
					Architecture:   images.UpdateFileArchitecture64BitX86,
					VirtualMediaID: "system:1",
				},
				ForceReboot:            true,
				BIOSProfiles:           []string{"generic"},
				BIOSAttributes:         map[string]any{"BootMode": "Uefi"},
				BIOSDeferredAttributes: map[string]any{"SecureBoot": "Enabled"},
				BIOSPending:            true,
				BIOSDeferredPending:    true,
				MediaBytesRead:         -1,
				StartedAt:              deploymentTestDate,
				StateEnteredAt:         deploymentTestDate,
				History:                []api.ServerDeploymentStep{},
			},
			wantStatus: api.ServerStatusDeploying,
			assertErr:  require.NoError,
		},
		{
			name:                    "success - the BIOS profile resolves to nothing to apply",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			withBIOSProfilePort:     true,
			biosProfileResolve:      &provisioning.BIOSProfileResolution{Profiles: []string{"generic"}},

			wantDeployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateRefreshBMCData,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID:      tokenUUID,
					Seed:           "default",
					ImageType:      api.ImageTypeISO,
					Architecture:   images.UpdateFileArchitecture64BitX86,
					VirtualMediaID: "system:1",
				},
				ForceReboot:    true,
				BIOSProfiles:   []string{"generic"},
				MediaBytesRead: -1,
				StartedAt:      deploymentTestDate,
				StateEnteredAt: deploymentTestDate,
				History:        []api.ServerDeploymentStep{},
			},
			wantStatus: api.ServerStatusDeploying,
			assertErr:  require.NoError,
		},
		{
			name:    "success - an explicitly requested virtual media device",
			nameArg: "one",
			requestArg: provisioning.ServerDeploymentRequest{
				TokenUUID:      tokenUUID,
				Seed:           "default",
				ImageType:      api.ImageTypeISO,
				Architecture:   images.UpdateFileArchitecture64BitX86,
				VirtualMediaID: "manager:1",
			},
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestUSBMedia, deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			withBIOSProfilePort:     true,
			biosProfileResolve:      &provisioning.BIOSProfileResolution{},

			wantDeployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateRefreshBMCData,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID:      tokenUUID,
					Seed:           "default",
					ImageType:      api.ImageTypeISO,
					Architecture:   images.UpdateFileArchitecture64BitX86,
					VirtualMediaID: "manager:1",
				},
				ForceReboot:    true,
				MediaBytesRead: -1,
				StartedAt:      deploymentTestDate,
				StateEnteredAt: deploymentTestDate,
				History:        []api.ServerDeploymentStep{},
			},
			wantStatus: api.ServerStatusDeploying,
			assertErr:  require.NoError,
		},
		{
			name:                    "success - the first matching virtual media device is selected",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData: deploymentTestBMCData(
				api.BMCVirtualMedia{ID: "manager:1", MediaTypes: []string{"CD"}},
				api.BMCVirtualMedia{ID: "system:1", MediaTypes: []string{"CD"}},
				api.BMCVirtualMedia{ID: "system:2", MediaTypes: []string{"DVD"}},
			),
			tokenSvcGetByUUID:   validToken,
			tokenSvcGetSeed:     validSeed,
			withBIOSProfilePort: true,
			biosProfileResolve:  &provisioning.BIOSProfileResolution{},

			wantDeployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateRefreshBMCData,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID:      tokenUUID,
					Seed:           "default",
					ImageType:      api.ImageTypeISO,
					Architecture:   images.UpdateFileArchitecture64BitX86,
					VirtualMediaID: "system:1",
				},
				ForceReboot:    true,
				MediaBytesRead: -1,
				StartedAt:      deploymentTestDate,
				StateEnteredAt: deploymentTestDate,
				History:        []api.ServerDeploymentStep{},
			},
			wantStatus: api.ServerStatusDeploying,
			assertErr:  require.NoError,
		},
		{
			name:                    "success - a seed without force reboot deployed with force",
			nameArg:                 "one",
			requestArg:              provisioning.ServerDeploymentRequest{TokenUUID: tokenUUID, Seed: "default", ImageType: api.ImageTypeISO, Architecture: images.UpdateFileArchitecture64BitX86, Force: true},
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Token: tokenUUID, Name: "default", Public: true},
			withBIOSProfilePort:     true,
			biosProfileResolve:      &provisioning.BIOSProfileResolution{},

			wantDeployment: &provisioning.ServerDeployment{
				State: api.ServerDeploymentStateRefreshBMCData,
				Request: provisioning.ServerDeploymentRequest{
					TokenUUID:      tokenUUID,
					Seed:           "default",
					ImageType:      api.ImageTypeISO,
					Architecture:   images.UpdateFileArchitecture64BitX86,
					VirtualMediaID: "system:1",
					Force:          true,
				},
				MediaBytesRead: -1,
				StartedAt:      deploymentTestDate,
				StateEnteredAt: deploymentTestDate,
				History:        []api.ServerDeploymentStep{},
			},
			wantStatus: api.ServerStatusDeploying,
			assertErr:  require.NoError,
		},
		{
			name:       "error - empty name",
			nameArg:    "",
			requestArg: validRequest,

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:       "error - request.Validate",
			nameArg:    "one",
			requestArg: provisioning.ServerDeploymentRequest{},

			assertErr: errassert.ValidationErrorContains("token UUID can not be empty"),
		},
		{
			name:       "error - operations center address is not configured",
			nameArg:    "one",
			requestArg: validRequest,

			assertErr: errassert.OperationNotPermittedErrorContains("Operations Center address is not configured"),
		},
		{
			name:                    "error - repo.GetByName",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			repoGetByNameErrs:       queue.Errs{boom.Error},

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - server has no BMC configured",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server: func() *provisioning.Server {
				server := deploymentTestServer("one")
				server.BMCConfig = api.BMCConfig{APIType: api.BMCAPITypeNone}

				return &server
			}(),

			assertErr: errassert.OperationNotPermittedErrorContains("has no BMC configured"),
		},
		{
			name:                    "error - bmcClient.GetData",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetDataErr:           boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - repo.GetByName in the transaction",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			repoGetByNameErrs:       queue.Errs{nil, nil, boom.Error},

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - server is not unregistered",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server: func() *provisioning.Server {
				server := deploymentTestServer("one")
				server.Status = api.ServerStatusReady

				return &server
			}(),
			bmcGetData: deploymentTestBMCData(deploymentTestOpticalMedia),

			assertErr: errassert.OperationNotPermittedErrorContains("only an unregistered server can be deployed"),
		},
		{
			name:                    "error - server is already being deployed",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server: func() *provisioning.Server {
				server := deploymentTestServer("one")
				server.StatusInternal.Deployment = &provisioning.ServerDeployment{State: api.ServerDeploymentStateAttachMedia}

				return &server
			}(),
			bmcGetData: deploymentTestBMCData(deploymentTestOpticalMedia),

			assertErr: errassert.OperationNotPermittedErrorContains("is already being deployed"),
		},
		{
			name:                    "error - tokenSvc.GetByUUID",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUIDErr:    boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - token has no uses remaining",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       &provisioning.Token{UUID: tokenUUID, ExpireAt: deploymentTestDate.Add(24 * time.Hour)},

			assertErr: errassert.OperationNotPermittedErrorContains("has no uses remaining"),
		},
		{
			name:                    "error - token expires before the deployment could complete",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID: &provisioning.Token{
				UUID:          tokenUUID,
				UsesRemaining: 1,
				ExpireAt:      deploymentTestDate.Add(config.ServerDeploymentTimeout - time.Minute),
			},

			assertErr: errassert.OperationNotPermittedErrorContains("does not cover the deployment timeout"),
		},
		{
			name:                    "error - tokenSvc.GetTokenSeedByName",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeedErr:      boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - token seed is not public",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Token: tokenUUID, Name: "default"},

			assertErr: errassert.OperationNotPermittedErrorContains("must be public"),
		},
		{
			name:                    "error - token seed does not force a reboot",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         &provisioning.TokenSeed{Token: tokenUUID, Name: "default", Public: true},

			assertErr: errassert.OperationNotPermittedErrorContains(`does not set "force_reboot"`),
		},
		{
			name:                    "error - channelSvc.GetByName",
			nameArg:                 "one",
			requestArg:              provisioning.ServerDeploymentRequest{TokenUUID: tokenUUID, Seed: "default", ImageType: api.ImageTypeISO, Architecture: images.UpdateFileArchitecture64BitX86, Channel: "daily"},
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			channelSvcGetByNameErr:  boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - the BMC reports no virtual media device",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,

			assertErr: errassert.NotFoundErrorContains("reports no virtual media device"),
		},
		{
			name:                    "error - the requested virtual media device does not exist",
			nameArg:                 "one",
			requestArg:              provisioning.ServerDeploymentRequest{TokenUUID: tokenUUID, Seed: "default", ImageType: api.ImageTypeISO, Architecture: images.UpdateFileArchitecture64BitX86, VirtualMediaID: "system:9"},
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,

			assertErr: errassert.OperationNotPermittedErrorContains(`has no virtual media device "system:9", the BMC reports system:1`),
		},
		{
			name:                    "error - no source of BIOS profiles is configured",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,

			assertErr: errassert.NotFoundErrorContains("No source of BIOS profiles is configured"),
		},
		{
			name:                    "error - biosProfile.Resolve",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			withBIOSProfilePort:     true,
			biosProfileResolveErr:   boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                    "error - no BIOS profile matches the server",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			withBIOSProfilePort:     true,

			assertErr: errassert.NotFoundErrorContains("No BIOS profile matches server"),
		},
		{
			name:                    "error - server.Validate",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server: func() *provisioning.Server {
				server := deploymentTestServer("one")
				server.Channel = ""

				return &server
			}(),
			bmcGetData:          deploymentTestBMCData(deploymentTestOpticalMedia),
			tokenSvcGetByUUID:   validToken,
			tokenSvcGetSeed:     validSeed,
			withBIOSProfilePort: true,
			biosProfileResolve:  validResolution,

			assertErr: errassert.ValidationErrorContains("channel can not be empty"),
		},
		{
			name:                    "error - repo.Update",
			nameArg:                 "one",
			requestArg:              validRequest,
			operationsCenterAddress: deploymentTestOperationsCenterAddress,
			server:                  new(deploymentTestServer("one")),
			bmcGetData:              deploymentTestBMCData(deploymentTestOpticalMedia),
			repoUpdateErrs:          queue.Errs{nil, boom.Error},
			tokenSvcGetByUUID:       validToken,
			tokenSvcGetSeed:         validSeed,
			withBIOSProfilePort:     true,
			biosProfileResolve:      validResolution,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config.InitTest(t, &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool { return false },
			}, nil)

			if tc.operationsCenterAddress != "" {
				err := config.UpdateNetwork(t.Context(), system.NetworkPut{
					OperationsCenterAddress: tc.operationsCenterAddress,
					RestServerAddress:       "[::]:8443",
				})
				require.NoError(t, err)
			}

			// Setup
			var servers []provisioning.Server
			if tc.server != nil {
				servers = append(servers, *tc.server)
			}

			store := newDeploymentServerStore(servers...)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					err := tc.repoGetByNameErrs.PopOrNil(t)
					if err != nil {
						return nil, err
					}

					return store.get(name)
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					err := tc.repoUpdateErrs.PopOrNil(t)
					if err != nil {
						return err
					}

					store.put(in)

					return nil
				},
			}

			bmcClient := &adapterMock.BMCServerClientPortMock{
				GetDataFunc: func(ctx context.Context, server provisioning.Server) (api.BMCData, error) {
					return tc.bmcGetData, tc.bmcGetDataErr
				},
			}

			tokenSvc := &svcMock.TokenServiceMock{
				GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
					require.Equal(t, tc.requestArg.TokenUUID, id)

					return tc.tokenSvcGetByUUID, tc.tokenSvcGetByUUIDErr
				},
				GetTokenSeedByNameFunc: func(ctx context.Context, id uuid.UUID, name string) (*provisioning.TokenSeed, error) {
					require.Equal(t, tc.requestArg.Seed, name)

					return tc.tokenSvcGetSeed, tc.tokenSvcGetSeedErr
				},
			}

			channelSvc := &svcMock.ChannelServiceMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Channel, error) {
					return &provisioning.Channel{}, tc.channelSvcGetByNameErr
				},
			}

			opts := []provisioningServer.Option{
				provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
				provisioningServer.AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
			}

			if tc.withBIOSProfilePort {
				opts = append(opts, provisioningServer.WithBIOSProfilePort(&adapterMock.BIOSProfilePortMock{
					ResolveFunc: func(ctx context.Context, server provisioning.Server) (*provisioning.BIOSProfileResolution, error) {
						return tc.biosProfileResolve, tc.biosProfileResolveErr
					},
				}))
			}

			serverSvc := provisioningServer.New(repo, nil, nil, tokenSvc, nil, channelSvc, nil, tls.Certificate{}, opts...)

			// Run test
			err := serverSvc.DeployByName(t.Context(), tc.nameArg, tc.requestArg)

			// Assert
			tc.assertErr(t, err)

			if tc.wantDeployment == nil {
				if tc.server != nil {
					stored, storeErr := store.get(tc.server.Name)
					require.NoError(t, storeErr)
					require.Equal(t, tc.server.StatusInternal.Deployment, stored.StatusInternal.Deployment, "a rejected request leaves the deployment record untouched")
				}

				return
			}

			stored, err := store.get(tc.nameArg)
			require.NoError(t, err)
			require.Equal(t, tc.wantDeployment, stored.StatusInternal.Deployment)
			require.Equal(t, tc.wantStatus, stored.Status)
			require.Equal(t, api.ServerStatusDetailDeployingPreparing, stored.StatusDetail)
			require.Equal(t, deploymentTestDate, stored.LastStatusUpdated)
		})
	}
}

func TestServerService_CancelDeploymentByName(t *testing.T) {
	activeServer := func() provisioning.Server {
		server := deploymentTestServer("one")
		server.Status = api.ServerStatusDeploying
		server.StatusInternal.Deployment = &provisioning.ServerDeployment{State: api.ServerDeploymentStateWaitInstall}

		return server
	}

	tests := []struct {
		name              string
		nameArg           string
		server            *provisioning.Server
		repoGetByNameErrs queue.Errs
		repoUpdateErrs    queue.Errs

		wantCancelRequested bool
		wantUpdateCount     int
		assertErr           require.ErrorAssertionFunc
	}{
		{
			name:    "success",
			nameArg: "one",
			server:  new(activeServer()),

			wantCancelRequested: true,
			wantUpdateCount:     1,
			assertErr:           require.NoError,
		},
		{
			name:    "success - the cancellation is already requested",
			nameArg: "one",
			server: func() *provisioning.Server {
				server := activeServer()
				server.StatusInternal.Deployment.CancelRequested = true

				return &server
			}(),

			wantCancelRequested: true,
			wantUpdateCount:     0,
			assertErr:           require.NoError,
		},
		{
			name:    "error - empty name",
			nameArg: "",

			assertErr: errassert.OperationNotPermittedError,
		},
		{
			name:              "error - repo.GetByName",
			nameArg:           "one",
			server:            new(activeServer()),
			repoGetByNameErrs: queue.Errs{boom.Error},

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - the server has no deployment at all",
			nameArg: "one",
			server:  new(deploymentTestServer("one")),

			assertErr: errassert.NotFoundErrorContains("has no deployment in progress"),
		},
		{
			name:    "error - the deployment has already finished",
			nameArg: "one",
			server: func() *provisioning.Server {
				server := activeServer()
				server.StatusInternal.Deployment.State = api.ServerDeploymentStateCompleted

				return &server
			}(),

			assertErr: errassert.NotFoundErrorContains("has no deployment in progress"),
		},
		{
			name:           "error - repo.Update",
			nameArg:        "one",
			server:         new(activeServer()),
			repoUpdateErrs: queue.Errs{boom.Error},

			wantUpdateCount: 1,
			assertErr:       boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			var servers []provisioning.Server
			if tc.server != nil {
				servers = append(servers, *tc.server)
			}

			store := newDeploymentServerStore(servers...)

			updateCount := 0

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					err := tc.repoGetByNameErrs.PopOrNil(t)
					if err != nil {
						return nil, err
					}

					return store.get(name)
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					updateCount++

					err := tc.repoUpdateErrs.PopOrNil(t)
					if err != nil {
						return err
					}

					store.put(in)

					return nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
			)

			// Run test
			err := serverSvc.CancelDeploymentByName(t.Context(), tc.nameArg)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantUpdateCount, updateCount, "the number of writes of the cancellation")

			if tc.server == nil {
				return
			}

			stored, storeErr := store.get(tc.server.Name)
			require.NoError(t, storeErr)

			if stored.StatusInternal.Deployment == nil {
				require.False(t, tc.wantCancelRequested)
				return
			}

			require.Equal(t, tc.wantCancelRequested, stored.StatusInternal.Deployment.CancelRequested)
		})
	}
}

func TestServerService_DeploymentControlLoopCandidates(t *testing.T) {
	deploying := func(name string) provisioning.Server {
		server := deploymentTestServer(name)
		server.Status = api.ServerStatusDeploying
		server.StatusInternal.Deployment = &provisioning.ServerDeployment{
			State:          api.ServerDeploymentStateWaitRegistration,
			StartedAt:      deploymentTestDate,
			StateEnteredAt: deploymentTestDate,
		}

		return server
	}

	registering := func(name string) provisioning.Server {
		server := deploying(name)
		server.Status = api.ServerStatusPending
		server.StatusDetail = api.ServerStatusDetailPendingRegistering

		return server
	}

	tests := []struct {
		name              string
		serverNameFilter  *string
		servers           []provisioning.Server
		repoGetByNameErrs queue.Errs
		repoGetAllErrs    queue.Errs

		wantAdvanced []string
		assertErr    require.ErrorAssertionFunc
	}{
		{
			name:    "success - no candidate at all",
			servers: nil,

			assertErr: require.NoError,
		},
		{
			name:    "success - a deploying and a registering server",
			servers: []provisioning.Server{deploying("one"), registering("two")},

			wantAdvanced: []string{"one", "two"},
			assertErr:    require.NoError,
		},
		{
			name: "success - a server without an active deployment is left alone",
			servers: []provisioning.Server{
				deploying("one"),
				func() provisioning.Server {
					server := deploying("two")
					server.StatusInternal.Deployment.State = api.ServerDeploymentStateCompleted

					return server
				}(),
			},

			wantAdvanced: []string{"one"},
			assertErr:    require.NoError,
		},
		{
			name:             "success - the filtered server",
			serverNameFilter: new("one"),
			servers:          []provisioning.Server{deploying("one"), deploying("two")},

			wantAdvanced: []string{"one"},
			assertErr:    require.NoError,
		},
		{
			name:             "success - the filtered server does not exist",
			serverNameFilter: new("nine"),
			servers:          []provisioning.Server{deploying("one")},

			assertErr: require.NoError,
		},
		{
			name:             "success - the filtered server has no active deployment",
			serverNameFilter: new("one"),
			servers:          []provisioning.Server{deploymentTestServer("one")},

			assertErr: require.NoError,
		},
		{
			name:              "error - repo.GetByName for the filtered server",
			serverNameFilter:  new("one"),
			servers:           []provisioning.Server{deploying("one")},
			repoGetByNameErrs: queue.Errs{boom.Error},

			assertErr: boom.ErrorIs,
		},
		{
			name:           "error - repo.GetAllWithFilter",
			servers:        []provisioning.Server{deploying("one")},
			repoGetAllErrs: queue.Errs{boom.Error},

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			store := newDeploymentServerStore(tc.servers...)

			advanced := map[string]struct{}{}

			var recordMu sync.Mutex

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					recordMu.Lock()
					defer recordMu.Unlock()

					err := tc.repoGetByNameErrs.PopOrNil(t)
					if err != nil {
						return nil, err
					}

					server, err := store.get(name)
					if err != nil {
						return nil, err
					}

					if server.StatusInternal.Deployment.IsActive() {
						advanced[name] = struct{}{}
					}

					return server, nil
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					recordMu.Lock()
					defer recordMu.Unlock()

					err := tc.repoGetAllErrs.PopOrNil(t)
					if err != nil {
						return nil, err
					}

					var matching provisioning.Servers

					for _, server := range store.all() {
						if ptr.From(filter.Status) != server.Status {
							continue
						}

						if filter.StatusDetail != nil && ptr.From(filter.StatusDetail) != server.StatusDetail {
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

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
			)

			// Run test
			err := serverSvc.DeploymentControlLoop(t.Context(), tc.serverNameFilter)

			// Assert
			tc.assertErr(t, err)

			recordMu.Lock()
			defer recordMu.Unlock()

			require.Equal(t, tc.wantAdvanced, slices.Sorted(maps.Keys(advanced)), "the servers the control loop stepped")
		})
	}
}

func TestServerService_DeploymentControlLoopFailsATerminallyBrokenDeployment(t *testing.T) {
	tests := []struct {
		name       string
		deployment *provisioning.ServerDeployment

		wantFailedState api.ServerDeploymentState
		wantLastError   string
	}{
		{
			name: "an unknown state",
			deployment: &provisioning.ServerDeployment{
				State:          api.ServerDeploymentState("bogus"),
				StartedAt:      deploymentTestDate,
				StateEnteredAt: deploymentTestDate,
			},

			wantFailedState: api.ServerDeploymentState("bogus"),
			wantLastError:   `Deployment is in the unknown state "bogus"`,
		},
		{
			name: "the deployment as a whole timed out",
			deployment: &provisioning.ServerDeployment{
				State:          api.ServerDeploymentStateWaitInstall,
				StartedAt:      deploymentTestDate.Add(-config.ServerDeploymentTimeout - time.Second),
				StateEnteredAt: deploymentTestDate,
			},

			wantFailedState: api.ServerDeploymentStateWaitInstall,
			wantLastError:   "Deployment did not complete within " + config.ServerDeploymentTimeout.String(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			server := deploymentTestServer("one")
			server.Status = api.ServerStatusDeploying
			server.StatusInternal.Deployment = tc.deployment

			store := newDeploymentServerStore(server)

			repo := &repoMock.ServerRepoMock{
				GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
					return store.get(name)
				},
				UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
					store.put(in)

					return nil
				},
			}

			serverSvc := provisioningServer.New(
				repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
				provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
			)

			// Run test
			err := serverSvc.DeploymentControlLoop(t.Context(), new("one"))

			// Assert
			require.NoError(t, err)

			stored, err := store.get("one")
			require.NoError(t, err)

			deployment := stored.StatusInternal.Deployment
			require.Equal(t, api.ServerDeploymentStateFailed, deployment.State)
			require.Equal(t, tc.wantFailedState, deployment.FailedState)
			require.Equal(t, tc.wantLastError, deployment.LastError, "the live record keeps the reason the deployment failed")
			require.Len(t, deployment.History, 1)
			require.Equal(t, tc.wantLastError, deployment.History[0].Error, "the history entry of the failing state carries the reason")
			require.Equal(t, api.ServerStatusUnregistered, stored.Status)
			require.Equal(t, api.ServerStatusDetailUnregisteredDeploymentFailed, stored.StatusDetail)
		})
	}
}

func TestServerService_DeploymentControlLoopPicksUpATriggerArrivingWhileAdvancing(t *testing.T) {
	server := deploymentTestServer("one")
	server.Status = api.ServerStatusDeploying
	server.StatusInternal.Deployment = &provisioning.ServerDeployment{
		State:          api.ServerDeploymentStateWaitRegistration,
		StartedAt:      deploymentTestDate,
		StateEnteredAt: deploymentTestDate,
	}

	store := newDeploymentServerStore(server)

	entered := make(chan struct{})
	release := make(chan struct{})

	var reads atomic.Int32

	repo := &repoMock.ServerRepoMock{
		GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
			// The second read is the one the control loop performs while holding
			// the mutex of the server.
			if reads.Add(1) == 2 {
				close(entered)
				<-release
			}

			return store.get(name)
		},
		UpdateFunc: func(ctx context.Context, in provisioning.Server) error {
			store.put(in)

			return nil
		},
	}

	serverSvc := provisioningServer.New(
		repo, nil, nil, nil, nil, nil, nil, tls.Certificate{},
		provisioningServer.WithNow(func() time.Time { return deploymentTestDate }),
	)

	// Run test
	first := make(chan error, 1)

	go func() {
		first <- serverSvc.DeploymentControlLoop(context.Background(), new("one"))
	}()

	<-entered

	err := serverSvc.DeploymentControlLoop(t.Context(), new("one"))

	close(release)

	// Assert
	require.NoError(t, err, "an entry, that finds the deployment being advanced, hands its trigger over instead of failing")
	require.NoError(t, <-first)
	require.Equal(t, int32(4), reads.Load(), "the holder re-evaluates the deployment for the trigger, that arrived while it was advancing")
}
