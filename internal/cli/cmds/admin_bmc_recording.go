package cmds

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/lxc/incus/v7/shared/units"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/render"
)

type cmdAdminBMCRecording struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdAdminBMCRecording) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "bmc-recording"
	cmd.Short = "Manage recordings of the BMC traffic of server deployments"
	cmd.Long = `Description:
  Manage recordings of the BMC traffic of server deployments

  A recording is made for a deployment started with "server deploy --record-bmc".
  The commands are internal and can change at any time.
`

	// Workaround for subcommand usage errors. See: https://github.com/spf13/cobra/issues/706
	cmd.Args = cobra.NoArgs
	cmd.Run = func(cmd *cobra.Command, args []string) { _ = cmd.Usage() }

	listCmd := cmdAdminBMCRecordingList{ocClient: c.ocClient}
	cmd.AddCommand(listCmd.Command())

	showCmd := cmdAdminBMCRecordingShow{ocClient: c.ocClient}
	cmd.AddCommand(showCmd.Command())

	downloadCmd := cmdAdminBMCRecordingDownload{ocClient: c.ocClient}
	cmd.AddCommand(downloadCmd.Command())

	deleteCmd := cmdAdminBMCRecordingDelete{ocClient: c.ocClient}
	cmd.AddCommand(deleteCmd.Command())

	return cmd
}

type cmdAdminBMCRecordingList struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdAdminBMCRecordingList) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "list <server>"
	cmd.Short = "List the BMC recordings of a server"
	cmd.Long = `Description:
  List the BMC recordings of a server, the newest first.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "table", `Format (csv|json|table|yaml|compact), use suffix ",noheader" to disable headers and ",header" to enable if demanded, e.g. csv,header`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdAdminBMCRecordingList) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return validate.FormatFlag(cmd.Flag("format").Value.String())
}

func (c *cmdAdminBMCRecordingList) run(cmd *cobra.Command, args []string) error {
	manifests, err := c.ocClient.GetServerBMCRecordings(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	header := []string{"ID", "Started", "Finished", "Final State", "Exchanges", "Bytes"}
	data := make([][]string, 0, len(manifests))

	for _, manifest := range manifests {
		finished := ""
		if !manifest.FinishedAt.IsZero() {
			finished = manifest.FinishedAt.Truncate(time.Second).String()
		}

		data = append(data, []string{
			manifest.RecordingID,
			manifest.StartedAt.Truncate(time.Second).String(),
			finished,
			manifest.FinalState.String(),
			strconv.FormatInt(manifest.Exchanges, 10),
			units.GetByteSizeString(manifest.Bytes, 2),
		})
	}

	return render.Table(cmd.OutOrStdout(), c.flagFormat, header, data, manifests)
}

type cmdAdminBMCRecordingShow struct {
	ocClient *client.OperationsCenterClient

	flagFormat string
}

func (c *cmdAdminBMCRecordingShow) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "show <server> <id>"
	cmd.Short = "Show the manifest of a BMC recording"
	cmd.Long = `Description:
  Show the manifest of a BMC recording.
`

	cmd.Flags().StringVarP(&c.flagFormat, "format", "f", "yaml", `Format (json|yaml)`)

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdAdminBMCRecordingShow) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	if !slices.Contains([]string{"json", "yaml"}, c.flagFormat) {
		return fmt.Errorf(`Invalid value for flag "--format": %q`, c.flagFormat)
	}

	return nil
}

func (c *cmdAdminBMCRecordingShow) run(cmd *cobra.Command, args []string) error {
	manifest, err := c.ocClient.GetServerBMCRecording(cmd.Context(), args[0], args[1])
	if err != nil {
		return err
	}

	if c.flagFormat == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		return enc.Encode(manifest)
	}

	enc := yaml.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent(2)

	err = enc.Encode(manifest)
	if err != nil {
		return err
	}

	return enc.Close()
}

type cmdAdminBMCRecordingDownload struct {
	ocClient *client.OperationsCenterClient

	flagOutput    string
	flagAnonymize bool
	flagForce     bool
}

func (c *cmdAdminBMCRecordingDownload) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "download <server> <id>"
	cmd.Short = "Download a BMC recording as zip archive"
	cmd.Long = `Description:
  Download a BMC recording as zip archive.

  The archive is written to "<server>-<id>.zip" in the working directory,
  unless --output is given. An existing file is only replaced with --force.
`

	cmd.Flags().StringVarP(&c.flagOutput, "output", "o", "", "Target file of the zip archive")
	cmd.Flags().BoolVar(&c.flagAnonymize, "anonymize", false, "Replace identifying values in the recording")
	cmd.Flags().BoolVar(&c.flagForce, "force", false, "Replace an existing target file")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdAdminBMCRecordingDownload) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdAdminBMCRecordingDownload) run(cmd *cobra.Command, args []string) error {
	name := args[0]
	id := args[1]

	targetFilename := c.flagOutput
	if targetFilename == "" {
		targetFilename = fmt.Sprintf("%s-%s.zip", name, id)
	}

	archiveReader, err := c.ocClient.GetServerBMCRecordingArchive(cmd.Context(), name, id, c.flagAnonymize)
	if err != nil {
		return err
	}

	defer archiveReader.Close()

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if c.flagForce {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	targetFile, err := os.OpenFile(targetFilename, flags, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("Target file %q already exists, use --force to replace it", targetFilename)
	}

	if err != nil {
		return err
	}

	defer targetFile.Close()

	quiet, _ := cmd.Flags().GetBool("quiet")
	format := fmt.Sprintf("Fetching BMC recording %q of server %q: %%s", id, name)

	progress, writer := render.ProgressWriter(targetFile, format, quiet)

	size, err := file.SafeCopy(writer, archiveReader)
	if err != nil {
		return err
	}

	progress.Done(fmt.Sprintf("Successfully written %s to %q", units.GetByteSizeString(size, 2), targetFilename))

	return nil
}

type cmdAdminBMCRecordingDelete struct {
	ocClient *client.OperationsCenterClient
}

func (c *cmdAdminBMCRecordingDelete) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "delete <server> <id>"
	cmd.Short = "Delete a BMC recording"
	cmd.Long = `Description:
  Delete a BMC recording.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *cmdAdminBMCRecordingDelete) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 2, 2)
	if exit {
		return err
	}

	return nil
}

func (c *cmdAdminBMCRecordingDelete) run(cmd *cobra.Command, args []string) error {
	return c.ocClient.DeleteServerBMCRecording(cmd.Context(), args[0], args[1])
}
