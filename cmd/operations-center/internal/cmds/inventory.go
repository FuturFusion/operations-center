package cmds

import (
	"github.com/spf13/cobra"
)

type CmdInventory struct{}

func (c *CmdInventory) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "inventory"
	cmd.Short = "Interact with inventory"
	cmd.Long = `Description:
  Interact with operations center inventory

  Configure inventory of operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	return cmd
}
