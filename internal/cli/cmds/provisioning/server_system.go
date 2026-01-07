package provisioning

import (
	"github.com/spf13/cobra"

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

	// System Storage
	serverSystemStorageCmd := cmdServerSystemStorage{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverSystemStorageCmd.Command())

	return cmd
}
