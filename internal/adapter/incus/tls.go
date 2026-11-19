package incus

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"

	incustls "github.com/lxc/incus/v7/shared/tls"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (c Client) GetRemoteCertificate(_ context.Context, endpoint provisioning.Endpoint) (*x509.Certificate, error) {
	return incustls.GetRemoteCertificate(endpoint.GetConnectionURL(), "")
}

func (c Client) GetPublicCertificate(ctx context.Context, publicURL string) (*x509.Certificate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, publicURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return nil, errors.New("Connection has no TLS peer certificate")
	}

	return resp.TLS.PeerCertificates[0], nil
}
