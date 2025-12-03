package provisioning

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/ptr"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdCluster struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdCluster) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "cluster"
	cmd.Short = "Interact with clusters"
	cmd.Long = `Description:
  Interact with clusters

  Configure clusters for use by operations center.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// Add
	clusterAddCmd := cmdClusterAdd{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterAddCmd.Command())

	// List
	clusterListCmd := cmdClusterList{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterListCmd.Command())

	// Remove
	clusterRemoveCmd := cmdClusterRemove{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterRemoveCmd.Command())

	// Rename
	clusterRenameCmd := cmdClusterRename{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterRenameCmd.Command())

	// Show
	clusterShowCmd := cmdClusterShow{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterShowCmd.Command())

	// Resync
	clusterResyncCmd := cmdClusterResync{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterResyncCmd.Command())

	// Update certificate
	clusterUpdateCertificateCmd := cmdClusterUpdateCertificate{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterUpdateCertificateCmd.Command())

	// artifact sub-command
	clusterArtifactCmd := cmdClusterArtifact{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterArtifactCmd.Command())

	return cmd
}

// Add cluster.
type cmdClusterAdd struct {
	ocClient *client.OperationsCenterClient

	serverNames                  []string
	serverType                   string
	servicesConfigFile           string
	applicationConfigFile        string
	clusterTemplate              string
	clusterTemplateVariablesFile string
}

func (c *cmdClusterAdd) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "add <name> <connection-url>"
	cmd.Short = "Add a new cluster"
	cmd.Long = `Description:
  Add a new cluster

  Adds a new cluster to the operations center.
`

	const flagServerNames = "server-names"
	cmd.Flags().StringSliceVarP(&c.serverNames, flagServerNames, "s", nil, "Server names of the cluster members")
	_ = cmd.MarkFlagRequired(flagServerNames)

	cmd.Flags().StringVarP(&c.serverType, "server-type", "t", "incus", "Type of servers, that should be clustered, supported values are (incus, migration-manager, operations-center)")
	cmd.Flags().StringVarP(&c.servicesConfigFile, "services-config", "c", "", "Services config applied on the cluster nodes during pre clustering")
	cmd.Flags().StringVarP(&c.applicationConfigFile, "application-seed-config", "a", "", "Application seed configuration applied on the cluster during post clustering")
	cmd.Flags().StringVar(&c.clusterTemplate, "cluster-template", "", "Name of the cluster template to be applied. Mutual exclusive with --services-config and --application-seed-config")
	cmd.Flags().StringVar(&c.clusterTemplateVariablesFile, "cluster-template-variables", "", "Name of the variables.yaml file containing the values to be applied in the cluster template. Required, if --cluster-template is provided")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	if c.clusterTemplate != "" {
		if c.servicesConfigFile != "" || c.applicationConfigFile != "" {
			return fmt.Errorf(`--cluster-template is mutual exclusive with --services-config and --application-seed-config`)
		}

		if c.clusterTemplateVariablesFile == "" {
			return fmt.Errorf(`--cluster-template-variables is required with --cluster-template`)
		}
	}

	// TODO: maybe we could support in-flight templates, where the user provides
	// templated service config and application config files, a variables.yaml
	// and a variables definition. This would allow the user to use cluster
	// templates without storing them in Operations Center.
	if (c.servicesConfigFile != "" || c.applicationConfigFile != "") && c.clusterTemplateVariablesFile != "" {
		return fmt.Errorf(`--cluster-template-variables is incompatible with required with --services-config and --application-seed-config`)
	}

	return nil
}

func (c *cmdClusterAdd) Run(cmd *cobra.Command, args []string) error {
	name := args[0]
	connectionURL := args[1]

	var serverType api.ServerType
	err := serverType.UnmarshalText([]byte(c.serverType))
	if err != nil {
		return err
	}

	servicesConfig := map[string]any{}

	if c.servicesConfigFile != "" {
		body, err := os.ReadFile(c.servicesConfigFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &servicesConfig)
		if err != nil {
			return err
		}
	}

	applicationConfig := map[string]any{}

	if c.applicationConfigFile != "" {
		body, err := os.ReadFile(c.applicationConfigFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &applicationConfig)
		if err != nil {
			return err
		}
	}

	clusterTemplateVariables := api.ConfigMap{}

	if c.clusterTemplateVariablesFile != "" {
		body, err := os.ReadFile(c.clusterTemplateVariablesFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &clusterTemplateVariables)
		if err != nil {
			return err
		}
	}

	err = c.ocClient.CreateCluster(cmd.Context(), api.ClusterPost{
		Cluster: api.Cluster{
			Name:          name,
			ConnectionURL: connectionURL,
		},
		ServerNames:                   c.serverNames,
		ServerType:                    serverType,
		ServicesConfig:                servicesConfig,
		ApplicationSeedConfig:         applicationConfig,
		ClusterTemplate:               c.clusterTemplate,
		ClusterTemplateVariableValues: clusterTemplateVariables,
	})
	if err != nil {
		return err
	}

	return nil
}

// List clusters.
type cmdClusterList struct {
	ocClient *client.OperationsCenterClient

	flagFilterExpression string

	flagFormat string
}

func (c *cmdClusterList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available clusters"
	cmd.Long = `Description:
  List the available clusters
`

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.flagFilterExpression, "filter", "", "filter expression to apply")

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		return validate.FormatFlag(cmd.Flag("format").Value.String())
	}

	return cmd
}

func (c *cmdClusterList) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	var filter provisioning.ClusterFilter

	if c.flagFilterExpression != "" {
		filter.Expression = ptr.To(c.flagFilterExpression)
	}

	clusters, err := c.ocClient.GetWithFilterClusters(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Connection URL", "Certificate Fingerprint", "Status", "Last Updated"}
	data := [][]string{}

	for _, cluster := range clusters {
		data = append(data, []string{
			cluster.Name,
			cluster.ConnectionURL,
			cluster.Fingerprint[:min(len(cluster.Fingerprint), 12)],
			cluster.Status.String(),
			cluster.LastUpdated.Truncate(time.Second).String(),
		})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, clusters)
}

// Remove cluster.
type cmdClusterRemove struct {
	ocClient *client.OperationsCenterClient

	flagDeleteMode string
}

func (c *cmdClusterRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a cluster"
	cmd.Long = `Description:
  Remove a cluster

  Removes a cluster from the operations center. This operation supports the
  following modes, controlled through the --mode flag:

  - normal: cluster record is only removed from operations center if it is in state pending or unknown and there are no servers referencing the cluster.
  - force: cluster and server records including all associated inventory information is removed from operations center, does not do any change to the cluster it self.
  - factory-reset: everything from "force" and additionally a factory reset is performed on every server, that is part of the cluster.
`

	cmd.RunE = c.Run

	cmd.Flags().StringVar(&c.flagDeleteMode, "mode", api.ClusterDeleteModeNormal.String(), "delete mode for removal of cluster, supported values: normal, force, factory-reset")

	return cmd
}

func (c *cmdClusterRemove) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]
	deleteMode := api.ClusterDeleteMode(c.flagDeleteMode)
	_, ok := api.ClusterDeleteModes[deleteMode]
	if !ok {
		deleteMode = api.ClusterDeleteModeNormal
	}

	if deleteMode == api.ClusterDeleteModeForce {
		cmd.Println(`WARNING: removal of a cluster with delete mode "force" does not do any change to the actual cluster, but the cluster and the server records including all accosiated inventory information is removed from operations center.`)
	}

	err = c.ocClient.DeleteCluster(cmd.Context(), name, deleteMode)
	if err != nil {
		return err
	}

	return nil
}

// Rename cluster.
type cmdClusterRename struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterRename) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "rename <name> <new-name>"
	cmd.Short = "Rename a cluster"
	cmd.Long = `Description:
  Rename a cluster

  Renames a cluster to a new name.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterRename) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	name := args[0]
	newName := args[1]

	if name == newName {
		return fmt.Errorf("Rename failed, name and new name are equal")
	}

	err = c.ocClient.RenameCluster(cmd.Context(), name, newName)
	if err != nil {
		return err
	}

	return nil
}

// Show cluster.
type cmdClusterShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a cluster"
	cmd.Long = `Description:
  Show information about a cluster.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterShow) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	cluster, err := c.ocClient.GetCluster(cmd.Context(), name)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", cluster.Name)
	fmt.Printf("Connection URL: %s\n", cluster.ConnectionURL)
	fmt.Printf("Certificate:\n%s", indent("  ", strings.TrimSpace(cluster.Certificate)))
	fmt.Printf("Certificate Fingerprint: %s\n", cluster.Fingerprint)
	fmt.Printf("Status: %s\n", cluster.Status.String())
	fmt.Printf("Last Updated: %s\n", cluster.LastUpdated.Truncate(time.Second).String())

	return nil
}

// Resync cluster.
type cmdClusterResync struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterResync) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "resync <name>"
	cmd.Short = "Resync inventory for a cluster"
	cmd.Long = `Description:
  Resync inventory for a cluster.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterResync) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	name := args[0]

	err = c.ocClient.ResyncCluster(cmd.Context(), name)
	if err != nil {
		return err
	}

	return nil
}

// Update cluster certificate.
type cmdClusterUpdateCertificate struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterUpdateCertificate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update-certificate <name> <cert.crt> <cert.key>"
	cmd.Short = "Update cluster certificate"
	cmd.Long = `Description:
  Update the certificate and key for a cluster.
`

	cmd.RunE = c.Run

	return cmd
}

func (c *cmdClusterUpdateCertificate) Run(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	name := args[0]
	certificateFilename := args[1]
	certificateKeyFilename := args[2]

	certificatePEM, err := os.ReadFile(certificateFilename)
	if err != nil {
		return fmt.Errorf("Failed to read certificate file %q: %w", certificateFilename, err)
	}

	certificateKeyPEM, err := os.ReadFile(certificateKeyFilename)
	if err != nil {
		return fmt.Errorf("Failed to read key file %q: %w", certificateKeyFilename, err)
	}

	_, err = tls.LoadX509KeyPair(certificateFilename, certificateKeyFilename)
	if err != nil {
		return fmt.Errorf("Failed to load X509 key pair: %w", err)
	}

	err = c.ocClient.UpdateClusterCertificate(cmd.Context(), name, api.ClusterCertificatePut{
		ClusterCertificate:    string(certificatePEM),
		ClusterCertificateKey: string(certificateKeyPEM),
	})
	if err != nil {
		return err
	}

	return nil
}
