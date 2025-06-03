package provisioning

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
)

type CmdServer struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdServer) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server"
	cmd.Short = "Interact with servers"
	cmd.Long = `Description:
  Interact with servers

  Configure servers for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	serverListCmd := cmdServerList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverListCmd.Command())

	// Remove
	serverRemoveCmd := cmddServerRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverRemoveCmd.Command())

	// Show
	serverShowCmd := cmddServerShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverShowCmd.Command())

	return cmd
}

// List servers.
type cmdServerList struct {
	ocClient *client.OperationsCenterClient

	flagFilterCluster    string
	flagFilterExpression string

	flagFormat string
}

func (c *cmdServerList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available servers"
	cmd.Long = `Description:
  List the available servers
`

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.flagFilterCluster, "cluster", "", "cluster name to filter for")
	cmd.Flags().StringVar(&c.flagFilterExpression, "filter", "", "filter expression to apply")

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdServerList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	var filter provisioning.ServerFilter

	if c.flagFilterCluster != "" {
		filter.Cluster = ptr.To(c.flagFilterCluster)
	}

	if c.flagFilterExpression != "" {
		filter.Expression = ptr.To(c.flagFilterExpression)
	}

	servers, err := c.ocClient.GetWithFilterServers(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Cluster", "Name", "Connection URL", "Type", "Status", "Last Updated"}
	data := [][]string{}

	for _, server := range servers {
		data = append(data, []string{server.Cluster, server.Name, server.ConnectionURL, string(server.Type), server.Status.String(), server.LastUpdated.String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, servers)
}

// Remove server.
type cmddServerRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddServerRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a server"
	cmd.Long = `Description:
  Remove a server

  Removes a custer from the operations center.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddServerRemove) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	err = c.ocClient.DeleteServer(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Show server.
type cmddServerShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddServerShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a server"
	cmd.Long = `Description:
  Show information about a server.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddServerShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	server, err := c.ocClient.GetServer(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Cluster: %s\n", server.Cluster)
	fmt.Printf("Name: %s\n", server.Name)
	fmt.Printf("Connection URL: %s\n", server.ConnectionURL)
	fmt.Printf("Type: %s\n", server.Type)
	fmt.Printf("Status: %s\n", server.Status.String())
	fmt.Printf("Last Updated: %s\n", server.LastUpdated.String())

	return nil
}
