package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

// testConfig has the shape of the configuration before the split, so validation cases can cover both parts at once.
type testConfig struct {
	Network  testNetwork
	Security system.Security
	Settings testSettings
	Updates  system.Updates
}

type testNetwork struct {
	NetworkPut testNetworkPut
}

type testNetworkPut struct {
	RestServerAddress       string
	OperationsCenterAddress string
}

type testSettings struct {
	SettingsPut testSettingsPut
}

type testSettingsPut struct {
	LogLevel          string
	LogLevels         map[string]string
	MetricsEnabled    bool
	MetricsComponents []string
}

func (c testConfig) split() (localConfig, system.SharedConfig) {
	local := localConfig{
		Network: system.Network{NetworkPut: system.NetworkPut{RestServerAddress: c.Network.NetworkPut.RestServerAddress}},
		Settings: system.Settings{SettingsPut: system.SettingsPut{
			LogLevel:          c.Settings.SettingsPut.LogLevel,
			LogLevels:         c.Settings.SettingsPut.LogLevels,
			MetricsEnabled:    c.Settings.SettingsPut.MetricsEnabled,
			MetricsComponents: c.Settings.SettingsPut.MetricsComponents,
		}},
	}

	shared := system.SharedConfig{
		Network:  system.NetworkGlobal{NetworkGlobalPut: system.NetworkGlobalPut{OperationsCenterAddress: c.Network.NetworkPut.OperationsCenterAddress}},
		Security: c.Security,
		Updates:  c.Updates,
	}

	return local, shared
}

func Test_validate(t *testing.T) {
	tests := []struct {
		name      string
		oldCfg    *testConfig
		cfg       testConfig
		isIncusOS bool
		clustered bool

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "empty",
			cfg: testConfig{
				Updates: defaultUpdates,
			},

			assertErr: require.NoError,
		},

		// Network
		{
			name: "require network.rest_server_address to be not empty on IncusOS",
			oldCfg: &testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "127.0.0.1:7443",
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "", // empty not allowed on IncusOS
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.rest_server_address" can not be empty when running on IncusOS`)
			},
		},
		{
			name: "require network.rest_server_address to be valid - multiple single colons",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "127.0.0.1:0:7443", // invalid multiple single colons
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.rest_server_address" is not a valid address:`)
			},
		},
		{
			name: "require network.rest_server_address to be valid - not an ip",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "localhost:7443", // invalid not an ip
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.rest_server_address" does not contain a valid ip`)
			},
		},
		{
			name: "require network.rest_server_address to be valid - invalid port",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "127.0.0.1:abc", // invalid port
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.rest_server_address" does not contain a valid port`)
			},
		},
		{
			name: "require network.rest_server_address to be valid - port out of range",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "127.0.0.1:0", // invalid port out of range
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.rest_server_address" port out of range (1 - 65535)`)
			},
		},
		{
			name: "require network.rest_server_address to be valid - port out of range",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "127.0.0.1:70000", // invalid port out of range
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.rest_server_address" port out of range (1 - 65535)`)
			},
		},
		{
			name: "network.address without network.rest_server_address",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.NoError,
		},
		{
			name: "require network.address if network.rest_server_address is set",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress: "127.0.0.1:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.address" must be set if Operations Center is clustered or "network.rest_server_address" is set`)
			},
		},
		{
			name: "require network.address if clustered",
			oldCfg: &testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						OperationsCenterAddress: "https://192.0.2.1:8443",
					},
				},
				Updates: defaultUpdates,
			},
			cfg: testConfig{
				Updates: defaultUpdates,
			},
			clustered: true,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.address" must be set if Operations Center is clustered or "network.rest_server_address" is set`)
			},
		},
		{
			name: "network.rest_server_address can be unset if clustered",
			oldCfg: &testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "192.0.2.1:8443",
						OperationsCenterAddress: "https://192.0.2.1:8443",
					},
				},
				Updates: defaultUpdates,
			},
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						OperationsCenterAddress: "https://192.0.2.1:8443",
					},
				},
				Updates: defaultUpdates,
			},
			clustered: true,

			assertErr: require.NoError,
		},
		{
			name: "invalid network.address",
			cfg: testConfig{
				Network: testNetwork{
					NetworkPut: testNetworkPut{
						RestServerAddress:       "127.0.0.1:7443",
						OperationsCenterAddress: ":|\\", // invalid
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.address" property is expected to be a valid URL:`)
			},
		},

		// Updates
		{
			name: "invalid updates.source",
			cfg: testConfig{
				Updates: system.Updates{
					UpdatesPut: system.UpdatesPut{
						Source:                      ":|\\", // invalid
						SignatureVerificationRootCA: signatureVerificationRootCA,
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "empty updates.signature_verification_root_ca",
			cfg: testConfig{
				Updates: system.Updates{
					UpdatesPut: system.UpdatesPut{
						SignatureVerificationRootCA: ``, // empty
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "invalid updates.signature_verification_root_ca",
			cfg: testConfig{
				Updates: system.Updates{
					UpdatesPut: system.UpdatesPut{
						SignatureVerificationRootCA: `invalid`, // invalid
					},
				},
			},

			assertErr: require.Error,
		},

		// Security
		{
			name: "invalid security.oidc.issuer",
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						OIDC: system.SecurityOIDC{
							Issuer: ":|\\", // invalid
						},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "invalid security.openfga.api_url",
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						OpenFGA: system.SecurityOpenFGA{
							APIURL: ":|\\", // invalid
						},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "invalid security.acme config",
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						ACME: system.SecurityACME{
							CAURL: ":|\\", // invalid
						},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "invalid security.trusted_https_proxies config",
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedHTTPSProxies: []string{
							":|\\", // invalid
						},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "empty security.trusted_tls_client_cert_fingerprints",
			oldCfg: &testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertFingerprints: []string{"fingerprint"},
					},
				},
				Updates: defaultUpdates,
			},
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertFingerprints: []string{}, // empty
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: false,

			assertErr: require.NoError,
		},
		{
			name: "empty security.trusted_tls_client_cert_fingerprints on IncusOS",
			oldCfg: &testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertFingerprints: []string{"fingerprint"},
					},
				},
				Updates: defaultUpdates,
			},
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertFingerprints: []string{}, // empty
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.Error,
		},
		{
			name: "valid security.trusted_tls_client_certificates",
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertificates: []string{testcert.ClientCertificate},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.NoError,
		},
		{
			name: "security.trusted_tls_client_certificates is a valid fallback authentication method on IncusOS",
			oldCfg: &testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertFingerprints: []string{"fingerprint"},
					},
				},
				Updates: defaultUpdates,
			},
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertFingerprints: []string{}, // empty
						TrustedTLSClientCertificates:     []string{testcert.ClientCertificate},
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.NoError,
		},
		{
			name: "invalid security.trusted_tls_client_certificates",
			cfg: testConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedTLSClientCertificates: []string{"not a certificate"}, // invalid
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},

		// Settings
		{
			name: "invalid log level",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						LogLevel: "invalid", // invalid log level.
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.Error,
		},
		{
			name: "valid log levels per component",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						LogLevels: map[string]string{
							"provisioning":             "DEBUG",
							"provisioning.server_repo": "TRACE",
						},
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.NoError,
		},
		{
			name: "invalid log levels per component - invalid level",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						LogLevels: map[string]string{
							"provisioning": "invalid", // invalid log level.
						},
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.Error,
		},
		{
			name: "invalid log levels per component - empty level",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						LogLevels: map[string]string{
							"provisioning": "", // empty log level.
						},
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.Error,
		},
		{
			name: "invalid log levels per component - invalid component name",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						LogLevels: map[string]string{
							"Provisioning.": "DEBUG", // invalid component name.
						},
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.Error,
		},
		{
			name: "valid metrics components",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						MetricsEnabled:    true,
						MetricsComponents: []string{"runtime", "provisioning.server_service"},
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.NoError,
		},
		{
			name: "invalid metrics components - invalid component name",
			cfg: testConfig{
				Settings: testSettings{
					SettingsPut: testSettingsPut{
						MetricsComponents: []string{"Runtime."}, // invalid component name.
					},
				},
				Updates: defaultUpdates,
			},
			isIncusOS: true,

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			oldCfg := testConfig{}
			if tc.oldCfg != nil {
				oldCfg = *tc.oldCfg
			}

			oldLocal, oldShared := oldCfg.split()
			newLocal, newShared := tc.cfg.split()

			// The validation of a configuration update, see system.UpdateNetworkConfig.
			err := validate(oldLocal, newLocal, tc.isIncusOS, tc.clustered)
			if err == nil {
				err = validateShared(oldShared, newShared, tc.isIncusOS)
			}

			if err == nil {
				err = validateSharedNetworkConfig(newLocal.Network, oldShared.Network, newShared.Network, tc.clustered)
			}

			tc.assertErr(t, err)
		})
	}
}

func TestValidateClusterAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string

		assertErr require.ErrorAssertionFunc
	}{
		{name: "IPv4", address: "192.0.2.1:8443", assertErr: require.NoError},
		{name: "IPv6", address: "[2001:db8::1]:8443", assertErr: require.NoError},
		{name: "empty", address: "", assertErr: require.Error},
		{name: "no port", address: "192.0.2.1", assertErr: require.Error},
		{name: "empty port", address: "192.0.2.1:", assertErr: require.Error},
		{name: "port not a number", address: "192.0.2.1:abc", assertErr: require.Error},
		{name: "port 0", address: "192.0.2.1:0", assertErr: require.Error},
		{name: "port out of range", address: "192.0.2.1:70000", assertErr: require.Error},
		{name: "host name", address: "localhost:8443", assertErr: require.Error},
		{name: "loopback", address: "127.0.0.1:8443", assertErr: require.Error},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateClusterAddress(tc.address)
			tc.assertErr(t, err)

			// A valid cluster address is a valid address of Operations Center.
			if err == nil {
				require.NoError(t, validateURI("https://"+tc.address, true, false, true))
			}
		})
	}
}

func Test_validateDelegated(t *testing.T) {
	tests := []struct {
		name                              string
		oldCfg                            system.SharedConfig
		cfg                               system.SharedConfig
		updateValidateSignalListenerErr   error
		settingsValidateSignalListenerErr error
		securityValidateSignalListenerErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:                            "update validation signal error",
			updateValidateSignalListenerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name:                              "settings validation signal error",
			settingsValidateSignalListenerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "security validation signal is not emitted if oidc and openfga are unchanged",
			cfg: system.SharedConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						TrustedHTTPSProxies: []string{"127.0.0.1"},
					},
				},
			},
			securityValidateSignalListenerErr: boom.Error,

			assertErr: require.NoError,
		},
		{
			name: "security validation signal error on oidc change",
			cfg: system.SharedConfig{
				Security: system.Security{
					SecurityPut: system.SecurityPut{
						OIDC: system.SecurityOIDC{
							Issuer: "https://oidc.local",
						},
					},
				},
			},
			securityValidateSignalListenerErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle.UpdatesValidateSignal.AddListenerWithErr(func(_ context.Context, _ system.Updates) error {
				return tc.updateValidateSignalListenerErr
			}, tc.name)
			defer lifecycle.UpdatesValidateSignal.RemoveListener(tc.name)

			lifecycle.SharedSettingsValidateSignal.AddListenerWithErr(func(_ context.Context, _ system.SettingsGlobal) error {
				return tc.settingsValidateSignalListenerErr
			}, tc.name)
			defer lifecycle.SharedSettingsValidateSignal.RemoveListener(tc.name)

			lifecycle.SecurityValidateSignal.AddListenerWithErr(func(_ context.Context, _ system.Security) error {
				return tc.securityValidateSignalListenerErr
			}, tc.name)
			defer lifecycle.SecurityValidateSignal.RemoveListener(tc.name)

			err := validateDelegated(t.Context(), tc.oldCfg, tc.cfg)

			tc.assertErr(t, err)
		})
	}
}

var defaultUpdates = system.Updates{
	UpdatesPut: system.UpdatesPut{
		SignatureVerificationRootCA: signatureVerificationRootCA,
		UpdatesDefaultChannel:       "stable",
		ServerDefaultChannel:        "stable",
	},
}

const signatureVerificationRootCA = `-----BEGIN CERTIFICATE-----
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
-----END CERTIFICATE-----`

func Test_UpdateSecurity_trustedClientCertificates(t *testing.T) {
	env := &mock.EnvironmentMock{
		ClusteredFunc: func() bool { return false },
		IsIncusOSFunc: func() bool {
			return true
		},
	}

	InitTest(t, env, nil)

	// On IncusOS, a security config without any trusted TLS client would lock the
	// user out. Providing only a trusted client certificate is enough, since its
	// fingerprint is derived from the certificate.
	err := UpdateSecurity(t.Context(), system.SecurityPut{
		TrustedTLSClientCertificates: []string{testcert.ClientCertificate},
	})
	require.NoError(t, err)

	require.Empty(t, GetSecurity().TrustedTLSClientCertFingerprints)
	require.Equal(t, []string{testcert.ClientCertificate}, GetSecurity().TrustedTLSClientCertificates)

	err = UpdateSecurity(t.Context(), system.SecurityPut{
		TrustedTLSClientCertFingerprints: []string{"fingerprint"},
	})
	require.NoError(t, err)

	require.Equal(t, []string{"fingerprint"}, GetSecurity().TrustedTLSClientCertFingerprints)
	require.Empty(t, GetSecurity().TrustedTLSClientCertificates)
}

func Test_validateURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		uri            string
		required       bool
		enforceNoPath  bool
		enforceNoQuery bool

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success - empty",

			assertErr: require.NoError,
		},
		{
			name: "success - full URI",
			uri:  "http://domain.tld:1234/path?query=true",

			assertErr: require.NoError,
		},

		{
			name:     "error - required but empty",
			required: true,

			assertErr: require.Error,
		},
		{
			name: "error - parse failure",
			uri:  ":|\\",

			assertErr: require.Error,
		},
		{
			name: "error - no scheme",
			uri:  "//1.2.3.4:1234",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to determine scheme")
			},
		},
		{
			name: "error - no hostname",
			uri:  "http:///",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to determine host")
			},
		},
		{
			name: "error - invalid port",
			uri:  "http://domain.tld:10000000000000000000", // > max int 64 (9.223.372.036.854.775.807)
			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Port "10000000000000000000" is invalid`)
			},
		},
		{
			name: "error - invalid port 0",
			uri:  "http://domain.tld:0",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Port 0 is invalid")
			},
		},
		{
			name:          "error - enforceNoPath with path",
			uri:           "http://domain.tld:1234/path",
			enforceNoPath: true,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Contains path")
			},
		},
		{
			name:           "error - enforceNoQuery with query",
			uri:            "http://domain.tld:1234/?query=true",
			enforceNoQuery: true,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Contains query")
			},
		},
		{
			name: "error - with fragment",
			uri:  "http://domain.tld:1234/#fragment",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Contains fragment")
			},
		},
		{
			name: "error - has user",
			uri:  "http://user@domain.tld:1234",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Contains username")
			},
		},
		{
			name: "error - has password",
			uri:  "http://:password@domain.tld",

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Contains password")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateURI(tc.uri, tc.required, tc.enforceNoPath, tc.enforceNoQuery)

			tc.assertErr(t, err)
		})
	}
}
