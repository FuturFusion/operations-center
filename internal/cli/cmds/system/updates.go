package system

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/lxc/incus/v6/shared/termios"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdUpdates struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdUpdates) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "updates"
	cmd.Short = "Interact with updates config"
	cmd.Long = `Description:
  Interact with updates config

  Configure updates config for operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Show
	updatesShowCmd := cmdUpdatesShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updatesShowCmd.Command())

	// Update
	updatesEditCmd := cmdUpdatesEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updatesEditCmd.Command())

	return cmd
}

// Show updates config.
type cmdUpdatesShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdatesShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show"
	cmd.Short = "Show updates config"
	cmd.Long = `Description:
  Show updates config.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdUpdatesShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdUpdatesShow) run(cmd *cobra.Command, args []string) error {
	config, err := c.ocClient.GetSystemUpdatesConfig(cmd.Context())
	if err != nil {
		return err
	}

	enc := yaml.NewEncoder(c.Command().OutOrStdout())
	enc.SetIndent(2)
	return enc.Encode(config)
}

// Edit server system updates configuration.
type cmdUpdatesEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdUpdatesEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit"
	cmd.Short = "Edit updates configuration"
	cmd.Long = `Description:
  Edit updates configuration
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing instance configurations.
func (c *cmdUpdatesEdit) helpTemplate() string {
	return `### This is a YAML representation of the updates configuration.
### Any line starting with a '# will be ignored.`
}

func (c *cmdUpdatesEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdUpdatesEdit) run(cmd *cobra.Command, args []string) error {
	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.SystemUpdatesPut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateSystemUpdatesConfig(cmd.Context(), newdata)
		if err != nil {
			return err
		}

		return nil
	}

	updatesConfig, err := c.ocClient.GetSystemUpdatesConfig(cmd.Context())
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(updatesConfig)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.SystemUpdatesPut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateSystemUpdatesConfig(cmd.Context(), newdata)
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
