package provisioning

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
)

// Configure server system.
type cmdServerSystem struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerSystem) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "system"
	cmd.Short = "Interact with system configuration of servers"
	cmd.Long = `Description:
  Interact with system configuration of servers

  Configure system of servers.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// System Network
	serverSystemNetworkCmd := cmdServerSystemNetwork{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverSystemNetworkCmd.Command())

	// System Reboot
	serverRebootCmd := cmdServerReboot{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverRebootCmd.Command())

	// System Storage
	serverSystemStorageCmd := cmdServerSystemStorage{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverSystemStorageCmd.Command())

	return cmd
}

// Reboot server.
type cmdServerReboot struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerReboot) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "reboot <name>"
	cmd.Short = "Reboot a server"
	cmd.Long = `Description:
  Reboot a server

  Reboots a server.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerReboot) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerReboot) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.RebootServerSystem(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}
