package incusos

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

type client struct {
	clientCert []byte
	clientKey  []byte
}

var _ provisioning.ServerClientPort = &client{}

func New(clientCert []byte, clientKey []byte) *client {
	return &client{
		clientCert: clientCert,
		clientKey:  clientKey,
	}
}

type serverClient struct {
	http.Client
}

func (c client) getClient(server provisioning.Server) (*serverClient, error) {
	cert, err := tls.X509KeyPair(c.clientCert, c.clientKey)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM([]byte(server.Certificate))

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
			RootCAs:      caPool,
		},
	}

	return &serverClient{
		Client: http.Client{
			Transport: transport,
		},
	}, nil
}

func (c *client) Ping(ctx context.Context, server provisioning.Server) error {
	client, err := c.getClient(server)
	if err != nil {
		return err
	}

	resp, err := client.Get(server.ConnectionURL)
	if err != nil {
		return fmt.Errorf("Ping to %q failed: %w", server.ConnectionURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected response for ping to %q: %d %s", server.ConnectionURL, resp.StatusCode, resp.Status)
	}

	return nil
}
