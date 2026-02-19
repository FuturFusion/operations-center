package provisioning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lxc/incus/v6/shared/termios"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdServer struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdServer) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "server"
	cmd.Short = "Interact with servers"
	cmd.Long = `Description:
  Interact with servers

  Configure servers for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	serverListCmd := cmdServerList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverListCmd.Command())

	// Edit
	serverEditCmd := cmdServerEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverEditCmd.Command())

	// Remove
	serverRemoveCmd := cmdServerRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverRemoveCmd.Command())

	// Rename
	serverRenameCmd := cmdServerRename{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverRenameCmd.Command())

	// Resync
	serverResyncCmd := cmdServerResync{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverResyncCmd.Command())

	// Show
	serverShowCmd := cmdServerShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverShowCmd.Command())

	// System
	serverSystemCmd := cmdServerSystem{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(serverSystemCmd.Command())

	return cmd
}

// List servers.
type cmdServerList struct {
	ocClient *client.OperationsCenterClient

	flagFilterCluster    string
	flagFilterStatus     string
	flagFilterExpression string

	flagFormat string
}

func (c *cmdServerList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available servers"
	cmd.Long = `Description:
  List the available servers
`

	cmd.Flags().StringVar(&c.flagFilterCluster, "cluster", "", "cluster name to filter for")
	cmd.Flags().StringVar(&c.flagFilterStatus, "status", "", "status to filter for, valid values: pending, ready")
	cmd.Flags().StringVar(&c.flagFilterExpression, "filter", "", "filter expression to apply")

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdServerList) run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	var filter provisioning.ServerFilter

	if c.flagFilterCluster != "" {
		filter.Cluster = ptr.To(c.flagFilterCluster)
	}

	if c.flagFilterStatus != "" {
		var status api.ServerStatus
		err = status.UnmarshalText([]byte(c.flagFilterStatus))
		if err != nil {
			return fmt.Errorf("Invalid value for status: %v", err)
		}

		filter.Status = &status
	}

	if c.flagFilterExpression != "" {
		filter.Expression = ptr.To(c.flagFilterExpression)
	}

	servers, err := c.ocClient.GetWithFilterServers(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Cluster", "Name", "Connection URL", "Public Connection URL", "Certificate Fingerprint", "Type", "Channel", "Status", "Update Status", "Last Updated", "Last Seen"}
	data := [][]string{}

	for _, server := range servers {
		data = append(data, []string{
			server.Cluster,
			server.Name,
			server.ConnectionURL,
			server.PublicConnectionURL,
			server.Fingerprint[:min(len(server.Fingerprint), 12)],
			server.Type.String(),
			server.Channel,
			server.Status.String(),
			server.VersionData.State().String(),
			server.LastUpdated.Truncate(time.Second).String(),
			server.LastSeen.Truncate(time.Second).String(),
		})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, servers)
}

// Edit servers.
type cmdServerEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit server"
	cmd.Long = `Description:
  Edit the server
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing token seed configurations.
func (c *cmdServerEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### public_connection_url: ""
`
}

func (c *cmdServerEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerEdit) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ServerPut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateServer(cmd.Context(), name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	tokenSeedConfig, err := c.ocClient.GetServer(cmd.Context(), name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(tokenSeedConfig.ServerPut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.ServerPut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateServer(cmd.Context(), name, newdata)
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

// Remove server.
type cmdServerRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a server"
	cmd.Long = `Description:
  Remove a server

  Removes a server from the operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.DeleteServer(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Rename server.
type cmdServerRename struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerRename) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "rename <name> <new-name>"
	cmd.Short = "Rename a server"
	cmd.Long = `Description:
  Rename a server

  Renames a server to a new name.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerRename) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerRename) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	newName := args[1]

	if name == newName {
		return fmt.Errorf("Rename failed, name and new name are equal")
	}

	err := c.ocClient.RenameServer(cmd.Context(), name, newName)
	if err != nil {
		return err
	}

	return nil
}

// Resync server.
type cmdServerResync struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdServerResync) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "resync <name>"
	cmd.Short = "Resync a server"
	cmd.Long = `Description:
  Resync a server's state

  Resyncs a server's state to the inventory.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerResync) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerResync) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.ResyncServer(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Show server.
type cmdServerShow struct {
	ocClient *client.OperationsCenterClient

	flagShowResources   bool
	flagShowOSData      bool
	flagShowVersionData bool
}

func (c *cmdServerShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a server"
	cmd.Long = `Description:
  Show information about a server.
`

	cmd.Flags().BoolVar(&c.flagShowResources, "resources", false, "show server resource details")
	cmd.Flags().BoolVar(&c.flagShowOSData, "os-data", false, "show server OS data")
	cmd.Flags().BoolVar(&c.flagShowVersionData, "version-data", false, "show server version data")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdServerShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdServerShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	server, err := c.ocClient.GetServer(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Cluster: %s\n", server.Cluster)
	fmt.Printf("Name: %s\n", server.Name)
	fmt.Printf("Connection URL: %s\n", server.ConnectionURL)
	fmt.Printf("Public Connection URL: %s\n", server.PublicConnectionURL)
	fmt.Printf("Certificate:\n%s", indent("  ", strings.TrimSpace(server.Certificate)))
	fmt.Printf("Certificate Fingerprint: %s\n", server.Fingerprint)
	fmt.Printf("Type: %s\n", server.Type.String())
	fmt.Printf("Channel: %s\n", server.Channel)
	fmt.Printf("Status: %s\n", server.Status.String())
	fmt.Printf("Update Status: %s\n", server.VersionData.State().String())
	fmt.Printf("Last Updated: %s\n", server.LastUpdated.Truncate(time.Second).String())
	fmt.Printf("Last Seen: %s\n", server.LastSeen.Truncate(time.Second).String())

	if c.flagShowResources {
		hardwareDataJSON, err := json.MarshalIndent(server.HardwareData, "", "  ")
		if err != nil {
			return err
		}

		fmt.Printf("Resources:\n%s\n", render.Indent(4, string(hardwareDataJSON)))
	}

	if c.flagShowOSData {
		osDataJSON, err := json.MarshalIndent(server.OSData, "", "  ")
		if err != nil {
			return err
		}

		fmt.Printf("OS Data:\n%s\n", render.Indent(4, string(osDataJSON)))
	}

	if c.flagShowVersionData {
		versionDataJSON, err := json.MarshalIndent(server.VersionData, "", "  ")
		if err != nil {
			return err
		}

		fmt.Printf("Version Data:\n%s\n", render.Indent(4, string(versionDataJSON)))
	}

	return nil
}
