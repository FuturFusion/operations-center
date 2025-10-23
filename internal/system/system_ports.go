package system

import (
	"context"

	"github.com/FuturFusion/operations-center/shared/api/system"
)

type SystemService interface {
	UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error

	GetNetworkConfig(ctx context.Context) system.SystemNetwork
	UpdateNetworkConfig(ctx context.Context, cfg system.SystemNetworkPut) error

	GetSecurityConfig(ctx context.Context) system.SystemSecurity
	UpdateSecurityConfig(ctx context.Context, cfg system.SystemSecurityPut) error

	GetUpdatesConfig(ctx context.Context) system.SystemUpdates
	UpdateUpdatesConfig(ctx context.Context, cfg system.SystemUpdatesPut) error
}
