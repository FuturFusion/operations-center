package system

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type SystemService interface {
	UpdateCertificate(ctx context.Context, certificatePEM string, keyPEM string) error
	TriggerCertificateRenew(ctx context.Context, force bool) (changed bool, _ error)

	GetNetworkConfig(ctx context.Context) system.Network
	UpdateNetworkConfig(ctx context.Context, cfg system.NetworkPut) error

	GetSecurityConfig(ctx context.Context) system.Security
	UpdateSecurityConfig(ctx context.Context, cfg system.SecurityPut) error

	GetSettingsConfig(ctx context.Context) system.Settings
	UpdateSettingsConfig(ctx context.Context, cfg system.SettingsPut) error

	GetUpdatesConfig(ctx context.Context) system.Updates
	UpdateUpdatesConfig(ctx context.Context, cfg system.UpdatesPut) error
}

type ProvisioningServerService interface {
	GetAll(ctx context.Context) (provisioning.Servers, error)
	GetSystemProvider(ctx context.Context, name string) (provisioning.ServerSystemProvider, error)
	UpdateSystemProvider(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error
}
