package provisioning

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdChannel struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdChannel) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "channel"
	cmd.Short = "Interact with channels"
	cmd.Long = `Description:
  Interact with channels

  Manage channels.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	updateListCmd := cmdChannelList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateListCmd.Command())

	// Show
	updateShowCmd := cmdChannelShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateShowCmd.Command())

	// Add
	updateAddCmd := cmdChannelAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateAddCmd.Command())

	return cmd
}

// List channels.
type cmdChannelList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdChannelList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available channels"
	cmd.Long = `Description:
  List the available channels
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdChannelList) run(cmd *cobra.Command, args []string) error {
	channels, err := c.ocClient.GetChannels(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Description", "Last Updated"}
	data := [][]string{}

	for _, channel := range channels {
		data = append(data, []string{channel.Name, channel.Description, channel.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // Name
			Less:  sort.StringLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, channels)
}

// Show update.
type cmdChannelShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdChannelShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a channel"
	cmd.Long = `Description:
  Show information about a channel.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdChannelShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	channel, err := c.ocClient.GetChannel(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", channel.Name)
	fmt.Printf("Description: %s\n", channel.Description)
	fmt.Printf("Last Updated: %s\n", channel.LastUpdated.Truncate(time.Second).String())

	return nil
}

// Add update.
type cmdChannelAdd struct {
	ocClient *client.OperationsCenterClient

	description string
}

func (c *cmdChannelAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name>"
	cmd.Short = "Add a channel"
	cmd.Long = `Description:
  Add a channel.
`

	cmd.Flags().StringVar(&c.description, "description", "", "Description of the channel")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdChannelAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.CreateChannel(cmd.Context(), api.ChannelPost{
		Name: name,
		ChannelPut: api.ChannelPut{
			Description: c.description,
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to create channel %q: %w", name, err)
	}

	return nil
}
