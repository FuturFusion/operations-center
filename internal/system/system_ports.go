package system

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api"
)

type SystemService interface {
	UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error

	GetNetworkConfig(ctx context.Context) api.SystemNetwork
	UpdateNetworkConfig(ctx context.Context, cfg api.SystemNetworkPut) error

	GetSecurityConfig(ctx context.Context) api.SystemSecurity
	UpdateSecurityConfig(ctx context.Context, cfg api.SystemSecurityPut) error

	GetSettingsConfig(ctx context.Context) api.SystemSettings
	UpdateSettingsConfig(ctx context.Context, cfg api.SystemSettingsPut) error

	GetUpdatesConfig(ctx context.Context) api.SystemUpdates
	UpdateUpdatesConfig(ctx context.Context, cfg api.SystemUpdatesPut) error
}

type ProvisioningServerService interface {
	GetAll(ctx context.Context) (provisioning.Servers, error)
	GetSystemProvider(ctx context.Context, name string) (provisioning.ServerSystemProvider, error)
	UpdateSystemProvider(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error
}
