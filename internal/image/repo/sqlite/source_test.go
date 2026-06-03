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
	"github.com/FuturFusion/operations-center/shared/api"
)

func TestImageSourceDatabaseActions(t *testing.T) {
	imageSourceA := image.ImageSource{
		Name:             "linuxcontainers.org",
		URL:              "https://images.linuxcontainers.org",
		Type:             api.ImageSourceTypeIncus,
		FilterExpression: `Architecture == "amd64"`,
	}

	imageSourceB := image.ImageSource{
		Name:             "images.org",
		URL:              "https://images.org",
		Type:             api.ImageSourceTypeIncus,
		FilterExpression: ``,
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
	imageSource := sqlite.NewImageSource(tx)

	// Add image source
	_, err = imageSource.Create(ctx, imageSourceA)
	require.NoError(t, err)
	_, err = imageSource.Create(ctx, imageSourceB)
	require.NoError(t, err)

	// Ensure we have two entries
	imageSources, err := imageSource.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, imageSources, 2)

	imageSourceIDs, err := imageSource.GetAllNames(ctx)
	require.NoError(t, err)
	require.Len(t, imageSourceIDs, 2)
	require.ElementsMatch(t, []string{"linuxcontainers.org", "images.org"}, imageSourceIDs)

	// Should get back imageSourceA unchanged.
	dbImageSourceA, err := imageSource.GetByName(ctx, imageSourceA.Name)
	require.NoError(t, err)
	imageSourceA.ID = dbImageSourceA.ID
	imageSourceA.LastUpdated = dbImageSourceA.LastUpdated
	require.Equal(t, imageSourceA, *dbImageSourceA)

	dbImageSourceB, err := imageSource.GetByName(ctx, imageSourceB.Name)
	require.NoError(t, err)
	imageSourceB.ID = dbImageSourceB.ID
	imageSourceB.LastUpdated = dbImageSourceB.LastUpdated
	require.Equal(t, imageSourceB, *dbImageSourceB)

	// Test updating a image source.
	imageSourceB.URL = "https://new.images.org"
	err = imageSource.Update(ctx, imageSourceB)
	require.NoError(t, err)
	dbImageSourceB, err = imageSource.GetByName(ctx, imageSourceB.Name)
	require.NoError(t, err)
	imageSourceB.ID = dbImageSourceB.ID
	imageSourceB.LastUpdated = dbImageSourceB.LastUpdated
	require.Equal(t, imageSourceB, *dbImageSourceB)

	// Delete a image source.
	err = imageSource.DeleteByName(ctx, imageSourceA.Name)
	require.NoError(t, err)
	_, err = imageSource.GetByName(ctx, imageSourceA.Name)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Should have one image sources remaining.
	imageSources, err = imageSource.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, imageSources, 1)

	// Can't delete a image source that doesn't exist.
	err = imageSource.DeleteByName(ctx, "three")
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't update a image source that doesn't exist.
	err = imageSource.Update(ctx, imageSourceA)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// Can't add a duplicate image source.
	_, err = imageSource.Create(ctx, imageSourceB)
	require.ErrorIs(t, err, domain.ErrConstraintViolation)
}
