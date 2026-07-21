package redfish

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/stmcginnis/gofish"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type redfish struct{}

var _ provisioning.BMCServerClientPort = redfish{}

func New() redfish {
	return redfish{}
}

func (r redfish) getClient(ctx context.Context, server provisioning.Server) (_ *gofish.APIClient, logout func(), _ error) {
	if transaction.IsActive(ctx) {
		slog.WarnContext(ctx, "Redfish API call inside of a transaction", logger.AddStacktrace())
	}

	c, err := gofish.Connect(gofish.ClientConfig{
		Endpoint: server.BMCEndpoint,
		Username: server.BMCUsername,
		Password: server.BMCPassword,
		// Insecure: true,
		// BasicAuth: true,
		// DumpWriter: os.Stdout,
	})
	if err != nil {
		return nil, nil, err
	}

	return c, c.Logout, nil
}

func (r redfish) GetServerDetails(ctx context.Context, server provisioning.Server) (api.BMCServerDetails, error) {
	client, logout, err := r.getClient(ctx, server)
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCEndpoint, err)
	}

	defer logout()

	systems, err := client.Service.Systems()
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to get BMC systems on %q: %w", server.BMCEndpoint, err)
	}

	if len(systems) == 0 {
		return api.BMCServerDetails{}, fmt.Errorf("No BMC systems found on %q", server.BMCEndpoint)
	}

	system := systems[0]

	return api.BMCServerDetails{
		SystemUUID: system.ID,
	}, nil
}
