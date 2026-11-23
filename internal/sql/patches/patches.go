package patches

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	"github.com/FuturFusion/operations-center/internal/image"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/shared/api"
)

type patchStage int

// Define the stages that patches can run at.
const (
	noStageSet patchStage = iota
	PreSecurityInfrastructure
)

type patchScope int

const (
	// scopeCluster patches change the database or the shared config and run once per cluster.
	scopeCluster patchScope = iota

	// scopeMember patches change the files of a member and run once on every member.
	scopeMember
)

/*
Patches are one-time actions that are sometimes needed to update:

  - move things around on the filesystem
  - migrate settings in the configuration

Those patches are applied at startup time after the database schema
has been fully updated. Patches can therefore assume a working database.
A cluster patch and the record, that it is applied, run in the same transaction.
Changes on the filesystem can not be rolled back, so patches must be
safe to run again.

DO NOT use this mechanism for database schema updates. Schema updates must be
done through the separate schema update mechanism.

Only append to the patches list, never remove entries and never re-order them.
*/
var patches = []patch{
	{name: "rename_channels_to_upstream_channels", stage: PreSecurityInfrastructure, scope: scopeCluster, run: patchRenameChannelsToUpstreamChannels},
	{name: "remove_var_lib_operations_center_terraform_dirs", stage: PreSecurityInfrastructure, scope: scopeMember, run: patchRemoveVarLibOperationCenterTerraformDirs},
	{name: "incus_images_per_source_directory", stage: PreSecurityInfrastructure, scope: scopeCluster, run: patchIncusImagesPerSourceDirectory},
}

func patchRenameChannelsToUpstreamChannels(ctx context.Context, name string, _ dbdriver.DBTX, _ *os.Root) error {
	updatesCfg := config.GetUpdates()

	updatesCfg.FilterExpression = strings.ReplaceAll(updatesCfg.FilterExpression, "channels", "upstream_channels")
	return config.UpdateUpdates(ctx, updatesCfg.UpdatesPut)
}

func patchRemoveVarLibOperationCenterTerraformDirs(ctx context.Context, name string, _ dbdriver.DBTX, varRoot *os.Root) error {
	err := varRoot.RemoveAll("servercerts")
	if err != nil {
		return err
	}

	return varRoot.RemoveAll("terraform")
}

func patchIncusImagesPerSourceDirectory(ctx context.Context, name string, db dbdriver.DBTX, varRoot *os.Root) error {
	// Move the files from "images/<os>/<release>/<arch>/<variant>" to
	// "images/<source>/<os>/<release>/<arch>/<variant>". Before, images of
	// different sources could share a directory.
	const newDir = "images"

	// A source can have the same name as an operating system, so the old tree
	// moves to "images/.v1" first. Source names cannot contain ".". The old tree
	// stays inside "images", because "images" might be a mount point.
	oldDir := filepath.Join(newDir, ".v1")

	images, err := getIncusImagesLocalFirst(ctx, db)
	if err != nil {
		return err
	}

	oldExists, err := file.ExistsIn(varRoot, oldDir)
	if err != nil {
		return fmt.Errorf("Failed to check %q: %w", oldDir, err)
	}

	if !oldExists {
		newExists, err := file.ExistsIn(varRoot, newDir)
		if err != nil {
			return fmt.Errorf("Failed to check %q: %w", newDir, err)
		}

		if !newExists {
			return nil
		}

		// Only move the directories of the images. Other entries stay, for
		// example "lost+found" of a mount point.
		err = moveDirEntries(varRoot, newDir, oldDir, incusImageOperatingSystemDirs(images))
		if err != nil {
			return err
		}
	}

	// Versions without files are removed.
	for _, img := range images {
		oldImageDir := filepath.Join(oldDir, strings.ToLower(img.OperatingSystem), strings.ToLower(img.Release), strings.ToLower(img.Architecture), strings.ToLower(img.Variant))
		newImageDir := filepath.Join(newDir, img.FilePath())

		validImagePath := isIncusImagePathValid(img)

		removedVersions := false
		for versionIdentifier := range img.Versions {
			if !validImagePath || !file.IsPathElement(versionIdentifier) {
				slog.WarnContext(ctx, "Removed incus image version with an invalid path", slog.String("source", img.Source), slog.String("image", img.Name), slog.String("version", versionIdentifier))
				delete(img.Versions, versionIdentifier)
				removedVersions = true
				continue
			}

			newVersionDir := filepath.Join(newImageDir, versionIdentifier)
			moved, err := file.ExistsIn(varRoot, newVersionDir)
			if err != nil {
				return fmt.Errorf("Failed to check %q: %w", newVersionDir, err)
			}

			if moved {
				continue
			}

			oldVersionDir := filepath.Join(oldImageDir, versionIdentifier)
			exists, err := file.ExistsIn(varRoot, oldVersionDir)
			if err != nil {
				return fmt.Errorf("Failed to check %q: %w", oldVersionDir, err)
			}

			if !exists {
				slog.WarnContext(ctx, "Removed incus image version without files", slog.String("source", img.Source), slog.String("image", img.Name), slog.String("version", versionIdentifier))
				delete(img.Versions, versionIdentifier)
				removedVersions = true
				continue
			}

			err = varRoot.MkdirAll(newImageDir, 0o700)
			if err != nil {
				return fmt.Errorf("Failed to create %q: %w", newImageDir, err)
			}

			err = varRoot.Rename(oldVersionDir, newVersionDir)
			if err != nil {
				return fmt.Errorf("Failed to move %q to %q: %w", oldVersionDir, newVersionDir, err)
			}
		}

		if removedVersions {
			_, err = transaction.GetDBTX(ctx, db).ExecContext(ctx, `UPDATE incus_images SET versions = ? WHERE id = ?`, img.Versions, img.ID)
			if err != nil {
				return fmt.Errorf("Failed to update incus image %q of source %q: %w", img.Name, img.Source, err)
			}
		}
	}

	// Remove the old tree only after the commit. A failed patch runs again and
	// still needs the old files. The patch is applied at this point, so a failed
	// removal must not fail the startup.
	return transaction.AfterCommit(ctx, func(ctx context.Context) error {
		logRemovedFiles(ctx, varRoot, oldDir)

		err := varRoot.RemoveAll(oldDir)
		if err != nil {
			slog.WarnContext(ctx, "Failed to remove old incus image directory, remove it manually", slog.String("patch", name), slog.String("path", oldDir), logger.Err(err))
		}

		return nil
	})
}

// getIncusImagesLocalFirst returns the incus images. Images of other sources
// can be downloaded again. Local images cannot, so they get the files first.
func getIncusImagesLocalFirst(ctx context.Context, db dbdriver.DBTX) (image.IncusImages, error) {
	rows, err := transaction.GetDBTX(ctx, db).QueryContext(ctx, `
SELECT incus_images.id, incus_image_sources.name, incus_images.name, operating_system, release, architecture, variant, versions
  FROM incus_images
  JOIN incus_image_sources ON incus_images.incus_image_source_id = incus_image_sources.id
  ORDER BY incus_image_sources.name <> ?, incus_images.id`, api.IncusImageSourceLocal)
	if err != nil {
		return nil, fmt.Errorf("Failed to get incus images: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var images image.IncusImages
	for rows.Next() {
		var img image.IncusImage
		err = rows.Scan(&img.ID, &img.Source, &img.Name, &img.OperatingSystem, &img.Release, &img.Architecture, &img.Variant, &img.Versions)
		if err != nil {
			return nil, fmt.Errorf("Failed to scan incus image: %w", err)
		}

		images = append(images, img)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("Failed to get incus images: %w", err)
	}

	return images, nil
}

func incusImageOperatingSystemDirs(images image.IncusImages) []string {
	dirs := make([]string, 0, len(images))
	for _, img := range images {
		dir := strings.ToLower(img.OperatingSystem)
		if file.IsPathElement(dir) {
			dirs = append(dirs, dir)
		}
	}

	slices.Sort(dirs)
	return slices.Compact(dirs)
}

func moveDirEntries(root *os.Root, dir string, target string, names []string) error {
	partialDir := target + ".partial"
	err := root.MkdirAll(partialDir, 0o700)
	if err != nil {
		return fmt.Errorf("Failed to create %q: %w", partialDir, err)
	}

	for _, name := range names {
		if name == filepath.Base(partialDir) {
			continue
		}

		err = root.Rename(filepath.Join(dir, name), filepath.Join(partialDir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return fmt.Errorf("Failed to move %q to %q: %w", name, partialDir, err)
		}
	}

	// The final rename marks the move as complete. A failed move continues on
	// the next run.
	err = root.Rename(partialDir, target)
	if err != nil {
		return fmt.Errorf("Failed to move %q to %q: %w", partialDir, target, err)
	}

	return nil
}

func logRemovedFiles(ctx context.Context, root *os.Root, dir string) {
	err := fs.WalkDir(root.FS(), dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		slog.WarnContext(ctx, "Removed incus image file without image", slog.String("path", path))
		return nil
	})
	if err != nil {
		slog.WarnContext(ctx, "Failed to list the removed incus image files", slog.String("path", dir), logger.Err(err))
	}
}

func isIncusImagePathValid(img image.IncusImage) bool {
	for _, element := range []string{img.Source, img.OperatingSystem, img.Release, img.Architecture, img.Variant} {
		if !file.IsPathElement(element) {
			return false
		}
	}

	return true
}
