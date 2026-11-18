package cluster

import (
	"context"

	"github.com/cowsql/go-cowsql/cluster"
	cowdb "github.com/cowsql/go-cowsql/cluster/db"

	"github.com/FuturFusion/operations-center/shared/api"
)

type MembershipService interface {
	Enable(ctx context.Context, clusterAddress string, globalRepo GlobalRaftRepo) error
	StartLeaderTasks()
	SetGlobalDB(repo GlobalRaftRepo)
	DetermineNodeID(ctx context.Context, address string) error
	Gateway() cluster.Gateway

	Stop(ctx context.Context) error
}

type GlobalRaftRepo interface {
	cowdb.Cluster

	GetCertificates(ctx context.Context) (map[string]api.Certificate, error)
	DetermineNodeID(ctx context.Context, address string) error
	GetNodeID() int64
	SchemaVersion() int
}

type LocalRaftRepo interface {
	cowdb.Node

	GetCertificates(ctx context.Context) (map[string]api.Certificate, error)
	SetRaftCertificateByAddress(ctx context.Context, cert api.Certificate, address string) error
}
