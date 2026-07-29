package provisioning

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
)

// Interact with BMC of servers.
type cmdServerBMC struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMC) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "bmc"
	cmd.Short = "Interact with the BMC of servers"
	cmd.Long = `Description:
  Interact with the BMC of servers.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Refresh
	serverBMCDataRefreshCmd := cmdServerBMCDataRefresh{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCDataRefreshCmd.Command())

	return cmd
}

// Refresh server's BMC data.
type cmdServerBMCDataRefresh struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCDataRefresh) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "refresh <name>"
	cmd.Short = "Refresh a server's BMC data"
	cmd.Long = `Description:
  Refresh a server's BMC data.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCDataRefresh) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCDataRefresh) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCDataRefresh(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}
