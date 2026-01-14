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

type cmdUpdateExposedchannel struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateExposedchannel) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "exposedchannel"
	cmd.Short = "Interact with exposed channels for updates"
	cmd.Long = `Description:
  Interact with exposed channels for updates

  Manage exposed channels for updates provided by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	updateListCmd := cmdUpdateExposedchannelList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(updateListCmd.Command())

	// Show
	updateShowCmd := cmdUpdateExposedchannelShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(updateShowCmd.Command())

	// Add
	updateAddCmd := cmdUpdateExposedchannelAdd{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(updateAddCmd.Command())

	return cmd
}

// List update exposed channels.
type cmdUpdateExposedchannelList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdUpdateExposedchannelList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available exposed channels for updates"
	cmd.Long = `Description:
  List the available exposed channels for updates
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdUpdateExposedchannelList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdUpdateExposedchannelList) run(cmd *cobra.Command, args []string) error {
	exposedchannels, err := c.ocClient.GetUpdateExposechannels(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Description", "Last Updated"}
	data := [][]string{}

	for _, exposedchannel := range exposedchannels {
		data = append(data, []string{exposedchannel.Name, exposedchannel.Description, exposedchannel.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // Name
			Less:  sort.StringLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, exposedchannels)
}

// Show update.
type cmdUpdateExposedchannelShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateExposedchannelShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about an exposed channel for updates"
	cmd.Long = `Description:
  Show information about an exposed channel for update.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdUpdateExposedchannelShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdUpdateExposedchannelShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	exposedchannel, err := c.ocClient.GetUpdateExposedchannel(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", exposedchannel.Name)
	fmt.Printf("Description: %s\n", exposedchannel.Description)
	fmt.Printf("Last Updated: %s\n", exposedchannel.LastUpdated.Truncate(time.Second).String())

	return nil
}

// Add update.
type cmdUpdateExposedchannelAdd struct {
	ocClient *client.OperationsCenterClient

	description string
}

func (c *cmdUpdateExposedchannelAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name>"
	cmd.Short = "Add an exposed channel for updates"
	cmd.Long = `Description:
  Add an exposed channel for updates.
`

	cmd.Flags().StringVar(&c.description, "description", "", "Description of the exposed channel")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdUpdateExposedchannelAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdUpdateExposedchannelAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.CreateUpdateExposedchannel(cmd.Context(), api.UpdateExposedchannelPost{
		Name: name,
		UpdateExposedchannelPut: api.UpdateExposedchannelPut{
			Description: c.description,
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to create exposed channel for updates %q: %w", name, err)
	}

	return nil
}
