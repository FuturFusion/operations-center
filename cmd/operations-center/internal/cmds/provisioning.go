package cmds

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/cmds/provisioning"
)

type CmdProvisioning struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdProvisioning) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "provisioning"
	cmd.Short = "Interact with provisioning"
	cmd.Long = `Description:
  Interact with operations center provisioning

  Configure provisioning of operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	clusterCmd := provisioning.CmdCluster{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(clusterCmd.Command())

	serverCmd := provisioning.CmdServer{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(serverCmd.Command())

	tokenCmd := provisioning.CmdToken{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(tokenCmd.Command())

	updateCmd := provisioning.CmdUpdate{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(updateCmd.Command())

	return cmd
}
