package system_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	incustls "github.com/lxc/incus/v7/shared/tls"
	"github.com/stretchr/testify/require"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/domain"
	envMock "github.com/FuturFusion/operations-center/internal/environment/mock"
	"github.com/FuturFusion/operations-center/internal/lifecycle"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/system"
	"github.com/FuturFusion/operations-center/internal/system/mock"
	repoMock "github.com/FuturFusion/operations-center/internal/system/repo/mock"
	"github.com/FuturFusion/operations-center/internal/util/certificate"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/testing/boom"
	"github.com/FuturFusion/operations-center/internal/util/testing/certs"
	"github.com/FuturFusion/operations-center/internal/util/testing/log"
	testingnet "github.com/FuturFusion/operations-center/internal/util/testing/net"
	"github.com/FuturFusion/operations-center/internal/util/testing/queue"
	"github.com/FuturFusion/operations-center/internal/util/testing/testcert"
	"github.com/FuturFusion/operations-center/shared/api"
	systemapi "github.com/FuturFusion/operations-center/shared/api/system"
)

func TestSystemService_GetCertificate(t *testing.T) {
	t.Parallel()

	certPEM, _, err := incustls.GenerateMemCert(false, false)
	require.NoError(t, err)

	certFingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	cert, err := certificate.Decode(certPEM)
	require.NoError(t, err)

	tests := []struct {
		name              string
		certificateFileIn []byte

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:              "success",
			certificateFileIn: certPEM,

			assertErr: require.NoError,
		},
		{
			name: "error - certificate file not found",

			assertErr: require.Error,
		},
		{
			name:              "error - invalid certificate",
			certificateFileIn: []byte("invalid"),

			assertErr: require.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()

			if tc.certificateFileIn != nil {
				err := os.WriteFile(filepath.Join(tmpDir, "server.crt"), tc.certificateFileIn, 0o600)
				require.NoError(t, err)
			}

			env := &envMock.EnvironmentMock{
				ClusteredFunc: func() bool { return false },
				VarDirFunc: func() string {
					return tmpDir
				},
			}

			systemSvc := system.NewSystemService(env, nil, nil)

			// Execute test
			gotCertificate, err := systemSvc.GetCertificate(context.Background())

			// Assert results
			tc.assertErr(t, err)
			if err != nil {
				return
			}

			require.Equal(t, string(certPEM), gotCertificate.Certificate)
			require.Equal(t, certFingerprint, gotCertificate.Fingerprint)
			require.Equal(t, cert.Subject.String(), gotCertificate.Subject)
			require.Equal(t, cert.Issuer.String(), gotCertificate.Issuer)
			require.Equal(t, cert.NotAfter, gotCertificate.NotAfter)
		})
	}
}

func TestSystemService_UpdateCertificate(t *testing.T) {
	currentCertPEM, currentKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	currentCertFingerprint, err := incustls.CertFingerprintStr(string(currentCertPEM))
	require.NoError(t, err)

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	certFingerprint, err := incustls.CertFingerprintStr(string(certPEM))
	require.NoError(t, err)

	rootPEM, intermediatePEM, leafPEM, leafKeyPEM := certs.GenerateChain(t)

	leafFingerprint, err := incustls.CertFingerprintStr(string(leafPEM))
	require.NoError(t, err)

	chainPEM := string(leafPEM) + string(intermediatePEM)

	_, _, otherLeafPEM, _ := certs.GenerateChain(t)

	operationsCenterServer := provisioning.Server{
		Name: "operations-center",
		Type: api.ServerTypeOperationsCenter,
	}

	pinnedProviderConfig := func() provisioning.ServerSystemProvider {
		return incusosapi.SystemProvider{
			Config: incusosapi.SystemProviderConfig{
				Config: map[string]string{
					"server_certificate": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
				},
			},
		}
	}

	tests := []struct {
		name                        string
		setupEnv                    func(t *testing.T, targetDir string)
		certPEM                     string
		keyPEM                      string
		serverGetAllWithFilter      provisioning.Servers
		serverGetAllWithFilterErr   error
		serverGetAll                provisioning.Servers
		serverGetAllErr             error
		serverGetSystemProvider     []queue.Item[provisioning.ServerSystemProvider]
		serverUpdateSystemProvider  []queue.Item[struct{}]
		serverRestartApplicationErr error
		clustered                   bool

		assertErr                       require.ErrorAssertionFunc
		assertLog                       log.MatcherFunc
		wantServerCertificateUpdateEmit []queue.Item[string]
		wantProviderCertificate         string
		wantCertificateFile             string
		wantKeyFile                     string
		wantRestarted                   []string
	}{
		{
			name: "success - no registered servers except for self-registered operations-center",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
			serverGetAll:           provisioning.Servers{operationsCenterServer},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "success - self-registered operations-center with openfga",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{
				{
					Name:        "operations-center",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new(string(currentCertPEM)),
					VersionData: api.ServerVersionData{
						Applications: []api.ApplicationVersionData{
							{
								Name: "openfga",
							},
						},
					},
				},
			},
			serverGetAll: provisioning.Servers{operationsCenterServer},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
			wantRestarted:       []string{"operations-center"},
		},
		{
			name: "success - deployed Operations Center is not restarted",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{
				{
					Name:        "operations-center",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new(string(currentCertPEM)),
				},
				{
					Name:        "deployed",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new(string(otherLeafPEM)),
					VersionData: api.ServerVersionData{
						Applications: []api.ApplicationVersionData{{Name: "openfga"}},
					},
				},
			},
			serverGetAll: provisioning.Servers{operationsCenterServer},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
			wantRestarted:       []string{},
		},
		{
			name: "success - clustered - only cluster members are restarted",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:   string(certPEM),
			keyPEM:    string(keyPEM),
			clustered: true,
			serverGetAllWithFilter: provisioning.Servers{
				{
					Name:        "one",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new("member-one"),
					VersionData: api.ServerVersionData{
						Applications: []api.ApplicationVersionData{{Name: "openfga"}},
					},
				},
				{
					Name:        "deployed",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new(string(otherLeafPEM)),
					VersionData: api.ServerVersionData{
						Applications: []api.ApplicationVersionData{{Name: "openfga"}},
					},
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(currentCertPEM),
			wantKeyFile:         string(currentKeyPEM),
			wantRestarted:       []string{"one"},
		},
		{
			name: "success - no registered servers - same certificate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(currentCertPEM),
			keyPEM:  string(currentKeyPEM),

			assertErr:                       require.NoError,
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "success - with registered servers",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				operationsCenterServer,
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{
					Value: pinnedProviderConfig(),
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "success - with registered servers - provider config without config map",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
			},
			serverGetSystemProvider: []queue.Item[provisioning.ServerSystemProvider]{
				{},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "success - with registered servers - unregistered server is skipped",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
			serverGetAll: provisioning.Servers{
				{
					Name:   "one",
					Type:   api.ServerTypeIncus,
					Status: api.ServerStatusUnregistered,
				},
			},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "success - same leaf certificate with added intermediate",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()

				err := os.WriteFile(filepath.Join(targetDir, "server.crt"), leafPEM, 0o600)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(targetDir, "server.key"), leafKeyPEM, 0o600)
				require.NoError(t, err)
			},
			certPEM:                chainPEM,
			keyPEM:                 string(leafKeyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
			serverGetAll:           provisioning.Servers{operationsCenterServer},

			assertErr: require.NoError,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: leafFingerprint,
				},
			},
			wantCertificateFile: chainPEM,
			wantKeyFile:         string(leafKeyPEM),
		},
		{
			name: "success - with registered servers - serverSvc.GetAll fails",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
			serverGetAllErr:        boom.Error,

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to get servers for server certificate update, retry on next poll"),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "success - with registered servers - serverSvc.GetSystemProvider fails",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
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

			assertErr: require.NoError,
			assertLog: log.Contains("Failed to get system provider for server certificate update, retry on next poll"),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "success - with registered servers - one of two servers fails, other one updated",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},
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
					Value: pinnedProviderConfig(),
				},
				{
					Value: pinnedProviderConfig(),
				},
			},
			serverUpdateSystemProvider: []queue.Item[struct{}]{
				{
					Err: boom.Error,
				},
				{},
			},

			assertErr: require.NoError,
			assertLog: log.Match(`Failed to update server certificate in system provider, retry on next poll.*server=one`),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Value: certFingerprint,
				},
			},
			wantCertificateFile: string(certPEM),
			wantKeyFile:         string(keyPEM),
		},
		{
			name: "error - invalid certificate chain",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(leafPEM) + string(otherLeafPEM),
			keyPEM:  string(leafKeyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var validationErr domain.ErrValidation
				require.ErrorAs(tt, err, &validationErr)
				require.ErrorContains(tt, err, "Invalid certificate chain")
			},
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "error - duplicated issuer certificates",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(leafPEM) + string(intermediatePEM) + string(rootPEM) + string(intermediatePEM) + string(rootPEM),
			keyPEM:  string(leafKeyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				var validationErr domain.ErrValidation
				require.ErrorAs(tt, err, &validationErr)
				require.ErrorContains(tt, err, "Invalid certificate chain")
			},
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
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
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
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
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(keyPEM),
		},
		{
			name: "error - unable to write certificate file",
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
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "error - unable to write certificate key file - certificate file is restored",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
				err := os.Chmod(filepath.Join(targetDir, "server.key"), 0o400)
				require.NoError(t, err)
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Failed to persist")
				require.ErrorContains(tt, err, "server.key")
			},
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "error - serverSvc.GetAllWithFilter",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                   string(certPEM),
			keyPEM:                    string(keyPEM),
			serverGetAllWithFilterErr: boom.Error,

			assertErr:                       boom.ErrorIs,
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "error - serverSvc.RestartApplication - on another cluster member",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:   string(certPEM),
			keyPEM:    string(keyPEM),
			clustered: true,
			serverGetAllWithFilter: provisioning.Servers{
				{Name: "one", Type: api.ServerTypeOperationsCenter, Certificate: new("member-one")},
				{
					Name:        "two",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new("member-two"),
					VersionData: api.ServerVersionData{
						Applications: []api.ApplicationVersionData{{Name: "openfga"}},
					},
				},
			},
			serverRestartApplicationErr: boom.Error,

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorIs(tt, err, boom.Error)
				require.ErrorContains(tt, err, `Failed to restart OpenFGA application on "two"`)
			},
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "error - serverSvc.RestartApplication",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM: string(certPEM),
			keyPEM:  string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{
				{
					Name:        "operations-center",
					Type:        api.ServerTypeOperationsCenter,
					Certificate: new(string(currentCertPEM)),
					VersionData: api.ServerVersionData{
						Applications: []api.ApplicationVersionData{
							{
								Name: "openfga",
							},
						},
					},
				},
			},
			serverRestartApplicationErr: boom.Error,

			assertErr:                       boom.ErrorIs,
			assertLog:                       log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{},
			wantCertificateFile:             string(currentCertPEM),
			wantKeyFile:                     string(currentKeyPEM),
		},
		{
			name: "error - ServerCertificateUpdateSignal",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Err:   boom.Error,
					Value: certFingerprint,
				},
				{
					Value: currentCertFingerprint,
				},
			},
			wantCertificateFile: string(currentCertPEM),
			wantKeyFile:         string(currentKeyPEM),
		},
		{
			name: "error - ServerCertificateUpdateSignal - revert error",
			setupEnv: func(t *testing.T, targetDir string) {
				t.Helper()
			},
			certPEM:                string(certPEM),
			keyPEM:                 string(keyPEM),
			serverGetAllWithFilter: provisioning.Servers{operationsCenterServer},

			assertErr: boom.ErrorIs,
			assertLog: log.EmptyWithIgnorePattern(log.IgnorePatternDebugLines),
			wantServerCertificateUpdateEmit: []queue.Item[string]{
				{
					Err:   fmt.Errorf("error"),
					Value: certFingerprint,
				},
				{
					Err:   boom.Error,
					Value: currentCertFingerprint,
				},
			},
			wantCertificateFile: string(currentCertPEM),
			wantKeyFile:         string(currentKeyPEM),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if isRoot(t) {
				t.Skip("Test is skipped, if executed as root user")
			}

			// Setup
			logBuf := &bytes.Buffer{}
			err := logger.InitLogger(logBuf, "", false, true, true)
			require.NoError(t, err)

			tmpDir := t.TempDir()

			err = os.WriteFile(filepath.Join(tmpDir, "server.crt"), currentCertPEM, 0o600)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(tmpDir, "server.key"), currentKeyPEM, 0o600)
			require.NoError(t, err)

			// When clustered, the cluster certificate is presented to API clients.
			err = os.WriteFile(filepath.Join(tmpDir, "cluster.crt"), currentCertPEM, 0o600)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(tmpDir, "cluster.key"), currentKeyPEM, 0o600)
			require.NoError(t, err)

			env := &envMock.EnvironmentMock{
				ClusteredFunc: func() bool { return tc.clustered },
				VarDirFunc: func() string {
					return tmpDir
				},
				IsIncusOSFunc: func() bool {
					return true
				},
			}

			tc.setupEnv(t, env.VarDir())

			wantProviderCertificate := tc.wantProviderCertificate
			if wantProviderCertificate == "" {
				wantProviderCertificate = tc.certPEM
			}

			listenerID := uuid.New()
			lifecycle.ServerCertificateUpdateSignal.AddListenerWithErr(func(ctx context.Context, cert tls.Certificate) error {
				wantCertificateFingerprint, err := queue.Pop(t, &tc.wantServerCertificateUpdateEmit)

				certFingerprint := incustls.CertFingerprint(cert.Leaf)
				require.Equal(t, wantCertificateFingerprint, certFingerprint)

				return err
			}, listenerID.String())
			t.Cleanup(func() {
				lifecycle.ServerCertificateUpdateSignal.RemoveListener(listenerID.String())
			})

			restarted := []string{}
			serverSvc := &mock.ProvisioningServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return tc.serverGetAll, tc.serverGetAllErr
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return tc.serverGetAllWithFilter, tc.serverGetAllWithFilterErr
				},
				GetSystemProviderFunc: func(ctx context.Context, name string) (provisioning.ServerSystemProvider, error) {
					return queue.Pop(t, &tc.serverGetSystemProvider)
				},
				UpdateSystemProviderFunc: func(ctx context.Context, name string, providerConfig provisioning.ServerSystemProvider) error {
					require.Equal(t, wantProviderCertificate, providerConfig.Config.Config["server_certificate"])
					_, err := queue.Pop(t, &tc.serverUpdateSystemProvider)
					return err
				},
				RestartApplicationFunc: func(ctx context.Context, name, applicationName string) error {
					restarted = append(restarted, name)
					return tc.serverRestartApplicationErr
				},
			}

			systemSvc := system.NewSystemService(env, serverSvc, nil,
				system.WithClusterCertificateUpdater(func(ctx context.Context, certificatePEM string, keyPEM string) error {
					return nil
				}),
				system.WithClusterMemberCertificates(func(certificatePEM string) bool {
					return certificatePEM == "member-one" || certificatePEM == "member-two"
				}),
			)

			// Run test
			err = systemSvc.UpdateCertificate(context.Background(), tc.certPEM, tc.keyPEM)

			// Assert
			tc.assertErr(t, err)
			tc.assertLog(t, logBuf)

			require.Empty(t, tc.serverGetSystemProvider)
			require.Empty(t, tc.serverUpdateSystemProvider)
			require.Empty(t, tc.wantServerCertificateUpdateEmit)

			if tc.wantRestarted != nil {
				require.Equal(t, tc.wantRestarted, restarted)
			}

			certificateFile, err := os.ReadFile(filepath.Join(tmpDir, "server.crt"))
			require.NoError(t, err)
			require.Equal(t, tc.wantCertificateFile, string(certificateFile), "Certificate file on disk")

			keyFile, err := os.ReadFile(filepath.Join(tmpDir, "server.key"))
			require.NoError(t, err)
			require.Equal(t, tc.wantKeyFile, string(keyFile), "Key file on disk")
		})
	}
}

func isRoot(t *testing.T) bool {
	t.Helper()

	currentUser, err := user.Current()
	require.NoError(t, err)

	return currentUser.Username == "root"
}

func TestSystemService_TriggerCertificateRenew(t *testing.T) {
	t.Parallel()

	currentCertPEM, currentKeyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	certPEM, keyPEM, err := incustls.GenerateMemCert(true, false)
	require.NoError(t, err)

	tests := []struct {
		name                      string
		certPEM                   string
		keyPEM                    string
		acmeUpdateCertificate     *systemapi.CertificatePost
		acmeUpdateCertificateErr  error
		serverGetAllWithFilterErr error

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
			acmeUpdateCertificate: &systemapi.CertificatePost{
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
			acmeUpdateCertificate: &systemapi.CertificatePost{
				Certificate: string(certPEM),
				Key:         string(keyPEM),
			},
			serverGetAllWithFilterErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			tmpDir := t.TempDir()

			err := os.WriteFile(filepath.Join(tmpDir, "server.crt"), currentCertPEM, 0o600)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(tmpDir, "server.key"), currentKeyPEM, 0o600)
			require.NoError(t, err)

			env := &envMock.EnvironmentMock{
				ClusteredFunc: func() bool { return false },
				VarDirFunc: func() string {
					return tmpDir
				},
				CacheDirFunc: func() string {
					return tmpDir
				},
				IsIncusOSFunc: func() bool {
					return true
				},
			}

			serverSvc := &mock.ProvisioningServerServiceMock{
				GetAllFunc: func(ctx context.Context) (provisioning.Servers, error) {
					return nil, nil
				},
				GetAllWithFilterFunc: func(ctx context.Context, filter provisioning.ServerFilter) (provisioning.Servers, error) {
					return provisioning.Servers{
						{
							Name: "operations-center",
							Type: api.ServerTypeOperationsCenter,
						},
					}, tc.serverGetAllWithFilterErr
				},
			}

			systemSvc := system.NewSystemService(
				env,
				serverSvc,
				nil,
				system.WithACMEUpdateCertificateFunc(
					func(
						ctx context.Context,
						fsEnv interface {
							CacheDir() string
						},
						cfg systemapi.SecurityACME,
						current *x509.Certificate,
						force bool,
					) (*systemapi.CertificatePost, error) {
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
		networkConfig              systemapi.NetworkFullPut
		serverGetAll               provisioning.Servers
		serverGetAllErr            error
		serverGetSystemProvider    []queue.Item[provisioning.ServerSystemProvider]
		serverUpdateSystemProvider []queue.Item[struct{}]
		configSaveErr              error

		assertErr         require.ErrorAssertionFunc
		wantNetworkConfig systemapi.NetworkFull
	}{
		{
			name:          "success - empty",
			networkConfig: systemapi.NetworkFullPut{},

			assertErr:         require.NoError,
			wantNetworkConfig: systemapi.NetworkFull{},
		},
		{
			name: "success - OperationsCenterAddress change",
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
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
			wantNetworkConfig: systemapi.NetworkFull{
				Network: systemapi.Network{
					NetworkPut: systemapi.NetworkPut{
						RestServerAddress: "192.168.1.200:8443",
					},
				},
				NetworkGlobal: systemapi.NetworkGlobal{
					NetworkGlobalPut: systemapi.NetworkGlobalPut{
						OperationsCenterAddress: "https://new:8443/",
					},
				},
			},
		},
		{
			name: "success - OperationsCenterAddress change - system provider config not initialized",
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
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
			wantNetworkConfig: systemapi.NetworkFull{
				Network: systemapi.Network{
					NetworkPut: systemapi.NetworkPut{
						RestServerAddress: "192.168.1.200:8443",
					},
				},
				NetworkGlobal: systemapi.NetworkGlobal{
					NetworkGlobalPut: systemapi.NetworkGlobalPut{
						OperationsCenterAddress: "https://new:8443/",
					},
				},
			},
		},
		{
			name: "success - OperationsCenterAddress change - unregistered server skipped",
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
				},
			},
			serverGetAll: provisioning.Servers{
				{
					Name: "one",
					Type: api.ServerTypeIncus,
				},
				{
					Name:   "two",
					Type:   api.ServerTypeIncus,
					Status: api.ServerStatusUnregistered,
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
			wantNetworkConfig: systemapi.NetworkFull{
				Network: systemapi.Network{
					NetworkPut: systemapi.NetworkPut{
						RestServerAddress: "192.168.1.200:8443",
					},
				},
				NetworkGlobal: systemapi.NetworkGlobal{
					NetworkGlobalPut: systemapi.NetworkGlobalPut{
						OperationsCenterAddress: "https://new:8443/",
					},
				},
			},
		},
		{
			name: "error - NetworkSetDefaults",
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: ":::", // invalid
				},
			},

			assertErr: require.Error,
		},
		{
			name: "error - validation",
			networkConfig: systemapi.NetworkFullPut{
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: ":|\\", // invalid
				},
			},

			assertErr: require.Error,
		},
		{
			name:          "error - config.UpdateNetwork",
			networkConfig: systemapi.NetworkFullPut{},
			configSaveErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - repo.GetAll",
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
				},
			},
			serverGetAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
		{
			name: "error - OperationsCenterAddress change - server.GetSystemProvider",
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
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
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
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
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
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
			networkConfig: systemapi.NetworkFullPut{
				NetworkPut: systemapi.NetworkPut{
					RestServerAddress: "192.168.1.200:8443",
				},
				NetworkGlobalPut: systemapi.NetworkGlobalPut{
					OperationsCenterAddress: "https://new:8443/",
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
				ClusteredFunc: func() bool { return false },
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
			systemSvc := system.NewSystemService(nil, serverSvc, nil)

			// Run test
			err := systemSvc.UpdateNetworkConfig(t.Context(), tc.networkConfig)
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
			networkConfig := systemapi.NetworkFull{
				Network: systemapi.Network{
					NetworkPut: systemapi.NetworkPut{
						RestServerAddress: testingnet.LocalhostIP(t) + ":1234",
					},
				},
				NetworkGlobal: systemapi.NetworkGlobal{
					NetworkGlobalPut: systemapi.NetworkGlobalPut{
						OperationsCenterAddress: "https://someaddress:1234",
					},
				},
			}

			env := &envMock.EnvironmentMock{
				ClusteredFunc: func() bool { return false },
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			err := config.UpdateLocalNetwork(t.Context(), networkConfig.NetworkPut)
			require.NoError(t, err)

			err = config.UpdateNetwork(t.Context(), networkConfig.NetworkGlobalPut)
			require.NoError(t, err)

			systemSvc := system.NewSystemService(nil, nil, nil)

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
		securityConfig systemapi.Security

		assertErr          require.ErrorAssertionFunc
		wantSecurityConfig systemapi.Security
	}{
		{
			name: "success",
			securityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertFingerprints: []string{"foobar"},
				},
			},

			assertErr: require.NoError,
			wantSecurityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertFingerprints: []string{"foobar"},
				},
			},
		},
		{
			// The fingerprint derived from the certificate is not persisted, it is
			// only added to the trusted fingerprints in memory when the security
			// configuration is loaded.
			name: "success - trusted client certificate",
			securityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertificates: []string{testcert.ClientCertificate},
				},
			},

			assertErr: require.NoError,
			wantSecurityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertificates: []string{testcert.ClientCertificate},
				},
			},
		},
		{
			name: "error - invalid trusted client certificate",
			securityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertificates: []string{"not a certificate"}, // invalid
				},
			},

			assertErr: require.Error,
			wantSecurityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertFingerprints: []string{},
					TrustedTLSClientCertificates:     []string{},
					ACME: systemapi.SecurityACME{
						CAURL:               "https://acme-v02.api.letsencrypt.org/directory",
						Challenge:           "HTTP-01",
						Address:             ":80",
						ProviderEnvironment: []string{},
						ProviderResolvers:   []string{},
					},
				},
			},
		},
		{
			name: "error",
			securityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					OIDC: systemapi.SecurityOIDC{
						Issuer: ":|\\", // invalid
					},
				},
			},

			assertErr: require.Error,
			wantSecurityConfig: systemapi.Security{
				SecurityPut: systemapi.SecurityPut{
					TrustedTLSClientCertFingerprints: []string{},
					TrustedTLSClientCertificates:     []string{},
					ACME: systemapi.SecurityACME{
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
				ClusteredFunc: func() bool { return false },
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			systemSvc := system.NewSystemService(nil, nil, nil)

			// Run test
			err := systemSvc.UpdateSecurityConfig(t.Context(), tc.securityConfig.SecurityPut)
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
		securityConfig systemapi.SettingsFullPut

		assertErr          require.ErrorAssertionFunc
		wantSettingsConfig systemapi.SettingsFull
	}{
		{
			name: "success",
			securityConfig: systemapi.SettingsFullPut{
				SettingsPut: systemapi.SettingsPut{
					LogLevel: "INFO",
				},
			},

			assertErr: require.NoError,
			wantSettingsConfig: systemapi.SettingsFull{
				Settings: systemapi.Settings{
					SettingsPut: systemapi.SettingsPut{
						LogLevel: "INFO",
					},
				},
			},
		},
		{
			name: "error",
			securityConfig: systemapi.SettingsFullPut{
				SettingsPut: systemapi.SettingsPut{
					LogLevel: "invalid", // invalid log level
				},
			},

			assertErr: require.Error,
			wantSettingsConfig: systemapi.SettingsFull{
				Settings: systemapi.Settings{
					SettingsPut: systemapi.SettingsPut{
						LogLevel:          "",
						MetricsEnabled:    true,
						MetricsComponents: []string{"runtime"},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			env := &envMock.EnvironmentMock{
				ClusteredFunc: func() bool { return false },
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			systemSvc := system.NewSystemService(nil, nil, nil)

			// Run test
			err := systemSvc.UpdateSettingsConfig(t.Context(), tc.securityConfig)
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
		updatesConfig systemapi.Updates

		assertErr         require.ErrorAssertionFunc
		wantUpdatesConfig systemapi.Updates
	}{
		{
			name: "success",
			updatesConfig: systemapi.Updates{
				UpdatesPut: systemapi.UpdatesPut{
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
					UpdatesDefaultChannel: "stable",
					ServerDefaultChannel:  "stable",
				},
			},

			assertErr: require.NoError,
			wantUpdatesConfig: systemapi.Updates{
				UpdatesPut: systemapi.UpdatesPut{
					Source: "https://somesource:443",
				},
			},
		},
		{
			name: "error",
			updatesConfig: systemapi.Updates{
				UpdatesPut: systemapi.UpdatesPut{
					Source: ":|\\", // invalid
				},
			},

			assertErr: require.Error,
			wantUpdatesConfig: systemapi.Updates{
				UpdatesPut: systemapi.UpdatesPut{
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
				ClusteredFunc: func() bool { return false },
				IsIncusOSFunc: func() bool {
					return false
				},
			}

			config.InitTest(t, env, nil)
			systemSvc := system.NewSystemService(nil, nil, nil)

			// Run test
			err := systemSvc.UpdateUpdatesConfig(t.Context(), tc.updatesConfig.UpdatesPut)
			gotUpdatesConfig := systemSvc.GetUpdatesConfig(t.Context())

			// Assert
			tc.assertErr(t, err)
			require.Equal(t, tc.wantUpdatesConfig.Source, gotUpdatesConfig.Source)
		})
	}
}

func TestSystemService_CleanCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		cacheRepoCleanupAllErr error

		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "success",

			assertErr: require.NoError,
		},
		{
			name:                   "error - cacheRepo.CleanupAll",
			cacheRepoCleanupAllErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			cacheRepo := &repoMock.CacheRepoMock{
				CleanupAllFunc: func(ctx context.Context) error {
					return tc.cacheRepoCleanupAllErr
				},
			}

			systemSvc := system.NewSystemService(nil, nil, cacheRepo)

			// Run test
			err := systemSvc.CleanCache(t.Context())

			// Assert
			tc.assertErr(t, err)
		})
	}
}
