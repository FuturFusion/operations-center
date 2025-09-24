package provisioning

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
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

	// Apply
	clusterTemplateApplyCmd := cmdClusterTemplateApply{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterTemplateApplyCmd.Command())

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

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.description, "description", "", "Description of the cluster template")
	cmd.Flags().StringVarP(&c.servicesConfigFile, "services-config", "c", "", "Services config for the cluster template")
	cmd.Flags().StringVarP(&c.applicationConfigFile, "application-seed-config", "a", "", "Application seed configuration for the cluster template")
	cmd.Flags().StringVar(&c.variablesFile, "variables", "", "Variable definitions for the cluster template")

	return cmd
}

func (c *cmdClusterTemplateAdd) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	var servicesConfigBody []byte
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

	cmd.RunE = c.Run

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdClusterTemplateList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

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

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterTemplateRemove) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	err = c.ocClient.DeleteClusterTemplate(cmd.Context(), name)
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

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterTemplateShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

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

// Apply cluster template.
type cmdClusterTemplateApply struct {
	ocClient *client.OperationsCenterClient

	serverNames []string
	serverType  string
}

func (c *cmdClusterTemplateApply) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "apply <template-name> <cluster-name> <cluster-connection-url> <variables.yaml>"
	cmd.Short = "Apply the cluster template to add a new cluster"
	cmd.Long = `Description:
  Apply the cluster template to add a new cluster

  Applies the cluster template to adds a new cluster to the operations center.
`

	cmd.RunE = c.Run

	const flagServerNames = "server-names"
	cmd.Flags().StringSliceVarP(&c.serverNames, flagServerNames, "s", nil, "Server names of the cluster members")
	_ = cmd.MarkFlagRequired(flagServerNames)

	cmd.Flags().StringVarP(&c.serverType, "server-type", "t", "incus", "Type of servers, that should be clustered, supported values are (incus, migration-manager, operations-center)")

	return cmd
}

func (c *cmdClusterTemplateApply) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 4, 4)
	if exit {
		return err
	}

	templateName := args[0]
	clusterName := args[1]
	clusterConnectionURL := args[2]
	variableFilename := args[3]

	variableValues := map[string]string{}
	body, err := os.ReadFile(variableFilename)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(body, &variableValues)
	if err != nil {
		return err
	}

	var serverType api.ServerType
	err = serverType.UnmarshalText([]byte(c.serverType))
	if err != nil {
		return err
	}

	clusterTemplate, err := c.ocClient.GetClusterTemplate(cmd.Context(), templateName)
	if err != nil {
		return err
	}

	serviceConfigFilled, err := applyVariables(clusterTemplate.ServiceConfigTemplate, clusterTemplate.Variables, variableValues)
	if err != nil {
		return err
	}

	servicesConfig := map[string]any{}
	err = yaml.Unmarshal([]byte(serviceConfigFilled), &servicesConfig)
	if err != nil {
		return err
	}

	applicationConfigFilled, err := applyVariables(clusterTemplate.ApplicationConfigTemplate, clusterTemplate.Variables, variableValues)
	if err != nil {
		return err
	}

	applicationConfig := map[string]any{}
	err = yaml.Unmarshal([]byte(applicationConfigFilled), &applicationConfig)
	if err != nil {
		return err
	}

	err = c.ocClient.CreateCluster(cmd.Context(), api.ClusterPost{
		Cluster: api.Cluster{
			Name:          clusterName,
			ConnectionURL: clusterConnectionURL,
		},
		ServerNames:           c.serverNames,
		ServerType:            serverType,
		ServicesConfig:        servicesConfig,
		ApplicationSeedConfig: applicationConfig,
	})
	if err != nil {
		return err
	}

	return nil
}

func applyVariables(template string, variables api.ClusterTemplateVariables, variableValues map[string]string) (string, error) {
	for variableName, variableDefinition := range variables {
		_, ok := variableValues[variableName]
		if !ok {
			if variableDefinition.DefaultValue == "" {
				return "", fmt.Errorf("No value provided for variable %q, which is required, since it has no default value defined", variableName)
			}

			// Use default value.
			variableValues[variableName] = variableDefinition.DefaultValue
		}
	}

	for name, value := range variableValues {
		variableName := "@" + name + "@"
		template = strings.ReplaceAll(template, variableName, value)
	}

	return template, nil
}
