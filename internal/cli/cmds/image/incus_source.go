package image

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lxc/incus/v7/shared/termios"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdSource struct {
	ocClient *client.OperationsCenterClient
}

func (c *CmdSource) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "source"
	cmd.Short = "Interact with image sources"
	cmd.Long = `Description:
  Interact with image sources

  Manage image sources.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	imageSourceListCmd := cmdImageSourceList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(imageSourceListCmd.Command())

	// Show
	imageSourceShowCmd := cmdImageSourceShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(imageSourceShowCmd.Command())

	// Add
	imageSourceAddCmd := cmdImageSourceAdd{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(imageSourceAddCmd.Command())

	// Edit
	imageSourceEditCmd := cmdImageSourceEdit{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(imageSourceEditCmd.Command())

	// Remove
	imageSourceRemoveCmd := cmdImageSourceRemove{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(imageSourceRemoveCmd.Command())

	// Refresh
	imageSourceRefreshCmd := cmdImageSourceRefresh{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(imageSourceRefreshCmd.Command())

	return cmd
}

// List image sources.
type cmdImageSourceList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdImageSourceList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available image sources"
	cmd.Long = `Description:
  List the available image sources
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdImageSourceList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdImageSourceList) run(cmd *cobra.Command, args []string) error {
	imageSource, err := c.ocClient.GetImageIncusSources(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "URL", "Filter Expression", "Last Updated"}
	data := [][]string{}

	for _, imageSource := range imageSource {
		data = append(data, []string{imageSource.Name, imageSource.URL, imageSource.FilterExpression, imageSource.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // Name
			Less:  sort.NaturalLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, imageSource)
}

// Show image source.
type cmdImageSourceShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdImageSourceShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about an image source"
	cmd.Long = `Description:
  Show information about an image source.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdImageSourceShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdImageSourceShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	imageSource, err := c.ocClient.GetImageIncusSource(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", imageSource.Name)
	fmt.Printf("URL: %s\n", imageSource.URL)
	fmt.Printf("Filter Expression: %s\n", imageSource.FilterExpression)
	fmt.Printf("Last Updated: %s\n", imageSource.LastUpdated.Truncate(time.Second).String())

	return nil
}

// Add image source.
type cmdImageSourceAdd struct {
	ocClient *client.OperationsCenterClient

	flagFilterExpression string
}

func (c *cmdImageSourceAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name> <url>"
	cmd.Short = "Add an image source"
	cmd.Long = `Description:
  Add an image source.
`

	cmd.Flags().StringVarP(&c.flagFilterExpression, "filter", "f", "", `Filter expression applied to filter images fetched from the image source`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdImageSourceAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	if c.flagFilterExpression == "" {
		return fmt.Errorf(`Filter expression can not be empty. To allow all images from being fetch, use "true" as the filter expression.`)
	}

	return nil
}

func (c *cmdImageSourceAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	url := args[1]

	imageSource := api.ImageSourcePost{
		Name: name,
		ImageSourcePut: api.ImageSourcePut{
			URL:              url,
			FilterExpression: c.flagFilterExpression,
		},
	}

	err := c.ocClient.CreateImageIncusSource(cmd.Context(), imageSource)
	if err != nil {
		return fmt.Errorf("Failed to create image source: %w", err)
	}

	return nil
}

// Edit image source.
type cmdImageSourceEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdImageSourceEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit image source"
	cmd.Long = `Description:
  Edit the image source
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing image source settings.
func (c *cmdImageSourceEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### url: ""
### type: incus
### filter_expression: ""
`
}

func (c *cmdImageSourceEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdImageSourceEdit) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ImageSourcePut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateImageIncusSource(cmd.Context(), name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	serverConfig, err := c.ocClient.GetImageIncusSource(cmd.Context(), name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(serverConfig.ImageSourcePut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.ImageSourcePut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateImageIncusSource(cmd.Context(), name, newdata)
		}

		// Respawn the editor
		if err != nil {
			fmt.Fprintf(os.Stderr, "Config parsing error: %s\n", err)
			fmt.Println("Press enter to open the editor again or ctrl+c to abort change")

			_, err := os.Stdin.Read(make([]byte, 1))
			if err != nil {
				return err
			}

			content, err = editor.Spawn("", content)
			if err != nil {
				return err
			}

			continue
		}

		break
	}

	return nil
}

// Remove image source.
type cmdImageSourceRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdImageSourceRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove an image source"
	cmd.Long = `Description:
  Remove an image source

  Removes an image source from operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdImageSourceRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdImageSourceRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.DeleteImageIncusSource(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Refresh image source.
type cmdImageSourceRefresh struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdImageSourceRefresh) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "refresh <name>"
	cmd.Short = "Refresh an image source"
	cmd.Long = `Description:
  Refresh an image source

  Trigger a refresh for an image source to align the local state with the
  upstream state.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdImageSourceRefresh) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdImageSourceRefresh) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.RefreshImageIncusSource(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}
