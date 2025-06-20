// Code generated by generate-inventory; DO NOT EDIT.

package inventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
)

type CmdInstance struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdInstance) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "instance"
	cmd.Short = "Interact with instances"
	cmd.Long = `Description:
  Interact with instances
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	instanceListCmd := cmdInstanceList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(instanceListCmd.Command())

	// Show
	instanceShowCmd := cmdInstanceShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(instanceShowCmd.Command())

	return cmd
}

// List instances.
type cmdInstanceList struct {
	ocClient *client.OperationsCenterClient

	flagFilterCluster    string
	flagFilterServer     string
	flagFilterProject    string
	flagFilterExpression string

	flagColumns string
	flagFormat  string
}

const instanceDefaultColumns = `{{ .UUID }},{{ .Cluster }},{{ .Server }},{{ .ProjectName }},{{ .Name }},{{ .LastUpdated }}`

func (c *cmdInstanceList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available instances"
	cmd.Long = `Description:
  List the available instances
`

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.flagFilterCluster, "cluster", "", "cluster name to filter for")
	cmd.Flags().StringVar(&c.flagFilterServer, "server", "", "server name to filter for")
	cmd.Flags().StringVar(&c.flagFilterProject, "project", "", "project name to filter for")
	cmd.Flags().StringVar(&c.flagFilterExpression, "filter", "", "filter expression to apply")

	cmd.Flags().StringVarP(&c.flagColumns, "columns", "c", instanceDefaultColumns, `Comma separated list of columns to print with the respective value in Go Template format`)
	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdInstanceList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	var filter inventory.InstanceFilter

	if c.flagFilterCluster != "" {
		filter.Cluster = ptr.To(c.flagFilterCluster)
	}

	if c.flagFilterServer != "" {
		filter.Server = ptr.To(c.flagFilterServer)
	}

	if c.flagFilterProject != "" {
		filter.Project = ptr.To(c.flagFilterProject)
	}

	if c.flagFilterExpression != "" {
		filter.Expression = ptr.To(c.flagFilterExpression)
	}

	instances, err := c.ocClient.GetWithFilterInstances(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	fields := strings.Split(c.flagColumns, ",")

	header := []string{}
	tmpl := template.New("")

	for _, field := range fields {
		title := strings.Trim(field, "{} .")
		header = append(header, title)
		fieldTmpl := tmpl.New(title)
		_, err := fieldTmpl.Parse(field)
		if err != nil {
			return err
		}
	}

	data := [][]string{}
	wr := &bytes.Buffer{}

	for _, instance := range instances {
		row := make([]string, len(header))
		for i, field := range header {
			wr.Reset()
			err := tmpl.ExecuteTemplate(wr, field, instance)
			if err != nil {
				return err
			}

			row[i] = wr.String()
		}

		data = append(data, row)
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, instances)
}

// Show instance.
type cmdInstanceShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdInstanceShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid>"
	cmd.Short = "Show information about a instance"
	cmd.Long = `Description:
  Show information about a instance.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdInstanceShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	id := args[0]

	instance, err := c.ocClient.GetInstance(cmd.Context(), id)
	if err != nil {
		return err
	}

	objectJSON, err := json.MarshalIndent(instance.Object, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("UUID: %s\n", instance.UUID.String())
	fmt.Printf("Cluster: %s\n", instance.Cluster)
	fmt.Printf("Server: %s\n", instance.Server)
	fmt.Printf("Project Name: %s\n", instance.ProjectName)
	fmt.Printf("Name: %s\n", instance.Name)
	fmt.Printf("Last Updated: %s\n", instance.LastUpdated.String())
	fmt.Printf("Object:\n%s\n", objectJSON)

	return nil
}
