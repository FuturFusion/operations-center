package provisioning

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
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
	updateShowCmd := cmddUpdateShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateShowCmd.Command())

	// Files
	updateFilesCmd := cmdUpdateFiles{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateFilesCmd.Command())

	return cmd
}

// List updates.
type cmdUpdateList struct {
	ocClient *client.OperationsCenterClient

	flagFilterChannel string

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

	updates, err := c.ocClient.GetWithFilterUpdates(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"UUID", "Channel", "Version", "Published At", "Severity", "Components"}
	data := [][]string{}

	for _, update := range updates {
		data = append(data, []string{update.UUID.String(), update.Channel, update.Version, update.PublishedAt.String(), update.Severity.String(), update.Components.String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, updates)
}

// Show update.
type cmddUpdateShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddUpdateShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid>"
	cmd.Short = "Show information about a update"
	cmd.Long = `Description:
  Show information about a update.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddUpdateShow) Run(cmd *cobra.Command, args []string) error {
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
	fmt.Printf("Channel: %s\n", update.Channel)
	fmt.Printf("Version: %s\n", update.Version)
	fmt.Printf("Published At: %s\n", update.PublishedAt.String())
	fmt.Printf("Severity: %s\n", update.Severity)
	fmt.Printf("Components: %s\n", update.Components)
	fmt.Println("Files:")

	for _, updateFile := range updateFiles {
		fmt.Printf("- %s (%s)\n", updateFile.Filename, humanize.Bytes(uint64(updateFile.Size)))
	}

	return nil
}

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
	updateFileShowCmd := cmddUpdateFileShow{
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
	header := []string{"Filename", "Size", "URL"}
	data := [][]string{}

	for _, updateFile := range updateFiles {
		data = append(data, []string{updateFile.Filename, humanize.Bytes(uint64(updateFile.Size)), updateFile.URL})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, updateFiles)
}

// Show updateFile.
type cmddUpdateFileShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddUpdateFileShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid> <filename>"
	cmd.Short = "Show information about a update file"
	cmd.Long = `Description:
  Show information about a update file.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddUpdateFileShow) Run(cmd *cobra.Command, args []string) error {
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
	fmt.Printf("URL: %s\n", updateFile.URL)

	return nil
}
