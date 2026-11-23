package cluster

import (
	"context"
	"net/http"

	"github.com/cowsql/go-cowsql/cluster"
	cowdb "github.com/cowsql/go-cowsql/cluster/db"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type MembershipService interface {
	Enable(ctx context.Context, clusterAddress string, configRepo ConfigRepo, globalRepo GlobalRaftRepo) error
	StartLeaderTasks()
	SetGlobalDB(repo GlobalRaftRepo)
	DetermineNodeID(ctx context.Context, address string) error
	Gateway() cluster.Gateway
	NotifyUpgradeCompleted(ctx context.Context) error
	HandlerFuncs() map[string]http.HandlerFunc

	Stop(ctx context.Context) error

	Bootstrap(ctx context.Context, cfg api.ClusterBootstrapPost) error

	PersistSharedConfig(ctx context.Context, change config.SharedChange) (config.SharedConfig, error)
}

type GlobalRaftRepo interface {
	cowdb.Cluster

	GetCertificates(ctx context.Context) (map[string]api.Certificate, error)
	DetermineNodeID(ctx context.Context, address string) error
	GetNodeID() int64
	SchemaVersion() int
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
