package cluster

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdTokens struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdTokens) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "tokens"
	cmd.Short = "Manage cluster membership tokens"

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	cmd.AddCommand((&cmdTokensList{ocClient: c.OCClient}).Command())
	// Issue
	cmd.AddCommand((&cmdTokensIssue{ocClient: c.OCClient}).Command())
	// Delete
	cmd.AddCommand((&cmdTokensDelete{ocClient: c.OCClient}).Command())

	return cmd
}

// List tokens.
type cmdTokensList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdTokensList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List cluster membership tokens"

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokensList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdTokensList) run(cmd *cobra.Command, args []string) error {
	resp, err := c.ocClient.DoRequest(cmd.Context(), http.MethodGet, "/membership/tokens", nil, nil)
	if err != nil {
		return err
	}

	var tokens []api.ClusterJoinToken
	err = json.Unmarshal(resp.Metadata, &tokens)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Member Name", "Token"}
	data := [][]string{}

	for _, m := range tokens {
		token, err := m.String()
		if err != nil {
			return err
		}

		data = append(data, []string{
			m.ServerName,
			token,
		})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, tokens)
}

// Issue a token.
type cmdTokensIssue struct {
	ocClient *client.OperationsCenterClient

	flagExpiry time.Duration
}

func (c *cmdTokensIssue) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "issue"
	cmd.Aliases = []string{"add"}
	cmd.Short = "Issue cluster membership tokens"

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	cmd.Flags().DurationVarP(&c.flagExpiry, "expiry", "e", 3*time.Hour, "Duraton that the token will remain valid for (default: 3 hours)")

	return cmd
}

func (c *cmdTokensIssue) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdTokensIssue) run(cmd *cobra.Command, args []string) error {
	_, err := c.ocClient.DoRequest(cmd.Context(), http.MethodPost, "/membership/tokens", nil, api.ClusterTokenPost{ServerName: args[0], Expiry: c.flagExpiry.String()})
	if err != nil {
		return err
	}

	resp, err := c.ocClient.DoRequest(cmd.Context(), http.MethodGet, "/membership/tokens/"+args[0], nil, nil)
	if err != nil {
		return err
	}

	var token string
	err = json.Unmarshal(resp.Metadata, &token)
	if err != nil {
		return err
	}

	fmt.Println(token)

	return nil
}

// Delete a token.
type cmdTokensDelete struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokensDelete) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "delete"
	cmd.Aliases = []string{"remove", "rm", "del"}
	cmd.Short = "Delete cluster membership tokens"

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokensDelete) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdTokensDelete) run(cmd *cobra.Command, args []string) error {
	_, err := c.ocClient.DoRequest(cmd.Context(), http.MethodDelete, "/membership/tokens/"+args[0], nil, nil)

	return err
}
