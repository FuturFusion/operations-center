package provisioning

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
)

// List server's BMC log sources.
type cmdServerBMCLogs struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdServerBMCLogs) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "logs <name>"
	cmd.Short = "List a server's BMC log sources"
	cmd.Long = `Description:
  List the log sources available via a server's BMC.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCLogs) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdServerBMCLogs) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	logSources, err := c.ocClient.GetServerBMCLogSources(cmd.Context(), name)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Log Source"}
	data := [][]string{}

	for _, logSource := range logSources {
		data = append(data, []string{logSource})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, logSources)
}

// List server's BMC log entries of a log source.
type cmdServerBMCLogEntries struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdServerBMCLogEntries) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "log-entries <name> <source>"
	cmd.Short = "List a server's BMC log entries of a log source"
	cmd.Long = `Description:
  List the log entries of a log source available via a server's BMC.

  The log source has the structure "service/logService", e.g. "chassis/Logs".
  The available log sources can be listed with the "logs" command.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCLogEntries) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdServerBMCLogEntries) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	logSource := args[1]

	logEntries, err := c.ocClient.GetServerBMCLogEntries(cmd.Context(), name, logSource)
	if err != nil {
		return err
	}

	// Render the table. The entries are already ordered by timestamp, so the
	// order is kept as returned.
	header := []string{"Timestamp", "Severity", "Type", "Code", "Message"}
	data := [][]string{}

	for _, logEntry := range logEntries {
		data = append(data, []string{
			logEntry.Timestamp.Format(time.RFC3339),
			logEntry.Severity,
			logEntry.EntryType,
			logEntry.EntryCode,
			logEntry.Message,
		})
	}

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, logEntries)
}

// Dump the raw responses of a server's BMC API.
type cmdServerBMCDump struct {
	ocClient *client.OperationsCenterClient

	flagEndpoints      []string
	flagEndpointFile   string
	flagSkipPredefined bool
	flagTrace          bool
}

func (c *cmdServerBMCDump) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "dump <name>"
	cmd.Short = "Dump the raw responses of a server's BMC API"
	cmd.Long = `Description:
  Dump the raw responses of a curated set of a server's BMC API (e.g. Redfish)
  endpoints.

  Additional endpoints can be dumped alongside the predefined set via
  --endpoint/--endpoint-file, or --skip-predefined can be used to skip the
  predefined set entirely and dump only the additional endpoints.

  The dump is best effort: a failing endpoint is included with its error
  instead of stopping the dump.
`

	cmd.Flags().StringSliceVar(&c.flagEndpoints, "endpoint", nil, "additional BMC endpoint(s) to dump alongside the predefined set")
	cmd.Flags().StringVar(&c.flagEndpointFile, "endpoint-file", "", "file with additional BMC endpoints to dump, one per line")
	cmd.Flags().BoolVar(&c.flagSkipPredefined, "skip-predefined", false, "skip the predefined endpoint set and dump only the additional endpoint(s)")
	cmd.Flags().BoolVar(&c.flagTrace, "trace", false, "include additional opaque trace information (e.g. HTTP headers)")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCDump) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	if c.flagSkipPredefined && len(c.flagEndpoints) == 0 && c.flagEndpointFile == "" {
		return fmt.Errorf("At least one --endpoint or --endpoint-file must be given when --skip-predefined is set")
	}

	return nil
}

func (c *cmdServerBMCDump) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	endpoints := c.flagEndpoints

	if c.flagEndpointFile != "" {
		content, err := os.ReadFile(c.flagEndpointFile)
		if err != nil {
			return fmt.Errorf("Failed to read endpoint file %q: %w", c.flagEndpointFile, err)
		}

		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			endpoints = append(endpoints, line)
		}
	}

	dump, err := c.ocClient.GetServerBMCDump(cmd.Context(), name, endpoints, c.flagSkipPredefined, c.flagTrace)
	if err != nil {
		return err
	}

	dumpJSON, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(dumpJSON))

	return err
}
