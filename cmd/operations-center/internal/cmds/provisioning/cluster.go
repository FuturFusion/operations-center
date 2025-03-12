package provisioning

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdCluster struct{}

func (c *CmdCluster) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cluster"
	cmd.Short = "Interact with clusters"
	cmd.Long = `Description:
  Interact with clusters

  Configure clusters for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	clusterAddCmd := cmdClusterAdd{}
	cmd.AddCommand(clusterAddCmd.Command())

	// List
	clusterListCmd := cmdClusterList{}
	cmd.AddCommand(clusterListCmd.Command())

	// Remove
	clusterRemoveCmd := cmdClusterRemove{}
	cmd.AddCommand(clusterRemoveCmd.Command())

	// Show
	clusterShowCmd := cmdClusterShow{}
	cmd.AddCommand(clusterShowCmd.Command())

	// Resync
	clusterResyncCmd := cmdClusterResync{}
	cmd.AddCommand(clusterResyncCmd.Command())

	return cmd
}

// Add cluster.
type cmdClusterAdd struct {
	connectionURL string
	serverNames   []string
}

func (c *cmdClusterAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name>"
	cmd.Short = "Add a new cluster"
	cmd.Long = `Description:
  Add a new cluster

  Adds a new custer to the operations center.
`

	cmd.RunE = c.Run

	const flagConnectionURL = "connection-url"
	cmd.Flags().StringVarP(&c.connectionURL, flagConnectionURL, "c", "", "Connection URL for the cluster")
	_ = cmd.MarkFlagRequired(flagConnectionURL)

	const flagServerHostnames = "server-hostnames"
	cmd.Flags().StringSliceVarP(&c.serverNames, flagServerHostnames, "s", nil, "Server hostnames of the cluster members")
	_ = cmd.MarkFlagRequired("server-hostnames")

	cmd.PreRunE = c.ValidateFlags

	return cmd
}

func (c *cmdClusterAdd) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	// Client call
	ocClient := client.New()

	err = ocClient.CreateCluster(api.ClusterPost{
		Cluster: api.Cluster{
			Name:          name,
			ConnectionURL: c.connectionURL,
		},
		ServerNames: c.serverNames,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *cmdClusterAdd) ValidateFlags(cmd *cobra.Command, _ []string) error {
	if cmd.Flag("connection-url").Value.String() == "" {
		return fmt.Errorf(`Flag "--connection-url" is mandatory`)
	}

	if cmd.Flag("server-hostnames").Value.String() == "" {
		return fmt.Errorf(`Flag "--server-hostnames" is mandatory`)
	}

	return nil
}

// List clusters.
type cmdClusterList struct {
	flagFormat string
}

func (c *cmdClusterList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available clusters"
	cmd.Long = `Description:
  List the available clusters
`

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdClusterList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	// Client call
	ocClient := client.New()

	clusters, err := ocClient.GetClusters()
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Connection URL", "Last Updated"}
	data := [][]string{}

	for _, cluster := range clusters {
		data = append(data, []string{cluster.Name, cluster.ConnectionURL, cluster.LastUpdated.String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, clusters)
}

// Remove cluster.
type cmdClusterRemove struct{}

func (c *cmdClusterRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a cluster"
	cmd.Long = `Description:
  Remove a cluster

  Removes a custer from the operations center.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterRemove) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	// Client call
	ocClient := client.New()

	err = ocClient.DeleteCluster(name)
	if err != nil {
		return err
	}

	return nil
}

// Show cluster.
type cmdClusterShow struct{}

func (c *cmdClusterShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a cluster"
	cmd.Long = `Description:
  Show information about a cluster.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	// Client call
	ocClient := client.New()

	cluster, err := ocClient.GetCluster(name)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", cluster.Name)
	fmt.Printf("Connection URL: %s\n", cluster.ConnectionURL)
	fmt.Printf("Last Updated: %s\n", cluster.LastUpdated.String())

	return nil
}

// Resync cluster.
type cmdClusterResync struct{}

func (c *cmdClusterResync) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "resync <name>"
	cmd.Short = "Resync inventory for a cluster"
	cmd.Long = `Description:
  Resync inventory for a cluster.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterResync) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	// Client call
	ocClient := client.New()

	err = ocClient.ResyncCluster(name)
	if err != nil {
		return err
	}

	return nil
}
