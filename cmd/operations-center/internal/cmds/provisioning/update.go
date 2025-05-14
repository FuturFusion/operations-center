package provisioning

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
)

type CmdUpdate struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdUpdate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update"
	cmd.Short = "Interact with updates"
	cmd.Long = `Description:
  Interact with updates

  Manage updates provided by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	updateListCmd := cmdUpdateList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateListCmd.Command())

	// Show
	updateShowCmd := cmddUpdateShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateShowCmd.Command())

	return cmd
}

// List updates.
type cmdUpdateList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdUpdateList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available updates"
	cmd.Long = `Description:
  List the available updates
`

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdUpdateList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	updates, err := c.ocClient.GetUpdates()
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"UUID", "Channel", "Version", "Published At", "Severity", "Components"}
	data := [][]string{}

	for _, update := range updates {
		data = append(data, []string{update.UUID.String(), update.Channel, update.Version, update.PublishedAt.String(), update.Severity.String(), update.Components.String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, updates)
}

// Show update.
type cmddUpdateShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddUpdateShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a update"
	cmd.Long = `Description:
  Show information about a update.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddUpdateShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	update, err := c.ocClient.GetUpdate(name)
	if err != nil {
		return err
	}

	fmt.Printf("UUID: %s\n", update.UUID.String())
	fmt.Printf("Channel: %s\n", update.Channel)
	fmt.Printf("Version: %s\n", update.Version)
	fmt.Printf("Published At: %s\n", update.PublishedAt.String())
	fmt.Printf("Severity: %s\n", update.Severity)
	fmt.Printf("Components: %s\n", update.Components)

	return nil
}
