package environment

import (
	"os"
	"path/filepath"
)

const (
	logPathDefaultPrefix = "/var"
	logPathSuffix        = "log"
	runPathDefaultPrefix = "/run"
	varPathDefaultPrefix = "/var/lib"

	applicationDirEnvSuffix    = "_DIR"
	applicationSocketEnvSuffix = "_SOCKET"
)

// Environment is a high-level facade for accessing operating-system level functionalities.
type Environment struct {
	applicationName      string
	applicationEnvPrefix string
}

// New returns an Environment initialized with sane default values.
// The applicationName might be added to directory paths where reasonable.
// The applicationNameEnvPrefix is used to form the names of environment
// variables, that can be used to override the default paths.
// For example with the applicationNameEnvPrefix "APP", the env var
// APP_DIR is formed.
func New(applicationName, applicationEnvPrefix string) Environment {
	return Environment{
		applicationName:      applicationName,
		applicationEnvPrefix: applicationEnvPrefix,
	}
}

// LogDir returns the path to the log directory of the application (e.g. /var/log/).
// It respects <APP_PREFIX>_DIR environment variable.
func (e Environment) LogDir() string {
	return e.pathWithEnvOverride(logPathDefaultPrefix, logPathSuffix)
}

// RunDir returns the path to the runtime directory of the application (e.g. /run/<application-name>).
// It respects <APP_PREFIX>_DIR environment variable.
func (e Environment) RunDir() string {
	return e.pathWithEnvOverride(runPathDefaultPrefix, e.applicationName)
}

// VarDir returns the path to the data directory of the application (e.g. /var/lib/<application-name>).
// It respects <APP_PREFIX>_DIR environment variable.
func (e Environment) VarDir() string {
	return e.pathWithEnvOverride(varPathDefaultPrefix, e.applicationName)
}

// GetUnixSocket returns the full file name of the unix socket.
func (e Environment) GetUnixSocket() string {
	path := os.Getenv(e.applicationEnvPrefix + applicationSocketEnvSuffix)
	if path != "" {
		return path
	}

	return filepath.Join(e.RunDir(), "unix.socket")
}

// pathWithEnvOverride returns the directory combined from prefixDir and suffixDir
// where the prefix maybe overridden by a value provided by the prefixDirEnvVar.
func (e Environment) pathWithEnvOverride(prefixDir, suffixDir string) string {
	dirEnvVar := e.applicationEnvPrefix + applicationDirEnvSuffix
	prefix := prefixDir
	if os.Getenv(dirEnvVar) != "" {
		return os.Getenv(dirEnvVar)
	}

	return filepath.Join(prefix, suffixDir)
}
