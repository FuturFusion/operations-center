package api

import (
	"net/http"

	"github.com/FuturFusion/operations-center/internal/inventory"
	inventorySqlite "github.com/FuturFusion/operations-center/internal/inventory/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
)

func registerInventoryRoutes(db dbdriver.DBTX, clusterSvc provisioning.ClusterService, serverSvc provisioning.ServerService, serverClient inventory.ServerClient, inventoryRouter *http.ServeMux) {
	// Service
	inventoryImageSvc := inventory.NewImageService(
		inventorySqlite.NewImage(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryInstanceSvc := inventory.NewInstanceService(
		inventorySqlite.NewInstance(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryNetworkSvc := inventory.NewNetworkService(
		inventorySqlite.NewNetwork(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryNetworkACLSvc := inventory.NewNetworkACLService(
		inventorySqlite.NewNetworkACL(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryNetworkIntegrationSvc := inventory.NewNetworkIntegrationService(
		inventorySqlite.NewNetworkIntegration(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryNetworkZoneSvc := inventory.NewNetworkZoneService(
		inventorySqlite.NewNetworkZone(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryProfileSvc := inventory.NewProfileService(
		inventorySqlite.NewProfile(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryProjectSvc := inventory.NewProjectService(
		inventorySqlite.NewProject(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryStorageBucketSvc := inventory.NewStorageBucketService(
		inventorySqlite.NewStorageBucket(db),
		clusterSvc,
		serverSvc,
		serverClient,
		serverClient,
	)

	inventoryStoragePoolSvc := inventory.NewStoragePoolService(
		inventorySqlite.NewStoragePool(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	inventoryStorageVolumeSvc := inventory.NewStorageVolumeService(
		inventorySqlite.NewStorageVolume(db),
		clusterSvc,
		serverSvc,
		serverClient,
		serverClient,
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

	inventoryNetworkIntegrationRouter := newSubRouter(inventoryRouter, "/network_integrations")
	registerInventoryNetworkIntegrationHandler(inventoryNetworkIntegrationRouter, inventoryNetworkIntegrationSvc)

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
}
