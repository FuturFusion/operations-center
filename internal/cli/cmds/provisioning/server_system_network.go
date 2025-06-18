package provisioning

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/lxc/incus/v6/shared/revert"
	"github.com/lxc/incus/v6/shared/termios"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/shared/api"
)

// Configure server system network.
type cmdServerSystemNetwork struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerSystemNetwork) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "network"
	cmd.Short = "Interact with system network configuration of servers"
	cmd.Long = `Description:
  Interact with system network configuration of servers

  Configure system network of servers.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Edit
	serverSystemNetworkEditCmd := cmdServerSystemNetworkEdit{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(serverSystemNetworkEditCmd.Command())

	return cmd
}

// Edit server system network configuration.
type cmdServerSystemNetworkEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerSystemNetworkEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit server system network"
	cmd.Long = `Description:
  Edit server system network
`

	cmd.RunE = c.Run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing instance configurations.
func (c *cmdServerSystemNetworkEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
### config:
###   dns:
###     hostname: host
###     domain: local
###     search_domains:
###       - local
###     nameservers:
###       - 1.1.1.1
###   ntp:
###     timeservers:
###       - 0.pool.ntp.org
###   interfaces:
###     - name: eth0
###       MTU: 1500
###       Addresses:
###         - 192.168.1.2`
}

func (c *cmdServerSystemNetworkEdit) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(getStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ServerSystemNetwork{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateServerSystemNetwork(cmd.Context(), name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	networkConfig, err := c.ocClient.GetServerSystemNetwork(cmd.Context(), name)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(networkConfig)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := textEditor("", []byte(c.helpTemplate()+"\n\n"+string(data)))
	if err != nil {
		return err
	}

	for {
		newdata := api.ServerSystemNetwork{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateServerSystemNetwork(cmd.Context(), name, newdata)
		}

		// Respawn the editor
		if err != nil {
			fmt.Fprintf(os.Stderr, "Config parsing error: %s\n", err)
			fmt.Println("Press enter to open the editor again or ctrl+c to abort change")

			_, err := os.Stdin.Read(make([]byte, 1))
			if err != nil {
				return err
			}

			content, err = textEditor("", content)
			if err != nil {
				return err
			}

			continue
		}

		break
	}

	return nil
}

// Spawn the editor with a temporary YAML file for editing configs.
func textEditor(inPath string, inContent []byte) ([]byte, error) {
	var f *os.File
	var err error
	var path string

	// Detect the text editor to use
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
		if editor == "" {
			for _, p := range []string{"editor", "vi", "emacs", "nano"} {
				_, err := exec.LookPath(p)
				if err == nil {
					editor = p
					break
				}
			}
			if editor == "" {
				return []byte{}, errors.New("No text editor found, please set the EDITOR environment variable")
			}
		}
	}

	if inPath == "" {
		// If provided input, create a new file
		f, err = os.CreateTemp("", "incus_editor_")
		if err != nil {
			return []byte{}, err
		}

		reverter := revert.New()
		defer reverter.Fail()

		reverter.Add(func() {
			_ = f.Close()
			_ = os.Remove(f.Name())
		})

		err = os.Chmod(f.Name(), 0o600)
		if err != nil {
			return []byte{}, err
		}

		_, err = f.Write(inContent)
		if err != nil {
			return []byte{}, err
		}

		err = f.Close()
		if err != nil {
			return []byte{}, err
		}

		path = fmt.Sprintf("%s.yaml", f.Name())
		err = os.Rename(f.Name(), path)
		if err != nil {
			return []byte{}, err
		}

		reverter.Success()
		reverter.Add(func() { _ = os.Remove(path) })
	} else {
		path = inPath
	}

	cmdParts := strings.Fields(editor)
	cmd := exec.Command(cmdParts[0], append(cmdParts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return []byte{}, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return []byte{}, err
	}

	return content, nil
}
