package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus-os/incus-osd/api/seed"
	incus "github.com/lxc/incus/v6/client"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/shared/api"
)

type client struct {
	clientCert string
	clientKey  string
	clientCA   string
}

var (
	_ provisioning.ServerClientPort  = client{}
	_ provisioning.ClusterClientPort = client{}
)

type transportWrapper struct {
	transport *http.Transport
}

func (t *transportWrapper) Transport() *http.Transport {
	return t.transport
}

func (t *transportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.transport.RoundTrip(req)
}

func New(clientCert string, clientKey string) client {
	return client{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

func (c client) getClient(ctx context.Context, endpoint provisioning.Endpoint) (incus.InstanceServer, error) {
	serverName, err := endpoint.GetServerName()
	if err != nil {
		return nil, err
	}

	args := &incus.ConnectionArgs{
		TLSClientCert: c.clientCert,
		TLSClientKey:  c.clientKey,
		TLSServerCert: endpoint.GetCertificate(),
		TLSCA:         c.clientCA,
		SkipGetServer: true,
		TransportWrapper: func(t *http.Transport) incus.HTTPTransporter {
			if endpoint.GetCertificate() == "" {
				t.TLSClientConfig.ServerName = serverName
			}

			return &transportWrapper{transport: t}
		},

		// Bypass system proxy for communication to IncusOS servers.
		Proxy: func(r *http.Request) (*url.URL, error) {
			return nil, nil
		},
	}

	return incus.ConnectIncusWithContext(ctx, endpoint.GetConnectionURL(), args)
}

func (c client) Ping(ctx context.Context, endpoint provisioning.Endpoint) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodGet, "/", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to ping %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nil
}

func (c client) GetResources(ctx context.Context, endpoint provisioning.Endpoint) (api.HardwareData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.HardwareData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/resources", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.HardwareData{}, fmt.Errorf("Get resources from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var resources incusapi.Resources
	err = json.Unmarshal(resp.Metadata, &resources)
	if err != nil {
		return api.HardwareData{}, fmt.Errorf("Unexpected response metadata while getting resource information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	return api.HardwareData{
		Resources: resources,
	}, nil
}

func (c client) GetOSData(ctx context.Context, endpoint provisioning.Endpoint) (api.OSData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.OSData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/network", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.OSData{}, fmt.Errorf("Get OS network data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var network incusosapi.SystemNetwork
	err = json.Unmarshal(resp.Metadata, &network)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS network information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/security", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS security data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var security incusosapi.SystemSecurity
	err = json.Unmarshal(resp.Metadata, &security)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS security information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/storage", http.NoBody, "")
	if err != nil {
		return api.OSData{}, fmt.Errorf("Get OS storage data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var storage incusosapi.SystemStorage
	err = json.Unmarshal(resp.Metadata, &storage)
	if err != nil {
		return api.OSData{}, fmt.Errorf("Unexpected response metadata while fetching OS storage information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	return api.OSData{
		Network:  network,
		Security: security,
		Storage:  storage,
	}, nil
}

func (c client) GetVersionData(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerVersionData, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.ServerVersionData{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.ServerVersionData{}, fmt.Errorf("Get OS version data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var osVersionData struct {
		Environment struct {
			OSName        string `json:"os_name"`
			OSVersion     string `json:"os_version"`
			OSVersionNext string `json:"os_version_next"`
		} `json:"environment"`
	}
	err = json.Unmarshal(resp.Metadata, &osVersionData)
	if err != nil {
		return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching OS version information from %q: %w", endpoint.GetConnectionURL(), err)
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/applications", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.ServerVersionData{}, fmt.Errorf("Get applications from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var applications []string
	err = json.Unmarshal(resp.Metadata, &applications)
	if err != nil {
		return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching applications from %q: %w", endpoint.GetConnectionURL(), err)
	}

	applicationVersions := make([]api.ApplicationVersionData, 0, len(applications))
	for _, applicationURL := range applications {
		applicationName := path.Base(applicationURL)

		resp, _, err = client.RawQuery(http.MethodGet, path.Join("/os/1.0/applications", applicationName), http.NoBody, "")
		if err != nil {
			err = api.AsNotIncusOSError(err)

			return api.ServerVersionData{}, fmt.Errorf("Get application version data for %q from %q failed: %w", applicationName, endpoint.GetConnectionURL(), err)
		}

		var application incusosapi.Application
		err = json.Unmarshal(resp.Metadata, &application)
		if err != nil {
			return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching application %q from %q: %w", applicationName, endpoint.GetConnectionURL(), err)
		}

		applicationVersions = append(applicationVersions, api.ApplicationVersionData{
			Name:    applicationName,
			Version: application.State.Version,
		})
	}

	resp, _, err = client.RawQuery(http.MethodGet, "/os/1.0/system/update", http.NoBody, "")
	if err != nil {
		err = api.AsNotIncusOSError(err)

		return api.ServerVersionData{}, fmt.Errorf("Get OS version data from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var systemUpdate incusosapi.SystemUpdate
	err = json.Unmarshal(resp.Metadata, &systemUpdate)
	if err != nil {
		return api.ServerVersionData{}, fmt.Errorf("Unexpected response metadata while fetching system update information from %q: %w", endpoint.GetConnectionURL(), err)
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

func (c client) GetServerType(ctx context.Context, endpoint provisioning.Endpoint) (api.ServerType, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return api.ServerTypeUnknown, err
	}

	const endpointPath = "/os/1.0/applications"

	resp, _, err := client.RawQuery(http.MethodGet, endpointPath, http.NoBody, "")
	if err != nil {
		return api.ServerTypeUnknown, fmt.Errorf("Get applications from %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	var applications []string
	err = json.Unmarshal(resp.Metadata, &applications)
	if err != nil {
		return api.ServerTypeUnknown, fmt.Errorf("Unexpected response metadata while fetching applications from %q: %w", endpoint.GetConnectionURL(), err)
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

	return api.ServerTypeUnknown, fmt.Errorf("Server did not return any known server type defining application (%v)", applications)
}

func (c client) UpdateNetworkConfig(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/network", server.OSData.Network, "")
	if err != nil {
		return fmt.Errorf("Put OS network data to %q failed: %w", server.ConnectionURL, err)
	}

	return nil
}

func (c client) UpdateStorageConfig(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/storage", server.OSData.Storage, "")
	if err != nil {
		return fmt.Errorf("Put OS storage data to %q failed: %w", server.ConnectionURL, err)
	}

	return nil
}

func (c client) GetProviderConfig(ctx context.Context, server provisioning.Server) (provisioning.ServerSystemProvider, error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return incusosapi.SystemProvider{}, err
	}

	resp, _, err := client.RawQuery(http.MethodGet, "/os/1.0/system/provider", nil, "")
	if err != nil {
		return incusosapi.SystemProvider{}, fmt.Errorf("Put OS provider config to %q failed: %w", server.ConnectionURL, err)
	}

	var providerConfig incusosapi.SystemProvider
	err = json.Unmarshal(resp.Metadata, &providerConfig)
	if err != nil {
		return incusosapi.SystemProvider{}, fmt.Errorf("Unexpected response metadata while getting provider information from %q: %w", server.GetConnectionURL(), err)
	}

	return providerConfig, nil
}

func (c client) UpdateProviderConfig(ctx context.Context, server provisioning.Server, providerConfig provisioning.ServerSystemProvider) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/system/provider", providerConfig, "")
	if err != nil {
		return fmt.Errorf("Put OS provider config to %q failed: %w", server.ConnectionURL, err)
	}

	return nil
}

func (c client) Poweroff(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/:poweroff", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to poweroff %q: %w", server.GetConnectionURL(), err)
	}

	return nil
}

func (c client) Reboot(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/:reboot", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to reboot %q: %w", server.GetConnectionURL(), err)
	}

	return nil
}

func (c client) EnableOSService(ctx context.Context, server provisioning.Server, name string, config map[string]any) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	nameSanitized := url.PathEscape(name)

	serviceConfig := map[string]any{
		"config": config,
	}

	_, _, err = client.RawQuery(http.MethodPut, "/os/1.0/services/"+nameSanitized, serviceConfig, "")
	if err != nil {
		return fmt.Errorf("Enable OS service %q on %q failed: %w", nameSanitized, server.ConnectionURL, err)
	}

	return nil
}

func (c client) SetServerConfig(ctx context.Context, endpoint provisioning.Endpoint, config map[string]string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	svr, etag, err := client.GetServer()
	if err != nil {
		return fmt.Errorf("Failed to get current config from %q: %w", endpoint.GetConnectionURL(), err)
	}

	if svr.Config == nil {
		svr.Config = map[string]string{}
	}

	for key, value := range config {
		svr.Config[key] = value
	}

	err = client.UpdateServer(svr.Writable(), etag)
	if err != nil {
		return fmt.Errorf("Failed to set config on %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nil
}

func (c client) EnableCluster(ctx context.Context, server provisioning.Server) (clusterCertificate string, _ error) {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return "", err
	}

	req := incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
		},
	}

	op, err := client.UpdateCluster(req, "")
	if err != nil {
		return "", fmt.Errorf("Failed to update cluster on %q: %w", server.GetConnectionURL(), err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return "", fmt.Errorf("Failed to update cluster on %q: %w", server.GetConnectionURL(), err)
	}

	anyClusterCertificate, ok := op.Get().Metadata["certificate"]
	if !ok {
		return "", nil
	}

	clusterCertificate, ok = anyClusterCertificate.(string)
	if !ok {
		return "", nil
	}

	return clusterCertificate, nil
}

func (c client) GetClusterNodeNames(ctx context.Context, endpoint provisioning.Endpoint) ([]string, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	nodeNames, err := client.GetClusterMemberNames()
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster node names on %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nodeNames, nil
}

func (c client) GetClusterJoinToken(ctx context.Context, endpoint provisioning.Endpoint, memberName string) (joinToken string, _ error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return "", err
	}

	op, err := client.CreateClusterMember(incusapi.ClusterMembersPost{
		ServerName: memberName,
	})
	if err != nil {
		return "", fmt.Errorf("Failed to get cluster join token on %q: %w", endpoint.GetConnectionURL(), err)
	}

	opAPI := op.Get()
	token, err := opAPI.ToClusterJoinToken()
	if err != nil {
		return "", fmt.Errorf("Failed converting token operation to join token: %w", err)
	}

	return token.String(), nil
}

func (c client) JoinCluster(ctx context.Context, server provisioning.Server, joinToken string, endpoint provisioning.Endpoint) error {
	client, err := c.getClient(ctx, server)
	if err != nil {
		return err
	}

	// Ignore error, connection URL has been parsed by incus client already.
	serverAddressURL, _ := url.Parse(server.ConnectionURL)
	clusterAddressURL, _ := url.Parse(endpoint.GetConnectionURL())

	op, err := client.UpdateCluster(incusapi.ClusterPut{
		Cluster: incusapi.Cluster{
			ServerName: server.Name,
			Enabled:    true,
			// TODO: Add storage pool config?
			MemberConfig: []incusapi.ClusterMemberConfigKey{},
		},
		ClusterCertificate: endpoint.GetCertificate(),
		ServerAddress:      serverAddressURL.Host,
		ClusterToken:       joinToken,
		ClusterAddress:     clusterAddressURL.Host,
	}, "")
	if err != nil {
		return fmt.Errorf("Failed to update cluster during cluster join on %q: %w", server.GetConnectionURL(), err)
	}

	err = op.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("Failed to wait for update operation during cluster join on %q: %w", server.GetConnectionURL(), err)
	}

	return nil
}

func (c client) UpdateClusterCertificate(ctx context.Context, endpoint provisioning.Endpoint, certificatePEM string, keyPEM string) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	return client.UpdateClusterCertificate(incusapi.ClusterCertificatePut{
		ClusterCertificate:    certificatePEM,
		ClusterCertificateKey: keyPEM,
	}, "")
}

func (c client) SystemFactoryReset(ctx context.Context, endpoint provisioning.Endpoint, allowTPMResetFailure bool, seedConfig provisioning.TokenImageSeedConfigs, providerConfig api.TokenProviderConfig) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	providerSeed := &seed.Provider{
		SystemProviderConfig: providerConfig.SystemProviderConfig,
		Version:              providerConfig.Version,
	}

	seedData := map[string]any{
		"applications": seedConfig.Applications,
		"incus":        seedConfig.Incus,
		"provider":     providerSeed,
	}

	if len(seedConfig.Network) > 0 {
		seedData["network"] = seedConfig.Network
	}

	if len(seedConfig.Update) > 0 {
		seedData["update"] = seedConfig.Update
	}

	resetData := map[string]any{
		"allow_tpm_reset_failure": allowTPMResetFailure,
		"seeds":                   seedData,
		"wipe_existing_seeds":     false,
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/:factory-reset", resetData, "")
	if err != nil {
		return fmt.Errorf("Factory reset on %q failed: %w", endpoint.GetConnectionURL(), err)
	}

	return nil
}

func (c client) SubscribeLifecycleEvents(ctx context.Context, endpoint provisioning.Endpoint) (chan domain.LifecycleEvent, chan error, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}

	listener, err := client.GetEventsAllProjectsByType([]string{incusapi.EventTypeLifecycle})
	if err != nil {
		return nil, nil, err
	}

	// Allow for up to 100 in-flight events to prevent the sender or the websocket
	// connection from being blocked due to slow processing.
	lifecycleEvents := make(chan domain.LifecycleEvent, 100)
	errChan := make(chan error)
	// ignore the error, only happens, if the passed function is nil.
	target, _ := listener.AddHandler([]string{incusapi.EventTypeLifecycle}, func(event incusapi.Event) {
		lifecycleEvent, ok, err := mapIncusEventToLifecycleEvent(ctx, event)
		if err != nil {
			slog.WarnContext(ctx, "Failed to map incus event to lifecycle event", logger.Err(err))
			return
		}

		if !ok {
			return
		}

		select {
		case lifecycleEvents <- lifecycleEvent:
		case <-ctx.Done():
			return
		}
	})

	go func() {
		select {
		// Disconnect, if we are done and the context is cancelled.
		case <-ctx.Done():
			// ignore the error, unlikely to happen und there is not really anything we can do about it.
			_ = listener.RemoveHandler(target)

			listener.Disconnect()

		// Signal, if listener disconnect.
		case errChan <- listener.Wait():
			// ignore the error, unlikely to happen und there is not really anything we can do about it.
			_ = listener.RemoveHandler(target)
		}

		// Block potential senders, the will be "released" when the context is cancelled.
		// We can not close the channel here, since already inflight handlers might still
		// try to send on the channel, if these have been spawned before the handler
		// has been removed.
		lifecycleEvents = nil
		close(errChan)
	}()

	return lifecycleEvents, errChan, nil
}

func mapIncusEventToLifecycleEvent(ctx context.Context, event incusapi.Event) (domain.LifecycleEvent, bool, error) {
	if event.Type != incusapi.EventTypeLifecycle {
		return domain.LifecycleEvent{}, false, nil
	}

	incusLifecycleEvent := incusapi.EventLifecycle{}
	err := json.Unmarshal(event.Metadata, &incusLifecycleEvent)
	if err != nil {
		return domain.LifecycleEvent{}, false, err
	}

	slog.DebugContext(ctx, "map incus event to lifecycle event - inputs",
		slog.Any("event", map[string]any{
			"type":     event.Type,
			"project":  event.Project,
			"location": event.Location,
		}),
		slog.Any("metadata", map[string]any{
			"action":    incusLifecycleEvent.Action,
			"context":   incusLifecycleEvent.Context,
			"name":      incusLifecycleEvent.Name,
			"source":    incusLifecycleEvent.Source,
			"project":   incusLifecycleEvent.Project,
			"requestor": ptr.From(incusLifecycleEvent.Requestor),
		}),
	)

	lifecycleResourceTypeOperation, ok := domain.MapLifecycleAction[incusLifecycleEvent.Action]
	if !ok {
		return domain.LifecycleEvent{}, false, nil
	}

	// Example values of incusLifecycleEvent.Source:
	//
	//   /1.0/instances/d1 -> no parent
	//   /1.0/storage-pools/default/volumes/custom/default_foo -> parent type: storage-pools, parent name: default
	//   /1.0/profiles/some-profile?project=some-project
	sourceURL, err := url.Parse(incusLifecycleEvent.Source)
	if err != nil {
		return domain.LifecycleEvent{}, false, err
	}

	name := firstNonEmpty(incusLifecycleEvent.Name, path.Base(sourceURL.Path))
	projectName := firstNonEmpty(incusLifecycleEvent.Project, event.Project, sourceURL.Query().Get("project"), "default")

	// Process source for the existens of a parent.
	var lifecycleEventParentType string
	var lifecycleEventParentName string
	sourceParts := strings.Split(strings.TrimLeft(sourceURL.Path, "/"), "/")
	if len(sourceParts) > 3 {
		lifecycleEventParentType, _ = strings.CutSuffix(sourceParts[1], "s") // remove pluralization
		lifecycleEventParentName = sourceParts[2]
	}

	// Rename events provide the old name of the resource in "old_name".
	var oldName string
	if lifecycleResourceTypeOperation.Operation == domain.LifecycleOperationRename {
		oldNameAny, ok := incusLifecycleEvent.Context["old_name"]
		if ok {
			oldName, _ = oldNameAny.(string) // nolint:revive // zero value is ok, if the type assertion fails.
		}
	}

	// For storage volumes, the type is also part of the identifying key.
	var lifecycleEventType string
	if lifecycleResourceTypeOperation.ResourceType == domain.ResourceTypeStorageVolume {
		incusEventContextTypeAny, ok := incusLifecycleEvent.Context["type"]
		if ok {
			lifecycleEventType, _ = incusEventContextTypeAny.(string) // nolint:revive // zero value is ok, if the type assertion fails.
		}

		lifecycleEventType = firstNonEmpty(
			lifecycleEventType,
			path.Base(path.Dir(sourceURL.Path)), // Second last segment of the path is the type of the storage volume.
		)
	}

	ret := domain.LifecycleEvent{
		ResourceType: lifecycleResourceTypeOperation.ResourceType,
		Operation:    lifecycleResourceTypeOperation.Operation,
		Source: domain.LifecycleSource{
			ParentType:  lifecycleEventParentType,
			ParentName:  lifecycleEventParentName,
			ProjectName: projectName,
			Name:        name,
			Type:        lifecycleEventType,
			OldName:     oldName,
		},
	}

	slog.DebugContext(ctx, "map incus event to lifecycle event - return",
		slog.Any("lifecycle_event", ret),
	)

	return ret, true, nil
}

func firstNonEmpty(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}

	return ""
}
