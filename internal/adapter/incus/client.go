package incus

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	incus "github.com/lxc/incus/v7/client"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/scriptlet"
	securitytls "github.com/FuturFusion/operations-center/internal/security/tls"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Environment provides access to the local Incus daemon unix socket.
type Environment interface {
	GetUnixSocket() string
}

type Client struct {
	keypair       *securitytls.ClientKeypairHolder
	clientCA      string
	env           Environment
	skipGetServer bool

	// isClusterMember determines if the endpoint is for a cluster member of this Operations Center.
	isClusterMember func(certificatePEM string) bool
	// clusterCertificate returns the certificate presented by all cluster members of this Operations Center.
	clusterCertificate func() string
}

var (
	_ provisioning.ServerClientPort  = Client{}
	_ provisioning.ClusterClientPort = Client{}
	_ provisioning.TokenClientPort   = Client{}
	_ scriptlet.ScriptletClientPort  = Client{}
	_ inventory.ServerClient         = Client{}
)

// Option configures a Client.
type Option func(*Client)

// WithEnvironment provides the runtime environment, which enables connecting to
// the local Incus daemon through its unix socket.
func WithEnvironment(env Environment) Option {
	return func(c *Client) {
		c.env = env
	}
}

// WithSkipGetServer skips the request for the server information, which is
// otherwise performed while establishing a connection. Note that HasExtension
// depends on this information and therefore always reports false, if the
// request is skipped.
func WithSkipGetServer(skipGetServer bool) Option {
	return func(c *Client) {
		c.skipGetServer = skipGetServer
	}
}

// WithOperationsCenterCluster sets the parameters for detecting servers that are cluster members with this server.
func WithOperationsCenterCluster(isClusterMember func(certificatePEM string) bool, clusterCertificate func() string) Option {
	return func(c *Client) {
		c.isClusterMember = isClusterMember
		c.clusterCertificate = clusterCertificate
	}
}

type transportWrapper struct {
	transport *http.Transport
}

func (t *transportWrapper) Transport() *http.Transport {
	return t.transport
}

func (t *transportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.transport.RoundTrip(req)
}

func New(keypair *securitytls.ClientKeypairHolder, opts ...Option) Client {
	c := Client{
		keypair: keypair,
	}

	for _, opt := range opts {
		opt(&c)
	}

	return c
}

// serverCertificate returns the certificate to pin for endpoint.
func (c Client) serverCertificate(endpoint provisioning.Endpoint) string {
	if c.isClusterMember == nil || c.clusterCertificate == nil {
		return endpoint.GetCertificate()
	}

	var serverType api.ServerType
	switch server := endpoint.(type) {
	case provisioning.Server:
		serverType = server.Type

	case *provisioning.Server:
		serverType = server.Type
	}

	if serverType == api.ServerTypeOperationsCenter && c.isClusterMember(endpoint.GetCertificate()) {
		cert := c.clusterCertificate()
		if cert != "" {
			return cert
		}
	}

	return endpoint.GetCertificate()
}

func (c Client) getClient(ctx context.Context, endpoint provisioning.Endpoint) (incus.InstanceServer, error) {
	if transaction.IsActive(ctx) {
		slog.WarnContext(ctx, "Incus API call inside of a transaction", logger.AddStacktrace())
	}

	serverName, err := endpoint.GetServerName()
	if err != nil {
		return nil, err
	}

	// If the provisioning.ServerSelf endpoint is used, connect through unix socket.
	if endpoint.GetConnectionURL() == provisioning.ServerSelf.ConnectionURL {
		if c.env == nil {
			return nil, fmt.Errorf("Failed to connect to %q: no environment configured", provisioning.ServerSelf.ConnectionURL)
		}

		return incus.ConnectIncusUnix(c.env.GetUnixSocket(), &incus.ConnectionArgs{})
	}

	keypair := c.keypair.Get()
	serverCert := c.serverCertificate(endpoint)

	args := &incus.ConnectionArgs{
		TLSClientCert: keypair.Certificate,
		TLSClientKey:  keypair.Key,
		TLSServerCert: serverCert,
		TLSCA:         c.clientCA,
		SkipGetServer: c.skipGetServer,
		TransportWrapper: func(t *http.Transport) incus.HTTPTransporter {
			if serverCert == "" {
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

// HasExtension reports whether the endpoint supports the given API extension.
// It reports false, if the connection can not be established or if the client
// is configured with WithSkipGetServer, since the extensions are part of the
// server information.
func (c Client) HasExtension(ctx context.Context, endpoint provisioning.Endpoint, extension string) (exists bool) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return false
	}

	return client.HasExtension(extension)
}

func (c Client) Ping(ctx context.Context, endpoint provisioning.Endpoint) error {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodGet, "/", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to ping %q (%s): %w", endpoint.GetName(), endpoint.GetConnectionURL(), err)
	}

	return nil
}

func (c Client) IncusClient(ctx context.Context, endpoint provisioning.Endpoint) (incus.InstanceServer, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	return client, nil
}
