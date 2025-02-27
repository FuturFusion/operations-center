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

func (s serverService) GetAllByClusterID(ctx context.Context, clusterID int) (Servers, error) {
	return s.repo.GetAllByClusterID(ctx, clusterID)
}

func (s serverService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s serverService) GetByID(ctx context.Context, id int) (Server, error) {
	return s.repo.GetByID(ctx, id)
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

		newServer.ID = server.ID

		server, err = s.repo.UpdateByID(ctx, newServer)
		return err
	})
	if err != nil {
		return Server{}, err
	}

	return server, nil
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

		server, err = s.repo.UpdateByID(ctx, server)
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

	return transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return err
		}

		// FIXME: deleteting a server also requires to delete all the inventory.

		return s.repo.DeleteByID(ctx, server.ID)
	})
}
