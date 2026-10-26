package cmds

import (
	"net/http"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/cmds/cluster"
	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdCluster struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdCluster) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cluster"
	cmd.Short = "Manage Operations Center cluster"

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	cmd.AddCommand((&cluster.CmdMembers{OCClient: c.OCClient}).Command())
	cmd.AddCommand((&cluster.CmdTokens{OCClient: c.OCClient}).Command())

	cmd.AddCommand((&cmdClusterEnable{ocClient: c.OCClient}).Command())
	cmd.AddCommand((&cmdClusterJoin{ocClient: c.OCClient}).Command())

	return cmd
}

// Enable clustering.
type cmdClusterEnable struct {
	ocClient *client.OperationsCenterClient

	flagName    string
	flagAddress string
}

func (c *cmdClusterEnable) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "enable"
	cmd.Short = "Enable clustering"

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	cmd.Flags().StringVarP(&c.flagName, "server-name", "n", "", "Name representing this member in the cluster")
	cmd.Flags().StringVarP(&c.flagAddress, "server-address", "a", "", "Address used for intra-cluster communication to this member by the cluster")

	return cmd
}

func (c *cmdClusterEnable) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterEnable) run(cmd *cobra.Command, args []string) error {
	_, err := c.ocClient.DoRequest(cmd.Context(), http.MethodPost, "/membership/:enable", nil, api.ClusterBootstrapPost{ServerName: c.flagName, ClusterAddress: c.flagAddress})

	return err
}

// Join a cluster.
type cmdClusterJoin struct {
	ocClient *client.OperationsCenterClient

	flagName    string
	flagAddress string
}

func (c *cmdClusterJoin) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "join <token>"
	cmd.Short = "Join an existing cluster"

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	cmd.Flags().StringVarP(&c.flagName, "server-name", "n", "", "Name representing this member in the cluster")
	cmd.Flags().StringVarP(&c.flagAddress, "server-address", "a", "", "Address used for intra-cluster communication to this member by the cluster")

	return cmd
}

func (c *cmdClusterJoin) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterJoin) run(cmd *cobra.Command, args []string) error {
	_, err := c.ocClient.DoRequest(cmd.Context(), http.MethodPost, "/membership/:join", nil, api.ClusterJoinPost{ServerName: c.flagName, ClusterAddress: c.flagAddress, Token: args[0]})

	return err
}
