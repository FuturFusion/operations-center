// Package memberfiles tracks the files of updates, images and cluster artifacts, each of which is stored on a single cluster member.
package memberfiles

import (
	"context"
	"errors"
	"time"

	cowsqldb "github.com/cowsql/go-cowsql/cluster/db"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
)

// StandaloneMemberID is the member ID of a server that is not clustered.
const StandaloneMemberID = 1

// Cluster provides the members of the cluster.
type Cluster interface {
	// LocalMemberID returns the ID of this member.
	LocalMemberID() (int64, error)

	// GetMembersByID returns the cluster members by their ID from the global database.
	GetMembersByID(ctx context.Context) (map[int64]cowsqldb.NodeInfo, error)

	// HeartbeatOfflineThreshold returns the time after which a member without heartbeat is offline.
	HeartbeatOfflineThreshold() time.Duration

	// MemberClient returns a client for the cluster member with the given address.
	MemberClient(address string) (client.OperationsCenterClient, error)
}

// Member is the cluster member storing a set of files.
type Member struct {
	ID      int64
	Name    string
	Address string
	Offline bool

	// Local is true for this member.
	Local bool
}

// Targets finds the cluster members storing files.
type Targets struct {
	cluster func() Cluster
}

// NewTargets returns the file targets, cluster returns nil until the cluster is set up.
func NewTargets(cluster func() Cluster) Targets {
	return Targets{cluster: cluster}
}

// LocalID returns the ID of this member.
func (t Targets) LocalID() (int64, error) {
	cluster := t.cluster()
	if cluster == nil {
		return StandaloneMemberID, nil
	}

	return cluster.LocalMemberID()
}

// Member returns the member memberID, looking up this member without accessing the database.
func (t Targets) Member(ctx context.Context, memberID int64) (Member, error) {
	localID, err := t.LocalID()
	if err != nil {
		return Member{}, err
	}

	if memberID == localID {
		return Member{ID: memberID, Local: true}, nil
	}

	cluster := t.cluster()
	if cluster == nil {
		return Member{}, notFoundErr(memberID)
	}

	members, err := cluster.GetMembersByID(ctx)
	if err != nil {
		return Member{}, err
	}

	member, ok := members[memberID]
	if !ok {
		return Member{}, notFoundErr(memberID)
	}

	return Member{ID: memberID, Name: member.Name, Address: member.Address, Offline: member.IsOffline(cluster.HeartbeatOfflineThreshold())}, nil
}

// Others returns the members except this one.
func (t Targets) Others(ctx context.Context) ([]Member, error) {
	cluster := t.cluster()
	if cluster == nil {
		return nil, nil
	}

	localID, err := cluster.LocalMemberID()
	if err != nil {
		return nil, err
	}

	members, err := cluster.GetMembersByID(ctx)
	if err != nil {
		return nil, err
	}

	others := make([]Member, 0, len(members))
	for id, member := range members {
		if id == localID {
			continue
		}

		others = append(others, Member{ID: id, Name: member.Name, Address: member.Address, Offline: member.IsOffline(cluster.HeartbeatOfflineThreshold())})
	}

	return others, nil
}

// Client returns a client for the other member.
func (t Targets) Client(member Member) (client.OperationsCenterClient, error) {
	cluster := t.cluster()
	if cluster == nil || member.Local {
		//domain-errors:internal Only other cluster members are reached through a client.
		return client.OperationsCenterClient{}, errors.New("No client for the local member")
	}

	return cluster.MemberClient(member.Address)
}

// Do calls fn with a client for the other member memberID, limited by the
// request timeout. An unreachable member is reported as unavailable.
func (t Targets) Do(ctx context.Context, memberID int64, fn func(ctx context.Context, c client.OperationsCenterClient) error) error {
	member, err := t.Member(ctx, memberID)
	if err != nil {
		return err
	}

	c, err := t.Client(member)
	if err != nil {
		return err
	}

	ctx, cancel := Context(ctx, member)
	defer cancel()

	return Unavailable(member, fn(ctx, c))
}

// Context applies the timeout of a request to member that does not stream file content.
func Context(ctx context.Context, member Member) (context.Context, context.CancelFunc) {
	if member.Offline {
		return context.WithTimeout(ctx, offlineRequestTimeout)
	}

	return context.WithTimeout(ctx, requestTimeout)
}

// Unavailable reports member as unavailable if err is not an error response of the member.
func Unavailable(member Member, err error) error {
	if err == nil {
		return nil
	}

	var serverErr *client.ServerError
	if errors.As(err, &serverErr) {
		return err
	}

	return domain.NewErrorf(domain.ErrUnavailable, "", "The files are stored by the cluster member %q, which is unreachable", member.Name).
		WithHintf("Bring the cluster member back online.").
		WithCause(err).
		WithDetail("member", member.Name)
}

func notFoundErr(memberID int64) error {
	return domain.NewErrorf(domain.ErrNotFound, "", "Cluster member with ID %d not found", memberID)
}

const (
	// requestTimeout limits requests to members that do not stream file content.
	requestTimeout = 30 * time.Second

	// offlineRequestTimeout limits requests to members that missed their heartbeats.
	offlineRequestTimeout = 5 * time.Second
)
