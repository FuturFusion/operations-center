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

	// API routes
	inventoryImageRouter := newSubRouter(inventoryRouter, "/images")
	registerInventoryImageHandler(inventoryImageRouter, inventoryImageSvc)

	inventoryInstanceRouter := newSubRouter(inventoryRouter, "/instances")
	registerInventoryInstanceHandler(inventoryInstanceRouter, inventoryInstanceSvc)

	inventoryNetworkRouter := newSubRouter(inventoryRouter, "/networks")
	registerInventoryNetworkHandler(inventoryNetworkRouter, inventoryNetworkSvc)

	inventoryNetworkACLRouter := newSubRouter(inventoryRouter, "/network_acls")
	registerInventoryNetworkACLHandler(inventoryNetworkACLRouter, inventoryNetworkACLSvc)
}
