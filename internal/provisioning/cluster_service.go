package provisioning

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	"github.com/maniartech/signals"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

type clusterService struct {
	repo             ClusterRepo
	localartifact    ClusterArtifactRepo
	client           ClusterClientPort
	serverSvc        ServerService
	tokenSvc         TokenService
	inventorySyncers map[domain.ResourceType]InventorySyncer
	provisioner      ClusterProvisioningPort

	createClusterRetries      int
	createClusterRetryTimeout time.Duration

	clusterUpdateSignal               signals.Signal[ClusterUpdateMessage]
	lifecycleEventHandlerBackoffStart time.Duration
	lifecycleEventHandlerBackoffLimit time.Duration
}

var _ ClusterService = &clusterService{}

type ClusterServiceOption func(s *clusterService)

func ClusterServiceCreateClusterRetryTimeout(timeout time.Duration) ClusterServiceOption {
	return func(s *clusterService) {
		s.createClusterRetryTimeout = timeout
	}
}

func ClusterServiceUpdateSignal(updateSignal signals.Signal[ClusterUpdateMessage]) ClusterServiceOption {
	return func(s *clusterService) {
		s.clusterUpdateSignal = updateSignal
	}
}

func NewClusterService(
	repo ClusterRepo,
	localartifact ClusterArtifactRepo,
	client ClusterClientPort,
	serverSvc ServerService,
	tokenSvc TokenService,
	inventorySyncers map[domain.ResourceType]InventorySyncer,
	provisioner ClusterProvisioningPort,
	opts ...ClusterServiceOption,
) *clusterService {
	clusterSvc := &clusterService{
		repo:             repo,
		localartifact:    localartifact,
		client:           client,
		serverSvc:        serverSvc,
		tokenSvc:         tokenSvc,
		inventorySyncers: inventorySyncers,
		provisioner:      provisioner,

		createClusterRetries:      6,
		createClusterRetryTimeout: 200 * time.Millisecond,

		clusterUpdateSignal:               signals.NewSync[ClusterUpdateMessage](),
		lifecycleEventHandlerBackoffStart: 200 * time.Millisecond,
		lifecycleEventHandlerBackoffLimit: 60 * time.Second,
	}

	for _, opt := range opts {
		opt(clusterSvc)
	}

	return clusterSvc
}

func (s *clusterService) SetInventorySyncers(inventorySyncers map[domain.ResourceType]InventorySyncer) {
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
//
// Perform post-clustering initialization using provisioner (Terraform):
//   - Create internal project
//   - Initialize default storage:
//     Create local storage pool on each server and finalize it for the cluster.
//     Create two volumes on that pool on each server named images and backups.
//     Set storage.images_volume and storage.backups_volume on each server to point to the volumes.
//     Update the default profile in the default project to use the local storage pool.
//     Update the default profile in the internal project to use the local storage pool.
//   - Initialize default networking:
//     Create local network bridge "incusbr0" on each server.
//     Create an "internal" network bridge on each server.
//     Update the default profile in the default project to use incusbr0 for networking.
//     Update the default profile in the internal project to use internal-mesh for networking.
func (s clusterService) Create(ctx context.Context, newCluster Cluster) (_ Cluster, err error) {
	err = newCluster.ValidateCreate()
	if err != nil {
		return Cluster{}, err
	}

	var bootstrapServer Server
	var servers []Server

	// 1st DB transaction.
	err = transaction.Do(ctx, func(ctx context.Context) error {
		// Ensure there is no name conflict for the new cluster.
		exists, err := s.repo.ExistsByName(ctx, newCluster.Name)
		if err != nil {
			return fmt.Errorf("Error while verifying cluster name: %w", err)
		}

		if exists {
			return fmt.Errorf("Cluster with name %q already exists: %w", newCluster.Name, domain.ErrOperationNotPermitted)
		}

		// Validate all listed servers are already known.
		for _, serverName := range newCluster.ServerNames {
			server, err := s.serverSvc.GetByName(ctx, serverName)
			if err != nil {
				return err
			}

			if server.Cluster != nil {
				return fmt.Errorf("Server %q is already part of cluster %q: %w", serverName, *server.Cluster, domain.ErrOperationNotPermitted)
			}

			servers = append(servers, *server)
		}

		// Select first server as the bootstrap server.
		bootstrapServer = servers[0]

		// Create Cluster record in pending state in the repo.
		newCluster.Status = api.ClusterStatusPending

		newCluster.ID, err = s.repo.Create(ctx, newCluster)
		if err != nil {
			return fmt.Errorf("Failed to create cluster record in the repository: %w", err)
		}

		return nil
	})
	if err != nil {
		return newCluster, err
	}

	// Verify, that all the servers that are clustered have the expected server type.
	for _, server := range servers {
		if server.Type != newCluster.ServerType {
			return newCluster, fmt.Errorf("Server %q has type %q but %q was expected: %w", server.Name, server.Type, newCluster.ServerType, domain.ErrOperationNotPermitted)
		}
	}

	// Perform pre-clustering and clustering API calls.

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
		for service, configAny := range newCluster.ServicesConfig {
			config, ok := configAny.(map[string]any)
			if !ok {
				return newCluster, fmt.Errorf("Failed to enable OS service %q on %q: config is not an object", service, server.Name)
			}

			// LVM system_id is controlled by Operations Center and not the user.
			// system_id is required to be between 1 and 2000. Just using the server.ID
			// will fail, when we hit values > 2000.
			if service == "lvm" {
				enabledAny := config["enabled"]
				enabled, ok := enabledAny.(bool)
				if !ok {
					return newCluster, fmt.Errorf(`Failed to enable OS service "lvm" on %q: "enabled" is not a bool`, server.Name)
				}

				if enabled {
					if server.ID > 2000 {
						return newCluster, fmt.Errorf(`Failed to enable OS service "lvm" on %q: can not enable LVM on servers with internal ID > 2000`, server.Name)
					}

					config["system_id"] = server.ID
				}
			}

			err = s.client.EnableOSService(ctx, server, service, config)
			if err != nil {
				return newCluster, fmt.Errorf("Failed to enable OS service %q on %q: %w", service, server.Name, err)
			}
		}

		// Ignore error, connection URL has been parsed by incus client already.
		serverAddressURL, _ := url.Parse(server.ConnectionURL)

		err = s.client.SetServerConfig(ctx, server, map[string]string{
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
	clusterEndpoint := ClusterEndpoint{
		Server{
			ConnectionURL:        bootstrapServer.ConnectionURL,
			Cluster:              &newCluster.Name,
			ClusterCertificate:   &clusterCertificate,
			ClusterConnectionURL: &bootstrapServer.ConnectionURL,
		},
	}

	// Ensure, that the bootstrap server has joined the cluster.
	var i int
	for i = range s.createClusterRetries {
		var nodeNames []string
		nodeNames, err = s.client.GetClusterNodeNames(ctx, clusterEndpoint)
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
		joinToken, err := s.client.GetClusterJoinToken(ctx, clusterEndpoint, server.Name)
		if err != nil {
			return newCluster, fmt.Errorf("Failed to get cluster join token from cluster %q (bootstrap server: %s) for server %q: %w", newCluster.Name, bootstrapServer.ConnectionURL, server.Name, err)
		}

		joinTokens = append(joinTokens, joinToken)
	}

	// Send the join tokens to the remaining servers to join the cluster.
	for i, server := range servers[1:] {
		err := s.client.JoinCluster(ctx, server, joinTokens[i], clusterEndpoint)
		if err != nil {
			return newCluster, fmt.Errorf("Failed to join cluster on %q: %w", server.Name, err)
		}
	}

	// Update server records for further use.
	for i := range servers {
		servers[i].Cluster = &newCluster.Name
		servers[i].ClusterCertificate = &clusterCertificate
	}

	// 2nd DB transaction.
	err = transaction.Do(ctx, func(ctx context.Context) error {
		// Validate again all listed servers are not yet part of cluster.
		for _, server := range servers {
			server, err := s.serverSvc.GetByName(ctx, server.Name)
			if err != nil {
				return err
			}

			if server.Cluster != nil {
				return fmt.Errorf("Server %q was not part of a cluster, but is now part of %q: %w", server.Name, *server.Cluster, domain.ErrOperationNotPermitted)
			}
		}

		// Update cluster entry in the repo, set state to ready and certificate.
		newCluster.Status = api.ClusterStatusReady
		newCluster.Certificate = &clusterCertificate

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

	// Refresh OS Data, required for the detection of the network interface for
	// the internal mesh.
	for i, server := range servers {
		osData, err := s.client.GetOSData(ctx, server)
		if err != nil {
			return newCluster, err
		}

		servers[i].OSData = osData
	}

	// Perform post-clustering initialization using provisioner (Terraform).
	temporaryPath, cleanup, err := s.provisioner.Init(ctx, newCluster.Name, ClusterProvisioningConfig{
		ClusterEndpoint:       clusterEndpoint,
		Servers:               servers,
		ApplicationSeedConfig: newCluster.ApplicationSeedConfig,
	})
	if err != nil {
		return newCluster, err
	}

	defer func() {
		err = errors.Join(err, cleanup())
	}()

	err = s.provisioner.Apply(ctx, newCluster)
	if err != nil {
		return newCluster, err
	}

	_, err = s.localartifact.CreateClusterArtifactFromPath(ctx, ClusterArtifact{
		Cluster:     newCluster.Name,
		Name:        "terraform-configuration",
		Description: "Initial terraform configuration used for post-clustering.",
	}, temporaryPath, []string{".terraform.lock.hcl"})
	if err != nil {
		return newCluster, err
	}

	err = s.ResyncInventoryByName(ctx, newCluster.Name)
	if err != nil {
		slog.WarnContext(ctx, "Post cluster creation inventory sync failed", logger.Err(err))
	}

	s.clusterUpdateSignal.Emit(ctx, ClusterUpdateMessage{
		Operation: ClusterUpdateOperationCreate,
		Name:      newCluster.Name,
	})

	return newCluster, nil
}

func (s clusterService) GetAll(ctx context.Context) (Clusters, error) {
	return s.repo.GetAll(ctx)
}

func (s clusterService) GetAllWithFilter(ctx context.Context, filter ClusterFilter) (Clusters, error) {
	var filterExpression *vm.Program
	var err error

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(ToExprCluster(Cluster{}))}...)
		if err != nil {
			return nil, domain.NewValidationErrf("Failed to compile filter expression: %v", err)
		}
	}

	clusters, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	var filteredClusters Clusters
	if filter.Expression != nil {
		for _, cluster := range clusters {
			output, err := expr.Run(filterExpression, ToExprCluster(cluster))
			if err != nil {
				return nil, domain.NewValidationErrf("Failed to execute filter expression: %v", err)
			}

			result, ok := output.(bool)
			if !ok {
				return nil, domain.NewValidationErrf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
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
		Name string `expr:"name"`
	}

	if filter.Expression != nil {
		filterExpression, err = expr.Compile(*filter.Expression, []expr.Option{expr.Env(Env{})}...)
		if err != nil {
			return nil, domain.NewValidationErrf("Failed to compile filter expression: %v", err)
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
				return nil, domain.NewValidationErrf("Failed to execute filter expression: %v", err)
			}

			result, ok := output.(bool)
			if !ok {
				return nil, domain.NewValidationErrf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
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

	return s.repo.Update(ctx, newCluster)
}

func (s clusterService) Rename(ctx context.Context, oldName string, newName string) error {
	if oldName == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if newName == "" {
		return domain.NewValidationErrf("New Cluster name cannot by empty")
	}

	err := s.repo.Rename(ctx, oldName, newName)
	if err != nil {
		return err
	}

	s.clusterUpdateSignal.Emit(ctx, ClusterUpdateMessage{
		Operation: ClusterUpdateOperationRename,
		Name:      newName,
		OldName:   oldName,
	})

	return nil
}

func (s clusterService) DeleteByName(ctx context.Context, name string, force bool) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// forceful delete
	if force {
		err := s.repo.DeleteByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete cluster: %w", err)
		}

		s.clusterUpdateSignal.Emit(ctx, ClusterUpdateMessage{
			Operation: ClusterUpdateOperationDelete,
			Name:      name,
		})

		return nil
	}

	// normal delete
	err := transaction.Do(ctx, func(ctx context.Context) error {
		cluster, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete cluster: %w", err)
		}

		switch cluster.Status {
		case api.ClusterStatusUnknown,
			api.ClusterStatusPending:
			// delete is fine
		case api.ClusterStatusReady:
			return fmt.Errorf("Delete for cluster in state %q: %w", cluster.Status.String(), domain.ErrOperationNotPermitted)

		default:
			return fmt.Errorf("Delete for cluster with invalid state: %w", domain.ErrOperationNotPermitted)
		}

		servers, err := s.serverSvc.GetAllNamesWithFilter(ctx, ServerFilter{
			Cluster: &name,
		})
		if err != nil {
			return fmt.Errorf("Failed to get servers linked with cluster: %w", err)
		}

		if len(servers) > 0 {
			return fmt.Errorf("Delete for cluster with %d linked servers (%v): %w", len(servers), servers, domain.ErrOperationNotPermitted)
		}

		err = s.repo.DeleteByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to delete cluster: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("Failed to delete cluster: %w", err)
	}

	s.clusterUpdateSignal.Emit(ctx, ClusterUpdateMessage{
		Operation: ClusterUpdateOperationDelete,
		Name:      name,
	})

	return nil
}

func (s clusterService) DeleteAndFactoryResetByName(ctx context.Context, name string, tokenID *uuid.UUID, tokenSeedName *string) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	servers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
		Cluster: ptr.To(name),
	})
	if err != nil {
		return fmt.Errorf("Get cluster servers for factory reset: %w", err)
	}

	if len(servers) == 0 {
		return fmt.Errorf("Cluster not found")
	}

	for _, server := range servers {
		err = s.client.Ping(ctx, server)
		if err != nil {
			return fmt.Errorf("Pre factory reset connection test to server %s: %w", server.Name, err)
		}
	}

	var seed TokenImageSeedConfigs
	if tokenID != nil && tokenSeedName != nil {
		tokenSeed, err := s.tokenSvc.GetTokenSeedByName(ctx, *tokenID, *tokenSeedName)
		if err != nil {
			return fmt.Errorf("Pre factory reset failed to get token seed: %w", err)
		}

		seed = tokenSeed.Seeds
	}

	if tokenID == nil {
		token, err := s.tokenSvc.Create(ctx, Token{
			UsesRemaining: len(servers),
			ExpireAt:      time.Now().Add(1 * time.Hour),
		})
		if err != nil {
			return fmt.Errorf("Pre factory reset failed to get a provisioning token: %w", err)
		}

		tokenID = &token.UUID
	}

	if tokenSeedName == nil {
		seed = TokenImageSeedConfigs{
			Applications: map[string]any{
				"version": "1",
				"applications": []any{
					map[string]any{
						"name": "incus",
					},
				},
			},
			Incus: map[string]any{
				"version":        "1",
				"apply_defaults": false,
			},
		}
	}

	providerConfig, err := s.tokenSvc.GetTokenProviderConfig(ctx, *tokenID)
	if err != nil {
		return fmt.Errorf("Pre factory reset failed to get provider config: %w", err)
	}

	for _, server := range servers {
		// TODO: First try with allowTPMResetFailure = false and later retry with true, if an error occurs. Print an warning in this case.
		err = s.client.SystemFactoryReset(ctx, server, false, seed, *providerConfig)
		if err != nil {
			return fmt.Errorf("Factory reset on server %s: %w", server.Name, err)
		}
	}

	err = s.repo.DeleteByName(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to delete cluster: %w", err)
	}

	s.clusterUpdateSignal.Emit(ctx, ClusterUpdateMessage{
		Operation: ClusterUpdateOperationDelete,
		Name:      name,
	})

	return nil
}

func (s clusterService) ResyncInventory(ctx context.Context) error {
	clusters, err := s.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get clusters while resyncing the inventory: %w", err)
	}

	for _, cluster := range clusters {
		// Exit early, if context is done.
		err = ctx.Err()
		if err != nil {
			return fmt.Errorf("Failed to resync inventory: %w", err)
		}

		err = s.ResyncInventoryByName(ctx, cluster.Name)
		if err != nil {
			return fmt.Errorf("Failed to resync inventory: %w", err)
		}
	}

	return nil
}

func (s clusterService) ResyncInventoryByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	// We iterate a map, so the order is random. But this should not be an issue
	// since there are no constraints in the DB between the different resource
	// types. The data in the DB will become eventually consistent after the
	// sync is completed.
	for _, inventorySyncer := range s.inventorySyncers {
		err := inventorySyncer.SyncCluster(ctx, name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s clusterService) StartLifecycleEventsMonitor(ctx context.Context) error {
	clusters, err := s.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to initially load clusters for lifecycle events monitor: %w", err)
	}

	lifecycleMonitors := make(map[string]context.CancelFunc, len(clusters))
	lifecycleMonitorsMu := sync.Mutex{}

	lifecycleMonitorsMu.Lock()
	defer lifecycleMonitorsMu.Unlock()

	for _, cluster := range clusters {
		cancel, err := s.startLifecycleEventHandler(ctx, cluster.Name)
		if err != nil {
			slog.WarnContext(ctx, "Failed to start lifecycle monitor", slog.String("cluster", cluster.Name), logger.Err(err))
			continue
		}

		lifecycleMonitors[cluster.Name] = cancel
	}

	s.clusterUpdateSignal.AddListener(func(ctx context.Context, cum ClusterUpdateMessage) {
		lifecycleMonitorsMu.Lock()
		defer lifecycleMonitorsMu.Unlock()

		switch cum.Operation {
		case ClusterUpdateOperationCreate:
			cancel, err := s.startLifecycleEventHandler(context.Background(), cum.Name)
			if err != nil {
				slog.WarnContext(ctx, "Failed to start lifecycle monitor", slog.String("cluster", cum.Name), logger.Err(err))
				return
			}

			lifecycleMonitors[cum.Name] = cancel

		case ClusterUpdateOperationDelete:
			cancel, ok := lifecycleMonitors[cum.Name]
			if !ok {
				return
			}

			cancel()
			delete(lifecycleMonitors, cum.Name)
		}
	})

	return nil
}

func (s clusterService) startLifecycleEventHandler(ctx context.Context, clusterName string) (context.CancelFunc, error) {
	endpoint, err := s.GetEndpoint(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster endpoint for lifecycle event handler: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		for {
			var events chan domain.LifecycleEvent
			var errChan chan error
			var err error

		retry:
			for backoff := range exponentialBackoff(s.lifecycleEventHandlerBackoffStart, s.lifecycleEventHandlerBackoffLimit) {
				events, errChan, err = s.client.SubscribeLifecycleEvents(ctx, endpoint)
				if err == nil {
					// Event stream re-established, break retry loop and start processing.
					break
				}

				slog.WarnContext(ctx, "Failed to re-establish event stream", slog.String("cluster", clusterName), logger.Err(err))

				select {
				case <-time.After(backoff):
					continue retry

				case <-ctx.Done():
					return
				}
			}

		process:
			for {
				select {
				case event := <-events:
					slog.InfoContext(ctx, "lifecycle event", slog.String("cluster", clusterName), slog.Any("action", event.Operation), slog.Any("resource_type", event.ResourceType), slog.String("source", event.Source.String()))

					inventorySyncer, ok := s.inventorySyncers[event.ResourceType]
					if !ok {
						slog.WarnContext(ctx, "No inventory syncer available for the resource type", slog.String("cluster", clusterName), slog.String("action", string(event.Operation)), slog.Any("resource_type", event.ResourceType), slog.String("source", event.Source.String()))
						continue
					}

					err := inventorySyncer.ResyncByName(ctx, clusterName, event)
					if err != nil {
						slog.WarnContext(ctx, "Failed to resync", slog.String("cluster", clusterName), slog.String("action", string(event.Operation)), slog.Any("resource_type", event.ResourceType), slog.String("source", event.Source.String()), logger.Err(err))
					}

				case err := <-errChan:
					if err != nil {
						slog.WarnContext(ctx, "Lifecycle events subscription ended", logger.Err(err))
					}

					break process

				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return cancel, nil
}

func exponentialBackoff(start time.Duration, limit time.Duration) iter.Seq[time.Duration] {
	return func(yield func(time.Duration) bool) {
		for {
			if !yield(start) {
				return
			}

			start = min(start*2, limit)
		}
	}
}

func (s clusterService) UpdateCertificate(ctx context.Context, name string, certificatePEM string, keyPEM string) error {
	_, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
	if err != nil {
		return domain.NewValidationErrf("Failed to validate key pair: %v", err)
	}

	endpoint, err := s.GetEndpoint(ctx, name)
	if err != nil {
		return fmt.Errorf("Failed to get cluster endpoint for certificate update: %w", err)
	}

	err = s.client.UpdateClusterCertificate(ctx, endpoint, certificatePEM, keyPEM)
	if err != nil {
		return fmt.Errorf("Failed to update cluster certificate: %w", err)
	}

	return transaction.Do(ctx, func(ctx context.Context) error {
		cluster, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get cluster for certificate update: %w", err)
		}

		cluster.Certificate = &certificatePEM

		err = s.repo.Update(ctx, *cluster)
		if err != nil {
			return fmt.Errorf("Failed to persist updated cluster certificate: %w", err)
		}

		return nil
	})
}

func (s clusterService) GetEndpoint(ctx context.Context, name string) (Endpoint, error) {
	servers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
		Cluster: &name,
	})
	if err != nil {
		return ClusterEndpoint{}, err
	}

	return ClusterEndpoint(servers), nil
}

func (s clusterService) GetClusterArtifactAll(ctx context.Context, clusterName string) (ClusterArtifacts, error) {
	if clusterName == "" {
		return nil, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.localartifact.GetClusterArtifactAll(ctx, clusterName)
}

func (s clusterService) GetClusterArtifactAllNames(ctx context.Context, clusterName string) ([]string, error) {
	if clusterName == "" {
		return nil, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.localartifact.GetClusterArtifactAllNames(ctx, clusterName)
}

func (s clusterService) GetClusterArtifactByName(ctx context.Context, clusterName string, artifactName string) (*ClusterArtifact, error) {
	if clusterName == "" {
		return nil, fmt.Errorf("Cluster name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	if artifactName == "" {
		return nil, fmt.Errorf("Cluster artifact name cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	return s.localartifact.GetClusterArtifactByName(ctx, clusterName, artifactName)
}

func (s clusterService) GetClusterArtifactFileByName(ctx context.Context, clusterName string, artifactName string, filename string) (*ClusterArtifactFile, error) {
	if filename == "" {
		return nil, fmt.Errorf("Filename cannot be empty: %w", domain.ErrOperationNotPermitted)
	}

	artifact, err := s.GetClusterArtifactByName(ctx, clusterName, artifactName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get artifact %q for cluster %q: %w", artifactName, clusterName, err)
	}

	for _, file := range artifact.Files {
		if file.Name == filename {
			return &file, nil
		}
	}

	return nil, fmt.Errorf("File %q not found in artifact %q for cluster %q: %w", filename, artifactName, clusterName, domain.ErrNotFound)
}

func (s clusterService) GetClusterArtifactArchiveByName(ctx context.Context, clusterName string, artifactName string, archiveType ClusterArtifactArchiveType) (_ io.ReadCloser, size int, _ error) {
	rc, size, err := s.localartifact.GetClusterArtifactArchiveByName(ctx, clusterName, artifactName, archiveType)
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to get artifact %q for cluster %q: %w", artifactName, clusterName, err)
	}

	return rc, size, nil
}
