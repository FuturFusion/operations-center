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
	"github.com/FuturFusion/operations-center/internal/editor"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdSettings struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdSettings) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "settings"
	cmd.Short = "Interact with settings config"
	cmd.Long = `Description:
  Interact with settings config

  Configure settings config for operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Show
	settingsShowCmd := cmdSettingsShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(settingsShowCmd.Command())

	// Update
	settingsEditCmd := cmdSettingsEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(settingsEditCmd.Command())

	return cmd
}

// Show settings config.
type cmdSettingsShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdSettingsShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show"
	cmd.Short = "Show settings config"
	cmd.Long = `Description:
  Show settings config.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdSettingsShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdSettingsShow) run(cmd *cobra.Command, args []string) error {
	config, err := c.ocClient.GetSystemSettingsConfig(cmd.Context())
	if err != nil {
		return err
	}

	enc := yaml.NewEncoder(c.Command().OutOrStdout())
	enc.SetIndent(2)
	return enc.Encode(config)
}

// Edit server system settings configuration.
type cmdSettingsEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdSettingsEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit"
	cmd.Short = "Edit settings configuration"
	cmd.Long = `Description:
  Edit settings configuration
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing instance configurations.
func (c *cmdSettingsEdit) helpTemplate() string {
	return `### This is a YAML representation of the settings configuration.
### Any line starting with a '# will be ignored.`
}

func (c *cmdSettingsEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdSettingsEdit) run(cmd *cobra.Command, args []string) error {
	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.SystemSettingsPut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateSystemSettingsConfig(cmd.Context(), newdata)
		if err != nil {
			return err
		}

		return nil
	}

	settingsConfig, err := c.ocClient.GetSystemSettingsConfig(cmd.Context())
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(settingsConfig)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.SystemSettingsPut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateSystemSettingsConfig(cmd.Context(), newdata)
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
