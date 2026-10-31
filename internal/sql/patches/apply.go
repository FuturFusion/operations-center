package patches

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"

	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
)

type patchRun func(ctx context.Context, name string, varRoot *os.Root) error

type patch struct {
	name  string
	stage patchStage
	scope patchScope
	run   patchRun
}

// apply runs a member patch and records it in the local database of this member.
func (p *patch) apply(ctx context.Context, db dbdriver.DBTX, varRoot *os.Root) error {
	slog.InfoContext(ctx, "Applying patch", slog.String("name", p.name))

	err := p.run(ctx, p.name, varRoot)
	if err != nil {
		return fmt.Errorf("Failed applying patch %q: %w", p.name, err)
	}

	err = MarkApplied(ctx, db, p.name)
	if err != nil {
		return fmt.Errorf("Failed marking patch applied %q: %w", p.name, err)
	}

	return nil
}

// applyOnce runs a cluster patch, unless another cluster member claimed it in the global database first.
func (p *patch) applyOnce(ctx context.Context, db dbdriver.DBTX, varRoot *os.Root) error {
	result, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO patches (name, applied_at) VALUES (?, strftime("%s"))`, p.name)
	if err != nil {
		return fmt.Errorf("Failed claiming patch %q: %w", p.name, err)
	}

	claimed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("Failed claiming patch %q: %w", p.name, err)
	}

	if claimed == 0 {
		return nil
	}

	slog.InfoContext(ctx, "Applying patch", slog.String("name", p.name))

	err = p.run(ctx, p.name, varRoot)
	if err != nil {
		// The next start runs the patch again.
		_, releaseErr := db.ExecContext(ctx, `DELETE FROM patches WHERE name = ?`, p.name)

		return errors.Join(fmt.Errorf("Failed applying patch %q: %w", p.name, err), releaseErr)
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
		return []string{}, fmt.Errorf("Failed to read applied patches: %w", err)
	}

	return response, nil
}

// MarkApplied marks the patch with the given name as applied on this system.
func MarkApplied(ctx context.Context, db dbdriver.DBTX, name string) error {
	stmt := `INSERT INTO patches (name, applied_at) VALUES (?, strftime("%s"))`
	_, err := db.ExecContext(ctx, stmt, name)
	return err
}

// GetNames returns the names of all available patches.
func GetNames() []string {
	names := make([]string, len(patches))
	for i, patch := range patches {
		if patch.stage == noStageSet {
			continue // Ignore any patch without explicitly set stage (it is defined incorrectly).
		}

		names[i] = patch.name
	}

	return names
}

// Apply applies the patches for the respective stage, which access the var dir through varRoot. Cluster patches are recorded in the global database db, member patches in the local database localDB.
func Apply(ctx context.Context, db dbdriver.DBTX, localDB dbdriver.DBTX, varRoot *os.Root, stage patchStage) error {
	appliedPatches, err := getAppliedPatches(ctx, db)
	if err != nil {
		return err
	}

	appliedMemberPatches, err := getAppliedPatches(ctx, localDB)
	if err != nil {
		return err
	}

	for _, patch := range patches {
		if patch.stage == noStageSet {
			return fmt.Errorf("Patch %q has no stage set: %d", patch.name, patch.stage)
		}

		if patch.stage != stage {
			continue
		}

		if patch.scope == scopeMember {
			if slices.Contains(appliedMemberPatches, patch.name) {
				continue
			}

			err := patch.apply(ctx, localDB, varRoot)
			if err != nil {
				return err
			}

			continue
		}

		if slices.Contains(appliedPatches, patch.name) {
			continue
		}

		err := patch.applyOnce(ctx, db, varRoot)
		if err != nil {
			return err
		}
	}

	return nil
}

func List() []string {
	names := make([]string, 0, len(patches))
	for _, p := range patches {
		names = append(names, p.name)
	}

	return names
}
