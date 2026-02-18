package provisioning

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/lxc/incus-os/incus-osd/api/images"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdToken struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdToken) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "token"
	cmd.Short = "Interact with tokens"
	cmd.Long = `Description:
  Interact with tokens

  Configure tokens for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	tokenAddCmd := cmdTokenAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenAddCmd.Command())

	// List
	tokenListCmd := cmdTokenList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenListCmd.Command())

	// Remove
	tokenRemoveCmd := cmdTokenRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenRemoveCmd.Command())

	// Show
	tokenShowCmd := cmdTokenShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenShowCmd.Command())

	// Get Image
	tokenGetImageCmd := cmdTokenGetImage{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenGetImageCmd.Command())

	// Token seed sub-command
	tokenSeedCmd := cmdTokenSeed{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenSeedCmd.Command())

	return cmd
}

// Add token.
type cmdTokenAdd struct {
	ocClient *client.OperationsCenterClient

	uses          int
	validDuration time.Duration
	description   string
	channel       string
}

func (c *cmdTokenAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add"
	cmd.Short = "Add a new token"
	cmd.Long = `Description:
  Add a new token

  Adds a new token to the operations center.
`

	cmd.Flags().IntVar(&c.uses, "uses", 1, "Allowed count of uses for the token")
	cmd.Flags().DurationVar(&c.validDuration, "lifetime", 24*30*time.Hour, "Lifetime of the token as duration")
	cmd.Flags().StringVar(&c.description, "description", "", "Description of the token")
	cmd.Flags().StringVar(&c.channel, "channel", "", "Update channel, servers provisioned using this token should be assigned to")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokenAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	if c.uses <= 0 {
		return fmt.Errorf(`Value for flag "--uses" needs to be greater or equal to 1`)
	}

	if c.validDuration <= 0 {
		return fmt.Errorf(`Value for flag "--lifetime" needs to be greater or equal to 1`)
	}

	return nil
}

func (c *cmdTokenAdd) run(cmd *cobra.Command, args []string) error {
	err := c.ocClient.CreateToken(cmd.Context(), api.TokenPut{
		UsesRemaining: c.uses,
		ExpireAt:      time.Now().Add(c.validDuration),
		Description:   c.description,
		Channel:       c.channel,
	})
	if err != nil {
		return err
	}

	return nil
}

// List tokens.
type cmdTokenList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdTokenList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available tokens"
	cmd.Long = `Description:
  List the available tokens
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokenList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdTokenList) run(cmd *cobra.Command, args []string) error {
	tokens, err := c.ocClient.GetTokens(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"UUID", "Uses Remaining", "Expire At", "Channel", "Description"}
	data := [][]string{}

	for _, token := range tokens {
		data = append(data, []string{token.UUID.String(), strconv.FormatInt(int64(token.UsesRemaining), 10), token.ExpireAt.Truncate(time.Second).String(), token.Channel, token.Description})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // UUID
			Less:  sort.StringLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, tokens)
}

// Remove token.
type cmdTokenRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokenRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <uuid>"
	cmd.Short = "Remove a token"
	cmd.Long = `Description:
  Remove a token

  Removes a token from the operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokenRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdTokenRemove) run(cmd *cobra.Command, args []string) error {
	id := args[0]

	err := c.ocClient.DeleteToken(cmd.Context(), id)
	if err != nil {
		return err
	}

	return nil
}

// Show token.
type cmdTokenShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdTokenShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid>"
	cmd.Short = "Show information about a token"
	cmd.Long = `Description:
  Show information about a token.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokenShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdTokenShow) run(cmd *cobra.Command, args []string) error {
	id := args[0]

	token, err := c.ocClient.GetToken(cmd.Context(), id)
	if err != nil {
		return err
	}

	fmt.Printf("UUID: %s\n", token.UUID.String())
	fmt.Printf("Uses Remaining: %s\n", strconv.FormatInt(int64(token.UsesRemaining), 10))
	fmt.Printf("Expire At: %s\n", token.ExpireAt.Truncate(time.Second).String())
	fmt.Printf("Channel: %s\n", token.Channel)
	fmt.Printf("Description: %s\n", token.Description)

	return nil
}

// Get image for token.
type cmdTokenGetImage struct {
	ocClient *client.OperationsCenterClient

	flagImageType    string
	flagArchitecture string
	flagApplications []string
}

func (c *cmdTokenGetImage) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "get-image <uuid> <target-file> [pre-seed.yaml]"
	cmd.Short = "Get a pre-seeded ISO or raw image for a token"
	cmd.Long = `Description:
  Get a pre-seeded ISO or raw image for a token.
`

	cmd.Flags().StringVar(&c.flagImageType, "type", "iso", "type of image (iso|raw)")
	cmd.Flags().StringVar(&c.flagArchitecture, "architecture", "x86_64", "CPU architecture for the image (x86_64|aarch64)")
	cmd.Flags().StringSliceVar(&c.flagApplications, "application", []string{}, "Applications to be seeded in the image, e.g. incus, migration-manager, non-primary applications")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdTokenGetImage) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 3)
	if exit {
		return err
	}

	switch c.flagImageType {
	case api.ImageTypeISO.String(), api.ImageTypeRaw.String():
	default:
		return fmt.Errorf(`Invalid value for flag "--type": %q`, c.flagImageType)
	}

	_, ok := images.UpdateFileArchitectures[images.UpdateFileArchitecture(c.flagArchitecture)]
	if !ok {
		return fmt.Errorf(`Invalid value for flag "--architecture": %q`, c.flagArchitecture)
	}

	var primaryApplicationCount int
	for _, application := range c.flagApplications {
		fileComponent := images.UpdateFileComponent(application)
		_, ok := images.UpdateFileComponents[fileComponent]
		if !ok {
			return fmt.Errorf(`Invalid value for flag "--application": %q`, application)
		}

		if fileComponent == images.UpdateFileComponentOperationsCenter {
			return fmt.Errorf(`Deploying operations-center through operations-center is not supported`)
		}

		switch fileComponent {
		case images.UpdateFileComponentIncus, images.UpdateFileComponentMigrationManager:
			primaryApplicationCount++
		}
	}

	if len(c.flagApplications) > 0 && primaryApplicationCount != 1 {
		return fmt.Errorf(`Exactly one primary application (incus, migration-manager) is required`)
	}

	return nil
}

func (c *cmdTokenGetImage) run(cmd *cobra.Command, args []string) (err error) {
	id := args[0]
	targetFilename := args[1]

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

	preseed := api.TokenImagePost{
		Type:         imageType,
		Architecture: architecture,
	}

	if len(c.flagApplications) > 0 {
		applications := make([]any, 0, len(c.flagApplications))
		for _, application := range c.flagApplications {
			applications = append(applications, map[string]any{
				"name": application,
			})
		}

		preseed.Seeds.Applications = map[string]any{
			"version":      "1",
			"applications": applications,
		}
	}

	if len(args) == 3 {
		body, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &preseed.Seeds)
		if err != nil {
			return err
		}
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

	imageID, err := c.ocClient.PreparePreSeededImage(cmd.Context(), id, preseed)
	if err != nil {
		return err
	}

	imageReader, err := c.ocClient.GetPreSeededImage(cmd.Context(), id, imageID)
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
