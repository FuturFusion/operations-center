package provisioning

import (
	"context"
	"fmt"
	"runtime"

	"github.com/google/uuid"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type channelService struct {
	repo      ChannelRepo
	serverSvc ServerService
	updateSvc UpdateService
}

var _ ChannelService = &channelService{}

func NewChannelService(repo ChannelRepo, updateSvc UpdateService) *channelService {
	service := &channelService{
		repo:      repo,
		updateSvc: updateSvc,
	}

	// Register for the UpdatesValidateSignal to validate the updates channels.
	// The way through signals is chosen here to prevent a dependency cycle
	// between the config and the provisioning package.
	listenerKey := uuid.New().String()
	config.UpdatesValidateSignal.AddListenerWithErr(service.validateUpdatesConfig, listenerKey)
	runtime.AddCleanup(service, func(listenerKey string) {
		config.UpdatesValidateSignal.RemoveListener(listenerKey)
	}, listenerKey)

	return service
}

func (s *channelService) SetServerService(serverSvc ServerService) {
	s.serverSvc = serverSvc
}

func (s *channelService) Create(ctx context.Context, newChannel Channel) (Channel, error) {
	err := newChannel.Validate()
	if err != nil {
		return Channel{}, err
	}

	newChannel.ID, err = s.repo.Create(ctx, newChannel)
	if err != nil {
		return Channel{}, err
	}

	return newChannel, nil
}

func (s *channelService) GetAll(ctx context.Context) (Channels, error) {
	return s.repo.GetAll(ctx)
}

func (s *channelService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s *channelService) GetByName(ctx context.Context, name string) (*Channel, error) {
	if name == "" {
		return nil, fmt.Errorf("Channel name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s *channelService) Update(ctx context.Context, newChannel Channel) error {
	err := newChannel.Validate()
	if err != nil {
		return err
	}

	return s.repo.Update(ctx, newChannel)
}

func (s *channelService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Channel name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return transaction.Do(ctx, func(ctx context.Context) error {
		updates, err := s.updateSvc.GetUpdatesByAssignedChannelName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get updates assigned to channel %q: %w", name, err)
		}

		if len(updates) > 0 {
			return fmt.Errorf("Delete of channel not supported, if in use by any update: %w", domain.ErrOperationNotPermitted)
		}

		servers, err := s.serverSvc.GetAll(ctx)
		if err != nil {
			return fmt.Errorf("Failed to fetch servers: %w", err)
		}

		for _, server := range servers {
			if name == server.VersionData.UpdateChannel {
				return fmt.Errorf("Delete of channel not supported, if in use by any server: %w", domain.ErrOperationNotPermitted)
			}
		}

		err = s.repo.DeleteByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete channel: %w", err)
		}

		return nil
	})
}

func (s channelService) validateUpdatesConfig(ctx context.Context, su api.SystemUpdates) error {
	if su.UpdatesDefaultChannel == "" {
		return domain.NewValidationErrf(`Invalid config, "updates.updates_default_channel" can not be empty`)
	}

	_, err := s.repo.GetByName(ctx, su.UpdatesDefaultChannel)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, failed to get "updates.updates_default_channel": %v`, err)
	}

	if su.ServerDefaultChannel == "" {
		return domain.NewValidationErrf(`Invalid config, "updates.server_default_channel" can not be empty`)
	}

	_, err = s.repo.GetByName(ctx, su.ServerDefaultChannel)
	if err != nil {
		return domain.NewValidationErrf(`Invalid config, failed to get "updates.server_default_channel": %v`, err)
	}

	return nil
}
