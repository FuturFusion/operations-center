package provisioning

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lxc/incus/v6/shared/termios"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/environment"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/editor"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/internal/util/sort"
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

	// Factory Reset
	clusterFactoryResetCmd := cmdClusterFactoryReset{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterFactoryResetCmd.Command())

	// Edit
	clusterEditCmd := cmdClusterEdit{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterEditCmd.Command())

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

	// Bulk update
	clusterWideUpdateCmd := cmdClusterBulkUpdate{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterWideUpdateCmd.Command())

	// Cluster wide update
	clusterUpdateCmd := cmdClusterUpdate{
		ocClient: c.OCClient,
	}

	cmd.AddCommand(clusterUpdateCmd.Command())

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

	flagServerNames                  []string
	flagServerType                   string
	flagServicesConfigFile           string
	flagApplicationConfigFile        string
	flagClusterTemplate              string
	flagClusterTemplateVariablesFile string
	flagChannel                      string
	flagDescription                  string
	flagPropertiesFile               string
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
	cmd.Flags().StringSliceVarP(&c.flagServerNames, flagServerNames, "s", nil, "Server names of the cluster members")
	_ = cmd.MarkFlagRequired(flagServerNames)

	cmd.Flags().StringVarP(&c.flagServerType, "server-type", "t", "incus", "Type of servers, that should be clustered, supported values are (incus, migration-manager, operations-center)")
	cmd.Flags().StringVarP(&c.flagServicesConfigFile, "services-config", "c", "", "Services config applied on the cluster nodes during pre clustering")
	cmd.Flags().StringVarP(&c.flagApplicationConfigFile, "application-seed-config", "a", "", "Application seed configuration applied on the cluster during post clustering")
	cmd.Flags().StringVar(&c.flagClusterTemplate, "cluster-template", "", "Name of the cluster template to be applied. Mutual exclusive with --services-config and --application-seed-config")
	cmd.Flags().StringVar(&c.flagClusterTemplateVariablesFile, "cluster-template-variables", "", "Name of the variables.yaml file containing the values to be applied in the cluster template. Required, if --cluster-template is provided")
	cmd.Flags().StringVar(&c.flagChannel, "channel", "", "Name of the channel, the cluster follows for updates")
	cmd.Flags().StringVar(&c.flagDescription, "description", "", "Description of the cluster")
	cmd.Flags().StringVar(&c.flagPropertiesFile, "properties", "", "Filename of the file containing the properties of the cluster")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterAdd) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	if c.flagClusterTemplate != "" {
		if c.flagServicesConfigFile != "" || c.flagApplicationConfigFile != "" {
			return fmt.Errorf(`--cluster-template is mutual exclusive with --services-config and --application-seed-config`)
		}

		if c.flagClusterTemplateVariablesFile == "" {
			return fmt.Errorf(`--cluster-template-variables is required with --cluster-template`)
		}
	}

	// TODO: maybe we could support in-flight templates, where the user provides
	// templated service config and application config files, a variables.yaml
	// and a variables definition. This would allow the user to use cluster
	// templates without storing them in Operations Center.
	if (c.flagServicesConfigFile != "" || c.flagApplicationConfigFile != "") && c.flagClusterTemplateVariablesFile != "" {
		return fmt.Errorf(`--cluster-template-variables is incompatible with required with --services-config and --application-seed-config`)
	}

	return nil
}

func (c *cmdClusterAdd) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	connectionURL := args[1]

	var serverType api.ServerType
	err := serverType.UnmarshalText([]byte(c.flagServerType))
	if err != nil {
		return err
	}

	servicesConfig := map[string]any{}

	if c.flagServicesConfigFile != "" {
		body, err := os.ReadFile(c.flagServicesConfigFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &servicesConfig)
		if err != nil {
			return err
		}
	}

	applicationConfig := map[string]any{}

	if c.flagApplicationConfigFile != "" {
		body, err := os.ReadFile(c.flagApplicationConfigFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &applicationConfig)
		if err != nil {
			return err
		}
	}

	clusterTemplateVariables := api.ConfigMap{}

	if c.flagClusterTemplateVariablesFile != "" {
		body, err := os.ReadFile(c.flagClusterTemplateVariablesFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &clusterTemplateVariables)
		if err != nil {
			return err
		}
	}

	clusterProperties := api.ConfigMap{}

	if c.flagPropertiesFile != "" {
		body, err := os.ReadFile(c.flagPropertiesFile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(body, &clusterProperties)
		if err != nil {
			return err
		}
	}

	err = c.ocClient.CreateCluster(cmd.Context(), api.ClusterPost{
		Cluster: api.Cluster{
			Name: name,
			ClusterPut: api.ClusterPut{
				ConnectionURL: connectionURL,
				Channel:       c.flagChannel,
				Description:   c.flagDescription,
				Properties:    clusterProperties,
			},
		},
		ServerNames:                   c.flagServerNames,
		ServerType:                    serverType,
		ServicesConfig:                servicesConfig,
		ApplicationSeedConfig:         applicationConfig,
		ClusterTemplate:               c.flagClusterTemplate,
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

	cmd.Flags().StringVar(&c.flagFilterExpression, "filter", "", "filter expression to apply")
	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 0, 0)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdClusterList) run(cmd *cobra.Command, args []string) error {
	var filter provisioning.ClusterFilter

	if c.flagFilterExpression != "" {
		filter.Expression = ptr.To(c.flagFilterExpression)
	}

	clusters, err := c.ocClient.GetWithFilterClusters(cmd.Context(), filter)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Connection URL", "Description", "Certificate Fingerprint", "Channel", "Status", "Update Status", "Last Updated"}
	data := [][]string{}

	for _, cluster := range clusters {
		updateStatusDescription := ""
		if cluster.UpdateStatus.InProgressStatus.StatusDescription != nil {
			updateStatusDescription = fmt.Sprintf(" (%s)", *cluster.UpdateStatus.InProgressStatus.StatusDescription)
		}

		data = append(data, []string{
			cluster.Name,
			cluster.ConnectionURL,
			cluster.Description,
			cluster.Fingerprint[:min(len(cluster.Fingerprint), 12)],
			cluster.Channel,
			cluster.Status.String(),
			fmt.Sprintf("%d / %d / %d%s", len(cluster.UpdateStatus.NeedsUpdate), len(cluster.UpdateStatus.NeedsReboot), len(cluster.UpdateStatus.InMaintenance), updateStatusDescription),
			cluster.LastUpdated.Truncate(time.Second).String(),
		})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, clusters)
}

// Remove cluster.
type cmdClusterRemove struct {
	ocClient *client.OperationsCenterClient

	flagForce bool
}

func (c *cmdClusterRemove) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a cluster"
	cmd.Long = `Description:
  Remove a cluster

  Removes a cluster from the operations center. This operation supports the
  following modes, controlled through the --force flag:

  - normal: cluster record is only removed from operations center if it is in state pending or unknown and there are no servers referencing the cluster.
  - force: cluster and server records including all associated inventory information is removed from operations center, does not do any change to the cluster it self.
`

	cmd.Flags().BoolVarP(&c.flagForce, "force", "f", false, "if this flag is provided, a forceful delete is performed")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterRemove) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterRemove) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	if c.flagForce {
		cmd.Println(`WARNING: removal of a cluster with delete mode "force" does not do any change to the actual cluster, but the cluster and the server records including all accosiated inventory information is removed from operations center.`)
	}

	err := c.ocClient.DeleteCluster(cmd.Context(), name, c.flagForce)
	if err != nil {
		return err
	}

	return nil
}

// FactoryReset cluster.
type cmdClusterFactoryReset struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterFactoryReset) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "factory-reset <name> [<token> [<token-seed-name>]]"
	cmd.Short = "Factory reset a cluster"
	cmd.Long = `Description:
  Factory reset a cluster

  Factory resets a cluster from the operations center.
  Cluster and server records including all associated inventory information is
  removed from operations center. Additionally a factory reset is performed on
  every server, that is part of the cluster.

  For the factory reset an optional token and the name of a token seed can
  be provided. If they are not provided, Operations Center will generate a
  token and use default seed values.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterFactoryReset) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 3)
	if exit {
		return err
	}

	if len(args) > 1 {
		_, err := uuid.Parse(args[1])
		if err != nil {
			return fmt.Errorf("Failed to parse token: %w", err)
		}
	}

	if len(args) > 2 {
		if args[2] == "" {
			return fmt.Errorf("Invalid token seed name: empty string")
		}
	}

	return nil
}

func (c *cmdClusterFactoryReset) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	tokenArgs := make([]string, 0, 2)
	if len(args) > 1 {
		tokenArgs = args[1:]
	}

	err := c.ocClient.FactoryResetCluster(cmd.Context(), name, tokenArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Update cluster.
type cmdClusterEdit struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterEdit) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "edit <name>"
	cmd.Short = "Edit a cluster"
	cmd.Long = `Description:
  Edit a cluster

  Edits a cluster's connection URL.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

// helpTemplate returns a sample YAML configuration and guidelines for editing cluster configurations.
func (c *cmdClusterEdit) helpTemplate() string {
	return `### This is a YAML representation of the configuration.
### Any line starting with a '# will be ignored.
###
### A sample configuration looks like:
###
### connection_url: ""
### channel: stable
### description: ""
### properties: {}
`
}

func (c *cmdClusterEdit) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterEdit) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	// If stdin isn't a terminal, read text from it.
	if !termios.IsTerminal(environment.GetStdinFd()) {
		contents, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ClusterPut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}

		err = c.ocClient.UpdateCluster(cmd.Context(), name, newdata)
		if err != nil {
			return err
		}

		return nil
	}

	clusterConfig, err := c.ocClient.GetCluster(cmd.Context(), name)
	if err != nil {
		return err
	}

	b := &bytes.Buffer{}
	encoder := yaml.NewEncoder(b)
	encoder.SetIndent(2)
	err = encoder.Encode(clusterConfig.ClusterPut)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := editor.Spawn("", append([]byte(c.helpTemplate()+"\n\n"), b.Bytes()...))
	if err != nil {
		return err
	}

	for {
		newdata := api.ClusterPut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = c.ocClient.UpdateCluster(cmd.Context(), name, newdata)
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

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterRename) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterRename) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	newName := args[1]

	if name == newName {
		return fmt.Errorf("Rename failed, name and new name are equal")
	}

	err := c.ocClient.RenameCluster(cmd.Context(), name, newName)
	if err != nil {
		return err
	}

	return nil
}

// Show cluster.
type cmdClusterShow struct {
	ocClient *client.OperationsCenterClient

	flagShowProperties bool
}

func (c *cmdClusterShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <name>"
	cmd.Short = "Show information about a cluster"
	cmd.Long = `Description:
  Show information about a cluster.
`

	cmd.Flags().BoolVar(&c.flagShowProperties, "properties", false, "show cluster properties")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterShow) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	cluster, err := c.ocClient.GetCluster(cmd.Context(), name)
	if err != nil {
		return err
	}

	needUpdate := strings.Join(cluster.UpdateStatus.NeedsUpdate, ", ")
	if needUpdate == "" {
		needUpdate = "-"
	}

	needReboot := strings.Join(cluster.UpdateStatus.NeedsReboot, ", ")
	if needReboot == "" {
		needReboot = "-"
	}

	inMaintenance := strings.Join(cluster.UpdateStatus.InMaintenance, ", ")
	if inMaintenance == "" {
		inMaintenance = "-"
	}

	fmt.Printf("Name: %s\n", cluster.Name)
	fmt.Printf("Connection URL: %s\n", cluster.ConnectionURL)
	fmt.Printf("Description: %s\n", cluster.Description)
	fmt.Printf("Certificate:\n%s", indent("  ", strings.TrimSpace(cluster.Certificate)))
	fmt.Printf("Certificate Fingerprint: %s\n", cluster.Fingerprint)
	fmt.Printf("Channel: %s\n", cluster.Channel)
	fmt.Printf("Status: %s\n", cluster.Status.String())
	fmt.Printf("Update Status:\n")
	fmt.Printf("  Need Update: %s\n", needUpdate)
	fmt.Printf("  Need Reboot: %s\n", needReboot)
	fmt.Printf("  In Maintenance: %s\n", inMaintenance)
	fmt.Printf("Last Updated: %s\n", cluster.LastUpdated.Truncate(time.Second).String())

	if c.flagShowProperties {
		propertiesYAML, err := yaml.Marshal(cluster.Properties)
		if err != nil {
			return err
		}

		fmt.Printf("Properties:\n%s\n", render.Indent(4, string(propertiesYAML)))
	}

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

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterResync) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterResync) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.ResyncCluster(cmd.Context(), name)
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

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterUpdateCertificate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterUpdateCertificate) run(cmd *cobra.Command, args []string) error {
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

// Bulk update cluster.
type cmdClusterBulkUpdate struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterBulkUpdate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "bulk-update <name> <bulk-request.json>"
	cmd.Short = "Update cluster certificate"
	cmd.Long = `Description:
  Bulk update for a cluster. Provide the request through a file or through stdin.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterBulkUpdate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterBulkUpdate) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	var err error
	bulkUpdateRequestReader := os.Stdin

	if len(args) > 1 {
		bulkUpdateRequestFile := args[1]

		bulkUpdateRequestReader, err = os.Open(bulkUpdateRequestFile)
		if err != nil {
			return fmt.Errorf("Failed to read file %q: %w", bulkUpdateRequestFile, err)
		}

		defer func() {
			_ = bulkUpdateRequestReader.Close()
		}()
	}

	var bulkUpdateRequest api.ClusterBulkUpdatePost
	err = json.NewDecoder(bulkUpdateRequestReader).Decode(&bulkUpdateRequest)
	if err != nil {
		return fmt.Errorf("Failed to decode bulk update request: %w", err)
	}

	err = c.ocClient.BulkUpdateCluster(cmd.Context(), name, bulkUpdateRequest)
	if err != nil {
		return err
	}

	return nil
}

// Cluster wide update.
type cmdClusterUpdate struct {
	ocClient *client.OperationsCenterClient

	flagReboot bool
}

func (c *cmdClusterUpdate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "update <name>"
	cmd.Short = "Launch cluster wide update for all servers"
	cmd.Long = `Description:
  Perform a cluster wide update of all servers.
`

	cmd.Flags().BoolVar(&c.flagReboot, "reboot", false, "perform rolling reboot after applying the update")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterUpdate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterUpdate) run(cmd *cobra.Command, args []string) error {
	name := args[0]

	err := c.ocClient.LaunchClusterWideUpdate(cmd.Context(), name, c.flagReboot)
	if err != nil {
		return err
	}

	return nil
}
