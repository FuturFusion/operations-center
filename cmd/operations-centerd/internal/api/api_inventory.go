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
	inventoryInstanceSvc := inventory.NewInstanceService(
		inventorySqlite.NewInstance(db),
		clusterSvc,
		serverSvc,
		serverClient,
	)

	// API routes
	inventoryInstanceRouter := newSubRouter(inventoryRouter, "/instances")
	registerInventoryInstanceHandler(inventoryInstanceRouter, inventoryInstanceSvc)
}
