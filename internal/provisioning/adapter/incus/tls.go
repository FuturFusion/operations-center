package incus

import (
	"context"
	"crypto/x509"

	incustls "github.com/lxc/incus/v6/shared/tls"

	"github.com/FuturFusion/operations-center/internal/provisioning"
)

func (c client) GetRemoteCertificate(_ context.Context, endpoint provisioning.Endpoint) (*x509.Certificate, error) {
	return incustls.GetRemoteCertificate(endpoint.GetConnectionURL(), "")
}
