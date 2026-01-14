package provisioning

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
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
