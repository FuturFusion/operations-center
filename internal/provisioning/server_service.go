package provisioning

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"

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
		newServer.LastUpdated = s.now()

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
		err = s.pollPendingServer(ctx, newServer)
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
	if filter.Name == nil && filter.Cluster == nil {
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

func (s serverService) Update(ctx context.Context, newServer Server) error {
	err := newServer.Validate()
	if err != nil {
		return err
	}

	newServer.LastUpdated = s.now()

	return s.repo.Update(ctx, newServer)
}

func (s serverService) Rename(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New Server name cannot by empty")
	}

	return s.repo.Rename(ctx, oldName, newName)
}

func (s serverService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// FIXME: deleteting a server also requires to delete all the inventory (in a transaction).

	return s.repo.DeleteByName(ctx, name)
}

func (s serverService) PollPendingServers(ctx context.Context) error {
	servers, err := s.repo.GetAllWithFilter(ctx, ServerFilter{
		Status: ptr.To(api.ServerStatusPending),
	})
	if err != nil {
		return err
	}

	var errs []error
	for _, server := range servers {
		err = s.pollPendingServer(ctx, server)
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}

	return errors.Join(errs...)
}

func (s serverService) pollPendingServer(ctx context.Context, server Server) error {
	// Since we re-try frequently, we only grant a short timeout for the
	// connection attept.
	ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, 1*time.Second)
	err := s.client.Ping(ctxWithTimeout, server)
	cancelFunc()
	if err != nil {
		// Errors are expected if a system is not (yet) available. Therefore
		// we ignore the errors.
		slog.WarnContext(ctx, "Initial server connection test failed", logger.Err(err), slog.String("name", server.Name), slog.String("url", server.ConnectionURL))
		return nil
	}

	// Perform the update of the server in a transaction in order to respect
	// potential updates, that happened since we queried for the list of servers
	// in pending state.
	return transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, server.Name)
		if err != nil {
			return err
		}

		server.Status = api.ServerStatusReady

		return s.repo.Update(ctx, *server)
	})
}
