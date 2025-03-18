package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type server struct {
	db sqlite.DBTX
}

var _ provisioning.ServerRepo = &server{}

func NewServer(db sqlite.DBTX) (*server, error) {
	dbprepare, ok := db.(interface {
		Prepare(query string) (*sql.Stmt, error)
	})
	if !ok {
		return nil, fmt.Errorf("Provided db does not support prepare")
	}

	stmts, err := entities.PrepareStmts(dbprepare, false)
	if err != nil {
		return nil, fmt.Errorf("Failed to prepare statements: %w", err)
	}

	entities.PreparedStmts = stmts

	return &server{
		db: db,
	}, nil
}

func (s server) Create(ctx context.Context, in provisioning.Server) (provisioning.Server, error) {
	_, err := entities.CreateServer(ctx, s.db, entities.Server{
		Cluster:       in.Cluster,
		Name:          in.Name,
		Type:          in.Type,
		ConnectionURL: in.ConnectionURL,
		HardwareData: entities.MarshalableResource{
			CPU:     in.HardwareData.CPU,
			Memory:  in.HardwareData.Memory,
			GPU:     in.HardwareData.GPU,
			Network: in.HardwareData.Network,
			Storage: in.HardwareData.Storage,
			USB:     in.HardwareData.USB,
			PCI:     in.HardwareData.PCI,
			System:  in.HardwareData.System,
			Load:    in.HardwareData.Load,
		},
		VersionData: in.VersionData,
		LastUpdated: in.LastUpdated,
	})
	if err != nil {
		return provisioning.Server{}, err
	}

	return in, nil
}

func (s server) GetAll(ctx context.Context) (provisioning.Servers, error) {
	dbServers, err := entities.GetServers(ctx, s.db)
	if err != nil {
		return nil, err
	}

	servers := make(provisioning.Servers, 0, len(dbServers))
	for _, dbServer := range dbServers {
		servers = append(servers, provisioning.Server{
			Cluster:       dbServer.Cluster,
			Name:          dbServer.Name,
			Type:          dbServer.Type,
			ConnectionURL: dbServer.ConnectionURL,
			HardwareData: api.Resources{
				CPU:     dbServer.HardwareData.CPU,
				Memory:  dbServer.HardwareData.Memory,
				GPU:     dbServer.HardwareData.GPU,
				Network: dbServer.HardwareData.Network,
				Storage: dbServer.HardwareData.Storage,
				USB:     dbServer.HardwareData.USB,
				PCI:     dbServer.HardwareData.PCI,
				System:  dbServer.HardwareData.System,
				Load:    dbServer.HardwareData.Load,
			},
			VersionData: dbServer.VersionData,
			LastUpdated: dbServer.LastUpdated,
		})
	}

	return servers, nil
}

func (s server) GetAllByCluster(ctx context.Context, cluster string) (provisioning.Servers, error) {
	// TODO: handling somewhat redundant with GetAll
	dbServers, err := entities.GetServers(ctx, s.db, entities.ServerFilter{Cluster: ptr.To(cluster)})
	if err != nil {
		return nil, err
	}

	servers := make(provisioning.Servers, 0, len(dbServers))
	for _, dbServer := range dbServers {
		servers = append(servers, provisioning.Server{
			Cluster:       dbServer.Cluster,
			Name:          dbServer.Name,
			Type:          dbServer.Type,
			ConnectionURL: dbServer.ConnectionURL,
			HardwareData: api.Resources{
				CPU:     dbServer.HardwareData.CPU,
				Memory:  dbServer.HardwareData.Memory,
				GPU:     dbServer.HardwareData.GPU,
				Network: dbServer.HardwareData.Network,
				Storage: dbServer.HardwareData.Storage,
				USB:     dbServer.HardwareData.USB,
				PCI:     dbServer.HardwareData.PCI,
				System:  dbServer.HardwareData.System,
				Load:    dbServer.HardwareData.Load,
			},
			VersionData: dbServer.VersionData,
			LastUpdated: dbServer.LastUpdated,
		})
	}

	return servers, nil
}

func (s server) GetAllNames(ctx context.Context) ([]string, error) {
	// TODO: fix overfetching, we don't need all the servers, we only need the names
	dbServers, err := entities.GetServers(ctx, s.db)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(dbServers))
	for _, cluster := range dbServers {
		names = append(names, cluster.Name)
	}

	return names, nil
}

func (s server) GetByName(ctx context.Context, name string) (provisioning.Server, error) {
	dbServer, err := entities.GetServer(ctx, s.db, name)
	if err != nil {
		return provisioning.Server{}, err
	}

	return provisioning.Server{
		Cluster:       dbServer.Cluster,
		Name:          dbServer.Name,
		Type:          dbServer.Type,
		ConnectionURL: dbServer.ConnectionURL,
		HardwareData: api.Resources{
			CPU:     dbServer.HardwareData.CPU,
			Memory:  dbServer.HardwareData.Memory,
			GPU:     dbServer.HardwareData.GPU,
			Network: dbServer.HardwareData.Network,
			Storage: dbServer.HardwareData.Storage,
			USB:     dbServer.HardwareData.USB,
			PCI:     dbServer.HardwareData.PCI,
			System:  dbServer.HardwareData.System,
			Load:    dbServer.HardwareData.Load,
		},
		VersionData: dbServer.VersionData,
		LastUpdated: dbServer.LastUpdated,
	}, nil
}

func (s server) UpdateByName(ctx context.Context, name string, in provisioning.Server) (provisioning.Server, error) {
	err := transaction.ForceTx(ctx, s.db, func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateServer(ctx, tx, name, entities.Server{
			Cluster: in.Cluster,
			Name:    in.Name,
			Type:    in.Type,
			HardwareData: entities.MarshalableResource{
				CPU:     in.HardwareData.CPU,
				Memory:  in.HardwareData.Memory,
				GPU:     in.HardwareData.GPU,
				Network: in.HardwareData.Network,
				Storage: in.HardwareData.Storage,
				USB:     in.HardwareData.USB,
				PCI:     in.HardwareData.PCI,
				System:  in.HardwareData.System,
				Load:    in.HardwareData.Load,
			},
			VersionData:   in.VersionData,
			ConnectionURL: in.ConnectionURL,
			LastUpdated:   in.LastUpdated,
		})
	})
	in.Name = name
	return in, err
}

func (s server) Rename(ctx context.Context, name string, to string) error {
	return entities.RenameCluster(ctx, s.db, name, to)
}

func (s server) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteServer(ctx, s.db, name)
}
