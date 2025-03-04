package provisioning

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type serverService struct {
	repo ServerRepo
}

var _ ServerService = &serverService{}

func NewServerService(repo ServerRepo) serverService {
	return serverService{
		repo: repo,
	}
}

func (s serverService) Create(ctx context.Context, newServer Server) (Server, error) {
	err := newServer.Validate()
	if err != nil {
		return Server{}, err
	}

	return s.repo.Create(ctx, newServer)
}

func (s serverService) GetAll(ctx context.Context) (Servers, error) {
	return s.repo.GetAll(ctx)
}

func (s serverService) GetAllByCluster(ctx context.Context, cluster string) (Servers, error) {
	return s.repo.GetAllByCluster(ctx, cluster)
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

	var server Server
	err = transaction.Do(ctx, func(ctx context.Context) error {
		server, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return err
		}

		server, err = s.repo.UpdateByName(ctx, name, newServer)
		return err
	})
	if err != nil {
		return Server{}, err
	}

	return server, nil
}

func (s serverService) Rename(ctx context.Context, name string, to string) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if to == "" {
		return domain.NewValidationErrf("New server name cannot by empty")
	}

	return s.repo.Rename(ctx, name, to)
}

func (s serverService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Server name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.DeleteByName(ctx, name)
}
