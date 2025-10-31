package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incus "github.com/lxc/incus/v6/client"
	incusapi "github.com/lxc/incus/v6/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/logger"
	"github.com/FuturFusion/operations-center/internal/provisioning"
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
			t.TLSClientConfig.ServerName = serverName

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

	return api.OSData{
		Network:  network,
		Security: security,
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

func (c client) FactoryReset(ctx context.Context, endpoint provisioning.Endpoint) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodPost, "/os/1.0/system/:factory-reset", map[string]any{}, "")
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

	listener, err := client.GetEventsAllProjects()
	if err != nil {
		return nil, nil, err
	}

	// Allow for up to 100 in-flight events to prevent the sender or the websocket
	// connection from being blocked due to slow processing.
	lifecycleEvents := make(chan domain.LifecycleEvent, 100)
	errChan := make(chan error)
	target, err := listener.AddHandler(nil, func(event incusapi.Event) {
		lifecycleEvent, ok, err := mapIncusEventToLifecycleEvent(event)
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
	if err != nil {
		return nil, nil, err
	}

	go func() {
		select {
		// Disconnect, if we are done and the context is cancelled.
		case <-ctx.Done():
			err = listener.RemoveHandler(target)
			if err != nil {
				slog.WarnContext(ctx, "Failed ro remove handler from event listener", logger.Err(err))
			}

			listener.Disconnect()

		// Signal, if listener disconnect.
		case errChan <- listener.Wait():
			err = listener.RemoveHandler(target)
			if err != nil {
				slog.WarnContext(ctx, "Failed ro remove handler from event listener", logger.Err(err))
			}
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

func mapIncusEventToLifecycleEvent(event incusapi.Event) (domain.LifecycleEvent, bool, error) {
	if event.Type != incusapi.EventTypeLifecycle {
		return domain.LifecycleEvent{}, false, nil
	}

	incusLifecycleEvent := incusapi.EventLifecycle{}
	err := json.Unmarshal(event.Metadata, &incusLifecycleEvent)
	if err != nil {
		return domain.LifecycleEvent{}, false, err
	}

	sep := strings.LastIndex(incusLifecycleEvent.Action, "-")
	if sep < 1 || sep+1 >= len(incusLifecycleEvent.Action) {
		return domain.LifecycleEvent{}, false, fmt.Errorf("Incus lifecycle event action %q has invalid format", incusLifecycleEvent.Action)
	}

	resourceType, action := incusLifecycleEvent.Action[:sep], incusLifecycleEvent.Action[sep+1:]

	lifecycleEventResourceType := domain.LifecycleResourceType(resourceType)
	_, ok := domain.LifecycleResources[lifecycleEventResourceType]
	if !ok {
		// Life cycle resource not relevant, ignore.
		return domain.LifecycleEvent{}, false, nil
	}

	var lifecycleEventAction domain.LifecycleAction
	switch action {
	case "created":
		lifecycleEventAction = domain.LifecycleActionCreate

	case "updated", "enabled", "disabled", "added", "removed":
		lifecycleEventAction = domain.LifecycleActionUpdate

	case "deleted":
		lifecycleEventAction = domain.LifecycleActionDelete

	default:
		// Lifecycle action not relevant, ignore.
		return domain.LifecycleEvent{}, false, nil
	}

	var lifecycleEventType string

	incusEventContextTypeAny, ok := incusLifecycleEvent.Context["type"]
	if ok {
		lifecycleEventType, _ = incusEventContextTypeAny.(string) // nolint:revive // zero value is ok, if the type assertion fails.
	}

	// Example values of incusLifecycleEvent.Source:
	//
	//   /1.0/instances/d1 -> no parent
	//   /1.0/storage-pools/default/volumes/custom/default_foo -> parent type: storage-pools, parent name: default
	var lifecycleEventParentType string
	var lifecycleEventParentName string
	source := strings.TrimLeft(incusLifecycleEvent.Source, "/")
	sourceParts := strings.Split(source, "/")
	if len(sourceParts) > 3 {
		lifecycleEventParentType, _ = strings.CutSuffix(sourceParts[1], "s") // remove pluralization
		lifecycleEventParentName = sourceParts[2]
		sourceParts = sourceParts[2:]
	}

	name := incusLifecycleEvent.Name
	if name == "" {
		name = sourceParts[len(sourceParts)-1]
	}

	if lifecycleEventResourceType == domain.LifecycleResourceTypeStorageVolume && lifecycleEventType == "" {
		lifecycleEventType = sourceParts[len(sourceParts)-2]
	}

	return domain.LifecycleEvent{
		ResourceType: lifecycleEventResourceType,
		Action:       lifecycleEventAction,
		Source: domain.LifecycleSource{
			ParentType:  lifecycleEventParentType,
			ParentName:  lifecycleEventParentName,
			ProjectName: incusLifecycleEvent.Project,
			Name:        name,
			Type:        lifecycleEventType,
		},
	}, true, nil
}
