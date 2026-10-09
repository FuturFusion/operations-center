package system

import (
	"context"
	"io"

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

	Backup(ctx context.Context, options BackupOptions) (io.ReadCloser, error)
	Restore(ctx context.Context, archive io.Reader) error
}

type CacheRepo interface {
	CleanupAll(ctx context.Context) error
}

type BackupRepo interface {
	// Create returns the state as gzip compressed tar archive, snapshotDatabase writes the database to include to dir.
	Create(ctx context.Context, options BackupOptions, snapshotDatabase func(ctx context.Context, dir string) error) (io.ReadCloser, error)

	// Extract extracts archive and returns the directory holding the extracted state, which replaces a previous one.
	Extract(ctx context.Context, archive io.Reader) (dir string, _ error)

	// Stage hands the extracted state over to be applied by the next start.
	Stage(ctx context.Context) error

	// Discard removes the extracted state.
	Discard(ctx context.Context) error
}

type DatabaseRepo interface {
	// Snapshot writes a copy of the database without the inventory to dir, the images are only kept, if they are part of the backup.
	Snapshot(ctx context.Context, dir string, options BackupOptions) error

	// SchemaVersion returns the current and the latest schema version of the database in dir.
	SchemaVersion(ctx context.Context, dir string) (current int, latest int, _ error)
}

type ProvisioningClusterService interface {
	GetAll(ctx context.Context) (provisioning.Clusters, error)
}

type ProvisioningServerService interface {
	GetAll(ctx context.Context) (provisioning.Servers, error)
	GetAllWithFilter(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error)
	GetSystemProvider(ctx context.Context, name string) (provisioning.ServerSystemProvider, error)
	UpdateSystemProvider(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error
	RestartApplication(ctx context.Context, name string, applicationName string) error
}
