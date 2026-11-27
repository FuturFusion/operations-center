package cluster

import (
	"context"
	"crypto/x509"
	"net/http"
	"time"

	"github.com/cowsql/go-cowsql/cluster"
	cowdb "github.com/cowsql/go-cowsql/cluster/db"

	"github.com/FuturFusion/operations-center/internal/client"
	membershipapi "github.com/FuturFusion/operations-center/internal/cluster/api"
	"github.com/FuturFusion/operations-center/internal/cluster/repo/sqlite/entities"
	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/security/authn"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type MembershipService interface {
	authn.Auther

	Enable(ctx context.Context, clusterAddress string, tokenRepo JoinTokenRepo, configRepo ConfigRepo, globalRepo GlobalRaftRepo) error
	StartLeaderTasks()
	LeaderContext(name string) (context.Context, func(), bool)
	TaskStatus() membershipapi.InternalClusterTaskStatus
	SetGlobalDB(repo GlobalRaftRepo)
	DetermineNodeID(ctx context.Context, address string) error
	Gateway() cluster.Gateway
	NotifyUpgradeCompleted(ctx context.Context) error
	HandlerFuncs() map[string]http.HandlerFunc

	HandOverRole(ctx context.Context) error
	Stop(ctx context.Context) error

	Bootstrap(ctx context.Context, cfg api.ClusterBootstrapPost) error
	Join(ctx context.Context, cfg api.ClusterJoinPost) error
	Trust(ctx context.Context, token string, fingerprint string) error
	Accept(ctx context.Context, cfg api.ClusterAcceptPost) (*membershipapi.InternalClusterAcceptResponse, error)
	RemoveMember(ctx context.Context, name string, force bool) error
	Reset(ctx context.Context)
	GetMembers(ctx context.Context) ([]api.ClusterMember, error)
	GetMembersByID(ctx context.Context) (map[int64]cowdb.NodeInfo, error)
	HeartbeatOfflineThreshold() time.Duration
	LocalMemberID() (int64, error)
	MemberClient(address string) (client.OperationsCenterClient, error)
	LeaderAddress(ctx context.Context) (string, error)
	Handover(ctx context.Context, address string) error
	Rebalance(ctx context.Context) error

	GetTokens(ctx context.Context) (entities.ClusterJoinTokens, error)
	GetTokenByServerName(ctx context.Context, serverName string) (*entities.ClusterJoinToken, error)
	CreateToken(ctx context.Context, req api.ClusterTokenPost) error
	DeleteTokenByName(ctx context.Context, serverName string) error

	TrustedClients() map[string]x509.Certificate
	IsMemberCertificate(certificatePEM string) bool

	PersistSharedConfig(ctx context.Context, change config.SharedChange) (stored config.SharedConfig, previous config.SharedConfig, err error)
	PublishSharedConfig(ctx context.Context, change config.SharedChange, previous config.SharedConfig) error
	CheckMembersOnline(ctx context.Context) error
	UpdateClusterCertificate(ctx context.Context, certificatePEM string, keyPEM string) error
	ApplyClusterCertificate(ctx context.Context, certificatePEM string, keyPEM string) error
	ReloadSharedConfig(ctx context.Context) error

	ContactMembers(ctx context.Context, except ...string) error
}

type GlobalRaftRepo interface {
	cowdb.Cluster

	GetCertificates(ctx context.Context) (map[string]api.Certificate, error)
	DetermineNodeID(ctx context.Context, address string) error
	GetNodeID() int64
	SchemaVersion() int
}

type JoinTokenRepo interface {
	GetTokens(ctx context.Context) ([]entities.ClusterJoinToken, error)
	GetTokenByServerName(ctx context.Context, serverName string) (*entities.ClusterJoinToken, error)
	CreateToken(ctx context.Context, token entities.ClusterJoinToken) error
	SetJoinerFingerprint(ctx context.Context, serverName string, fingerprint string) error
	DeleteTokenByName(ctx context.Context, serverName string) error
}

type ConfigRepo interface {
	GetSharedConfig(ctx context.Context) (*config.SharedConfig, error)
	UpdateSharedConfig(ctx context.Context, cfg config.SharedConfig) error
	CreateSharedConfig(ctx context.Context, cfg config.SharedConfig) error
}

type LocalRaftRepo interface {
	cowdb.Node

	GetNetworkConfig(ctx context.Context) system.NetworkLocal
	GetCertificates(ctx context.Context) (map[string]api.Certificate, error)
	SetRaftCertificateByAddress(ctx context.Context, cert api.Certificate, address string) error
}
