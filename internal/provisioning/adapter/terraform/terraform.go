package terraform

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"

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
	// Since we override the INCUS_CONF directory with the operations center var dir,
	// we need to store the server certificates in the "servercerts" folder
	// as expected by Incus.
	servercertsDir := filepath.Join(t.clientCertDir, "servercerts")
	err := os.MkdirAll(servercertsDir, 0o700)
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

	tmpl, err := template.ParseFS(templatesFS, "templates/*")
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
					"StoragePools":         config.Config.StoragePools,
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

func (t terraform) Apply(ctx context.Context, name string) error {
	configDir := filepath.Join(t.storageDir, name)
	if !file.PathExists(configDir) {
		return fmt.Errorf("Initialized Terraform config not found")
	}

	err := t.terraformApplyFunc(ctx, configDir)
	if err != nil {
		return fmt.Errorf("Failed to apply Terraform configuration: %w", err)
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
