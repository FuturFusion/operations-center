package incus

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	incus "github.com/lxc/incus/v6/client"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type serverClient struct {
	clientCert    string
	clientKey     string
	clientCA      string
	skipGetServer bool
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

type Option func(c *serverClient)

func WithSkipGetServer(skipGetServer bool) Option {
	return func(s *serverClient) {
		s.skipGetServer = skipGetServer
	}
}

func New(clientCert string, clientKey string, opts ...Option) inventory.ServerClient {
	c := serverClient{
		clientCert: clientCert,
		clientKey:  clientKey,
	}

	for _, opt := range opts {
		opt(&c)
	}

	return c
}

func (s serverClient) getClient(ctx context.Context, endpoint provisioning.Endpoint) (incus.InstanceServer, error) {
	serverName, err := endpoint.GetServerName()
	if err != nil {
		return nil, err
	}

	args := &incus.ConnectionArgs{
		TLSClientCert: s.clientCert,
		TLSClientKey:  s.clientKey,
		TLSServerCert: endpoint.GetCertificate(),
		TLSCA:         s.clientCA,
		SkipGetServer: s.skipGetServer,
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

func (s serverClient) HasExtension(ctx context.Context, endpoint provisioning.Endpoint, extension string) (exists bool) {
	client, err := s.getClient(ctx, endpoint)
	if err != nil {
		return false
	}

	return client.HasExtension(extension)
}

func (s serverClient) Ping(ctx context.Context, endpoint provisioning.Endpoint) error {
	client, err := s.getClient(ctx, endpoint)
	if err != nil {
		return err
	}

	_, _, err = client.RawQuery(http.MethodGet, "/", http.NoBody, "")
	if err != nil {
		return fmt.Errorf("Failed to ping %q: %w", endpoint.GetConnectionURL(), err)
	}

	return nil
}
