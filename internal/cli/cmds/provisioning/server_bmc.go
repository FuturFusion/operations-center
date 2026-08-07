package provisioning

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
)

// Interact with BMC of servers.
type cmdServerBMC struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMC) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "bmc"
	cmd.Short = "Interact with the BMC of servers"
	cmd.Long = `Description:
  Interact with the BMC of servers.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Refresh
	serverBMCDataRefreshCmd := cmdServerBMCDataRefresh{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCDataRefreshCmd.Command())
	// Server Power On
	serverBMCServerPowerOnCmd := cmdServerBMCServerPowerOn{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerPowerOnCmd.Command())

	// Server Power Off
	serverBMCServerPowerOffCmd := cmdServerBMCServerPowerOff{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerPowerOffCmd.Command())

	// Server Restart
	serverBMCServerRestartCmd := cmdServerBMCServerRestart{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCServerRestartCmd.Command())

	// Apply BIOS attributes
	serverBMCApplyBIOSAttributesCmd := cmdServerBMCApplyBIOSAttributes{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCApplyBIOSAttributesCmd.Command())

	// Setup secure boot certificates
	serverBMCSetupSecureBootCertificatesCmd := cmdServerBMCSetupSecureBootCertificates{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCSetupSecureBootCertificatesCmd.Command())

	// Logs
	serverBMCLogsCmd := cmdServerBMCLogs{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCLogsCmd.Command())

	// Log entries
	serverBMCLogEntriesCmd := cmdServerBMCLogEntries{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCLogEntriesCmd.Command())

	// Dump
	serverBMCDumpCmd := cmdServerBMCDump{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverBMCDumpCmd.Command())

	return cmd
}

// Refresh server's BMC data.
type cmdServerBMCDataRefresh struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCDataRefresh) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "refresh <name>"
	cmd.Short = "Refresh a server's BMC data"
	cmd.Long = `Description:
  Refresh a server's BMC data.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCDataRefresh) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCDataRefresh) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCDataRefresh(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Power on server via BMC.
type cmdServerBMCServerPowerOn struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCServerPowerOn) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-power-on <name>"
	cmd.Short = "Power on a server via BMC"
	cmd.Long = `Description:
  Power on a server via BMC

  Triggers a server power on via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a power on")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerPowerOn) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCServerPowerOn) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerPowerOn(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Power off server via BMC.
type cmdServerBMCServerPowerOff struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCServerPowerOff) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-power-off <name>"
	cmd.Short = "Power off a server via BMC"
	cmd.Long = `Description:
  Power off a server via BMC

  Triggers a server power off via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a server power off")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerPowerOff) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCServerPowerOff) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerPowerOff(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Restart server via BMC.
type cmdServerBMCServerRestart struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdServerBMCServerRestart) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server-restart <name>"
	cmd.Short = "Restart a server via BMC"
	cmd.Long = `Description:
  Restart a server via BMC

  Triggers a server restart via BMC.
`

	cmd.Flags().BoolVar(&c.flagForce, "force", false, "forcefully trigger a server restart")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCServerRestart) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCServerRestart) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCServerRestart(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// Apply BIOS attributes to a server via BMC.
type cmdServerBMCApplyBIOSAttributes struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCApplyBIOSAttributes) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "apply-bios-attributes <name> [attributes.yaml]"
	cmd.Short = "Apply BIOS attributes to a server via BMC"
	cmd.Long = `Description:
  Apply BIOS attributes to a server via BMC

  Applies the given BIOS attributes to the server via BMC. The settings are
  applied on the next reset of the server.

  The attributes are provided as a YAML document with attribute names and
  values at the root level, either from the given file or, if no file is
  given, from stdin.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCApplyBIOSAttributes) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCApplyBIOSAttributes) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	var attributesReader io.Reader = os.Stdin

	if len(args) > 1 {
		attributesFile := args[1]

		f, err := os.Open(attributesFile)
		if err != nil {
			return fmt.Errorf("Failed to read file %q: %w", attributesFile, err)
		}

		defer func() {
			_ = f.Close()
		}()

		attributesReader = f
	}

	body, err := io.ReadAll(attributesReader)
	if err != nil {
		return fmt.Errorf("Failed to read BIOS attributes: %w", err)
	}

	attributes := map[string]any{}

	err = yaml.Unmarshal(body, &attributes)
	if err != nil {
		return fmt.Errorf("Failed to parse BIOS attributes YAML: %w", err)
	}

	err = c.ocClient.ApplyBIOSAttributes(cmd.Context(), name, attributes)
	if err != nil {
		return err
	}

	return nil
}

// Setup the secure boot certificates of a server via BMC.
type cmdServerBMCSetupSecureBootCertificates struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerBMCSetupSecureBootCertificates) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "setup-secure-boot-certificates <name>"
	cmd.Short = "Setup the secure boot certificates of a server via BMC"
	cmd.Long = `Description:
  Setup the secure boot certificates of a server via BMC

  Wipes the KEK, DB and DBX secure boot databases of the server and
  reinitializes them with the configured certificates.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerBMCSetupSecureBootCertificates) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerBMCSetupSecureBootCertificates) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.BMCSetupSecureBootCertificates(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

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
