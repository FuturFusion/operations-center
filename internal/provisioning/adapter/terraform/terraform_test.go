package terraform_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/provisioning/adapter/terraform"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/testing/boom"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Run "go test github.com/FuturFusion/operations-center/internal/provisioning/adapter/terraform/ -update-goldenfiles" to update the golden files automatically.
var updateGoldenfiles = flag.Bool("update-goldenfiles", false, "golden files are updated, if this flag is provided")

func TestTerraform_Init(t *testing.T) {
	tests := []struct {
		name             string
		clusterName      string
		terraformInitErr error

		assertErr         require.ErrorAssertionFunc
		wantTemporaryPath string
	}{
		{
			name:        "success",
			clusterName: "foobar",

			assertErr:         require.NoError,
			wantTemporaryPath: "foobar",
		},
		{
			name:             "error - terraform init",
			clusterName:      "foobar",
			terraformInitErr: boom.Error,

			assertErr: boom.ErrorIs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()

			tf, err := terraform.New(tmpDir, tmpDir, terraform.WithTerraformInitFunc(func(ctx context.Context, configDir string) error {
				return tc.terraformInitErr
			}))
			require.NoError(t, err)

			const applicationConfig = `---
config:
  user.ui.sso_only: "true"
  storage.images_volume: shared
storage_pools:
  - name: shared
    driver: lvmcluster
    description: Shared storage pool (lvmcluster)
    config:
      lvm.vg_name: vg0
      source: /dev/sda
certificates:
  - name: cert1
    description: metrics certificate 1
    type: metrics
    restricted: true
    projects:
      - project1
      - project2
    certificate: |-
      -----BEGIN CERTIFICATE-----
      MIIB1jCCAVygAwIBAgIQaKBbJqVWID8NqSoMxF/nHzAKBggqhkjOPQQDAzAzMRkw
      FwYDVQQKExBMaW51eCBDb250YWluZXJzMRYwFAYDVQQDDA1sdWJyQHN1cnZpc3Rh
      MB4XDTI1MDIxMjE0MTEzMVoXDTM1MDIxMDE0MTEzMVowMzEZMBcGA1UEChMQTGlu
      dXggQ29udGFpbmVyczEWMBQGA1UEAwwNbHVickBzdXJ2aXN0YTB2MBAGByqGSM49
      AgEGBSuBBAAiA2IABDXH+i9i6WilQA56Qe4wvTGZL1NGDeGZFCCskJduZietB0bX
      K30ug6JdxUHGfhg3CL92lTnmtMwJC+Ev+IQFhLGv/Yk/OLP4BB1zdqBgmyA7Mmwq
      jcrp8B8FTBZ9AQmCe6M1MDMwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsG
      AQUFBwMCMAwGA1UdEwEB/wQCMAAwCgYIKoZIzj0EAwMDaAAwZQIxAPmS67jexjgT
      6PrxAo/fQpK71BwgpsHOCZHM2b3t4lZlDirjN40xNGPeNH+KG95R3wIwexlentZZ
      0x2N/SJBYGltBnBjH8mm8OTWa1N/MpOAl2K7jRVuSeuWGBDf0/n+M6br
      -----END CERTIFICATE-----
cluster_groups:
  - name: cluster_group1
    description: cluster group 1
    config:
      key: value
      other_key: other_value
    members:
      - server1
      - server2
`

			applicationSeedConfig := map[string]any{}
			err = yaml.Unmarshal([]byte(applicationConfig), &applicationSeedConfig)
			require.NoError(t, err)

			// Run tests
			temporaryPath, cleanup, err := tf.Init(t.Context(), tc.clusterName, provisioning.ClusterProvisioningConfig{
				Servers: []provisioning.Server{
					{
						Name: "server-1",
						OSData: api.OSData{
							Network: incusosapi.SystemNetwork{
								State: incusosapi.SystemNetworkState{
									Interfaces: map[string]incusosapi.SystemNetworkInterfaceState{
										"enp5s0": {
											Roles:     []string{"cluster"},
											Addresses: []string{"1.2.3.4"},
										},
									},
								},
							},
						},
					},
				},
				ClusterEndpoint: provisioning.ClusterEndpoint{
					provisioning.Server{
						ConnectionURL:      "https://127.0.0.1:8443",
						Cluster:            ptr.To("cluster"),
						ClusterCertificate: ptr.To("cluster certificate"),
					},
				},

				Cluster: provisioning.Cluster{
					ID:                    1,
					ApplicationSeedConfig: applicationSeedConfig,
				},
			})

			// Assert
			tc.assertErr(t, err)
			if err == nil {
				defer func() {
					err := cleanup()
					require.NoError(t, err)
				}()

				require.Contains(t, temporaryPath, tmpDir)
				require.Contains(t, temporaryPath, tc.wantTemporaryPath)
			}

			fileContains(t, filepath.Join(tmpDir, "servercerts", tc.clusterName+".crt"), "cluster certificate")

			require.FileExists(t, filepath.Join(tmpDir, tc.clusterName, "data_cluster.tf"))

			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "providers.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_certificates.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_cluster_groups.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_networks.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_profiles.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_projects.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_server.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_storage_pools.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_storage_volumes.tf")
		})
	}
}

func fileContains(t *testing.T, filename string, contains ...string) {
	t.Helper()

	require.FileExists(t, filename)

	body, err := os.ReadFile(filename)
	require.NoError(t, err)
	for _, contain := range contains {
		require.Contains(t, string(body), contain)
	}
}

func fileMatch(t *testing.T, path string, name string) {
	t.Helper()

	filename := filepath.Join(path, name)

	require.FileExists(t, filename)

	body, err := os.ReadFile(filename)
	require.NoError(t, err)

	goldenFilename := filepath.Join("./testdata", name)
	if *updateGoldenfiles {
		err := os.WriteFile(goldenFilename, body, 0o600)
		require.NoError(t, err)
	}

	want, err := os.ReadFile(goldenFilename)
	require.NoError(t, err)

	require.Equal(t, string(want), string(body))
}

func TestTerraform_Apply(t *testing.T) {
	noopAssertPostProcessedFiles := func(*testing.T, string, string) {}

	tests := []struct {
		name                 string
		clusterConnectionURL string
		setup                func(t *testing.T, configDir string)
		terraformApplyErr    error

		assertErr                require.ErrorAssertionFunc
		assertPostProcessedFiles func(t *testing.T, dir string, clusterName string)
	}{
		{
			name:                 "success",
			clusterConnectionURL: "https://localhost:8443",
			setup: func(t *testing.T, configDir string) {
				t.Helper()

				err := os.MkdirAll(configDir, 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(configDir, "providers.tf"), []byte(`provider "incus" {
  default_remote = "mycluster"
  remote {
    name    = "mycluster"
    address = "https://some-host:1234"
  }
}`), 0o600)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertPostProcessedFiles: func(t *testing.T, dir, clusterName string) {
				t.Helper()

				fileContains(t, filepath.Join(dir, clusterName, "providers.tf"),
					`"https://localhost:8443"`,
				)
			},
		},
		{
			name: "error - config directory not initialized",
			setup: func(t *testing.T, configDir string) {
				t.Helper()

				// config directory not created.
			},

			assertErr: func(tt require.TestingT, err error, a ...any) {
				require.ErrorContains(tt, err, "Initialized Terraform config not found")
			},
			assertPostProcessedFiles: noopAssertPostProcessedFiles,
		},
		{
			name: "error - terraform apply",
			setup: func(t *testing.T, configDir string) {
				t.Helper()

				err := os.MkdirAll(configDir, 0o700)
				require.NoError(t, err)
			},
			terraformApplyErr: boom.Error,

			assertErr:                boom.ErrorIs,
			assertPostProcessedFiles: noopAssertPostProcessedFiles,
		},
		{
			name: "error - providers.tf not found",
			setup: func(t *testing.T, configDir string) {
				t.Helper()

				err := os.MkdirAll(configDir, 0o700)
				require.NoError(t, err)
			},

			assertErr:                require.Error,
			assertPostProcessedFiles: noopAssertPostProcessedFiles,
		},
		{
			name: "error - providers.tf invalid Terraform config",
			setup: func(t *testing.T, configDir string) {
				t.Helper()

				err := os.MkdirAll(configDir, 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(configDir, "providers.tf"), []byte(`provider "incus" {`), 0o600) // invalid Terraform configuration.
				require.NoError(t, err)
			},

			assertErr:                require.Error,
			assertPostProcessedFiles: noopAssertPostProcessedFiles,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()
			clusterName := "foobar"
			tc.setup(t, filepath.Join(tmpDir, clusterName))

			tf, err := terraform.New(tmpDir, tmpDir, terraform.WithTerraformApplyFunc(func(ctx context.Context, configDir string) error {
				return tc.terraformApplyErr
			}))
			require.NoError(t, err)

			// Run tests
			cluster := provisioning.Cluster{
				Name:          clusterName,
				ConnectionURL: tc.clusterConnectionURL,
			}

			err = tf.Apply(t.Context(), cluster)

			// Assert
			tc.assertErr(t, err)
			tc.assertPostProcessedFiles(t, tmpDir, clusterName)
		})
	}
}
