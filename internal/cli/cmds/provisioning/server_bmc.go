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
	// Server Power On
	serverBMCServerPowerOnCmd := cmdServerBMCServerPowerOn{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerPowerOnCmd.Command())

	// Server Power Off
	serverBMCServerPowerOffCmd := cmdServerBMCServerPowerOff{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerPowerOffCmd.Command())

	// Server Restart
	serverBMCServerRestartCmd := cmdServerBMCServerRestart{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerRestartCmd.Command())

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

// Power on server via BMC.
type cmdServerBMCServerPowerOn struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCServerPowerOn) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-power-on <name>"
	cmd.Short = "Power on a server via BMC"
	cmd.Long = `Description:
  Power on a server via BMC

  Triggers a server power on via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a power on")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerPowerOn) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCServerPowerOn) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerPowerOn(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Power off server via BMC.
type cmdServerBMCServerPowerOff struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCServerPowerOff) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-power-off <name>"
	cmd.Short = "Power off a server via BMC"
	cmd.Long = `Description:
  Power off a server via BMC

  Triggers a server power off via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a server power off")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerPowerOff) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCServerPowerOff) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerPowerOff(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Restart server via BMC.
type cmdServerBMCServerRestart struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCServerRestart) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-restart <name>"
	cmd.Short = "Restart a server via BMC"
	cmd.Long = `Description:
  Restart a server via BMC

  Triggers a server restart via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a server restart")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerRestart) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCServerRestart) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerRestart(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}
