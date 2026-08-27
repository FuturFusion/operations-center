package provisioning

import (
	"fmt"

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

	// Server locate
	serverBMCServerLocateCmd := cmdServerBMCServerLocate{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerLocateCmd.Command())

	// BIOS attributes
	serverBMCBIOSAttributesCmd := cmdServerBMCBIOSAttributes{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCBIOSAttributesCmd.Command())

	// Apply secure boot certificates
	serverBMCApplySecureBootCertificatesCmd := cmdServerBMCApplySecureBootCertificates{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCApplySecureBootCertificatesCmd.Command())

	// Logs
	serverBMCLogsCmd := cmdServerBMCLogs{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCLogsCmd.Command())

	// Log entries
	serverBMCLogEntriesCmd := cmdServerBMCLogEntries{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCLogEntriesCmd.Command())

	// Dump
	serverBMCDumpCmd := cmdServerBMCDump{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCDumpCmd.Command())

	// Attach media
	serverBMCAttachMediaCmd := cmdServerBMCAttachMedia{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCAttachMediaCmd.Command())

	// Detach media
	serverBMCDetachMediaCmd := cmdServerBMCDetachMedia{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCDetachMediaCmd.Command())

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

// Set the state of the location indicator LED of a server via BMC.
type cmdServerBMCServerLocate struct {
	ocClient *client.OperationsCenterClient
}

const (
	locationIndicatorStateOn  = "on"
	locationIndicatorStateOff = "off"
)

func (c *cmdServerBMCServerLocate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-locate <name> <on|off>"
	cmd.Short = "Set the state of the location indicator LED of a server via BMC"
	cmd.Long = `Description:
  Set the state of the location indicator LED of a server via BMC

  Turns the location indicator LED of a server on or off via BMC.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerLocate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	if args[1] != locationIndicatorStateOn && args[1] != locationIndicatorStateOff {
		return fmt.Errorf("Invalid state %q, must be one of %q, %q", args[1], locationIndicatorStateOn, locationIndicatorStateOff)
	}

	return nil
}

func (c *cmdServerBMCServerLocate) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerSetLocationIndicator(cmd.Context(), name, args[1] == locationIndicatorStateOn)
	if err != nil {
		return err
	}

	return nil
}

// Apply the secure boot certificates of a server via BMC.
type cmdServerBMCApplySecureBootCertificates struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCApplySecureBootCertificates) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "apply-secure-boot-certificates <name>"
	cmd.Short = "Apply the secure boot certificates of a server via BMC"
	cmd.Long = `Description:
  Apply the secure boot certificates of a server via BMC

  Wipes the KEK, DB and DBX secure boot databases of the server and
  reinitializes them with the secure boot certificates provided by IncusOS.

  The server has to be powered off and its BIOS has to allow the secure boot
  databases to be modified, which on most systems means secure boot being
  enabled with a custom policy in user mode. Use "bios-attributes apply" to
  configure the BIOS accordingly and power cycle the server for the settings to
  take effect before running this command.

  The enrolled certificates only take effect once the server is powered on
  again and the firmware has picked them up.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCApplySecureBootCertificates) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCApplySecureBootCertificates) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCApplySecureBootCertificates(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}
