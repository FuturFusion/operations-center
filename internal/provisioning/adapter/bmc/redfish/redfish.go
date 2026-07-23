package redfish

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"

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

	c, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint: server.BMCConfig.Endpoint,
		Username: server.BMCConfig.Username,
		Password: server.BMCConfig.Password,
		Insecure: server.BMCConfig.Insecure,
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
		return api.BMCServerDetails{}, fmt.Errorf("Failed to connect to BMC %q: %w", server.BMCConfig.Endpoint, err)
	}

	defer logout()

	system, err := getFirstSystem(client)
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to get BMC system: %w", err)
	}

	manager, err := getFirstManager(client)
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to get BMC manager: %w", err)
	}

	processor, err := getFirstProcessor(system)
	if err != nil {
		return api.BMCServerDetails{}, fmt.Errorf("Failed to get first processor of BMC system: %w", err)
	}

	serverLocationIndicatorActive := system.IndicatorLED == schemas.BlinkingIndicatorLED || system.IndicatorLED == schemas.LitIndicatorLED // nolint: staticcheck
	if system.LocationIndicatorActive != nil {
		serverLocationIndicatorActive = *system.LocationIndicatorActive
	}

	return api.BMCServerDetails{
		BMCProtocol:                   "Redfish",
		BMCProtocolVersion:            client.Service.RedfishVersion,
		BMCVendor:                     client.Service.Vendor,
		BMCModel:                      manager.Model,
		BMCFirmwareVersion:            manager.FirmwareVersion,
		BMCServiceIdentification:      manager.ServiceIdentification,
		ServerManufacturer:            system.Manufacturer,
		ServerModel:                   system.Model,
		ServerSubModel:                system.SubModel,
		ServerUUID:                    system.UUID,
		ServerAssetTag:                system.AssetTag,
		ServerHostName:                system.HostName,
		ServerSKU:                     system.SKU,
		ServerSerialNumber:            system.SerialNumber,
		ServerBIOSVersion:             system.BiosVersion,
		ServerProcessorArchitecture:   string(processor.ProcessorArchitecture),
		ServerProcessorInstructionSet: string(processor.InstructionSet),
		ServerPowerState:              string(system.PowerState),
		ServerLocationIndicatorActive: serverLocationIndicatorActive,
		ServerHealthStatus:            string(system.Status.Health),
		LastUpdated:                   time.Now(),
	}, nil
}

func getFirstManager(client *gofish.APIClient) (*schemas.Manager, error) {
	managers, err := client.Service.Managers()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC managers: %w", err)
	}

	if len(managers) == 0 {
		return nil, fmt.Errorf("No BMC managers found")
	}

	sort.Slice(managers, func(i, j int) bool { return managers[i].ID < managers[j].ID })

	return managers[0], nil
}

func getFirstSystem(client *gofish.APIClient) (*schemas.ComputerSystem, error) {
	systems, err := client.Service.Systems()
	if err != nil {
		return nil, fmt.Errorf("Failed to get BMC systems: %w", err)
	}

	if len(systems) == 0 {
		return nil, fmt.Errorf("No BMC systems found")
	}

	sort.Slice(systems, func(i, j int) bool { return systems[i].ID < systems[j].ID })

	return systems[0], nil
}

func getFirstProcessor(system *schemas.ComputerSystem) (*schemas.Processor, error) {
	processors, err := system.Processors()
	if err != nil {
		return nil, fmt.Errorf("Failed to get processors of BMC system: %w", err)
	}

	if len(processors) == 0 {
		return nil, fmt.Errorf("No processors found for the BMC system")
	}

	sort.Slice(processors, func(i, j int) bool { return processors[i].ID < processors[j].ID })

	return processors[0], nil
}
