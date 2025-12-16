package cmds

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/cmds/system"
	"github.com/FuturFusion/operations-center/internal/client"
)

type CmdSystem struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdSystem) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "system"
	cmd.Short = "Interact with system"
	cmd.Long = `Description:
  Interact with operations center system

  Configure system of operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	certificateCmd := system.CmdCertificate{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(certificateCmd.Command())

	networkCmd := system.CmdNetwork{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(networkCmd.Command())

	securityCmd := system.CmdSecurity{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(securityCmd.Command())

	updatesCmd := system.CmdUpdates{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(updatesCmd.Command())

	return cmd
}
