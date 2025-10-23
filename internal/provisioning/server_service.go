package provisioning

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	"github.com/lxc/incus/v6/shared/revert"
	"github.com/maniartech/signals"

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

func NewServerService(repo ServerRepo, client ServerClientPort, tokenSvc TokenService, clusterSvc ClusterService, opts ...ServerServiceOption) *serverService {
	serverSvc := &serverService{
		repo:       repo,
		client:     client,
		tokenSvc:   tokenSvc,
		clusterSvc: clusterSvc,

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

func (s serverService) Create(ctx context.Context, token uuid.UUID, newServer Server) (Server, error) {
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

		err = newServer.Validate()
		if err != nil {
			return fmt.Errorf("Validate server: %w", err)
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
		err = s.pollServer(ctx, newServer, true)
		if err != nil {
			slog.WarnContext(ctx, "Initial server connection test failed", logger.Err(err), slog.String("name", newServer.Name), slog.String("url", newServer.ConnectionURL))
		}
	}()

	return newServer, nil
}

func (s serverService) GetAll(ctx context.Context) (Servers, error) {
	return s.repo.GetAll(ctx)
}

func (s serverService) GetAllWithFilter(ctx context.Context, filter ServerFilter) (Servers, error) {
	var filterExpression *vm.Program
	var err error

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(Server{})}...)
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

	var filteredServers Servers
	if filter.Expression != nil {
		for _, server := range servers {
			output, err := expr.Run(filterExpression, server)
			if err != nil {
				return nil, domain.NewValidationErrf("Failed to execute filter expression: %v", err)
			}

			result, ok := output.(bool)
			if !ok {
				return nil, domain.NewValidationErrf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if result {
				filteredServers = append(filteredServers, server)
			}
		}

		return filteredServers, nil
	}

	return servers, nil
}

func (s serverService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s serverService) GetAllNamesWithFilter(ctx context.Context, filter ServerFilter) ([]string, error) {
	var filterExpression *vm.Program
	var err error

	type Env struct {
		Name string
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

func (s serverService) GetByName(ctx context.Context, name string) (*Server, error) {
	if name == "" {
		return nil, fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s serverService) Update(ctx context.Context, server Server) error {
	err := server.Validate()
	if err != nil {
		return fmt.Errorf("Failed to validate server for update: %w", err)
	}

	return s.repo.Update(ctx, server)
}

func (s serverService) UpdateSystemNetwork(ctx context.Context, name string, systemNetwork ServerSystemNetwork) (err error) {
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

		err = s.Update(ctx, *updatedServer)
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

func (s serverService) SelfUpdate(ctx context.Context, serverUpdate ServerSelfUpdate) error {
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

		return s.repo.Update(ctx, *server)
	})
	if err != nil {
		return err
	}

	s.selfUpdateSignal.Emit(ctx, *server)

	return nil
}

func (s serverService) Rename(ctx context.Context, oldName string, newName string) error {
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

func (s serverService) DeleteByName(ctx context.Context, name string) error {
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
func (s serverService) PollServers(ctx context.Context, serverStatus api.ServerStatus, updateServerConfiguration bool) error {
	servers, err := s.repo.GetAllWithFilter(ctx, ServerFilter{
		Status: ptr.To(serverStatus),
	})
	if err != nil {
		return fmt.Errorf("Failed to get servers for polling: %w", err)
	}

	var errs []error
	for _, server := range servers {
		err = s.pollServer(ctx, server, updateServerConfiguration)
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}

	return errors.Join(errs...)
}

func (s serverService) pollServer(ctx context.Context, server Server, updateServerConfiguration bool) error {
	// Since we re-try frequently, we only grant a short timeout for the
	// connection attept.
	ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, 1*time.Second)
	err := s.client.Ping(ctxWithTimeout, server)
	cancelFunc()

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		switch urlErr.Unwrap().(type) {
		case *tls.CertificateVerificationError:
			// If the servers certificates authority can not be verified it might be,
			// that the cluster now has a publicly valid certificate.
			//
			// This is only the case if:
			//
			//  - There is tls.CertificateVerificationError
			//  - The server is part of a cluster
			//  - The cluster has a pinned certificate set
			//
			// Retry connection with cluster certificate empty to test the cluster's
			// certificate against the system root certificates.
			if server.Cluster == nil || server.ClusterCertificate == nil && *server.ClusterCertificate == "" {
				break
			}

			server.ClusterCertificate = nil

			// Since we re-try frequently, we only grant a short timeout for the
			// connection attept.
			ctxWithTimeout, cancelFunc = context.WithTimeout(ctx, 1*time.Second)
			retryErr := s.client.Ping(ctxWithTimeout, server)
			cancelFunc()
			if retryErr != nil {
				// Ping without pinned certificate failed, keep the original error.
				break
			}

			retryErr = transaction.Do(ctx, func(ctx context.Context) error {
				cluster, err := s.clusterSvc.GetByName(ctx, *server.Cluster)
				if err != nil {
					return fmt.Errorf("Failed to get cluster for server %q: %w", server.Name, err)
				}

				cluster.Certificate = ""

				err = s.clusterSvc.Update(ctx, *cluster)
				if err != nil {
					return fmt.Errorf("Failed to update cluster's certificate for server %q: %w", server.Name, err)
				}

				return nil
			})
			if retryErr != nil {
				// The clusters certificate has passed validation against system root
				// certificates but we failed to update the cluster record in the DB.
				return retryErr
			}

			// Successfully updated the cluster's certificate, the original error has
			// been mitigated, so we can clear it.
			err = nil
		}
	}

	if err != nil {
		// Errors are expected if a system is not (yet) available. Therefore
		// we ignore the errors.
		slog.WarnContext(ctx, "Server connection test failed", logger.Err(err), slog.String("name", server.Name), slog.String("url", server.ConnectionURL))
		return nil
	}

	var hardwareData api.HardwareData
	var osData api.OSData
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

		if updateServerConfiguration {
			server.Status = api.ServerStatusReady
			server.HardwareData = hardwareData
			server.OSData = osData
			server.Type = serverType
		}

		return s.repo.Update(ctx, *server)
	})
}
