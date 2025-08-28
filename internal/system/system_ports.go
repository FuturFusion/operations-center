package system

import (
	"context"

	"github.com/FuturFusion/operations-center/shared/api"
)

type SystemService interface {
	UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error

	GetNetworkConfig(ctx context.Context) api.SystemNetwork
	UpdateNetworkConfig(ctx context.Context, cfg api.SystemNetworkPut) error

	GetSecurityConfig(ctx context.Context) api.SystemSecurity
	UpdateSecurityConfig(ctx context.Context, cfg api.SystemSecurityPut) error

	GetUpdatesConfig(ctx context.Context) api.SystemUpdates
	UpdateUpdatesConfig(ctx context.Context, cfg api.SystemUpdatesPut) error
}
