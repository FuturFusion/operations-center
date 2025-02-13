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

func (s serverService) GetAllHostnames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllHostnames(ctx)
}

func (s serverService) GetByID(ctx context.Context, id int) (Server, error) {
	return s.repo.GetByID(ctx, id)
}

func (s serverService) GetByHostname(ctx context.Context, hostname string) (Server, error) {
	if hostname == "" {
		return Server{}, fmt.Errorf("Server hostname cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByHostname(ctx, hostname)
}

func (s serverService) UpdateByHostname(ctx context.Context, hostname string, newServer Server) (Server, error) {
	if hostname == "" {
		return Server{}, fmt.Errorf("Server hostname cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	err := newServer.Validate()
	if err != nil {
		return Server{}, err
	}

	if hostname != newServer.Hostname {
		return Server{}, domain.NewValidationErrf("Invalid server, hostname mismatch")
	}

	var server Server
	err = transaction.Do(ctx, func(ctx context.Context) error {
		server, err = s.repo.GetByHostname(ctx, hostname)
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

func (s serverService) RenameByHostname(ctx context.Context, hostname string, newServer Server) (Server, error) {
	if hostname == "" {
		return Server{}, fmt.Errorf("Server hostname cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newServer.Hostname == "" {
		return Server{}, domain.NewValidationErrf("Invalid server, hostname cannot by empty")
	}

	var server Server
	var err error
	err = transaction.Do(ctx, func(ctx context.Context) error {
		server, err = s.repo.GetByHostname(ctx, hostname)
		if err != nil {
			return err
		}

		server.Hostname = newServer.Hostname

		server, err = s.repo.UpdateByID(ctx, server)
		return err
	})
	if err != nil {
		return Server{}, err
	}

	return server, nil
}

func (s serverService) DeleteByHostname(ctx context.Context, hostname string) error {
	if hostname == "" {
		return fmt.Errorf("Server hostname cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return transaction.Do(ctx, func(ctx context.Context) error {
		server, err := s.repo.GetByHostname(ctx, hostname)
		if err != nil {
			return err
		}

		return s.repo.DeleteByID(ctx, server.ID)
	})
}
