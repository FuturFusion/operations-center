package provisioning

import (
	"bytes"
	"fmt"
	"io"
	"os"

	incusosapi "github.com/lxc/incus-os/incus-osd/api"
	"github.com/lxc/incus/v6/shared/termios"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Configure server system storage.
type cmdServerSystemStorage struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerSystemStorage) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "storage"
	cmd.Short = "Interact with system storage configuration of servers"
	cmd.Long = `Description:
  Interact with system storage configuration of servers

  Configure system storage of servers.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Edit
	serverSystemStorageEditCmd := cmdServerSystemStorageEdit{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverSystemStorageEditCmd.Command())

	return cmd
}

// Edit server system storage configuration.
type cmdServerSystemStorageEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerSystemStorageEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit server system storage"
	cmd.Long = `Description:
  Edit server system storage
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing system storage configuration.
func (c *cmdServerSystemStorageEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### scrub_schedule: ""
### pools:
###   - name: zfs0
###     type: zfs-raid0
###     devices: []
###     cache: []
###     log: []`
}

func (c *cmdServerSystemStorageEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerSystemStorageEdit) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ServerSystemStorage{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateServerSystemStorage(cmd.Context(), name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	storageConfig, err := c.ocClient.GetServerSystemStorage(cmd.Context(), name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(storageConfig.Config)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := incusosapi.SystemStorageConfig{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateServerSystemStorage(cmd.Context(), name, api.ServerSystemStorage{
				Config: newdata,
			})
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
