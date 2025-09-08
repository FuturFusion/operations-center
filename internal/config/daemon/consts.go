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
	DefaultRestServerPort = 7443

	// Interval in which the update server is polled for new updates.
	UpdatesSourcePollInterval = 1 * time.Hour

	// Interval in which a connectivity check is performed for the servers
	// known by Operations Center.
	ConnectivityCheckInterval = 5 * time.Minute

	// Interval in which servers in pending state are queried.
	PendingServerPollInterval = 1 * time.Minute

	// Interval in which the server state and configuration is updated in
	// Operations Center. Since collecting this information might be an expensive
	// operation on the servers, this information should not be quieried
	// excessively.
	InventoryUpdateInterval = 1 * time.Hour

	// Filename of the client certificate.
	ClientCertificateFilename = "client.crt"

	// Filename of the client key.
	ClientKeyFilename = "client.key"

	// Filename of the system config file.
	ConfigFilename = "config.yml"
)

//go:embed default.yml
var defaultConfig []byte
