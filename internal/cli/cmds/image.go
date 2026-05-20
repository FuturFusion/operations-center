package cmds

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/cmds/image"
	"github.com/FuturFusion/operations-center/internal/client"
)

type CmdImage struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdImage) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "image"
	cmd.Short = "Interact with images"
	cmd.Long = `Description:
  Interact with operations center images

  Interact with operations center images.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	ChannelCmd := image.CmdIncusImage{
		OCClient: c.OCClient,
	}

	cmd.AddCommand(ChannelCmd.Command())

	return cmd
}
