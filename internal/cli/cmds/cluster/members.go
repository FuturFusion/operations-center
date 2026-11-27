package cluster

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdMembers struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdMembers) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "members"
	cmd.Short = "Interact with cluster members"

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	clusterListCmd := cmdMembersList{ocClient: c.OCClient}
	cmd.AddCommand(clusterListCmd.Command())

	// Remove
	clusterRemoveCmd := cmdMembersRemove{ocClient: c.OCClient}
	cmd.AddCommand(clusterRemoveCmd.Command())

	// Move files
	clusterMoveFilesCmd := cmdMembersMoveFiles{ocClient: c.OCClient}
	cmd.AddCommand(clusterMoveFilesCmd.Command())

	return cmd
}

// List clusters.
type cmdMembersList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdMembersList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List cluster members"

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdMembersList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdMembersList) run(cmd *cobra.Command, args []string) error {
	resp, err := c.ocClient.DoRequest(cmd.Context(), http.MethodGet, "/membership/members", nil, nil)
	if err != nil {
		return err
	}

	var members []api.ClusterMember
	err = json.Unmarshal(resp.Metadata, &members)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Address", "Role", "Last Heartbeat", "Status", "File Sets"}
	data := [][]string{}

	for _, m := range members {
		status := "Online"
		if m.Offline {
			status = "Offline"
		}

		role := m.Roles[0]
		if m.Leader {
			role += " (leader)"
		}

		data = append(data, []string{
			m.Name,
			m.Address,
			role,
			time.Since(m.LastHeartbeat).Truncate(time.Second).String() + " ago",
			status,
			strconv.Itoa(m.FileSets),
		})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, members)
}

// Remove cluster members.
type cmdMembersRemove struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdMembersRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Aliases = []string{"rm", "delete"}
	cmd.Short = "Remove cluster members"
	cmd.Long = `Description:
  Remove cluster members

  The removed member is reset to an empty standalone server. A member, which
  is the file target of files of updates, images or cluster artifacts, is only removed after
  its files were moved to another member with "move-files". Use --force to skip
  the checks and the reset, e.g. if the member is offline, its files are lost.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "Remove the member without checks and without resetting it, its files are lost")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdMembersRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdMembersRemove) run(cmd *cobra.Command, args []string) error {
	query := url.Values{}
	if c.flagForce {
		query.Add("force", "1")
	}

	_, err := c.ocClient.DoRequest(cmd.Context(), http.MethodDelete, "/membership/members/"+url.PathEscape(args[0]), query, nil)

	return err
}

// Move the files of cluster members.
type cmdMembersMoveFiles struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdMembersMoveFiles) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "move-files <name> <target>"
	cmd.Short = "Move the files of a cluster member to another one"
	cmd.Long = `Description:
  Move the files of a cluster member to another one

  The files of updates, images and cluster artifacts are stored by a single
  member. Move them off a member, before removing it from the cluster. Both
  members need to be online.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdMembersMoveFiles) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdMembersMoveFiles) run(cmd *cobra.Command, args []string) error {
	_, err := c.ocClient.DoRequest(cmd.Context(), http.MethodPost, "/membership/members/"+url.PathEscape(args[0])+"/:move-files", nil, api.ClusterMemberMoveFilesPost{Target: args[1]})

	return err
}
