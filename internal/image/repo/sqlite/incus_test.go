package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/dbschema"
	dbdriver "github.com/FuturFusion/operations-center/internal/sql/sqlite"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func TestIncusImageDatabaseActions(t *testing.T) {
	incusImageA := image.IncusImage{
		Name:            "almalinux:10:amd64:default",
		OperatingSystem: "almalinux",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "default",
		Description:     "almalinux 10 (default) (amd64)",
	}

	incusImageB := image.IncusImage{
		Name:            "almalinux:10:amd64:cloud",
		OperatingSystem: "almalinux",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "cloud",
		Description:     "almalinux 10 (cloud) (amd64)",
	}

	incusImageC := image.IncusImage{
		Name:            "rocky:10:amd64:cloud",
		OperatingSystem: "rocky",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "cloud",
		Description:     "rocky 10 (cloud) (amd64)",
	}

	ctx := context.Background()

	// Create a new temporary database.
	tmpDir := t.TempDir()
	db, err := dbdriver.Open(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	_, err = dbschema.Ensure(ctx, db, tmpDir)
	require.NoError(t, err)

	tx := transaction.Enable(db)
	entities.PreparedStmts, err = entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	// update := sqlite.NewUpdate(tx)
	incusImage := sqlite.NewIncusImage(tx)

	// Add incusImage
	_, err = incusImage.Create(ctx, incusImageA)
	require.NoError(t, err)
	_, err = incusImage.Create(ctx, incusImageB)
	require.NoError(t, err)

	// Ensure we have two entries
	incusImages, err := incusImage.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, incusImages, 2)

	incusImages, err = incusImage.GetAllWithFilter(ctx, image.IncusImageFilter{
		Name: ptr.To("almalinux:10:amd64:default"),
	})
	require.NoError(t, err)
	require.Len(t, incusImages, 1)

	incusImageIDs, err := incusImage.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, incusImageIDs, 2)
	require.ElementsMatch(t, []string{"almalinux:10:amd64:default", "almalinux:10:amd64:cloud"}, incusImageIDs)

	dbIncusImageAExists, err := incusImage.ExistsByName(ctx, incusImageA.Name)
	require.NoError(t, err)
	require.True(t, dbIncusImageAExists)

	// Should get back incusImageA unchanged.
	dbIncusImageA, err := incusImage.GetByName(ctx, incusImageA.Name)
	require.NoError(t, err)
	incusImageA.ID = dbIncusImageA.ID
	incusImageA.LastUpdated = dbIncusImageA.LastUpdated
	require.Equal(t, incusImageA, *dbIncusImageA)

	dbIncusImageB, err := incusImage.GetByName(ctx, incusImageB.Name)
	require.NoError(t, err)
	incusImageB.ID = dbIncusImageB.ID
	incusImageB.LastUpdated = dbIncusImageB.LastUpdated
	require.Equal(t, incusImageB, *dbIncusImageB)

	// Test updating a incusImage.
	incusImageB.Description = "description (updated)"
	err = incusImage.Update(ctx, incusImageB)
	require.NoError(t, err)
	dbIncusImageB, err = incusImage.GetByName(ctx, incusImageB.Name)
	require.NoError(t, err)
	incusImageB.ID = dbIncusImageB.ID
	incusImageB.LastUpdated = dbIncusImageB.LastUpdated
	require.Equal(t, incusImageB, *dbIncusImageB)

	// Delete a incusImage.
	err = incusImage.DeleteByName(ctx, incusImageA.Name)
	require.NoError(t, err)
	_, err = incusImage.GetByName(ctx, incusImageA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one incusImages remaining.
	incusImages, err = incusImage.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, incusImages, 1)

	err = incusImage.Upsert(ctx, incusImageC)
	require.NoError(t, err)

	// Can't delete a incusImage that doesn't exist.
	err = incusImage.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a incusImage that doesn't exist.
	err = incusImage.Update(ctx, incusImageA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate incusImage.
	_, err = incusImage.Create(ctx, incusImageB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
