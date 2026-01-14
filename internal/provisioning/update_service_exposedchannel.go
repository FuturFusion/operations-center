package provisioning

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

func (s *updateService) CreateExposedchannel(ctx context.Context, newExposedchannel Exposedchannel) (Exposedchannel, error) {
	err := newExposedchannel.Validate()
	if err != nil {
		return Exposedchannel{}, err
	}

	newExposedchannel.ID, err = s.repo.CreateExposedchannel(ctx, newExposedchannel)
	if err != nil {
		return Exposedchannel{}, err
	}

	return newExposedchannel, nil
}

func (s *updateService) GetAllExposedchannels(ctx context.Context) (Exposedchannels, error) {
	return s.repo.GetAllExposedchannels(ctx)
}

func (s *updateService) GetAllExposedchannelNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllExposedchannelNames(ctx)
}

func (s *updateService) GetExposedchannelByName(ctx context.Context, name string) (*Exposedchannel, error) {
	if name == "" {
		return nil, fmt.Errorf("Exposed channel name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetExposedchannelByName(ctx, name)
}

func (s *updateService) UpdateExposedchannel(ctx context.Context, newExposedchannel Exposedchannel) error {
	err := newExposedchannel.Validate()
	if err != nil {
		return err
	}

	return s.repo.UpdateExposedchannel(ctx, newExposedchannel)
}

func (s *updateService) RenameExposedchannel(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Exposed channel name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New exposed channel name cannot by empty")
	}

	return transaction.Do(ctx, func(ctx context.Context) error {
		servers, err := s.serverSvc.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to fetch servers: %w", err)
		}

		for _, server := range servers {
			if oldName == server.VersionData.UpdateChannel {
				return fmt.Errorf("Rename of exposed channel not supported, if in use by any server: %w", domain.ErrOperationNotPermitted)
			}
		}

		err = s.repo.RenameExposedchannel(ctx, oldName, newName)
		if err != nil {
			return fmt.Errorf("Failed to rename exposed channel from %q to %q: %w", oldName, newName, err)
		}

		return nil
	})
}

func (s *updateService) DeleteExposedchannelByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Exposed channel name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return transaction.Do(ctx, func(ctx context.Context) error {
		updates, err := s.repo.GetUpdatesByAssignedExposedchannelName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get updates assigned to exposed channel %q: %w", name, err)
		}

		if len(updates) > 0 {
			return fmt.Errorf("Delete of exposed channel not supported, if in use by any update: %w", domain.ErrOperationNotPermitted)
		}

		servers, err := s.serverSvc.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to fetch servers: %w", err)
		}

		for _, server := range servers {
			if name == server.VersionData.UpdateChannel {
				return fmt.Errorf("Delete of exposed channel not supported, if in use by any server: %w", domain.ErrOperationNotPermitted)
			}
		}

		err = s.repo.DeleteExposedchannelByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete exposed channel: %w", err)
		}

		return nil
	})
}
