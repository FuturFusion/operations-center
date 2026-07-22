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

	// Start
	serverBMCStartCmd := cmdServerBMCStart{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCStartCmd.Command())

	// Stop
	serverBMCStopCmd := cmdServerBMCStop{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCStopCmd.Command())

	// Restart
	serverBMCRestartCmd := cmdServerBMCRestart{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCRestartCmd.Command())

	return cmd
}

// Start server via BMC.
type cmdServerBMCStart struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCStart) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "start <name>"
	cmd.Short = "Start a server via BMC"
	cmd.Long = `Description:
  Start a server via BMC

  Triggers a server start via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a start")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCStart) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCStart) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCStartServer(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Stop server via BMC.
type cmdServerBMCStop struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCStop) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "stop <name>"
	cmd.Short = "Stop a server via BMC"
	cmd.Long = `Description:
  Stop a server via BMC

  Triggers a server stop via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a stop")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCStop) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCStop) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCStopServer(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Restart server via BMC.
type cmdServerBMCRestart struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCRestart) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "restart <name>"
	cmd.Short = "Restart a server via BMC"
	cmd.Long = `Description:
  Restart a server via BMC

  Triggers a server restart via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a restart")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCRestart) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCRestart) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCRestartServer(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}
