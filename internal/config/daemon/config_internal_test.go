package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_validate(t *testing.T) {
	tests := []struct {
		name      string
		oldCfg    *config
		cfg       config
		isIncusOS bool

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "empty",
			cfg: config{
				Updates: defaultUpdates,
			},

			assertErr: require.NoError,
		},

		// Network
		{
			name: "require network.rest_server_address to be not empty on IncusOS",
			oldCfg: &config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						RestServerAddress:       "127.0.0.1:7443",
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
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
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
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
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
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
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
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
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
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
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
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
			name: "require network.rest_server_address if network.address is set",
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.address" and "network.rest_server_address" either both are set or both are unset`)
			},
		},
		{
			name: "require network.address if network.rest_server_address is set",
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						RestServerAddress: "127.0.0.1:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Invalid config, "network.address" and "network.rest_server_address" either both are set or both are unset`)
			},
		},
		{
			name: "invalid network.address",
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						RestServerAddress:       "127.0.0.1:7443",
						OperationsCenterAddress: ":|\\", // invalid,
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
			cfg: config{
				Updates: api.SystemUpdates{
					SystemUpdatesPut: api.SystemUpdatesPut{
						Source:                      ":|\\", // invalid,
						SignatureVerificationRootCA: signatureVerificationRootCA,
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "empty updates.signature_verification_root_ca",
			cfg: config{
				Updates: api.SystemUpdates{
					SystemUpdatesPut: api.SystemUpdatesPut{
						SignatureVerificationRootCA: ``, // empty
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "invalid updates.signature_verification_root_ca",
			cfg: config{
				Updates: api.SystemUpdates{
					SystemUpdatesPut: api.SystemUpdatesPut{
						SignatureVerificationRootCA: `invalid`, // invalid
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "invalid updates.filter_expression",
			cfg: config{
				Updates: api.SystemUpdates{
					SystemUpdatesPut: api.SystemUpdatesPut{
						SignatureVerificationRootCA: signatureVerificationRootCA,
						FilterExpression:            `invalid`, // invalid
					},
				},
			},

			assertErr: require.Error,
		},
		{
			name: "invalid updates.file_filter_expression",
			cfg: config{
				Updates: api.SystemUpdates{
					SystemUpdatesPut: api.SystemUpdatesPut{
						SignatureVerificationRootCA: signatureVerificationRootCA,
						FileFilterExpression:        `invalid`, // invalid
					},
				},
			},

			assertErr: require.Error,
		},

		// Security
		{
			name: "invalid security.oidc.issuer",
			cfg: config{
				Security: api.SystemSecurity{
					SystemSecurityPut: api.SystemSecurityPut{
						OIDC: api.SystemSecurityOIDC{
							Issuer: ":|\\", // invalid,
						},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "invalid security.openfga.api_url",
			cfg: config{
				Security: api.SystemSecurity{
					SystemSecurityPut: api.SystemSecurityPut{
						OpenFGA: api.SystemSecurityOpenFGA{
							APIURL: ":|\\", // invalid,
						},
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "empty security.trusted_tls_client_cert_fingerprints",
			oldCfg: &config{
				Security: api.SystemSecurity{
					SystemSecurityPut: api.SystemSecurityPut{
						TrustedTLSClientCertFingerprints: []string{"fingerprint"},
					},
				},
				Updates: defaultUpdates,
			},
			cfg: config{
				Security: api.SystemSecurity{
					SystemSecurityPut: api.SystemSecurityPut{
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
			oldCfg: &config{
				Security: api.SystemSecurity{
					SystemSecurityPut: api.SystemSecurityPut{
						TrustedTLSClientCertFingerprints: []string{"fingerprint"},
					},
				},
				Updates: defaultUpdates,
			},
			cfg: config{
				Security: api.SystemSecurity{
					SystemSecurityPut: api.SystemSecurityPut{
						TrustedTLSClientCertFingerprints: []string{}, // empty
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
			env := &mock.EnvironmentMock{
				IsIncusOSFunc: func() bool {
					return tc.isIncusOS
				},
			}

			InitTest(t, env, nil)

			if tc.oldCfg != nil {
				err := UpdateNetwork(t.Context(), tc.oldCfg.Network.SystemNetworkPut)
				require.NoError(t, err)

				err = UpdateUpdates(t.Context(), tc.oldCfg.Updates.SystemUpdatesPut)
				require.NoError(t, err)

				err = UpdateSecurity(t.Context(), tc.oldCfg.Security.SystemSecurityPut)
				require.NoError(t, err)
			}

			err := validate(tc.cfg)

			tc.assertErr(t, err)
		})
	}
}

var defaultUpdates = api.SystemUpdates{
	SystemUpdatesPut: api.SystemUpdatesPut{
		SignatureVerificationRootCA: signatureVerificationRootCA,
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
