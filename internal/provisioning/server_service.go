package provisioning

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	"github.com/lxc/incus/v6/shared/revert"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type serverService struct {
	repo     ServerRepo
	client   ServerClientPort
	tokenSvc TokenService

	now                    func() time.Time
	initialConnectionDelay time.Duration
}

var _ ServerService = &serverService{}

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

func NewServerService(repo ServerRepo, client ServerClientPort, tokenSvc TokenService, opts ...ServerServiceOption) serverService {
	serverSvc := serverService{
		repo:     repo,
		client:   client,
		tokenSvc: tokenSvc,

		now:                    time.Now,
		initialConnectionDelay: 1 * time.Second,
	}

	for _, opt := range opts {
		opt(&serverSvc)
	}

	return serverSvc
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
			return nil, err
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
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
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
			return nil, err
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
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
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
		return err
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
			return err
		}

		updatedServer, _ = ptr.Clone(server)

		updatedServer.OSData.Network = systemNetwork
		updatedServer.Status = api.ServerStatusPending

		updatedServer.LastSeen = s.now()

		err = s.Update(ctx, *updatedServer)
		if err != nil {
			return err
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

	err = s.client.UpdateNetworkConfig(ctx, *updatedServer)
	if err != nil {
		return err
	}

	reverter.Success()

	return nil
}

func (s serverService) SelfUpdate(ctx context.Context, serverUpdate ServerSelfUpdate) error {
	return transaction.Do(ctx, func(ctx context.Context) error {
		authenticationCertificatePEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: serverUpdate.AuthenticationCertificate.Raw,
		})

		server, err := s.repo.GetByCertificate(ctx, string(authenticationCertificatePEM))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return domain.ErrNotAuthorized
			}

			return err
		}

		server.ConnectionURL = serverUpdate.ConnectionURL
		server.Status = api.ServerStatusReady
		server.LastSeen = s.now()

		err = server.Validate()
		if err != nil {
			return err
		}

		return s.repo.Update(ctx, *server)
	})
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
		return err
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
	if err != nil {
		// Errors are expected if a system is not (yet) available. Therefore
		// we ignore the errors.
		slog.WarnContext(ctx, "Server connection test failed", logger.Err(err), slog.String("name", server.Name), slog.String("url", server.ConnectionURL))
		return nil
	}

	var hardwareData api.HardwareData
	var osData api.OSData
	if updateServerConfiguration {
		hardwareData, err = s.client.GetResources(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get resources from server %q: %w", server.Name, err)
		}

		osData, err = s.client.GetOSData(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get os data from server %q: %w", server.Name, err)
		}
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
		}

		return s.repo.Update(ctx, *server)
	})
}
