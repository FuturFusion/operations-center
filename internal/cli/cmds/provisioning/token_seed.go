package provisioning

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/lxc/incus/v6/shared/termios"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/editor"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type cmdTokenSeed struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokenSeed) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "seed"
	cmd.Short = "Interact with token seeds"
	cmd.Long = `Description:
  Interact with token seeds

  Configure token seeds for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	tokenSeedAddCmd := cmdTokenSeedAdd{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(tokenSeedAddCmd.Command())

	// List
	tokenListCmd := cmdTokenSeedList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(tokenListCmd.Command())

	// Edit
	tokenEditCmd := cmdTokenSeedEdit{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(tokenEditCmd.Command())

	// Remove
	tokenRemoveCmd := cmdTokenSeedRemove{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(tokenRemoveCmd.Command())

	// Show
	tokenSeedShowCmd := cmdTokenSeedShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(tokenSeedShowCmd.Command())

	// Get Image
	tokenSeedGetImageCmd := cmdTokenSeedGetImage{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(tokenSeedGetImageCmd.Command())

	return cmd
}

// Add token seed.
type cmdTokenSeedAdd struct {
	ocClient *client.OperationsCenterClient

	public      bool
	description string
}

func (c *cmdTokenSeedAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <uuid> <name> <pre-seed.yaml>"
	cmd.Short = "Add a new token seed"
	cmd.Long = `Description:
  Add a new token seed

  Adds a new token seed to the operations center.
`

	cmd.RunE = c.Run

	cmd.Flags().BoolVar(&c.public, "public", false, "Is fetching images based on this configuration allowed without authentication")
	cmd.Flags().StringVar(&c.description, "description", "", "Description of the token")

	return cmd
}

func (c *cmdTokenSeedAdd) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	id := args[0]
	name := args[1]

	tokenSeedPost := api.TokenSeedPost{
		Name: name,
		TokenSeedPut: api.TokenSeedPut{
			Public:      c.public,
			Description: c.description,
		},
	}

	body, err := os.ReadFile(args[2])
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(body, &tokenSeedPost.Seeds)
	if err != nil {
		return err
	}

	err = c.ocClient.CreateTokenSeed(cmd.Context(), id, tokenSeedPost)
	if err != nil {
		return err
	}

	return nil
}

// List token seeds.
type cmdTokenSeedList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdTokenSeedList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list <uuid>"
	cmd.Short = "List available token seed configurations"
	cmd.Long = `Description:
  List the available seed configurations for the given token
`

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdTokenSeedList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	id := args[0]

	tokenSeeds, err := c.ocClient.GetTokenSeeds(cmd.Context(), id)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Token", "Name", "Public", "Description", "Last Updated"}
	data := [][]string{}

	for _, tokenSeed := range tokenSeeds {
		data = append(data, []string{tokenSeed.Token.String(), tokenSeed.Name, strconv.FormatBool(tokenSeed.Public), tokenSeed.Description, tokenSeed.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // UUID
			Less:  sort.StringLess,
		},
		{
			Index: 1, // Name
			Less:  sort.NaturalLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, tokenSeeds)
}

// Edit token seeds.
type cmdTokenSeedEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokenSeedEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <uuid> <name>"
	cmd.Short = "Edit available token seed configuration"
	cmd.Long = `Description:
  Edit the seed configuration for the given token
`

	cmd.RunE = c.Run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing token seed configurations.
func (c *cmdTokenSeedEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### description: ""
### public: false
### seeds:
###   applications: {}
###   network: {}
###   install: {}
`
}

func (c *cmdTokenSeedEdit) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	id := args[0]
	name := args[1]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.TokenSeedPut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateTokenSeed(cmd.Context(), id, name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	tokenSeedConfig, err := c.ocClient.GetTokenSeed(cmd.Context(), id, name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(tokenSeedConfig.TokenSeedPut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.TokenSeedPut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateTokenSeed(cmd.Context(), id, name, newdata)
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

// Remove token seed.
type cmdTokenSeedRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokenSeedRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <uuid> <name>"
	cmd.Short = "Remove a token seed configuration"
	cmd.Long = `Description:
  Remove a token seed configuration

  Removes a token seed configuration from the operations center.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdTokenSeedRemove) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	id := args[0]
	name := args[1]

	err = c.ocClient.DeleteTokenSeed(cmd.Context(), id, name)
	if err != nil {
		return err
	}

	return nil
}

// Show token seed.
type cmdTokenSeedShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokenSeedShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid> <name>"
	cmd.Short = "Show information about a token seed"
	cmd.Long = `Description:
  Show information about a token seed.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdTokenSeedShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	id := args[0]
	name := args[1]

	tokenSeed, err := c.ocClient.GetTokenSeed(cmd.Context(), id, name)
	if err != nil {
		return err
	}

	seeds, err := yaml.Marshal(tokenSeed.Seeds)
	if err != nil {
		return fmt.Errorf("Invalid token seed: %w", err)
	}

	fmt.Printf("Token : %s\n", tokenSeed.Token)
	fmt.Printf("Name: %s\n", tokenSeed.Name)
	fmt.Printf("Public: %t\n", tokenSeed.Public)
	fmt.Printf("Description: %s\n", tokenSeed.Description)
	fmt.Printf("Last updated: %s\n", tokenSeed.LastUpdated.Truncate(time.Second).String())
	fmt.Printf("Seeds:\n%s\n", render.Indent(4, string(seeds)))

	return nil
}

// Get image for token seed.
type cmdTokenSeedGetImage struct {
	ocClient *client.OperationsCenterClient

	flagImageType    string
	flagArchitecture string
}

func (c *cmdTokenSeedGetImage) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "get-image <uuid> <name> <target-file>"
	cmd.Short = "Get a pre-seeded ISO or raw image for a token seed"
	cmd.Long = `Description:
  Get a pre-seeded ISO or raw image for a token seed.
`

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.flagImageType, "type", "iso", "type of image (iso|raw)")
	cmd.Flags().StringVar(&c.flagArchitecture, "architecture", "x86_64", "CPU architecture for the image (x86_64|aarch64)")
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		imageType := cmd.Flag("type").Value.String()
		switch imageType {
		case api.ImageTypeISO.String(), api.ImageTypeRaw.String():
		default:
			return fmt.Errorf(`Invalid value for flag "--type": %q`, imageType)
		}

		architecture := cmd.Flag("architecture").Value.String()
		_, ok := images.UpdateFileArchitectures[images.UpdateFileArchitecture(architecture)]
		if !ok {
			return fmt.Errorf(`Invalid value for flag "--architecture": %q`, architecture)
		}

		return nil
	}

	return cmd
}

func (c *cmdTokenSeedGetImage) Run(cmd *cobra.Command, args []string) (err error) {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	id := args[0]
	name := args[1]
	targetFilename := args[2]

	if file.PathExists(targetFilename) {
		return fmt.Errorf("target file %q already exists", targetFilename)
	}

	var imageType api.ImageType
	err = imageType.UnmarshalText([]byte(c.flagImageType))
	if err != nil {
		return err
	}

	var architecture images.UpdateFileArchitecture
	err = architecture.UnmarshalText([]byte(c.flagArchitecture))
	if err != nil {
		return err
	}

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		closeErr := targetFile.Close()
		var removeErr error
		if err != nil {
			removeErr = os.Remove(targetFilename)
		}

		err = errors.Join(err, closeErr, removeErr)
	}()

	imageReader, err := c.ocClient.GetTokenImageFromSeed(cmd.Context(), id, name, imageType, architecture)
	if err != nil {
		return err
	}

	defer imageReader.Close()

	size, err := io.Copy(targetFile, imageReader)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully written %d bytes to %q\n", size, targetFilename)

	return nil
}
