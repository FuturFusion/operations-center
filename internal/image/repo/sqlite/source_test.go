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
)

func TestImageSourceDatabaseActions(t *testing.T) {
	t.Parallel()

	imageSourceA := image.IncusImageSource{
		Name:             "linuxcontainers.org",
		URL:              "https://images.linuxcontainers.org",
		FilterExpression: `Architecture == "amd64"`,
	}

	imageSourceB := image.IncusImageSource{
		Name:             "images.org",
		URL:              "https://images.org",
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
	stmts, err := entities.PrepareStmts(tx, false)
	require.NoError(t, err)

	ctx = entities.WithPreparedStmts(ctx, stmts)

	// update := sqlite.NewUpdate(tx)
	imageSource := sqlite.NewImageSource(tx)

	// Add image source
	_, err = imageSource.Create(ctx, imageSourceA)
	require.NoError(t, err)
	_, err = imageSource.Create(ctx, imageSourceB)
	require.NoError(t, err)

	imageSources, err := imageSource.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, imageSources, 3, "the two added sources and the local source of a fresh database")

	imageSourceIDs, err := imageSource.GetAllNames(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"linuxcontainers.org", "images.org", "local"}, imageSourceIDs, "the names of the added sources and the local source")

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

	imageSources, err = imageSource.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, imageSources, 2, "one added source and the local source remain")

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
