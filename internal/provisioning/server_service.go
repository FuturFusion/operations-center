package provisioning

import (
	"context"
	"fmt"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/FuturFusion/operations-center/internal/domain"
)

type serverService struct {
	repo ServerRepo

	now func() time.Time
}

var _ ServerService = &serverService{}

type ServerServiceOption func(s *serverService)

func ServerServiceWithNow(nowFunc func() time.Time) ServerServiceOption {
	return func(s *serverService) {
		s.now = nowFunc
	}
}

func NewServerService(repo ServerRepo, opts ...ServerServiceOption) serverService {
	serverSvc := serverService{
		repo: repo,

		now: time.Now,
	}

	for _, opt := range opts {
		opt(&serverSvc)
	}

	return serverSvc
}

func (s serverService) Create(ctx context.Context, newServer Server) (Server, error) {
	err := newServer.Validate()
	if err != nil {
		return Server{}, err
	}

	newServer.LastUpdated = s.now()

	newServer.ID, err = s.repo.Create(ctx, newServer)
	if err != nil {
		return Server{}, err
	}

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
