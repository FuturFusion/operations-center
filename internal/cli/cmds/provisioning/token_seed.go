package provisioning

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/render"
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

	// // List
	// tokenListCmd := cmdTokenSeedList{
	// 	ocClient: c.OCClient,
	// }

	// cmd.AddCommand(tokenListCmd.Command())

	// // Remove
	// tokenRemoveCmd := cmdTokenSeedRemove{
	// 	ocClient: c.OCClient,
	// }

	// cmd.AddCommand(tokenRemoveCmd.Command())

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

	tokenImagePost := api.TokenImagesPost{
		Name: name,
		TokenImagesPut: api.TokenImagesPut{
			Public:      c.public,
			Description: c.description,
		},
	}

	body, err := os.ReadFile(args[2])
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(body, &tokenImagePost.Seeds)
	if err != nil {
		return err
	}

	err = c.ocClient.CreateTokenSeed(cmd.Context(), id, tokenImagePost)
	if err != nil {
		return err
	}

	return nil
}

// // List token seeds.
// type cmdTokenSeedList struct {
// 	ocClient *client.OperationsCenterClient

// 	flagFormat string
// }

// func (c *cmdTokenSeedList) Command() *cobra.Command {
// 	cmd := &cobra.Command{}
// 	cmd.Use = "list"
// 	cmd.Short = "List available tokens"
// 	cmd.Long = `Description:
//   List the available tokens
// `

// 	cmd.RunE = c.Run

// 	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
// 	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
// 		return validate.FormatFlag(cmd.Flag("format").Value.String())
// 	}

// 	return cmd
// }

// func (c *cmdTokenSeedList) Run(cmd *cobra.Command, args []string) error {
// 	// Quick checks.
// 	exit, err := validate.Args(cmd, args, 0, 0)
// 	if exit {
// 		return err
// 	}

// 	tokens, err := c.ocClient.GetTokens(cmd.Context())
// 	if err != nil {
// 		return err
// 	}

// 	// Render the table.
// 	header := []string{"UUID", "Uses Remaining", "Expire At", "Description"}
// 	data := [][]string{}

// 	for _, token := range tokens {
// 		data = append(data, []string{token.UUID.String(), strconv.FormatInt(int64(token.UsesRemaining), 10), token.ExpireAt.Truncate(time.Second).String(), token.Description})
// 	}

// 	sort.ColumnsSort(data, []sort.ColumnSorter{
// 		{
// 			Index: 0, // UUID
// 			Less:  sort.StringLess,
// 		},
// 	})

// 	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, tokens)
// }

// // Remove token seed.
// type cmdTokenSeedRemove struct {
// 	ocClient *client.OperationsCenterClient
// }

// func (c *cmdTokenSeedRemove) Command() *cobra.Command {
// 	cmd := &cobra.Command{}
// 	cmd.Use = "remove <uuid>"
// 	cmd.Short = "Remove a token"
// 	cmd.Long = `Description:
//   Remove a token

//   Removes a token from the operations center.
// `

// 	cmd.RunE = c.Run

// 	return cmd
// }

// func (c *cmdTokenSeedRemove) Run(cmd *cobra.Command, args []string) error {
// 	// Quick checks.
// 	exit, err := validate.Args(cmd, args, 1, 1)
// 	if exit {
// 		return err
// 	}

// 	id := args[0]

// 	err = c.ocClient.DeleteToken(cmd.Context(), id)
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

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
	fmt.Printf("Description: %s\n", tokenSeed.Description)
	fmt.Printf("Public: %t\n", tokenSeed.Public)
	fmt.Printf("Last seen: %s\n", tokenSeed.LastUpdated.Truncate(time.Second).String())
	fmt.Printf("Seeds:\n%s\n", render.Indent(4, string(seeds)))

	return nil
}

// Get image for token seed.
type cmdTokenSeedGetImage struct {
	ocClient *client.OperationsCenterClient

	flagImageType string
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
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		imageType := cmd.Flag("type").Value.String()
		switch imageType {
		case api.ImageTypeISO.String(), api.ImageTypeRaw.String():
		default:
			return fmt.Errorf(`Invalid value for flag "--type": %q`, imageType)
		}

		return nil
	}

	return cmd
}

func (c *cmdTokenSeedGetImage) Run(cmd *cobra.Command, args []string) error {
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

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer targetFile.Close()

	imageReader, err := c.ocClient.GetTokenImageFromSeed(cmd.Context(), id, name)
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
