package provisioning

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdUpdate struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdUpdate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update"
	cmd.Short = "Interact with updates"
	cmd.Long = `Description:
  Interact with updates

  Manage updates provided by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	updateListCmd := cmdUpdateList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateListCmd.Command())

	// Show
	updateShowCmd := cmdUpdateShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateShowCmd.Command())

	// Add
	updateAddCmd := cmdUpdateAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateAddCmd.Command())

	// Files
	updateFilesCmd := cmdUpdateFiles{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateFilesCmd.Command())

	// Cleanup
	updateCleanupCmd := cmdUpdateCleanup{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateCleanupCmd.Command())

	// Refresh
	updateRefreshCmd := cmdUpdateRefresh{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateRefreshCmd.Command())

	return cmd
}

// List updates.
type cmdUpdateList struct {
	ocClient *client.OperationsCenterClient

	flagFilterChannel string
	flagFilterOrigin  string
	flagFilterStatus  string

	flagFormat string
}

func (c *cmdUpdateList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available updates"
	cmd.Long = `Description:
  List the available updates
`

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.flagFilterChannel, "channel", "", "channel name to filter for")
	cmd.Flags().StringVar(&c.flagFilterOrigin, "origin", "", "origin to filter for")
	cmd.Flags().StringVar(&c.flagFilterStatus, "status", "", "status to filter for, valid values: pending, ready")

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdUpdateList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	var filter provisioning.UpdateFilter

	if c.flagFilterChannel != "" {
		filter.Channel = ptr.To(c.flagFilterChannel)
	}

	if c.flagFilterOrigin != "" {
		filter.Origin = ptr.To(c.flagFilterOrigin)
	}

	if c.flagFilterStatus != "" {
		var status api.UpdateStatus
		err = status.UnmarshalText([]byte(c.flagFilterStatus))
		if err != nil {
			return fmt.Errorf("Invalid value for status: %v", err)
		}

		filter.Status = &status
	}

	updates, err := c.ocClient.GetWithFilterUpdates(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"UUID", "Origin", "Channels", "Version", "Published At", "Severity", "Status"}
	data := [][]string{}

	for _, update := range updates {
		data = append(data, []string{update.UUID.String(), update.Origin, strings.Join(update.Channels, ", "), update.Version, update.PublishedAt.Truncate(time.Second).String(), update.Severity.String(), update.Status.String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index:   3, // Version
			Reverse: true,
			Less:    sort.NaturalLess,
		},
		{
			Index: 1, // Origin
			Less:  sort.NaturalLess,
		},
		{
			Index: 0, // UUID
			Less:  sort.StringLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, updates)
}

// Show update.
type cmdUpdateShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid>"
	cmd.Short = "Show information about a update"
	cmd.Long = `Description:
  Show information about a update.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdUpdateShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	id := args[0]

	update, err := c.ocClient.GetUpdate(cmd.Context(), id)
	if err != nil {
		return err
	}

	updateFiles, err := c.ocClient.GetUpdateFiles(cmd.Context(), id)
	if err != nil {
		return err
	}

	fmt.Printf("UUID: %s\n", update.UUID.String())
	fmt.Printf("Origin: %s\n", update.Origin)
	fmt.Printf("Channel: %s\n", strings.Join(update.Channels, ", "))
	fmt.Printf("Version: %s\n", update.Version)
	fmt.Printf("Published At: %s\n", update.PublishedAt.Truncate(time.Second).String())
	fmt.Printf("Severity: %s\n", update.Severity.String())
	fmt.Printf("Status: %s\n", update.Status.String())
	fmt.Printf("Changelog:\n%s", indent("  ", strings.TrimSpace(update.Changelog)))
	fmt.Println("Files:")

	for _, updateFile := range updateFiles {
		fmt.Printf("- %s (%s)\n", updateFile.Filename, humanize.Bytes(uint64(updateFile.Size)))
	}

	return nil
}

// Add update.
type cmdUpdateAdd struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <filename>"
	cmd.Short = "Add an update"
	cmd.Long = `Description:
  Add an update.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdUpdateAdd) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	filename := args[0]

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("Failed to open %q: %w", filename, err)
	}

	err = c.ocClient.CreateUpdate(cmd.Context(), f)
	if err != nil {
		return fmt.Errorf("Failed to create update from %q: %w", filename, err)
	}

	return nil
}

// Cleanup updates.
type cmdUpdateCleanup struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateCleanup) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cleanup"
	cmd.Short = "Cleanup updates"
	cmd.Long = `Description:
  Remove all update artifacts from Operations Center.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdUpdateCleanup) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	err = c.ocClient.CleanupAllUpdates(cmd.Context())
	if err != nil {
		return fmt.Errorf("Failed to cleanup updates: %w", err)
	}

	return nil
}

// Refresh updates.
type cmdUpdateRefresh struct {
	ocClient *client.OperationsCenterClient

	flagWait bool
}

func (c *cmdUpdateRefresh) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "refresh"
	cmd.Short = "Refresh updates"
	cmd.Long = `Description:
  Refresh updates provided by Operations Center.
`

	cmd.RunE = c.Run

	cmd.Flags().BoolVar(&c.flagWait, "wait", false, "wait for the operation to complete")

	return cmd
}

func (c *cmdUpdateRefresh) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	err = c.ocClient.RefreshUpdates(cmd.Context(), c.flagWait)
	if err != nil {
		return fmt.Errorf("Failed to refresh updates: %w", err)
	}

	return nil
}

// File sub-command.
type cmdUpdateFiles struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateFiles) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "file"
	cmd.Short = "Interact with update file"
	cmd.Long = `Description:
  Interact with update file

  Manage update file provided by operations-center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	updateFileListCmd := cmdUpdateFileList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(updateFileListCmd.Command())

	// Show
	updateFileShowCmd := cmdUpdateFileShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(updateFileShowCmd.Command())

	return cmd
}

// List updateFiles.
type cmdUpdateFileList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdUpdateFileList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list <uuid>"
	cmd.Short = "List available update files"
	cmd.Long = `Description:
  List the available update files
`

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdUpdateFileList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	id := args[0]

	updateFiles, err := c.ocClient.GetUpdateFiles(cmd.Context(), id)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Filename", "Size", "SHA256", "Component", "Type", "Architecture"}
	data := [][]string{}

	for _, updateFile := range updateFiles {
		data = append(data, []string{updateFile.Filename, humanize.Bytes(uint64(updateFile.Size)), updateFile.Sha256[:min(len(updateFile.Sha256), 12)], updateFile.Component.String(), updateFile.Type.String(), updateFile.Architecture.String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, updateFiles)
}

// Show updateFile.
type cmdUpdateFileShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdateFileShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid> <filename>"
	cmd.Short = "Show information about a update file"
	cmd.Long = `Description:
  Show information about a update file.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdUpdateFileShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	id := args[0]
	filename := args[1]

	updateFiles, err := c.ocClient.GetUpdateFiles(cmd.Context(), id)
	if err != nil {
		return err
	}

	var updateFile api.UpdateFile
	var found bool

	for _, updateFile = range updateFiles {
		if updateFile.Filename == filename {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("File %q for Update %q not found", filename, id)
	}

	fmt.Printf("Filename: %s\n", updateFile.Filename)
	fmt.Printf("Size: %s\n", humanize.Bytes(uint64(updateFile.Size)))
	fmt.Printf("SHA256: %s\n", updateFile.Sha256)
	fmt.Printf("Component: %s\n", updateFile.Component)
	fmt.Printf("Type: %s\n", updateFile.Type)
	fmt.Printf("Architecture: %s\n", updateFile.Architecture.String())

	return nil
}
