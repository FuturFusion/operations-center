package provisioning

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/file"
	"github.com/FuturFusion/operations-center/internal/render"
	"github.com/FuturFusion/operations-center/internal/sort"
)

type cmdClusterArtifact struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterArtifact) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "artifact"
	cmd.Short = "Interact with cluster artifacts"
	cmd.Long = `Description:
  Interact with cluster artifacts
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	// List
	clusterArtifactListCmd := cmdClusterArtifactList{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(clusterArtifactListCmd.Command())

	// Show
	clusterArtifactShowCmd := cmdClusterArtifactShow{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(clusterArtifactShowCmd.Command())

	// Get Archive
	clusterArtifactArchiveCmd := cmdClusterArtifactGetArchive{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(clusterArtifactArchiveCmd.Command())

	// Get File
	clusterArtifactFileCmd := cmdClusterArtifactGetFile{
		ocClient: c.ocClient,
	}

	cmd.AddCommand(clusterArtifactFileCmd.Command())

	return cmd
}

// List cluster artifacts.
type cmdClusterArtifactList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdClusterArtifactList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list"
	cmd.Short = "List available cluster artifacts"
	cmd.Long = `Description:
  List the available cluster artifacts.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterArtifactList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdClusterArtifactList) run(cmd *cobra.Command, args []string) error {
	clusterName := args[0]

	clusterArtifacts, err := c.ocClient.GetClusterArtifacts(cmd.Context(), clusterName)
	if err != nil {
		return err
	}

	// Render the table.
	header := []string{"Name", "Cluster", "Description", "Last Updated"}
	data := [][]string{}

	for _, artifact := range clusterArtifacts {
		data = append(data, []string{artifact.Name, artifact.Cluster, artifact.Description, artifact.LastUpdated.Truncate(time.Second).String()})
	}

	sort.ColumnsNaturally(data)

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, clusterArtifacts)
}

// Show cluster artifact.
type cmdClusterArtifactShow struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterArtifactShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <clusterName> <artifactName>"
	cmd.Short = "Show information about a cluster artifact"
	cmd.Long = `Description:
  Show information about a cluster artifact.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterArtifactShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterArtifactShow) run(cmd *cobra.Command, args []string) error {
	clusterName := args[0]
	artifactName := args[1]

	clusterArtifact, err := c.ocClient.GetClusterArtifact(cmd.Context(), clusterName, artifactName)
	if err != nil {
		return err
	}

	fmt.Printf("Name: %s\n", clusterArtifact.Name)
	fmt.Printf("Cluster: %s\n", clusterArtifact.Cluster)
	fmt.Printf("Description: %s\n", clusterArtifact.Description)

	fmt.Printf("Properties:\n")
	header := []string{"Key", "Value"}
	data := [][]string{}

	for key, value := range clusterArtifact.Properties {
		data = append(data, []string{key, value})
	}

	if len(data) > 0 {
		sort.ColumnsNaturally(data)

		renderErr := render.Table(cmd.OutOrStdout(), "compact", header, data, nil)
		if renderErr != nil {
			err = renderErr
		}
	}

	fmt.Printf("Files:\n")
	header = []string{"Filename", "Mimetype", "Size"}
	data = [][]string{}

	for _, f := range clusterArtifact.Files {
		data = append(data, []string{f.Name, f.MimeType, units.BytesSize(float64(f.Size))})
	}

	if len(data) > 0 {
		sort.ColumnsNaturally(data)

		renderErr := render.Table(cmd.OutOrStdout(), "compact", header, data, nil)
		if err != nil {
			err = errors.Join(err, renderErr)
		}
	}

	fmt.Printf("Last Updated: %s\n", clusterArtifact.LastUpdated.Truncate(time.Second).String())

	return err
}

// Get cluster artifact archive.
type cmdClusterArtifactGetArchive struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterArtifactGetArchive) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "archive <clusterName> <artifactName> <target-file.zip>"
	cmd.Short = "Get cluster artifact as zip archive"
	cmd.Long = `Description:
  Get the cluster artifact as zip archive.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterArtifactGetArchive) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 3, 3)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterArtifactGetArchive) run(cmd *cobra.Command, args []string) error {
	clusterName := args[0]
	artifactName := args[1]
	targetFilename := args[2]

	if file.PathExists(targetFilename) {
		return fmt.Errorf("target file %q already exists", targetFilename)
	}

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer targetFile.Close()

	archiveReader, err := c.ocClient.GetClusterArtifactArchive(cmd.Context(), clusterName, artifactName, "zip")
	if err != nil {
		return err
	}

	defer archiveReader.Close()

	size, err := io.Copy(targetFile, archiveReader)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully written %d bytes to %q\n", size, targetFilename)

	return nil
}

// Get cluster artifact file.
type cmdClusterArtifactGetFile struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdClusterArtifactGetFile) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "file <clusterName> <artifactName> <filename> <target-file.zip>"
	cmd.Short = "Get cluster artifact file"
	cmd.Long = `Description:
  Get a cluster artifact file.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdClusterArtifactGetFile) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 4, 4)
	if exit {
		return err
	}

	return nil
}

func (c *cmdClusterArtifactGetFile) run(cmd *cobra.Command, args []string) error {
	clusterName := args[0]
	artifactName := args[1]
	filename := args[2]
	targetFilename := args[3]

	if file.PathExists(targetFilename) {
		return fmt.Errorf("target file %q already exists", targetFilename)
	}

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	defer targetFile.Close()

	fileReader, err := c.ocClient.GetClusterArtifactFile(cmd.Context(), clusterName, artifactName, filename)
	if err != nil {
		return err
	}

	defer fileReader.Close()

	size, err := io.Copy(targetFile, fileReader)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully written %d bytes to %q\n", size, targetFilename)

	return nil
}
