package provisioning

import (
	"context"
	"fmt"
	"time"

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

func (s serverService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
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
