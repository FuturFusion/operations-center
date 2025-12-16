package system

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
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

  Set server certificate for operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Set
	certificateSetCmd := cmdCertificateSet{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(certificateSetCmd.Command())

	return cmd
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

	certificateRequest := api.SystemCertificatePost{
		Certificate: string(certificatePEM),
		Key:         string(keyPEM),
	}

	err = c.ocClient.SetSystemCertificate(cmd.Context(), certificateRequest)
	if err != nil {
		return err
	}

	return nil
}
