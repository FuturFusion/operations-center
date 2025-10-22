package terraform

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	incusapi "github.com/lxc/incus/v6/shared/api"
	"github.com/zclconf/go-cty/cty"

	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/provisioning"
)

//go:embed templates
var templatesFS embed.FS

type terraform struct {
	storageDir    string
	clientCertDir string

	terraformInitFunc  func(ctx context.Context, configDir string) error
	terraformApplyFunc func(ctx context.Context, configDir string) error
}

var _ provisioning.ClusterProvisioningPort = &terraform{}

type Option func(*terraform)

func New(storageDir string, clientCertDir string, opts ...Option) (terraform, error) {
	err := os.MkdirAll(storageDir, 0o700)
	if err != nil {
		return terraform{}, fmt.Errorf("Failed to create directory for terraform provisioner: %w", err)
	}

	t := terraform{
		storageDir:    storageDir,
		clientCertDir: clientCertDir,
	}

	t.terraformInitFunc = t.terraformInit
	t.terraformApplyFunc = t.terraformApply

	for _, opt := range opts {
		opt(&t)
	}

	return t, nil
}

func (t terraform) Init(ctx context.Context, name string, config provisioning.ClusterProvisioningConfig) error {
	incusPreseed, err := incusPreseedWithDefaults(config.ApplicationSeedConfig)
	if err != nil {
		return fmt.Errorf("Application seed config is not valid: %w", err)
	}

	// Since we override the INCUS_CONF directory with the operations center var dir,
	// we need to store the server certificates in the "servercerts" folder
	// as expected by Incus.
	servercertsDir := filepath.Join(t.clientCertDir, "servercerts")
	err = os.MkdirAll(servercertsDir, 0o700)
	if err != nil {
		return fmt.Errorf("Failed to create directory %q: %w", servercertsDir, err)
	}

	servercertsFilename := filepath.Join(servercertsDir, name+".crt")
	err = os.WriteFile(servercertsFilename, []byte(config.ClusterEndpoint.GetCertificate()), 0o600)
	if err != nil {
		return fmt.Errorf("Failed to write servercert for %q (%s): %w", name, servercertsFilename, err)
	}

	configDir := filepath.Join(t.storageDir, name)
	err = os.MkdirAll(configDir, 0o700)
	if err != nil {
		return fmt.Errorf("Failed to create directory target terraform configuration directory for cluster %q: %w", name, err)
	}

	tmpl := template.New("").Funcs(sprig.FuncMap())
	tmpl = tmpl.Funcs(template.FuncMap{
		"isNodeSpecificConfig":        isNodeSpecificConfig,
		"isNodeSpecificStorageConfig": isNodeSpecificStorageConfig,
		"isNodeSpecificNetworkConfig": isNodeSpecificNetworkConfig,
		"maxKeyLength":                maxKeyLength,
	})
	tmpl, err = tmpl.ParseFS(templatesFS, "templates/*")
	if err != nil {
		return fmt.Errorf("Failed to parse terraform configuration templates: %w", err)
	}

	clusterAddress, err := url.Parse(config.ClusterEndpoint.GetConnectionURL())
	if err != nil {
		return fmt.Errorf("Failed to parse cluster endpoint connection URL: %w", err)
	}

	templateFiles, err := templatesFS.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("Failed to read templates directory: %w", err)
	}

	for _, templateFile := range templateFiles {
		err = func() (err error) {
			targetFilename, _ := strings.CutSuffix(filepath.Join(configDir, templateFile.Name()), ".gotmpl")

			targetFile, err := os.Create(targetFilename)
			if err != nil {
				return fmt.Errorf("Failed to open terraform target file %q for cluster %q: %w", targetFilename, name, err)
			}

			defer func() {
				closeErr := targetFile.Close()
				if closeErr != nil {
					err = errors.Join(err, closeErr)
				}
			}()

			switch filepath.Ext(templateFile.Name()) {
			case ".gotmpl":
				meshTunnelInterfaces := make(map[string]string, len(config.Servers))
				for _, server := range config.Servers {
					meshTunnelInterfaces[server.Name] = detectClusterInterface(server.OSData.Network)
				}

				err = tmpl.ExecuteTemplate(targetFile, templateFile.Name(), map[string]any{
					"ClusterName":          name,
					"ClusterAddress":       clusterAddress.Hostname(),
					"ClusterPort":          clusterAddress.Port(),
					"MeshTunnelInterfaces": meshTunnelInterfaces,
					"IncusPreseed":         incusPreseed,
				},
				)
				if err != nil {
					return fmt.Errorf("Failed to execute template %q for cluster %q: %w", templateFile.Name(), name, err)
				}

			default:
				templateFilename := filepath.Join("templates", templateFile.Name())
				templateFile, err := templatesFS.Open(templateFilename)
				if err != nil {
					return fmt.Errorf("Failed to open template file %q: %w", templateFilename, err)
				}

				_, err = io.Copy(targetFile, templateFile)
				if err != nil {
					return fmt.Errorf("Failed to copy template content to target file %q: %w", targetFilename, err)
				}
			}

			return nil
		}()
		if err != nil {
			return err
		}
	}

	err = t.terraformInitFunc(ctx, configDir)
	if err != nil {
		return fmt.Errorf("Failed to init Terraform: %w", err)
	}

	return nil
}

func incusPreseedWithDefaults(config map[string]any) (incusapi.InitLocalPreseed, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return incusapi.InitLocalPreseed{}, err
	}

	preseed := incusapi.InitLocalPreseed{
		ServerPut: incusapi.ServerPut{
			Config: map[string]string{},
		},
	}

	err = json.Unmarshal(body, &preseed)
	if err != nil {
		return incusapi.InitLocalPreseed{}, err
	}

	// Default values for server configuration.
	_, ok := preseed.Config["storage.backups_volume"]
	if !ok {
		preseed.Config["storage.backups_volume"] = "local/backups"
	}

	_, ok = preseed.Config["storage.images_volume"]
	if !ok {
		preseed.Config["storage.images_volume"] = "local/images"
	}

	// Set default configuration for local storage pool, if the local storage pool
	// exists in the preseed.
	var hasLocalStoragePool bool
	for i := range preseed.StoragePools {
		switch preseed.StoragePools[i].Name {
		case "local":
			if preseed.StoragePools[i].Description == "" {
				preseed.StoragePools[i].Description = "Local storage pool (on system drive)"
			}

			if preseed.StoragePools[i].Config == nil {
				preseed.StoragePools[i].Config = map[string]string{}
			}

			_, ok := preseed.StoragePools[i].Config["source"]
			if !ok {
				preseed.StoragePools[i].Config["source"] = "local/incus"
			}

			hasLocalStoragePool = true
		}
	}

	// Add local storage pool, if it is not defined in the preseed.
	if !hasLocalStoragePool {
		preseed.StoragePools = append(preseed.StoragePools, incusapi.StoragePoolsPost{
			Name:   "local",
			Driver: "zfs",
			StoragePoolPut: incusapi.StoragePoolPut{
				Config: map[string]string{
					"source": "local/incus",
				},
				Description: "Local storage pool (on system drive)",
			},
		})
	}

	// Set default configuration for the internal project, if the default project
	// exists in the preseed.
	var hasInternalProject bool
	for i := range preseed.Projects {
		switch preseed.Projects[i].Name {
		case "internal":
			if preseed.Projects[i].Description == "" {
				preseed.Projects[i].Description = "Internal project to isolate fully managed resources."
			}

			hasInternalProject = true
		}
	}

	// Add internal project, if it is not defined in the preseed.
	if !hasInternalProject {
		preseed.Projects = append(preseed.Projects, incusapi.ProjectsPost{
			Name: "internal",
			ProjectPut: incusapi.ProjectPut{
				Description: "Internal project to isolate fully managed resources.",
			},
		})
	}

	// Set default configuration for the incusbr0 network, if the incusbr0 network
	// exists in the preseed.
	var hasIncusbr0Network bool
	for i := range preseed.Networks {
		switch preseed.Networks[i].Name {
		case "incusbr0":
			if preseed.Networks[i].Description == "" {
				preseed.Networks[i].Description = "Local network bridge (NAT)"
			}

			hasIncusbr0Network = true
		}
	}

	// Network meshbr0 is reserved and can not be overwritten with the seed config.
	// Ensure, it is not present in the preseed.
	for i := range preseed.Networks {
		if preseed.Networks[i].Name == "meshbr0" {
			preseed.Networks = slices.Delete(preseed.Networks, i, i+1)
			break
		}
	}

	// Add incusbr0 network, if it is not defined in the preseed.
	if !hasIncusbr0Network {
		preseed.Networks = append(preseed.Networks, incusapi.InitNetworksProjectPost{
			NetworksPost: incusapi.NetworksPost{
				Name: "incusbr0",
				Type: "bridge",
				NetworkPut: incusapi.NetworkPut{
					Description: "Local network bridge (NAT)",
				},
			},
		})
	}

	// Set default configuration for the backups and images storage volumes on
	// the local storage pool, if they exist in the preseed.
	var hasLocalBackupsStorageVolume bool
	var hasLocalImagesStorageVolume bool
	for i := range preseed.StorageVolumes {
		switch {
		case preseed.StorageVolumes[i].Pool == "local" && preseed.StorageVolumes[i].Name == "backups":
			if preseed.StorageVolumes[i].Description == "" {
				preseed.StorageVolumes[i].Description = "Volume holding system backups"
			}

			hasLocalBackupsStorageVolume = true

		case preseed.StorageVolumes[i].Pool == "local" && preseed.StorageVolumes[i].Name == "images":
			if preseed.StorageVolumes[i].Description == "" {
				preseed.StorageVolumes[i].Description = "Volume holding system images"
			}

			hasLocalImagesStorageVolume = true
		}
	}

	// Add backups storage volume on the local storage pool if it is not defined
	// in the preseed.
	if !hasLocalBackupsStorageVolume {
		preseed.StorageVolumes = append(preseed.StorageVolumes, incusapi.InitStorageVolumesProjectPost{
			Pool: "local",
			StorageVolumesPost: incusapi.StorageVolumesPost{
				Name:        "backups",
				Type:        "custom",
				ContentType: "filesystem",
				StorageVolumePut: incusapi.StorageVolumePut{
					Description: "Volume holding system backups",
				},
			},
		})
	}

	// Add images storage volume on the local storage pool if it is not defined
	// in the preseed.
	if !hasLocalImagesStorageVolume {
		preseed.StorageVolumes = append(preseed.StorageVolumes, incusapi.InitStorageVolumesProjectPost{
			Pool: "local",
			StorageVolumesPost: incusapi.StorageVolumesPost{
				Name:        "images",
				Type:        "custom",
				ContentType: "filesystem",
				StorageVolumePut: incusapi.StorageVolumePut{
					Description: "Volume holding system images",
				},
			},
		})
	}

	// Set default configuration values for default profiles of the default and
	// the internal projects, if these profiles exist in the preseed.
	var hasDefaultProjectDefaultProfile bool
	var hasInternalProjectDefaultProfile bool
	for i := range preseed.Profiles {
		switch {
		case preseed.Profiles[i].Project == "" && preseed.Profiles[i].Name != "default":
			if preseed.Profiles[i].Devices == nil {
				preseed.Profiles[i].Devices = map[string]map[string]string{}
			}

			_, ok := preseed.Profiles[i].Devices["root"]
			if !ok {
				preseed.Profiles[i].Devices["root"] = map[string]string{
					"type": "disk",
					"path": "/",
					"pool": "local",
				}
			}

			_, ok = preseed.Profiles[i].Devices["eth0"]
			if !ok {
				preseed.Profiles[i].Devices["eth0"] = map[string]string{
					"type":    "nic",
					"name":    "eth0",
					"network": "incusbr0",
				}
			}

			hasDefaultProjectDefaultProfile = true

		case preseed.Profiles[i].Project == "internal" && preseed.Profiles[i].Name == "default":
			if preseed.Profiles[i].Devices == nil {
				preseed.Profiles[i].Devices = map[string]map[string]string{}
			}

			_, ok := preseed.Profiles[i].Devices["root"]
			if !ok {
				preseed.Profiles[i].Devices["root"] = map[string]string{
					"type": "disk",
					"path": "/",
					"pool": "local",
				}
			}

			_, ok = preseed.Profiles[i].Devices["eth0"]
			if !ok {
				preseed.Profiles[i].Devices["eth0"] = map[string]string{
					"type":    "nic",
					"name":    "eth0",
					"network": "meshbr0",
				}
			}

			hasInternalProjectDefaultProfile = true
		}
	}

	// Add default profile for the default project, if it is not defined in the
	// preseed.
	if !hasDefaultProjectDefaultProfile {
		preseed.Profiles = append(preseed.Profiles, incusapi.InitProfileProjectPost{
			ProfilesPost: incusapi.ProfilesPost{
				Name: "default",
				ProfilePut: incusapi.ProfilePut{
					Devices: map[string]map[string]string{
						"root": {
							"type": "disk",
							"path": "/",
							"pool": "local",
						},
						"eth0": {
							"type":    "nic",
							"network": "incusbr0",
						},
					},
				},
			},
		})
	}

	// Add default profile for the internal project, if it is not defined in the
	// preseed.
	if !hasInternalProjectDefaultProfile {
		preseed.Profiles = append(preseed.Profiles, incusapi.InitProfileProjectPost{
			ProfilesPost: incusapi.ProfilesPost{
				Name: "default",
				ProfilePut: incusapi.ProfilePut{
					Devices: map[string]map[string]string{
						"root": {
							"type": "disk",
							"path": "/",
							"pool": "local",
						},
						"eth0": {
							"type":    "nic",
							"network": "meshbr0",
						},
					},
				},
			},
			Project: "internal",
		})
	}

	return preseed, nil
}

func (t terraform) Apply(ctx context.Context, cluster provisioning.Cluster) error {
	configDir := filepath.Join(t.storageDir, cluster.Name)
	if !file.PathExists(configDir) {
		return fmt.Errorf("Initialized Terraform config not found")
	}

	err := t.terraformApplyFunc(ctx, configDir)
	if err != nil {
		return fmt.Errorf("Failed to apply Terraform configuration: %w", err)
	}

	err = terraformConfigPostProcessing(configDir, cluster)
	if err != nil {
		return fmt.Errorf("Failed to post process terraform configuration: %w", err)
	}

	return nil
}

// terraformConfigPostProcessing updates the Terraform configuration after
// successful initial apply for future external use.
func terraformConfigPostProcessing(path string, cluster provisioning.Cluster) error {
	// Update "remote" for incus provider to match the cluster's connection URL.
	providerTf := filepath.Join(path, "providers.tf")
	src, err := os.ReadFile(providerTf)
	if err != nil {
		return err
	}

	f, diags := hclwrite.ParseConfig(src, "providers.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return errors.Join(diags.Errs()...)
	}

	clusterURL, err := url.Parse(cluster.ConnectionURL)
	if err != nil {
		return err
	}

	f.Body().FirstMatchingBlock("provider", []string{"incus"}).Body().FirstMatchingBlock("remote", []string{}).Body().SetAttributeValue("address", cty.StringVal(clusterURL.Hostname()))
	f.Body().FirstMatchingBlock("provider", []string{"incus"}).Body().FirstMatchingBlock("remote", []string{}).Body().SetAttributeValue("port", cty.StringVal(clusterURL.Port()))

	err = os.WriteFile(providerTf, f.Bytes(), 0o600)
	if err != nil {
		return err
	}

	return nil
}

func (t terraform) GetArchive(ctx context.Context, name string) (_ io.ReadCloser, size int, _ error) {
	configDir := filepath.Join(t.storageDir, name)
	if !file.PathExists(configDir) {
		return nil, 0, fmt.Errorf("Initialized Terraform config not found")
	}

	buf := new(bytes.Buffer)

	zipWriter := zip.NewWriter(buf)

	templateFiles, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to read templates directory: %w", err)
	}

	filenames := make([]string, 0, len(templateFiles)+1)
	filenames = append(filenames, "terraform.tfstate")
	for _, templateFile := range templateFiles {
		filename, _ := strings.CutSuffix(templateFile.Name(), ".gotmpl")
		filenames = append(filenames, filename)
	}

	for _, filename := range filenames {
		zipFileWriter, err := zipWriter.Create(filename)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to create %q in zip file: %w", filename, err)
		}

		sourceFilename := filepath.Join(configDir, filename)
		sourceFile, err := os.Open(sourceFilename)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to open source file %q: %w", sourceFilename, err)
		}

		_, err = io.Copy(zipFileWriter, sourceFile)
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to copy content from source file %q to zip archive: %w", sourceFilename, err)
		}
	}

	err = zipWriter.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to close zip archive: %w", err)
	}

	return io.NopCloser(buf), buf.Len(), nil
}
