package provisioning

import (
	"context"
	"fmt"
	"time"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
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

	return s.repo.Create(ctx, newServer)
}

func (s serverService) GetAll(ctx context.Context) (Servers, error) {
	return s.repo.GetAll(ctx)
}

func (s serverService) GetAllByClusterID(ctx context.Context, clusterID int) (Servers, error) {
	return s.repo.GetAllByClusterID(ctx, clusterID)
}

func (s serverService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s serverService) GetByName(ctx context.Context, name string) (Server, error) {
	if name == "" {
		return Server{}, fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s serverService) UpdateByName(ctx context.Context, name string, newServer Server) (Server, error) {
	if name == "" {
		return Server{}, fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := newServer.Validate()
	if err != nil {
		return Server{}, err
	}

	if name != newServer.Name {
		return Server{}, domain.NewValidationErrf("Invalid server, name mismatch")
	}

	newServer.LastUpdated = s.now()

	return s.repo.UpdateByName(ctx, name, newServer)
}

func (s serverService) RenameByName(ctx context.Context, name string, newServer Server) (Server, error) {
	if name == "" {
		return Server{}, fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newServer.Name == "" {
		return Server{}, domain.NewValidationErrf("Invalid server, name cannot by empty")
	}

	var server Server
	var err error
	err = transaction.Do(ctx, func(ctx context.Context) error {
		server, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return err
		}

		server.Name = newServer.Name
		server.LastUpdated = s.now()

		server, err = s.repo.UpdateByName(ctx, name, server)
		return err
	})
	if err != nil {
		return Server{}, err
	}

	return server, nil
}

func (s serverService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// FIXME: deleteting a server also requires to delete all the inventory (in a transaction).

	return s.repo.DeleteByName(ctx, name)
}
