package provisioning

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/lxc/incus/v6/shared/revert"
	"github.com/maniartech/signals"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type serverService struct {
	repo       ServerRepo
	client     ServerClientPort
	tokenSvc   TokenService
	clusterSvc ClusterService
	channelSvc ChannelService
	updateSvc  UpdateService

	httpClient *http.Client

	mu                sync.Mutex
	serverURL         string
	serverCertificate tls.Certificate

	now                    func() time.Time
	initialConnectionDelay time.Duration

	selfUpdateSignal signals.Signal[Server]
}

var _ ServerService = &serverService{}

// ErrSelfUpdateNotification is used as cause when the context is
// cancelled while waiting for the update of the network config
// to complete.
var ErrSelfUpdateNotification = errors.New("self update notification")

type ServerServiceOption func(s *serverService)

func ServerServiceWithNow(nowFunc func() time.Time) ServerServiceOption {
	return func(s *serverService) {
		s.now = nowFunc
	}
}

func ServerServiceWithInitialConnectionDelay(delay time.Duration) ServerServiceOption {
	return func(s *serverService) {
		s.initialConnectionDelay = delay
	}
}

func (s *serverService) UpdateServerURL(ctx context.Context, serverURL string) error {
	s.mu.Lock()
	s.serverURL = serverURL
	s.mu.Unlock()

	return s.SelfRegisterOperationsCenter(ctx)
}

func (s *serverService) UpdateServerCertificate(ctx context.Context, serverCertificate tls.Certificate) error {
	s.mu.Lock()
	s.serverCertificate = serverCertificate
	s.mu.Unlock()

	return s.SelfRegisterOperationsCenter(ctx)
}

func NewServerService(repo ServerRepo, client ServerClientPort, tokenSvc TokenService, clusterSvc ClusterService, channelSvc ChannelService, updateSvc UpdateService, serverConnectionURL string, serverCertificate tls.Certificate, opts ...ServerServiceOption) *serverService {
	serverSvc := &serverService{
		repo:       repo,
		client:     client,
		tokenSvc:   tokenSvc,
		clusterSvc: clusterSvc,
		channelSvc: channelSvc,
		updateSvc:  updateSvc,
		httpClient: &http.Client{},

		serverURL:         serverConnectionURL,
		serverCertificate: serverCertificate,

		now:                    time.Now,
		initialConnectionDelay: 1 * time.Second,

		selfUpdateSignal: signals.New[Server](),
	}

	for _, opt := range opts {
		opt(serverSvc)
	}

	return serverSvc
}

func (s *serverService) SetClusterService(clusterSvc ClusterService) {
	s.clusterSvc = clusterSvc
}

func (s *serverService) Create(ctx context.Context, token uuid.UUID, newServer Server) (Server, error) {
	err := transaction.Do(ctx, func(ctx context.Context) error {
		err := s.tokenSvc.Consume(ctx, token)
		if err != nil {
			return fmt.Errorf("Consume token for server creation: %w", err)
		}

		newServer.Status = api.ServerStatusPending
		newServer.LastSeen = s.now()

		if newServer.Type == "" {
			newServer.Type = api.ServerTypeUnknown
		}

		if newServer.Channel == "" {
			newServer.Channel = config.GetUpdates().ServerDefaultChannel
		}

		err = newServer.Validate()
		if err != nil {
			return fmt.Errorf("Validate server: %w", err)
		}

		if newServer.Type == api.ServerTypeOperationsCenter {
			return domain.NewValidationErrf("Remote operations centers can not be registered")
		}

		newServer.ID, err = s.repo.Create(ctx, newServer)
		if err != nil {
			return fmt.Errorf("Create server: %w", err)
		}

		return nil
	})
	if err != nil {
		return Server{}, err
	}

	// Perform initial connection test to server right after registration.
	// Since we have the background task to update the server state, we do not
	// care about graceful shutdown for this "one off" check.
	go func() {
		time.Sleep(s.initialConnectionDelay)

		ctx := context.Background()
		err = s.PollServer(ctx, newServer, true)
		if err != nil {
			slog.WarnContext(ctx, "Initial server connection test failed", logger.Err(err), slog.String("name", newServer.Name), slog.String("url", newServer.ConnectionURL))
		}
	}()

	return newServer, nil
}

func (s *serverService) GetAll(ctx context.Context) (Servers, error) {
	return s.GetAllWithFilter(ctx, ServerFilter{})
}

func (s *serverService) GetAllWithFilter(ctx context.Context, filter ServerFilter) (Servers, error) {
	var filterExpression *vm.Program
	var err error

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(ToExprServer(Server{}))}...)
		if err != nil {
			return nil, domain.NewValidationErrf("Failed to compile filter expression: %v", err)
		}
	}

	var servers Servers
	if filter.Name == nil && filter.Cluster == nil && filter.Status == nil {
		servers, err = s.repo.GetAll(ctx)
	} else {
		servers, err = s.repo.GetAllWithFilter(ctx, filter)
	}

	if err != nil {
		return nil, err
	}

	if filter.Expression != nil {
		n := 0
		for i := range servers {
			output, err := expr.Run(filterExpression, ToExprServer(servers[i]))
			if err != nil {
				return nil, domain.NewValidationErrf("Failed to execute filter expression: %v", err)
			}

			result, ok := output.(bool)
			if !ok {
				return nil, domain.NewValidationErrf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if !result {
				continue
			}

			servers[n] = servers[i]
			n++
		}

		servers = servers[:n]
	}

	for i := range servers {
		err = s.enrichServerWithVersionDetails(ctx, &servers[i])
		if err != nil {
			return nil, err
		}
	}

	return servers, nil
}

func (s *serverService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s *serverService) GetAllNamesWithFilter(ctx context.Context, filter ServerFilter) ([]string, error) {
	var filterExpression *vm.Program
	var err error

	type Env struct {
		Name string `expr:"name"`
	}

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(Env{})}...)
		if err != nil {
			return nil, domain.NewValidationErrf("Failed to compile filter expression: %v", err)
		}
	}

	var serverIDs []string

	if filter.Name == nil && filter.Cluster == nil {
		serverIDs, err = s.repo.GetAllNames(ctx)
	} else {
		serverIDs, err = s.repo.GetAllNamesWithFilter(ctx, filter)
	}

	if err != nil {
		return nil, err
	}

	var filteredServerIDs []string
	if filter.Expression != nil {
		for _, serverID := range serverIDs {
			output, err := expr.Run(filterExpression, Env{serverID})
			if err != nil {
				return nil, domain.NewValidationErrf("Failed to execute filter expression: %v", err)
			}

			result, ok := output.(bool)
			if !ok {
				return nil, domain.NewValidationErrf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if result {
				filteredServerIDs = append(filteredServerIDs, serverID)
			}
		}

		return filteredServerIDs, nil
	}

	return serverIDs, nil
}

func (s *serverService) GetByName(ctx context.Context, name string) (*Server, error) {
	if name == "" {
		return nil, fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	server, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	err = s.enrichServerWithVersionDetails(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("Failed to enrich server %q with update version details: %w", name, err)
	}

	return server, nil
}

func (s *serverService) enrichServerWithVersionDetails(ctx context.Context, server *Server) error {
	updates, err := s.updateSvc.GetAllWithFilter(ctx, UpdateFilter{
		Channel: &server.Channel,
	})
	if err != nil {
		return fmt.Errorf("Failed to get channel for server %q: %w", server.Name, err)
	}

	if len(updates) == 0 {
		// No updates found, enrich without update version information.
		server.VersionData.Compute(nil)
		return nil
	}

	serverComponents := make([]string, 0, len(server.VersionData.Applications)+1) // All applications + OS
	serverComponents = append(serverComponents, string(images.UpdateFileComponentOS))
	for i, app := range server.VersionData.Applications {
		serverComponents = append(serverComponents, app.Name)
		server.VersionData.Applications[i].NeedsUpdate = ptr.To(false)
	}

	// For each component installed on the server (OS, applications), we need to
	// find the most recent update. Since updates are not necessarily covering
	// all components (OS, applications), we need to iterate over the updates
	// in decending order (the updates are already returned sorted correctly).
	//
	// For each component, where we found a corresponding update, we update the
	// latestAvailableVersions map and remove the component from
	// `serverComponents`. We are done with the work, if `serverComponents` is
	// empty.
	latestAvailableVersions := make(map[images.UpdateFileComponent]string, len(updates))
	for _, update := range updates {
		if len(serverComponents) == 0 {
			break
		}

		for _, updateComponent := range update.Components() {
			serverComponents = slices.DeleteFunc(serverComponents, func(serverComponent string) bool {
				if serverComponent == updateComponent.String() {
					latestAvailableVersions[updateComponent] = update.Version
					return true
				}

				return false
			})
		}
	}

	if len(serverComponents) != 0 {
		// This indicates, that for some components, we have not found any update.
		// This is a possible case, e.g. if someone clears and refreshes all the
		// updates and then queries servers, registered in Operations Center, before
		// Operations Center has refreshed the Updates from upstream.
		slog.WarnContext(ctx, "Failed to find updates for some components while enriching server record with update version information", slog.Any("remaining_server_components", serverComponents))
	}

	server.VersionData.Compute(latestAvailableVersions)

	return nil
}

func availableVersionGreaterThan(currentVersion string, availableVersion string) bool {
	current, err := strconv.ParseInt(currentVersion, 16, 64)
	if err != nil {
		current = math.MinInt // invalid versions are moved to the end.
	}

	available, err := strconv.ParseInt(availableVersion, 16, 64)
	if err != nil {
		available = math.MinInt // invalid versions are moved to the end.
	}

	return available > current
}

// Update writes the new server state to the DB and pushes the changed
// settings to the system as well, if updateSystem argument is set to true.
func (s *serverService) Update(ctx context.Context, server Server, updateSystem bool) error {
	err := server.Validate()
	if err != nil {
		return fmt.Errorf("Failed to validate server for update: %w", err)
	}

	err = s.repo.Update(ctx, server)
	if err != nil {
		return fmt.Errorf("Failed to update server %q: %w", server.Name, err)
	}

	if !updateSystem {
		return nil
	}

	err = s.UpdateSystemUpdate(ctx, server.Name, incusosapi.SystemUpdate{
		Config: incusosapi.SystemUpdateConfig{
			Channel: server.Channel,
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to update system update configuration for server %q: %w", server.Name, err)
	}

	return nil
}

func (s *serverService) UpdateSystemNetwork(ctx context.Context, name string, systemNetwork ServerSystemNetwork) (err error) {
	server := &Server{}
	updatedServer := &Server{}

	reverter := revert.New()
	defer reverter.Fail()

	err = transaction.Do(ctx, func(ctx context.Context) error {
		var err error

		server, err = s.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server %q: %w", name, err)
		}

		updatedServer, _ = ptr.Clone(server)

		updatedServer.OSData.Network = systemNetwork
		updatedServer.Status = api.ServerStatusPending

		updatedServer.LastSeen = s.now()

		err = s.Update(ctx, *updatedServer, false)
		if err != nil {
			return fmt.Errorf("Failed to update system network: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	reverter.Add(func() {
		revertErr := s.repo.Update(ctx, *server)
		if revertErr != nil {
			err = errors.Join(err, revertErr)
		}
	})

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// Listen to self update signal for the case, when the network update prevents the response for the regular update call.
	signalHandlerKey := uuid.New().String()
	s.selfUpdateSignal.AddListener(func(_ context.Context, inServer Server) {
		if inServer.Name == server.Name {
			// Received self update from the updated server. Cancel potientiall hanging update config request.
			cancel(ErrSelfUpdateNotification)
			s.selfUpdateSignal.RemoveListener(signalHandlerKey)
		}
	}, signalHandlerKey)
	defer s.selfUpdateSignal.RemoveListener(signalHandlerKey)

	err = s.client.UpdateNetworkConfig(ctx, *updatedServer)
	// If context is cancelled with cause ErrSelfUpdateNotification, the self update
	// call has been processed and the operations was successful.
	// Therefore this is not considered an error.
	if err != nil && !errors.Is(context.Cause(ctx), ErrSelfUpdateNotification) {
		return err
	}

	reverter.Success()

	return nil
}

func (s *serverService) UpdateSystemStorage(ctx context.Context, name string, systemStorage ServerSystemStorage) (err error) {
	server := &Server{}
	updatedServer := &Server{}

	reverter := revert.New()
	defer reverter.Fail()

	err = transaction.Do(ctx, func(ctx context.Context) error {
		var err error

		server, err = s.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server %q: %w", name, err)
		}

		updatedServer, _ = ptr.Clone(server)

		updatedServer.OSData.Storage = systemStorage
		updatedServer.Status = api.ServerStatusPending

		updatedServer.LastSeen = s.now()

		err = s.Update(ctx, *updatedServer, false)
		if err != nil {
			return fmt.Errorf("Failed to update system network: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	reverter.Add(func() {
		revertErr := s.repo.Update(ctx, *server)
		if revertErr != nil {
			err = errors.Join(err, revertErr)
		}
	})

	err = s.client.UpdateStorageConfig(ctx, *updatedServer)
	if err != nil {
		return err
	}

	reverter.Success()

	return nil
}

func (s *serverService) GetSystemProvider(ctx context.Context, name string) (ServerSystemProvider, error) {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return ServerSystemProvider{}, fmt.Errorf("Failed to get server %q: %w", name, err)
	}

	providerConfig, err := s.client.GetProviderConfig(ctx, *server)
	if err != nil {
		return ServerSystemProvider{}, err
	}

	return providerConfig, nil
}

func (s *serverService) UpdateSystemProvider(ctx context.Context, name string, providerConfig ServerSystemProvider) error {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q: %w", name, err)
	}

	err = s.client.UpdateProviderConfig(ctx, *server, providerConfig)
	if err != nil {
		return err
	}

	return nil
}

func (s *serverService) GetSystemUpdate(ctx context.Context, name string) (ServerSystemUpdate, error) {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return ServerSystemUpdate{}, fmt.Errorf("Failed to get server %q: %w", name, err)
	}

	updateConfig, err := s.client.GetUpdateConfig(ctx, *server)
	if err != nil {
		return ServerSystemUpdate{}, fmt.Errorf("Failed to get update config from %q: %w", server.Name, err)
	}

	return updateConfig, nil
}

func (s *serverService) UpdateSystemUpdate(ctx context.Context, name string, updateConfig ServerSystemUpdate) error {
	_, err := s.channelSvc.GetByName(ctx, updateConfig.Config.Channel)
	if err != nil {
		return fmt.Errorf("Failed to get channel %q: %w", name, err)
	}

	server, err := s.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q: %w", name, err)
	}

	serverSystemUpdate := ServerSystemUpdate{
		Config: incusosapi.SystemUpdateConfig{
			AutoReboot: false, // forced by Operations Center
			// For now, the only setting that we allow to be changed by the user is the Update Channel.
			Channel:        updateConfig.Config.Channel,
			CheckFrequency: "never", // forced by Operations Center
		},
	}

	err = s.client.UpdateUpdateConfig(ctx, *server, serverSystemUpdate)
	if err != nil {
		return fmt.Errorf("Failed to update the update config for %q: %w", server.Name, err)
	}

	err = s.PollServer(ctx, *server, true)
	if err != nil {
		slog.WarnContext(ctx, "Server poll after changing the update configuration failed (non-critical), fixed by the next successful server poll interval", logger.Err(err), slog.String("name", server.Name), slog.String("url", server.ConnectionURL))
	}

	return nil
}

func (s *serverService) SelfUpdate(ctx context.Context, serverUpdate ServerSelfUpdate) error {
	if serverUpdate.Self {
		return s.SelfRegisterOperationsCenter(ctx)
	}

	var server *Server

	err := transaction.Do(ctx, func(ctx context.Context) error {
		var err error

		authenticationCertificatePEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: serverUpdate.AuthenticationCertificate.Raw,
		})

		server, err = s.repo.GetByCertificate(ctx, string(authenticationCertificatePEM))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.ErrNotAuthorized
			}

			return fmt.Errorf("Failed to get server by certificate: %w", err)
		}

		server.ConnectionURL = serverUpdate.ConnectionURL
		server.Status = api.ServerStatusReady
		server.LastSeen = s.now()

		err = server.Validate()
		if err != nil {
			return fmt.Errorf("Failed to validate server update: %w", err)
		}

		err = s.repo.Update(ctx, *server)
		if err != nil {
			return fmt.Errorf("Failed to self-update server: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	err = s.PollServer(ctx, *server, true)
	if err != nil {
		return fmt.Errorf("Failed to update server configuration after self update: %w", err)
	}

	s.selfUpdateSignal.Emit(ctx, *server)

	return nil
}

func (s *serverService) SelfRegisterOperationsCenter(ctx context.Context) error {
	var server Server
	pollAfterCreate := false

	err := transaction.Do(ctx, func(ctx context.Context) error {
		servers, err := s.repo.GetAllWithFilter(ctx, ServerFilter{
			Type: ptr.To(api.ServerTypeOperationsCenter),
		})
		if err != nil {
			return fmt.Errorf(`Failed to get server of type "operations-center": %w`, err)
		}

		if len(servers) > 1 {
			return fmt.Errorf(`Invalid internal state, expect at most 1 server of type "operations-center", found %d`, len(servers))
		}

		s.mu.Lock()
		serverCert := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: s.serverCertificate.Leaf.Raw,
		})

		serverURL := s.serverURL
		s.mu.Unlock()

		var upsert func(context.Context, Server) error

		if len(servers) == 0 {
			// Create server entry
			server = Server{
				Name:          api.ServerNameOperationsCenter,
				Type:          api.ServerTypeOperationsCenter,
				ConnectionURL: serverURL,
				Certificate:   string(serverCert),
				Status:        api.ServerStatusReady,
				LastSeen:      s.now(),
				Channel:       config.GetUpdates().ServerDefaultChannel,
			}

			upsert = func(ctx context.Context, server Server) error {
				_, err := s.repo.Create(ctx, server)
				return err
			}

			pollAfterCreate = true
		} else {
			// Update existing server entry
			server = servers[0]
			server.ConnectionURL = serverURL
			server.Certificate = string(serverCert)
			server.Status = api.ServerStatusReady
			server.LastSeen = s.now()

			upsert = func(ctx context.Context, server Server) error {
				return s.repo.Update(ctx, server)
			}
		}

		err = server.Validate()
		if err != nil {
			return fmt.Errorf("Validate server: %w", err)
		}

		err = upsert(ctx, server)
		if err != nil {
			return fmt.Errorf("Self register operations-center as server: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if pollAfterCreate {
		err = s.PollServer(ctx, server, true)
		if err != nil {
			return fmt.Errorf("Failed to update server configuration after self registration: %w", err)
		}
	}

	return nil
}

func (s *serverService) Rename(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New Server name cannot by empty")
	}

	if oldName == newName {
		return domain.NewValidationErrf("Old and new Server name are equal")
	}

	return s.repo.Rename(ctx, oldName, newName)
}

func (s *serverService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get server for delete: %w", err)
		}

		if server.Cluster != nil {
			return fmt.Errorf("Failed to delete server, server is part of cluster %q", *server.Cluster)
		}

		err = s.repo.DeleteByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete server: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("Failed to delete server: %w", err)
	}

	return nil
}

// PollServers tests server connectivity for servers registered in operations center.
// This is used in the following ways:
//   - Periodic connectivity test for all servers in the inventory.
//   - Periodic connectivity test for all pending servers in the inventory.
//   - Periodic update of server configuration data (network, security, resources)
func (s *serverService) PollServers(ctx context.Context, serverStatus api.ServerStatus, updateServerConfiguration bool) error {
	servers, err := s.repo.GetAllWithFilter(ctx, ServerFilter{
		Status: ptr.To(serverStatus),
	})
	if err != nil {
		return fmt.Errorf("Failed to get servers for polling: %w", err)
	}

	var errs []error
	for _, server := range servers {
		err = s.PollServer(ctx, server, updateServerConfiguration)
		if err != nil && !errors.Is(err, api.NotIncusOSError) {
			errs = append(errs, err)
			continue
		}
	}

	return errors.Join(errs...)
}

func (s *serverService) PoweroffSystemByName(ctx context.Context, name string) error {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	err = s.client.Poweroff(ctx, *server)
	if err != nil {
		return fmt.Errorf("Failed to poweroff server %q by name: %w", name, err)
	}

	return nil
}

func (s *serverService) RebootSystemByName(ctx context.Context, name string) error {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	err = s.client.Reboot(ctx, *server)
	if err != nil {
		return fmt.Errorf("Failed to reboot server %q by name: %w", name, err)
	}

	return nil
}

func (s *serverService) UpdateSystemByName(ctx context.Context, name string, updateRequest api.ServerUpdatePost) error {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	// Forcefully set channel and update frequency on server before triggering update.
	err = s.UpdateSystemUpdate(ctx, name, incusosapi.SystemUpdate{
		Config: incusosapi.SystemUpdateConfig{
			Channel: server.Channel,
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to enforce update channel for server %q: %w", server.Name, err)
	}

	if updateRequest.OS.TriggerUpdate {
		err = s.client.UpdateOS(ctx, *server)
		if err != nil {
			return fmt.Errorf("Failed to update the OS of server %q by name: %w", name, err)
		}
	}

	// FIXME: iterate over the applications and trigger the update for the applications
	// as well, if the TriggerUpdate flag is set to true for the particular application.
	// https://github.com/FuturFusion/operations-center/issues/616

	return nil
}

func (s *serverService) ResyncByName(ctx context.Context, name string) error {
	server, err := s.GetByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get server %q by name: %w", name, err)
	}

	err = s.PollServer(ctx, *server, true)
	if err != nil {
		return fmt.Errorf("Failed to resync server %q by name: %w", name, err)
	}

	return nil
}

func (s *serverService) PollServer(ctx context.Context, server Server, updateServerConfiguration bool) error {
	log := slog.With(slog.String("name", server.Name), slog.String("url", server.ConnectionURL))

	// Since we re-try frequently, we only grant a short timeout for the
	// connection attept.
	ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, 1*time.Second)
	err := s.client.Ping(ctxWithTimeout, server)
	cancelFunc()

	var updatedServerCertificate string

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
	refreshAttempt:
		switch urlErr.Unwrap().(type) {
		case *tls.CertificateVerificationError:
			// If the servers certificates authority can not be verified it might be,
			// that the cluster now has a publicly valid certificate.
			//
			// There are two main cases to distinguish:
			//
			//   1. Clustered Incus
			//   2. Standalone non Incus server (currently only Migration Manager)
			//
			// For the first case, clustered Incus, the following preconditions have
			// to be met:
			//
			//   - There is tls.CertificateVerificationError
			//   - The server is part of a cluster
			//   - The cluster has a pinned certificate set
			//
			// If the preconditions hold, retry connection with cluster certificate
			// empty to test the cluster's certificate against the system root
			// certificates. If this is successful, reset the cluster's certificate
			// in the DB to empty, causing subsequent connection attempts to rely
			// on the system root certificates.
			//
			// For the second case, standalone non Incus server, the following
			// preconditions have to be met:
			//
			//   - The server has a public connection URL configured
			//
			// If the preconditions hold, retry connection with server certificate
			// empty and connect to the public connection URL to test the server's
			// certificate against the system root certificates. If this is
			// successful, update the servers certificate in the DB, causing
			// subsequent connection attempts to verify against the new certificate.

			// Since we re-try frequently, we only grant a short timeout for the
			// connection attept.
			ctxWithTimeout, cancelFunc = context.WithTimeout(ctx, 5*time.Second)
			defer cancelFunc()

			isClusteredIncus := server.Cluster != nil && server.ClusterCertificate != nil && *server.ClusterCertificate != ""
			isStandaloneNonIncusServerWithPublicConnectionURL := server.Cluster == nil && server.Type != api.ServerTypeIncus && server.PublicConnectionURL != ""

			switch {
			case isClusteredIncus: // case 1, clustered Incus with cluster certificate set
				server.ClusterCertificate = nil

				retryErr := s.client.Ping(ctxWithTimeout, server)
				cancelFunc()
				if retryErr != nil {
					// Ping without pinned certificate failed, keep the original error.
					break refreshAttempt
				}

				retryErr = transaction.Do(ctx, func(ctx context.Context) error {
					cluster, retryErr := s.clusterSvc.GetByName(ctx, *server.Cluster)
					if retryErr != nil {
						return fmt.Errorf("Failed to get cluster for server %q: %w", server.Name, retryErr)
					}

					cluster.Certificate = nil

					retryErr = s.clusterSvc.Update(ctx, *cluster)
					if retryErr != nil {
						return fmt.Errorf("Failed to update cluster's certificate for server %q: %w", server.Name, retryErr)
					}

					return nil
				})
				if retryErr != nil {
					// The clusters certificate has passed validation against system root
					// certificates but we failed to update the cluster record in the DB.
					return retryErr
				}

			case isStandaloneNonIncusServerWithPublicConnectionURL: // case 2, standalone non Incus server
				req, retryErr := http.NewRequestWithContext(ctxWithTimeout, http.MethodGet, server.PublicConnectionURL, http.NoBody)
				if retryErr != nil {
					// Create request for certificate check failed, keep the original error.
					break refreshAttempt
				}

				resp, retryErr := (s.httpClient).Do(req)
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}

				if retryErr != nil {
					// Connection to public connection URL failed. This can be a network
					// issue, an invalid or unreachable public connection URL or a
					// certificate error. We don't care about the root cause in this
					// case and break the refresh attempt and keep the original error.
					log.DebugContext(ctx, "Refresh certificate connection attempt to public connection URL failed", logger.Err(retryErr))
					break refreshAttempt
				}

				if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
					// Connection was successful, but we don't have a TLS connection
					// or no peer certificates (should not happen, as long as
					// public connection URL is https).  We don't care about the root
					// cause in this case and break the refresh attempt and keep the
					// original error.
					log.DebugContext(ctx, "Refresh certificate connection attempt did not return TLS connection or no peer certificates")
					break refreshAttempt
				}

				serverCert := pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: resp.TLS.PeerCertificates[0].Raw,
				})

				// Only set the certificate here, the Update in the DB happens at the
				// end of the function.
				updatedServerCertificate = string(serverCert)

			default:
				// neither case 1 nor case 2, don't attempt to refresh certificate
				break refreshAttempt
			}

			// Successfully updated the servers's or the cluster's certificate, the
			// original error has been mitigated, so we can clear it.
			err = nil
		}
	}

	if err != nil {
		// Errors are expected if a system is not (yet) available. Therefore
		// we ignore the errors.
		log.WarnContext(ctx, "Server connection test failed", logger.Err(err))
		return nil
	}

	var hardwareData api.HardwareData
	var osData api.OSData
	var versionData api.ServerVersionData
	var serverType api.ServerType
	if updateServerConfiguration {
		hardwareData, err = s.client.GetResources(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get resources from server %q: %w", server.Name, err)
		}

		osData, err = s.client.GetOSData(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get os data from server %q: %w", server.Name, err)
		}

		versionData, err = s.client.GetVersionData(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get version data from server %q: %w", server.Name, err)
		}

		// For now, we ignore the error and we are fine to persist type "unknown",
		// if we are not able to determine the server type.
		serverType, _ = s.client.GetServerType(ctx, server)
	}

	// Perform the update of the server in a transaction in order to respect
	// potential updates, that happened since we queried for the list of servers
	// in pending state.
	return transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, server.Name)
		if err != nil {
			return err
		}

		server.LastSeen = s.now()

		if updatedServerCertificate != "" {
			server.Certificate = updatedServerCertificate
		}

		if updateServerConfiguration {
			server.Status = api.ServerStatusReady
			server.HardwareData = hardwareData
			server.OSData = osData
			server.VersionData = versionData
			server.Type = serverType
		}

		return s.repo.Update(ctx, *server)
	})
}
