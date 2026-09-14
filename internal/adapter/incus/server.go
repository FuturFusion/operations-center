package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

func (c Client) IsReady(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return fmt.Errorf("Get resources from %q (%s) failed: %w", server.GetName(), server.GetConnectionURL(), err)
	}

	var osData struct {
		Environment struct {
			// TODO: Checking uptime is kept for backwards compatibility for now.
			Uptime        int   `json:"uptime"`
			SystemIsReady *bool `json:"system_is_ready"`
		} `json:"environment"`
	}
	err = json.Unmarshal(resp.Metadata, &osData)
	if err != nil {
		return fmt.Errorf("Unexpected response metadata while fetching OS information from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	// Legacy mode based on uptime
	if osData.Environment.SystemIsReady == nil {
		if osData.Environment.Uptime < 120 {
			return domain.NewRetryableErr(fmt.Errorf("Server uptime is less than 120s"))
		}

		return nil
	}

	if !ptr.From(osData.Environment.SystemIsReady) {
		return domain.NewRetryableErr(fmt.Errorf("Server is not yet ready"))
	}

	return nil
}

func (c Client) GetResources(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.HardwareData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/resources", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.HardwareData{}, fmt.Errorf("Get resources from %q (%s) failed: %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	var resources incusapi.Resources
	err = json.Unmarshal(resp.Metadata, &resources)
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Unexpected response metadata while getting resource information from %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	return api.HardwareData{
		Resources: resources,
	}, nil
}

func (c Client) GetOSData(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.OSData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/network", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.OSData{}, fmt.Errorf("Get OS network data from %q (%s) failed: %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	var network incusosapi.SystemNetwork
	err = json.Unmarshal(resp.Metadata, &network)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS network information from %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/security", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS security data from %q (%s) failed: %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	var security incusosapi.SystemSecurity
	err = json.Unmarshal(resp.Metadata, &security)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS security information from %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/storage", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS storage data from %q (%s) failed: %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	var storage incusosapi.SystemStorage
	err = json.Unmarshal(resp.Metadata, &storage)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS storage information from %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	return api.OSData{
		Network:  network,
		Security: security,
		Storage:  storage,
	}, nil
}

func (c Client) GetVersionData(ctx context.Context, server provisioning.Server) (api.ServerVersionData, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return api.ServerVersionData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.ServerVersionData{}, fmt.Errorf("Get OS version data from %q (%s) failed: %w", server.Name, server.GetConnectionURL(), err)
	}

	var osVersionData struct {
		Environment struct {
			Hostname      string `json:"hostname"`
			OSName        string `json:"os_name"`
			OSVersion     string `json:"os_version"`
			OSVersionNext string `json:"os_version_next"`
		} `json:"environment"`
	}
	err = json.Unmarshal(resp.Metadata, &osVersionData)
	if err != nil {
		return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching OS version information from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/applications", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.ServerVersionData{}, fmt.Errorf("Get applications from %q failed: %w", server.GetConnectionURL(), err)
	}

	var applications []string
	err = json.Unmarshal(resp.Metadata, &applications)
	if err != nil {
		return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching applications from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	applicationVersions := make([]api.ApplicationVersionData, 0, len(applications))
	for _, applicationURL := range applications {
		applicationName := path.Base(applicationURL)

		resp, _, err = client.RawQuery(http.MethodGet, path.Join("/os/1.0/applications", applicationName), http.NoBody, "")
		if err != nil {
			err = api.AsNotIncusOSError(err)

			return api.ServerVersionData{}, fmt.Errorf("Get application version data for %q from %q (%s) failed: %w", applicationName, server.Name, server.GetConnectionURL(), err)
		}

		var application incusosapi.Application
		err = json.Unmarshal(resp.Metadata, &application)
		if err != nil {
			return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching application %q from %q (%s): %w", applicationName, server.Name, server.GetConnectionURL(), err)
		}

		inMaintenance := api.NotInMaintenance
		if domain.IsApplicationNameIncusKind(applicationName) && server.Cluster != nil {
			member, _, err := client.GetClusterMember(server.Name)
			if err != nil {
				return api.ServerVersionData{}, fmt.Errorf("Failed to get Incus cluster member details for %q (%s): %w", server.Name, server.GetConnectionURL(), err)
			}

			switch member.Status {
			case "Evacuating":
				inMaintenance = api.InMaintenanceEvacuating

			case "Evacuated":
				inMaintenance = api.InMaintenanceEvacuated

			case "Restoring":
				inMaintenance = api.InMaintenanceRestoring
			}
		}

		applicationVersions = append(applicationVersions, api.ApplicationVersionData{
			Name:            applicationName,
			Version:         application.State.Version,
			FriendlyVersion: application.State.FriendlyVersion,
			InMaintenance:   inMaintenance,
		})
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/update", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.ServerVersionData{}, fmt.Errorf("Get OS version data from %q (%s) failed: %w", server.Name, server.GetConnectionURL(), err)
	}

	var systemUpdate incusosapi.SystemUpdate
	err = json.Unmarshal(resp.Metadata, &systemUpdate)
	if err != nil {
		return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching system update information from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return api.ServerVersionData{
		OS: api.OSVersionData{
			Name:        osVersionData.Environment.OSName,
			Version:     osVersionData.Environment.OSVersion,
			VersionNext: osVersionData.Environment.OSVersionNext,
			NeedsReboot: systemUpdate.State.NeedsReboot,
		},
		Applications:  applicationVersions,
		UpdateChannel: systemUpdate.Config.Channel,
	}, nil
}

func (c Client) GetServerType(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.ServerTypeUnknown, err
	}

	const endpointPath = "/os/1.0/applications"

	resp, _, err := client.RawQuery(http.MethodGet, endpointPath, http.NoBody, "")
	if err != nil {
		return api.ServerTypeUnknown, fmt.Errorf("Get applications from %q (%s) failed: %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	var applications []string
	err = json.Unmarshal(resp.Metadata, &applications)
	if err != nil {
		return api.ServerTypeUnknown, fmt.Errorf("Unexpected response metadata while fetching applications from %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	for _, applicationPath := range applications {
		application := strings.TrimLeft(strings.TrimPrefix(applicationPath, endpointPath), "/")

		var serverType api.ServerType
		err := serverType.UnmarshalText([]byte(application))
		if err != nil {
			continue
		}

		if serverType == api.ServerTypeUnknown {
			continue
		}

		return serverType, nil
	}

	return api.ServerTypeUnknown, fmt.Errorf("Server %q (%s) did not return any known server type defining application (%v)", endpoint.GetName(), endpoint.GetConnectionURL(), applications)
}

func (c Client) GetNodeSpecificConfigKeys(ctx context.Context, endpoint provisioning.Endpoint) (map[string]map[string]bool, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	meta, err := client.GetMetadataConfiguration()
	if err != nil {
		return nil, fmt.Errorf("Failed to get metadata configuration for node specific config keys: %w", err)
	}

	nodeSpecificConfigsMap := make(map[string]map[string]bool, len(meta.Config))

	for entityKeyName, entities := range meta.Config {
		for _, groups := range entities {
			for _, values := range groups.Keys {
				for key, value := range values {
					if value.Scope == "local" {
						entityKey := string(entityKeyName)
						_, ok := nodeSpecificConfigsMap[entityKey]
						if !ok {
							nodeSpecificConfigsMap[entityKey] = map[string]bool{}
						}

						nodeSpecificConfigsMap[entityKey][key] = true
					}
				}
			}
		}
	}

	// NOTE: Manually added, since `/1.0/metadata/configuration` currently lacks details about `storage_lvmcluster`.
	nodeSpecificConfigsMap["storage_lvmcluster"] = nodeSpecificConfigsMap["storage_lvm"]
	delete(nodeSpecificConfigsMap["storage_lvmcluster"], "size")

	return nodeSpecificConfigsMap, nil
}

func (c Client) GetNetworkConfig(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemNetwork, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return provisioning.ServerSystemNetwork{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/network", http.NoBody, "")
	if err != nil {
		return provisioning.ServerSystemNetwork{}, fmt.Errorf("Failed to get system network configuration on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	var networkConfig provisioning.ServerSystemNetwork
	err = json.Unmarshal(resp.Metadata, &networkConfig)
	if err != nil {
		return provisioning.ServerSystemNetwork{}, fmt.Errorf("Unexpected response metadata while fetching system network configuration from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return networkConfig, nil
}

func (c Client) UpdateNetworkConfig(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/network", server.OSData.Network, "")
	if err != nil {
		return fmt.Errorf("Put OS network data to %q (%s) failed: %w", server.Name, server.ConnectionURL, err)
	}

	return nil
}

func (c Client) GetStorageConfig(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemStorage, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return provisioning.ServerSystemStorage{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/storage", http.NoBody, "")
	if err != nil {
		return provisioning.ServerSystemStorage{}, fmt.Errorf("Failed to get system storage configuration on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	var storageConfig provisioning.ServerSystemStorage
	err = json.Unmarshal(resp.Metadata, &storageConfig)
	if err != nil {
		return provisioning.ServerSystemStorage{}, fmt.Errorf("Unexpected response metadata while fetching system storage configuration from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return storageConfig, nil
}

func (c Client) UpdateStorageConfig(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/storage", server.OSData.Storage, "")
	if err != nil {
		return fmt.Errorf("Put OS storage data to %q (%s) failed: %w", server.Name, server.ConnectionURL, err)
	}

	return nil
}

func (c Client) GetProviderConfig(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemProvider, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return incusosapi.SystemProvider{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/provider", nil, "")
	if err != nil {
		return incusosapi.SystemProvider{}, fmt.Errorf("Get OS provider config from %q (%s) failed: %w", server.Name, server.ConnectionURL, err)
	}

	var providerConfig incusosapi.SystemProvider
	err = json.Unmarshal(resp.Metadata, &providerConfig)
	if err != nil {
		return incusosapi.SystemProvider{}, fmt.Errorf("Unexpected response metadata while getting provider information from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return providerConfig, nil
}

func (c Client) UpdateProviderConfig(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemProvider) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/provider", providerConfig, "")
	if err != nil {
		return fmt.Errorf("Put OS provider config to %q (%s) failed: %w", server.Name, server.ConnectionURL, err)
	}

	return nil
}

func (c Client) GetUpdateConfig(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemUpdate, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return incusosapi.SystemUpdate{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/update", nil, "")
	if err != nil {
		return incusosapi.SystemUpdate{}, fmt.Errorf("Get OS update config from %q (%s) failed: %w", server.Name, server.ConnectionURL, err)
	}

	var updateConfig incusosapi.SystemUpdate
	err = json.Unmarshal(resp.Metadata, &updateConfig)
	if err != nil {
		return incusosapi.SystemUpdate{}, fmt.Errorf("Unexpected response metadata while getting update information from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return updateConfig, nil
}

func (c Client) UpdateUpdateConfig(ctx context.Context, server provisioning.Server, updateConfig provisioning.ServerSystemUpdate) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/update", updateConfig, "")
	if err != nil {
		return fmt.Errorf("Put OS update config to %q (%s) failed: %w", server.Name, server.ConnectionURL, err)
	}

	return nil
}

func (c Client) Evacuate(ctx context.Context, server provisioning.Server, callback func(ctx context.Context, err error)) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	op, err := client.UpdateClusterMemberState(server.Name, incusapi.ClusterMemberStatePost{
		Action: "evacuate",
		Mode:   "auto",
	})
	if err != nil {
		return fmt.Errorf("Failed to update cluster member state to evacuated on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	go func() {
		// Use detached context for async background operation.
		ctx := logger.DetachedContext(ctx)

		callback(ctx, op.Wait())
	}()

	return nil
}

func (c Client) TriggerSystemAction(ctx context.Context, server provisioning.Server, resource string, action string, body any) error {
	if strings.Contains(resource, "/") || strings.Contains(action, "/") {
		return fmt.Errorf(`Resource and action must not contain forward slashes ("/")`)
	}

	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(action, ":") {
		action = ":" + action
	}

	_, _, err = client.RawQuery(http.MethodPost, path.Join("/os/1.0/system", resource, action), body, "")
	if err != nil {
		return fmt.Errorf("Failed to trigger %s on %q (%s): %w", action, server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) Poweroff(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/:poweroff", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to trigger poweroff on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) Reboot(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/:reboot", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to trigger reboot on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) Restore(ctx context.Context, server provisioning.Server, restoreModeSkip bool, callback func(ctx context.Context, err error)) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	restoreMode := ""
	if restoreModeSkip {
		restoreMode = "skip"
	}

	op, err := client.UpdateClusterMemberState(server.Name, incusapi.ClusterMemberStatePost{
		Action: "restore",
		Mode:   restoreMode,
	})
	if err != nil {
		return fmt.Errorf("Failed to update cluster member state to evacuated on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	go func() {
		// Use detached context for async background operation.
		ctx := logger.DetachedContext(ctx)

		callback(ctx, op.Wait())
	}()

	return nil
}

func (c Client) UpdateOS(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/update/:check", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to trigger update check on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) AddApplication(ctx context.Context, server provisioning.Server, application string) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/applications", map[string]string{
		"name": application,
	}, "")
	if err != nil {
		return fmt.Errorf("Failed to add application %q on %q (%s): %w", application, server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) RestartApplication(ctx context.Context, server provisioning.Server, application string) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, path.Join("/os/1.0/applications", application, ":restart"), nil, "")
	if err != nil {
		return fmt.Errorf("Failed to restart application %q on %q (%s): %w", application, server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) UpdateApplication(ctx context.Context, server provisioning.Server, application string) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, path.Join("/os/1.0/applications", application, ":check-update"), http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return fmt.Errorf("Failed to trigger update check for application %q on %q (%s): %w", application, server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) GetSystem(ctx context.Context, server provisioning.Server, resource string) (map[string]any, error) {
	if strings.Contains(resource, "/") {
		return nil, fmt.Errorf(`Resource name must not contain forward slashes ("/")`)
	}

	client, err := c.getClient(ctx, server)
	if err != nil {
		return nil, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, path.Join("/os/1.0/system", resource), http.NoBody, "")
	if err != nil {
		return nil, fmt.Errorf("Failed to get system %s configuration on %q (%s): %w", resource, server.Name, server.GetConnectionURL(), err)
	}

	config := map[string]any{}
	err = json.Unmarshal(resp.Metadata, &config)
	if err != nil {
		return nil, fmt.Errorf("Unexpected response metadata while fetching system %s configuration from %q (%s): %w", resource, server.Name, server.GetConnectionURL(), err)
	}

	return config, nil
}

func (c Client) UpdateSystem(ctx context.Context, server provisioning.Server, resource string, config any) error {
	if strings.Contains(resource, "/") {
		return fmt.Errorf(`Resource name must not contain forward slashes ("/")`)
	}

	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, path.Join("/os/1.0/system", resource), config, "")
	if err != nil {
		return fmt.Errorf("Failed to update system %s configuration on %q (%s): %w", resource, server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) GetSystemKernel(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemKernel, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return provisioning.ServerSystemKernel{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/kernel", http.NoBody, "")
	if err != nil {
		return provisioning.ServerSystemKernel{}, fmt.Errorf("Failed to get system kernel configuration on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	var kernelConfig provisioning.ServerSystemKernel
	err = json.Unmarshal(resp.Metadata, &kernelConfig)
	if err != nil {
		return provisioning.ServerSystemKernel{}, fmt.Errorf("Unexpected response metadata while fetching system kernel configuration from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return kernelConfig, nil
}

func (c Client) UpdateSystemKernel(ctx context.Context, server provisioning.Server, config provisioning.ServerSystemKernel) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/kernel", config, "")
	if err != nil {
		return fmt.Errorf("Failed to update system kernel configuration on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) GetSystemLogging(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemLogging, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return provisioning.ServerSystemLogging{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/logging", http.NoBody, "")
	if err != nil {
		return provisioning.ServerSystemLogging{}, fmt.Errorf("Failed to get system logging configuration on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	var loggingConfig provisioning.ServerSystemLogging
	err = json.Unmarshal(resp.Metadata, &loggingConfig)
	if err != nil {
		return provisioning.ServerSystemLogging{}, fmt.Errorf("Unexpected response metadata while fetching system logging configuration from %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return loggingConfig, nil
}

func (c Client) UpdateSystemLogging(ctx context.Context, server provisioning.Server, config provisioning.ServerSystemLogging) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/logging", config, "")
	if err != nil {
		return fmt.Errorf("Failed to update system logging configuration on %q (%s): %w", server.Name, server.GetConnectionURL(), err)
	}

	return nil
}
