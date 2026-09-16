package provisioning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/lxc/incus/v7/shared/termios"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/decodestrict"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdChannel struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdChannel) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "channel"
	cmd.Short = "Interact with channels"
	cmd.Long = `Description:
  Interact with channels

  Manage channels.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	updateListCmd := cmdChannelList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateListCmd.Command())

	// Show
	updateShowCmd := cmdChannelShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateShowCmd.Command())

	// Add
	updateAddCmd := cmdChannelAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateAddCmd.Command())

	// Edit
	updateEditCmd := cmdChannelEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateEditCmd.Command())

	// Remove
	updateRemoveCmd := cmdChannelRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateRemoveCmd.Command())

	// Changelog
	updateChangelogCmd := cmdChannelChangelog{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(updateChangelogCmd.Command())

	return cmd
}

// List channels.
type cmdChannelList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdChannelList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available channels"
	cmd.Long = `Description:
  List the available channels
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdChannelList) run(cmd *cobra.Command, args []string) error {
	channels, err := c.ocClient.GetChannels(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Description", "Last Updated"}
	data := [][]string{}

	for _, channel := range channels {
		data = append(data, []string{channel.Name, channel.Description, channel.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsSort(data, []sort.ColumnSorter{
		{
			Index: 0, // Name
			Less:  sort.StringLess,
		},
	})

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, channels)
}

// Show channel.
type cmdChannelShow struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdChannelShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a channel"
	cmd.Long = `Description:
  Show information about a channel.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	validFormats := []string{"", "json", "yaml"}
	if !slices.Contains(validFormats, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdChannelShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	channel, err := c.ocClient.GetChannel(cmd.Context(), name)
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent("", "  ")
		err = enc.Encode(channel)
		if err != nil {
			return err
		}

	case "yaml":
		enc := yaml.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent(2)
		err = enc.Encode(channel)
		if err != nil {
			return err
		}

	default:
		clusters, err := c.ocClient.GetWithFilterClusters(cmd.Context(), provisioning.ClusterFilter{
			Expression: new(fmt.Sprintf(`channel == %q`, name)),
		})
		if err != nil {
			return err
		}

		servers, err := c.ocClient.GetWithFilterServers(cmd.Context(), provisioning.ServerFilter{
			Expression: new(fmt.Sprintf(`version_data.update_channel == %q`, name)),
		})
		if err != nil {
			return err
		}

		updates, err := c.ocClient.GetWithFilterUpdates(cmd.Context(), provisioning.UpdateFilter{
			Channel: new(name),
		})
		if err != nil {
			return err
		}

		fmt.Printf("Name: %s\n", channel.Name)
		fmt.Printf("Description: %s\n", channel.Description)
		fmt.Printf("Last Updated: %s\n", channel.LastUpdated.Truncate(time.Second).String())

		fmt.Printf("Assigned Clusters\n")
		for _, cluster := range clusters {
			fmt.Printf("- %s (%s)\n", cluster.Name, cluster.ConnectionURL)
		}

		fmt.Printf("Assigned Servers:\n")
		for _, server := range servers {
			fmt.Printf("- %s (%s)\n", server.Name, server.ConnectionURL)
		}

		fmt.Printf("Linked Updates:\n")
		for _, update := range updates {
			fmt.Printf("- %s, %s, %s\n", update.UUID.String(), update.Origin, update.Version)
		}
	}

	return nil
}

// Add channel.
type cmdChannelAdd struct {
	ocClient *client.OperationsCenterClient

	description string
}

func (c *cmdChannelAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name>"
	cmd.Short = "Add a channel"
	cmd.Long = `Description:
  Add a channel.
`

	cmd.Flags().StringVar(&c.description, "description", "", "Description of the channel")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdChannelAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.CreateChannel(cmd.Context(), api.ChannelPost{
		Name: name,
		ChannelPut: api.ChannelPut{
			Description: c.description,
		},
	})
	if err != nil {
		return fmt.Errorf("Failed to create channel %q: %w", name, err)
	}

	return nil
}

// Edit channel.
type cmdChannelEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdChannelEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit a channel"
	cmd.Long = `Description:
  Edit a channel.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing channel configurations.
func (c *cmdChannelEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### description: ""
`
}

func (c *cmdChannelEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdChannelEdit) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ChannelPut{}
		err = decodestrict.YAML(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateChannel(cmd.Context(), name, newdata)
		if err != nil {
			return fmt.Errorf("Failed to update channel %q: %w", name, err)
		}

		return nil
	}

	channel, err := c.ocClient.GetChannel(cmd.Context(), name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(channel.ChannelPut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.ChannelPut{}
		err = decodestrict.YAML(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateChannel(cmd.Context(), name, newdata)
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

// Remove channel.
type cmdChannelRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdChannelRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a channel"
	cmd.Long = `Description:
  Remove a channel.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdChannelRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.DeleteChannel(cmd.Context(), name)
	if err != nil {
		return fmt.Errorf("Failed to delete channel %q: %w", name, err)
	}

	return nil
}

// Show changelog of a channel.
type cmdChannelChangelog struct {
	ocClient *client.OperationsCenterClient

	flagArchitecture string
}

func (c *cmdChannelChangelog) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "changelog <name>"
	cmd.Short = "Changelog information of a channel"
	cmd.Long = `Description:
  Changelog from one update to the next for all updates in a channel.
`

	cmd.Flags().StringVar(&c.flagArchitecture, "architecture", "x86_64", "architecture the changelog should be shown for")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdChannelChangelog) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdChannelChangelog) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	changelog, err := c.ocClient.GetChannelChangelog(cmd.Context(), name, c.flagArchitecture)
	if err != nil {
		return err
	}

	changelogYAML, err := yaml.Marshal(changelog)
	if err != nil {
		return err
	}

	fmt.Printf("Changelog:\n%s\n", render.Indent(4, string(changelogYAML)))

	return nil
}
