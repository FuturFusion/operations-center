package provisioning

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type clusterService struct {
	repo             ClusterRepo
	client           ClusterClientPort
	serverSvc        ServerService
	inventorySyncers []InventorySyncer

	now                       func() time.Time
	createClusterRetries      int
	createClusterRetryTimeout time.Duration
}

var _ ClusterService = &clusterService{}

type ClusterServiceOption func(s *clusterService)

func ClusterServiceWithNow(nowFunc func() time.Time) ClusterServiceOption {
	return func(s *clusterService) {
		s.now = nowFunc
	}
}

func ClusterServiceCreateClusterRetryTimeout(timeout time.Duration) ClusterServiceOption {
	return func(s *clusterService) {
		s.createClusterRetryTimeout = timeout
	}
}

func NewClusterService(repo ClusterRepo, client ClusterClientPort, serverSvc ServerService, inventorySyncers []InventorySyncer, opts ...ClusterServiceOption) *clusterService {
	clusterSvc := &clusterService{
		repo:             repo,
		client:           client,
		serverSvc:        serverSvc,
		inventorySyncers: inventorySyncers,

		now:                       time.Now,
		createClusterRetries:      6,
		createClusterRetryTimeout: 200 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(clusterSvc)
	}

	return clusterSvc
}

func (s *clusterService) SetInventorySyncers(inventorySyncers []InventorySyncer) {
	(*s).inventorySyncers = inventorySyncers
}

// Create forms a new Incus cluster from servers which previously registered themselves
// in Operations Center. The process has the following phases:
//
// 1st DB transaction:
//   - Ensure name of the cluster is not taken.
//   - Create a pending cluster entry to reserve the name.
//   - Fetch server IDs to configure the LVM cluster.
//
// Perform pre-clustering and clustering API calls.
//
// 2nd DB transaction:
//   - Update cluster entry with certificate and mark the cluster as ready.
//   - Update server entries by linking them with the cluster.
func (s clusterService) Create(ctx context.Context, newCluster Cluster) (Cluster, error) {
	err := newCluster.Validate()
	if err != nil {
		return Cluster{}, err
	}

	var bootstrapServer Server
	var servers []Server

	err = transaction.Do(ctx, func(ctx context.Context) error {
		// Ensure there is no name conflict for the new cluster.
		exists, err := s.repo.ExistsByName(ctx, newCluster.Name)
		if err != nil {
			return fmt.Errorf("Error while verifying cluster name: %w", err)
		}

		if exists {
			return fmt.Errorf("Cluster with name %q already exists", newCluster.Name)
		}

		// Validate all listed servers are already known.
		for _, serverName := range newCluster.ServerNames {
			server, err := s.serverSvc.GetByName(ctx, serverName)
			if err != nil {
				return err
			}

			if server.Cluster != nil {
				return fmt.Errorf("Server %q is already part of cluster %q", serverName, *server.Cluster)
			}

			servers = append(servers, *server)
		}

		// Select first server as the bootstrap server.
		bootstrapServer = servers[0]

		// Create Cluster record in pending state in the repo.
		newCluster.Status = api.ClusterStatusPending
		newCluster.ConnectionURL = bootstrapServer.ConnectionURL
		newCluster.LastUpdated = s.now()

		newCluster.ID, err = s.repo.Create(ctx, newCluster)
		if err != nil {
			return fmt.Errorf("Failed to create cluster record in the repository: %w", err)
		}

		return nil
	})
	if err != nil {
		return newCluster, err
	}

	// Check, that all the listed servers are online.
	for _, server := range servers {
		ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, 1*time.Second)
		err = s.client.Ping(ctxWithTimeout, server)
		cancelFunc()
		if err != nil {
			return newCluster, fmt.Errorf("Connection test for server %q failed: %w", server.Name, err)
		}
	}

	// Push pre-clustering configuration to the servers.
	for _, server := range servers {
		err = s.client.EnableOSServiceLVM(ctx, server)
		if err != nil {
			return newCluster, fmt.Errorf("Failed to enable OS service LVM on %q: %w", server.Name, err)
		}

		// Ignore error, connection URL has been parsed by incus client already.
		serverAddressURL, _ := url.Parse(server.ConnectionURL)

		err = s.client.SetServerConfig(ctx, server, map[string]string{
			"core.https_address":    serverAddressURL.Host, // TODO: Remove once https://github.com/lxc/incus/pull/2218 is available in incus-os.
			"cluster.https_address": serverAddressURL.Host,
		})
		if err != nil {
			return newCluster, fmt.Errorf("Failed to set cluster.https_address on %q: %w", server.Name, err)
		}
	}

	// Bootstrap cluster on bootstrap server (first server of the provided server list).
	clusterCertificate, err := s.client.EnableCluster(ctx, bootstrapServer)
	if err != nil {
		return newCluster, fmt.Errorf("Failed to enable clustering on bootstrap server %q: %w", bootstrapServer.Name, err)
	}

	// From now on, use the cluster certificate to connect to the cluster instead
	// of the certificate of the bootstrap server.
	// Fake "server", which represents the cluster to get the join tokens.
	cluster := Server{
		Name:          newCluster.Name,
		ConnectionURL: bootstrapServer.ConnectionURL,
		Certificate:   clusterCertificate,
	}

	// Ensure, that the bootstrap server has joined the cluster.
	var i int
	for i = range s.createClusterRetries {
		var nodeNames []string
		nodeNames, err = s.client.GetClusterNodeNames(ctx, cluster)
		if err == nil && len(nodeNames) > 0 {
			break
		}

		// TODO: Should also consider context done.
		time.Sleep(s.createClusterRetryTimeout)
	}

	if err != nil {
		return newCluster, fmt.Errorf("Failed to perform connection test to the bootstrap node using the cluster certificate in %d attempts: %w", i, err)
	}

	// Get join tokens on from the cluster, skip the bootstrap server.
	joinTokens := make([]string, 0, len(servers[1:]))
	for _, server := range servers[1:] {
		joinToken, err := s.client.GetClusterJoinToken(ctx, cluster, server.Name)
		if err != nil {
			return newCluster, fmt.Errorf("Failed to get cluster join token from cluster %q (%s) for server %q: %w", cluster.Name, cluster.ConnectionURL, server.Name, err)
		}

		joinTokens = append(joinTokens, joinToken)
	}

	// Send the join tokens to the remaining servers to join the cluster.
	for i, server := range servers[1:] {
		err := s.client.JoinCluster(ctx, server, joinTokens[i], cluster)
		if err != nil {
			return newCluster, fmt.Errorf("Failed to join cluster on %q: %w", server.Name, err)
		}
	}

	// Update server records for further use.
	for i := range servers {
		servers[i].Cluster = &newCluster.Name
		servers[i].ClusterCertificate = &clusterCertificate
	}

	// TODO: Remove once https://github.com/lxc/incus/pull/2218 is available in incus-os.
	// Reset core.https_address to :8443
	for _, server := range servers {
		err = s.client.SetServerConfig(ctx, server, map[string]string{
			"core.https_address": ":8443",
		})
		if err != nil {
			return newCluster, fmt.Errorf("Failed to reset core.https_address on %q: %w", server.Name, err)
		}
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		// Validate again all listed servers are not yet part of cluster.
		for _, server := range servers {
			server, err := s.serverSvc.GetByName(ctx, server.Name)
			if err != nil {
				return err
			}

			if server.Cluster != nil {
				return fmt.Errorf("Server %q was not part of a cluster, but is now part of %q", server.Name, *server.Cluster)
			}
		}

		// Update cluster entry in the repo, set state to ready and certificate.
		newCluster.Status = api.ClusterStatusReady
		newCluster.Certificate = clusterCertificate
		newCluster.LastUpdated = s.now()

		err = s.repo.Update(ctx, newCluster)
		if err != nil {
			return fmt.Errorf("Failed to update cluster record in the repository: %w", err)
		}

		// Update Server records in the repo.
		for _, server := range servers {
			err = s.serverSvc.Update(ctx, server)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return newCluster, err
	}

	return newCluster, nil
}

func (s clusterService) GetAll(ctx context.Context) (Clusters, error) {
	return s.repo.GetAll(ctx)
}

func (s clusterService) GetAllWithFilter(ctx context.Context, filter ClusterFilter) (Clusters, error) {
	var filterExpression *vm.Program
	var err error

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(Cluster{})}...)
		if err != nil {
			return nil, err
		}
	}

	clusters, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var filteredClusters Clusters
	if filter.Expression != nil {
		for _, cluster := range clusters {
			output, err := expr.Run(filterExpression, cluster)
			if err != nil {
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if result {
				filteredClusters = append(filteredClusters, cluster)
			}
		}

		return filteredClusters, nil
	}

	return clusters, nil
}

func (s clusterService) GetAllNames(ctx context.Context) ([]string, error) {
	return s.repo.GetAllNames(ctx)
}

func (s clusterService) GetAllNamesWithFilter(ctx context.Context, filter ClusterFilter) ([]string, error) {
	var filterExpression *vm.Program
	var err error

	type Env struct {
		Name string
	}

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(Env{})}...)
		if err != nil {
			return nil, err
		}
	}

	clusterIDs, err := s.repo.GetAllNames(ctx)
	if err != nil {
		return nil, err
	}

	var filteredClusterIDs []string
	if filter.Expression != nil {
		for _, clusterID := range clusterIDs {
			output, err := expr.Run(filterExpression, Env{clusterID})
			if err != nil {
				return nil, err
			}

			result, ok := output.(bool)
			if !ok {
				return nil, fmt.Errorf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
			}

			if result {
				filteredClusterIDs = append(filteredClusterIDs, clusterID)
			}
		}

		return filteredClusterIDs, nil
	}

	return clusterIDs, nil
}

func (s clusterService) GetByName(ctx context.Context, name string) (*Cluster, error) {
	if name == "" {
		return nil, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.repo.GetByName(ctx, name)
}

func (s clusterService) Update(ctx context.Context, newCluster Cluster) error {
	err := newCluster.Validate()
	if err != nil {
		return err
	}

	newCluster.LastUpdated = s.now()

	return s.repo.Update(ctx, newCluster)
}

func (s clusterService) Rename(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New Cluster name cannot by empty")
	}

	return s.repo.Rename(ctx, oldName, newName)
}

func (s clusterService) DeleteByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// FIXME: deleteting a cluster also requires to delete all the inventory (in a transaction).

	return s.repo.DeleteByName(ctx, name)
}

func (s clusterService) ResyncInventoryByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	for _, inventorySyncer := range s.inventorySyncers {
		err := inventorySyncer.SyncCluster(ctx, name)
		if err != nil {
			return err
		}
	}

	return nil
}
