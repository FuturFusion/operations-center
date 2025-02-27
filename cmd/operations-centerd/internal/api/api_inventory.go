package api

import (
	"log/slog"
	"net/http"

	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/inventory"
	inventoryServiceMiddleware "github.com/FuturFusion/operations-center/internal/inventory/middleware"
	inventoryRepoMiddleware "github.com/FuturFusion/operations-center/internal/inventory/repo/middleware"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func registerInventoryRoutes(db dbdriver.DBTX, clusterSvc provisioning.ClusterService, serverSvc provisioning.ServerService, serverClient inventory.ServerClient, inventoryRouter *http.ServeMux) []provisioning.InventorySyncer {
	// Service
	inventoryImageSvc := inventoryServiceMiddleware.NewImageServiceWithSlog(
		inventory.NewImageService(
			inventoryRepoMiddleware.NewImageRepoWithSlog(
				inventorySqlite.NewImage(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryInstanceSvc := inventoryServiceMiddleware.NewInstanceServiceWithSlog(
		inventory.NewInstanceService(
			inventoryRepoMiddleware.NewInstanceRepoWithSlog(
				inventorySqlite.NewInstance(db),
				slog.Default(),
			),
			clusterSvc,
			serverSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryNetworkSvc := inventoryServiceMiddleware.NewNetworkServiceWithSlog(
		inventory.NewNetworkService(
			inventoryRepoMiddleware.NewNetworkRepoWithSlog(
				inventorySqlite.NewNetwork(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryNetworkACLSvc := inventoryServiceMiddleware.NewNetworkACLServiceWithSlog(
		inventory.NewNetworkACLService(
			inventoryRepoMiddleware.NewNetworkACLRepoWithSlog(
				inventorySqlite.NewNetworkACL(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryNetworkForwardSvc := inventoryServiceMiddleware.NewNetworkForwardServiceWithSlog(
		inventory.NewNetworkForwardService(
			inventoryRepoMiddleware.NewNetworkForwardRepoWithSlog(
				inventorySqlite.NewNetworkForward(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
			serverClient,
			inventory.NetworkForwardWithParentFilter(func(network incusapi.Network) bool {
				return !network.Managed
			}),
		),
		slog.Default(),
	)

	inventoryNetworkIntegrationSvc := inventoryServiceMiddleware.NewNetworkIntegrationServiceWithSlog(
		inventory.NewNetworkIntegrationService(
			inventoryRepoMiddleware.NewNetworkIntegrationRepoWithSlog(
				inventorySqlite.NewNetworkIntegration(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryNetworkLoadBalancerSvc := inventoryServiceMiddleware.NewNetworkLoadBalancerServiceWithSlog(
		inventory.NewNetworkLoadBalancerService(
			inventoryRepoMiddleware.NewNetworkLoadBalancerRepoWithSlog(
				inventorySqlite.NewNetworkLoadBalancer(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
			serverClient,
			inventory.NetworkLoadBalancerWithParentFilter(func(network incusapi.Network) bool {
				if !network.Managed {
					return true
				}

				if network.Type == "bridge" {
					return true
				}

				return false
			}),
		),
		slog.Default(),
	)

	inventoryNetworkPeerSvc := inventoryServiceMiddleware.NewNetworkPeerServiceWithSlog(
		inventory.NewNetworkPeerService(
			inventoryRepoMiddleware.NewNetworkPeerRepoWithSlog(
				inventorySqlite.NewNetworkPeer(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
			serverClient,
			inventory.NetworkPeerWithParentFilter(func(network incusapi.Network) bool {
				if !network.Managed {
					return true
				}

				if network.Type == "bridge" {
					return true
				}

				return false
			}),
		),
		slog.Default(),
	)

	inventoryNetworkZoneSvc := inventoryServiceMiddleware.NewNetworkZoneServiceWithSlog(
		inventory.NewNetworkZoneService(
			inventoryRepoMiddleware.NewNetworkZoneRepoWithSlog(
				inventorySqlite.NewNetworkZone(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryProfileSvc := inventoryServiceMiddleware.NewProfileServiceWithSlog(
		inventory.NewProfileService(
			inventoryRepoMiddleware.NewProfileRepoWithSlog(
				inventorySqlite.NewProfile(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryProjectSvc := inventoryServiceMiddleware.NewProjectServiceWithSlog(
		inventory.NewProjectService(
			inventoryRepoMiddleware.NewProjectRepoWithSlog(
				inventorySqlite.NewProject(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryStorageBucketSvc := inventoryServiceMiddleware.NewStorageBucketServiceWithSlog(
		inventory.NewStorageBucketService(
			inventoryRepoMiddleware.NewStorageBucketRepoWithSlog(
				inventorySqlite.NewStorageBucket(db),
				slog.Default(),
			),
			clusterSvc,
			serverSvc,
			serverClient,
			serverClient,
		),
		slog.Default(),
	)

	inventoryStoragePoolSvc := inventoryServiceMiddleware.NewStoragePoolServiceWithSlog(
		inventory.NewStoragePoolService(
			inventoryRepoMiddleware.NewStoragePoolRepoWithSlog(
				inventorySqlite.NewStoragePool(db),
				slog.Default(),
			),
			clusterSvc,
			serverClient,
		),
		slog.Default(),
	)

	inventoryStorageVolumeSvc := inventoryServiceMiddleware.NewStorageVolumeServiceWithSlog(
		inventory.NewStorageVolumeService(
			inventoryRepoMiddleware.NewStorageVolumeRepoWithSlog(
				inventorySqlite.NewStorageVolume(db),
				slog.Default(),
			),
			clusterSvc,
			serverSvc,
			serverClient,
			serverClient,
		),
		slog.Default(),
	)

	// API routes
	inventoryImageRouter := newSubRouter(inventoryRouter, "/images")
	registerInventoryImageHandler(inventoryImageRouter, inventoryImageSvc)

	inventoryInstanceRouter := newSubRouter(inventoryRouter, "/instances")
	registerInventoryInstanceHandler(inventoryInstanceRouter, inventoryInstanceSvc)

	inventoryNetworkRouter := newSubRouter(inventoryRouter, "/networks")
	registerInventoryNetworkHandler(inventoryNetworkRouter, inventoryNetworkSvc)

	inventoryNetworkACLRouter := newSubRouter(inventoryRouter, "/network_acls")
	registerInventoryNetworkACLHandler(inventoryNetworkACLRouter, inventoryNetworkACLSvc)

	inventoryNetworkForwardRouter := newSubRouter(inventoryRouter, "/network_forward")
	registerInventoryNetworkForwardHandler(inventoryNetworkForwardRouter, inventoryNetworkForwardSvc)

	inventoryNetworkIntegrationRouter := newSubRouter(inventoryRouter, "/network_integrations")
	registerInventoryNetworkIntegrationHandler(inventoryNetworkIntegrationRouter, inventoryNetworkIntegrationSvc)

	inventoryNetworkLoadBalancerRouter := newSubRouter(inventoryRouter, "/network_load_balancers")
	registerInventoryNetworkLoadBalancerHandler(inventoryNetworkLoadBalancerRouter, inventoryNetworkLoadBalancerSvc)

	inventoryNetworkPeerRouter := newSubRouter(inventoryRouter, "/network_peers")
	registerInventoryNetworkPeerHandler(inventoryNetworkPeerRouter, inventoryNetworkPeerSvc)

	inventoryNetworkZoneRouter := newSubRouter(inventoryRouter, "/network_zones")
	registerInventoryNetworkZoneHandler(inventoryNetworkZoneRouter, inventoryNetworkZoneSvc)

	inventoryProfileRouter := newSubRouter(inventoryRouter, "/profiles")
	registerInventoryProfileHandler(inventoryProfileRouter, inventoryProfileSvc)

	inventoryProjectRouter := newSubRouter(inventoryRouter, "/projects")
	registerInventoryProjectHandler(inventoryProjectRouter, inventoryProjectSvc)

	inventoryStorageBucketRouter := newSubRouter(inventoryRouter, "/storage_buckets")
	registerInventoryStorageBucketHandler(inventoryStorageBucketRouter, inventoryStorageBucketSvc)

	inventoryStoragePoolRouter := newSubRouter(inventoryRouter, "/storage_pools")
	registerInventoryStoragePoolHandler(inventoryStoragePoolRouter, inventoryStoragePoolSvc)

	inventoryStorageVolumeRouter := newSubRouter(inventoryRouter, "/storage_volumes")
	registerInventoryStorageVolumeHandler(inventoryStorageVolumeRouter, inventoryStorageVolumeSvc)

	return []provisioning.InventorySyncer{
		inventoryImageSvc,
		inventoryInstanceSvc,
		inventoryNetworkSvc,
		inventoryNetworkACLSvc,
		inventoryNetworkForwardSvc,
		inventoryNetworkIntegrationSvc,
		inventoryNetworkLoadBalancerSvc,
		inventoryNetworkPeerSvc,
		inventoryNetworkZoneSvc,
		inventoryProfileSvc,
		inventoryProjectSvc,
		inventoryStorageBucketSvc,
		inventoryStoragePoolSvc,
		inventoryStorageVolumeSvc,
	}
}
