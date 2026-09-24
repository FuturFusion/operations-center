package cmds

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

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

	// debug
	adminDebugCmd := cmdAdminDebug{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(adminDebugCmd.Command())

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

type cmdAdminDebug struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdAdminDebug) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "debug"
	cmd.Short = "Debug Operations Center"
	cmd.Long = `Description:
  Debug Operations Center
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// pprof
	adminDebugPprofCmd := cmdAdminDebugPprof{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(adminDebugPprofCmd.Command())

	return cmd
}

type cmdAdminDebugPprof struct {
	ocClient *client.OperationsCenterClient

	flagSeconds int
	flagDebug   int
	flagOutput  string
}

func (c *cmdAdminDebugPprof) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "pprof <profile>"
	cmd.Short = "Download a pprof profile"
	cmd.Long = `Description:
  Download a pprof profile

  Supported profiles are allocs, block, cmdline, goroutine, heap, mutex,
  profile (CPU), threadcreate and trace. The pprof endpoints need to be enabled
  with the "pprof_enabled" system setting.

  For the profile and trace profiles, --seconds sets the capture duration, for
  the other profiles it returns the delta over the given duration.

  The profile is written to <profile>.pprof by default, a --debug level above
  0 returns a text representation, which is written to stdout by default. The
  profile and trace captures are only available in the binary format.

  The downloaded profile can be analyzed with "go tool pprof <file>" or, for a
  trace, "go tool trace <file>".
`

	cmd.Flags().IntVar(&c.flagSeconds, "seconds", 0, "Duration in seconds for profile, trace and delta profiles")
	cmd.Flags().IntVar(&c.flagDebug, "debug", 0, "Text output level, 0 for the binary format")
	cmd.Flags().StringVarP(&c.flagOutput, "output", "o", "", `Output file, "-" for stdout`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdAdminDebugPprof) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	// The profile and trace captures are only available in binary format.
	if c.flagDebug > 0 && (args[0] == "profile" || args[0] == "trace") {
		return fmt.Errorf("Flag --debug is not supported for %q", args[0])
	}

	return nil
}

func (c *cmdAdminDebugPprof) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	query := url.Values{}
	if c.flagSeconds > 0 {
		query.Set("seconds", strconv.Itoa(c.flagSeconds))
	}

	if c.flagDebug > 0 {
		query.Set("debug", strconv.Itoa(c.flagDebug))
	}

	output := c.flagOutput
	if output == "" {
		output = name + ".pprof"

		if c.flagDebug > 0 || name == "cmdline" {
			output = "-"
		}
	}

	profile, err := c.ocClient.GetPprof(cmd.Context(), name, query)
	if err != nil {
		return err
	}

	defer profile.Close()

	if output == "-" {
		_, err = io.Copy(cmd.OutOrStdout(), profile)
		return err
	}

	targetFile, err := os.OpenFile(output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer targetFile.Close()

	_, err = io.Copy(targetFile, profile)
	if err != nil {
		return err
	}

	err = targetFile.Close()
	if err != nil {
		return err
	}

	cmd.Printf("Profile written to %q\n", output)

	return nil
}
