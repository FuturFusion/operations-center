package client_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/util/testing/certs"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

func Test_GetSystemNetworkConfig(t *testing.T) {
	d := daemonSetup(t)

	networkConfig, err := d.socketClient.GetSystemNetworkConfig(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, networkConfig.OperationsCenterAddress)
	require.NotEmpty(t, networkConfig.RestServerAddress)

	_, err = d.unauthorizedHTTPClient.GetSystemNetworkConfig(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

// Test_UpdateSystemNetworkConfig only covers the rejected updates. Applying a
// new network configuration rebinds the listener of the running daemon.
func Test_UpdateSystemNetworkConfig(t *testing.T) {
	d := daemonSetup(t)

	currentConfig, err := d.socketClient.GetSystemNetworkConfig(t.Context())
	require.NoError(t, err)

	tests := []struct {
		name   string
		client client.OperationsCenterClient

		networkConfig system.NetworkPut

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:   "error - not authorized",
			client: d.unauthorizedHTTPClient,

			networkConfig: currentConfig.NetworkPut,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
		},
		{
			name:   "error - validation, rest server address is not an address",
			client: d.socketClient,

			networkConfig: system.NetworkPut{
				OperationsCenterAddress: currentConfig.OperationsCenterAddress,
				RestServerAddress:       "not-an-address",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `"not-an-address" is not a valid IP address`)
			},
		},
		{
			name:   "error - validation, rest server address without valid ip",
			client: d.socketClient,

			networkConfig: system.NetworkPut{
				OperationsCenterAddress: currentConfig.OperationsCenterAddress,
				RestServerAddress:       "not-an-ip:8443",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `"not-an-ip" is not a valid IP address`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.client.UpdateSystemNetworkConfig(t.Context(), tc.networkConfig)

			tc.assertErr(t, err)
		})
	}

	// The network configuration is unchanged.
	unchangedConfig, err := d.socketClient.GetSystemNetworkConfig(t.Context())
	require.NoError(t, err)
	require.Equal(t, currentConfig, unchangedConfig)
}

func Test_GetSystemSecurityConfig(t *testing.T) {
	d := daemonSetup(t)

	securityConfig, err := d.socketClient.GetSystemSecurityConfig(t.Context())
	require.NoError(t, err)
	require.Len(t, securityConfig.TrustedTLSClientCertFingerprints, 1)

	_, err = d.unauthorizedHTTPClient.GetSystemSecurityConfig(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

// Test_UpdateSystemSecurityConfig only covers the rejected updates. Applying a
// new security configuration would revoke the trust into the client
// certificate used by the tests.
func Test_UpdateSystemSecurityConfig(t *testing.T) {
	d := daemonSetup(t)

	currentConfig, err := d.socketClient.GetSystemSecurityConfig(t.Context())
	require.NoError(t, err)

	err = d.unauthorizedHTTPClient.UpdateSystemSecurityConfig(t.Context(), currentConfig.SecurityPut)
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	securityConfig := currentConfig.SecurityPut
	securityConfig.TrustedTLSClientCertificates = []string{"not a PEM encoded certificate"}

	err = d.socketClient.UpdateSystemSecurityConfig(t.Context(), securityConfig)
	require.Error(t, err)

	// The security configuration is unchanged.
	unchangedConfig, err := d.socketClient.GetSystemSecurityConfig(t.Context())
	require.NoError(t, err)
	require.Equal(t, currentConfig, unchangedConfig)
}

func Test_GetSystemSettingsConfig(t *testing.T) {
	d := daemonSetup(t)

	settingsConfig, err := d.socketClient.GetSystemSettingsConfig(t.Context())
	require.NoError(t, err)
	require.Empty(t, settingsConfig.ServerRegistrationScriptlet)

	_, err = d.unauthorizedHTTPClient.GetSystemSettingsConfig(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

func Test_UpdateSystemSettingsConfig(t *testing.T) {
	d := daemonSetup(t)

	tests := []struct {
		name   string
		client client.OperationsCenterClient

		settingsConfig system.SettingsPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,

			settingsConfig: system.SettingsPut{
				LogLevel:                    "DEBUG",
				ServerRegistrationScriptlet: "",
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				settingsConfig, err := d.socketClient.GetSystemSettingsConfig(t.Context())
				require.NoError(t, err)
				require.Equal(t, "DEBUG", settingsConfig.LogLevel)
			},
		},
		{
			name:   "error - not authorized",
			client: d.unauthorizedHTTPClient,

			settingsConfig: system.SettingsPut{
				LogLevel: "INFO",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:   "success - log levels per component",
			client: d.socketClient,

			settingsConfig: system.SettingsPut{
				// A level less verbose than the default keeps the rest of the
				// test suite quiet.
				LogLevels: map[string]string{"provisioning": "ERROR"},
			},

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				settingsConfig, err := d.socketClient.GetSystemSettingsConfig(t.Context())
				require.NoError(t, err)
				require.Equal(t, map[string]string{"provisioning": "ERROR"}, settingsConfig.LogLevels)

				require.NoError(t, d.socketClient.UpdateSystemSettingsConfig(t.Context(), system.SettingsPut{}))
			},
		},
		{
			name:   "error - validation, invalid log level",
			client: d.socketClient,

			settingsConfig: system.SettingsPut{
				LogLevel: "not-a-log-level",
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Log level "not-a-log-level" is invalid`)
			},
			assertFunc: noop,
		},
		{
			name:   "error - validation, invalid component name",
			client: d.socketClient,

			settingsConfig: system.SettingsPut{
				LogLevels: map[string]string{"Provisioning": "DEBUG"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `Component "Provisioning" is invalid`)
			},
			assertFunc: noop,
		},
		{
			name:   "error - validation, unknown component",
			client: d.socketClient,

			settingsConfig: system.SettingsPut{
				LogLevels: map[string]string{"provisionning": "DEBUG"},
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `unknown components "provisionning"`)
			},
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.client.UpdateSystemSettingsConfig(t.Context(), tc.settingsConfig)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_GetSystemUpdatesConfig(t *testing.T) {
	d := daemonSetup(t)

	updatesConfig, err := d.socketClient.GetSystemUpdatesConfig(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, updatesConfig.Source)
	require.Equal(t, defaultChannelName, updatesConfig.UpdatesDefaultChannel)
	require.Equal(t, defaultChannelName, updatesConfig.ServerDefaultChannel)

	_, err = d.unauthorizedHTTPClient.GetSystemUpdatesConfig(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

func Test_UpdateSystemUpdatesConfig(t *testing.T) {
	d := daemonSetup(t)

	currentConfig, err := d.socketClient.GetSystemUpdatesConfig(t.Context())
	require.NoError(t, err)

	tests := []struct {
		name   string
		client client.OperationsCenterClient

		updatesConfig system.UpdatesPut

		assertErr  require.ErrorAssertionFunc
		assertFunc func(t *testing.T)
	}{
		{
			name:   "success",
			client: d.socketClient,

			updatesConfig: func() system.UpdatesPut {
				cfg := currentConfig.UpdatesPut
				cfg.FilterExpression = `"stable" in upstream_channels`
				cfg.ImageServerAuthenticationByQueryParam = true

				return cfg
			}(),

			assertErr: require.NoError,
			assertFunc: func(t *testing.T) {
				t.Helper()

				updatesConfig, err := d.socketClient.GetSystemUpdatesConfig(t.Context())
				require.NoError(t, err)
				require.Equal(t, `"stable" in upstream_channels`, updatesConfig.FilterExpression)
				require.True(t, updatesConfig.ImageServerAuthenticationByQueryParam)
			},
		},
		{
			name:   "error - not authorized",
			client: d.unauthorizedHTTPClient,

			updatesConfig: currentConfig.UpdatesPut,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, domain.ErrNotAuthenticated)
			},
			assertFunc: noop,
		},
		{
			name:   "error - validation, unknown default channel",
			client: d.socketClient,

			updatesConfig: func() system.UpdatesPut {
				cfg := currentConfig.UpdatesPut
				cfg.UpdatesDefaultChannel = "unknown"

				return cfg
			}(),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, `"updates.updates_default_channel"`)
			},
			assertFunc: noop,
		},
		{
			name:   "error - validation, invalid filter expression",
			client: d.socketClient,

			updatesConfig: func() system.UpdatesPut {
				cfg := currentConfig.UpdatesPut
				cfg.FilterExpression = "this is not a valid expression"

				return cfg
			}(),

			assertErr:  require.Error,
			assertFunc: noop,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.client.UpdateSystemUpdatesConfig(t.Context(), tc.updatesConfig)

			tc.assertErr(t, err)
			tc.assertFunc(t)
		})
	}
}

func Test_GetSystemCertificate(t *testing.T) {
	d := daemonSetup(t)

	certificate, err := d.socketClient.GetSystemCertificate(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, certificate.Certificate)
	require.NotEmpty(t, certificate.Fingerprint)
	require.False(t, certificate.NotBefore.IsZero())
	require.False(t, certificate.NotAfter.IsZero())

	_, err = d.unauthorizedHTTPClient.GetSystemCertificate(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)
}

// Test_SetSystemCertificate uses a dedicated daemon, because it replaces the
// server certificate, which invalidates the certificate pinned by the HTTPS
// clients.
func Test_SetSystemCertificate(t *testing.T) {
	d := daemonSetup(t)

	_, intermediatePEM, leafPEM, leafKeyPEM := certs.GenerateChain(t)

	err := d.unauthorizedHTTPClient.SetSystemCertificate(t.Context(), system.CertificatePost{
		Certificate: string(leafPEM),
		Key:         string(leafKeyPEM),
	})
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	err = d.socketClient.SetSystemCertificate(t.Context(), system.CertificatePost{
		Certificate: "not a PEM encoded certificate",
		Key:         string(leafKeyPEM),
	})
	require.ErrorContains(t, err, "Failed to validate key pair")

	// The certificate chain has to be provided leaf first.
	err = d.socketClient.SetSystemCertificate(t.Context(), system.CertificatePost{
		Certificate: string(intermediatePEM) + string(leafPEM),
		Key:         string(leafKeyPEM),
	})
	require.Error(t, err)

	previousCertificate, err := d.socketClient.GetSystemCertificate(t.Context())
	require.NoError(t, err)

	err = d.socketClient.SetSystemCertificate(t.Context(), system.CertificatePost{
		Certificate: string(leafPEM) + string(intermediatePEM),
		Key:         string(leafKeyPEM),
	})
	require.NoError(t, err)

	newCertificate, err := d.socketClient.GetSystemCertificate(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, previousCertificate.Fingerprint, newCertificate.Fingerprint)
	require.Equal(t, "CN=localhost,O=Linux Containers", newCertificate.Subject)
}

// Test_RenewSystemCertificate does not cover an actual renewal, that requires a
// reachable ACME server.
func Test_RenewSystemCertificate(t *testing.T) {
	d := daemonSetup(t)

	err := d.unauthorizedHTTPClient.RenewSystemCertificate(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	previousCertificate, err := d.socketClient.GetSystemCertificate(t.Context())
	require.NoError(t, err)

	// With ACME disabled, which is the default, the renewal is a no-op and the
	// server certificate is left untouched.
	err = d.socketClient.RenewSystemCertificate(t.Context())
	require.NoError(t, err)

	currentCertificate, err := d.socketClient.GetSystemCertificate(t.Context())
	require.NoError(t, err)
	require.Equal(t, previousCertificate.Fingerprint, currentCertificate.Fingerprint)
}

func Test_CleanSystemCache(t *testing.T) {
	d := daemonSetup(t)

	err := d.unauthorizedHTTPClient.CleanSystemCache(t.Context())
	require.ErrorIs(t, err, domain.ErrNotAuthenticated)

	err = d.socketClient.CleanSystemCache(t.Context())
	require.NoError(t, err)
}
