package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
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
	return s.getAllWithFilter(ctx, nil)
}

func (s server) GetAllWithFilter(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
	return s.getAllWithFilter(ctx, &filter)
}

func (s server) getAllWithFilter(ctx context.Context, filter *provisioning.ServerFilter) (provisioning.Servers, error) {
	var servers provisioning.Servers
	var err error

	if filter == nil {
		servers, err = entities.GetServers(ctx, transaction.GetDBTX(ctx, s.db))
	} else {
		servers, err = entities.GetServers(ctx, transaction.GetDBTX(ctx, s.db), *filter)
	}

	if err != nil {
		return nil, err
	}

	var errs []error
	for i := range servers {
		if servers[i].Certificate == nil {
			continue
		}

		servers[i].Fingerprint, err = incustls.CertFingerprintStr(*servers[i].Certificate)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return servers, errors.Join(errs...)
}

func (s server) GetAllNames(ctx context.Context) ([]string, error) {
	return entities.GetServerNames(ctx, transaction.GetDBTX(ctx, s.db))
}

func (s server) GetAllNamesWithFilter(ctx context.Context, filter provisioning.ServerFilter) ([]string, error) {
	return entities.GetServerNames(ctx, transaction.GetDBTX(ctx, s.db), filter)
}

// serverNamesWithActiveDeploymentStmt selects the servers, that still have an
// automated deployment to advance, based on the deployment state (and not
// the server state).
var serverNamesWithActiveDeploymentStmt = fmt.Sprintf(`
SELECT servers.name
  FROM servers
  WHERE json_extract(servers.status_internal, '$.deployment.state') IS NOT NULL
    AND json_extract(servers.status_internal, '$.deployment.state') NOT IN (%s)
  ORDER BY servers.name
`, strings.TrimSuffix(strings.Repeat("?, ", len(api.ServerDeploymentTerminalStates())), ", "))

func (s server) GetAllNamesWithActiveDeployment(ctx context.Context) ([]string, error) {
	terminalStates := api.ServerDeploymentTerminalStates()

	args := make([]any, 0, len(terminalStates))
	for _, state := range terminalStates {
		args = append(args, state.String())
	}

	rows, err := transaction.GetDBTX(ctx, s.db).QueryContext(ctx, serverNamesWithActiveDeploymentStmt, args...)
	if err != nil {
		return nil, fmt.Errorf("Failed to get the names of the servers with an active deployment: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var names []string

	for rows.Next() {
		var name string

		err = rows.Scan(&name)
		if err != nil {
			return nil, fmt.Errorf("Failed to get the names of the servers with an active deployment: %w", err)
		}

		names = append(names, name)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("Failed to get the names of the servers with an active deployment: %w", err)
	}

	return names, nil
}

func (s server) GetByName(ctx context.Context, name string) (*provisioning.Server, error) {
	server, err := entities.GetServer(ctx, transaction.GetDBTX(ctx, s.db), name)
	if err != nil {
		return nil, err
	}

	if server.Certificate == nil {
		return server, nil
	}

	server.Fingerprint, err = incustls.CertFingerprintStr(*server.Certificate)
	if err != nil {
		return nil, err
	}

	return server, nil
}

func (s server) GetByCertificate(ctx context.Context, certificatePEM string) (*provisioning.Server, error) {
	if certificatePEM == "" {
		return nil, domain.ErrNotFound
	}

	servers, err := s.getAllWithFilter(ctx, &provisioning.ServerFilter{
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

func (s server) GetBySystemUUID(ctx context.Context, systemUUID string) (*provisioning.Server, error) {
	servers, err := s.getAllWithFilter(ctx, &provisioning.ServerFilter{
		SystemUUID: &systemUUID,
	})
	if err != nil {
		return nil, err
	}

	if len(servers) == 0 {
		return nil, domain.ErrNotFound
	}

	if len(servers) != 1 {
		return nil, fmt.Errorf("More than one server matches the system UUID") // this should never happen, since we have a unique constraint on column servers.certificate in the database.
	}

	return &servers[0], nil
}

func (s server) GetByMachineID(ctx context.Context, machineID string) (*provisioning.Server, error) {
	servers, err := s.getAllWithFilter(ctx, &provisioning.ServerFilter{
		MachineID: &machineID,
	})
	if err != nil {
		return nil, err
	}

	if len(servers) == 0 {
		return nil, domain.ErrNotFound
	}

	if len(servers) != 1 {
		return nil, fmt.Errorf("More than one server matches the machine-id") // this should never happen, since we have a unique constraint on column servers.certificate in the database.
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
