package system_test

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/system/mock"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/internal/testing/queue"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSystemService_UpdateCertificate(t *testing.T) {
	currentCertPEM, currentKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	currentCertFingerprint, err := incustls.CertFingerprintStr(string(currentCertPEM))
	require.NoError(t, err)

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	certFingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	tests := []struct {
		name                       string
		setupEnv                   func(t *testing.T, targetDir string)
		certPEM                    string
		keyPEM                     string
		serverGetAll               provisioning.Servers
		serverGetAllErr            error
		serverGetSystemProvider    []queue.Item[provisioning.ServerSystemProvider]
		serverUpdateSystemProvider []queue.Item[struct{}]

		assertErr                       require.ErrorAssertionFunc
		wantServerCertificateUpdateEmit []queue.Item[string]
	}{
		{
			name: "success - no registered servers",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: require.NoError,
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
		},
		{
			name: "success - no registered servers - same certificate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(currentCertPEM),
			keyPEM:  string(currentKeyPEM),

			assertErr:                       require.NoError,
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "success - with registered servers",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name: "operations-center",
					Type: api.ServerTypeOperationsCenter,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
		},
		{
			name: "error - invalid certificate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: "invalid-cert",
			keyPEM:  "invalid-key",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to validate key pair")
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "error - unable to read certificate file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.RemoveAll(filepath.Join(targetDir, "server.crt"))
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.crt")
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "error - unable to read certificate key file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.RemoveAll(filepath.Join(targetDir, "server.key"))
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.key")
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "error - invalid current certificate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()

				err := os.WriteFile(filepath.Join(targetDir, "server.key"), keyPEM, 0o600)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to validate current key pair")
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "error - unable to read certificate file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.Chmod(filepath.Join(targetDir, "server.crt"), 0o400)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.crt")
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "error - unable to read certificate key file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.Chmod(filepath.Join(targetDir, "server.key"), 0o400)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.key")
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
		},
		{
			name: "error - with registered servers - repo.GetAll",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:         string(certPEM),
			keyPEM:          string(keyPEM),
			serverGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
				{
					Value: currentCertFingerprint,
				},
			},
		},
		{
			name: "error - with registered servers - server.GetSystemProvider",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
				{
					Value: currentCertFingerprint,
				},
			},
		},
		{
			name: "error - with registered servers - server.UpdateSystemProvider",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
				{
					Value: currentCertFingerprint,
				},
			},
		},
		{
			name: "error - with registered servers - revert ok",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name: "two",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
							},
						},
					},
				},
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
				{
					Err: boom.Error,
				},
				{},
			},

			assertErr: boom.ErrorIs,
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
				{
					Value: currentCertFingerprint,
				},
			},
		},
		{
			name: "error - with registered servers - revert error",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name: "two",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
							},
						},
					},
				},
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
				{
					Err: boom.Error,
				},
				{
					Err: boom.Error,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				boom.ErrorIs(tt, err)
				require.ErrorContains(tt, err, `Failed to revert provider config of "one"`)
			},
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
				{
					Value: currentCertFingerprint,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()

			err := os.WriteFile(filepath.Join(tmpDir, "server.crt"), currentCertPEM, 0o600)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(tmpDir, "server.key"), currentKeyPEM, 0o600)
			require.NoError(t, err)

			env := &envMock.EnvironmentMock{
				VarDirFunc: func() string {
					return tmpDir
				},
			}

			tc.setupEnv(t, env.VarDir())

			listenerID := uuid.New()
			config.ServerCertificateUpdateSignal.AddListener(func(ctx context.Context, cert tls.Certificate) {
				wantCertificateFingerprint, _ := queue.Pop(t, &tc.wantServerCertificateUpdateEmit)

				certFingerprint := incustls.CertFingerprint(cert.Leaf)
				require.Equal(t, wantCertificateFingerprint, certFingerprint)
			}, listenerID.String())
			t.Cleanup(func() {
				config.ServerCertificateUpdateSignal.RemoveListener(listenerID.String())
			})

			serverSvc := &mock.ProvisioningServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.serverGetAll, tc.serverGetAllErr
				},
				GetSystemProviderFunc: func(ctx context.Context, name string) (provisioning.ServerSystemProvider, error) {
					return queue.Pop(t, &tc.serverGetSystemProvider)
				},
				UpdateSystemProviderFunc: func(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error {
					require.Equal(t, string(certPEM), providerConfig.Config.Config["server_certificate"])
					_, err := queue.Pop(t, &tc.serverUpdateSystemProvider)
					return err
				},
			}

			systemSvc := system.NewSystemService(env, serverSvc)

			// Run test
			err = systemSvc.UpdateCertificate(context.Background(), tc.certPEM, tc.keyPEM)

			// Assert
			tc.assertErr(t, err)

			require.Empty(t, tc.serverGetSystemProvider)
			require.Empty(t, tc.serverUpdateSystemProvider)
			require.Empty(t, tc.wantServerCertificateUpdateEmit)
		})
	}
}

func TestSystemService_TriggerCertificateRenew(t *testing.T) {
	currentCertPEM, currentKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tests := []struct {
		name                     string
		certPEM                  string
		keyPEM                   string
		acmeUpdateCertificate    *api.SystemCertificatePost
		acmeUpdateCertificateErr error
		serverGetAllErr          error

		assertErr   require.ErrorAssertionFunc
		wantChanged bool
	}{
		{
			name:    "success - no new certificate",
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: require.NoError,
		},
		{
			name:    "success - new certificate",
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			acmeUpdateCertificate: &api.SystemCertificatePost{
				Certificate: string(certPEM),
				Key:         string(keyPEM),
			},

			assertErr:   require.NoError,
			wantChanged: true,
		},
		{
			name:                     "error - acmeUpdateCertificate",
			certPEM:                  string(certPEM),
			keyPEM:                   string(keyPEM),
			acmeUpdateCertificateErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:    "error - update certificate",
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			acmeUpdateCertificate: &api.SystemCertificatePost{
				Certificate: string(certPEM),
				Key:         string(keyPEM),
			},
			serverGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()

			err := os.WriteFile(filepath.Join(tmpDir, "server.crt"), currentCertPEM, 0o600)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(tmpDir, "server.key"), currentKeyPEM, 0o600)
			require.NoError(t, err)

			env := &envMock.EnvironmentMock{
				VarDirFunc: func() string {
					return tmpDir
				},
				CacheDirFunc: func() string {
					return tmpDir
				},
			}

			serverSvc := &mock.ProvisioningServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return nil, tc.serverGetAllErr
				},
			}

			systemSvc := system.NewSystemService(
				env,
				serverSvc,
				system.WithACMEUpdateCertificateFunc(
					func(
						ctx context.Context,
						fsEnv interface {
							VarDir() string
							CacheDir() string
						},
						cfg api.SystemSecurityACME,
						force bool,
					) (*api.SystemCertificatePost, error) {
						return tc.acmeUpdateCertificate, tc.acmeUpdateCertificateErr
					},
				),
			)

			// Run test
			changed, err := systemSvc.TriggerCertificateRenew(t.Context(), false)

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantChanged, changed)
		})
	}
}

func TestSystemService_UpdateNetworkConfig(t *testing.T) {
	tests := []struct {
		name                       string
		networkConfig              api.SystemNetwork
		serverGetAll               provisioning.Servers
		serverGetAllErr            error
		serverGetSystemProvider    []queue.Item[provisioning.ServerSystemProvider]
		serverUpdateSystemProvider []queue.Item[struct{}]
		configSaveErr              error

		assertErr         require.ErrorAssertionFunc
		wantNetworkConfig api.SystemNetwork
	}{
		{
			name: "success - empty",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{},
			},

			assertErr: require.NoError,
			wantNetworkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{},
			},
		},
		{
			name: "success - OperationsCenterAddress change",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name: "operations-center",
					Type: api.ServerTypeOperationsCenter,
				},
			},
			serverGetSystemProvider: []queue.Item[incusosapi.SystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_url": "https://one:8443/",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			wantNetworkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
		},
		{
			name: "success - OperationsCenterAddress change - system provider config not initialized",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[incusosapi.SystemProvider]{
				{
					Value: incusosapi.SystemProvider{},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			wantNetworkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
		},
		{
			name: "error - NetworkSetDefaults",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					RestServerAddress: ":::", // invalid
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - validation",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: ":|\\", // invalid
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - config.UpdateNetwork",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{},
			},
			configSaveErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - repo.GetAll",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - server.GetSystemProvider",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - server.UpdateSystemProvider first",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[incusosapi.SystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_url": "https://one:8443/",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - server.UpdateSystemProvider second - revert ok",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name: "two",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[incusosapi.SystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_url": "https://one:8443/",
							},
						},
					},
				},
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_url": "https://one:8443/",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
				{
					Err: boom.Error,
				},
				{},
			},

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - server.UpdateSystemProvider second - revert error",
			networkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://new:8443/",
					RestServerAddress:       "192.168.1.200:8443",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name: "two",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[incusosapi.SystemProvider]{
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_url": "https://one:8443/",
							},
						},
					},
				},
				{
					Value: incusosapi.SystemProvider{
						Config: incusosapi.SystemProviderConfig{
							Config: map[string]string{
								"server_url": "https://one:8443/",
							},
						},
					},
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
				{
					Err: boom.Error,
				},
				{
					Err: boom.Error,
				},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				boom.ErrorIs(tt, err)
				require.ErrorContains(tt, err, `Failed to revert provider config of "one"`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			env := &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			serverSvc := &mock.ProvisioningServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.serverGetAll, tc.serverGetAllErr
				},
				GetSystemProviderFunc: func(ctx context.Context, name string) (provisioning.ServerSystemProvider, error) {
					return queue.Pop(t, &tc.serverGetSystemProvider)
				},
				UpdateSystemProviderFunc: func(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error {
					require.Equal(t, "https://new:8443/", providerConfig.Config.Config["server_url"])
					_, err := queue.Pop(t, &tc.serverUpdateSystemProvider)
					return err
				},
			}

			config.InitTest(t, env, tc.configSaveErr)
			// config.UpdateNetwork(t.Context(), tc.networkConfig)
			systemSvc := system.NewSystemService(nil, serverSvc)

			// Run test
			err := systemSvc.UpdateNetworkConfig(t.Context(), tc.networkConfig.SystemNetworkPut)
			gotNetworkConfig := systemSvc.GetNetworkConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantNetworkConfig, gotNetworkConfig)
			require.Empty(t, tc.serverGetSystemProvider)
			require.Empty(t, tc.serverUpdateSystemProvider)
		})
	}
}

func TestSystemService_GetNetworkConfig(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			networkConfig := api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://someaddress:1234",
					RestServerAddress:       "[::1]:1234",
				},
			}

			env := &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			err := config.UpdateNetwork(t.Context(), networkConfig.SystemNetworkPut)
			require.NoError(t, err)

			systemSvc := system.NewSystemService(nil, nil)

			// Run test
			gotNetworkConfig := systemSvc.GetNetworkConfig(t.Context())

			// Assert
			require.Equal(t, networkConfig, gotNetworkConfig)
		})
	}
}

func TestSystemService_UpdateSecurityConfig(t *testing.T) {
	tests := []struct {
		name           string
		securityConfig api.SystemSecurity

		assertErr          require.ErrorAssertionFunc
		wantSecurityConfig api.SystemSecurity
	}{
		{
			name: "success",
			securityConfig: api.SystemSecurity{
				SystemSecurityPut: api.SystemSecurityPut{
					TrustedTLSClientCertFingerprints: []string{"foobar"},
				},
			},

			assertErr: require.NoError,
			wantSecurityConfig: api.SystemSecurity{
				SystemSecurityPut: api.SystemSecurityPut{
					TrustedTLSClientCertFingerprints: []string{"foobar"},
				},
			},
		},
		{
			name: "error",
			securityConfig: api.SystemSecurity{
				SystemSecurityPut: api.SystemSecurityPut{
					OIDC: api.SystemSecurityOIDC{
						Issuer: ":|\\", // invalid
					},
				},
			},

			assertErr: require.Error,
			wantSecurityConfig: api.SystemSecurity{
				SystemSecurityPut: api.SystemSecurityPut{
					TrustedTLSClientCertFingerprints: []string{},
					ACME: api.SystemSecurityACME{
						CAURL:               "https://acme-v02.api.letsencrypt.org/directory",
						Challenge:           "HTTP-01",
						Address:             ":80",
						ProviderEnvironment: []string{},
						ProviderResolvers:   []string{},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			env := &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			systemSvc := system.NewSystemService(nil, nil)

			// Run test
			err := systemSvc.UpdateSecurityConfig(t.Context(), tc.securityConfig.SystemSecurityPut)
			gotSecurityConfig := systemSvc.GetSecurityConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantSecurityConfig, gotSecurityConfig)
		})
	}
}

func TestSystemService_UpdateSettingsConfig(t *testing.T) {
	tests := []struct {
		name           string
		securityConfig api.SystemSettings

		assertErr          require.ErrorAssertionFunc
		wantSettingsConfig api.SystemSettings
	}{
		{
			name: "success",
			securityConfig: api.SystemSettings{
				SystemSettingsPut: api.SystemSettingsPut{
					LogLevel: "INFO",
				},
			},

			assertErr: require.NoError,
			wantSettingsConfig: api.SystemSettings{
				SystemSettingsPut: api.SystemSettingsPut{
					LogLevel: "INFO",
				},
			},
		},
		{
			name: "error",
			securityConfig: api.SystemSettings{
				SystemSettingsPut: api.SystemSettingsPut{
					LogLevel: "invalid", // invalid log level
				},
			},

			assertErr: require.Error,
			wantSettingsConfig: api.SystemSettings{
				SystemSettingsPut: api.SystemSettingsPut{
					LogLevel: "",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			env := &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			systemSvc := system.NewSystemService(nil, nil)

			// Run test
			err := systemSvc.UpdateSettingsConfig(t.Context(), tc.securityConfig.SystemSettingsPut)
			gotSettingsConfig := systemSvc.GetSettingsConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantSettingsConfig, gotSettingsConfig)
		})
	}
}

func TestSystemService_UpdateUpdatesConfig(t *testing.T) {
	tests := []struct {
		name          string
		updatesConfig api.SystemUpdates

		assertErr         require.ErrorAssertionFunc
		wantUpdatesConfig api.SystemUpdates
	}{
		{
			name: "success",
			updatesConfig: api.SystemUpdates{
				SystemUpdatesPut: api.SystemUpdatesPut{
					Source: "https://somesource:443",
					SignatureVerificationRootCA: `-----BEGIN CERTIFICATE-----
MIIBxTCCAWugAwIBAgIUKFh7jSFs4OIymJR60kMDizaaUu0wCgYIKoZIzj0EAwMw
ODEbMBkGA1UEAwwSSW5jdXMgT1MgLSBSb290IEUxMRkwFwYDVQQKDBBMaW51eCBD
b250YWluZXJzMB4XDTI1MDYyNjA4MTA1NFoXDTQ1MDYyMTA4MTA1NFowODEbMBkG
A1UEAwwSSW5jdXMgT1MgLSBSb290IEUxMRkwFwYDVQQKDBBMaW51eCBDb250YWlu
ZXJzMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEkuL+o9TxVlcmn7rQjSQUPtVW
YhISgnMOWIMbg4sh0hWh5LJeH7mPA41I80TAR84O+rcnj/AtFG+O2dZgTK47UaNT
MFEwHQYDVR0OBBYEFERR7s37UYWIfjdauwuftLTUULcaMB8GA1UdIwQYMBaAFERR
7s37UYWIfjdauwuftLTUULcaMA8GA1UdEwEB/wQFMAMBAf8wCgYIKoZIzj0EAwMD
SAAwRQIhAId625vznH0/C9E/gLLRz5S95x3mZmqIHOQBFHRf2mLyAiB2kMK4Idcn
dzfuFuN/tMIqY355bBYk3m6/UAIK5Pum/Q==
-----END CERTIFICATE-----`,
				},
			},

			assertErr: require.NoError,
			wantUpdatesConfig: api.SystemUpdates{
				SystemUpdatesPut: api.SystemUpdatesPut{
					Source: "https://somesource:443",
				},
			},
		},
		{
			name: "error",
			updatesConfig: api.SystemUpdates{
				SystemUpdatesPut: api.SystemUpdatesPut{
					Source: ":|\\", // invalid
				},
			},

			assertErr: require.Error,
			wantUpdatesConfig: api.SystemUpdates{
				SystemUpdatesPut: api.SystemUpdatesPut{
					// From default.yml
					Source: "https://images.linuxcontainers.org/os/",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			env := &envMock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			systemSvc := system.NewSystemService(nil, nil)

			// Run test
			err := systemSvc.UpdateUpdatesConfig(t.Context(), tc.updatesConfig.SystemUpdatesPut)
			gotUpdatesConfig := systemSvc.GetUpdatesConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantUpdatesConfig.Source, gotUpdatesConfig.Source)
		})
	}
}
