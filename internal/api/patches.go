package api

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
	imageSqlite "github.com/FuturFusion/operations-center/internal/image/repo/sqlite"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/file"
	"github.com/FuturFusion/operations-center/internal/util/logger"
)

type patchStage int

// Define the stages that patches can run at.
const (
	patchNoStageSet patchStage = iota
	patchPreSecurityInfrastructure
)

/*
Patches are one-time actions that are sometimes needed to update:

  - move things around on the filesystem
  - migrate settings in the configuration

Those patches are applied at startup time after the database schema
has been fully updated. Patches can therefore assume a working database.
A patch and the record, that it is applied, run in the same transaction.
Changes on the filesystem can not be rolled back, so patches must be
safe to run again.

DO NOT use this mechanism for database schema updates. Schema updates must be
done through the separate schema update mechanism.

Only append to the patches list, never remove entries and never re-order them.
*/
var patches = []patch{
	{name: "rename_channels_to_upstream_channels", stage: patchPreSecurityInfrastructure, run: patchRenameChannelsToUpstreamChannels},
	{name: "remove_var_lib_operations_center_terraform_dirs", stage: patchPreSecurityInfrastructure, run: patchRemoveVarLibOperationCenterTerraformDirs},
	{name: "incus_images_per_source_directory", stage: patchPreSecurityInfrastructure, run: patchIncusImagesPerSourceDirectory},
}

type patchRun func(ctx context.Context, name string, d *Daemon, db dbdriver.DBTX) error

type patch struct {
	name  string
	stage patchStage
	run   patchRun
}

func (p *patch) apply(ctx context.Context, d *Daemon, db dbdriver.DBTX) error {
	slog.InfoContext(ctx, "Applying patch", slog.String("name", p.name))

	return transaction.Do(ctx, func(ctx context.Context) error {
		err := p.run(ctx, p.name, d, db)
		if err != nil {
			return fmt.Errorf("Failed applying patch %q: %w", p.name, err)
		}

		err = markPatchAsApplied(ctx, db, p.name)
		if err != nil {
			return fmt.Errorf("Failed marking patch applied %q: %w", p.name, err)
		}

		return nil
	})
}

// getAppliedPatches returns the names of all patches currently applied on this system.
func getAppliedPatches(ctx context.Context, db dbdriver.DBTX) ([]string, error) {
	var response []string

	rows, err := db.QueryContext(ctx, "SELECT name FROM patches")
	if err != nil {
		return []string{}, fmt.Errorf("Failed to get applied patches: %w", err)
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return []string{}, fmt.Errorf("Failed to scan applied patches from sql rows: %w", err)
		}

		response = append(response, name)
	}

	err = rows.Err()
	if err != nil {
		return []string{}, fmt.Errorf("Failed to iterate applied patches: %w", err)
	}

	return response, nil
}

// markPatchAsApplied marks the patch with the given name as applied on this system.
func markPatchAsApplied(ctx context.Context, db dbdriver.DBTX, name string) error {
	stmt := `INSERT INTO patches (name, applied_at) VALUES (?, strftime("%s"))`
	_, err := db.ExecContext(ctx, stmt, name)
	return err
}

// Return the names of all available patches.
func patchesGetNames() []string {
	names := make([]string, len(patches))
	for i, patch := range patches {
		if patch.stage == patchNoStageSet {
			continue // Ignore any patch without explicitly set stage (it is defined incorrectly).
		}

		names[i] = patch.name
	}

	return names
}

// patchesApply applies the patches for the respective stage.
func patchesApply(ctx context.Context, d *Daemon, db dbdriver.DBTX, stage patchStage) error {
	appliedPatches, err := getAppliedPatches(ctx, db)
	if err != nil {
		return err
	}

	for _, patch := range patches {
		if patch.stage == patchNoStageSet {
			return fmt.Errorf("Patch %q has no stage set: %d", patch.name, patch.stage)
		}

		if patch.stage != stage {
			continue
		}

		if slices.Contains(appliedPatches, patch.name) {
			continue
		}

		err := patch.apply(ctx, d, db)
		if err != nil {
			return err
		}
	}

	return nil
}

func patchRenameChannelsToUpstreamChannels(ctx context.Context, name string, d *Daemon, db dbdriver.DBTX) error {
	updatesCfg := config.GetUpdates()

	updatesCfg.FilterExpression = strings.ReplaceAll(updatesCfg.FilterExpression, "channels", "upstream_channels")
	return config.UpdateUpdates(ctx, updatesCfg.UpdatesPut)
}

func patchRemoveVarLibOperationCenterTerraformDirs(ctx context.Context, name string, d *Daemon, db dbdriver.DBTX) error {
	err := d.varRoot.RemoveAll("servercerts")
	if err != nil {
		return err
	}

	return d.varRoot.RemoveAll("terraform")
}

func patchIncusImagesPerSourceDirectory(ctx context.Context, name string, d *Daemon, db dbdriver.DBTX) error {
	// Move the files from "images/<os>/<release>/<arch>/<variant>" to
	// "images/<source>/<os>/<release>/<arch>/<variant>". Before, images of
	// different sources could share a directory.
	const newDir = "images"

	// A source can have the same name as an operating system, so the old tree
	// moves to "images/.v1" first. Source names cannot contain ".". The old tree
	// stays inside "images", because "images" might be a mount point.
	oldDir := filepath.Join(newDir, ".v1")

	repo := imageSqlite.NewIncusImage(db)

	images, err := repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("Failed to get incus images: %w", err)
	}

	oldExists, err := file.ExistsIn(d.varRoot, oldDir)
	if err != nil {
		return fmt.Errorf("Failed to check %q: %w", oldDir, err)
	}

	if !oldExists {
		newExists, err := file.ExistsIn(d.varRoot, newDir)
		if err != nil {
			return fmt.Errorf("Failed to check %q: %w", newDir, err)
		}

		if !newExists {
			return nil
		}

		// Only move the directories of the images. Other entries stay, for
		// example "lost+found" of a mount point.
		err = moveDirEntries(d.varRoot, newDir, oldDir, incusImageOperatingSystemDirs(images))
		if err != nil {
			return err
		}
	}

	// Images of other sources can be downloaded again. Local images cannot, so
	// they get the files first. Versions without files are removed.
	sortLocalIncusImagesFirst(images)

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
			moved, err := file.ExistsIn(d.varRoot, newVersionDir)
			if err != nil {
				return fmt.Errorf("Failed to check %q: %w", newVersionDir, err)
			}

			if moved {
				continue
			}

			oldVersionDir := filepath.Join(oldImageDir, versionIdentifier)
			exists, err := file.ExistsIn(d.varRoot, oldVersionDir)
			if err != nil {
				return fmt.Errorf("Failed to check %q: %w", oldVersionDir, err)
			}

			if !exists {
				slog.WarnContext(ctx, "Removed incus image version without files", slog.String("source", img.Source), slog.String("image", img.Name), slog.String("version", versionIdentifier))
				delete(img.Versions, versionIdentifier)
				removedVersions = true
				continue
			}

			err = d.varRoot.MkdirAll(newImageDir, 0o700)
			if err != nil {
				return fmt.Errorf("Failed to create %q: %w", newImageDir, err)
			}

			err = d.varRoot.Rename(oldVersionDir, newVersionDir)
			if err != nil {
				return fmt.Errorf("Failed to move %q to %q: %w", oldVersionDir, newVersionDir, err)
			}
		}

		if removedVersions {
			err = repo.Update(ctx, img)
			if err != nil {
				return fmt.Errorf("Failed to update incus image %q of source %q: %w", img.Name, img.Source, err)
			}
		}
	}

	// Remove the old tree only after the commit. A failed patch runs again and
	// still needs the old files. The patch is applied at this point, so a failed
	// removal must not fail the startup.
	return transaction.AfterCommit(ctx, func() error {
		logRemovedFiles(ctx, d.varRoot, oldDir)

		err := d.varRoot.RemoveAll(oldDir)
		if err != nil {
			slog.WarnContext(ctx, "Failed to remove old incus image directory, remove it manually", slog.String("patch", name), slog.String("path", oldDir), logger.Err(err))
		}

		return nil
	})
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
