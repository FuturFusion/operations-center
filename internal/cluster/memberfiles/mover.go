package memberfiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

// Unit is a directory of a store whose files are stored by a single member.
type Unit struct {
	Store string

	// Name is the slash separated path of the directory in the store.
	Name string

	// ID identifies the unit in the repository providing it.
	ID int64
}

// FileTargetRepo lists the units of one kind of files and records which member stores them.
type FileTargetRepo interface {
	// GetUnitsByTarget returns the units stored on the member memberID.
	GetUnitsByTarget(ctx context.Context, memberID int64) ([]Unit, error)

	// SetUnitTarget records that the member memberID stores unit.
	SetUnitTarget(ctx context.Context, unit Unit, memberID int64) error
}

// Mover moves the files of a member to another one.
type Mover struct {
	cluster func() Cluster
	repos   []FileTargetRepo
}

// NewMover returns a mover for the units provided by repos, cluster returns nil until it is set up.
func NewMover(cluster func() Cluster, repos ...FileTargetRepo) Mover {
	return Mover{
		cluster: cluster,
		repos:   repos,
	}
}

// CountByTarget returns the number of units stored on the member memberName.
func (m Mover) CountByTarget(ctx context.Context, memberName string) (int, error) {
	cluster := m.cluster()
	if cluster == nil {
		return 0, nil
	}

	members, err := cluster.GetMembersByID(ctx)
	if err != nil {
		return 0, err
	}

	memberID, _, err := memberByName(members, memberName)
	if errors.Is(err, domain.ErrNotFound) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	count := 0
	for _, repo := range m.repos {
		units, err := repo.GetUnitsByTarget(ctx, memberID)
		if err != nil {
			return 0, err
		}

		count += len(units)
	}

	return count, nil
}

// Move copies the units of the member sourceName to the member targetName, records their new location and deletes them from the source.
// Units failing to move are kept on the source.
func (m Mover) Move(ctx context.Context, sourceName string, targetName string) error {
	if sourceName == targetName {
		return domain.NewValidationErrf("Source and target cluster member are both %q", sourceName)
	}

	cluster := m.cluster()
	if cluster == nil {
		//domain-errors:internal Files are only moved after the cluster has been set up.
		return errors.New("Cluster is not set up")
	}

	members, err := cluster.GetMembersByID(ctx)
	if err != nil {
		return err
	}

	sourceID, source, err := memberByName(members, sourceName)
	if err != nil {
		return err
	}

	targetID, target, err := memberByName(members, targetName)
	if err != nil {
		return err
	}

	for _, member := range []cowsqldb.NodeInfo{source, target} {
		if member.IsOffline(cluster.HeartbeatOfflineThreshold()) {
			return domain.NewErrorf(domain.ErrUnavailable, "", "Cluster member %q is offline", member.Name).
				WithHintf("Bring the cluster member back online.")
		}
	}

	targetClient, err := cluster.MemberClient(target.Address)
	if err != nil {
		return err
	}

	sourceClient, err := cluster.MemberClient(source.Address)
	if err != nil {
		return err
	}

	var failed []error
	for _, repo := range m.repos {
		units, err := repo.GetUnitsByTarget(ctx, sourceID)
		if err != nil {
			return err
		}

		for _, unit := range units {
			err = targetClient.PullMemberFiles(ctx, unit.Store, unit.Name, source.Address)
			if err != nil {
				failed = append(failed, fmt.Errorf("Failed to copy %q of store %q: %w", unit.Name, unit.Store, err))
				continue
			}

			err = repo.SetUnitTarget(ctx, unit, targetID)
			if err != nil {
				failed = append(failed, fmt.Errorf("Failed to record the file target of %q of store %q: %w", unit.Name, unit.Store, err))
				continue
			}

			// A copy left on the source is deleted on its next startup.
			err = sourceClient.DeleteMemberFile(ctx, unit.Store, unit.Name)
			if err != nil {
				slog.WarnContext(ctx, "Failed to delete moved files on cluster member", slog.String("store", unit.Store), slog.String("file", unit.Name), slog.String("member", source.Name), logger.Err(err))
			}
		}
	}

	if len(failed) > 0 {
		return domain.NewErrorf(domain.ErrUnavailable, "", "Failed to move %d file sets from cluster member %q to %q, they are kept on %q", len(failed), source.Name, target.Name, source.Name).
			WithHintf("Check the log for the cause and move the files again.").
			WithCause(errors.Join(failed...))
	}

	return nil
}

func memberByName(members map[int64]cowsqldb.NodeInfo, name string) (int64, cowsqldb.NodeInfo, error) {
	for id, member := range members {
		if member.Name == name {
			return id, member, nil
		}
	}

	return 0, cowsqldb.NodeInfo{}, domain.NewErrorf(domain.ErrNotFound, "", "Cluster member %q not found", name)
}
