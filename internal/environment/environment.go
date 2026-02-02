package environment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"

	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/shared/api"
)

const (
	logPathDefaultPrefix      = "/var"
	logPathSuffix             = "log"
	runPathDefaultPrefix      = "/run"
	varPathDefaultPrefix      = "/var/lib"
	cachePathDefaultPrefix    = "/var/cache"
	usrSharePathDefaultPrefix = "/usr/share"

	applicationDirEnvSuffix    = "_DIR"
	applicationSocketEnvSuffix = "_SOCKET"
	applicationConfEnvSuffix   = "_CONF"
)

type Environment interface {
	LogDir() string
	RunDir() string
	VarDir() string
	CacheDir() string
	UsrShareDir() string
	GetUnixSocket() string
	UserConfigDir() (string, error)
	IsIncusOS() bool
	GetToken(ctx context.Context) (string, error)
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// environment is a high-level facade for accessing operating-system level functionalities.
type environment struct {
	applicationName      string
	applicationEnvPrefix string

	httpClient httpClient
}

var _ Environment = environment{}

// New returns an Environment initialized with sane default values.
// The applicationName might be added to directory paths where reasonable.
// The applicationNameEnvPrefix is used to form the names of environment
// variables, that can be used to override the default paths.
// For example with the applicationNameEnvPrefix "APP", the env var
// APP_DIR is formed.
func New(applicationName, applicationEnvPrefix string) Environment {
	return environment{
		applicationName:      applicationName,
		applicationEnvPrefix: applicationEnvPrefix,

		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _ string, _ string) (net.Conn, error) {
					raddr, err := net.ResolveUnixAddr("unix", IncusOSSocket)
					if err != nil {
						return nil, err
					}

					return net.DialUnix("unix", nil, raddr)
				},

				DisableKeepAlives: true,
			},
		},
	}
}

// LogDir returns the path to the log directory of the application (e.g. /var/log/).
// It respects <APP_PREFIX>_DIR environment variable.
func (e environment) LogDir() string {
	return e.pathWithEnvOverride(logPathDefaultPrefix, logPathSuffix)
}

// RunDir returns the path to the runtime directory of the application (e.g. /run/<application-name>).
// It respects <APP_PREFIX>_DIR environment variable.
func (e environment) RunDir() string {
	return e.pathWithEnvOverride(runPathDefaultPrefix, e.applicationName)
}

// VarDir returns the path to the data directory of the application (e.g. /var/lib/<application-name>).
// It respects <APP_PREFIX>_DIR environment variable.
func (e environment) VarDir() string {
	return e.pathWithEnvOverride(varPathDefaultPrefix, e.applicationName)
}

// CacheDir returns the path to the cache directory of the application (e.g. /var/cache/<application-name>).
// It respects <APP_PREFIX>_DIR environment variable.
func (e environment) CacheDir() string {
	return e.pathWithEnvOverride(cachePathDefaultPrefix, e.applicationName)
}

// UsrShareDir returns the path to the static directory of the application (e.g. /usr/share/<application-name>).
// It respects <APP_PREFIX>_DIR environment variable.
func (e environment) UsrShareDir() string {
	return e.pathWithEnvOverride(usrSharePathDefaultPrefix, e.applicationName)
}

// GetUnixSocket returns the full file name of the unix socket.
func (e environment) GetUnixSocket() string {
	path := os.Getenv(e.applicationEnvPrefix + applicationSocketEnvSuffix)
	if path != "" {
		return path
	}

	return filepath.Join(e.RunDir(), "unix.socket")
}

func (e environment) UserConfigDir() (string, error) {
	applicationConfEnvVar := e.applicationEnvPrefix + applicationConfEnvSuffix
	if os.Getenv(applicationConfEnvVar) != "" {
		return os.ExpandEnv(os.Getenv(applicationConfEnvVar)), nil
	}

	configDir, err := os.UserConfigDir()
	if nil == err {
		return filepath.Join(configDir, e.applicationName), nil
	}

	if os.Getenv("HOME") != "" && file.PathExists(os.Getenv("HOME")) {
		return filepath.Join(os.Getenv("HOME"), ".config", e.applicationName), nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}

	if file.PathExists(currentUser.HomeDir) {
		return filepath.Join(currentUser.HomeDir, ".config", e.applicationName), nil
	}

	return "", fmt.Errorf("Failed to determine user config directory")
}

// pathWithEnvOverride returns the directory combined from prefixDir and suffixDir
// where the prefix maybe overridden by a value provided by the prefixDirEnvVar.
func (e environment) pathWithEnvOverride(prefixDir, suffixDir string) string {
	dirEnvVar := e.applicationEnvPrefix + applicationDirEnvSuffix
	prefix := prefixDir
	if os.Getenv(dirEnvVar) != "" {
		return os.Getenv(dirEnvVar)
	}

	return filepath.Join(prefix, suffixDir)
}

const IncusOSSocket = "/run/incus-os/unix.socket"

// IsIncusOS checks if the host system is running IncusOS.
func (e environment) IsIncusOS() bool {
	return file.PathExists("/var/lib/incus-os/")
}

func (e environment) GetToken(ctx context.Context) (string, error) {
	if !e.IsIncusOS() {
		return "", fmt.Errorf("Not an IncusOS system")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/internal/auth/:generate-token", http.NoBody)
	if err != nil {
		return "", fmt.Errorf("Failed to create request to get IncusOS token: %w", err)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Failed to get IncusOS token: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Failed to read request body of IncusOS token response: %w", err)
	}

	response := api.Response{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Failed to fetch IncusOS token: %s: %s", resp.Status, string(body))
		}

		return "", fmt.Errorf("Invalid response for IncusOS token: %w", err)
	}

	if response.Type == api.ErrorResponse {
		return "", api.StatusErrorf(resp.StatusCode, "%v", response.Error)
	}

	return string(response.Metadata), nil
}
