package system

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/lxc/incus/v7/shared/termios"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type CmdCluster struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdCluster) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cluster"
	cmd.Short = "Interact with cluster config"
	cmd.Long = `Description:
  Interact with cluster config

  Configure the offline threshold of the cluster members, after which a member
  missing heartbeats counts as offline.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Show
	clusterShowCmd := cmdClusterShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterShowCmd.Command())

	// Update
	clusterEditCmd := cmdClusterEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterEditCmd.Command())

	return cmd
}

// Show cluster config.
type cmdClusterShow struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdClusterShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show"
	cmd.Short = "Show cluster config"
	cmd.Long = `Description:
  Show cluster config.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	validFormats := []string{"", "json", "yaml"}
	if !slices.Contains(validFormats, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdClusterShow) run(cmd *cobra.Command, args []string) error {
	config, _, err := c.ocClient.GetSystemClusterConfig(cmd.Context())
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent("", "  ")
		err = enc.Encode(config)

	default:
		enc := yaml.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent(2)
		err = enc.Encode(config)
	}

	return err
}

// Edit server system cluster configuration.
type cmdClusterEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit"
	cmd.Short = "Edit cluster configuration"
	cmd.Long = `Description:
  Edit cluster configuration
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing instance configurations.
func (c *cmdClusterEdit) helpTemplate() string {
	return `### This is a YAML representation of the cluster configuration.
### Any line starting with a '# will be ignored.`
}

func (c *cmdClusterEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterEdit) run(cmd *cobra.Command, args []string) error {
	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := system.ClusterPut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateSystemClusterConfig(cmd.Context(), newdata, "")
		if err != nil {
			return err
		}

		return nil
	}

	clusterConfig, etag, err := c.ocClient.GetSystemClusterConfig(cmd.Context())
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(clusterConfig)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := system.ClusterPut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateSystemClusterConfig(cmd.Context(), newdata, etag)
		}

		// Editing again doesn't help, the change has to be based on the current config.
		if errors.Is(err, domain.ErrPreconditionFailed) {
			return err
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
