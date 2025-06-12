package sqlite

import (
	"context"
	"fmt"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sqlite"
	"github.com/FuturFusion/operations-center/internal/transaction"
)

type server struct {
	db sqlite.DBTX
}

var _ provisioning.ServerRepo = &server{}

func NewServer(db sqlite.DBTX) *server {
	return &server{
		db: db,
	}
}

func (s server) Create(ctx context.Context, in provisioning.Server) (int64, error) {
	return entities.CreateServer(ctx, transaction.GetDBTX(ctx, s.db), in)
}

func (s server) GetAll(ctx context.Context) (provisioning.Servers, error) {
	return entities.GetServers(ctx, transaction.GetDBTX(ctx, s.db))
}

func (s server) GetAllWithFilter(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
	return entities.GetServers(ctx, transaction.GetDBTX(ctx, s.db), filter)
}

func (s server) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetServerNames(ctx, transaction.GetDBTX(ctx, s.db))
}

func (s server) GetAllNamesWithFilter(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
	return entities.GetServerNames(ctx, transaction.GetDBTX(ctx, s.db), filter)
}

func (s server) GetByName(ctx context.Context, name string) (*provisioning.Server, error) {
	return entities.GetServer(ctx, transaction.GetDBTX(ctx, s.db), name)
}

func (s server) GetByCertificate(ctx context.Context, certificatePEM string) (*provisioning.Server, error) {
	servers, err := entities.GetServers(ctx, transaction.GetDBTX(ctx, s.db), provisioning.ServerFilter{
		Certificate: &certificatePEM,
	})
	if err != nil {
		return nil, err
	}

	if len(servers) == 0 {
		return nil, domain.ErrNotFound
	}

	if len(servers) != 1 {
		return nil, fmt.Errorf("More than one server matches the certificate") // this should never happen, since we have a unique constraint on column servers.certificate in the database.
	}

	return &servers[0], nil
}

func (s server) Update(ctx context.Context, in provisioning.Server) error {
	return transaction.ForceTx(ctx, transaction.GetDBTX(ctx, s.db), func(ctx context.Context, tx transaction.TX) error {
		return entities.UpdateServer(ctx, tx, in.Name, in)
	})
}

func (s server) Rename(ctx context.Context, oldName string, newName string) error {
	return entities.RenameServer(ctx, transaction.GetDBTX(ctx, s.db), oldName, newName)
}

func (s server) DeleteByName(ctx context.Context, name string) error {
	return entities.DeleteServer(ctx, transaction.GetDBTX(ctx, s.db), name)
}
