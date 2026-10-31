package cluster

import (
	"context"

	"github.com/cowsql/go-cowsql/cluster"
	cowdb "github.com/cowsql/go-cowsql/cluster/db"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/shared/api"
)

type MembershipService interface {
	Enable(ctx context.Context, clusterAddress string, configRepo ConfigRepo, globalRepo GlobalRaftRepo) error
	StartLeaderTasks()
	SetGlobalDB(repo GlobalRaftRepo)
	DetermineNodeID(ctx context.Context, address string) error
	Gateway() cluster.Gateway

	Stop(ctx context.Context) error

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

	GetCertificates(ctx context.Context) (map[string]api.Certificate, error)
	SetRaftCertificateByAddress(ctx context.Context, cert api.Certificate, address string) error
}
