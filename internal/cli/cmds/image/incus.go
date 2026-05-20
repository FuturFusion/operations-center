package image

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/lxc/incus/v6/shared/units"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/maps"
	"github.com/FuturFusion/operations-center/internal/util/multipartstreamer"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
)

type CmdIncusImage struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdIncusImage) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "incus"
	cmd.Short = "Interact with incus images"
	cmd.Long = `Description:
  Interact with incus images

  Manage incus images.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	incusImageListCmd := cmdIncusImagesList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(incusImageListCmd.Command())

	// Show
	incusImageShowCmd := cmdIncusImageShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(incusImageShowCmd.Command())

	// Add
	incusImageAddCmd := cmdIncusImageAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(incusImageAddCmd.Command())

	// Remove
	incusImageRemoveCmd := cmdIncusImageRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(incusImageRemoveCmd.Command())

	// Remove
	incusImageVersionRemoveCmd := cmdIncusImageVersionRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(incusImageVersionRemoveCmd.Command())

	// Get File
	incusImageVersionGetFileCmd := cmdIncusImageVersionGetFile{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(incusImageVersionGetFileCmd.Command())

	return cmd
}

// List incus images.
type cmdIncusImagesList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdIncusImagesList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available incus images"
	cmd.Long = `Description:
  List the available incus images
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImagesList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdIncusImagesList) run(cmd *cobra.Command, args []string) error {
	incusImages, err := c.ocClient.GetIncusImages(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Operating System", "Release", "Architecture", "Variant", "Description", "Last Updated"}
	data := [][]string{}

	for _, incusImage := range incusImages {
		data = append(data, []string{incusImage.Name, incusImage.OperatingSystem, incusImage.Release, incusImage.Architecture, incusImage.Variant, incusImage.Description, incusImage.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // Name
			Less:  sort.NaturalLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, incusImages)
}

// Show incus image.
type cmdIncusImageShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about an incus image"
	cmd.Long = `Description:
  Show information about an incus image.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	incusImage, err := c.ocClient.GetIncusImage(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", incusImage.Name)
	fmt.Printf("Operating system: %s\n", incusImage.OperatingSystem)
	fmt.Printf("Release: %s\n", incusImage.Release)
	fmt.Printf("Architecture: %s\n", incusImage.Architecture)
	fmt.Printf("Variant: %s\n", incusImage.Variant)
	fmt.Printf("Description: %s\n", incusImage.Description)
	fmt.Printf("Last Updated: %s\n", incusImage.LastUpdated.Truncate(time.Second).String())

	fmt.Printf("Versions:\n")
	for versionIdentifier, imageVersion := range maps.OrderedByKey(incusImage.Versions) {
		fmt.Printf("  %s:\n", versionIdentifier)
		for filename, item := range maps.OrderedByKey(imageVersion.Items) {
			fmt.Printf("    %s: %s\n", filename, units.GetByteSizeString(item.Size, 2))
		}
	}

	return nil
}

// Add incus image.
type cmdIncusImageAdd struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name> <version> <file> [<file> ...]"
	cmd.Short = "Add a incus image version"
	cmd.Long = `Description:
  Add an incus image version.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, -1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageAdd) run(cmd *cobra.Command, args []string) (err error) {
	name := args[0]
	version := args[1]

	mr := multipartstreamer.New(args[2:]...)
	defer func() {
		closeErr := mr.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	err = c.ocClient.CreateIncusImageVersion(cmd.Context(), name, version, mr)
	if err != nil {
		return fmt.Errorf("Failed to create incus image version %s/%s: %w", name, version, err)
	}

	return nil
}

// Remove incus image.
type cmdIncusImageRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove an incus image"
	cmd.Long = `Description:
  Remove an incus image

  Removes an incus image from the operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.DeleteIncusImage(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Remove incus image.
type cmdIncusImageVersionRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageVersionRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove-version <name>"
	cmd.Short = "Remove an incus image version version"
	cmd.Long = `Description:
  Remove an incus image version

  Removes an incus image version from the operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageVersionRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageVersionRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	version := args[1]

	err := c.ocClient.DeleteIncusImageVersion(cmd.Context(), name, version)
	if err != nil {
		return err
	}

	return nil
}

// Get incus image version file.
type cmdIncusImageVersionGetFile struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageVersionGetFile) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "file <name> <version> <filename> <target-filename>"
	cmd.Short = "Get incus image version file"
	cmd.Long = `Description:
  Get a file of an incus image version.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageVersionGetFile) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 4, 4)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageVersionGetFile) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	version := args[1]
	filename := args[2]
	targetFilename := args[3]

	if file.PathExists(targetFilename) {
		return fmt.Errorf("target file %q already exists", targetFilename)
	}

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer targetFile.Close()

	fileReader, err := c.ocClient.GetIncusImageVersionFile(cmd.Context(), name, version, filename)
	if err != nil {
		return err
	}

	defer fileReader.Close()

	quiet, _ := cmd.Flags().GetBool("quiet")
	format := fmt.Sprintf("Fetching file %q for image %q, version %q: %%s", filename, name, version)

	progress, writer := render.ProgressWriter(targetFile, format, quiet)

	size, err := file.SafeCopy(writer, fileReader)
	if err != nil {
		return err
	}

	progress.Done(fmt.Sprintf("Successfully written %s to %q ", units.GetByteSizeString(size, 2), targetFilename))

	return nil
}
