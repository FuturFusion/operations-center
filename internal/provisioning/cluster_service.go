package provisioning

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus/v6/shared/revert"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/structs"
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

	now func() time.Time

	lifecycleEventHandlerBackoffStart time.Duration
	lifecycleEventHandlerBackoffLimit time.Duration

	clusterUpdatePendingUpdateRecheckInterval time.Duration
}

var _ ClusterService = &clusterService{}

type ClusterServiceOption func(s *clusterService)

func WithClusterServiceCreateClusterRetryTimeout(timeout time.Duration) ClusterServiceOption {
	return func(s *clusterService) {
		s.createClusterRetryTimeout = timeout
	}
}

func WithClusterServiceNow(nowFunc func() time.Time) ClusterServiceOption {
	return func(s *clusterService) {
		s.now = nowFunc
	}
}

func WithClusterServicePendingUpdateRecheckInterval(d time.Duration) ClusterServiceOption {
	return func(s *clusterService) {
		s.clusterUpdatePendingUpdateRecheckInterval = d
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

		now: time.Now,

		lifecycleEventHandlerBackoffStart: 200 * time.Millisecond,
		lifecycleEventHandlerBackoffLimit: 60 * time.Second,

		clusterUpdatePendingUpdateRecheckInterval: 60 * time.Second,
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
	if newCluster.Channel == "" {
		newCluster.Channel = config.GetUpdates().ServerDefaultChannel
	}

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

		// Validate all listed servers are already known and do have configuration
		// valid for clustering.
		for _, serverName := range newCluster.ServerNames {
			server, err := s.serverSvc.GetByName(ctx, serverName)
			if err != nil {
				return err
			}

			if server.Cluster != nil {
				return fmt.Errorf("Server %q is already part of cluster %q: %w", serverName, *server.Cluster, domain.ErrOperationNotPermitted)
			}

			if server.Status != api.ServerStatusReady {
				return fmt.Errorf("Server %q is not in ready state and can therefore not be used for clustering: %w", serverName, domain.ErrOperationNotPermitted)
			}

			if newCluster.Channel != server.Channel {
				return fmt.Errorf("Server %q update channel %q does not match channel requested for cluster %q: %w", server.Name, server.Channel, newCluster.Channel, domain.ErrOperationNotPermitted)
			}

			if ptr.From(server.VersionData.NeedsUpdate) || ptr.From(server.VersionData.NeedsReboot) || ptr.From(server.VersionData.InMaintenance) != api.NotInMaintenance {
				return fmt.Errorf("Server %q not ready to be clustered (needs update: %t, needs reboot: %t, in maintenance: %v): %w", server.Name, ptr.From(server.VersionData.NeedsUpdate), ptr.From(server.VersionData.NeedsReboot), server.VersionData.InMaintenance.String(), domain.ErrOperationNotPermitted)
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
			cfg, ok := configAny.(map[string]any)
			if !ok {
				return newCluster, fmt.Errorf("Failed to enable OS service %q on %q: config is not an object", service, server.Name)
			}

			// LVM system_id is controlled by Operations Center and not the user.
			// system_id is required to be between 1 and 2000. Just using the server.ID
			// will fail, when we hit values > 2000.
			if service == "lvm" {
				enabledAny := cfg["enabled"]
				enabled, ok := enabledAny.(bool)
				if !ok {
					return newCluster, fmt.Errorf(`Failed to enable OS service "lvm" on %q: "enabled" is not a bool`, server.Name)
				}

				if enabled {
					if server.ID > 2000 {
						return newCluster, fmt.Errorf(`Failed to enable OS service "lvm" on %q: can not enable LVM on servers with internal ID > 2000`, server.Name)
					}

					cfg["system_id"] = server.ID
				}
			}

			err = s.client.UpdateOSService(ctx, server, service, cfg)
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

	clusterConnectionURL := determineClusterConnectionURL(bootstrapServer)

	// From now on, use the cluster certificate to connect to the cluster instead
	// of the certificate of the bootstrap server.
	clusterEndpoint := ClusterEndpoint{
		Server{
			ConnectionURL:        clusterConnectionURL,
			Cluster:              &newCluster.Name,
			ClusterCertificate:   &clusterCertificate,
			ClusterConnectionURL: &clusterConnectionURL,
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
		servers[i].ClusterConnectionURL = &newCluster.ConnectionURL
		servers[i].Channel = newCluster.Channel
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
			err = s.serverSvc.Update(ctx, server, true, true)
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
		ClusterEndpoint: clusterEndpoint,
		Servers:         servers,
		Cluster:         newCluster,
	})
	if err != nil {
		return newCluster, err
	}

	defer func() {
		err = errors.Join(err, cleanup())
	}()

	var retryCount int
	for {
		err = s.provisioner.Apply(ctx, newCluster)
		if err != nil {
			var retryableErr domain.ErrRetryable
			if errors.As(err, &retryableErr) {
				retryCount++
				if retryCount > 2 {
					return newCluster, fmt.Errorf("Failed to apply Terraform configuration, retried for %d times: %w", retryCount, err)
				}

				slog.WarnContext(ctx, "Terraform apply failed with a retryable error, will retry", logger.Err(err))

				// Terraform apply fails, when terraform configuration does update the certificate
				// e.g. due to ACME configuration. In this case, the cluster certificate is updated
				// half way through the terraform apply, which causes the client connection in the
				// provider to fail.
				// Therefore we poll the first server, which will cause the cluster certificate to get
				// updated in DB in the case it is now a publicly valid certificate (e.g. ACME).
				// The updated cluster certificate is then fetched from the DB and passed to the
				// terraform provider and terraform apply is retried.
				err := s.serverSvc.PollServer(ctx, servers[0], false)
				if err != nil {
					return newCluster, fmt.Errorf("Failed to poll server %q: %w", servers[0].Name, err)
				}

				updatedServers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
					Cluster: &newCluster.Name,
					Name:    &bootstrapServer.Name,
				})
				if err != nil || len(updatedServers) != 1 {
					return newCluster, fmt.Errorf("Failed to get servers for cluster %q: %w", newCluster.Name, err)
				}

				// After polling the server, we expect the cluster certificate to be empty.
				// If this is not the case, we hit an other issue and we fail.
				if ptr.From(updatedServers[0].ClusterCertificate) != "" {
					return newCluster, fmt.Errorf("Cluster certificate is not nil after polling the server, but we expected a publicly valid certificate")
				}

				newCluster.Certificate = updatedServers[0].ClusterCertificate

				clusterEndpoint = ClusterEndpoint{
					Server{
						ConnectionURL:        updatedServers[0].ConnectionURL,
						Cluster:              &newCluster.Name,
						ClusterCertificate:   updatedServers[0].ClusterCertificate,
						ClusterConnectionURL: &updatedServers[0].ConnectionURL,
					},
				}

				cert, err := s.client.GetRemoteCertificate(ctx, clusterEndpoint)
				if err != nil {
					return newCluster, fmt.Errorf("Failed to get remote certificate for %q: %w", clusterEndpoint.GetConnectionURL(), err)
				}

				certificate := string(pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: cert.Raw,
				}))

				err = s.provisioner.SeedCertificate(ctx, newCluster.Name, certificate)
				if err != nil {
					return newCluster, fmt.Errorf("Failed to update cluster certificate: %w", err)
				}

				continue
			}

			return newCluster, fmt.Errorf("Failed to apply Terraform configuration: %w", err)
		}

		break
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

	lifecycle.ClusterUpdateSignal.Emit(ctx, lifecycle.ClusterUpdateMessage{
		Operation: lifecycle.ClusterUpdateOperationCreate,
		Name:      newCluster.Name,
	})

	return newCluster, nil
}

func determineClusterConnectionURL(server Server) string {
	ip := server.OSData.Network.State.GetInterfaceAddressByRole(incusosapi.SystemNetworkInterfaceRoleCluster)
	if ip == nil {
		return server.ConnectionURL
	}

	return "https://" + net.JoinHostPort(ip.String(), "8443")
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

	var clusters Clusters
	err = transaction.Do(ctx, func(ctx context.Context) error {
		clusters, err = s.repo.GetAll(ctx)
		if err != nil {
			return err
		}

		var filteredClusters Clusters
		if filter.Expression != nil {
			for _, cluster := range clusters {
				output, err := expr.Run(filterExpression, ToExprCluster(cluster))
				if err != nil {
					return domain.NewValidationErrf("Failed to execute filter expression: %v", err)
				}

				result, ok := output.(bool)
				if !ok {
					return domain.NewValidationErrf("Filter expression %q does not evaluate to boolean result: %v", *filter.Expression, output)
				}

				if result {
					filteredClusters = append(filteredClusters, cluster)
				}
			}

			clusters = filteredClusters
		}

		for i := range clusters {
			err = s.getClusterUpdateStatus(ctx, clusters[i].Name, &clusters[i].UpdateStatus)
			if err != nil {
				return fmt.Errorf("Failed to get cluster update status for %q: %w", clusters[i].Name, err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
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

	var cluster *Cluster
	err := transaction.Do(ctx, func(ctx context.Context) error {
		var err error
		cluster, err = s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get cluster %q by name: %w", name, err)
		}

		err = s.getClusterUpdateStatus(ctx, name, &cluster.UpdateStatus)
		if err != nil {
			return fmt.Errorf("Failed to get cluster update status for %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (s clusterService) getClusterUpdateStatus(ctx context.Context, name string, clusterUpdateStatus *api.ClusterUpdateStatus) error {
	servers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
		Cluster: ptr.To(name),
	})
	if err != nil {
		return fmt.Errorf("Failed to get servers for cluster %q: %w", name, err)
	}

	clusterUpdateStatus.NeedsUpdate = make([]string, 0, len(servers))
	clusterUpdateStatus.NeedsReboot = make([]string, 0, len(servers))
	clusterUpdateStatus.InMaintenance = make([]string, 0, len(servers))

	for _, server := range servers {
		if server.VersionData.NeedsUpdate != nil && *server.VersionData.NeedsUpdate {
			clusterUpdateStatus.NeedsUpdate = append(clusterUpdateStatus.NeedsUpdate, server.Name)
		}

		if server.VersionData.NeedsReboot != nil && *server.VersionData.NeedsReboot {
			clusterUpdateStatus.NeedsReboot = append(clusterUpdateStatus.NeedsReboot, server.Name)
		}

		if server.VersionData.InMaintenance != nil && *server.VersionData.InMaintenance != api.NotInMaintenance {
			clusterUpdateStatus.InMaintenance = append(clusterUpdateStatus.InMaintenance, server.Name)
		}
	}

	if clusterUpdateStatus.InProgressStatus.InProgress != api.ClusterUpdateInProgressInactive {
		clusterUpdateStatus.InProgressStatus.StatusDescription = ptr.To(clusterUpdateState(servers))
	}

	return nil
}

func (s clusterService) Update(ctx context.Context, newCluster Cluster) error {
	err := newCluster.Validate()
	if err != nil {
		return err
	}

	err = transaction.Do(ctx, func(ctx context.Context) error {
		err = s.repo.Update(ctx, newCluster)
		if err != nil {
			return err
		}

		// Get servers of cluster and update "channel" to same value as cluster.
		servers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
			Cluster: &newCluster.Name,
		})
		if err != nil {
			return err
		}

		for _, server := range servers {
			server.Channel = newCluster.Channel
			err = s.serverSvc.Update(ctx, server, true, true)
			if err != nil {
				return fmt.Errorf("Failed to update member %q of cluster %q: %w", server.Name, newCluster.Name, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
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

	lifecycle.ClusterUpdateSignal.Emit(ctx, lifecycle.ClusterUpdateMessage{
		Operation: lifecycle.ClusterUpdateOperationRename,
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

		lifecycle.ClusterUpdateSignal.Emit(ctx, lifecycle.ClusterUpdateMessage{
			Operation: lifecycle.ClusterUpdateOperationDelete,
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

	lifecycle.ClusterUpdateSignal.Emit(ctx, lifecycle.ClusterUpdateMessage{
		Operation: lifecycle.ClusterUpdateOperationDelete,
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
			Description:   fmt.Sprintf("Factory reset of cluster %q", name),
			UsesRemaining: len(servers),
			ExpireAt:      time.Now().Add(1 * time.Hour),
			AutoRemove:    true,
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

	lifecycle.ClusterUpdateSignal.Emit(ctx, lifecycle.ClusterUpdateMessage{
		Operation: lifecycle.ClusterUpdateOperationDelete,
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

func (s clusterService) IsInstanceLifecycleOperationPermitted(ctx context.Context, name string) bool {
	if name == "" {
		return true
	}

	cluster, err := s.GetByName(ctx, name)
	if err != nil {
		return false
	}

	return !cluster.IsUpdateInProgress()
}

func (s clusterService) LaunchClusterUpdate(ctx context.Context, name string, reboot bool) error {
	// Check, that no update is in progress for this cluster and set cluster
	// update status to "in progress".
	var cluster *Cluster
	err := transaction.Do(ctx, func(ctx context.Context) error {
		var err error

		cluster, err = s.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get cluster %q: %w", name, err)
		}

		if cluster.IsUpdateInProgress() {
			return fmt.Errorf("Update for cluster %q already in progress: %w", name, domain.ErrOperationNotPermitted)
		}

		cluster.UpdateStatus.InProgressStatus.InProgress = api.ClusterUpdateInProgressApplyUpdate
		cluster.UpdateStatus.InProgressStatus.LastUpdated = s.now()

		err = s.Update(ctx, *cluster)
		if err != nil {
			return fmt.Errorf("Failed to update cluster %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	var servers Servers
	var evacuatedBefore []string
	for {
		if ctx.Err() != nil {
			return fmt.Errorf("Failed to get cluster %q into consistent state for update: %w", name, ctx.Err())
		}

		// Refresh all status information for all servers.
		err = s.serverSvc.PollServers(ctx, ServerFilter{
			Cluster: ptr.To(name),
		}, true)
		if err != nil {
			return fmt.Errorf("Failed to refresh server state information for cluster %q: %w", name, err)
		}

		// Make sure, cluster is ready for a rolling update. This is the case if:
		//   * All servers are in ready state with no update currently running.
		//   * None of the servers is in maintenance.
		servers, err = s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
			Cluster: ptr.To(name),
		})
		if err == nil && len(servers) == 0 {
			err = domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("Failed to get server details for cluster %q: %w", name, err)
		}

		for _, server := range servers {
			if server.Status != api.ServerStatusReady {
				return domain.NewValidationErrf("Cluster update can not be launched for %q: Server %q (%s) is in state %q (%s)", name, server.Name, server.ConnectionURL, server.Status, server.StatusDetail)
			}

			if server.VersionData.InMaintenance == nil || *server.VersionData.InMaintenance == api.InMaintenanceEvacuating || *server.VersionData.InMaintenance == api.InMaintenanceRestoring {
				return domain.NewValidationErrf("Cluster update can not be launched for %q: Server %q (%s) is in maintenance state %q", name, server.Name, server.ConnectionURL, server.VersionData.InMaintenance.String())
			}

			if ptr.From(server.VersionData.InMaintenance) == api.InMaintenanceEvacuated {
				evacuatedBefore = append(evacuatedBefore, server.Name)
			}
		}

		// Trigger update on each server, applications get updated immediately,
		// OS is prepared for update on next reboot.
		// Also verify, that none of the servers is still updating or has pending
		// updates for the applications and the next OS.
		allEqual := true
		for _, server := range servers {
			if !ptr.From(server.VersionData.NeedsUpdate) {
				continue
			}

			allEqual = false

			if server.StatusDetail == api.ServerStatusDetailReadyUpdating {
				// Update servers one by one, one server already updating, so we have
				// to wait.
				break
			}

			applicationUpdate := make([]api.ServerUpdateApplication, 0, len(server.VersionData.Applications))
			for _, app := range server.VersionData.Applications {
				if ptr.From(app.NeedsUpdate) {
					applicationUpdate = append(applicationUpdate, api.ServerUpdateApplication{
						Name:          app.Name,
						TriggerUpdate: true,
					})
				}
			}

			err = s.serverSvc.UpdateSystemByName(ctx, server.Name, api.ServerUpdatePost{
				OS: api.ServerUpdateApplication{
					Name:          "os",
					TriggerUpdate: true,
				},
				Applications: applicationUpdate,
			}, true)
			if err != nil {
				return fmt.Errorf("Failed to trigger server update on %q (%s): %w", server.Name, server.ConnectionURL, err)
			}

			// Update servers one by one, so we have to wait.
			break
		}

		if allEqual {
			break
		}

		time.Sleep(s.clusterUpdatePendingUpdateRecheckInterval)
	}

	// Set target version for cluster update.
	nextInProgressState := api.ClusterUpdateInProgressInactive
	if reboot {
		nextInProgressState = api.ClusterUpdateInProgressRollingRestart
	}

	cluster.UpdateStatus.InProgressStatus = api.ClusterUpdateInProgressStatus{
		InProgress:      nextInProgressState,
		EvacuatedBefore: evacuatedBefore,
		LastUpdated:     s.now(),
	}

	err = s.repo.Update(ctx, *cluster)
	if err != nil {
		return fmt.Errorf("Failed to update cluster %q: %w", cluster.Name, err)
	}

	if reboot {
		lifecycle.ServerLifecycleSignal.Emit(ctx, lifecycle.ServerLifecycleMessage{
			Cluster: &cluster.Name,
		})
	}

	return nil
}

func (s clusterService) ClusterUpdateControlLoop(ctx context.Context, clusterNameFilter *string) error {
	clusters, err := s.GetAllWithFilter(ctx, ClusterFilter{
		Name:       clusterNameFilter,
		Expression: ptr.To(fmt.Sprintf("string(update_status.in_progress_status.in_progress) == %q", api.ClusterUpdateInProgressRollingRestart)),
	})
	if err != nil {
		return fmt.Errorf("Failed to get clusters for update control loop: %w", err)
	}

	var errs []error
	for _, cluster := range clusters {
		// Refresh all status information for all servers.
		err = s.serverSvc.PollServers(ctx, ServerFilter{
			Cluster: &cluster.Name,
		}, true)
		if err != nil {
			var retryableErr domain.ErrRetryable
			if !errors.As(err, &retryableErr) {
				return fmt.Errorf("Failed to refresh server state information for cluster %q: %w", cluster.Name, err)
			}
		}

		// Get updated server state information.
		servers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
			Cluster: &cluster.Name,
		})
		if err == nil && len(servers) == 0 {
			err = domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("Failed to get server details for cluster %q: %w", cluster.Name, err)
		}

		err = s.executeRollingUpdateNextStep(ctx, cluster, servers)
		if err != nil {
			errs = append(errs, fmt.Errorf("Failed to execute next step for rolling update of cluster %q: %w", cluster.Name, err))
			continue
		}
	}

	return errors.Join(errs...)
}

func (s clusterService) executeRollingUpdateNextStep(ctx context.Context, cluster Cluster, servers Servers) error {
	// Calculate, if we are done based on the current state of all servers and the desired target state and
	// calculate next action if we are not done yet.
	var err error
	var nextAction func(context.Context) error

	noop := func(ctx context.Context) error {
		return nil
	}

	for _, server := range servers {
		if nextAction == nil {
			switch server.UpdateState() {
			case api.ServerUpdateStateUndefined:
				return fmt.Errorf("Server update state for %q (%s) is undefined", server.Name, server.ConnectionURL)

			case api.ServerUpdateStateUpToDate:
				continue

			case api.ServerUpdateStateUpdatePending:
				return fmt.Errorf("Server %q has a pending update while a cluster wide rolling reboot cycle is ongoing", server.Name)

			case api.ServerUpdateStateUpdating:
				return fmt.Errorf("Server %q is updating while a cluster wide rolling reboot cycle is ongoing", server.Name)

			case api.ServerUpdateStateEvacuationPending:
				nextAction = func(ctx context.Context) error {
					return s.serverSvc.EvacuateSystemByName(ctx, server.Name, true)
				}

			case api.ServerUpdateStateEvacuating:
				nextAction = noop

			case api.ServerUpdateStateInMaintenanceRebootPending:
				nextAction = func(ctx context.Context) error {
					return s.serverSvc.RebootSystemByName(ctx, server.Name, true)
				}

			case api.ServerUpdateStateInMaintenanceRebooting:
				nextAction = noop

			case api.ServerUpdateStateInMaintenanceRestorePending:
				// Servers, which have been in evacuated state before the update was
				// triggered, are kept in this state.
				if slices.Contains(cluster.UpdateStatus.InProgressStatus.EvacuatedBefore, server.Name) {
					continue
				}

				// Check if the post restore delay has passed.
				if cluster.UpdateStatus.InProgressStatus.LastUpdated.Add(cluster.Config.RollingRestart.PostRestoreDelay).Before(s.now()) {
					restoreModeSkip := cluster.Config.RollingRestart.RestoreMode == "skip"
					nextAction = func(ctx context.Context) error {
						return s.serverSvc.RestoreSystemByName(ctx, server.Name, true, restoreModeSkip)
					}
				} else {
					nextAction = noop
				}

			case api.ServerUpdateStateInMaintenanceRestoring:
				nextAction = noop

			default:
				return fmt.Errorf("Server update state %q for %q (%s) is not supported", server.UpdateState(), server.Name, server.ConnectionURL)
			}

			continue
		}

		// We know the next action so we need to determine, if we are allowed
		// to perform this action as well as the number of steps, that are pending.
		switch server.UpdateState() {
		case api.ServerUpdateStateUpToDate,
			api.ServerUpdateStateEvacuationPending:
			continue

		case api.ServerUpdateStateUndefined:
			return fmt.Errorf("Rolling update blocked, server %q (%s) is in unknown state", server.Name, server.ConnectionURL)

		case api.ServerUpdateStateUpdatePending:
			return fmt.Errorf("Server %q has a pending update while a cluster wide rolling reboot cycle is ongoing", server.Name)

		case api.ServerUpdateStateUpdating:
			return fmt.Errorf("Server %q is updating while a cluster wide rolling reboot cycle is ongoing", server.Name)

		case api.ServerUpdateStateInMaintenanceRebootPending,
			api.ServerUpdateStateInMaintenanceRestorePending:
			// Servers, which have been in evacuated state before the update was
			// triggered, are kept in this state.
			if slices.Contains(cluster.UpdateStatus.InProgressStatus.EvacuatedBefore, server.Name) {
				continue
			}

			fallthrough

		case api.ServerUpdateStateEvacuating,
			api.ServerUpdateStateInMaintenanceRebooting,
			api.ServerUpdateStateInMaintenanceRestoring,
			api.ServerUpdateStateRebootPending,
			api.ServerUpdateStateRebooting:

			return fmt.Errorf("Rolling update blocked, out of order update for server %q (%s) is ongoing, state %v", server.Name, server.ConnectionURL, server.UpdateState())
		}
	}

	// To get a consistent debug log, print the current state before triggering the next action, since
	// it will likely update the state.
	updateState := clusterUpdateState(servers)
	if updateState != "" {
		slog.DebugContext(ctx, "rolling update next step", slog.String("cluster_update_state", updateState))
	}

	done := nextAction == nil
	if !done {
		// Trigger next update action on the target server
		err = nextAction(ctx)
		if err != nil {
			return fmt.Errorf("Failed to trigger next action for rolling update of cluster %q: %w", cluster.Name, err)
		}
	}

	// Update the cluster update status in the DB, if we are done with the update.
	if nextAction == nil {
		err = transaction.Do(ctx, func(ctx context.Context) error {
			updateCluster, err := s.repo.GetByName(ctx, cluster.Name)
			if err != nil {
				return fmt.Errorf("Failed to get cluster %q: %w", cluster.Name, err)
			}

			updateCluster.UpdateStatus.InProgressStatus = api.ClusterUpdateInProgressStatus{
				LastUpdated: s.now(),
			}

			err = s.repo.Update(ctx, *updateCluster)
			if err != nil {
				return fmt.Errorf("Failed to update cluster %q: %w", cluster.Name, err)
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func clusterUpdateState(servers Servers) string {
	totalSteps := 0
	pendingSteps := 0
	currentStep := ""
	currentServer := ""

	for _, server := range servers {
		totalSteps += 6

		switch server.UpdateState() {
		case api.ServerUpdateStateEvacuationPending:
			pendingSteps += 6
			if currentServer == "" {
				currentStep = server.UpdateState().String()
				currentServer = server.Name
			}

		case api.ServerUpdateStateEvacuating:
			pendingSteps += 5
			currentStep = server.UpdateState().String()
			currentServer = server.Name

		case api.ServerUpdateStateInMaintenanceRebootPending:
			pendingSteps += 4
			currentStep = server.UpdateState().String()
			currentServer = server.Name

		case api.ServerUpdateStateInMaintenanceRebooting:
			pendingSteps += 3
			currentStep = server.UpdateState().String()
			currentServer = server.Name

		case api.ServerUpdateStateInMaintenanceRestorePending:
			pendingSteps += 2
			currentStep = server.UpdateState().String()
			currentServer = server.Name

		case api.ServerUpdateStateInMaintenanceRestoring:
			pendingSteps += 1
			currentStep = server.UpdateState().String()
			currentServer = server.Name
		}
	}

	if pendingSteps > 0 {
		return fmt.Sprintf("[%d/%d] %s server %q", totalSteps-pendingSteps+1, totalSteps, currentStep, currentServer)
	}

	return ""
}

func (s clusterService) AbortClusterUpdate(ctx context.Context, name string) error {
	err := transaction.Do(ctx, func(ctx context.Context) error {
		cluster, err := s.repo.GetByName(ctx, name)
		if err != nil {
			return fmt.Errorf("Failed to get cluster %q: %w", name, err)
		}

		cluster.UpdateStatus.InProgressStatus = api.ClusterUpdateInProgressStatus{
			LastUpdated: s.now(),
		}

		err = s.repo.Update(ctx, *cluster)
		if err != nil {
			return fmt.Errorf("Failed to update cluster %q: %w", name, err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (s clusterService) AddServerSystemNetworkVLANTags(ctx context.Context, clusterName string, interfaceName string, vlanTags []int) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Add VLAN tags to interface %q on cluster members for %q failed: %w", interfaceName, clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the interface is present on all of the servers and prepare
	// the updated interface config.
	currentNetworkConfig := make(map[string]*incusosapi.SystemNetworkConfig, len(servers))
	for serverIdx, server := range servers {
		found := false
		if server.OSData.Network.Config == nil {
			return domain.NewValidationErrf("Server %q (%s) does not have any network config", server.Name, server.GetConnectionURL())
		}

		ifaceIdx := 0
		for i, iface := range server.OSData.Network.Config.Interfaces {
			if iface.Name == interfaceName {
				found = true

				networkConfig := &incusosapi.SystemNetworkConfig{}
				_ = structs.DeepCopy(server.OSData.Network.Config, networkConfig)
				// Ignore the error, DeepCopy would fail, if source or dest are nil
				// which is ensured already before.

				currentNetworkConfig[server.Name] = networkConfig
				ifaceIdx = i

				break
			}
		}

		if !found {
			return domain.NewValidationErrf("Server %q (%s) does not have interface %q", server.Name, server.GetConnectionURL(), interfaceName)
		}

		// Append vlan tag if not yet present.
		for _, vlanTag := range vlanTags {
			if slices.Contains(server.OSData.Network.Config.Interfaces[ifaceIdx].VLANTags, vlanTag) {
				continue
			}

			servers[serverIdx].OSData.Network.Config.Interfaces[ifaceIdx].VLANTags = append(servers[serverIdx].OSData.Network.Config.Interfaces[ifaceIdx].VLANTags, vlanTag)
		}
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		err = s.client.UpdateNetworkConfig(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to update network configuration for server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			server.OSData.Network.Config = currentNetworkConfig[server.Name]
			revertErr := s.client.UpdateNetworkConfig(ctx, server)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated network configuration", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.ConnectionURL), slog.Any("vlan-tags", vlanTags), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) RemoveServerSystemNetworkVLANTags(ctx context.Context, clusterName string, interfaceName string, vlanTags []int) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Remove VLAN tags from interface %q on cluster members for %q failed: %w", interfaceName, clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the interface is present on all of the servers and prepare
	// the updated interface config.
	currentNetworkConfig := make(map[string]*incusosapi.SystemNetworkConfig, len(servers))
	for serverIdx, server := range servers {
		found := false
		if server.OSData.Network.Config == nil {
			return domain.NewValidationErrf("Server %q (%s) does not have any network config", server.Name, server.GetConnectionURL())
		}

		ifaceIdx := 0
		for i, iface := range server.OSData.Network.Config.Interfaces {
			if iface.Name == interfaceName {
				found = true

				networkConfig := &incusosapi.SystemNetworkConfig{}
				// Ignore the error, DeepCopy would fail, if source or dest are nil
				// which is ensured already before.
				_ = structs.DeepCopy(server.OSData.Network.Config, networkConfig)

				currentNetworkConfig[server.Name] = networkConfig
				ifaceIdx = i

				break
			}
		}

		if !found {
			return domain.NewValidationErrf("Server %q (%s) does not have interface %q", server.Name, server.GetConnectionURL(), interfaceName)
		}

		// Remove vlan tag if present.
		servers[serverIdx].OSData.Network.Config.Interfaces[ifaceIdx].VLANTags = slices.DeleteFunc(
			servers[serverIdx].OSData.Network.Config.Interfaces[ifaceIdx].VLANTags,
			func(vlanTag int) bool {
				return slices.Contains(vlanTags, vlanTag)
			},
		)
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		err = s.client.UpdateNetworkConfig(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to update network configuration for server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			server.OSData.Network.Config = currentNetworkConfig[server.Name]
			revertErr := s.client.UpdateNetworkConfig(ctx, server)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated network configuration", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.ConnectionURL), slog.Any("vlan-tags", vlanTags), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) UpdateSystemLogging(ctx context.Context, clusterName string, loggingConfig ServerSystemLogging) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Update logging for cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		var currentLoggingConfig ServerSystemLogging
		currentLoggingConfig, err = s.serverSvc.GetSystemLogging(ctx, server.Name)
		if err != nil {
			return fmt.Errorf("Failed to get current logging config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		err = s.serverSvc.UpdateSystemLogging(ctx, server.Name, loggingConfig)
		if err != nil {
			return fmt.Errorf("Failed to update logging config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.serverSvc.UpdateSystemLogging(ctx, server.Name, currentLoggingConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated logging config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("logging-config", currentLoggingConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) UpdateSystemKernel(ctx context.Context, clusterName string, kernelConfig ServerSystemKernel) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Update kernel for cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		var currentKernelConfig ServerSystemKernel
		currentKernelConfig, err = s.serverSvc.GetSystemKernel(ctx, server.Name)
		if err != nil {
			return fmt.Errorf("Failed to get current kernel config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		err = s.serverSvc.UpdateSystemKernel(ctx, server.Name, kernelConfig)
		if err != nil {
			return fmt.Errorf("Failed to update kernel config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.serverSvc.UpdateSystemKernel(ctx, server.Name, currentKernelConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated kernel config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("kernel-config", currentKernelConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) AddApplication(ctx context.Context, clusterName string, applicationName string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Add application to cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	for _, server := range servers {
		err = s.serverSvc.AddApplication(ctx, server.Name, applicationName)
		if err != nil {
			return fmt.Errorf("Failed to add application on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}
	}

	return nil
}

func (s clusterService) AddStorageTargetISCSI(ctx context.Context, clusterName string, target incusosapi.ServiceISCSITarget) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Add iscsi storage target to cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the service is enabled and target is not yet present on all servers.
	iscsiConfigs := make(map[string]incusosapi.ServiceISCSI, len(servers))
	for _, server := range servers {
		var iscsiConfig incusosapi.ServiceISCSI
		iscsiConfig, err = s.client.GetOSServiceISCSI(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get iscsi service config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		if slices.Contains(iscsiConfig.Config.Targets, target) {
			return fmt.Errorf("Service iscsi target %q (%s:%d) already defined on server %q (%s): %w", target.Target, target.Address, target.Port, server.Name, server.GetConnectionURL(), domain.ErrOperationNotPermitted)
		}

		iscsiConfigs[server.Name] = iscsiConfig
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		currentISCSIConfig := iscsiConfigs[server.Name]

		updatedISCSIConfig := incusosapi.ServiceISCSI{
			Config: incusosapi.ServiceISCSIConfig{
				Enabled: true,
				Targets: append(currentISCSIConfig.Config.Targets, target),
			},
		}

		err = s.client.UpdateOSService(ctx, server, "iscsi", updatedISCSIConfig)
		if err != nil {
			return fmt.Errorf("Failed to update iscsi service config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.client.UpdateOSService(ctx, server, "iscsi", currentISCSIConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated iscsi service config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("target", target), slog.Any("service-config", currentISCSIConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) RemoveStorageTargetISCSI(ctx context.Context, clusterName string, target incusosapi.ServiceISCSITarget) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Remove iscsi storage target from cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the service is enabled and target is present on all servers.
	iscsiConfigs := make(map[string]incusosapi.ServiceISCSI, len(servers))
	for _, server := range servers {
		var iscsiConfig incusosapi.ServiceISCSI
		iscsiConfig, err = s.client.GetOSServiceISCSI(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get iscsi service config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		if !slices.Contains(iscsiConfig.Config.Targets, target) {
			return fmt.Errorf("Service iscsi target %q (%s:%d) does not exist on server %q (%s): %w", target.Target, target.Address, target.Port, server.Name, server.GetConnectionURL(), domain.ErrOperationNotPermitted)
		}

		iscsiConfigs[server.Name] = iscsiConfig
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		currentISCSIConfig := iscsiConfigs[server.Name]

		updatedISCSIConfig := incusosapi.ServiceISCSI{
			Config: incusosapi.ServiceISCSIConfig{
				Enabled: true,
				Targets: slices.DeleteFunc(currentISCSIConfig.Config.Targets, func(t incusosapi.ServiceISCSITarget) bool {
					return t == target
				}),
			},
		}

		err = s.client.UpdateOSService(ctx, server, "iscsi", updatedISCSIConfig)
		if err != nil {
			return fmt.Errorf("Failed to update iscsi service config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.client.UpdateOSService(ctx, server, "iscsi", currentISCSIConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated iscsi service config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("target", target), slog.Any("service-config", currentISCSIConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) AddStorageTargetMultipath(ctx context.Context, clusterName string, target string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Add multipath storage target to cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the service is enabled and target is not yet present on all servers.
	multipathConfigs := make(map[string]incusosapi.ServiceMultipath, len(servers))
	for _, server := range servers {
		var multipathConfig incusosapi.ServiceMultipath
		multipathConfig, err = s.client.GetOSServiceMultipath(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get multipath service config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		if slices.Contains(multipathConfig.Config.WWNs, target) {
			return fmt.Errorf("Service multipath target %q already defined on server %q (%s): %w", target, server.Name, server.GetConnectionURL(), domain.ErrOperationNotPermitted)
		}

		multipathConfigs[server.Name] = multipathConfig
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		currentMultipathConfig := multipathConfigs[server.Name]

		updatedMultipathConfig := incusosapi.ServiceMultipath{
			Config: incusosapi.ServiceMultipathConfig{
				Enabled: true,
				WWNs:    append(currentMultipathConfig.Config.WWNs, target),
			},
		}

		err = s.client.UpdateOSService(ctx, server, "multipath", updatedMultipathConfig)
		if err != nil {
			return fmt.Errorf("Failed to update multipath service config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.client.UpdateOSService(ctx, server, "multipath", currentMultipathConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated multipath service config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("target", target), slog.Any("service-config", currentMultipathConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) RemoveStorageTargetMultipath(ctx context.Context, clusterName string, target string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Remove multipath storage target from cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the service is enabled and target is present on all servers.
	multipathConfigs := make(map[string]incusosapi.ServiceMultipath, len(servers))
	for _, server := range servers {
		var multipathConfig incusosapi.ServiceMultipath
		multipathConfig, err = s.client.GetOSServiceMultipath(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get multipath service config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		if !slices.Contains(multipathConfig.Config.WWNs, target) {
			return fmt.Errorf("Service multipath target %q does not exist on server %q (%s): %w", target, server.Name, server.GetConnectionURL(), domain.ErrOperationNotPermitted)
		}

		multipathConfigs[server.Name] = multipathConfig
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		currentMultipathConfig := multipathConfigs[server.Name]

		updatedMultipathConfig := incusosapi.ServiceMultipath{
			Config: incusosapi.ServiceMultipathConfig{
				Enabled: true,
				WWNs: slices.DeleteFunc(currentMultipathConfig.Config.WWNs, func(t string) bool {
					return t == target
				}),
			},
		}

		err = s.client.UpdateOSService(ctx, server, "multipath", updatedMultipathConfig)
		if err != nil {
			return fmt.Errorf("Failed to update multipath service config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.client.UpdateOSService(ctx, server, "multipath", currentMultipathConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated multipath service config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("target", target), slog.Any("service-config", currentMultipathConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) AddStorageTargetNVME(ctx context.Context, clusterName string, target incusosapi.ServiceNVMETarget) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Add nvme storage target to cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the service is enabled and target is not yet present on all servers.
	nvmeConfigs := make(map[string]incusosapi.ServiceNVME, len(servers))
	for _, server := range servers {
		var nvmeConfig incusosapi.ServiceNVME
		nvmeConfig, err = s.client.GetOSServiceNVME(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get nvme service config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		if slices.Contains(nvmeConfig.Config.Targets, target) {
			return fmt.Errorf("Service nvme transport %q (%s:%d) already defined on server %q (%s): %w", target.Transport, target.Address, target.Port, server.Name, server.GetConnectionURL(), domain.ErrOperationNotPermitted)
		}

		nvmeConfigs[server.Name] = nvmeConfig
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		currentNVMEConfig := nvmeConfigs[server.Name]

		updatedNVMEConfig := incusosapi.ServiceNVME{
			Config: incusosapi.ServiceNVMEConfig{
				Enabled: true,
				Targets: append(currentNVMEConfig.Config.Targets, target),
			},
		}

		err = s.client.UpdateOSService(ctx, server, "nvme", updatedNVMEConfig)
		if err != nil {
			return fmt.Errorf("Failed to update nvme service config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.client.UpdateOSService(ctx, server, "nvme", currentNVMEConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated nvme service config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("target", target), slog.Any("service-config", currentNVMEConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) RemoveStorageTargetNVME(ctx context.Context, clusterName string, target incusosapi.ServiceNVMETarget) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("Remove nvme storage target from cluster members for %q failed: %w", clusterName, err)
		}
	}()

	servers, err := s.prepareBulkUpdate(ctx, clusterName)
	if err != nil {
		return err
	}

	// Ensure the service is enabled and target is present on all servers.
	nvmeConfigs := make(map[string]incusosapi.ServiceNVME, len(servers))
	for _, server := range servers {
		var nvmeConfig incusosapi.ServiceNVME
		nvmeConfig, err = s.client.GetOSServiceNVME(ctx, server)
		if err != nil {
			return fmt.Errorf("Failed to get nvme service config from server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		if !slices.Contains(nvmeConfig.Config.Targets, target) {
			return fmt.Errorf("Service nvme transport %q (%s:%d) does not exist on server %q (%s): %w", target.Transport, target.Address, target.Port, server.Name, server.GetConnectionURL(), domain.ErrOperationNotPermitted)
		}

		nvmeConfigs[server.Name] = nvmeConfig
	}

	// Perform change on all servers.
	reverter := revert.New()
	defer reverter.Fail()

	for _, server := range servers {
		currentNVMEConfig := nvmeConfigs[server.Name]

		updatedNVMEConfig := incusosapi.ServiceNVME{
			Config: incusosapi.ServiceNVMEConfig{
				Enabled: true,
				Targets: slices.DeleteFunc(currentNVMEConfig.Config.Targets, func(t incusosapi.ServiceNVMETarget) bool {
					return t == target
				}),
			},
		}

		err = s.client.UpdateOSService(ctx, server, "nvme", updatedNVMEConfig)
		if err != nil {
			return fmt.Errorf("Failed to update nvme service config on server %q (%s): %w", server.Name, server.GetConnectionURL(), err)
		}

		reverter.Add(func() {
			revertErr := s.client.UpdateOSService(ctx, server, "nvme", currentNVMEConfig)
			if revertErr != nil {
				slog.ErrorContext(ctx, "Failed to revert previously updated nvme service config", logger.Err(revertErr), slog.String("server", server.Name), slog.String("connection_url", server.GetConnectionURL()), slog.Any("target", target), slog.Any("service-config", currentNVMEConfig), slog.Any("root-cause", err))
			}
		})
	}

	reverter.Success()

	return nil
}

func (s clusterService) prepareBulkUpdate(ctx context.Context, clusterName string) (Servers, error) {
	cluster, err := s.GetByName(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster %q: %w", clusterName, err)
	}

	if cluster.Status != api.ClusterStatusReady {
		return nil, fmt.Errorf("Cluster %q is not ready: %w", clusterName, domain.ErrOperationNotPermitted)
	}

	// Update the current server states in the DB, serves as a connection test at the same time.
	err = s.serverSvc.PollServers(ctx, ServerFilter{
		Cluster: ptr.To(clusterName),
	}, true)
	if err != nil {
		return nil, fmt.Errorf("Polling of cluster members failed: %w", err)
	}

	servers, err := s.serverSvc.GetAllWithFilter(ctx, ServerFilter{
		Cluster: ptr.To(clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to get server names for cluster %q: %w", clusterName, err)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("Cluster %q does not have any servers: %w", clusterName, domain.ErrOperationNotPermitted)
	}

	// Ensure all servers are in ready state and online.
	for _, server := range servers {
		isReady := server.Status == api.ServerStatusReady &&
			server.StatusDetail == api.ServerStatusDetailNone &&
			server.VersionData.InMaintenance != nil &&
			*server.VersionData.InMaintenance == api.NotInMaintenance
		if !isReady {
			return nil, fmt.Errorf("Server %q (%s) is not ready (status: %q, status detail: %q, maintenance: %q): %w", server.Name, server.GetConnectionURL(), server.Status, server.StatusDetail, ptr.From(server.VersionData.InMaintenance), domain.ErrOperationNotPermitted)
		}
	}

	return servers, nil
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

	lifecycle.ClusterUpdateSignal.AddListener(func(ctx context.Context, cum lifecycle.ClusterUpdateMessage) {
		lifecycleMonitorsMu.Lock()
		defer lifecycleMonitorsMu.Unlock()

		switch cum.Operation {
		case lifecycle.ClusterUpdateOperationCreate:
			cancel, err := s.startLifecycleEventHandler(context.Background(), cum.Name)
			if err != nil {
				slog.WarnContext(ctx, "Failed to start lifecycle monitor", slog.String("cluster", cum.Name), logger.Err(err))
				return
			}

			lifecycleMonitors[cum.Name] = cancel

		case lifecycle.ClusterUpdateOperationDelete:
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
					slog.InfoContext(ctx, "lifecycle event", slog.String("event", event.LifecycleEventAction), slog.String("cluster", clusterName), slog.Any("action", event.Operation), slog.Any("resource_type", event.ResourceType), slog.String("source", event.Source.String()))

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
