package cmds

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lxc/incus/v7/shared/units"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/shared/api"
)

type CmdSupportBundle struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdSupportBundle) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "support-bundle"
	cmd.Short = "Create support bundles"
	cmd.Long = `Description:
  Create support bundles

  A support bundle is a zip archive with diagnostic data of a cluster, a
  project, an instance or a server. It holds the records of operations-center,
  the configuration and state from Incus and IncusOS and the IncusOS journal
  and logs of the involved servers.

  Secrets in the configuration are redacted, logs are included as is.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	subcommands := []cmdSupportBundleCreate{
		{
			ocClient: c.OCClient,
			use:      "cluster <cluster>",
			short:    "Create a support bundle for a cluster",
			args:     1,
			request: func(c *cmdSupportBundleCreate, args []string) api.SupportBundlePost {
				return api.SupportBundlePost{Type: api.SupportBundleTypeCluster, Cluster: args[0]}
			},
		},
		{
			ocClient: c.OCClient,
			use:      "project <cluster> <project>",
			short:    "Create a support bundle for a project of a cluster",
			args:     2,
			request: func(c *cmdSupportBundleCreate, args []string) api.SupportBundlePost {
				return api.SupportBundlePost{Type: api.SupportBundleTypeProject, Cluster: args[0], Project: args[1]}
			},
		},
		{
			ocClient:    c.OCClient,
			use:         "instance <cluster> <instance>",
			short:       "Create a support bundle for an instance of a cluster",
			args:        2,
			withProject: true,
			request: func(c *cmdSupportBundleCreate, args []string) api.SupportBundlePost {
				return api.SupportBundlePost{Type: api.SupportBundleTypeInstance, Cluster: args[0], Project: c.flagProject, Name: args[1]}
			},
		},
		{
			ocClient: c.OCClient,
			use:      "server <server>",
			short:    "Create a support bundle for a server",
			args:     1,
			request: func(c *cmdSupportBundleCreate, args []string) api.SupportBundlePost {
				return api.SupportBundlePost{Type: api.SupportBundleTypeServer, Server: args[0]}
			},
		},
	}

	for i := range subcommands {
		cmd.AddCommand(subcommands[i].Command())
	}

	return cmd
}

type cmdSupportBundleCreate struct {
	ocClient *client.OperationsCenterClient

	use         string
	short       string
	args        int
	withProject bool
	request     func(c *cmdSupportBundleCreate, args []string) api.SupportBundlePost

	flagOutput  string
	flagProject string
}

func (c *cmdSupportBundleCreate) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = c.use
	cmd.Short = c.short
	cmd.Long = fmt.Sprintf(`Description:
  %s

  The support bundle is written to the file given by --output or to a file in
  the current directory named by operations-center.
`, c.short)

	cmd.Flags().StringVarP(&c.flagOutput, "output", "o", "", "Target file of the support bundle")
	if c.withProject {
		cmd.Flags().StringVar(&c.flagProject, "project", "default", "Project of the instance")
	}

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdSupportBundleCreate) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, c.args, c.args)
	if exit {
		return err
	}

	return nil
}

func (c *cmdSupportBundleCreate) run(cmd *cobra.Command, args []string) (err error) {
	if c.flagOutput != "" && file.PathExists(c.flagOutput) {
		return fmt.Errorf("target file %q already exists", c.flagOutput)
	}

	filename, bundleReader, err := c.ocClient.GetSupportBundle(cmd.Context(), c.request(c, args))
	if err != nil {
		return err
	}

	defer func() {
		closeErr := bundleReader.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	targetFilename := c.flagOutput
	if targetFilename == "" {
		targetFilename = filepath.Base(filename)
	}

	if targetFilename == "" || targetFilename == "." || targetFilename == "/" {
		targetFilename = "support-bundle.zip"
	}

	targetFile, err := os.OpenFile(targetFilename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		closeErr := targetFile.Close()
		var removeErr error
		if err != nil {
			removeErr = os.Remove(targetFilename)
		}

		err = errors.Join(err, closeErr, removeErr)
	}()

	quiet, _ := cmd.Flags().GetBool("quiet")
	progress, writer := render.ProgressWriter(targetFile, "Fetching support bundle: %s", quiet)

	size, err := file.SafeCopy(writer, bundleReader)
	if err != nil {
		return err
	}

	// A support bundle, which failed on the server while being streamed, is incomplete.
	_, err = zip.NewReader(targetFile, size)
	if err != nil {
		return fmt.Errorf("received support bundle is not a valid archive, it might have failed on the server: %w", err)
	}

	progress.Done(fmt.Sprintf("Successfully written %s to %q", units.GetByteSizeString(size, 2), targetFilename))

	return nil
}
