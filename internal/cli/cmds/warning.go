package cmds

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdWarning struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdWarning) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "warning"
	cmd.Short = "Manage warnings"
	cmd.Long = `Description:
  View and acknowledge warnings
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	warningListCmd := cmdWarningList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(warningListCmd.Command())

	// Show
	warningShowCmd := cmdWarningShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(warningShowCmd.Command())

	// Acknowledge
	warningAck := cmdWarningAck{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(warningAck.Command())

	return cmd
}

// List warnings.
type cmdWarningList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdWarningList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List warnings"
	cmd.Long = `Description:
  List all available warnings.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdWarningList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdWarningList) run(cmd *cobra.Command, args []string) error {
	warnings, err := c.ocClient.GetWarnings(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"UUID", "Status", "Scope", "Entity Type", "Entity", "Type", "Last Updated", "Num Messages", "Count"}
	data := [][]string{}

	for _, warning := range warnings {
		data = append(data, []string{warning.UUID.String(), string(warning.Status), warning.Scope.Scope, warning.Scope.EntityType, warning.Scope.Entity, string(warning.Type), warning.LastUpdated.String(), strconv.Itoa(len(warning.Messages)), strconv.Itoa(warning.Count)})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 2, // Scope
			Less:  sort.NaturalLess,
		},
		{
			Index: 3, // Entity Type
			Less:  sort.NaturalLess,
		},
		{
			Index: 4, // Entity
			Less:  sort.NaturalLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, warnings)
}

// Show warning.
type cmdWarningShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdWarningShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid>"
	cmd.Short = "Show warning messages"
	cmd.Long = `Description:
  Show warning messages and data.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdWarningShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdWarningShow) run(cmd *cobra.Command, args []string) error {
	id := args[0]

	warning, err := c.ocClient.GetWarning(cmd.Context(), id)
	if err != nil {
		return err
	}

	fmt.Printf("UUID: %s\n", warning.UUID.String())
	fmt.Printf("Status: %s\n", warning.Status)
	fmt.Printf("Scope: %s\n", warning.Scope.Scope)
	fmt.Printf("Entity Type: %s\n", warning.Scope.EntityType)
	fmt.Printf("Entity: %s\n", warning.Scope.Entity)
	fmt.Printf("Type: %s\n", warning.Type)
	fmt.Printf("First Occurrence: %s\n", warning.FirstOccurrence.Truncate(time.Second).String())
	fmt.Printf("Last Occurrence: %s\n", warning.LastOccurrence.Truncate(time.Second).String())
	fmt.Printf("Last Updated: %s\n", warning.LastUpdated.Truncate(time.Second).String())
	fmt.Printf("Messages:\n")

	for _, message := range warning.Messages {
		fmt.Printf("  - %s\n", message)
	}

	fmt.Printf("Count: %d\n", warning.Count)

	return nil
}

// Acknowledge warning.
type cmdWarningAck struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdWarningAck) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "acknowledge <uuid>"
	cmd.Aliases = []string{"ack"}
	cmd.Short = "Acknowledge warning"
	cmd.Long = `Description:
  Acknowledge the warning.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdWarningAck) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdWarningAck) run(cmd *cobra.Command, args []string) error {
	warningUUID := args[0]

	err := c.ocClient.UpdateWarningStatus(cmd.Context(), warningUUID, api.WarningStatusAcknowledged)
	if err != nil {
		return err
	}

	return nil
}
