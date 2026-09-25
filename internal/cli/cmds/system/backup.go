package system

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/lxc/incus/v7/shared/units"
	"github.com/spf13/cobra"

	"github.com/FuturFusion/operations-center/internal/cli/validate"
	"github.com/FuturFusion/operations-center/internal/client"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/render"
	"github.com/FuturFusion/operations-center/shared/api/system"
)

type CmdBackup struct {
	OCClient *client.OperationsCenterClient

	flagComplete    bool
	flagWithImages  bool
	flagWithUpdates bool
}

func (c *CmdBackup) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "backup <target-file.tar.gz>"
	cmd.Short = "Create a backup of operations-center"
	cmd.Long = `Description:
  Create a backup of operations-center

  The backup holds the configuration, the certificates and keys and the
  database of operations-center. The inventory is not part of the backup,
  it is synced again from the clusters after a restore.

  The images and the cached update files are only included on request.

  The backup contains secrets, it has to be stored safely.
`

	cmd.Flags().BoolVar(&c.flagWithImages, "with-images", false, "Include the images in the backup")
	cmd.Flags().BoolVar(&c.flagWithUpdates, "with-updates", false, "Include the cached update files in the backup")
	cmd.Flags().BoolVar(&c.flagComplete, "complete", false, "Include the images and the cached update files in the backup")

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *CmdBackup) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *CmdBackup) run(cmd *cobra.Command, args []string) (err error) {
	targetFilename := args[0]

	if file.PathExists(targetFilename) {
		return fmt.Errorf("target file %q already exists", targetFilename)
	}

	backupReader, err := c.OCClient.GetSystemBackup(cmd.Context(), system.BackupPost{
		Complete:    c.flagComplete,
		WithImages:  c.flagWithImages,
		WithUpdates: c.flagWithUpdates,
	})
	if err != nil {
		return err
	}

	defer func() {
		closeErr := backupReader.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

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
	progress, writer := render.ProgressWriter(targetFile, "Fetching backup: %s", quiet)

	size, err := file.SafeCopy(writer, backupReader)
	if err != nil {
		return err
	}

	// A backup, which failed on the server while being streamed, is incomplete.
	err = verifyArchive(targetFile)
	if err != nil {
		return fmt.Errorf("received backup is not a valid archive, the backup might have failed on the server: %w", err)
	}

	progress.Done(fmt.Sprintf("Successfully written %s to %q", units.GetByteSizeString(size, 2), targetFilename))

	return nil
}

// verifyArchive reads the gzip compressed tar archive in r to the end.
func verifyArchive(r io.ReadSeeker) error {
	_, err := r.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gzr)
	for {
		_, err = tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}
	}

	// Read to the end, to verify the checksum and to detect trailing data.
	_, err = io.Copy(io.Discard, gzr)
	if err != nil {
		return err
	}

	return gzr.Close()
}

type CmdRestore struct {
	OCClient *client.OperationsCenterClient
}

func (c *CmdRestore) Command() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "restore <backup-file.tar.gz>"
	cmd.Short = "Restore a backup of operations-center"
	cmd.Long = `Description:
  Restore a backup of operations-center

  The backup is validated, before operations-center restarts to apply it. It
  replaces the complete state of operations-center. Operations, which have
  been in progress, when the backup has been created, are aborted.

  A restore is refused, while servers are being deployed, updated, evacuated
  or restored or while cluster updates are in progress.
`

	cmd.PreRunE = c.validateArgsAndFlags
	cmd.RunE = c.run

	return cmd
}

func (c *CmdRestore) validateArgsAndFlags(cmd *cobra.Command, args []string) error {
	// Quick checks.
	exit, err := validate.Args(cmd, args, 1, 1)
	if exit {
		return err
	}

	return nil
}

func (c *CmdRestore) run(cmd *cobra.Command, args []string) (err error) {
	backupFile, err := os.Open(args[0])
	if err != nil {
		return err
	}

	defer func() {
		closeErr := backupFile.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	err = c.OCClient.RestoreSystemBackup(cmd.Context(), backupFile)
	if err != nil {
		return err
	}

	fmt.Println("Restore staged, operations-center is restarting")

	return nil
}
