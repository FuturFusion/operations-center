package system

import (
	"context"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type SystemService interface {
	GetCertificate(ctx context.Context) (system.Certificate, error)
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

	CleanCache(ctx context.Context) error
}

// CertificateRepo stores the server certificate and key.
type CertificateRepo interface {
	// GetServerCertificate returns the PEM encoded server certificate.
	GetServerCertificate(ctx context.Context) (certificatePEM []byte, _ error)

	// GetServerKeyPair returns the PEM encoded server certificate and key.
	GetServerKeyPair(ctx context.Context) (certificatePEM []byte, keyPEM []byte, _ error)

	// UpdateServerKeyPair persists the PEM encoded server certificate and key.
	// It writes the certificate first. If it fails, only the certificate might
	// be written.
	UpdateServerKeyPair(ctx context.Context, certificatePEM []byte, keyPEM []byte) error
}

type CacheRepo interface {
	CleanupAll(ctx context.Context) error
}

type ProvisioningServerService interface {
	GetAll(ctx context.Context) (provisioning.Servers, error)
	GetAllWithFilter(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error)
	GetSystemProvider(ctx context.Context, name string) (provisioning.ServerSystemProvider, error)
	UpdateSystemProvider(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error
	RestartApplication(ctx context.Context, name string, applicationName string) error
}
