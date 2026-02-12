package provisioning

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
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

	// Evacuate
	serverEvacuateCmd := cmdServerEvacuate{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverEvacuateCmd.Command())

	// Poweroff
	serverPoweroffCmd := cmdServerPoweroff{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverPoweroffCmd.Command())

	// System Reboot
	serverRebootCmd := cmdServerReboot{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverRebootCmd.Command())

	// Restore
	serverRestoreCmd := cmdServerRestore{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverRestoreCmd.Command())

	// System Update
	serverUpdateCmd := cmdServerUpdate{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverUpdateCmd.Command())

	// System Storage
	serverSystemStorageCmd := cmdServerSystemStorage{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverSystemStorageCmd.Command())

	return cmd
}

// Evacuate server.
type cmdServerEvacuate struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerEvacuate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "evacuate <name>"
	cmd.Short = "Evacuate a server"
	cmd.Long = `Description:
  Evacuate a server

  Evacuates a server.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerEvacuate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerEvacuate) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.EvacuateServerSystem(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Poweroff server.
type cmdServerPoweroff struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerPoweroff) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "poweroff <name>"
	cmd.Short = "Poweroff a server"
	cmd.Long = `Description:
  Poweroff a server

  Powers off a server.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerPoweroff) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerPoweroff) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.PoweroffServerSystem(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
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

// Restore server.
type cmdServerRestore struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerRestore) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "restore <name>"
	cmd.Short = "Restore a server"
	cmd.Long = `Description:
  Restore a server

  Restores a server.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerRestore) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerRestore) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.RestoreServerSystem(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Update server.
type cmdServerUpdate struct {
	ocClient *client.OperationsCenterClient

	flagUpdateOS bool
}

func (c *cmdServerUpdate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update <name>"
	cmd.Short = "Update a server"
	cmd.Long = `Description:
  Update a server

  Triggers an update on a server.
`

	cmd.Flags().BoolVar(&c.flagUpdateOS, "os", false, "trigger OS update")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerUpdate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerUpdate) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	updateRequest := api.ServerUpdatePost{
		OS: api.ServerUpdateApplication{
			Name:          "os",
			TriggerUpdate: c.flagUpdateOS,
		},
	}

	err := c.ocClient.UpdateServerSystem(cmd.Context(), name, updateRequest)
	if err != nil {
		return err
	}

	return nil
}
