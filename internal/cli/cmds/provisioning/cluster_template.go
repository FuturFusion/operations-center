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
	"github.com/FuturFusion/operations-center/internal/util/decodestrict"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdClusterTemplate struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdClusterTemplate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cluster-template"
	cmd.Short = "Interact with cluster-templates"
	cmd.Long = `Description:
  Interact with cluster-templates

  Configure cluster-templates for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	clusterTemplateAddCmd := cmdClusterTemplateAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateAddCmd.Command())

	// Edit
	clusterTemplateEditCmd := cmdClusterTemplateEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateEditCmd.Command())

	// List
	clusterTemplateListCmd := cmdClusterTemplateList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateListCmd.Command())

	// Remove
	clusterTemplateRemoveCmd := cmdClusterTemplateRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateRemoveCmd.Command())

	// Rename
	clusterTemplateRenameCmd := cmdClusterTemplateRename{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateRenameCmd.Command())

	// Show
	clusterTemplateShowCmd := cmdClusterTemplateShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateShowCmd.Command())

	return cmd
}

// Add clusterTemplate.
type cmdClusterTemplateAdd struct {
	ocClient *client.OperationsCenterClient

	description           string
	servicesConfigFile    string
	applicationConfigFile string
	variablesFile         string
}

func (c *cmdClusterTemplateAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name>"
	cmd.Short = "Add a new cluster-template"
	cmd.Long = `Description:
  Add a new cluster-template

  Adds a new cluster-template to the operations center.
`

	cmd.Flags().StringVar(&c.description, "description", "", "Description of the cluster template")
	cmd.Flags().StringVarP(&c.servicesConfigFile, "services-config", "c", "", "Services config for the cluster template")
	cmd.Flags().StringVarP(&c.applicationConfigFile, "application-seed-config", "a", "", "Application seed configuration for the cluster template")
	cmd.Flags().StringVar(&c.variablesFile, "variables", "", "Variable definitions for the cluster template")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterTemplateAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterTemplateAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	var servicesConfigBody []byte
	var err error
	if c.servicesConfigFile != "" {
		servicesConfigBody, err = os.ReadFile(c.servicesConfigFile)
		if err != nil {
			return err
		}
	}

	var applicationConfigBody []byte
	if c.applicationConfigFile != "" {
		applicationConfigBody, err = os.ReadFile(c.applicationConfigFile)
		if err != nil {
			return err
		}
	}

	variableDefinitions := api.ClusterTemplateVariables{}
	if c.applicationConfigFile != "" {
		body, err := os.ReadFile(c.variablesFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &variableDefinitions)
		if err != nil {
			return err
		}
	}

	err = c.ocClient.CreateClusterTemplate(cmd.Context(), api.ClusterTemplatePost{
		Name: name,
		ClusterTemplatePut: api.ClusterTemplatePut{
			Description:               c.description,
			ServiceConfigTemplate:     string(servicesConfigBody),
			ApplicationConfigTemplate: string(applicationConfigBody),
			Variables:                 variableDefinitions,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// Edit clusterTemplate.
type cmdClusterTemplateEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterTemplateEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit a cluster-template"
	cmd.Long = `Description:
  Edit a cluster-template

  Edits the description, the templates and the variable definitions of a
  cluster-template.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing cluster template configurations.
func (c *cmdClusterTemplateEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### description: ""
### service_config_template: ""
### application_config_template: ""
### variables:
###   SOME_VARIABLE:
###     description: ""
###     default: ""
`
}

func (c *cmdClusterTemplateEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterTemplateEdit) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ClusterTemplatePut{}
		err = decodestrict.YAML(contents, &newdata)
		if err != nil {
			return err
		}

		return c.ocClient.UpdateClusterTemplate(cmd.Context(), name, newdata)
	}

	clusterTemplate, err := c.ocClient.GetClusterTemplate(cmd.Context(), name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(clusterTemplate.ClusterTemplatePut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.ClusterTemplatePut{}
		err = decodestrict.YAML(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateClusterTemplate(cmd.Context(), name, newdata)
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

// List clusterTemplates.
type cmdClusterTemplateList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdClusterTemplateList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available cluster-templates"
	cmd.Long = `Description:
  List the available cluster-templates
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterTemplateList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdClusterTemplateList) run(cmd *cobra.Command, args []string) error {
	clusterTemplates, err := c.ocClient.GetClusterTemplates(cmd.Context())
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Description", "Last Updated"}
	data := [][]string{}

	for _, clusterTemplate := range clusterTemplates {
		data = append(data, []string{clusterTemplate.Name, clusterTemplate.Description, clusterTemplate.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, clusterTemplates)
}

// Remove clusterTemplate.
type cmdClusterTemplateRemove struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterTemplateRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a cluster-template"
	cmd.Long = `Description:
  Remove a cluster-template

  Removes a cluster-template from the operations center.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterTemplateRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterTemplateRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.DeleteClusterTemplate(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Rename clusterTemplate.
type cmdClusterTemplateRename struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterTemplateRename) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "rename <name> <new-name>"
	cmd.Short = "Rename a cluster-template"
	cmd.Long = `Description:
  Rename a cluster-template

  Renames a cluster-template to a new name.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterTemplateRename) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterTemplateRename) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	newName := args[1]

	if name == newName {
		return fmt.Errorf("Rename failed, name and new name are equal")
	}

	err := c.ocClient.RenameClusterTemplate(cmd.Context(), name, newName)
	if err != nil {
		return err
	}

	return nil
}

// Show clusterTemplate.
type cmdClusterTemplateShow struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdClusterTemplateShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a cluster-template"
	cmd.Long = `Description:
  Show information about a cluster-template.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterTemplateShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
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

func (c *cmdClusterTemplateShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	clusterTemplate, err := c.ocClient.GetClusterTemplate(cmd.Context(), name)
	if err != nil {
		return err
	}

	switch c.flagFormat {
	case "json":
		enc := json.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent("", "  ")
		err = enc.Encode(clusterTemplate)
		if err != nil {
			return err
		}

	case "yaml":
		enc := yaml.NewEncoder(c.Command().OutOrStdout())
		enc.SetIndent(2)
		err = enc.Encode(clusterTemplate)
		if err != nil {
			return err
		}

	default:
		variables, err := yaml.Marshal(clusterTemplate.Variables)
		if err != nil {
			return err
		}

		fmt.Printf("Name: %s\n", clusterTemplate.Name)
		fmt.Printf("Description: %s\n", clusterTemplate.Description)
		fmt.Printf("Service config template:\n%s\n", render.Indent(4, clusterTemplate.ServiceConfigTemplate))
		fmt.Printf("Application config template:\n%s\n", render.Indent(4, clusterTemplate.ApplicationConfigTemplate))
		fmt.Printf("Variables:\n%s\n", render.Indent(4, string(variables)))
		fmt.Printf("Last Updated: %s\n", clusterTemplate.LastUpdated.Truncate(time.Second).String())
	}

	return nil
}
