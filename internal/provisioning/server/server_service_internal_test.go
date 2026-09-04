package server

import (
	"context"
	"crypto/tls"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	adapterMock "github.com/FuturFusion/operations-center/internal/provisioning/adapter/mock"
	svcMock "github.com/FuturFusion/operations-center/internal/provisioning/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/provisioning/repo/mock"
	"github.com/FuturFusion/operations-center/shared/api"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_availableVersionGreaterThan(t *testing.T) {
	tests := []struct {
		name             string
		currentVersion   string
		availableVersion string

		want bool
	}{
		{
			name:             "available version greater",
			currentVersion:   "202601172317",
			availableVersion: "202601210123",

			want: true,
		},
		{
			name:             "available version equal",
			currentVersion:   "202601210123",
			availableVersion: "202601210123",

			want: false,
		},
		{
			name:             "available version smaller",
			currentVersion:   "202601210123",
			availableVersion: "202601172317",

			want: false,
		},
		{
			name:             "current invalid",
			currentVersion:   "invalid",
			availableVersion: "202601210123",

			want: true,
		},
		{
			name:             "available invalid",
			currentVersion:   "202601210123",
			availableVersion: "invalid",

			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := availableVersionGreaterThan(tc.currentVersion, tc.availableVersion)

			require.Equal(t, tc.want, got)
		})
	}
}

func Test_mediaURLWarnings(t *testing.T) {
	tests := []struct {
		name     string
		mediaURL string

		want []string
	}{
		{
			name:     "plain host name",
			mediaURL: "https://oc.example.com:8443/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: nil,
		},
		{
			name:     "IPv4 address with a non-default port",
			mediaURL: "https://192.0.2.10:8443/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: nil,
		},
		{
			name:     "IPv6 address on the default port",
			mediaURL: "https://[2001:db8::1]/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: nil,
		},
		{
			name:     "IPv6 address with a non-default port",
			mediaURL: "https://[2001:db8::1]:8443/1.0/provisioning/tokens/x/seeds/y/architecture/x86_64/type/iso/file.iso",

			want: []string{"it combines an IPv6 address with a non-default port, which BMC firmware is known to parse incorrectly"},
		},
		{
			name:     "overly long URL",
			mediaURL: "https://oc.example.com:8443/1.0/provisioning/tokens/00000000-0000-0000-0000-000000000000/seeds/" + strings.Repeat("s", 200) + "/architecture/x86_64/type/iso/file.iso",

			want: []string{"it is longer than the 255 characters some BMCs accept"},
		},
		{
			name:     "both",
			mediaURL: "https://[2001:db8::1]:8443/1.0/provisioning/tokens/00000000-0000-0000-0000-000000000000/seeds/" + strings.Repeat("s", 200) + "/architecture/x86_64/type/iso/file.iso",

			want: []string{
				"it combines an IPv6 address with a non-default port, which BMC firmware is known to parse incorrectly",
				"it is longer than the 255 characters some BMCs accept",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mediaURL, err := url.Parse(tc.mediaURL)
			require.NoError(t, err)

			require.Equal(t, tc.want, mediaURLWarnings(mediaURL))
		})
	}
}

func TestServerService_bmcTaskMonitorHandover(t *testing.T) {
	fixedDate := time.Date(2025, 3, 12, 10, 57, 43, 0, time.UTC)

	taskMonitor := &provisioning.BMCTaskMonitor{
		URI: "https://bmc.local/task/1",
	}

	setup := func(t *testing.T) *serverService {
		t.Helper()

		config.InitTest(t, &envMock.EnvironmentMock{
			IsIncusOSFunc: func() bool {
				return false
			},
		}, nil)

		err := config.UpdateNetwork(t.Context(), system.NetworkPut{
			OperationsCenterAddress: "https://192.168.1.200:8443",
			RestServerAddress:       "[::]:8443",
		})
		require.NoError(t, err)

		repo := &repoMock.ServerRepoMock{
			GetByNameFunc: func(ctx context.Context, name string) (*provisioning.Server, error) {
				return &provisioning.Server{
					Name: "one",
					BMCConfig: api.BMCConfig{
						APIType: api.BMCAPITypeRedfishV1Generic,
					},
				}, nil
			},
		}

		tokenSvc := &svcMock.TokenServiceMock{}

		bmcClient := &adapterMock.BMCServerClientPortMock{
			ServerPowerOnFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
				return taskMonitor, nil
			},
			ServerPowerOffFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
				return taskMonitor, nil
			},
			ServerRestartFunc: func(ctx context.Context, server provisioning.Server, force bool) (*provisioning.BMCTaskMonitor, error) {
				return taskMonitor, nil
			},
			ApplyBIOSAttributesFunc: func(ctx context.Context, server provisioning.Server, attributes map[string]any) (*provisioning.BMCTaskMonitor, error) {
				return taskMonitor, nil
			},
			DetachMediaFunc: func(ctx context.Context, server provisioning.Server, virtualMediaID string) (*provisioning.BMCTaskMonitor, error) {
				return taskMonitor, nil
			},
		}

		return New(
			repo, nil, nil, tokenSvc, nil, nil, nil, tls.Certificate{},
			WithNow(func() time.Time { return fixedDate }),
			AddBMCServerClient(api.BMCAPITypeRedfishV1Generic, bmcClient),
		)
	}

	tests := []struct {
		name string
		call func(t *testing.T, serverSvc *serverService) (*provisioning.BMCTaskMonitor, error)
	}{
		{
			name: "bmcServerPowerOnByName",
			call: func(t *testing.T, serverSvc *serverService) (*provisioning.BMCTaskMonitor, error) {
				t.Helper()
				return serverSvc.bmcServerPowerOnByName(t.Context(), "one", false, false)
			},
		},
		{
			name: "bmcServerPowerOffByName",
			call: func(t *testing.T, serverSvc *serverService) (*provisioning.BMCTaskMonitor, error) {
				t.Helper()
				return serverSvc.bmcServerPowerOffByName(t.Context(), "one", false, false)
			},
		},
		{
			name: "bmcServerRestartByName",
			call: func(t *testing.T, serverSvc *serverService) (*provisioning.BMCTaskMonitor, error) {
				t.Helper()
				return serverSvc.bmcServerRestartByName(t.Context(), "one", false, false)
			},
		},
		{
			name: "applyBIOSAttributesByName",
			call: func(t *testing.T, serverSvc *serverService) (*provisioning.BMCTaskMonitor, error) {
				t.Helper()
				return serverSvc.applyBIOSAttributesByName(t.Context(), "one", map[string]any{"NumaNodesPerSocket": 4}, false)
			},
		},
		{
			name: "bmcDetachMediaByName",
			call: func(t *testing.T, serverSvc *serverService) (*provisioning.BMCTaskMonitor, error) {
				t.Helper()
				return serverSvc.bmcDetachMediaByName(t.Context(), "one", "system:1", false)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTaskMonitor, err := tc.call(t, setup(t))

			require.NoError(t, err)
			require.Same(t, taskMonitor, gotTaskMonitor)
		})
	}
}
