package cmds

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/lxc/incus-os/incus-osd/cli"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/sql/dump"
	"github.com/FuturFusion/operations-center/internal/util/render"
)

type CmdAdmin struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdAdmin) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "admin"
	cmd.Short = "Manage IncusOS"
	cmd.Long = `Description:
  Manage IncusOS
`

	// os
	adminOSCmd := cmdAdminOS{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(adminOSCmd.Command())

	// sql
	adminSQLCmd := cmdAdminSQL{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(adminSQLCmd.Command())

	return cmd
}

type cmdAdminOS struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdAdminOS) Command() *cobra.Command {
	args := &cli.Args{
		SupportsTarget:    false,
		SupportsRemote:    false,
		DefaultListFormat: "table",
		DoHTTP: func(_ string, req *http.Request) (*http.Response, error) {
			var err error

			req.URL, err = url.Parse(c.ocClient.GetBaseAddr() + req.URL.String())
			if err != nil {
				return nil, err
			}

			return c.ocClient.DoHTTP(req)
		},
	}

	return cli.NewCommand(args)
}

type cmdAdminSQL struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdAdminSQL) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "sql <query>"
	cmd.Short = "Execute a SQL query against the local database"
	cmd.Long = `Description:
  Execute a SQL query against the local database

  If <query> is the special value "-", then the query is read from
  standard input.

  If <query> is the special value ".dump", the command returns a SQL text
  dump of the given database.

  If <query> is the special value ".schema", the command returns the SQL
  text schema of the given database.

  If <query> is the special value ".tables", the command returns the SQL
  text tables of the given database.

  This internal command is mostly useful for debugging and disaster
  recovery. The development team will occasionally provide hotfixes to users as a
  set of database queries to fix some data inconsistency.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdAdminSQL) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdAdminSQL) run(cmd *cobra.Command, args []string) error {
	query := args[0]

	if query == "-" {
		// Read from stdin
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("Failed to read from stdin: %w", err)
		}

		query = string(bytes)
	}

	if query == ".dump" || query == ".schema" || query == ".tables" {
		queryParams := url.Values{}
		switch query {
		case ".schema":
			queryParams.Add("dump", dump.OptionSchema.String())

		case ".tables":
			queryParams.Add("dump", dump.OptionTables.String())
		}

		response, err := c.ocClient.DoRequest(cmd.Context(), http.MethodGet, "/internal/sql", queryParams, nil)
		if err != nil {
			return fmt.Errorf("Failed to request dump: %w", err)
		}

		dumpResult := dump.SQLDump{}
		err = json.Unmarshal(response.Metadata, &dumpResult)
		if err != nil {
			return fmt.Errorf("Failed to parse dump response: %w", err)
		}

		fmt.Print(dumpResult.Text)
		return nil
	}

	data := dump.SQLQuery{
		Query: query,
	}

	response, err := c.ocClient.DoRequest(cmd.Context(), http.MethodPost, "/internal/sql", nil, data)
	if err != nil {
		return err
	}

	batch := dump.SQLBatch{}
	err = json.Unmarshal(response.Metadata, &batch)
	if err != nil {
		return err
	}

	for i, result := range batch.Results {
		if len(batch.Results) > 1 {
			fmt.Printf("=> Query %d:"+"\n\n", i)
		}

		if result.Type == "select" {
			err := c.sqlPrintSelectResult(cmd, result)
			if err != nil {
				return err
			}
		} else {
			fmt.Printf("Rows affected: %d"+"\n", result.RowsAffected)
		}

		if len(batch.Results) > 1 {
			fmt.Println("")
		}
	}

	return nil
}

func (c *cmdAdminSQL) sqlPrintSelectResult(cmd *cobra.Command, result dump.SQLResult) error {
	data := make([][]string, 0, len(result.Rows))

	for _, row := range result.Rows {
		rowData := make([]string, 0, len(row))

		for _, col := range row {
			rowData = append(rowData, fmt.Sprintf("%v", col))
		}

		data = append(data, rowData)
	}

	return render.Table(cmd.OutOrStdout(), c.flagFormat, result.Columns, data, result)
}
