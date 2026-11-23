package memberfiles_test

import (
	"context"
	"errors"
	"testing"
	"time"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/cluster/memberfiles"
	"github.com/FuturFusion/operations-center/internal/domain"
)

type fakeCluster struct {
	localID int64
	members map[int64]cowsqldb.NodeInfo
}

func (f fakeCluster) LocalMemberID() (int64, error) {
	return f.localID, nil
}

func (f fakeCluster) GetMembersByID(ctx context.Context) (map[int64]cowsqldb.NodeInfo, error) {
	return f.members, nil
}

func (f fakeCluster) HeartbeatOfflineThreshold() time.Duration {
	return 20 * time.Second
}

func (f fakeCluster) MemberClient(address string) (client.OperationsCenterClient, error) {
	return client.New(address)
}

func TestTargets_Member(t *testing.T) {
	standalone := memberfiles.NewTargets(func() memberfiles.Cluster { return nil })

	member, err := standalone.Member(t.Context(), memberfiles.StandaloneMemberID)
	require.NoError(t, err)
	require.True(t, member.Local)

	_, err = standalone.Member(t.Context(), 2)
	require.ErrorIs(t, err, domain.ErrNotFound)

	cluster := fakeCluster{localID: 2, members: map[int64]cowsqldb.NodeInfo{
		2: {Name: "two", Address: "192.0.2.2:8443", Heartbeat: time.Now()},
		3: {Name: "three", Address: "192.0.2.3:8443", Heartbeat: time.Now().Add(-time.Hour)},
	}}

	clustered := memberfiles.NewTargets(func() memberfiles.Cluster { return cluster })

	member, err = clustered.Member(t.Context(), 2)
	require.NoError(t, err)
	require.True(t, member.Local)

	member, err = clustered.Member(t.Context(), 3)
	require.NoError(t, err)
	require.Equal(t, memberfiles.Member{ID: 3, Name: "three", Address: "192.0.2.3:8443", Offline: true}, member)

	others, err := clustered.Others(t.Context())
	require.NoError(t, err)
	require.Equal(t, []memberfiles.Member{member}, others)

	_, err = clustered.Member(t.Context(), 4)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUnavailable(t *testing.T) {
	member := memberfiles.Member{Name: "two"}

	err := memberfiles.Unavailable(member, errors.New("connection refused"))
	require.ErrorIs(t, err, domain.ErrUnavailable)
	require.ErrorContains(t, err, `"two"`)

	err = memberfiles.Unavailable(member, &client.ServerError{StatusCode: 404})
	require.ErrorIs(t, err, domain.ErrNotFound)

	require.NoError(t, memberfiles.Unavailable(member, nil))
}
