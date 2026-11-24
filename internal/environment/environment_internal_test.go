package environment

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/errassert"
)

type httpClientFunc func(*http.Request) (*http.Response, error)

func (f httpClientFunc) Do(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestEnvironment_GetSecureBootPlatformKeyUpdate(t *testing.T) {
	t.Parallel()

	update := []byte("a signed platform key update")

	tests := []struct {
		name string

		isIncusOS bool
		content   []byte
		absent    bool

		want      []byte
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			isIncusOS: true,
			content:   update,

			want:      update,
			assertErr: require.NoError,
		},
		{
			name: "error - not an IncusOS system",

			isIncusOS: false,
			content:   update,

			assertErr: errassert.Contains("Not an IncusOS system"),
		},
		{
			name: "error - the update is not on the EFI system partition",

			isIncusOS: true,
			absent:    true,

			assertErr: errassert.Contains("Failed to read the IncusOS secure boot platform key update"),
		},
		{
			// An empty file enrolls nothing, so it is reported rather than being
			// built into a media, that leaves the server in setup mode.
			name: "error - the update is empty",

			isIncusOS: true,
			content:   []byte{},

			assertErr: errassert.Contains("is empty"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "PK.auth")

			if !tc.absent {
				require.NoError(t, os.WriteFile(path, tc.content, 0o600))
			}

			env := environment{
				platformKeyUpdatePath: path,
				isIncusOS:             func() bool { return tc.isIncusOS },
			}

			got, err := env.GetSecureBootPlatformKeyUpdate(t.Context())

			tc.assertErr(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestEnvironment_GetToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		isIncusOS bool
		status    int
		body      io.Reader
		doErr     error

		wantRequest string
		want        string
		assertErr   require.ErrorAssertionFunc
	}{
		{
			name:      "success",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader(`{"type": "sync", "metadata": "token"}`),
			doErr:     nil,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "token",
			assertErr:   require.NoError,
		},
		{
			name:      "error - not an IncusOS system",
			isIncusOS: false,
			status:    http.StatusOK,
			body:      strings.NewReader(""),
			doErr:     nil,

			wantRequest: "",
			want:        "",
			assertErr:   errassert.Contains("Not an IncusOS system"),
		},
		{
			name:      "error - request fails",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader(""),
			doErr:     boom.Error,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "",
			assertErr:   boom.ErrorIs,
		},
		{
			name:      "error - body read fails",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      iotest.ErrReader(boom.Error),
			doErr:     nil,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "",
			assertErr:   boom.ErrorIs,
		},
		{
			name:      "error - status not OK and body not JSON",
			isIncusOS: true,
			status:    http.StatusInternalServerError,
			body:      strings.NewReader("internal error"),
			doErr:     nil,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "",
			assertErr:   errassert.Contains("Failed to fetch IncusOS token: 500 Internal Server Error: internal error"),
		},
		{
			name:      "error - status OK and body not JSON",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader("not JSON"),
			doErr:     nil,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "",
			assertErr:   errassert.Contains("Invalid response for IncusOS token"),
		},
		{
			name:      "error - error response",
			isIncusOS: true,
			status:    http.StatusForbidden,
			body:      strings.NewReader(`{"type": "error", "error": "not allowed"}`),
			doErr:     nil,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "",
			assertErr:   errassert.StatusErrorContains(http.StatusForbidden, "not allowed"),
		},
		{
			name:      "error - metadata not a string",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader(`{"type": "sync", "metadata": 42}`),
			doErr:     nil,

			wantRequest: "POST http://unix/internal/auth/:generate-token",
			want:        "",
			assertErr:   errassert.Contains("Invalid response for IncusOS token"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			gotRequest := ""
			env := environment{
				isIncusOS: func() bool { return tc.isIncusOS },
				httpClient: httpClientFunc(func(r *http.Request) (*http.Response, error) {
					gotRequest = r.Method + " " + r.URL.String()
					return &http.Response{StatusCode: tc.status, Status: fmt.Sprintf("%d %s", tc.status, http.StatusText(tc.status)), Body: io.NopCloser(tc.body)}, tc.doErr
				}),
			}

			// Run test
			got, err := env.GetToken(t.Context())

			// Assert
			tc.assertErr(t, err, "error of GetToken")
			require.Equal(t, tc.want, got, "token")
			require.Equal(t, tc.wantRequest, gotRequest, "request sent to IncusOS")
		})
	}
}

func TestEnvironment_GetSecureBootCertificates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		isIncusOS bool
		status    int
		body      io.Reader
		doErr     error

		wantRequest string
		want        incusosapi.InternalSecureBootCertificates
		assertErr   require.ErrorAssertionFunc
	}{
		{
			name:      "success",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader(`{"type": "sync", "metadata": {"pk": "pk", "kek": ["kek"], "db": ["db"], "dbx": ["dbx"]}}`),
			doErr:     nil,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{PK: "pk", KEK: []string{"kek"}, DB: []string{"db"}, DBX: []string{"dbx"}},
			assertErr:   require.NoError,
		},
		{
			name:      "error - not an IncusOS system",
			isIncusOS: false,
			status:    http.StatusOK,
			body:      strings.NewReader(""),
			doErr:     nil,

			wantRequest: "",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   errassert.Contains("Not an IncusOS system"),
		},
		{
			name:      "error - request fails",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader(""),
			doErr:     boom.Error,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   boom.ErrorIs,
		},
		{
			name:      "error - body read fails",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      iotest.ErrReader(boom.Error),
			doErr:     nil,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   boom.ErrorIs,
		},
		{
			name:      "error - status not OK and body not JSON",
			isIncusOS: true,
			status:    http.StatusInternalServerError,
			body:      strings.NewReader("internal error"),
			doErr:     nil,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   errassert.Contains("Failed to fetch IncusOS secure boot certificates: 500 Internal Server Error: internal error"),
		},
		{
			name:      "error - status OK and body not JSON",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader("not JSON"),
			doErr:     nil,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   errassert.Contains("Invalid response for IncusOS secure boot certificates"),
		},
		{
			name:      "error - error response",
			isIncusOS: true,
			status:    http.StatusForbidden,
			body:      strings.NewReader(`{"type": "error", "error": "not allowed"}`),
			doErr:     nil,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   errassert.StatusErrorContains(http.StatusForbidden, "not allowed"),
		},
		{
			name:      "error - metadata not an object",
			isIncusOS: true,
			status:    http.StatusOK,
			body:      strings.NewReader(`{"type": "sync", "metadata": 42}`),
			doErr:     nil,

			wantRequest: "GET http://unix/internal/secureboot",
			want:        incusosapi.InternalSecureBootCertificates{},
			assertErr:   errassert.Contains("Invalid response for IncusOS secure boot certificates"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			gotRequest := ""
			env := environment{
				isIncusOS: func() bool { return tc.isIncusOS },
				httpClient: httpClientFunc(func(r *http.Request) (*http.Response, error) {
					gotRequest = r.Method + " " + r.URL.String()
					return &http.Response{StatusCode: tc.status, Status: fmt.Sprintf("%d %s", tc.status, http.StatusText(tc.status)), Body: io.NopCloser(tc.body)}, tc.doErr
				}),
			}

			// Run test
			got, err := env.GetSecureBootCertificates(t.Context())

			// Assert
			tc.assertErr(t, err, "error of GetSecureBootCertificates")
			require.Equal(t, tc.want, got, "secure boot certificates")
			require.Equal(t, tc.wantRequest, gotRequest, "request sent to IncusOS")
		})
	}
}

// Not parallel: the test changes environment variables.
func TestEnvironment_UserConfigDir(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err, "get current user")

	tests := []struct {
		name          string
		confEnv       string
		xdgConfigHome string
		home          string

		want      string
		assertErr require.ErrorAssertionFunc
	}{
		{
			name:          "config dir from environment variable with expansion",
			confEnv:       "$HOME/custom",
			xdgConfigHome: "/xdg",
			home:          "/home/test",

			want:      "/home/test/custom",
			assertErr: require.NoError,
		},
		{
			name:          "user config dir of the OS",
			confEnv:       "",
			xdgConfigHome: "/xdg",
			home:          "/home/test",

			want:      "/xdg/app",
			assertErr: require.NoError,
		},
		{
			name:          "relative XDG config home falls back to HOME",
			confEnv:       "",
			xdgConfigHome: "relative",
			home:          "/",

			want:      "/.config/app",
			assertErr: require.NoError,
		},
		{
			name:          "missing HOME falls back to home dir of the current user",
			confEnv:       "",
			xdgConfigHome: "relative",
			home:          "/nonexistent-home",

			want:      filepath.Join(currentUser.HomeDir, ".config", "app"),
			assertErr: require.NoError,
		},
		{
			name:          "empty HOME falls back to home dir of the current user",
			confEnv:       "",
			xdgConfigHome: "",
			home:          "",

			want:      filepath.Join(currentUser.HomeDir, ".config", "app"),
			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			t.Setenv("APP_CONF", tc.confEnv)
			t.Setenv("XDG_CONFIG_HOME", tc.xdgConfigHome)
			t.Setenv("HOME", tc.home)

			env := environment{applicationName: "app", applicationEnvPrefix: "APP"}

			// Run test
			got, err := env.UserConfigDir()

			// Assert
			tc.assertErr(t, err, "error of UserConfigDir")
			require.Equal(t, tc.want, got, "user config dir")
		})
	}
}

// Not parallel: the test changes environment variables.
func TestEnvironment_directories(t *testing.T) {
	tests := []struct {
		name      string
		dirEnv    string
		socketEnv string

		wantLogDir      string
		wantRunDir      string
		wantVarDir      string
		wantCacheDir    string
		wantUsrShareDir string
		wantUnixSocket  string
	}{
		{
			name:      "defaults",
			dirEnv:    "",
			socketEnv: "",

			wantLogDir:      "/var/log",
			wantRunDir:      "/run/app",
			wantVarDir:      "/var/lib/app",
			wantCacheDir:    "/var/cache/app",
			wantUsrShareDir: "/usr/share/app",
			wantUnixSocket:  "/run/app/unix.socket",
		},
		{
			name:      "directory override",
			dirEnv:    "/override",
			socketEnv: "",

			wantLogDir:      "/override",
			wantRunDir:      "/override",
			wantVarDir:      "/override",
			wantCacheDir:    "/override",
			wantUsrShareDir: "/override",
			wantUnixSocket:  "/override/unix.socket",
		},
		{
			name:      "socket override",
			dirEnv:    "",
			socketEnv: "/custom/app.socket",

			wantLogDir:      "/var/log",
			wantRunDir:      "/run/app",
			wantVarDir:      "/var/lib/app",
			wantCacheDir:    "/var/cache/app",
			wantUsrShareDir: "/usr/share/app",
			wantUnixSocket:  "/custom/app.socket",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			t.Setenv("APP_DIR", tc.dirEnv)
			t.Setenv("APP_SOCKET", tc.socketEnv)

			env := New("app", "APP")

			// Run test
			logDir := env.LogDir()
			runDir := env.RunDir()
			varDir := env.VarDir()
			cacheDir := env.CacheDir()
			usrShareDir := env.UsrShareDir()
			unixSocket := env.GetUnixSocket()

			// Assert
			require.Equal(t, tc.wantLogDir, logDir, "log dir")
			require.Equal(t, tc.wantRunDir, runDir, "run dir")
			require.Equal(t, tc.wantVarDir, varDir, "var dir")
			require.Equal(t, tc.wantCacheDir, cacheDir, "cache dir")
			require.Equal(t, tc.wantUsrShareDir, usrShareDir, "usr share dir")
			require.Equal(t, tc.wantUnixSocket, unixSocket, "unix socket")
		})
	}
}
