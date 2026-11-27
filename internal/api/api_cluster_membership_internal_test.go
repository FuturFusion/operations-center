package api

import (
	"context"
	"testing"
	"time"

	cowdb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/stretchr/testify/require"

	clusterSvc "github.com/FuturFusion/operations-center/internal/cluster"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	provisioningMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

// membersStub is a membership service only answering for the members.
type membersStub struct {
	clusterSvc.MembershipService

	members map[int64]cowdb.NodeInfo
}

func (s membersStub) GetMembersByID(context.Context) (map[int64]cowdb.NodeInfo, error) {
	return s.members, nil
}

func (s membersStub) HeartbeatOfflineThreshold() time.Duration {
	return time.Minute
}

func TestDaemon_memberRemovalChecks_bootingMedia(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		heartbeat time.Time
		state     api.ServerDeploymentState

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:      "online member storing the media a server boots from",
			heartbeat: time.Now(),
			state:     api.ServerDeploymentStateWaitSecureBootEnrolled,

			assertErr: func(tt require.TestingT, err error, _ ...any) {
				require.ErrorIs(tt, err, domain.ErrOperationNotPermitted)
			},
		},
		{
			name:      "offline member storing the media a server boots from",
			heartbeat: time.Now().Add(-time.Hour),
			state:     api.ServerDeploymentStateWaitSecureBootEnrolled,

			assertErr: require.NoError,
		},
		{
			name:      "online member storing the media of a server past its enrollment",
			heartbeat: time.Now(),
			state:     api.ServerDeploymentStateWaitInstall,

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := &Daemon{membershipSvc: membersStub{members: map[int64]cowdb.NodeInfo{2: {ID: 2, Name: "m2", Heartbeat: tc.heartbeat}}}}

			serverSvc := &provisioningMock.ServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return provisioning.Servers{{
						Name: "srv1",
						StatusInternal: provisioning.ServerStatusInternal{Deployment: &provisioning.ServerDeployment{
							State:             tc.state,
							SecureBootMediaID: "AAAAAAAAAAAA",
							Request:           provisioning.ServerDeploymentRequest{ImageType: api.ImageTypeISO},
						}},
					}}, nil
				},
			}

			secureBootMedia := &adapterMock.SecureBootMediaPortMock{
				HolderFunc: func(ctx context.Context, imageType api.ImageType, id string) (int64, error) {
					return 2, nil
				},
			}

			// Without a cluster, the mover counts no file sets, so the first check passes.
			checks := d.memberRemovalChecks(memberfiles.NewMover(func() memberfiles.Cluster { return nil }), serverSvc, secureBootMedia)
			require.Len(t, checks, 2)
			require.NoError(t, checks[0](t.Context(), "m2"))

			tc.assertErr(t, checks[1](t.Context(), "m2"))
		})
	}
}
