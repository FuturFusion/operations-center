// Package updatefiletarget lets other cluster members manage the update files they store.
package updatefiletarget

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path"

	"github.com/google/uuid"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

type updateFileTarget struct {
	targets memberfiles.Targets
}

var _ provisioning.UpdateFileTargetPort = updateFileTarget{}

// New returns access to the members storing update files, found through targets.
func New(targets memberfiles.Targets) updateFileTarget {
	return updateFileTarget{targets: targets}
}

func (u updateFileTarget) LocalMemberID(ctx context.Context) (int64, error) {
	return u.targets.LocalID()
}

func (u updateFileTarget) GetFile(ctx context.Context, update provisioning.Update, filename string) (io.ReadCloser, int, error) {
	member, err := u.targets.Member(ctx, update.MemberID)
	if err != nil {
		return nil, 0, err
	}

	c, err := u.targets.Client(member)
	if err != nil {
		return nil, 0, err
	}

	rc, size, err := c.GetMemberFile(ctx, memberfiles.StoreUpdates, path.Join(update.UUID.String(), filename))
	if err != nil {
		return nil, 0, memberfiles.Unavailable(member, err)
	}

	return rc, int(size), nil
}

func (u updateFileTarget) RefreshFiles(ctx context.Context, update provisioning.Update) error {
	return u.targets.Do(ctx, update.MemberID, func(ctx context.Context, c client.OperationsCenterClient) error {
		return c.RefreshMemberUpdateFiles(ctx, update.UUID)
	})
}

func (u updateFileTarget) DeleteFiles(ctx context.Context, update provisioning.Update) error {
	return u.targets.Do(ctx, update.MemberID, func(ctx context.Context, c client.OperationsCenterClient) error {
		return c.DeleteMemberFile(ctx, memberfiles.StoreUpdates, update.UUID.String())
	})
}

func (u updateFileTarget) CheckFiles(ctx context.Context, memberID int64, updates provisioning.Updates) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(updates))
	for _, update := range updates {
		ids = append(ids, update.UUID)
	}

	var incomplete []uuid.UUID
	err := u.targets.Do(ctx, memberID, func(ctx context.Context, c client.OperationsCenterClient) error {
		var err error
		incomplete, err = c.CheckMemberUpdateFiles(ctx, ids)
		return err
	})
	if err != nil {
		return nil, err
	}

	return incomplete, nil
}

func (u updateFileTarget) DeleteAllFiles(ctx context.Context) error {
	members, err := u.targets.Others(ctx)
	if err != nil {
		return err
	}

	for _, member := range members {
		err := u.targets.Do(ctx, member.ID, func(ctx context.Context, c client.OperationsCenterClient) error {
			return c.DeleteMemberFile(ctx, memberfiles.StoreUpdates, "")
		})

		// An unreachable member deletes the files on its next startup.
		if errors.Is(err, domain.ErrUnavailable) {
			slog.WarnContext(ctx, "Failed to delete the files of all updates on cluster member", slog.String("member", member.Name), logger.Err(err))
			continue
		}

		if err != nil {
			return err
		}
	}

	return nil
}
