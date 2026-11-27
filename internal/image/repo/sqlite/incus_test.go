package sqlite_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/image"
	"github.com/FuturFusion/operations-center/internal/image/repo/sqlite"
	"github.com/FuturFusion/operations-center/internal/image/repo/sqlite/entities"
	"github.com/FuturFusion/operations-center/internal/sql/cowsql/cowsqltest"
	"github.com/FuturFusion/operations-center/internal/sql/transaction"
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestIncusImageDatabaseActions(t *testing.T) {
	t.Parallel()

	localImageA := image.IncusImage{
		Source:          api.IncusImageSourceLocal,
		Name:            "almalinux:10:amd64:default",
		OperatingSystem: "almalinux",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "default",
		Description:     "almalinux 10 (default) (amd64)",
	}

	localImageB := image.IncusImage{
		Source:          api.IncusImageSourceLocal,
		Name:            "almalinux:10:amd64:cloud",
		OperatingSystem: "almalinux",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "cloud",
		Description:     "almalinux 10 (cloud) (amd64)",
	}

	sourceImageB := image.IncusImage{
		Source:          "images",
		Name:            "almalinux:10:amd64:cloud",
		OperatingSystem: "almalinux",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "cloud",
		Description:     "almalinux 10 (cloud) (amd64) from images",
	}

	sourceImageC := image.IncusImage{
		Source:          "images",
		Name:            "rocky:10:amd64:cloud",
		OperatingSystem: "rocky",
		Release:         "10",
		Architecture:    "amd64",
		Variant:         "cloud",
		Description:     "rocky 10 (cloud) (amd64)",
	}

	ctx := context.Background()

	// Create a new temporary database.
	db, err := cowsqltest.Open(t)
	require.NoError(t, err)

	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	tx := transaction.Enable(db)
	stmts, err := entities.PrepareStmts(tx.(transaction.Preparer), false)
	require.NoError(t, err)

	ctx = entities.WithPreparedStmts(ctx, stmts)

	_, err = sqlite.NewImageSource(tx).Create(ctx, image.IncusImageSource{
		Name:             "images",
		URL:              "https://images.example.com/",
		FilterExpression: "true",
	})
	require.NoError(t, err)

	incusImage := sqlite.NewIncusImage(tx)

	// Add images. The same name exists in two sources.
	for _, img := range []image.IncusImage{localImageA, localImageB, sourceImageB} {
		_, err = incusImage.Create(ctx, img)
		require.NoError(t, err)
	}

	incusImages, err := incusImage.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, incusImages, 3, "the same name can exist in different sources")

	incusImages, err = incusImage.GetAllWithFilter(ctx, image.IncusImageFilter{
		Source: new(api.IncusImageSourceLocal),
	})
	require.NoError(t, err)
	require.Len(t, incusImages, 2, "the filter by source only returns the images of this source")

	exists, err := incusImage.ExistsByName(ctx, api.IncusImageSourceLocal, localImageA.Name)
	require.NoError(t, err)
	require.True(t, exists, "the image exists in the local source")

	exists, err = incusImage.ExistsByName(ctx, "images", localImageA.Name)
	require.NoError(t, err)
	require.False(t, exists, "the image does not exist in the other source")

	// Should get back the images unchanged.
	for _, want := range []image.IncusImage{localImageA, localImageB, sourceImageB} {
		got, err := incusImage.GetByName(ctx, want.Source, want.Name)
		require.NoError(t, err)
		want.ID = got.ID
		want.LastUpdated = got.LastUpdated
		require.Equal(t, want, *got, "the image is found by source and name")
	}

	// Test updating an image.
	sourceImageB.Description = "description (updated)"
	err = incusImage.Update(ctx, sourceImageB)
	require.NoError(t, err)

	dbLocalImageB, err := incusImage.GetByName(ctx, localImageB.Source, localImageB.Name)
	require.NoError(t, err)
	require.Equal(t, localImageB.Description, dbLocalImageB.Description, "the update only changes the image of the given source")

	dbSourceImageB, err := incusImage.GetByName(ctx, sourceImageB.Source, sourceImageB.Name)
	require.NoError(t, err)
	require.Equal(t, sourceImageB.Description, dbSourceImageB.Description, "the update changes the image of the given source")

	// Upsert only replaces the image of the same source.
	sourceImageB.Description = "description (upserted)"
	err = incusImage.Upsert(ctx, sourceImageB)
	require.NoError(t, err)

	dbLocalImageB, err = incusImage.GetByName(ctx, localImageB.Source, localImageB.Name)
	require.NoError(t, err)
	require.Equal(t, localImageB.Description, dbLocalImageB.Description, "the upsert does not replace the image of another source")

	err = incusImage.Upsert(ctx, sourceImageC)
	require.NoError(t, err)

	// Delete an image.
	err = incusImage.DeleteByName(ctx, localImageA.Source, localImageA.Name)
	require.NoError(t, err)
	_, err = incusImage.GetByName(ctx, localImageA.Source, localImageA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	incusImages, err = incusImage.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, incusImages, 3, "local image B and the source images B and C remain")

	// Can't delete an image that doesn't exist.
	err = incusImage.DeleteByName(ctx, "images", localImageA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update an image that doesn't exist.
	err = incusImage.Update(ctx, localImageA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate image.
	_, err = incusImage.Create(ctx, localImageB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)

	// Can't add an image of an unknown source.
	unknownSourceImage := localImageA
	unknownSourceImage.Source = "unknown"
	_, err = incusImage.Create(ctx, unknownSourceImage)
	require.Error(t, err, "an image needs an existing source")
}
