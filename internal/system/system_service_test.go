package system_test

import (
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"

	incustls "github.com/lxc/incus/v6/shared/tls"
	"github.com/maniartech/signals"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestSystemService_UpdateCertificate(t *testing.T) {
	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tests := []struct {
		name     string
		setupEnv func(t *testing.T, targetDir string)
		certPEM  string
		keyPEM   string

		serverCertificateUpdateCallExpected bool
		assertError                         require.ErrorAssertionFunc
	}{
		{
			name: "success",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			serverCertificateUpdateCallExpected: true,
			assertError:                         require.NoError,
		},
		{
			name: "error - invalid certificate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: "invalid-cert",
			keyPEM:  "invalid-key",

			serverCertificateUpdateCallExpected: false,
			assertError: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to validate key pair")
			},
		},
		{
			name: "error - unable to write certificate file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.MkdirAll(filepath.Join(targetDir, "server.crt"), 0o000)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			serverCertificateUpdateCallExpected: false,
			assertError: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.crt")
			},
		},
		{
			name: "error - unable to write certificate key file",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.MkdirAll(filepath.Join(targetDir, "server.key"), 0o000)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			serverCertificateUpdateCallExpected: false,
			assertError: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "server.key")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			env := mockEnv{
				varDir: tmpDir,
			}

			tc.setupEnv(t, env.VarDir())

			serverCertificateUpdateResp := make(chan struct{}, 1)

			serverCertificateUpdate := signals.NewSync[tls.Certificate]()
			serverCertificateUpdate.AddListener(func(ctx context.Context, cert tls.Certificate) {
				serverCertificateUpdateResp <- struct{}{}
			})

			systemSvc := system.NewSystemService(env, serverCertificateUpdate)

			err = systemSvc.UpdateCertificate(context.Background(), tc.certPEM, tc.keyPEM)
			tc.assertError(t, err)

			serverCertificateUpdateCalled := false
			select {
			case <-serverCertificateUpdateResp:
				serverCertificateUpdateCalled = true
			case <-time.After(10 * time.Millisecond):
			}

			require.Equal(t, tc.serverCertificateUpdateCallExpected, serverCertificateUpdateCalled)
		})
	}
}

func TestSystemService_UpdateNetworkConfig(t *testing.T) {
	tests := []struct {
		name           string
		securityConfig api.SystemNetwork

		assertErr         require.ErrorAssertionFunc
		wantNetworkConfig api.SystemNetwork
	}{
		{
			name: "success",
			securityConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://somesource:443",
				},
			},

			assertErr: require.NoError,
			wantNetworkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: "https://somesource:443",
				},
			},
		},
		{
			name: "error",
			securityConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					OperationsCenterAddress: ":|\\", // invalid
				},
			},

			assertErr: require.Error,
			wantNetworkConfig: api.SystemNetwork{
				SystemNetworkPut: api.SystemNetworkPut{
					// From default.yml
					OperationsCenterAddress: "https://127.0.0.1:443",
					RestServerPort:          7443,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t)
			systemSvc := system.NewSystemService(nil, nil)

			// Run test
			err := systemSvc.UpdateNetworkConfig(t.Context(), tc.securityConfig.SystemNetworkPut)
			gotNetworkConfig := systemSvc.GetNetworkConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantNetworkConfig, gotNetworkConfig)
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
				},
			}

			config.InitTest(t)
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
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			config.InitTest(t)
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

func TestSystemService_UpdateUpdatesConfig(t *testing.T) {
	tests := []struct {
		name           string
		securityConfig api.SystemUpdates

		assertErr         require.ErrorAssertionFunc
		wantUpdatesConfig api.SystemUpdates
	}{
		{
			name: "success",
			securityConfig: api.SystemUpdates{
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
			securityConfig: api.SystemUpdates{
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
			config.InitTest(t)
			systemSvc := system.NewSystemService(nil, nil)

			// Run test
			err := systemSvc.UpdateUpdatesConfig(t.Context(), tc.securityConfig.SystemUpdatesPut)
			gotUpdatesConfig := systemSvc.GetUpdatesConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantUpdatesConfig.Source, gotUpdatesConfig.Source)
		})
	}
}

type mockEnv struct {
	varDir string
}

func (e mockEnv) VarDir() string { return e.varDir }
