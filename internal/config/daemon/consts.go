package config

import (
	_ "embed"
	"time"
)

// FIXME: Check which constants need to be public.

const (
	// Name of the application, also determines the directory names for e.g.
	// the applications var and log directories.
	ApplicationName = "operations-center"

	// Name of the executable.
	BinaryName = "operations-centerd"

	// Name of the env var prefix used by this application.
	ApplicationEnvPrefix = "OPERATIONS_CENTER"

	// Default TCP port used for REST.
	DefaultRestServerPort = "7443"

	// Interval in which the update server is polled for new updates.
	UpdatesSourcePollInterval = 1 * time.Hour

	// Interval in which the image sources are polled for new updates.
	ImageSourcePollInterval = 6 * time.Hour

	// Interval in which a connectivity check is performed for the servers
	// known by Operations Center.
	ConnectivityCheckInterval = 5 * time.Minute

	// Interval in which servers in pending state are queried.
	PendingServerPollInterval = 1 * time.Minute

	// Interval in which servers in updating state are queried.
	UpdatingServerPollInterval = 30 * time.Second

	// Interval in which servers in evacuating state are queried.
	EvacuatingServerPollInterval = 30 * time.Second

	// Interval in which servers in restoring state are queried.
	RestoringServerPollInterval = 30 * time.Second

	// Interval in which servers in rebooting state are quieried.
	RebootingServerPollInterval = 30 * time.Second

	// Interval in which servers in unresponsive state are queried.
	UnresponsiveServerPollInterval = 10 * time.Second

	// Interval in which the server state and configuration is updated in
	// Operations Center. Since collecting this information might be an expensive
	// operation on the servers, this information should not be quieried
	// excessively.
	InventoryUpdateInterval = 6 * time.Hour

	// Interval in which the BMC data is resynced.
	BMCDataResyncInterval = 1 * time.Hour

	// Interval in which the automated server deployment control loop is run.
	ServerDeploymentControlLoopInterval = 10 * time.Second

	// Number of times a single step of an automated server deployment is
	// retried, before the deployment is considered failed.
	ServerDeploymentStepRetries = 3

	// Base of the exponential backoff between two attempts of a step of an
	// automated server deployment.
	ServerDeploymentRetryBackoff = 5 * time.Second

	// Upper limit for the backoff between two attempts of a step of an
	// automated server deployment.
	ServerDeploymentRetryBackoffMax = 2 * time.Minute

	// Time granted to a single step of an automated server deployment, before
	// the respective action is triggered again.
	ServerDeploymentStepTimeout = 5 * time.Minute

	// Time granted to apply BIOS settings.
	ServerDeploymentStepWaitBIOSAppliedTimeout = 10 * time.Minute

	// Time granted to a server to come back up after the first stage of the
	// installation.
	ServerDeploymentRebootTimeout = 15 * time.Minute

	// Time, the reboot wait looks for an actual reboot of the server, before it
	// settles for the server merely not being powered off.
	ServerDeploymentRebootObservationWindow = 5 * time.Minute

	// Time granted to a server to register itself with Operations Center after
	// the installation has completed.
	ServerDeploymentRegistrationTimeout = 30 * time.Minute

	// Time without any read of the installation media, after which the first
	// stage of the installation is considered done. The period has to outlast the
	// phase, in which the installer partitions the disk without reading the media
	// anymore, but no more than that: it is the whole latency of the only signal
	// a deployment requested with force has, where the server is done installing
	// and waits for the media to go before it reboots.
	ServerDeploymentMediaIdlePeriod = 2 * time.Minute

	// Amount of the installation media, that has to have been read, before the
	// media idle period is taken as the signal, that the first stage of the
	// installation is done. A share of the image is no usable measure, since how
	// much of it the installer fetches depends on the image. Measured against a
	// 3.2 GiB image, the first stage covers about 920 MiB, so half of that is out
	// of reach for a firmware merely booting the media and is cleared well before
	// the installer stops reading.
	ServerDeploymentMediaMinBytesRead int64 = 500 * 1024 * 1024

	// Time, that has to have passed in the install wait, before the first stage
	// of the installation could be done at all, which the read progress of the
	// installation media is held against. A Lenovo ThinkSystem was through the
	// first stage in nine and a half minutes, so this stays well below what an
	// installation takes.
	ServerDeploymentMinInstallDuration = 5 * time.Minute

	// Time, that has to have passed in the install wait, before a reboot counts
	// as the end of the first stage, where the BMC reports no boot progress to
	// tell the reboot of the firmware, that picks up what has been staged for it,
	// from the one of the installer. It has to outlast the power on self test
	// plus the boot of the installation media, which is why it is longer than the
	// duration above, and it only applies where nothing better is available.
	ServerDeploymentInstallRebootFallbackDelay = 10 * time.Minute

	// Time, the server is left running after the secure boot certificates have
	// been enrolled, before the installation is started, where the firmware does
	// not reboot the server on its own to pick them up.
	ServerDeploymentSecureBootSettleDuration = 5 * time.Minute

	// Time granted to the first stage of the IncusOS installation.
	ServerDeploymentInstallTimeout = 45 * time.Minute

	// Time granted to an automated server deployment as a whole.
	ServerDeploymentTimeout = 2 * time.Hour

	// Time after a power on, after which a BMC task monitor, that the BMC does
	// not know anymore, is accepted as the applied BIOS attributes.
	ServerDeploymentSettleDelay = 1 * time.Minute

	// Maximum number of state transitions performed for a single server within
	// one tick of the automated server deployment control loop.
	ServerDeploymentMaxTransitionsPerTick = 10

	// Number of servers, whose deployment is advanced at the same time. A step
	// blocks on the BMC of its server, so the deployments have to progress
	// independently of each other.
	ServerDeploymentControlLoopConcurrency = 8

	// Time granted to the BMC operations of a single state of an automated server
	// deployment. It bounds the state, not the request, so an unresponsive BMC
	// ends the attempt instead of parking the control loop.
	ServerDeploymentStepCallTimeout = 2 * time.Minute

	// Time granted to attach the installation media, which a BMC, that uploads
	// the media instead of streaming it, only answers once it has read all of it.
	ServerDeploymentAttachMediaCallTimeout = 20 * time.Minute

	// Time granted to enroll the secure boot certificates, which removes every
	// entry of the key databases with a request of its own.
	ServerDeploymentSecureBootCallTimeout = 15 * time.Minute

	// Time granted to the secure boot enrollment media to enroll the
	// certificates, which covers the boot of the enrollment media plus the
	// reboot the firmware performs after having picked the certificates up.
	ServerDeploymentSecureBootEnrollTimeout = 15 * time.Minute

	// Time after the last access, after which a generated secure boot enrollment
	// media is removed.
	SecureBootMediaCacheTTL = 2 * time.Hour

	// Interval in which the generated secure boot enrollment media is pruned.
	SecureBootMediaPruneInterval = 10 * time.Minute

	// Time the deployment control loop is held back after a virtual media event,
	// so the BMC has reported the change by the time the loop looks.
	ServerDeploymentVirtualMediaTriggerDelay = 3 * time.Second

	// Time granted to await a BMC task monitor in the background.
	BMCTaskWaitTimeout = 15 * time.Minute

	// Time granted to collect the data of a BMC.
	BMCDataRefreshTimeout = 2 * time.Minute

	// Time after the last access, after which a cached seed image is removed.
	SeedImageCacheTTL = 2 * time.Hour

	// Interval in which the seed image cache is pruned.
	SeedImageCachePruneInterval = 10 * time.Minute

	// ACME server certificate renew interval.
	ACMEServerCertificateRenewInterval = 24 * time.Hour

	// Certificate validity check interval.
	CertificatesValidityCheckInterval = 24 * time.Hour

	// Threshold before expiration, at which a certificate expiration warning is emitted.
	CertificateExpiryWarningThreshold = 30 * 24 * time.Hour

	// Filename of the client certificate.
	ClientCertificateFilename = "client.crt"

	// Filename of the client key.
	ClientKeyFilename = "client.key"

	// Filename of the server certificate.
	ServerCertificateFilename = "server.crt"

	// Filename of the server key.
	ServerKeyFilename = "server.key"

	// Filename of the system config file.
	ConfigFilename = "config.yml"
)

//go:embed default.yml
var defaultConfig []byte
