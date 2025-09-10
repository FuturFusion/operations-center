package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/shared/api"
)

func Test_validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  config

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
			name: "require network.rest_server_address to be valid",
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						RestServerAddress:       "127.0.0.1:0:7443", // invalid address
						OperationsCenterAddress: "http://localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
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

			assertErr: require.Error,
		},
		{
			name: "require network.address if network.rest_server_address is set",
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						RestServerAddress: "localhost:7443",
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
		},
		{
			name: "invalid network.address",
			cfg: config{
				Network: api.SystemNetwork{
					SystemNetworkPut: api.SystemNetworkPut{
						RestServerAddress:       "localhost:7443",
						OperationsCenterAddress: ":|\\", // invalid,
					},
				},
				Updates: defaultUpdates,
			},

			assertErr: require.Error,
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
