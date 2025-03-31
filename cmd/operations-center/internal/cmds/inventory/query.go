package inventory

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/cmd/operations-center/internal/validate"
	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/ptr"
)

//go:embed templates/resource_tree.gotmpl
var templateResourceTree string

//go:embed templates/cluster_names.gotmpl
var templateClusterNames string

var embeddedTemplates = map[string]*string{
	"cluster_names": ptr.To(templateClusterNames),
	"resource_tree": ptr.To(templateResourceTree),
}

type CmdQuery struct {
	flagFilterKinds              []string
	flagFilterCluster            []string
	flagFilterServer             []string
	flagFilterServerIncludeNull  bool
	flagFilterProject            []string
	flagFilterProjectIncludeNull bool
	flagFilterParent             []string
	flagFilterParentIncludeNull  bool
	flagFilterExpression         string
	flagNoFilter                 bool

	flagOutputTemplate string
}

func (c *CmdQuery) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "query"
	cmd.Short = "Query the inventory"
	cmd.Long = `Description:
  Query the aggregated resource from the inventory.
`

	cmd.RunE = c.Run

	cmd.Flags().StringSliceVar(&c.flagFilterKinds, "kind", nil, "list of resource kinds to filter for")
	cmd.Flags().StringSliceVar(&c.flagFilterCluster, "cluster", nil, "cluster name to filter for")
	cmd.Flags().StringSliceVar(&c.flagFilterServer, "server", nil, "server name to filter for")
	cmd.Flags().BoolVar(&c.flagFilterServerIncludeNull, "server-include-empty", false, "include resources where server is not set")
	cmd.Flags().StringSliceVar(&c.flagFilterProject, "project", nil, "project name to filter for")
	cmd.Flags().BoolVar(&c.flagFilterProjectIncludeNull, "project-include-empty", false, "include resources where project is not set")
	cmd.Flags().StringSliceVar(&c.flagFilterParent, "parent", nil, "parent name to filter for")
	cmd.Flags().BoolVar(&c.flagFilterParentIncludeNull, "parent-include-empty", false, "include resources where parent is not set")
	cmd.Flags().StringVar(&c.flagFilterExpression, "filter", "", "filter expression to apply")
	cmd.Flags().BoolVar(&c.flagNoFilter, "no-filter", false, "force query without filter")

	cmd.Flags().StringVar(&c.flagOutputTemplate, "template", ":resource_tree", `path to Go template file or the name of an embedded template. Name of embedded templates require the prefix ":".`)

	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	return cmd
}

func (c *CmdQuery) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	if len(c.flagFilterKinds) == 0 &&
		len(c.flagFilterCluster) == 0 &&
		len(c.flagFilterProject) == 0 &&
		len(c.flagFilterServer) == 0 &&
		len(c.flagFilterParent) == 0 &&
		c.flagFilterExpression == "" &&
		!c.flagNoFilter {
		return fmt.Errorf("Using query without filter might cause serious load and produce huge responses. Please use some filters or provide --no-filter flag.")
	}

	var tmpl *template.Template
	if strings.HasPrefix(c.flagOutputTemplate, ":") {
		name := strings.TrimPrefix(c.flagOutputTemplate, ":")
		tmplBody, ok := embeddedTemplates[name]
		if !ok {
			return fmt.Errorf("%q is not a valid embedded template", name)
		}

		tmpl, err = template.New("").Parse(*tmplBody)
	} else {
		tmpl, err = template.ParseFiles(c.flagOutputTemplate)
	}

	if err != nil {
		return err
	}

	var filter inventory.InventoryAggregateFilter

	filter.Kinds = c.flagFilterKinds

	filter.Clusters = c.flagFilterCluster

	filter.Servers = c.flagFilterServer

	if c.flagFilterServerIncludeNull {
		filter.ServerIncludeNull = true
	}

	filter.Projects = c.flagFilterProject

	if c.flagFilterProjectIncludeNull {
		filter.ProjectIncludeNull = true
	}

	filter.Parents = c.flagFilterParent

	if c.flagFilterParentIncludeNull {
		filter.ParentIncludeNull = true
	}

	if c.flagFilterExpression != "" {
		filter.Expression = ptr.To(c.flagFilterExpression)
	}

	// Client call
	ocClient := client.New()

	inventoryAggregates, err := ocClient.GetWithFilterInventoryAggregates(filter)
	if err != nil {
		return err
	}

	err = tmpl.Execute(os.Stdout, inventoryAggregates)
	if err != nil {
		return err
	}

	return nil
}
