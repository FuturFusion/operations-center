package provisioning

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
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
	tokenRemoveCmd := cmddTokenRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenRemoveCmd.Command())

	// Show
	tokenShowCmd := cmddTokenShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(tokenShowCmd.Command())

	return cmd
}

// Add token.
type cmdTokenAdd struct {
	ocClient *client.OperationsCenterClient

	uses          int
	validDuration time.Duration
	description   string
}

func (c *cmdTokenAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add"
	cmd.Short = "Add a new token"
	cmd.Long = `Description:
  Add a new token

  Adds a new custer to the operations center.
`

	cmd.RunE = c.Run

	cmd.Flags().IntVar(&c.uses, "uses", 1, "Allowed count of uses for the token")
	cmd.Flags().DurationVar(&c.validDuration, "lifetime", 24*30*time.Hour, "Lifetime of the token as duration")
	cmd.Flags().StringVar(&c.description, "description", "", "Description of the token")

	cmd.PreRunE = c.ValidateFlags

	return cmd
}

func (c *cmdTokenAdd) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	err = c.ocClient.CreateToken(api.TokenPut{
		UsesRemaining: c.uses,
		ExpireAt:      time.Now().Add(c.validDuration),
		Description:   c.description,
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *cmdTokenAdd) ValidateFlags(cmd *cobra.Command, _ []string) error {
	if c.uses <= 0 {
		return fmt.Errorf(`Value for flag "--uses" needs to be greater or equal to 1`)
	}

	if c.validDuration <= 0 {
		return fmt.Errorf(`Value for flag "--lifetime" needs to be greater or equal to 1`)
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

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdTokenList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	tokens, err := c.ocClient.GetTokens()
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"UUID", "Uses Remaining", "Expire At", "Description"}
	data := [][]string{}

	for _, token := range tokens {
		data = append(data, []string{token.UUID.String(), strconv.FormatInt(int64(token.UsesRemaining), 10), token.ExpireAt.String(), token.Description})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, tokens)
}

// Remove token.
type cmddTokenRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddTokenRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <uuid>"
	cmd.Short = "Remove a token"
	cmd.Long = `Description:
  Remove a token

  Removes a custer from the operations center.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddTokenRemove) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	id := args[0]

	err = c.ocClient.DeleteToken(id)
	if err != nil {
		return err
	}

	return nil
}

// Show token.
type cmddTokenShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmddTokenShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <uuid>"
	cmd.Short = "Show information about a token"
	cmd.Long = `Description:
  Show information about a token.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmddTokenShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	id := args[0]

	token, err := c.ocClient.GetToken(id)
	if err != nil {
		return err
	}

	fmt.Printf("UUID: %s\n", token.UUID.String())
	fmt.Printf("Uses Remaining: %s\n", strconv.FormatInt(int64(token.UsesRemaining), 10))
	fmt.Printf("Expire At: %s\n", token.ExpireAt.String())
	fmt.Printf("Description: %s\n", token.Description)

	return nil
}
