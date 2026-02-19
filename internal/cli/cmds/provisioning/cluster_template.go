package provisioning

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
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

// Show clusterTemplate.
type cmdClusterTemplateShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterTemplateShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a cluster-template"
	cmd.Long = `Description:
  Show information about a cluster-template.
`

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

	return nil
}

func (c *cmdClusterTemplateShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	clusterTemplate, err := c.ocClient.GetClusterTemplate(cmd.Context(), name)
	if err != nil {
		return err
	}

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

	return nil
}
