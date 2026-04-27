package channel

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/lxc/incus-os/incus-osd/api/images"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type channelService struct {
	repo      provisioning.ChannelRepo
	serverSvc provisioning.ServerService
	updateSvc provisioning.UpdateService
}

var _ provisioning.ChannelService = &channelService{}

func New(repo provisioning.ChannelRepo, updateSvc provisioning.UpdateService) *channelService {
	service := &channelService{
		repo:      repo,
		updateSvc: updateSvc,
	}

	// Register for the UpdatesValidateSignal to validate the updates channels.
	// The way through signals is chosen here to prevent a dependency cycle
	// between the config and the provisioning package.
	listenerKey := uuid.New().String()
	lifecycle.UpdatesValidateSignal.AddListenerWithErr(service.validateUpdatesConfig, listenerKey)

	return service
}

func (s *channelService) SetServerService(serverSvc provisioning.ServerService) {
	s.serverSvc = serverSvc
}

func (s *channelService) Create(ctx context.Context, newChannel provisioning.Channel) (provisioning.Channel, error) {
	err := newChannel.Validate()
	if err != nil {
		return provisioning.Channel{}, err
	}

	newChannel.ID, err = s.repo.Create(ctx, newChannel)
	if err != nil {
		return provisioning.Channel{}, err
	}

	return newChannel, nil
}

func (s *channelService) GetAll(ctx context.Context) (provisioning.Channels, error) {
	return s.repo.GetAll(ctx)
}

func (s *channelService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s *channelService) GetByName(ctx context.Context, name string) (*provisioning.Channel, error) {
	if name == "" {
		return nil, fmt.Errorf("Channel name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s *channelService) Update(ctx context.Context, newChannel provisioning.Channel) error {
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

func (s channelService) validateUpdatesConfig(ctx context.Context, su system.Updates) error {
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

func (s channelService) GetChangelogByName(ctx context.Context, name string, architecture images.UpdateFileArchitecture) (api.UpdateChangelogs, error) {
	updates, err := s.updateSvc.GetAllWithFilter(ctx, provisioning.UpdateFilter{
		Channel: ptr.To(name),
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to get updates for channel %q: %w", name, err)
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("Channel %q does not contain any updates: %w", name, domain.ErrOperationNotPermitted)
	}

	if len(updates) == 1 {
		changelog, err := s.updateSvc.GetChangelog(ctx, updates[0].UUID, uuid.Nil, architecture)
		if err != nil {
			return nil, fmt.Errorf("Failed to get changelog for update %s: %w", updates[0].UUID.String(), err)
		}

		return api.UpdateChangelogs{changelog}, nil
	}

	sort.Sort(updates)

	channelChangelog := make(api.UpdateChangelogs, 0, len(updates)-1)
	for range updates[:len(updates)-1] {
		currentID := updates[0].UUID
		priorID := updates[1].UUID

		changelog, err := s.updateSvc.GetChangelog(ctx, currentID, priorID, architecture)
		if err != nil {
			return nil, fmt.Errorf("Failed to get changelog for update %s: %w", updates[0].UUID.String(), err)
		}

		channelChangelog = append(channelChangelog, changelog)
	}

	return channelChangelog, nil
}
