package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	config "github.com/FuturFusion/operations-center/internal/config/daemon"
	dbdriver "github.com/FuturFusion/operations-center/internal/sqlite"
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

DO NOT use this mechanism for database update. Schema updates must be
done through the separate schema update mechanism.

Only append to the patches list, never remove entries and never re-order them.
*/
var patches = []patch{
	{name: "rename_channels_to_upstream_channels", stage: patchPreSecurityInfrastructure, run: patchRenameChannelsToUpstreamChannels},
	{name: "remove_var_lib_operations_center_terraform_dirs", stage: patchPreSecurityInfrastructure, run: patchRemoveVarLibOperationCenterTerraformDirs},
}

type patchRun func(ctx context.Context, name string) error

type patch struct {
	name  string
	stage patchStage
	run   patchRun
}

func (p *patch) apply(ctx context.Context, db dbdriver.DBTX) error {
	slog.InfoContext(ctx, "Applying patch", slog.String("name", p.name))

	err := p.run(ctx, p.name)
	if err != nil {
		return fmt.Errorf("Failed applying patch %q: %w", p.name, err)
	}

	err = markPatchAsApplied(ctx, db, p.name)
	if err != nil {
		return fmt.Errorf("Failed marking patch applied %q: %w", p.name, err)
	}

	return nil
}

// getAppliedPatches returns the names of all patches currently applied on this system.
func getAppliedPatches(ctx context.Context, db dbdriver.DBTX) ([]string, error) {
	var response []string

	rows, err := db.QueryContext(ctx, "SELECT name FROM patches")
	if err != nil {
		return []string{}, fmt.Errorf("Failed to get applied patches: %w", err)
	}

	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return []string{}, fmt.Errorf("Failed to scan applied patches from sql rows: %w", err)
		}

		response = append(response, name)
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
func patchesApply(ctx context.Context, db dbdriver.DBTX, stage patchStage) error {
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

		err := patch.apply(ctx, db)
		if err != nil {
			return err
		}
	}

	return nil
}

func patchRenameChannelsToUpstreamChannels(ctx context.Context, name string) error {
	updatesCfg := config.GetUpdates()

	updatesCfg.FilterExpression = strings.ReplaceAll(updatesCfg.FilterExpression, "channels", "upstream_channels")
	return config.UpdateUpdates(ctx, updatesCfg.SystemUpdatesPut)
}

func patchRemoveVarLibOperationCenterTerraformDirs(ctx context.Context, name string) error {
	err := os.RemoveAll("/var/lib/operations-center/servercerts")
	if err != nil {
		return err
	}

	return os.RemoveAll("/var/lib/operations-center/terraform")
}
