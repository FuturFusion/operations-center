package api

import (
	"context"
	"errors"
	"strconv"

	clusteringSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

// memberRemovalChecks returns the checks refusing the removal of a cluster member, which still stores files needed by others.
// They run inside the membership service right before the removal, so they see the files assigned to the member meanwhile.
func (d *Daemon) memberRemovalChecks(mover memberfiles.Mover, serverSvc provisioning.ServerService, secureBootMedia provisioning.SecureBootMediaPort) []clusteringSvc.RemovalCheck {
	// The files only stored by the member would be lost.
	fileTarget := func(ctx context.Context, name string) error {
		fileSets, err := mover.CountByTarget(ctx, name)
		if err != nil {
			return err
		}

		if fileSets > 0 {
			return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster member %q is the file target of %d file sets, which no other member stores", name, fileSets).
				WithHintf("Move the files to another cluster member first, or force the removal, which loses them.").
				WithDetail("member", name).
				WithDetail("file_sets", strconv.Itoa(fileSets))
		}

		return nil
	}

	// An online member, which stores the secure boot enrollment media a server boots from right now, is kept.
	// A regenerated media is signed by another key, so a server reading parts of both could not enroll.
	bootingMedia := func(ctx context.Context, name string) error {
		members, err := d.membershipSvc.GetMembersByID(ctx)
		if err != nil {
			return err
		}

		var memberID int64
		for id, member := range members {
			if member.Name != name {
				continue
			}

			if member.IsOffline(d.membershipSvc.HeartbeatOfflineThreshold()) {
				return nil
			}

			memberID = id
		}

		if memberID == 0 {
			return nil
		}

		servers, err := serverSvc.GetAll(ctx)
		if err != nil {
			return err
		}

		for _, server := range servers {
			deployment := server.StatusInternal.Deployment
			if !deployment.BootsSecureBootMedia() {
				continue
			}

			holderID, err := secureBootMedia.Holder(ctx, deployment.Request.ImageType, deployment.SecureBootMediaID)
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}

			if err != nil {
				return err
			}

			if holderID == memberID {
				return domain.NewErrorf(domain.ErrOperationNotPermitted, "", "Cluster member %q stores the secure boot enrollment media server %q is booting from", name, server.Name).
					WithHintf("Wait for the enrollment of the server to finish, or force the removal.").
					WithDetail("member", name).
					WithDetail("server", server.Name)
			}
		}

		return nil
	}

	return []clusteringSvc.RemovalCheck{fileTarget, bootingMedia}
}
