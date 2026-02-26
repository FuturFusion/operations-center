package cmds

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
)

type CmdQuery struct {
	OCClient *client.OperationsCenterClient

	flagData    string
	flagRequest string
}

func (c *CmdQuery) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "query <API path>"
	cmd.Short = "Send a raw query to the server"
	cmd.Long = `Description:
  Send a raw query to the server
`

	cmd.Flags().StringVarP(&c.flagData, "data", "d", "", "Input data")
	cmd.Flags().StringVarP(&c.flagRequest, "request", "X", "GET", "Action (defaults to GET)")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *CmdQuery) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *CmdQuery) run(cmd *cobra.Command, args []string) error {
	query, _ := strings.CutSuffix(args[0], "/1.0")

	resp, err := c.OCClient.DoRequest(cmd.Context(), c.flagRequest, query, nil, bytes.NewBuffer([]byte(c.flagData)))
	if err != nil {
		return err
	}

	marshalled, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	cmd.Printf("%s\n", marshalled)

	return nil
}
