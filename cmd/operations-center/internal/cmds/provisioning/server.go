package provisioning

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
)

type CmdServer struct{}

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
	serverListCmd := cmddServerList{}
	cmd.AddCommand(serverListCmd.Command())

	// Remove
	serverRemoveCmd := cmddServerRemove{}
	cmd.AddCommand(serverRemoveCmd.Command())

	// Show
	serverShowCmd := cmddServerShow{}
	cmd.AddCommand(serverShowCmd.Command())

	return cmd
}

// List servers.
type cmddServerList struct {
	flagFormat string
}

func (c *cmddServerList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available servers"
	cmd.Long = `Description:
  List the available servers
`

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmddServerList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	// Client call
	ocClient := client.New()

	servers, err := ocClient.GetServers()
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Cluster", "Name", "Connection URL", "Type", "Last Updated"}
	data := [][]string{}

	for _, server := range servers {
		data = append(data, []string{server.Cluster, server.Name, server.ConnectionURL, string(server.Type), server.LastUpdated.String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, servers)
}

// Remove server.
type cmddServerRemove struct{}

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

	// Client call
	ocClient := client.New()

	err = ocClient.DeleteServer(name)
	if err != nil {
		return err
	}

	return nil
}

// Show server.
type cmddServerShow struct{}

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

	// Client call
	ocClient := client.New()

	server, err := ocClient.GetServer(name)
	if err != nil {
		return err
	}

	fmt.Printf("Cluster: %s\n", server.Cluster)
	fmt.Printf("Name: %s\n", server.Name)
	fmt.Printf("Connection URL: %s\n", server.ConnectionURL)
	fmt.Printf("Type: %s\n", server.Type)
	fmt.Printf("Last Updated: %s\n", server.LastUpdated.String())

	return nil
}
