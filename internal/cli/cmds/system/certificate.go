package system

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type CmdCertificate struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdCertificate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "certificate"
	cmd.Short = "Interact with certificate config"
	cmd.Long = `Description:
  Interact with certificate config

  Show and set server certificate for operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Show
	certificateShowCmd := cmdCertificateShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(certificateShowCmd.Command())

	// Set
	certificateSetCmd := cmdCertificateSet{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(certificateSetCmd.Command())

	return cmd
}

// Show system server certificate.
type cmdCertificateShow struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdCertificateShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show"
	cmd.Short = "Show server certificate"
	cmd.Long = `Description:
  Show server certificate

  The corresponding private key is never shown.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdCertificateShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	validFormats := []string{"", "json", "yaml"}
	if !slices.Contains(validFormats, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdCertificateShow) run(cmd *cobra.Command, args []string) error {
	certificate, err := c.ocClient.GetSystemCertificate(cmd.Context())
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent("", "  ")
		err = enc.Encode(certificate)

	default:
		enc := yaml.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent(2)
		err = enc.Encode(certificate)
	}

	return err
}

// Set system server certificate.
type cmdCertificateSet struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdCertificateSet) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "set <server.crt> <server.key>"
	cmd.Short = "Set server certificate"
	cmd.Long = `Description:
  Set server certificate
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdCertificateSet) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdCertificateSet) run(cmd *cobra.Command, args []string) error {
	certificateFilename := args[0]
	keyFilename := args[1]

	certificatePEM, err := os.ReadFile(certificateFilename)
	if err != nil {
		return err
	}

	keyPEM, err := os.ReadFile(keyFilename)
	if err != nil {
		return err
	}

	certificateRequest := system.CertificatePost{
		Certificate: string(certificatePEM),
		Key:         string(keyPEM),
	}

	err = c.ocClient.SetSystemCertificate(cmd.Context(), certificateRequest)
	if err != nil {
		return err
	}

	return nil
}
