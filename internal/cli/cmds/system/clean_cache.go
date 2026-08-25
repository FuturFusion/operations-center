package system

import (
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
)

type CmdCleanCache struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdCleanCache) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "clean-cache"
	cmd.Short = "Purge operations-center's cache"
	cmd.Long = `Description:
  Purge the operations-center's cache

  Removes the reproducible, cached data.

  Data, which is being generated, may be removed as well, in which case that
  operation has to be retried.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *CmdCleanCache) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *CmdCleanCache) run(cmd *cobra.Command, args []string) error {
	err := c.OCClient.CleanSystemCache(cmd.Context())
	if err != nil {
		return err
	}

	return nil
}
