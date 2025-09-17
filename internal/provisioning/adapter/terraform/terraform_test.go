package terraform_test

import (
	"archive/zip"
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsnet/golib/memfile"
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

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:        "success",
			clusterName: "foobar",

			assertErr: require.NoError,
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
`

			applicationSeedConfig := map[string]any{}
			err = yaml.Unmarshal([]byte(applicationConfig), &applicationSeedConfig)
			require.NoError(t, err)

			// Run tests
			err = tf.Init(t.Context(), tc.clusterName, provisioning.ClusterProvisioningConfig{
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
						ClusterCertificate: ptr.To("cluster certificate"),
					},
				},

				ApplicationSeedConfig: applicationSeedConfig,
			})

			// Assert
			tc.assertErr(t, err)

			fileContains(t, filepath.Join(tmpDir, "servercerts", tc.clusterName+".crt"), "cluster certificate")

			require.FileExists(t, filepath.Join(tmpDir, tc.clusterName, "data_cluster.tf"))
			require.FileExists(t, filepath.Join(tmpDir, tc.clusterName, "resources_network.tf"))
			require.FileExists(t, filepath.Join(tmpDir, tc.clusterName, "resources_project_internal.tf"))
			require.FileExists(t, filepath.Join(tmpDir, tc.clusterName, "resources_storage_pool_local.tf"))

			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "providers.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_network_locals.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_profiles.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_server.tf")
			fileMatch(t, filepath.Join(tmpDir, tc.clusterName), "resources_storage_pools.tf")
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
  remote {
    name    = "mycluster"
    default = true
    scheme  = "https"
    address = "some-host"
    port    = "1234"
  }
}`), 0o600)
				require.NoError(t, err)
			},

			assertErr: require.NoError,
			assertPostProcessedFiles: func(t *testing.T, dir, clusterName string) {
				t.Helper()

				fileContains(t, filepath.Join(dir, clusterName, "providers.tf"),
					`"localhost"`,
					`"8443"`,
					`"https"`,
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
		{
			name:                 "error - invalid cluster.connectionURL",
			clusterConnectionURL: ":|\\", // invalid
			setup: func(t *testing.T, configDir string) {
				t.Helper()

				err := os.MkdirAll(configDir, 0o700)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(configDir, "providers.tf"), []byte(`provider "incus" {
  remote {
    name    = "mycluster"
    default = true
    scheme  = "https"
    address = "some-host"
    port    = "1234"
  }
}`), 0o600)
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

func TestTerraform_GetArchive(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string

		assertErr require.ErrorAssertionFunc
	}{
		{
			name:        "success",
			clusterName: "foobar",

			assertErr: require.NoError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			tmpDir := t.TempDir()

			tf, err := terraform.New(tmpDir, tmpDir, terraform.WithTerraformInitFunc(func(ctx context.Context, configDir string) error {
				err := os.WriteFile(filepath.Join(configDir, "terraform.tfstate"), []byte("would be written by terraform"), 0o600)
				require.NoError(t, err)

				return nil
			}))
			require.NoError(t, err)

			err = tf.Init(t.Context(), "foobar", provisioning.ClusterProvisioningConfig{
				Servers: []provisioning.Server{
					{
						Name: "server-1",
					},
				},
				ClusterEndpoint: provisioning.ClusterEndpoint{
					provisioning.Server{
						ConnectionURL:      "https://127.0.0.1:8443",
						ClusterCertificate: ptr.To("cluster certificate"),
					},
				},
			})
			require.NoError(t, err)

			// Run tests
			rc, size, err := tf.GetArchive(t.Context(), "foobar")
			if err == nil {
				defer rc.Close()
			}

			// Assert
			tc.assertErr(t, err)
			require.Greater(t, size, 3000) // We don't know the exact size of the zip archive and it migth change over time, so just make sure, it is a reasonable value.

			buf := bytes.Buffer{}

			n, err := io.Copy(&buf, rc)
			require.NoError(t, err)
			require.Equal(t, int64(size), n)

			zipFile := memfile.New(buf.Bytes())

			zr, err := zip.NewReader(zipFile, int64(size))
			require.NoError(t, err)

			expectedFilesFound := map[string]bool{
				"data_cluster.tf":                 false,
				"providers.tf":                    false,
				"resources_network_locals.tf":     false,
				"resources_network.tf":            false,
				"resources_profiles.tf":           false,
				"resources_project_internal.tf":   false,
				"resources_server.tf":             false,
				"resources_storage_pool_local.tf": false,
				"resources_storage_pools.tf":      false,
				"terraform.tfstate":               false,
			}

			for _, file := range zr.File {
				found, ok := expectedFilesFound[file.Name]
				require.True(t, ok, "unexpected file %q found in zip archive", file.Name)
				require.False(t, found, "file %q has already been seen", file.Name)

				expectedFilesFound[file.Name] = true
			}

			for filename, found := range expectedFilesFound {
				require.True(t, found, "file %q not found in zip archive", filename)
			}
		})
	}
}
