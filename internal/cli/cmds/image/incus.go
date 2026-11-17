package image

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/lxc/incus/v7/shared/termios"
	"github.com/lxc/incus/v7/shared/units"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/util/archive"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/maps"
	"github.com/FuturFusion/operations-center/internal/util/multipartstreamer"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdIncusImage struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdIncusImage) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "incus"
	cmd.Short = "Interact with Incus image sources and their images"
	cmd.Long = `Description:
  Interact with Incus image sources and their images

  Manage Incus image sources. The images of a source are managed with the
  "image" sub command. The source "local" holds the manually uploaded images.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	addImageSourceCommands(cmd, c.OCClient)

	imagesCmd := cmdIncusImages{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(imagesCmd.Command())

	return cmd
}

// Incus images of a source.
type cmdIncusImages struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImages) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "image"
	cmd.Short = "Interact with the Incus images of an image source"
	cmd.Long = `Description:
  Interact with the Incus images of an image source

  Manage the Incus images of an image source. Only the images of the source
  "local" can be added, changed and removed. The images of the other sources
  follow the filter expression of their source.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	incusImageListCmd := cmdIncusImagesList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(incusImageListCmd.Command())

	// Show
	incusImageShowCmd := cmdIncusImageShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(incusImageShowCmd.Command())

	// Add
	incusImageAddCmd := cmdIncusImageAdd{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(incusImageAddCmd.Command())

	// Edit
	incusImageEditCmd := cmdIncusImageEdit{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(incusImageEditCmd.Command())

	// Remove
	incusImageRemoveCmd := cmdIncusImageRemove{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(incusImageRemoveCmd.Command())

	// Remove version
	incusImageVersionRemoveCmd := cmdIncusImageVersionRemove{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(incusImageVersionRemoveCmd.Command())

	// Get File
	incusImageVersionGetFileCmd := cmdIncusImageVersionGetFile{
		ocClient: c.ocClient,
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
	cmd.Use = "list <source>"
	cmd.Short = "List the Incus images of an image source"
	cmd.Long = `Description:
  List the Incus images of an image source
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImagesList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdIncusImagesList) run(cmd *cobra.Command, args []string) error {
	incusImages, err := c.ocClient.GetIncusImages(cmd.Context(), args[0])
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

	flagFormat string
}

func (c *cmdIncusImageShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <source> <name>"
	cmd.Short = "Show information about an Incus image"
	cmd.Long = `Description:
  Show information about an Incus image.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	validFormats := []string{"", "json", "yaml"}
	if !slices.Contains(validFormats, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdIncusImageShow) run(cmd *cobra.Command, args []string) error {
	source := args[0]
	name := args[1]

	incusImage, err := c.ocClient.GetIncusImage(cmd.Context(), source, name)
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent("", "  ")
		err = enc.Encode(incusImage)
		if err != nil {
			return err
		}

	case "yaml":
		enc := yaml.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent(2)
		err = enc.Encode(incusImage)
		if err != nil {
			return err
		}

	default:
		fmt.Printf("Name: %s\n", incusImage.Name)
		fmt.Printf("Aliases:\n")
		for _, alias := range incusImage.Aliases {
			fmt.Printf("  - %s\n", alias)
		}

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
	}

	return nil
}

// Add incus image.
type cmdIncusImageAdd struct {
	ocClient *client.OperationsCenterClient

	flagOS           string
	flagRelease      string
	flagArchitecture string
	flagVariant      string
	flagVersion      string
}

func (c *cmdIncusImageAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <metadata tarball> <file> [<file> ...]"
	cmd.Short = "Add an Incus image version"
	cmd.Long = `Description:
  Add an Incus image version to the image source "local".

  The required metadata is either provided through a metadata tarball, which
  is the first positional argument, or through the respective flags (e.g.
  --os). The two variants are mutually exclusive, if any of --os, --arch or
  --image-version is set, all the required flags have to be set and every
  positional argument is an image file.

  The metadata tarball is the file, "incus image export" writes as <target>,
  Incus itself names it incus.tar.xz. Its name does not matter, it is
  recognized by its content.

  The type of the image files is detected from their content.
`

	cmd.Flags().StringVar(&c.flagOS, "os", "", "Operating system name of the image version (required, if no metadata tarball is provided)")
	cmd.Flags().StringVar(&c.flagRelease, "release", "current", "Release identifier of the image version")
	cmd.Flags().StringVar(&c.flagArchitecture, "arch", "", "Architecture of the image version (required, if no metadata tarball is provided)")
	cmd.Flags().StringVar(&c.flagVariant, "variant", "default", "Variant of the image version")
	cmd.Flags().StringVar(&c.flagVersion, "image-version", "", "Version of the image (required, if no metadata tarball is provided)")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageAdd) hasMetadataFlags() bool {
	return c.flagOS != "" || c.flagArchitecture != "" || c.flagVersion != ""
}

func (c *cmdIncusImageAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, -1)
	if exit {
		return err
	}

	if !c.hasMetadataFlags() {
		// The first argument is the metadata tarball, so at least one image
		// file has to follow it.
		if len(args) < 2 {
			return fmt.Errorf("Either provide the image attributes through a metadata tarball followed by at least one image file or pass all the required flags")
		}

		return nil
	}

	if c.flagOS == "" || c.flagRelease == "" || c.flagArchitecture == "" || c.flagVariant == "" || c.flagVersion == "" {
		return fmt.Errorf("Either provide the image attributes through a metadata tarball or pass all the required flags")
	}

	err = api.ValidateIncusImageArchitecture(c.flagArchitecture)
	if err != nil {
		return err
	}

	err = api.ValidateIncusImageVersion(c.flagVersion)
	if err != nil {
		return err
	}

	return nil
}

func (c *cmdIncusImageAdd) assertMetadataTarball(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	extension, _, err := archive.DetectType(f)
	if err != nil {
		return fmt.Errorf("Failed to read %q: %w", filename, err)
	}

	if extension != ".tar.xz" {
		return fmt.Errorf("The first argument %q is not a metadata tarball, expected an xz compressed tarball containing a metadata.yaml", filename)
	}

	return nil
}

func (c *cmdIncusImageAdd) run(cmd *cobra.Command, args []string) (err error) {
	var mr client.ContentTypeReadCloser

	if c.hasMetadataFlags() {
		metadata := api.IncusImagePost{
			OperatingSystem: c.flagOS,
			Release:         c.flagRelease,
			Architecture:    c.flagArchitecture,
			Variant:         c.flagVariant,
			Version:         c.flagVersion,
		}

		requestJSON, err := json.Marshal(metadata)
		if err != nil {
			return err
		}

		mr = multipartstreamer.NewWithFields(map[string]string{
			"request_json": string(requestJSON),
		}, args...)
	} else {
		// The metadata tarball is the first argument and therefore becomes the
		// first part of the multipart request.
		err = c.assertMetadataTarball(args[0])
		if err != nil {
			return err
		}

		mr = multipartstreamer.New(args...)
	}

	defer func() {
		closeErr := mr.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	err = c.ocClient.CreateIncusImageVersion(cmd.Context(), mr)
	if err != nil {
		return fmt.Errorf("Failed to create Incus image version: %w", err)
	}

	return nil
}

// Edit incus image.
type cmdIncusImageEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <source> <name>"
	cmd.Short = "Edit Incus image"
	cmd.Long = `Description:
  Edit the Incus image

  Changes the description and the aliases of an Incus image. Only the images
  of the source "local" can be changed.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing incus image settings.
func (c *cmdIncusImageEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### aliases: []
### description: ""
`
}

func (c *cmdIncusImageEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageEdit) run(cmd *cobra.Command, args []string) error {
	source := args[0]
	name := args[1]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.IncusImagePut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateIncusImage(cmd.Context(), source, name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	serverConfig, err := c.ocClient.GetIncusImage(cmd.Context(), source, name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(serverConfig.IncusImagePut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.IncusImagePut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateIncusImage(cmd.Context(), source, name, newdata)
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

// Remove incus image.
type cmdIncusImageRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <source> <name>"
	cmd.Short = "Remove an Incus image"
	cmd.Long = `Description:
  Remove an Incus image

  Removes an Incus image from the operations center. Only the images of the
  source "local" can be removed. For the other sources, change the filter
  expression of the source.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageRemove) run(cmd *cobra.Command, args []string) error {
	source := args[0]
	name := args[1]

	err := c.ocClient.DeleteIncusImage(cmd.Context(), source, name)
	if err != nil {
		return err
	}

	return nil
}

// Remove incus image version.
type cmdIncusImageVersionRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdIncusImageVersionRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove-version <source> <name> <version>"
	cmd.Short = "Remove an Incus image version"
	cmd.Long = `Description:
  Remove an Incus image version

  Removes an Incus image version from the operations center. Only the
  versions of images of the source "local" can be removed. For the other
  sources, change the filter expression of the source.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageVersionRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageVersionRemove) run(cmd *cobra.Command, args []string) error {
	source := args[0]
	name := args[1]
	version := args[2]

	err := c.ocClient.DeleteIncusImageVersion(cmd.Context(), source, name, version)
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
	cmd.Use = "file <source> <name> <version> <filename> <target-filename>"
	cmd.Short = "Get Incus image version file"
	cmd.Long = `Description:
  Get a file of an Incus image version.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdIncusImageVersionGetFile) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 5, 5)
	if exit {
		return err
	}

	return nil
}

func (c *cmdIncusImageVersionGetFile) run(cmd *cobra.Command, args []string) error {
	source := args[0]
	name := args[1]
	version := args[2]
	filename := args[3]
	targetFilename := args[4]

	if file.PathExists(targetFilename) {
		return fmt.Errorf("target file %q already exists", targetFilename)
	}

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer targetFile.Close()

	fileReader, err := c.ocClient.GetIncusImageVersionFile(cmd.Context(), source, name, version, filename)
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
