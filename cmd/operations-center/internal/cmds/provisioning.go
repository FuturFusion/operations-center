package cmds

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/cmds/provisioning"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/config"
)

type CmdProvisioning struct {
	Config *config.Config
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
		Config: c.Config,
	}

	cmd.AddCommand(clusterCmd.Command())

	serverCmd := provisioning.CmdServer{
		Config: c.Config,
	}

	cmd.AddCommand(serverCmd.Command())

	return cmd
}
